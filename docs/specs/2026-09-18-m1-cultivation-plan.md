# M1 路线图 —— 修真养成核心闭环（stage_2/3 落地）

2026-09-18 总管制定。设计依据：`docs/stage_2.md`（架构/闭环）、`docs/stage_3.md`（数值公式）、
`docs/design/yiqi-zhoutian-ui-draft.html`（五屏设计稿，已获用户确认）。

## 本期定位（用户钦定）

**修炼实验室，不是修真游戏。** 本期唯一命题：玩家 + Agent 能否共同发现第一门功法，
并让它脱离 LLM 自动运行（stage_2 §40-41 六条验证）。没有地图/战斗/炼丹/宗门不影响成败；
一切 UI 都是实验仪器的面板；验证 6（"这是我们一起研究出来的"）与验证 1-5 同权。

## 0. 铁律

1. **感知隔离律（用户钦定，最高优先）**：玩家 Agent（LLM）的系统提示与观察载荷中，
   永不得出现——公式、参数值、精确数值、规则表、真值区间。只允许：
   定性感知词（胀/温热/阻滞…）、粗档位（trace/small/medium/large）、
   知识库内容（属 Agent Knowledge，非 World Truth）。
   感知解析器（cult/perception.go）是唯一出口；任何绕过它直读真值的代码路径都是 bug。
2. **确定性**：定点整数、无浮点进模拟；控制误差用 deterministic noise（stage_3 §8.2）；
   同 seed + Intent 流重演逐位一致；录像/重演沿用 match 层既有模式。
3. **Runtime 消费已知，LLM 探索未知**（stage_2 §33/§37）：三级运行时 L0/L1/L2；
   已验证 Protocol 由 Runtime 执行零 LLM；出已知边界即停、升级给 Agent。
4. stage_2/stage_3 为现役设计文档；world_laws/克曲线留作战斗篇（stage_2 §43），不删不改。
5. 引擎不知道 LLM，LLM 不改世界——沿用 world_rule §32 精神。
6. **意图回执律（SPIKE 结论，2026-09-19 增补）**：任何意图落地与否，会话层必须给
   Agent 一条中性回执事件（无法执行/已被拒/已生效及其粗档后果），属接口语义而非
   世界真值——静默丢弃是被 SPIKE 三轮证实的最大失败源（分身据此建立错误理论）。

## 0.1 SPIKE 结论对 WP-4 的设计输入（2026-09-19）

SPIKE 三轮：M1 首吸✅、M2 自主回调✅、M3 稳定循环❌（-selftest 证明世界可达 M3）。
失败模式与对策：
- **静默丢弃** → 回执律（铁律 6）
- **稀疏反馈下迷信累积** → 知识系统的证据结构必须区分三态：试过且成 / 试过被拒 /
  从未试过；假说校验以证据账本为准，修行笔记（cognition 回喂）不得作为唯一凭据
- **纳引相位割裂** → 知识系统沉淀"节律"类知识（吸收-引流并行的周期编排），
  Protocol 的 cadence 语义要能表达同息并行而非仅交替步骤

## 0.2 LLM 成本纪律（2026-09-19）

已升格为常驻工程约定，见 CLAUDE.md「LLM 成本纪律」：开发期默认禁用真 LLM
（假端点/脚本桩替代），真实验证只在里程碑节点且先报预算；阻断性例外同见该条。
本期内：SPIKE-2 复跑并入 M1 末期验收的一次真跑。

## 1. 数值标度约定（cult/params.go 顶部注释同步此节）

- **量**（qi / capacity / flow / amount）：×1000 整数，1.0 = 1000，典型域 0~30000。
- **比率**（0~1：openness、stability、activity、damage、retention…）：×1000。
- **亲和**（0.5~1.5）：×1000，即 500~1500。
- 乘法：`fpMul(a,b) = a*b/1000`，int64 中间量，长乘法链逐对折叠（≤6 因子安全）；
  **除法四舍五入到千分位**（WP-1 实测定夺：截断使黄金用例偏 2，四舍五入逐位精确）。
- 开方：`fpSqrt(x) = isqrt(x*1000)`（千分位精度，整数运算）。
- 慢速成长量（Adaptation/Openness/ChannelAdaptation 增量 ~10⁻⁴/Tick）以 ×10⁶ 微标度
  存储，参与公式时降采样为千分位（确定性截断）。
- 控制误差 ε：`noise(seed, tick, acupoint)` ∈ [-1000,1000]，线性映射到
  ±ControlErrorMax——不用全局 RNG，避免与重演互相干扰。
- 所有 stage_3 参数集中于 `cult/params.go`，每个常量注明 stage_3 章节号与默认值
  （§52：参数是实验值，公式关系不可变）。

## 2. 包结构

```
backend/cult/          修炼引擎（L0，不知道 LLM 存在）
  params.go            参数表 + 标度约定
  body.go              Character / Acupoint / Channel / Environment（stage_3 §3-6）
  tick.go              吸收/传输/损耗/相生/相克/活性/应变/稳固/损伤/适应（§7-24）
  perception.go        感知解析器：真值→定性词（§42-44）——感知隔离律的执行点
  sequence.go          Sequence/周天/Loop/筑基五条件判定（§25-38）
  cult_test.go         黄金用例 + Test 1-7 阶梯
backend/ctmatch/       对局层（L1/L2，知道两种运行时）
  match.go             tick 调度、三级运行时、行为生命周期（stage_2 §33-34）
  protocol.go          Protocol 编译/校验/运行/异常升级（stage_2 §35-36）
  knowledge.go         知识系统：Observation/Hypothesis/Evidence/Confidence（stage_2 §28）
  llm.go               分身 LLM：prompt 只含现象学与志向，绝不含规则（铁律 1）
  api.go               /api/cult/* HTTP 层
backend/cmd/worldd     挂载 /api/cult/*；旧对局路由不动（战斗篇资产）
frontend/h5            新增修炼五视图（按设计稿），旧视图保留不删
```

## 3. 工作包

| WP | 内容 | 验收 |
|----|------|------|
| WP-1 | params/body/tick 引擎核 | stage_3 §45-50 手算逐位复现为黄金用例；Test 1-3 绿 |
| WP-2 | perception.go 感知解析 | 词表完整、神识分级生效、单测覆盖每档边界 |
| WP-3 | 意图协议 + Protocol Runtime + 异常升级 | Test 4（周天）绿；protocol 编译校验拒绝非法 |
| SPIKE | 真 LLM 玩前半程 | 发现→调参→稳定可复现（stage_2 §41 验证 1/2 前半） |
| WP-4 | 知识系统 + Loop/筑基 + 持久化 | Test 5-7 绿；重演一致 |
| WP-5 | API + H5 五屏 | 设计稿逐屏对齐；手机视口可用 |

WP-1→WP-2→WP-3→SPIKE→WP-4→WP-5 串行为主；WP-2 与 WP-3 的接口在 WP-1 验收后可并行。
M1 总验收 = stage_2 §41 六条验证 + §54 三类路线 Monte Carlo 健康检查。

## 3.1 感知观察契约（WP-2 产出、WP-3 消费，双方不得偏离）

```go
// cult/perception.go —— 感知隔离律的执行点：输出只有词与档位，永无真值
type AcupointSense struct {
    Name       string   // 穴位名
    Feelings   []string // stage_2 §25 sensation 词（温热/胀/痛/阻滞…）
    Flow       string   // stage_2 §25 flow 词（进入/停滞/加速…）
    Nature     []string // stage_2 §25 nature 词（生发/炽烈…——推断性措辞）
    Openness   string   // 状态词表五档：闭/微开/半开/大开/洞开
    Stability  string   // 状态词表五档：稳如磐石/稳固/微澜/不稳/将溃
    Damage     string   // 状态词表五档：无损/隐伤/作痛/重创/脉裂
    PressureBand int    // §43 压力档 0-5（供 UI 色带；措辞按 acuity 生成）
}
type EnvSense struct{ Qi []string; Activity []string } // 逐五行定性词（微茫→沛然）
type Observation struct {
    Tick   int64
    Env    EnvSense
    Points []AcupointSense
}
func Observe(w *World, acuity int) *Observation // acuity=神识 1..3，措辞粒度随级解锁（§43）
```

## 4. 体验口径（对齐设计稿）

- 玩家五页：洞府（主厅/感知流）/ 内视（身体图）/ 参悟（假说·实验·请示）/ 功法（Protocol 库）/ 本尊（塑造·筑基五关）。
- 界面一律定性词不用数（设计稿状态词表）；数值只存在于引擎与调试视图。
- 分身自述 = LLM 认知摘要；请示卡 = 风险决策上浮；「未耗一问」= 自运行荣誉。
