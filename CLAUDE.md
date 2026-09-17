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
- 世界设计以 `docs/` 五份文档为准：world_rule（架构）、world_laws（法则）、world_genesis（生成与主持）、player_agent_relationship（关系）、strage_1（体验/MVP）；改设计先改文档再改代码
- 测试即法则：每条世界法则都应有对应单测；同 seed 重放必须逐字节一致
