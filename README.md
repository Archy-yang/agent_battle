# agent_battle

LLM 深度参与的角色扮演游戏：玩家塑造一个自主 Agent，与其一起在世界法则内探索、回合制战斗与创造工具。

> 玩家负责塑造、影响和提供价值方向；Agent 负责理解、判断和执行。
> LLM 决定意图，World Engine 决定现实。

## 文档（v0.1）

```text
docs/world_rule.md                  世界架构（语法）：Entity / Component / Intent / Observation
docs/world_laws.md                  世界法则（语义）：气论五行、生克矩阵、侮微衡乘曲线、禁区
docs/world_genesis.md               世界生成与主持：场景包、迷雾探索、敌人谱系、Replay
docs/player_agent_relationship.md   玩家与 Agent 的关系层：自主性与影响力
docs/strage_1.md                    玩家体验与 MVP：流程、五动作、Influence 经济
```

## 代码

```text
backend/    Go 服务端：World Engine + Agent Runtime + 场景管理
frontend/   Flutter 客户端（M3 启动，目录未建）
```

详见 backend/README.md。
