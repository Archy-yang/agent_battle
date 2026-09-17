# 《断言语义翻译清单》—— WP1 六边形坐标层全量切换

依据 v0.4 设计 §1.2 / §7 总管评审决定 #3：每个被改写、被保留（作为安全网）或被删除的
测试断言登记一行：**测试名 | 旧语义 | hex 语义 | 等价性理由**。
抽查对象：总管验收（WP1）。

图例：〔改写〕断言坐标/距离语义已翻译；〔保留〕原样通过，属安全网；
〔删除〕仅因方形网格而存在；〔新增〕hex 层自身测试。

## engine/laws_test.go（法则数学与坐标无关，全部保留）

| 测试 | 旧语义 | hex 语义 | 等价性理由 |
|---|---|---|---|
| TestGenerates 〔保留〕 | 五行相生矩阵 | 同 | 纯数值，无几何参与 |
| TestRestrains 〔保留〕 | 五行相克矩阵 | 同 | 纯数值，无几何参与 |
| TestResolveRestraintZones 〔保留〕 | 克曲线 侮/微/衡/乘 分区数值 | 同 | 纯数值，无几何参与 |
| TestApplyPowerNeutral 〔保留〕 | 非克关系 ×0.5 | 同 | 纯数值，无几何参与 |
| TestCurves 〔保留〕 | 耗气/生热/显迹 ∝ x、x² | 同 | 纯数值，无几何参与 |

## engine/sim_test.go

| 测试 | 旧语义 | hex 语义 | 等价性理由 |
|---|---|---|---|
| TestMoveCostsAndCurves 〔改写〕 | move 至正交邻格 (2,1)，耗气 4 | move 至 Hex{2,1}（HexDist((1,1),(2,1))=1 的 E 邻格） | 两断言均验证"移动一格 + 阴阳曲线"；单步耗气与方向无关 |
| TestMoveRejectedByOccupancy 〔改写〕 | 占位格 (2,1) 拒绝移动 | 同格 Hex{2,1} | 占位判定按 EntityAt，与网格几何无关 |
| TestDangerZoneDrains 〔改写〕 | 方形危险区 x,y∈4..5，站 (4,4) | 危险区经线性映射 (X,Y)=(2q+r,r) 定义的 {4,5}×{4,5}，站 Hex{0,4} | 设计 §2.2 的线性变换保证方形地带谓词逐字可用；断言只查耗气 2/生热 3，与区域形状无关 |
| TestOverheatDamages 〔保留〕 | 过热扣形 2 | 同 | 坐标无关 |
| TestEmitterAttackHeng 〔改写〕 | 目标 (3,1)：Chebyshev 2 ≤ 射程 3；衡区 600×1.5÷10 | 目标 Hex{3,1}：HexDist=2 ≤ 3；伤害断言不变 | HexDist((1,1),(3,1)) = (2+0+2)/2 = 2，与旧 Chebyshev 同值，射程语义等价；克曲线数值与几何无关 |
| TestEmitterAttackWu 〔改写〕 | 目标 (3,1)，反噬 5 | 目标 Hex{3,1}（距离同上） | 同上 |
| TestTransferPickup 〔保留〕 | 同格拾取 | 同 | 判定 `t.Pos != e.Pos`，坐标无关 |
| TestAssembleConsumesMaterials 〔保留〕 | 组装消耗物料、电池馈电 | 同 | freeAdjacent 由 4 方向改 6 方向，但断言不断言放置方位 |
| TestAssembleRejectsBadBlueprint 〔保留〕 | 无储气蓝图拒绝 | 同 | 坐标无关 |
| TestObservationRadius 〔改写+删除〕 | 被动可见 9（3×3）；感识可见 36（8×8 边界截断成 6×6） | 被动 7（HexDist≤1：3r²+3r+1, r=1）；感识 61（r=4） | 〔删除〕"方形边界截断 36 格"子断言——无限世界无边界，环遍历 (design §1.4) 下格数公式 3r²+3r+1 精确成立；半径语义（1/4）不变 |
| TestMoveToRoutesAroundObstacle 〔改写〕 | 方形墙列 x=3（缝 (3,5)），BFS 最短路 12 步、耗气 100−48 | hex 墙线 q=3（r∈−20..20，缝 (3,5)），最短路 10 步、耗气 100−40 | 任何路径必须过缝：min = HexDist((1,1),(3,5)) + HexDist((3,5),(5,1)) = 6+4 = 10（三角不等式取等，缝是唯一通路）；"绕障仍到达 + 走最短路"的行为断言原样保留 |
| TestMoveToWaitsForDynamicBlocker 〔改写〕 | 阻挡者 (2,1)、目标 (4,1)，order_blocked→继续→order_done | 同坐标 Hex（HexDist 3，语义同） | 动态占位等待逻辑与几何无关；坐标仅作标识 |
| TestMoveToClearsOrderOnArrival 〔改写〕 | 目标 (2,1)（正交邻格） | Hex{2,1}（hex 邻格） | 到达清单行为与邻接定义方式无关，只要目标可达且相邻一步即至 |
| TestExplicitMoveOverridesOrder 〔改写〕 | 显式 move 至 (1,2)（正交邻格） | Hex{1,2}（SE 邻格，HexDist=1） | "显式 move 本 tick 优先、订单保留"语义不变；(1,2) 在 hex 下仍是合法一步 |
| TestMoveToDoesNotConsumeIntentBudget 〔改写〕 | move_to + 2 动作同 tick 生效 | 同（目标 Hex{4,1}，显式移 Hex{1,2}） | 预算语义坐标无关；两动作目标均为合法邻格 |
| TestMoveToRejectsUnreachable 〔改写〕 | 目标格不可通行；目标被整列墙隔死（方形边界保证 BFS 终止） | 目标 Hex{2,1} 不可通行；目标 Hex{5,1} 被 q=3 **无限**墙线隔死，BFS 以 PathBudget=4096 兜底判不可达 | hex 下无限直线确实分割平面（不可绕行），"隔死"语义保留；无限图 BFS 无界，预算耗尽=不可达是 WP5 A*（§2.5）落地前的确定性兜底，拒单事件（intent_rejected）语义不变 |

## engine/hex_test.go（新增）

| 测试 | 断言 | 理由 |
|---|---|---|
| TestHexDist 〔新增〕 | 公式抽检：(0,0)→(3,−3)=3、(0,0)→(8,−4)=8（v0.4 Fragment）、(−3,21)→(0,12)=9（v02） | design §1.1 公式直接验证 |
| TestHexDistAxioms 〔新增〕 | 非负、对称、三角不等式、Adjacent ≡ HexDist==1 | 替代旧 Chebyshev 的距离公理基线 |
| TestHexNeighbors 〔新增〕 | 固定顺序 E/NE/NW/W/SW/SE，每个邻居距离 1，与 HexDirs 一致 | 固定方向序是重放确定性的锚 |
| TestHexStepToward 〔新增〕 | 首步必缩距 1；候选按距离升序、平局按 HexDirs | agent 贪心 / 怪物追击 / Unknown 逃离共用此序 |
| TestHexPixelRoundTrip 〔新增〕 | hex→px→hex 无损（多组坐标，含 v04/v02 关键格）；公式抽检；往返像素误差 <1e-6 | design §1.3，前端 TS 侧交叉一致性测试的 Go 基准 |
| TestPixelToHexNearCenters 〔新增〕 | 中心附近偏移拾取回原 hex | 拾取（点击→move_to）语义 |
| TestHexJSON 〔新增〕 | 序列化 `{"q":8,"r":-4}`，反序列化对称 | 评审决定 #2：{x,y}→{q,r} 硬切的契约锚点 |
| TestChunkCacheAt 〔新增〕 | 任意 (q,r)（含负）返回可通行土性格；指针复用；自定义 TerrainFunc 生效 | WP1 无限平原占位实现的行为契约，WP2 换 LRU 内部后仍须成立 |

## scenario/v02_test.go

| 测试 | 旧语义 | hex 语义 | 等价性理由 |
|---|---|---|---|
| TestV02Layout → TestV02LayoutHex 〔改写+删除〕 | 24×24 尺寸、出生 (1,22)、方格地带抽检 | 〔删除〕W/H 尺寸断言（无限世界无界）；出生 (−3,21)、spawn→Fragment HexDist 9、地带抽检经 (X,Y)=(2q+r,r)（金 (5,2)、水 (−2,6)、火 (0,10)）、怪物中带 (X,Y)∈[0,28]×[6,18] | 线性映射保持地带谓词逐字等价；中带范围由怪物锚点 (q∈−3..5, r∈6..18) 精确导出；"Fragment 在危险区深处、三怪五行齐备"的行为断言保留 |
| TestV02SpawnToFragmentReachable 〔改写〕 | 方形 4 邻 BFS 多 seed 连通 | hex 6 邻 BFS（early-exit + PathBudget 上限，无限图兜底） | 连通性语义与邻接定义同步切换；上限只在被完全隔断时触发（此时连通本应判假） |
| TestV02Obstacles 〔改写〕 | 遍历 24×24 数不可通行格；关键格不被吞 | hex 窗口（q∈−8..10, r∈−1..24，覆盖全部锚点蔓延范围）计数；关键格保护从"点"扩大为 HexDist≤3 邻域 | 无限世界无法全图遍历；计数窗覆盖障碍可能出现的全部格，上下界断言（4..24）语义不变；保护区扩大使怪物追击走廊确定性无障 |
| TestV02DeterministicGeneration 〔改写〕 | 24×24 全图逐格比对 | 中带 hex 窗口（q∈−6..8, r∈−2..24）逐格比对 | 无限世界无全图；窗口覆盖怪物活动与测试用到的全部格，"同 seed 逐格一致"语义不变 |
| TestV04Layout 〔新增〕 | 出生 (0,0)、Fragment (8,−4)（HexDist 8）、Unknown+3 怪物均在半径 8 出生环、可通行、三行齐备、TurnLimit 60、v0.2 物料配比 | design §2.4 最小包的布局契约 | 新增 |
| TestV04SpawnToFragmentReachable 〔新增〕 | 无限平原多 seed 连通 | 新增 | |
| TestV02GenesisMonsters 〔保留〕 | 怪物组件/形/气装配断言 | 同 | 坐标无关 |

## scenario/scenario.go（生产代码，随测试改写的布局决策）

| 项 | 旧语义 | hex 语义 | 理由 |
|---|---|---|---|
| Pack.W/H/Terrain(Vec2) 〔删除〕 | 方形网格与方格地形函数 | TerrainFn(seed,q,r) 纯函数 + Hex 坐标 | design §1.1/§2.2 |
| mvp 地形 〔改写〕 | 12×12 方形分区 | 谓词原样，输入经 (X,Y)=(2q+r,r)；出生 (0,10)、Fragment (0,5)（(X,Y)=(5,5) 危险区，HexDist 5）、Unknown (4,3) | 改动最小的直译路径（见任务判定）；支持 match 层胜利流程回归 |
| v02 障碍群/重试 〔改写〕 | 方格锚点 4..19、方形边界内蔓延、24×24 BFS 连通验证 | hex 锚点 (q∈−3..5, r∈4..19)、六方向蔓延 ≤3 步、关键格 HexDist≤3 豁免、early-exit+预算 BFS 连通验证、≤10 重试 | 重试链仍是 seed 的纯函数；连通语义等价 |
| GenesisV02 〔合并〕 | 双 Genesis 入口 | 单 Genesis()（含 monsters，空则无） | 消除重复装配代码，行为不变 |
| V04 〔新增〕 | 无限平原最小包：出生 (0,0)、Fragment (8,−4)、Unknown (−8,0)、怪物 (0,8)/(0,−8)/(−8,8) 全在 HexDist 8 环 | design §2.4；WP2 接入真实地形后结构不变 | 新增 |

## match/match_test.go

| 测试 | 旧语义 | hex 语义 | 等价性理由 |
|---|---|---|---|
| TestScriptedAgentWinsMVP 〔保留+翻译〕 | 脚本 Agent 方形图中取得 Fragment 返回出生点获胜 | 同行为，hex 贪心（HexStepToward 逐步缩距）+ hex 地形 | 行为级安全网：走→拾→返的完整回路；hex 贪心在开阔地形每步缩距 1，与方形贪心收敛性等价 |
| TestDeterminism 〔保留〕 | 同 seed 两局事件流逐字节一致 | 同 | world_genesis §7；六方向固定序 + 确定性 Rng 消耗保持该性质 |
| TestLossOnDeath 〔保留〕 | 形散即负 | 同 | 坐标无关 |
| TestGuidanceEconomy 〔改写〕 | 伪造 move 意图 Vec2{1,9} | engine.Hex{1,9} | 该意图只走 classifyIntents 的 type 分支，坐标值无语义 |
| TestV02MonsterChaseAndAttack 〔改写〕 | 追击圈 Chebyshev≤4、初始距 3 ((5,5)→(5,8))、"远离"格 (0,23)、感识 4 格可见 | HexDist≤4、初始 HexDist 3 ((0,0)→(3,0))、远离格 Hex{0,23}、感识 HexDist≤4 可见；走廊取 r=0（障碍锚点蔓延范围 r≥1 之外，追击路线确定性无障） | 行为链逐步保留：逐回合缩距→相邻攻击→in_combat→enemies 视图三态；距离度量切换后 3/4 的追击圈边界数值不变 |
| TestRegistryScenarioSelect 〔改写+删除〕 | v02 为 24×24（m.W.W/H 断言） | 〔删除〕尺寸断言（W/H 字段随方形网格删除）；〔新增〕v04 包校验（名/限/怪/出生点）；其余选择与落库断言保留 | 无限世界无尺寸可断言 |
| TestV02Determinism30Turns 〔保留〕 | 30 回合事件流逐字节一致 | 同 | 同 TestDeterminism |
| TestInCombatViaEvents 〔改写〕 | 敌我拉开 Chebyshev 23 | HexDist 23 ((0,23)→(12,12)) | in_combat 的距离侧（HexDist≤2）与事件侧断言均坐标无关地成立 |

## match/match.go（生产代码，随测试改写的行为决策）

| 项 | 旧语义 | hex 语义 | 理由 |
|---|---|---|---|
| unknownAct 〔改写〕 | 外环绕行（onRing 依赖 W/H 边界）、逃离取象限步 | 六方向固定序游荡（rotate 沿 HexDirs）、逃离按六邻格"远离玩家距离"降序 | 无限世界无外环；确定性（同 seed 同走位）保留 |
| monsterAct 追击 〔改写〕 | 象限三候选 (dx,dy)/(dx,0)/(0,dy) | HexStepToward 六候选按 HexDist 升序 | 每步缩距 1，追击收敛速度与旧版一致；平局序固定保证可重放 |
| monsterAct 游荡 〔改写〕 | 4 方向 Rng 序 | 6 方向 Rng 序（Intn(6)） | 覆盖全部合法移动方向；Rng 消耗仍确定性 |
| InCombat / View.Enemies 〔改写〕 | Chebyshev≤2 / Chebyshev≤radius | HexDist≤2 / HexDist≤radius | design §1.4 距离语义替换 |

## 其他跟随项（无测试断言变化）

| 项 | 说明 |
|---|---|
| engine/world.go | Vec2/W/H/Cells 删除；World.Chunks（ChunkCache 占位：无限平原，接口按 §2 留给 WP2 换 LRU）；At 一切格可查询；InBounds 恒真（resolveMove 越界校验语义随之取消，Passable/占位/储气检查保留） |
| engine/path.go | bfsDirs → HexDirs 六方向；PathBudget=4096 兜底（WP5 A* 替换） |
| engine/observation.go | 可见格方形 dx/dy 循环 → HexDist≤radius 环遍历（轴向枚举序确定）；Chebyshev → HexDist |
| agent/agent.go | ScriptedAgent.nextStep → HexStepToward 六方向贪心；W/H 字段删除 |
| agent/llm.go | 系统提示词坐标示例 {x,y}→{q,r}，补六方向与距离公式说明 |
| match/api.go | StateView.Order/Explored/EnemyView.Pos → Hex；JSON 自动变 {q,r}（评审决定 #2 硬切，无 v1/v2 并存）；sortVecs→sortHexes（R,Q 序） |
| cmd/worldd/main.go | version v0.2.0→v0.4.0；scenario 注释补 v04 |
