# agent_battle 开发约定

## 子 Agent 开发模式（持续使用）

本项目的开发任务优先分派给子 Agent（Agent tool，model 不传覆盖值，即使用 glm-5.3-flash[1m]）：

- 每个子 Agent 一个聚焦任务，提示词包含：目标、涉及文件、相关设计文档章节（docs/*.md）、验收标准（测试/构建通过）
- 相互独立的任务可并行派发；有共享文件依赖的任务串行
- 子 Agent 返回后由主会话审查验收：`go test ./...`、`npm run build`，UI 改动用 chrome-devtools 截图确认
- git commit 只由主会话执行，保持提交粒度与信息质量
- 设计决策不交给子 Agent——按 docs/ 文档执行；发现文档冲突或缺口时回报主会话，不得自行发挥

## 工程约定

- 后端 `backend/`（Go）：模拟运算只用定点整数，不用浮点；Engine 不知道具体模型，只认识 AgentProtocol
- 前端 `frontend/h5`（Vue3 + Vite + TS）：构建产物直出 `backend/web/dist`，由 worldd 内嵌
- 敏感配置 `backend/llm.env`、`backend/profile.json` 永不提交
- **LLM 成本纪律**：开发期默认禁止用真实 LLM 验证——子 Agent 的 LLM 相关测试一律 httptest 假端点或 ScriptedPilot；真实验证只在里程碑节点做，执行前先向用户报预算（目的、预计调用次数/token 量级、结果如何改变后续计划）。例外：若不真实验证将阻碍推进（存在假端点无法检验、且证伪将迫使已建层返工的关键假设），允许发起，但同样先报预算与计划；一次实验多采数据，不为同一问题反复付费
- 世界设计以 `docs/` 文档为准。**现役方向（2026-09-18 起）：修真养成**——`stage_2.md`（架构/闭环）、`stage_3.md`（数值公式）、`cultivation_core_rules_constitution_v0.1.md`（设计宪法：三层架构/新原语准入清单/玩法名称禁入底层）、`cultivation_realm_and_sensory_model_v0.1.md`（境界相变判据与感知模型：PerceptToken/趋势/滞回/痛感分型）、`specs/2026-09-18-m1-cultivation-plan.md`（M1 路线图与标度约定）、`design/yiqi-dongfu-redesign.html`（洞府改版设计稿）。战斗篇文档（world_rule、world_laws、world_genesis、player_agent_relationship、strage_1）保留作后续复用，不再是当前开发依据。改设计先改文档再改代码
- **感知隔离律**：玩家 Agent（LLM）的 prompt 与观察中永不得出现公式/参数/数值，只有定性感知词与知识库内容；感知解析器是唯一出口。玩家侧同理：拓扑、境界五量等底层事实只经"摸索→回执→沉淀"显形，UI 不得预-render 未被实验揭示的结构
- **底层规则准入**（依设计宪法 §7/§20）：新增 Primitive/Operator 前过准入清单；底层代码禁止玩法名称判断（pill/formation/sword/realm 名）；LLM 只能提交 Intent/Blueprint/Protocol，永不直接创造世界事实
- 测试即法则：每条世界法则都应有对应单测；同 seed 重放必须逐字节一致
