# v0.4「无限六边形世界」工程设计

```text
状态    草案（待总管拍板）
日期    2026-09-18
上游    world_rule v0.1 / world_laws v0.1 / world_genesis v0.1 / player_agent_relationship v0.1
前置    12×12 与 24×24 有限方形网格、BFS、move_to 订单、Logic 怪物、LLM 每回合决策
```

本文回答 v0.4 的四个问题：坐标怎么换（§1）、世界怎么无限（§2）、LLM 何时介入（§3）、前端怎么画（§4），并给出派工单（§5）与风险（§6）。

---

## 1. 六边形坐标系统

### 1.1 axial (q, r) 与存储布局

pointy-top，axial 坐标 `(q, r)`。距离与邻居用 cube 语义：

```text
HexDist(a, b) = (|dq| + |dr| + |dq+dr|) / 2        # 6 邻域上的真实步数
六邻居（固定顺序，替代 bfsDirs）：
  E(+1,0) NE(+1,-1) NW(0,-1) W(-1,0) SW(-1,+1) SE(0,+1)
```

**存储布局**：无限世界里不存在"全局 row-major 数组"。地形归 chunk（§2），
chunk 内按轴向平行四边形铺平：

```text
chunk (qc, rc) 持有 CS×CS = 12×12 格
局部索引 i∈[0,12), j∈[0,12)
cell(q, r) = Chunk[q/12, r/12].Cells[(r%12)*12 + (q%12)]   // Go 负数取模需 normalize
```

World 从 `W, H, Cells []Cell` 变为：

```go
type Hex struct{ Q, R int }          // 替代 Vec2，JSON 序列化为 {"q":..,"r":..}
type World struct {
    Tick    int64
    Seed    uint64
    Rng     *Rng
    Chunks  *ChunkCache   // 地形，按需生成 + LRU（§2.3）
    Entities map[EntityID]*Entity   // 实体：世界级存储，与 chunk 无关
    ...
}
```

`Vec2` 整体删除，不保留别名。JSON 坐标从 `{x, y}` 变 `{q, r}`，前后端契约同步升版（§6 风险三）。

### 1.2 迁移策略：全量切换，一次到位

**建议：直接全量切换，不做双坐标过渡层。** 理由：项目早期、调用面完全已知、
双坐标并存会让每个签名出现两种距离语义，是 bug 温床。

受影响文件清单（改动性质）：

| 文件 | 改动性质 |
|---|---|
| `backend/engine/world.go` | **重写坐标层**：Vec2→Hex；删 W/H/Cells；At/InBounds 改走 ChunkCache；Chebyshev→HexDist；Adjacent=HexDist==1；EntityAt/SortEntities 不变（已与坐标无关） |
| `backend/engine/sim.go` | **局部改**：resolveMove 邻接判定、freeAdjacent 六方向、project/spill 的作用距离改 HexDist；Advance/stepOrder 骨架不动 |
| `backend/engine/path.go` | **重写**：bfsDirs→六邻居；后续升级为 A*（§2.5） |
| `backend/engine/observation.go` | **局部改**：可见格从方形 dx/dy 循环改为 HexDist≤radius 的环遍历；实体的 Chebyshev 判距改 HexDist |
| `backend/engine/laws.go` | **基本不动**：克曲线/生热是纯数值，与几何无关（spill 溢出半径 Chebyshev≤1 → HexDist≤1，一处） |
| `backend/scenario/scenario.go` | **重写**：v0.1/v0.2 方形地形函数废弃；新增 v0.4 无限场景包（§2.4）；Genesis 系列实体装配逻辑保留 |
| `backend/agent/agent.go` | **重写 nextStep**：贪心方向改六方向；其余接口不变 |
| `backend/match/match.go` | **局部改**：坐标类型跟随；胜负判定坐标语义跟随 |
| `backend/match/api.go` | **契约改**：JSON 坐标 {x,y}→{q,r}，升 API 版本 |
| `frontend/h5/src/state.ts` | **契约改**：坐标字段与距离语义跟随 |
| `frontend/h5/src/views/MapView.vue` | **整体重写**（§4，canvas） |

**对"engine 测试全绿是安全网"的评估：部分成立，需修正表述。**

- 成立的部分：`laws_test.go`（法则数学）与 sim/path/observation 中**行为级断言**
  （订单推进、过热、形散、溢出）在把坐标常量翻译成 hex 语义后应当原样通过——
  这类测试是真正的安全网。
- 不成立的部分：`sim_test.go`、`v02_test.go`、`match_test.go` 里硬编码方形坐标、
  4 邻接步数、方形可达性的用例**必须改写**，不能只跑不改。安全网的正确表述是：
  **"改写后的测试全绿"**，且改写动作本身要在切换前完成一轮"语义翻译评审"
  （每个被改的断言注明 hex 下为什么等价）。
- 替代方案（若评审发现行为级断言占比过低）：先加 `hex.go` 与 Vec2 并存、
  引擎内部只暴露 Hex 接口、以 adapter 兼容一轮后再删 Vec2。
  成本是多一轮 PR 与临时双语义，仅作兜底。

### 1.3 像素换算（pointy-top，前端渲染用）

```text
设 hexsize = s（外接圆半径）
center_x = s * √3 * (q + r/2)
center_y = s * 3/2 * r

像素 → axial（拾取）：
  qf = (√3/3 * x − 1/3 * y) / s
  rf = (2/3 * y) / s
  再做 cube rounding：x=qf, z=rf, y=−x−z；round 后修正最大分量误差
```

Go 与 TS 各实现一份纯函数（同公式、同浮点顺序），并加交叉一致性测试（同一组
(q,r) 双端像素误差 < 0.5px）。

### 1.4 Adjacent / Chebyshev 的替代语义

| 旧语义 | 六边形下 |
|---|---|
| `Chebyshev(a,b)`（8 邻距离） | `HexDist(a,b)`（6 邻步数） |
| `Adjacent`（正交相邻） | `HexDist(a,b) == 1`（六个方向等价，无"斜角"歧义——六边形天然消灭了方形网格的角接触问题） |
| 传感器/检视范围 radius | `HexDist ≤ radius` 的所有格（环遍历） |
| 溢出波及 `≤1` | `HexDist ≤ 1`（六邻格） |

---

## 2. 无限世界生成（chunk 化）

### 2.1 chunk 尺寸

**建议 CS = 12（轴向平行四边形），144 hex/chunk。** 理由：

1. 与 v0.1 的 12×12 全图同规模——单 chunk 即"一代人的世界"，性能与心智都有锚；
2. 144 × Cell（约 4 字节）≈ 0.6KB/chunk，LRU 256 块 ≈ 150KB，内存无压力；
3. 12 是低频噪声格距（§2.2，L=24~32）的合理子采样：块内能呈现阴阳场的一个坡面，
   又不至于一块横跨多个地貌。

### 2.2 确定性生成函数：把 world_genesis §3.1 从"全局"改"逐块可计算"

核心技术动作：**把四步生成全部改写为以世界坐标为输入的纯函数**，
chunk 只负责枚举自己名下的 144 个 (q, r)。

```go
// F(seed, q, r) → Cell   纯函数，跨块连续，与 chunk 边界无关
func Terrain(seed uint64, q, r int) Cell
func Chunk(seed uint64, qc, rc int) *Chunk  // 对 144 格逐一调 Terrain
```

四步的逐块化方案：

```text
世界整数格坐标（线性变换，保证跨块连续）：
  X = 2q + r,  Y = r          # 邻格的 (X,Y) 距离近似恒定

第一步 阴阳场（低频值噪声）
  n1 = ValueNoise(seed, "yin", X/L1, Y/L1),  L1 = 32
  （格点值 = hash(seed, 格点X, 格点Y)；格点间 smoothstep 插值）
  → 因为 ValueNoise 只依赖绝对坐标，任何 chunk 算边界格都得到同一个值，
    连续性天然成立——这是"全局低频噪声 → 逐块可计算"的关键一步。

第二步 五行填充
  n1 < 0.30 → 阴极：水泽；n1 > 0.70 → 阳极：火地
  中间带用中频噪声 n2（L2=10）分三档：土（中和） / 金（偏阴档） / 木（偏阳档）

第三步 相生梯度
  用 n1 的有限差分梯度方向 g = ∇n1：
  处于两档过渡带（|n1 − 阈值| < ε）的格子，essence 取"沿 g 的相生过渡链"
  （如火→土的烬化带、金→水的凝露带），使地貌沿相生方向渐变而非突跳。

第四步 相克边界（冲突地貌）
  邻域六格内出现克对（水|火、金|木…）→ 该格标记 Danger/conflict
  （水火交界=迷雾泽，金木交界=荒铜林），危险位置由法则自洽产生。

通行性：单独一层 hash 噪声 n3（L3=6，阈值极稀疏）撒顽石，
        出生 chunk (0,0) 及其八邻块豁免——出生区域保证开阔。
```

**围裙（apron）技巧**：第三/四步依赖邻居值。chunk 生成时按 14×14 计算、
只保留内部 12×12，这样块内判定用的是"真邻居"而非猜测，且整个过程仍是
(seed, coord) 纯函数。这是解决跨块边界的标准做法。

### 2.3 chunk 缓存与生命周期

```text
ChunkCache：map[(qc,rc)]*Chunk + LRU（容量 256，命中即晋升）
淘汰：只丢地形，重新进入时按纯函数重算，零成本回放
永存：不落盘（v0.4 不做持久世界）；持久化属未来 persistent world 扩展
```

**Entity 不属于 chunk**：`Entities map[EntityID]*Entity` 是世界级存储，
实体可跨 chunk 移动，chunk 只是地形缓存。EntityAt / SortedEntities /
Materials 全部维持现状——这一条保证 sim.go 的主体逻辑与"无限"正交。

### 2.4 出生点、Fragment、怪物（v0.4 场景包 scenario/infinite-0.4）

```text
world:     无限；TurnLimit 60
出生点:    (0,0) chunk 中心，地形豁免区
Fragment:  由 seed 哈希确定方向（12 向量化）与距离（HexDist 20~28），
           落点格强制改写为可通行（纯函数内完成，不改噪声）
旧地图:    prior_knowledge 给出 Fragment 的粗略方位象限（可不准，§3.5）
怪物:      Tier 2 生成表（world_genesis §5.1）：每 chunk 用
           hash(seed,"mob",qc,rc) 决定是否出怪（概率随"深度"=HexDist(出生,格) 上升），
           essence 取该 chunk 主导行，Form 由 hash 定
           → 怪物在 chunk 首次被生成时懒激活，激活事件进事件流
```

### 2.5 寻路与 Replay

- **寻路**：无限图上 BFS 无界，改 A*（hex 距离启发，admissible）+ 扩展预算
  （如 512 节点）；超预算视为不可达，订单保留（沿用 order_blocked 语义）。
  A* 查询 At() 时按需触发 chunk 生成——地形是纯函数，"探路穿雾"不泄露任何
  未观测信息以外的内容，且与知由感律不冲突（引擎内部知道全部地形）。
- **Replay 不受影响**：chunk 生成不消耗 match Rng 的任何状态（hash 自带 seed），
  实体懒激活是 (seed, coord, tick) 的确定结果并进事件流。重放 = Engine + seed +
  Intent 流（world_genesis §7 两条硬规则原样成立）。

---

## 3. LLM 介入点设计（本设计的灵魂）

### 3.1 四层模型定稿

```text
┌────────────────────────────────────────────────────────────┐
│ 执行层   玩家点选 move_to → 引擎逐 tick 自动步进             │
│          0 次 LLM 调用                                      │
├────────────────────────────────────────────────────────────┤
│ 事件介入层 引擎判定"事件" → 唤醒 LLM 做一次决策              │
│   · 抵达    order_done / order_blocked(连续 3 tick)          │
│   · 遭遇    实体首次进入感知半径（或敌对实体敌意行为）        │
│   · 资源发现 进入含新 essence 物料点的已探索 chunk            │
│   · 状态越阈 battery<20% / heat>80% / integrity<30%          │
│             （边沿触发 + 迟滞，见 3.2）                      │
│   · Guidance 玩家 L2/L3 干预到达                             │
│   · Ask      玩家免费提问（不阻塞推进，异步插入）            │
├────────────────────────────────────────────────────────────┤
│ 心跳层   连续 N=8 tick 无任何事件 → 战略反思 + 认知摘要更新   │
├────────────────────────────────────────────────────────────┤
│ 战斗层   交战状态（双方 Emitter 在作用距离内激活）的每个      │
│          战斗 tick 逐回合调用 —— 战斗是唯一"每步都贵"的层     │
└────────────────────────────────────────────────────────────┘
```

### 3.2 触发条件精确化与事件去抖

引擎在 `Advance` 返回的事件流之上加一层 **EventClassifier**（match 包内，
确定性、可测试）：

```text
去抖规则（同一目的地行走途中不被反复触发）：
1. 遭遇：以 (EntityID) 为 key 首见即触发一次；离感知范围 ≥6 tick 后重新武装
2. 抵达：order_done 每 order 至多一次；order_blocked 连续 3 tick 才升级为事件
3. 状态越阈：边沿触发（跨越阈值那 tick），迟滞 10%（<20% 触发后须回到 >30% 才重新武装）
4. 心跳：lastEventTick 计数器，任何事件归零；无事件满 8 tick 触发一次反思，
   反思本身算一次事件（防连环反思）
5. 战斗：进入交战态置 combat=true，脱离（距离>4 且 2 tick 无互射）退出
```

所有去抖状态是引擎内的确定性状态，进 Replay 事件流，不破坏重放。

### 3.3 调用预算：典型一局 60 回合

| 层 | 触发次数（估） | 调用 |
|---|---|---|
| 执行层（点选行走，~15 次点选 × 平均 6 步） | — | **0** |
| 事件：抵达 | 15 | 15 |
| 事件：遭遇/资源发现 | 6 | 6 |
| 事件：状态越阈 | 4 | 4 |
| 事件：Guidance | 2 | 2 |
| 心跳反思 | 4 | 4 |
| 战斗（2 场 × 5 tick） | 10 | 10 |
| Ask（免费，异步） | 3 | 3 |
| **合计** | | **≈ 44 次**（现状：60 次且与移动耦合） |

关键收益不在总次数，而在**结构**：一次 12 步的 move_to 中间零调用；
LLM 只在"世界发生变化"时被唤醒——这与 player_agent_relationship 的
"Agent 负责判断"完全一致，只是把判断时机从"每 tick"改成了"每个值得判断的时刻"。

### 3.4 player_agent_relationship.md §2 修订草案（v0.2）

> **§2 玩家与 Agent 的职责边界（v0.2 修订）**
>
> 玩家负责：定义长期目标与价值偏好；赛前塑造人格、Memory、Build；局中以
> 五动作（Observe / Ask / Guide / Inform / Reflect）介入；对 Agent 判断提出质疑。
>
> Agent 负责：理解世界、判断局势、制定计划、选择行动、承担结果。
>
> **指令分级（替代 v0.1 "玩家永远不能调用 move() 等四条"）：**
>
> 1. **移动是执行层指令。** 玩家在地图上点选目的地，等价于一条
>    `move_to` 订单：Agent 执行层默认服从，由引擎逐 tick 执行，
>    **不消耗 LLM 调用，也不构成"玩家替 Agent 操作世界"**——正如教练可以说
>    "去那边"，但每一步怎么落脚、路上遇到什么怎么办，仍由 Agent 判断。
> 2. **高价值决策保留给 LLM。** 战斗（是否还击/撤退/乘区豪赌）、组装
>    （Blueprint 设计）、取舍（捡什么、修什么、信谁的情报）、以及一切
>    `activate / assemble / transfer / connect` 类意图，只能由 Agent 在
>    事件介入层 / 战斗层 / 心跳层自主决定。玩家对这些的动作仍是
>    Guide / Inform，不是 Command。
> 3. **执行层服从可以被 Agent 上诉。** 若 move_to 的目的地在 Agent 认知中
>    呈现明确致命风险（如深入已知的火阳过盛区而储气不足），Agent 可在
>    抵达前发出一次"风险陈述"并暂停订单，等待玩家确认或撤单。
>    默认关闭（Autonomy=Cooperative 时开启）。
> 4. 本分级不影响 §23 的长期原则第 1 条的精神：玩家仍然不直接调用
>    `attack()/assemble()/activate()`。变化只在 move：从"禁止"改为
>    "执行层默认服从的地图点选"。

### 3.5 LLM 决策输入的变化：从"整图"到"已探索边界"

- Observation 增加：

```json
{ "explored": { "min_q": -2, "min_r": -3, "max_q": 1, "max_r": 2,
                "chunk_count": 9, "frontier_hints": ["NE 方向未探索"] } }
```

- 认知摘要的措辞从"地图 12×12、Fragment 在 (5,5)"改为
  "已探索区域自出生点向东约 20 格；旧地图标注 Fragment 位于东南象限（未验证）"。
  **无限世界里"离目标多远"必须以已探索信息表述**——这恰好强化
  world_genesis §3.3"信旧图还是信传感器"的第一回合决策。
- 前沿（frontier）= 已探索 chunk 与未探索 chunk 的接壤边，作为 LLM 的
  可选探索方向提示，不作为事实。

---

## 4. 前端：六边形渲染与无限平移

### 4.1 放弃 DOM 网格的理由与 Canvas 方案

MapView.vue 现为 144/576 个 DOM cell。无限世界里格子数不可枚举，
DOM 方案在内存、重排、事件代理三层全部失效。改为单 `<canvas>`：

```text
渲染循环：camera {cx, cy, zoom} → 可视 axial 范围反解 → 六邻域环枚举可见格
         → 每格 1 次 path2D 绘制（地形色按 essence，危险区叠加纹理）
裁剪：   只绘 HexDist（像素意义）≤ 视口对角线的格；可见格数 ≈ 300~500
拾取：   §1.3 逆变换 + cube rounding，点击 → move_to 下单
交互：   拖拽平移（pointer events）、滚轮/双指缩放（zoom 0.5~2.0）、
         相机跟随 Agent（可关闭）、点击实体弹信息卡
```

`state.ts` 保持为唯一数据源，MapView 只做绘制层；实体/事件面板不动。

### 4.2 迷雾

```text
已探索 chunk：按 agent 记忆渲染（记忆=客户端侧已收到的 chunk 快照 + 观测时间）
  —— 注意"Engine 不替 Agent 记地图"（world_genesis §4.1）：前端迷雾是
     Agent Knowledge 的可视化，数据来自 Observation 的 explored/chunk 快照
当前感知半径内：全亮
未探索：深雾（不请求地形；服务端也只下发已探索 chunk，防止上帝视角，
        呼应 relationship §12）
```

### 4.3 触屏 / 移动端移植性

Canvas + 相机模型不含任何 Web 特有依赖，公式（§1.3）与裁剪逻辑可逐行移植：

- Flutter：`CustomPainter` + `GestureDetector`，hex 数学同构；
- pointer events 天然覆盖触屏拖拽/双指；
- 拾取/雾/实体层全部是纯函数，建议 TS 侧独立成 `src/hex/` 模块（无 Vue 依赖），
  即为未来移动端的参考实现。

---

## 5. 实施拆分（给总管的派工单）

```text
WP1  六边形坐标层（串行起点，最高优先）
  文件：engine/hex.go(新)、world.go、sim.go、path.go、observation.go、laws.go(一处)、
        各 *_test.go 改写、scenario/agent/match 跟随编译
  验收：cd backend && go test ./...  全绿（含改写后的行为级断言）
  依赖：无。完成后 WP2/3/5 才可开始；WP4 可与其并行（用 §1.3 公式先做 mock）

WP2  无限世界生成
  文件：engine/chunk.go(新)、engine/noise.go(新)、engine/chunk_test.go(新)、
        scenario/infinite.go(新)、scenario/v04_test.go(新)
  验收：go test ./scenario/ -run V04 ——
        同 seed 同 coord 生成字节一致（跨进程跑两次比对 hash）；
        跨块连续性测试：块边界两侧相邻格 essence 差符合相生梯度；
        出生区连通性（Fragment 可达）测试
  依赖：WP1

WP3  LLM 介入层
  文件：match/events.go(新, EventClassifier)、match/match.go(决策调度改造)、
        agent/runtime.go(新, 触发调度/去抖/心跳)、match/events_test.go(新)
  验收：go test ./match/ ./agent/ ——
        去抖用例（同途不重触发、迟滞、心跳归零）全绿；
        一局模拟（ScriptedAgent 驾驶）调用计数 ≤ 预算表上限
  依赖：WP1；WP2（资源发现事件用到 chunk 主导行）可后接

WP4  前端 Canvas 地图
  文件：src/hex/(新, 纯函数模块)、views/MapView.vue(重写)、state.ts(坐标契约)
  验收：cd frontend/h5 && npm run build && 手测：
        拖拽/缩放/拾取正确；迷雾三态渲染；60fps 拖拽（Chrome devtools perf）
  依赖：WP1 的 JSON 契约定稿（可与 WP1 末期并行）

WP5  A* 寻路与 Replay 校验
  文件：engine/path.go(升级 A*+预算)、engine/replay_test.go(新)
  验收：go test ./engine/ -run Replay ——
        同 seed+Intent 流重放 hash 一致；跨 chunk 长途寻路预算内完成
  依赖：WP1、WP2
```

**必须改写的测试**：`sim_test.go`（坐标常量与 4 邻步数）、`v02_test.go`
（整体随方形场景包废弃，保留可迁移的行为断言）、`match_test.go`（坐标与流程）、
`agent` 相关路径断言。
**必须保持通过的测试**：`laws_test.go` 全部（法则数学）、各行为级断言
（订单推进、过热、形散、溢出、物料守恒）——它们是切换的安全网（§1.2）。

建议顺序：WP1 → (WP2 ∥ WP4) → (WP3 ∥ WP5)。

---

## 6. 风险与未决问题

**风险一：跨块地貌的"相生梯度"质量。** 全局生成时梯度是一整张连续场；
逐块 + 围裙后，梯度由阈值带 + 有限差分近似，可能在地貌带交界出现碎屑状
essence 斑块，破坏"看地貌知资源"的可读性（world_genesis §3.1 的初衷）。
缓解：apron 加宽到 2；为梯度带建立专项视觉回归测试（固定 seed 渲染 5×5 chunk
快照人工评审）；必要时把 L1 从 32 提到 48 降低交界密度。

**风险二：A* 在"玩家点了雾外远方"时的语义。** 目标不可达时是拒绝订单、
还是走到已知边界停下？拒绝太硬（玩家被迫先探路），走到边界又可能与
"Agent 执行层默认服从"的新 §2 条文冲突。缓解：v0.4 先采用"预算内可达则执行，
否则拒单并给出最近可达格建议"（拒单即事件，走事件介入层由 LLM 陈述原因）；
"Agent 上诉暂停"条款默认关闭，降低首版复杂度。

**风险三：前后端坐标契约同日切换的联调窗口。** {x,y}→{q,r} 会让 API 层在
切换期间全面失配，若 WP1 与 WP4 不同步合入，主干长时间不可演示。
缓解：API 先加 `/api/v2`（坐标 hex 语义）与 v1 并存一个迭代期，前端切完即删 v1；
或 WP1 合入前先在独立分支完成 WP4 的 mock 联调。

**未决问题（请总管拍板，另见回复正文）**：
1. move 的关系层条文（§3.4）是否按草案定稿；
2. chunk CS=12 与 LRU=256 的参数；
3. API v1/v2 并存策略；
4. 怪物懒激活进事件流的具体事件格式（WP2 内定，风险低）。

---

## 7. 总管评审（2026-09-18，已批准）

评审结论：**方案通过**。六边形全量切换、绝对坐标 ValueNoise 的逐块化、
apron 围裙、去抖状态进 Replay、测试安全网的诚实分级，均予认可。
四个未决问题拍板如下：

| # | 决策 | 内容 |
|---|---|---|
| 1 | §3.4 关系层条文 | **按草案定稿为 v0.2**，写入 player_agent_relationship.md（原文保留并加 v0.2 指针，不删除）。风险上诉条款默认关闭（Cooperative 档可开，实现延后到 v0.5） |
| 2 | API 迁移 | **一次性硬切 {x,y}→{q,r}，不做 v1/v2 并存**。理由：前端地图页本波整体重写（DOM→Canvas），不存在需维持的旧消费方；并存双契约的维护成本高于一次联调窗口。缓解：WP1 与 WP4 同一波交付，由总管跑集成验收 |
| 3 | 测试安全网口径 | **接受"改写后的测试全绿"**，不启用双坐标过渡层。附加要求：WP1 必须交付《断言语义翻译清单》——每个被改写测试的旧语义→hex 语义映射表，总管验收时抽查 |
| 4 | chunk 参数 | CS=12、LRU=256 照案执行 |

补充要求：

- WP3 的调用预算验收以 §3.3 表格为上限基准（ScriptedAgent 一局 ≤ 50 次）
- WP2 的"视觉回归测试"简化为：固定 seed 的 chunk 布局快照（essence 矩阵 dump）
  进入代码库作 golden file，跨块交界带密度人工抽查一次
- 硬切当波必须包含 world_genesis §7 的重放验证：无限世界下同 seed+Intent 流
  跨 chunk 行走的重放 hash 一致性用例为发布门禁

