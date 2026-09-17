# agent_battle backend

Go 实现的 World Engine 与服务端。设计文档见仓库根目录 `docs/`。

## 结构

```text
cmd/worldd/   服务入口（当前为脚手架占位）
engine/       确定性世界引擎：实体图、五行法则、Intent 裁决、Observation
scenario/     场景包加载与校验（第一个包：scenario/mvp-0.1）
agent/        Agent Runtime 与 LLM 适配器（M1 起填充）
```

## 原则

- 模拟运算只用定点整数，不用浮点（Replay 逐字节可复现，world_genesis §7）
- Engine 不知道任何具体模型，只认识 AgentProtocol（world_rule §11）
- 一切结果由法则计算，不由 Agent 描述（world_rule §32）

## 里程碑

```text
M0  确定性引擎（headless + 脚本 Agent + Replay 测试）
M1  LLM 接入（CLI 对局，验证 world_rule §31 的核心问题）
M2  玩家介入层（Guidance / Influence / 认知摘要）
M3  手机客户端（Flutter，服务端不变）
```
