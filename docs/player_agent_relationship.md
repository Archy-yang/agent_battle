# Player-Agent Relationship v0.1

> v0.2 修订（2026-09-18）：§2 指令分级见文末附录 A；其余不变

## 1. 核心定义

玩家不是 Agent 的直接操作者。

Agent 也不是玩家的棋子。

两者关系定义为：

> **玩家负责塑造、影响和提供价值方向；Agent 负责理解、判断和执行。**

核心原则：

> **Agent 拥有自主性，玩家拥有影响力。**

进一步约束：

> **玩家可以改变 Agent 为什么行动，但不能直接决定 Agent 如何行动。**

---

# 2. 玩家与 Agent 的职责边界

## 玩家负责

* 定义长期目标
* 定义价值偏好
* 设计 Agent 初始人格
* 选择模型
* 配置 Memory
* 配置身体 / Component
* 配置能力边界
* 在局中提供建议
* 提供自己掌握的信息
* 对 Agent 的判断提出质疑
* 在局后复盘并调整 Agent

---

## Agent 负责

* 理解当前世界
* 判断局势
* 制定计划
* 选择行动
* 使用 Tool
* 创建设备
* 与其他 Agent 交流
* 判断玩家建议是否适用
* 承担行为产生的结果

玩家永远不能直接调用：

```text
move()
attack()
assemble()
activate()
```

这些只能由 Agent 调用。

> v0.2 修订：本条的 move 部分已由附录 A 的指令分级取代，见文末。

---

# 3. 玩家输入分为四类

玩家不能发送 Action，只能发送高层语义。

## 3.1 Goal — 目标

表示玩家希望 Agent 达成什么。

例如：

```text
找到安全区域。
```

```text
尽可能获得更多信息。
```

```text
尝试拿到中央区域的 Fragment。
```

Goal 描述：

> 想要什么结果。

不描述：

> 如何完成。

---

# 3.2 Preference — 偏好

表示玩家的价值取向。

例如：

```text
尽量避免没有必要的战斗。
```

```text
相比资源，我更重视长期生存。
```

```text
尽量保持与其他 Agent 的友好关系。
```

Preference 不要求 Agent 必须遵守。

它属于：

> 决策权重。

---

# 3.3 Concern — 提醒 / 风险关注

例如：

```text
我们的 Energy 已经很低了。
```

```text
我觉得进入这个区域风险比较高。
```

```text
不要忘记刚才那个 Agent 有远程武器。
```

Concern 的作用不是下命令。

而是让 Agent：

> 重新评估。

---

# 3.4 Information — 玩家提供的信息

例如：

```text
我注意到 B 上一回合可能在拖延时间。
```

或者：

```text
我认为地图西边的结构可能有入口。
```

注意：

玩家信息不一定是真的。

Agent 应该区分：

```text
World Observation
Player Information
Agent Inference
```

不能把玩家的话直接当成世界事实。

---

# 4. 禁止玩家直接下达 Action

例如下面这些属于非法直接控制：

```text
移动到 C7。
```

```text
攻击 Agent B。
```

```text
使用 Sensor。
```

```text
制造一个机器人。
```

系统应该把这类指令转换或者降级为高层 Intent。

例如：

玩家：

```text
攻击 B。
```

转换为：

```text
Goal:
尝试削弱 B。
```

Agent 最终可能：

* 攻击
* 威胁
* 埋伏
* 抢资源
* 不执行

这由 Agent 判断。

---

# 5. Guidance System

局中玩家可以通过：

```text
Guidance
```

影响 Agent。

Guidance 包含：

```text
type
content
priority
timestamp
```

例如：

```json
{
  "type": "concern",
  "content": "目前 Integrity 很低，我倾向先保证生存。",
  "priority": "normal"
}
```

Agent 在下一次决策时接收到。

---

# 6. Guidance 不等于 Command

Agent 得到 Guidance 后必须自行判断。

例如：

玩家：

```text
我觉得我们应该撤退。
```

Agent 可能回答：

```text
我理解你的担忧。

但敌方当前也严重受损，而我们距离目标只有一步。
我认为继续行动的收益仍然高于撤退。
```

然后继续行动。

这个能力非常重要。

否则 Agent 只是一个语言包装过的遥控角色。

---

# 7. Influence Resource

为了避免玩家不停给 Agent 发指令，可以设置：

```text
Influence
```

作为有限资源。

例如一局：

```text
Influence: 5
```

普通观察：

```text
不消耗 Influence
```

向 Agent 提供重要 Guidance：

```text
-1 Influence
```

要求 Agent 重新评估当前计划：

```text
-1 Influence
```

要求 Agent 与玩家展开深度讨论：

```text
-2 Influence
```

这样玩家会面临一个真实决策：

> 现在值不值得干预？

---

# 8. Guidance 等级

可以设置三种玩家影响方式。

## Level 1 — Whisper

轻度建议。

例如：

```text
注意一下能源。
```

特点：

* 成本低
* Agent 可以忽略
* 不强制重新规划

---

## Level 2 — Guidance

明确表达方向。

例如：

```text
接下来我更希望以生存为优先。
```

Agent 会重新评估当前计划。

---

## Level 3 — Deep Link

重大干预。

例如：

```text
重新评估整个当前战略。
我认为我们对 B 的判断可能是错误的。
```

此时 Agent：

```text
暂停当前计划
↓
重新读取关键状态
↓
重新规划
```

但依然由 Agent 决定行动。

---

# 9. Agent 主动向玩家请求意见

关系不能永远是：

```text
玩家 → Agent
```

Agent 也应该可以：

```text
Agent → 玩家
```

例如 Agent 遇到：

* 多个收益相近选择
* 重大价值冲突
* 信息严重不足
* 高风险行为
* 长期战略选择

可以询问：

```text
目前有两个方案：

A：
进入遗迹，可能获得 Fragment，但风险很高。

B：
先寻找修复资源，成功率更高，但可能错失机会。

你更偏向哪一种？
```

玩家回答的是：

```text
偏好
```

而不是：

```text
动作
```

---

# 10. Agent Autonomy

不同玩家可以配置不同自主程度。

例如：

```text
Autonomy = 80
```

代表 Agent 倾向独立判断。

可以划分：

## High Autonomy

Agent：

* 很少询问
* 更少依赖玩家
* 更容易坚持自己的判断

---

## Balanced

重要决策时会询问。

---

## Cooperative

更频繁寻求玩家意见。

但无论哪种：

> 玩家都不能直接替 Agent 操作世界。

---

# 11. Player-Agent Dialogue

玩家可以和 Agent 对话。

但建议把对话分成两种。

## Casual Dialogue

不改变 Agent 当前决策状态。

例如：

```text
你怎么看刚才那个人？
```

Agent：

```text
我认为他目前表现得比较合作，
但他的行为和他说的话存在一定矛盾。
```

---

## Strategic Dialogue

可能改变：

```text
Goal
Preference
Plan
```

因此需要消耗 Influence 或时间。

例如：

```text
我觉得我们之前的策略不对，重新考虑一下。
```

---

# 12. 玩家不能看到 Agent 看不到的信息

这是公平性的重要原则。

默认情况下：

> 玩家与 Agent 共享感知边界。

Agent 没看到的东西，玩家也不应该通过 UI 看到。

否则玩家会变成：

```text
上帝视角雷达
```

然后不断告诉 Agent：

> B 就在你后面。

整个 Information System 就失效了。

所以玩家 UI 应基于：

```text
Agent Knowledge
```

而不是：

```text
World State
```

---

# 13. 但玩家和 Agent 的“理解”可以不同

这是很有意思的一点。

例如 Engine 告诉双方：

```text
Turn 21:
Unknown Agent seen at D7.
```

Agent 判断：

```text
可能是敌人。
```

玩家可能判断：

```text
我觉得他没有敌意。
```

于是玩家可以说：

```text
我觉得你对它的敌意判断可能过于激进。
```

这就是：

> 人类判断 + AI 判断

真正结合的地方。

---

# 14. Agent 应展示自己的“当前认知”

否则玩家根本没法参与。

UI 至少需要展示：

```text
Current Goal

Current Plan

Known Information

Suspicions

Risk Assessment

Current Concerns
```

例如：

```text
Current Goal:
获取第二个 Fragment

Current Plan:
绕过中央区域，从北边进入遗迹

Concern:
Energy 只剩 31%

Suspicion:
Agent B 可能正在尾随我们
```

注意这里展示的是：

> 可供玩家交互的显式认知摘要。

不是模型隐藏的内部推理过程。

---

# 15. 玩家干预应该针对这些认知对象

例如玩家可以点击：

```text
Suspicion:
B 可能正在尾随我们
```

然后：

```text
“为什么这么判断？”
```

Agent可以给出基于游戏事实的简短解释：

```text
B 连续两次出现在我们的移动方向附近，
并且没有继续向它之前声明的目标方向移动。
```

玩家：

```text
“先不要因此攻击它。”
```

于是形成：

```text
Preference:
不要仅凭当前怀疑主动攻击 B。
```

这种交互会比直接聊天更有游戏感。

---

# 16. Long-Term Relationship

玩家与 Agent 的关系不应只存在于单局。

可以存在：

```text
Creator Profile
```

Agent 可以逐渐形成对玩家偏好的总结。

例如：

```text
Creator Preferences:

- 高度重视生存
- 不喜欢无意义战斗
- 对陌生 Agent 比较谨慎
- 喜欢探索未知机制
```

下一局 Agent 可以参考。

但这些仍然只是：

```text
Preference
```

而不是硬规则。

---

# 17. 玩家是在“培养决策方式”

局后玩家真正升级的不是：

```text
+5 attack
```

而可以是调整：

```text
人格
价值权重
Memory
知识
经验
身体 Build
Blueprint Library
```

例如发现 Agent 总是过度冒险：

玩家可以调整：

```text
Risk Preference:
0.7 → 0.4
```

或者增加一条经验：

```text
在信息不足时，不要因为短期资源收益承担致命风险。
```

下一局观察它行为是否变化。

---

# 18. Match Review

局后是玩家参与感非常重要的一环。

系统应该自动选出：

```text
关键决策点
```

例如：

```text
Turn 17
你建议撤退。

Agent 决定继续进入遗迹。

结果：
获得 Fragment，但 Integrity 从 72 降至 28。
```

以及：

```text
Turn 31
Agent 信任了 Agent C。

结果：
完成资源交换。
```

这样玩家可以调整：

```text
Prompt
Preference
Memory
Build
```

形成：

```text
观察
↓
理解
↓
调整
↓
再次实验
```

---

# 19. 玩家参与的完整循环

```text
               玩家
                │
         赛前塑造 Agent
                │
                ▼
              Agent
                │
             入场
                │
                ▼
        自主 Observe / Act
                │
           ┌────┴────┐
           │         │
        玩家观察   Agent判断
           │         │
           ▼         │
     Guidance / Advice
           │         │
           └────┬────┘
                ▼
           Agent重新判断
                │
                ▼
             行动
                │
                ▼
              世界
                │
                ▼
             比赛结束
                │
                ▼
              复盘
                │
                ▼
            调整 Agent
```

---

# 20. 玩家与 Agent 的五种互动

最终可以稳定为五个动作：

```text
Observe
```

观察 Agent 当前认知与世界。

```text
Ask
```

向 Agent 询问判断。

```text
Guide
```

提供 Goal / Preference / Concern。

```text
Inform
```

提供玩家自己的判断或信息。

```text
Reflect
```

赛后调整 Agent。

玩家永远没有：

```text
Control
```

---

# 21. 一个完整例子

当前状态：

```text
Integrity: 32%
Energy: 41%

距离 Fragment：2 格

检测到：
Unknown Agent

Agent 当前计划：
继续获取 Fragment
```

Agent：

```text
我准备继续前进。
虽然 Integrity 较低，但 Fragment 距离已经很近。
```

玩家：

```text
Guide:

我更重视生存。
在不知道那个 Agent 意图的情况下，
我觉得继续推进风险偏高。
```

Agent重新评估：

```text
收到。

我仍然认为 Fragment 很重要，
但当前风险已经超过我的可接受范围。

新计划：
先观察 Unknown Agent，
如果没有明显威胁再继续推进。
```

然后 Agent 自己调用：

```text
activate(sensor)
```

而不是玩家调用。

这就是我们希望形成的关系。

---

# 22. 最核心的平衡

玩家参与太少：

```text
配置 Agent
↓
看录像
```

会失去参与感。

玩家参与太多：

```text
玩家下命令
↓
Agent执行
```

Agent 又失去意义。

因此目标状态是：

```text
玩家
决定价值和方向

Agent
决定策略和行动
```

玩家应该经常产生这种感觉：

> “这个决定不是我做的，
> 但我的建议影响了它。”

同时也应该偶尔出现：

> “它没有听我的，
> 但现在看来它的判断可能是对的。”

当这种感觉成立的时候，玩家和 Agent 才真正成为两个独立但有联系的游戏角色。

---

# 23. 长期不变原则

未来无论 LLM 能力如何增强，都建议保持：

1. 玩家不直接调用世界 Action。
2. 玩家只能改变 Agent 的目标、偏好、信息和关注点。
3. Agent 保留最终行动决策权。
4. Agent 可以主动向玩家寻求意见。
5. 玩家和 Agent 默认共享同一信息边界。
6. 玩家培养的是 Agent 的决策方式，而不是单纯控制行为。

最终关系可以浓缩为一句话：

> **玩家不是 Agent 的操作者，而是它的创造者、教练和长期伙伴。**

---

# 附录 A：§2 职责边界的 v0.2 修订（指令分级）

依据：docs/specs/2026-09-18-v04-infinite-hex-design.md §3.4 草案，总管评审（2026-09-18）已批准定稿为 v0.2。

## A.1 玩家负责

定义长期目标与价值偏好；赛前塑造人格、Memory、Build；局中以五动作
（Observe / Ask / Guide / Inform / Reflect）介入；对 Agent 判断提出质疑。

## A.2 Agent 负责

理解世界、判断局势、制定计划、选择行动、承担结果。

## A.3 指令分级（替代 v0.1 "玩家永远不能调用 move() 等四条"）

1. **移动是执行层指令。** 玩家在地图上点选目的地，等价于一条
   `move_to` 订单：Agent 执行层默认服从，由引擎逐 tick 执行，
   **不消耗 LLM 调用，也不构成"玩家替 Agent 操作世界"**——正如教练可以说
   "去那边"，但每一步怎么落脚、路上遇到什么怎么办，仍由 Agent 判断。
2. **高价值决策保留给 LLM。** 战斗（是否还击/撤退/乘区豪赌）、组装
   （Blueprint 设计）、取舍（捡什么、修什么、信谁的情报）、以及一切
   `activate / assemble / transfer / connect` 类意图，只能由 Agent 在
   事件介入层 / 战斗层 / 心跳层自主决定。玩家对这些的动作仍是
   Guide / Inform，不是 Command。
3. **执行层服从可以被 Agent 上诉。** 若 move_to 的目的地在 Agent 认知中
   呈现明确致命风险（如深入已知的火阳过盛区而储气不足），Agent 可在
   抵达前发出一次"风险陈述"并暂停订单，等待玩家确认或撤单。
   默认关闭（Autonomy=Cooperative 时开启）。
4. 本分级不影响 §23 长期原则第 1 条的精神：玩家仍然不直接调用
   `attack()/assemble()/activate()`。变化只在 move：从"禁止"改为
   "执行层默认服从的地图点选"。
