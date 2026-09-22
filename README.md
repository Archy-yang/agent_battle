# 一炁周天 · agent_battle

LLM 原生修真养成游戏：**你（本尊）定方向，你的 LLM 分身在规则未知的身体里感知、试错、悟出功法，并让它自动运转。**

> 这不是一款"修真皮肤"的游戏。境界不是经验值，而是引擎中真实出现的稳定结构；
> 功法不是技能书，而是分身用实验换来的、可自动执行的协议；
> 分身永远看不到世界公式——它只能靠定性感知做研究，就像真实的科学。

## 核心理念

1. **规则少而稳定，组合多而开放**——引擎只定义能量怎样流动、结构怎样稳定（[设计宪法](docs/cultivation_core_rules_constitution_v0.1.md)）。聚灵阵、功法、丹药这类名字是玩家与文明给涌现结构起的命名，不是底层类型。
2. **感知隔离律**——玩家与 LLM 的观察中永无公式与数字，只有"微热、胀、顺畅、滞涩"这类定性词；感知解析器是唯一出口。玩家侧同样成立：穴位拓扑要靠摸索显形，境界五量只以档位词呈现。
3. **确定性引擎**——定点整数运算、seeded 随机，同 seed 重放逐字节一致；LLM 提交意图与协议，世界事实只由引擎计算。

## 三层运行时

```text
L0 World Engine    定点整数模拟：穴位/经脉/五行/逆流做功/炼化/守恒归账
L1 Protocol Runtime 消费已知：已固化的功法自动运转，异常才升级
L2 LLM Agent       探索未知：观察 → 假设 → 实验 → 归纳 → 固化协议
```

## 现在能玩到什么（M2 进行中）

- **洞府**：单页叙事流——身体图上穴位以"模糊波动"待你发现，感知流如对话般流淌，心念卡是你给分身的心意
- **主环周天**：丹田 → 土府 → 中庭 → 阳枢 → 神庭 → 青关 → 归丹田。逆经脉而行唯有丹田能推动，费力且生热——火候
- **气炼化**：气在循环中被身体筛选，"更少更精纯"是涌现现象而非属性
- **筑基**：不是打卡一百次，而是闭合、平衡、自持、安稳、内化五项真实跨过相变条件

## 运行

```bash
# 前端（构建产物由 worldd 内嵌，改前端后须重编 worldd）
cd frontend/h5 && npm install && npm run build

# 服务端
cd backend && go build -o worldd ./cmd/worldd
PORT=:8090 ./worldd
# 浏览器打开 http://localhost:8090 → 修炼（不配置 LLM 时可用脚本分身演示）
```

LLM 分身（可选）：`backend/llm.env` 配置 `AGENTBATTLE_LLM_BASE_URL / MODEL / API_KEY / THINKING`。

## 文档地图

```text
docs/cultivation_core_rules_constitution_v0.1.md   设计宪法：三层架构、新原语准入、玩法名称禁入底层
docs/cultivation_realm_and_sensory_model_v0.1.md   境界相变判据 + 感知模型（PerceptToken/趋势/滞回）
docs/stage_2.md                                    架构与闭环（玩家=本尊，LLM=分身，Engine=因果）
docs/stage_3.md                                    数值公式与标度约定
docs/specs/2026-09-21-stage3-revision-circulation-refinement.md  主环拓扑/逆流/炼化修订包（已标定）
docs/specs/2026-09-22-m2-plan.md                   M2 路线图
docs/design/yiqi-dongfu-redesign.html              洞府交互设计稿（可直接打开玩）
backend/cmd/cultmc/                                Monte Carlo 标定台（六命题）
```

## 状态

`RuleVersion 0.3.0-mainloop` · M2 进行中（洞府前端已重构；功法编译端到端、探索僵局对策、角色差异性验证、金丹设计待做）。

战斗篇（回合制对战、场景包、Replay）保留于 `backend/engine|match|scenario` 与 docs 战斗文档，作为后续"修真世界的斗法"复用底座。
