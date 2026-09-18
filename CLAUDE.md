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
- 世界设计以 `docs/` 文档为准。**现役方向（2026-09-18 起）：修真养成**——`stage_2.md`（架构/闭环）、`stage_3.md`（数值公式）、`specs/2026-09-18-m1-cultivation-plan.md`（M1 路线图与标度约定）、`design/yiqi-zhoutian-ui-draft.html`（五屏设计稿）。战斗篇文档（world_rule、world_laws、world_genesis、player_agent_relationship、strage_1）保留作后续复用，不再是当前开发依据。改设计先改文档再改代码
- **感知隔离律**：玩家 Agent（LLM）的 prompt 与观察中永不得出现公式/参数/数值，只有定性感知词与知识库内容；感知解析器是唯一出口
- 测试即法则：每条世界法则都应有对应单测；同 seed 重放必须逐字节一致
