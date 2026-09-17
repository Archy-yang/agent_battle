# agent_battle backend

Go 实现的 World Engine 与服务端。设计文档见仓库根目录 `docs/`。

## 结构

```text
cmd/worldd/   服务入口：/api 对局接口 + / 内嵌 H5 静态路由
engine/       确定性世界引擎：五行法则、侮微衡乘曲线、Intent 裁决、Observation
scenario/     场景包（已实现：scenario/mvp-0.1）
agent/        Agent 决策接口（脚本 pilot 已实现；LLM 适配器 M1+ 填充）
match/        对局会话：回合循环、迷雾、Guidance、HTTP API 视图
web/          go:embed 打包 frontend/h5 构建产物
```

## 原则

- 模拟运算只用定点整数，不用浮点（Replay 逐字节可复现，world_genesis §7）
- Engine 不知道任何具体模型，只认识 AgentProtocol（world_rule §11）
- 一切结果由法则计算，不由 Agent 描述（world_rule §32）

## 里程碑

```text
M0  确定性引擎 + 场景包 + 脚本 Agent 整局（✅ 已完成，含决定性测试）
M1  LLM 接入（Agent Runtime 接 LLM 适配器，验证 world_rule §31 核心问题）
M2  玩家介入层进阶（Guidance 进入决策、Influence 经济、认知摘要 LLM 化）
M3  客户端（H5 最小版已内嵌 ✅；Flutter / 小程序为后续形态）
```

## 运行

```bash
cd backend && go run ./cmd/worldd     # 默认 :8080，可用 PORT 覆盖
cd frontend/h5 && npm run build       # 前端改动后重建（产物直出 backend/web/dist）
go test ./...                         # 法则单测 + 整局集成 + 决定性重放
```
