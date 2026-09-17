# Player Experience & MVP v0.1

本文档把《World Rule v0.1》和《Player-Agent Relationship v0.1》落到可测试的玩家体验层。  
目标不是继续扩展世界，而是回答四个问题：

1. 玩家到底玩什么？
2. 一局 MVP 怎么走完？
3. 玩家 UI 长什么样？
4. Influence 经济怎么调？

---

# 一、玩家体验说明书 v0.1

## 1. 核心幻想

> **我塑造了一个有自主性的 Agent。它在世界里生存、判断、创造。我的建议会影响它，但不能替它行动。**

玩家不是操作员，不是上帝，也不是纯观众。  
玩家是：

- 创造者：决定 Agent 的初始人格、身体、模型、目标。
- 教练：局中给方向、提醒风险、提供信息。
- 伙伴：接受 Agent 会拒绝你，也会反过来问你。
- 复盘者：赛后调整它的决策方式，而不是刷数值。

---

## 2. 核心情绪目标

| 情绪 | 来源 |
|---|---|
| 影响感 | 我的建议改变了它的计划 |
| 意外感 | 它没听我的，但后来证明它对了 |
| 理解感 | 我能看懂它在想什么、怕什么、怀疑什么 |
| 培养感 | 下一局它因为我上次的调整而变强 |
| 故事感 | 关键决策点形成可复盘的叙事 |
| 创造感 | 它造出了我没预设过但有用的设备 |

## 3. 反情绪

必须避免：

- 被无视的挫败：玩家发建议，Agent 完全不理，且不解释。
- 旁观无聊：玩家每回合没事可做。
- 遥控失落：玩家一发指令，Agent 就执行，Agent 失去意义。
- Prompt 工程：赛后调整变成写小作文。
- 上帝视角：玩家看到 Agent 看不到的信息。

---

## 4. 玩家局中五个动作

| 动作 | 作用 | 消耗 Influence |
|---|---|---|
| Observe | 看 Agent 认知摘要与感知地图 | 0 |
| Ask | 问 Agent 为什么这么判断 | 0 |
| Guide | 给 Goal / Preference / Concern | 0.5–2 |
| Inform | 提供玩家自己的推断或信息 | 1 |
| Predict | 猜 Agent 下一步关键行动 | 0，猜对奖励 |

玩家永远没有：

```text
Control
```

不能直接调用：

```text
move()
attack()
assemble()
activate()
```

如果玩家输入“去 C7”，系统必须转换为：

```text
Goal:
前往 C7 附近。
```

然后由 Agent 决定是否去、怎么去。

---

## 5. 一局节奏

建议 MVP 一局：

```text
赛前配置：5–10 分钟
局中回合：20–30 回合
赛后复盘：5 分钟
总时长：30–45 分钟
```

每回合玩家操作时间控制在 30–60 秒。  
Agent 思考可以异步，UI 显示“正在判断”。

---

## 6. 成长系统

玩家成长的不是攻击力，而是：

- 人格滑块：冒险 / 谨慎、合作 / 竞争、探索 / 利用。
- 经验卡：从复盘中选择教训。
- 组件库：解锁 Frame、Battery、Motor、Sensor、Processor。
- Blueprint 库：保存 Agent 设计的设备。
- 模型选择：不同模型作为风格、成本、难度。
- Creator Profile：Agent 对玩家偏好的长期总结。

---

## 7. MVP 成功标准

MVP 只验证两件事：

1. 玩家是否产生这种感觉：

> “这个决定不是我做的，但我的建议影响了它。”

2. 玩家是否偶尔产生这种感觉：

> “它没听我的，但现在看它可能是对的。”

如果这两种感觉成立，关系设定就成立。

---

# 二、MVP 一局完整流程

## 1. MVP 范围

```text
1 个玩家
1 个 Agent
1 张 12×12 网格地图
30 回合上限
组件：Frame、Battery、Motor、Sensor、Processor
属性：Mass、Energy、Heat、Integrity
动作：inspect、move、connect、activate、assemble
目标：进入危险区域，找到 Fragment，返回起点
威胁：环境损耗 + 1 个简单未知 Agent
```

---

## 2. 赛前配置

玩家操作：

```text
选择模型：Balanced
人格滑块：
  冒险 0.6
  合作 0.7
  探索 0.5

初始 Goal：
  进入北区遗迹，找到 Fragment，带回起点。

初始 Preference：
  生存优先。
  避免无意义战斗。

初始组件：
  Light Frame
  Small Battery
  Basic Motor
  Optical Sensor
  Processor L1

Influence：
  5
```

Agent 初始身体：

```text
Entity: agent_body
Components:
  frame_light
  battery_small
  motor_basic
  sensor_optical
  processor_l1
```

---

## 3. 回合循环

每回合：

```text
1. World Engine 生成 Observation
2. 玩家观察 Agent 认知摘要
3. 玩家可 Observe / Ask / Guide / Inform / Predict
4. Agent 接收 Observation + Guidance + Memory
5. Agent 返回 Intent[]
6. Engine Validate → Resolve → Physics Update
7. Engine 生成新 Observation
8. 记录关键决策点
```

---

## 4. 示例回合：Turn 17

状态：

```text
Integrity: 32%
Energy: 41%
Fragment 距离：2 格
检测到：Unknown Agent
```

Agent 当前认知：

```text
Goal:
获取 Fragment 并返回。

Plan:
继续向北进入遗迹。

Risk:
高。Integrity 低，Energy 偏低。

Suspicion:
Unknown Agent 可能尾随。
```

玩家操作：

```text
Guide:
我更重视生存。
在不知道那个 Agent 意图的情况下，
我觉得继续推进风险偏高。
```

消耗 Influence：1。

Agent 重新评估：

```text
收到。

我仍然认为 Fragment 很重要，
但当前风险超过我的可接受范围。

新计划：
先观察 Unknown Agent。
如果没有明显威胁，再继续推进。
```

Agent 返回 Intent：

```json
{
  "intents": [
    {
      "type": "activate",
      "component": "sensor_optical",
      "parameters": { "direction": "east" }
    },
    {
      "type": "move",
      "target": [5, 8]
    }
  ]
}
```

Engine 结算：

```text
Sensor 激活：Energy -5，Heat +2
发现 Unknown Agent 是低威胁采集者
Integrity 不变
```

Agent 继续行动。

---

## 5. 结束条件

任一满足：

```text
获得 Fragment 并返回起点：胜利
30 回合结束：按目标完成度评分
Agent Integrity <= 0：失败
```

---

## 6. 赛后复盘

系统自动列出关键决策点：

```text
Turn 17
玩家建议：撤退或先观察。
Agent 决策：先观察，再继续。
结果：发现 Unknown Agent 无威胁，获得 Fragment。
Integrity 从 72 降至 28。
```

玩家调整：

```text
Risk Preference: 0.7 → 0.4
新增经验卡：
  在信息不足时，不要因为短期资源收益承担致命风险。
```

保存：

```text
Creator Profile 更新
Blueprint 库更新
下一局可用新组件或新经验
```

---

# 三、玩家 UI / 认知摘要原型

## 1. 布局

```text
┌──────────────────────────────────────────────────────────┐
│ Turn 17/30 | Energy 41% | Integrity 32% | Influence 3.0 │
├───────────────────────────┬──────────────────────────────┤
│                           │ Agent 认知摘要                │
│   世界观察地图            │                              │
│   （仅 Agent 感知范围）   │ Goal: 获取 Fragment 并返回    │
│                           │ Plan: 先观察 Unknown Agent    │
│   [迷雾] [未知结构]       │ Known: Fragment 距离 2        │
│   [Agent] [信号]          │ Suspicions: Unknown 可能尾随  │
│                           │ Risk: 高，Integrity 32%       │
│                           │ Confidence: 0.72              │
├───────────────────────────┴──────────────────────────────┤
│ [Observe] [Ask] [Guide] [Inform] [Predict]               │
│ 输入：我更重视生存，不要仅凭怀疑主动攻击。                │
│ 预计消耗 Influence：1.0        [发送]                     │
└──────────────────────────────────────────────────────────┘
```

---

## 2. 认知摘要字段

必须展示：

```text
Current Goal
Current Plan
Known Information
Suspicions
Risk Assessment
Concerns
Confidence
Memory Highlights
```

每个字段可点击。

点击 Suspicions：

```text
玩家：
为什么你认为 Unknown Agent 可能尾随？

Agent：
它连续两回合出现在我们的移动方向附近，
并且没有继续向它之前声明的目标方向移动。
```

点击 Plan：

```text
玩家：
为什么先观察，而不是直接撤退？

Agent：
当前 Energy 仍可支持 6 回合。
撤退会放弃接近的 Fragment。
观察成本更低，且能降低不确定性。
```

---

## 3. Guidance 输入模板

玩家不自由写 Action，而是选择类型：

```text
Goal：
  我希望你优先保证生存。

Preference：
  不要仅凭怀疑主动攻击。

Concern：
  我们的 Energy 已经很低。

Information：
  我认为 B 上一回合可能在拖延时间。
```

发送前显示：

```text
类型：Preference
消耗：1.0 Influence
Agent 可以拒绝，但必须解释。
```

---

## 4. 禁止显示

```text
完整 World State
其他 Agent 的真实位置
Agent 的隐藏推理链
未来事件
```

只显示：

```text
Agent Knowledge
Agent 的显式认知摘要
基于游戏事实的解释
```

---

# 四、Influence 经济数值表 v0.1

## 1. 基础规则

```text
初始 Influence：5
上限：5
每 10 回合恢复：+1
预测正确：+0.5
Agent 主动请求意见时，玩家回答：0
```

---

## 2. 消耗表

| 玩家动作 | Influence 消耗 | 说明 |
|---|---:|---|
| Observe | 0 | 看认知摘要与感知地图 |
| Ask | 0 | 问为什么，但不改变计划 |
| Inform | 1.0 | 提供玩家推断或信息 |
| Whisper | 0.5 | 轻度建议，Agent 可忽略 |
| Guide | 1.0 | 明确方向，Agent 会重新评估 |
| Deep Link | 2.0 | 要求重新评估整个战略 |
| 紧急暂停 | 3.0 | 每局最多 1 次，强制 Agent 暂停一回合 |
| 预测 | 0 | 猜对奖励 +0.5 |

---

## 3. 被忽略是否退还

建议：

```text
Whisper：不退还。
Guide：不退还。
Deep Link：不退还。
紧急暂停：不退还。
```

但 Agent 必须解释：

```text
我理解你的担忧。
但我判断当前收益仍然高于撤退。
原因：Fragment 距离 2，Energy 可支持 6 回合。
```

解释本身是玩家体验的一部分。

---

## 4. 自主性调节

| 自主性 | 效果 |
|---|---|
| High Autonomy | Guide +0.5，Deep Link +1，询问少 |
| Balanced | 标准 |
| Cooperative | Guide -0.5，询问多，Influence 上限 +1 |

MVP 默认：

```text
Balanced
```

---

## 5. 调参目标

一局 30 回合内：

```text
玩家有效干预：3–6 次
Agent 忽略率：20%–40%
紧急暂停使用：0–1 次
预测参与：2–5 次
```

如果玩家干预少于 3 次，参与感不足。  
如果多于 8 次，Agent 会变成遥控角色。

---

# 五、设计红线

1. 玩家不能直接调用世界 Action。
2. Agent 保留最终行动决策权。
3. 玩家和 Agent 默认共享信息边界。
4. Agent 必须能解释接受或拒绝建议的理由。
5. 赛后调整必须游戏化，不能变成写 Prompt。
6. Agent 可以主动问玩家，但玩家回答的是偏好，不是动作。
7. 所有 Agent 输出必须经过 Engine 裁决。
8. 世界有唯一真实状态，Agent 只能通过感知了解世界。

---

# 六、结论

这四份 v0.1 合在一起，已经可以支撑一个 MVP：

- 《World Rule》定义世界底层。
- 《Player-Agent Relationship》定义玩家与 Agent 的关系。
- 《Player Experience & MVP》定义玩家实际体验。
- 《Influence 经济》定义干预成本与节奏。