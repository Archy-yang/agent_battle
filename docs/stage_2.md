# LLM 原生修真养成游戏

## 世界规则与 Agent 运行体系 v0.1

---

# 1. 项目目标

本项目不是传统意义上的：

> 修真游戏 + LLM NPC

而是尝试构建一个：

> **由确定性世界规则驱动，由 LLM 负责探索、推理、创造，由玩家参与方向判断与知识沉淀的 AI-native 修真世界。**

游戏真正的核心玩法不是：

* 点击升级；
* 刷经验；
* 获得固定功法；
* 按配方炼丹。

而是：

```text
观察世界
↓
提出猜想
↓
进行实验
↓
获得反馈
↓
归纳规律
↓
形成知识
↓
固化为功法 / 丹方 / 阵法 / 技艺
↓
自动运行
↓
探索新的未知
```

玩家和 LLM 共同完成这个过程。

---

# 2. 核心设计哲学

整个系统遵循以下几个长期不变原则。

## 2.1 LLM 决定 Intent，Engine 决定 Reality

LLM 可以决定：

* 想研究什么；
* 想怎么尝试；
* 选择什么路线；
* 提出什么假设；
* 如何调整方案；
* 如何解释观察结果；
* 如何命名一个新功法。

LLM 不可以决定：

* 是否成功；
* 吸收了多少灵气；
* 是否受伤；
* 某条路线是否稳定；
* 某种材料实际有什么性质；
* 是否筑基；
* 是否炼丹成功。

所有事实均由 Game Engine 根据固定规则计算。

---

## 2.2 Game Engine 必须脱离 LLM 独立运行

即使完全移除 LLM，世界仍然可以：

```text
时间推进
灵气流动
五行相生相克
穴位承载变化
经脉流动
材料转化
压力变化
结构失稳
损伤
恢复
丹炉反应
稳定态形成
稳定态崩溃
```

LLM 只是：

> 世界中的观察者、研究者和决策者。

不是世界规则的一部分。

---

## 2.3 Runtime 消费已知，LLM 探索未知

LLM 不应该参与所有 Tick。

一个已经掌握、验证并固化的行为：

```text
吸收灵气
运行固定路线
执行已有功法
重复成熟丹方
```

应该完全由确定性的 Runtime 执行。

LLM 只在以下情况参与：

```text
未知
异常
创造
优化
重大决策
```

这是控制 LLM 成本与保证玩法合理性的核心原则。

---

# 3. 世界哲学框架

世界规则借鉴：

> 道生一，一生二，二生三，三生万物。

但不把道教概念简单做成属性皮肤。

建议映射为：

```text
道
↓
世界不可违反的基本规律

一
↓
存在 / Essence

二
↓
差异、方向、对立

三
↓
Interaction / Transformation

五行
↓
基础变化范式

组合
↓
万物
```

所谓：

```text
功法
丹药
阵法
法器
修士
魔法
科技
```

都只是底层规则的不同上层显化。

---

# 4. 五行定义

五行不要定义为五种攻击属性。

建议定义为五种**变化倾向**。

| 五行 | 核心倾向           |
| -- | -------------- |
| 木  | 生长、生成、积累、恢复、扩张 |
| 火  | 转化、放大、释放、活跃    |
| 土  | 承载、绑定、稳定、缓冲    |
| 金  | 结构、约束、切分、收敛    |
| 水  | 流动、传递、扩散、适应    |

五行之间存在全局统一：

```text
相生：
木 → 火 → 土 → 金 → 水 → 木

相克：
木 → 土
土 → 水
水 → 火
火 → 金
金 → 木
```

但“生克”不能简单表达成：

```text
+20%
-30%
```

而应进入统一的 Transformation / Suppression 公式。

---

# 5. 世界基础概念

v0.1 暂定八个核心概念。

## 5.1 Essence

世界中参与变化的基础量。

修真世界上层称：

> 灵气。

内部至少具有：

```text
amount
density
activity
nature[5]
```

---

## 5.2 Nature

五行倾向向量：

```text
[木, 火, 土, 金, 水]
```

示例：

```text
[0.6, 0.2, 0.1, 0.05, 0.05]
```

表示以木性为主，但并非纯木。

任何：

```text
灵气
角色
穴位
材料
丹药
环境
法器
```

均可拥有 Nature。

---

## 5.3 Carrier

灵气或状态的承载体。

例如：

```text
肉身
穴位
丹田
药材
丹炉
法器
```

主要属性：

```text
capacity
strength
stability
compatibility
```

---

## 5.4 Channel

负责把 Essence 从 A 传递到 B。

在角色身体里对应：

> 经脉。

主要属性：

```text
conductance
throughput
damage
```

---

## 5.5 Structure

描述一个系统是否仍能保持当前结构。

主要属性：

```text
integrity
stability
```

`integrity` 更偏是否损坏。

`stability` 更偏当前动态状态是否稳定。

---

## 5.6 Environment

环境影响所有变化。

暂定：

```text
ambient_qi[5]
temperature
pressure
```

后续可扩展。

---

## 5.7 Threshold

很多现象不是线性变化，而需要阈值。

例如：

```text
压力低：
正常

压力中：
不适

压力高：
结构损伤
```

---

## 5.8 Time

Engine 按 Tick 推进。

即使 UI 表现为：

```text
一息
一个周天
一小时
一天
```

底层仍为：

```text
Tick Simulation
```

---

# 6. 基础规则算子

v0.1 建议控制在约 12 个 Operator。

复杂性来自组合，而不是单个公式非常复杂。

## Generate / 生

```text
Generate(X, rate)
```

生成某种状态或 Essence。

---

## Consume / 耗

```text
Consume(X, rate)
```

消耗资源。

---

## Transfer / 行

```text
Transfer(A → B)
```

通过 Channel 传递 Essence。

---

## Transform / 化

```text
Transform(A → B)
```

一种状态转化成另一种状态。

例如：

```text
木 → 火
```

---

## Amplify / 盛

```text
Amplify(X)
```

强化已有过程。

不能凭空产生。

---

## Suppress / 克

```text
Suppress(A, B)
```

压制 B 的 Activity 或变化速度。

---

## Accumulate / 聚

```text
Accumulate(X)
```

逐渐积累。

---

## Diffuse / 散

```text
Diffuse(X)
```

自然逸散。

---

## Bind / 结

```text
Bind(A, B, ...)
```

将多个状态形成较稳定结构。

炼丹、炼器、结丹都大量依赖该算子。

---

## Release / 放

```text
Release(X)
```

释放已积累或者绑定的力量。

---

## Stabilize / 定

```text
Stabilize(System)
```

提高系统进入稳定状态的能力。

---

## Destabilize / 乱

```text
Destabilize(System)
```

使系统远离稳定状态。

---

# 7. 角色身体模型

角色身体不是固定职业模板。

身体本质为：

```text
BodyGraph
```

其中：

```text
Node = 穴位 / 丹田
Edge = 经脉
```

---

# 8. 角色先天亲和

角色拥有：

```text
CharacterAffinity[5]
```

例如：

```text
木 1.25
火 0.80
土 0.90
金 0.75
水 1.10
```

`1.0` 为普通水平。

亲和进入真实公式，而不是：

```text
木系技能 +25%
```

亲和主要影响：

```text
吸收
控制
适应
转化
稳定
```

---

# 9. 穴位模型

每个穴位不编写特殊技能。

所有穴位统一：

```text
Acupoint {
    affinity[5]

    capacity
    throughput
    retention
    base_stability

    openness
    stability
    damage

    adaptation[5]

    qi[5]
}
```

## affinity

五行亲和。

## capacity

当前节点可承载多少 Essence。

## throughput

单位时间安全通过量。

## retention

灵气是否容易停留。

## openness

开放程度：

```text
0 ~ 1
```

## adaptation

后天通过长期修炼形成的适应度。

---

# 10. 经脉模型

经脉结构：

```text
Channel {
    base_conductance
    throughput
    damage
}
```

经脉对某个元素的有效传导能力由：

```text
经脉基础属性
×
两个穴位开放度
×
两个穴位对应五行亲和
×
损伤状态
```

共同决定。

可采用：

```text
C(i,j,e)
=
BaseConductance(i,j)
× O(i)
× O(j)
× sqrt(A(i,e) × A(j,e))
× (1 - Damage)
```

避免给每一条经脉配置五套独立参数。

---

# 11. 灵气吸收公式

角色从环境吸收属性 e：

```text
Intake[e]
=
Environment[e]
× CharacterAffinity[e]
× AcupointAffinity[e]
× Openness
× TechniqueIntensity
× BackPressure
```

其中：

```text
BackPressure
=
1 /
(
    1 + (CurrentLoad / Capacity)^2
)
```

因此：

> 越接近容量极限，继续吸收越困难。

---

# 12. 经脉传输

Agent 请求流量：

```text
F_request
```

真实可通过：

```text
F_actual
=
min(
    F_request,
    SourceAvailable,
    ChannelCapacity
)
```

如果请求：

```text
2.5
```

但实际经脉只能承载：

```text
1.8
```

那么：

```text
F_actual = 1.8
```

多余部分可能造成：

```text
残留
压力
热
逸散
```

---

# 13. 相生公式

对于：

```text
木 → 火
```

可以先采用：

```text
TransformAmount
=
SourceAmount
× Activity
× BaseShengRate
× LocalTransformAffinity
```

局部亲和可以由当前穴位：

```text
sqrt(
    Affinity[source]
    × Affinity[target]
)
```

计算。

转化存在效率：

```text
TargetGain
=
TransformAmount × Efficiency
```

剩余部分可以变成：

```text
热
损耗
逸散
```

---

# 14. 相克公式

相克优先作用于：

```text
Activity
```

而不是直接删除 Amount。

例如：

```text
Suppression
=
BaseKeRate
× CounterRatio
× CounterActivity
× LocalAffinity
```

然后：

```text
TargetActivity'
=
TargetActivity
× (1 - Suppression)
```

这样：

> 水克火

更像让火失去活跃性。

而不是水火碰撞以后简单抵消。

---

# 15. 压力公式

穴位当前：

```text
Load = Σ Qi[e]
```

压力比：

```text
PressureRatio = Load / Capacity
```

压力：

```text
Pressure = PressureRatio²
```

这样越接近极限：

> 风险增长越来越快。

---

# 16. 稳定性

v0.1 可以先只考虑：

```text
超载
属性不匹配
流量突变
```

例如：

```text
Overload =
max(0, Load / Capacity - 1)
```

属性兼容：

```text
Compatibility
=
Σ (
    QiRatio[e]
    × AcupointAffinity[e]
)
```

流量冲击：

```text
Shock
=
abs(CurrentFlow - PreviousFlow)
/
Throughput
```

总 Strain：

```text
Strain
=
0.6 × Overload²
+
0.2 × max(0, 1 - Compatibility)²
+
0.2 × Shock²
```

稳定度：

```text
Stability(t+1)
=
clamp(
    Stability(t)
    + Recovery
    - Strain,
    0,
    1
)
```

参数权重只是 v0.1 实验值，需要仿真调整。

---

# 17. 功法的正式定义

功法不是游戏数据库中的固定技能。

功法本质是：

> **已经经过验证的一套身体运行 Protocol。**

其结构：

```text
Technique {
    path
    amount
    flow
    dwell
    cadence
    cycles
    guards
}
```

---

## Path

灵气运行路径。

例如：

```text
丹田
→ 青关
→ 阳枢
→ 土府
→ 丹田
```

---

## Amount

一轮使用多少灵气。

例如：

```text
5 Qi
```

---

## Flow

每 Tick 流量。

例如：

```text
1.8 Qi / Tick
```

---

## Dwell

某穴位停留时间。

例如：

```text
阳枢：1 Tick
土府：2 Tick
```

---

## Cadence

运行节奏。

例如：

```text
运行一周天
休息一 Tick
继续运行
```

---

## Cycles

运行多少周天。

炼气期：

```text
3 周天
10 周天
30 周天
```

本质为：

```text
Repeat Sequence
```

而不是自动 Loop。

---

## Guards

安全条件。

例如：

```text
如果出现明显经脉疼痛：
停止。

如果丹田压力持续升高：
降低流速。

如果稳定性快速下降：
停止当前运行。
```

---

# 18. Sequence 与 Loop

这是境界体系的重要基础。

## Sequence

筑基以前：

```text
执行一遍路线
```

例如：

```text
丹田
→ 青关
→ 阳枢
→ 丹田
```

完成以后结束。

如果继续：

> LLM 或 Runtime 再执行下一遍。

---

## Loop

筑基后：

```text
Sequence
↓
形成身体自己的稳定循环
```

即使角色不再主动逐步推动：

> 灵气仍然按照该循环继续运转。

---

# 19. 筑基定义

筑基不等于：

```text
经验值达到某个数字。
```

筑基代表身体第一次形成：

> **能够长期自行维持的稳定灵气循环。**

至少需要：

```text
PhysicalReadiness
Closure
RuleBalance
SelfSustain
DynamicStability
Internalization
```

共同满足。

---

## Closure

路线真正闭合。

---

## RuleBalance

其中的：

```text
生成
转化
放大
抑制
逸散
稳定
```

能够形成反馈平衡。

---

## SelfSustain

循环不再完全依赖外部主动推动。

---

## DynamicStability

长时间运行以后：

```text
Flow
Pressure
QiDistribution
Stability
```

保持在稳定范围，而不是发散。

---

## Internalization

稳定循环已经由：

```text
自身经脉
穴位
丹田
后天适应
```

承担。

而不是依赖：

```text
外部阵法
外部法器
特殊丹药
```

维持。

---

# 20. “触碰筑基”

不要一检测到接近条件就直接：

> 筑基成功。

例如某一循环第一次可以自行维持：

```text
18 Tick
```

可以反馈：

> 灵气完成一周天之后并未完全停下，而是自行沿原有路径再次开始流动，但很快因为火性积累而失稳。

这叫：

> 触碰筑基门槛。

然后继续优化：

```text
18 Tick
→
35 Tick
→
70 Tick
→
150 Tick
→
长期稳定
```

最后真正筑基。

---

# 21. 金丹方向

金丹暂不需要 v0.1 实现。

但底层方向应该预留。

筑基：

> Stable Loop

金丹：

> Loop 内形成 Stable Core。

即：

```text
Sequence
↓
Loop
↓
Core
```

之后每个境界都应对应：

> 一种新的稳定结构。

而不是单纯增加数值。

---

# 22. 感知体系

LLM 永远不能读取完整 World State。

整个链路：

```text
World State
↓
Rule Engine
↓
State Diff
↓
Event
↓
Perception Resolver
↓
Observation
↓
LLM / 玩家
```

---

# 23. 三层信息必须分离

## Reality

只有 Engine 知道：

```text
wood_qi = 7.318
pressure = 0.633
stability = 0.812
```

---

## Perception

角色真正感受到：

```text
流量：中等
丹田：略有胀感
青关：运行顺畅
胸口：逐渐温热
```

---

## Interpretation

Agent 自己推断：

> 阳枢可能容易激发火性。

这不是 World Fact。

它只是 Agent Knowledge。

---

# 24. 感知分为三个阶段

## Detection

知道：

> 有灵气。

## Recognition

知道：

> 大概偏木、偏火等。

## Measurement

能够进一步知道：

> 强弱、方向、变化趋势甚至精确量。

随着：

```text
神识
功法
经验
工具
```

增强逐步开放。

---

# 25. Observation 类型

建议统一为：

```text
sensation
flow
nature
structural
```

例如：

### Sensation

```text
温热
清凉
胀
疼痛
阻塞
震荡
```

### Flow

```text
进入
流出
停滞
加速
汇聚
逸散
```

### Nature

```text
生发
炽烈
厚重
锐利
流动
```

### Structural

```text
穴位松动
经脉压力
丹田充盈
结构不稳
损伤
```

---

# 26. LLM 基础交互接口

v0.1 可以有：

```text
sense
absorb
guide
stop
inspect
record_hypothesis
```

例如：

```text
sense()
```

表示：

> 主动感知环境。

---

```text
absorb(
    intensity,
    duration
)
```

表示：

> 尝试吸收灵气。

---

```text
guide(
    source,
    target,
    amount,
    intensity
)
```

表示：

> 主动引导灵气。

---

LLM 提交的是 Intent。

Engine 决定实际结果。

---

# 27. 初期不一定给精确参数

新手角色可以只能：

```text
trace
small
medium
large
```

例如：

```text
吸收少量灵气
```

Engine 根据角色能力转成真实参数。

随着控制能力提升：

```text
Flow = 1.8
Intensity = 0.42
```

等精确控制逐渐开放。

这本身可以成为成长体验。

---

# 28. Knowledge System

Agent 不应只有普通聊天 Memory。

应该维护独立：

```text
Knowledge Base
```

核心结构：

```text
Observation
Hypothesis
Evidence
Contradiction
Confidence
```

示例：

```text
Hypothesis:
青关可能偏亲木性。

Evidence:
实验12
实验15
实验21

Contradiction:
实验29

Confidence:
0.72
```

---

# 29. 从知识到功法

知识生命周期：

```text
Observation
↓
Hypothesis
↓
Experiment
↓
Repeated Validation
↓
Knowledge
↓
Protocol
```

只有形成：

```text
可解释
可重复
稳定
```

的 Protocol，才真正变成：

```text
功法
丹方
阵法
炼器方案
```

---

# 30. 玩家与 Agent 的关系

玩家不是 Agent 的遥控器。

定义为：

> 创造者 + 教练 + 长期伙伴。

核心原则：

> **玩家负责方向和价值，Agent 负责判断和执行。**

---

# 31. 玩家可以做什么

玩家可以：

```text
提出 Goal
表达 Preference
指出 Concern
提供自己的 Information
提出 Hypothesis
选择研究方向
决定风险接受度
决定什么值得固化
```

玩家不能直接：

```text
move()
absorb()
guide()
activate()
```

这些永远由 Agent 执行。

---

# 32. 修真世界中的关系设定

可以把：

```text
玩家 = 本尊
LLM = 分身
```

作为一种自然世界观。

日常情况下：

> 分身自主修炼、实验和探索。

玩家提供：

```text
研究方向
猜想
价值判断
风险判断
```

---

# 33. LLM 何时参与

整个运行体系分为三级。

## L0：World Engine

绝不调用 LLM。

负责：

```text
数值计算
灵气流动
五行作用
穴位压力
结构变化
损伤
适应
稳定态
```

---

## L1：Protocol Runtime

已经学会的东西。

例如：

```text
成熟吸气方式
已有功法
稳定丹方
成熟阵法
```

完全由 Runtime 自动执行。

不调用 LLM。

---

## L2：LLM Agent

仅处理：

```text
未知
异常
优化
创造
重大策略问题
```

---

# 34. 行为生命周期

所有新行为：

```text
未知
↓
探索
↓
验证
↓
固化
↓
自动运行
↓
出现异常
↓
重新唤醒 LLM
```

这是整个游戏最重要的 Agent 调用控制机制。

> 注记（2026-09-27，M2 落地裁决）：「验证」与「固化」之间补上回执环节——
> 协议试跑自然行满时会话层发**定性总结回执**并唤醒分身：达标则固化入册、
> 未达标则归纳定性败因（归返不足/试跑次数不足为凭/行功异象）交回修订。
> 固化语义的可执行定义：已固化功法不可被同名草案覆盖。
>
> 注记（2026-09-27，M2 落地裁决）：认知层**会话附言**通道正式登记——僵局
> 对策等认知辅助经唤醒载荷的附言段送达分身：定性词、零数字；**建议非强制**
> （听不听由分身自定）；僵局解除即撤。附言只进认知层（LLM 征询载荷），
> 不进玩家感知流正文之外的世界事实。

---

# 35. Protocol Compiler

LLM 找到稳定方式以后，不应该永久保留自然语言步骤。

应该把它编译成确定性的：

```text
Protocol
```

例如：

```yaml
protocol: qingguan_absorb_v1

preconditions:
  qingguan.stability >= safe
  dantian.pressure <= safe

actions:
  - absorb:
      target: qingguan
      intensity: low

guards:
  - if qingguan.pain >= noticeable:
      stop
  - if dantian.pressure >= high:
      reduce_flow

termination:
  dantian_state: full_enough
```

真正运行的时候：

> Runtime 读取 Protocol。

不用 LLM。

---

# 36. Exception Escalation

Runtime 自动执行过程中：

```text
正常情况
→ 自己执行

已知轻微异常
→ 使用 Guard 自动调整

无法处理异常
→ Escalate To Agent
```

例如：

```text
运行 1286 周天

正常 1279
自动调整 6
未知异常 1
```

只需要最后一次未知异常调用 LLM。

---

# 37. Knowledge Boundary

非常建议正式设计这一概念。

例如角色目前知道：

```text
青关木气 Flow:

0 ~ 2.2
已验证安全区

2.2 ~ 2.5
数据不足

> 2.5
未知
```

那么：

```text
Runtime
=
在已知空间运行

LLM
=
探索未知边界
```

核心原则：

> **Runtime 消费已知，LLM 探索未知。**

---

# 38. 功法修炼也不依赖经验值

长期执行某一功法：

```text
相关穴位 Adaptation ↑
Openness ↑
经脉 Conductance ↑
Control Error ↓
```

所以以前：

```text
Flow 1.5
已经危险
```

后来：

```text
Flow 2.2
仍然稳定
```

这就代表：

> 功法练得更深了。

不一定需要：

```text
功法经验 +5
```

作为底层逻辑。

---

# 39. 境界同样不是经验等级

境界应该代表：

> 身体内部形成了一种新的稳定结构。

目前初步定义：

```text
炼气：
Sequence

筑基：
Stable Loop

金丹：
Stable Core
```

未来继续：

```text
元婴
更高境界
```

也应首先回答：

> Engine 中到底产生了哪一种以前不存在的结构？

如果只是：

```text
灵气更多
攻击更高
```

就不应该成为独立境界。

---

# 40. MVP 建议

第一阶段不要实现完整修真世界。

只验证：

> **玩家 + Agent 是否能够真正共同发现第一门功法，并最终向稳定 Loop 演化。**

---

## Body Graph

6~9 个虚构穴位即可。

例如：

```text
下丹田
青关
阳枢
土府
金门
玄泉
神庭
中庭
命门
```

---

## 角色

只有：

```text
Affinity[5]
Control
Perception
```

---

## 穴位

```text
Affinity[5]
Capacity
Throughput
Retention
Openness
Stability
Damage
Adaptation[5]
```

---

## 经脉

```text
Conductance
Throughput
Damage
```

---

## 环境

一个修炼房间。

支持：

```text
EnvironmentQi[5]
```

---

## 可执行操作

```text
sense
absorb
guide
stop
inspect
```

---

## Agent 目标

从：

```text
完全不知道怎么修炼
```

开始。

逐步：

```text
发现灵气
↓
完成第一次吸收
↓
发现第一个穴位
↓
找到稳定吸收参数
↓
固化吸收 Protocol
↓
探索第二个穴位
↓
开始运行 Sequence
↓
发现稳定周天
↓
尝试形成自循环
↓
触碰筑基
```

---

# 41. MVP 成功标准

第一阶段不看：

```text
美术
剧情
宗门
战斗
地图
PVP
```

只看以下问题。

### 验证 1

不同角色是否因为：

```text
亲和
穴位
经脉
```

而自然形成不同最优路径？

---

### 验证 2

LLM 是否能够仅通过 Observation：

```text
提出假设
做控制变量实验
逐渐识别穴位特性
```

---

### 验证 3

LLM 是否能够真正创造：

```text
Path
+
Amount
+
Flow
+
Dwell
+
Cadence
+
Guard
```

组成的稳定 Technique？

---

### 验证 4

Technique 被固化后：

> 是否能够完全脱离 LLM 自动运行？

---

### 验证 5

自动运行出现未知异常时：

> 是否能够正确重新唤醒 Agent？

---

### 验证 6

玩家是否感觉：

> “这是我和自己的分身一起研究出来的功法。”

而不是：

> “AI 自动算出了一个答案。”

这是最重要的产品验证。

---

# 42. 暂缓实现的内容

第一阶段不要过早加入：

```text
炼丹
炼器
阵法
战斗
宗门
多人
经济
装备
任务
完整境界树
真实人体穴位体系
几十种材料
复杂天气
```

先把：

> 修炼探索 → 知识形成 → Protocol → 自动化 → 筑基

这一条闭环跑通。

---

# 43. 后续扩展方向

当修炼系统成立以后，同一套框架可以复用。

## 炼丹

```text
材料
+
温度
+
顺序
+
时间
+
五行变化
↓
实验
↓
丹方 Protocol
```

成熟丹方以后：

> Runtime 自动炼制。

---

## 阵法

```text
阵眼 = Node
阵纹 = Channel
灵气 = Essence
```

几乎可以复用 Body Graph。

---

## 炼器

```text
材料
+
Component
+
结构
+
能量流动
```

继续使用同一套基础 Operator。

---

## 战斗

角色已有：

```text
功法
身体结构
灵气循环
```

直接进入战斗行为。

战斗不需要另一套独立“技能系统”。

---

# 44. 给开发 Agent 的实现原则

后续设计和编码时必须遵守：

### 原则一

不要为了实现具体修真概念不断增加特殊 if/else。

优先检查：

> 是否可以由已有基础属性和 Operator 表达？

---

### 原则二

所有真实状态只存在 Engine。

不要允许 LLM 直接写 World State。

---

### 原则三

Perception 与 Reality 分离。

LLM 和玩家看到：

> 角色能感知到的信息。

不是 Engine 调试数据。

---

### 原则四

Agent 的推断不能反向改变世界规律。

```text
Agent Knowledge ≠ World Truth
```

---

### 原则五

已经掌握的行为必须尽快 Protocol 化。

避免重复调用 LLM。

---

### 原则六

新增参数必须回答：

> 它是否能产生新的可观察策略差异？

如果不能，不要加入。

---

### 原则七

规则尽可能简单。

复杂性来自：

```text
简单公式
×
Body Graph
×
五行关系
×
时间
×
不同角色
×
不同操作
```

而不是来自一个巨大复杂公式。

---

# 45. 建议开发 Agent 下一步任务

优先按照以下顺序推进。

## P0 — 建立最小模拟器

实现：

```text
Character
Acupoint
Channel
Environment
World Tick
```

以及：

```text
吸收
传输
压力
稳定
简单相生相克
```

---

## P1 — Body Graph

构造：

```text
6~9 个穴位
若干经脉
```

并让不同随机角色获得不同：

```text
Affinity
```

---

## P2 — Perception Resolver

完成：

```text
Reality
→ Event
→ Perception
→ Observation
```

保证角色无法直接读取真实隐藏参数。

---

## P3 — Agent Protocol

实现：

```text
sense
absorb
guide
stop
inspect
```

---

## P4 — Knowledge System

支持：

```text
Observation
Hypothesis
Evidence
Confidence
```

---

## P5 — Protocol Compiler / Runtime

允许 Agent 把稳定行为固化为：

```text
Protocol
```

之后不再调用 LLM。

---

## P6 — Exception Escalation

成熟 Protocol 遇到未知情况：

```text
Runtime
→ Escalate
→ Agent
```

---

## P7 — Sequence → Loop

模拟：

```text
反复 Sequence
↓
穴位适应
↓
经脉改善
↓
自维持能力增加
↓
形成短暂自循环
↓
长期 Stable Loop
↓
筑基
```

---

# 46. 项目最核心的验证问题

最终先不要问：

> 这个游戏有没有 100 个功法？

而要问：

> **只有一套底层规则，没有任何预置功法时，玩家和 LLM 是否能够共同发现并创造出多套性质不同、可稳定执行的功法？**

例如自然出现：

```text
高效率高风险功法

低效率高稳定功法

偏木恢复型功法

火土高速转化功法

小流量长周期功法

大流量短周期功法
```

如果这些差异来自规则自然演化，而不是策划提前定义：

> 第一阶段世界模型即验证成功。

---

# 47. 最终架构摘要

```text
               Player / 本尊
                    │
           Goal / Hypothesis
                    │
                    ▼
                LLM 分身
          探索 / 推理 / 创造
                    │
                  Intent
                    │
                    ▼
            Agent Protocol
                    │
          ┌─────────┴─────────┐
          │                   │
        未知                 已知
          │                   │
          ▼                   ▼
        LLM             Protocol Runtime
          │                   │
          └─────────┬─────────┘
                    ▼
               World Engine
                    │
           Deterministic Rules
                    │
                    ▼
                World State
                    │
                    ▼
              Event Resolver
                    │
                    ▼
            Perception Resolver
                    │
             Observation
                    │
          ┌─────────┴────────┐
          ▼                  ▼
        Player              Agent
```

最终牢记三个职责：

> **Engine 生产因果。**

> **Agent 生产知识。**

> **Player 生产方向与价值。**

以及整个系统最核心的一句话：

> **Runtime 消费已知，LLM 探索未知。**
