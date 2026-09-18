# 修炼数值模型与公式规范 v0.1

## 1. 文档目标

本文档用于定义修真世界 v0.1 的确定性数值模型。

目标不是追求真实物理，而是建立一套：

* 简单
* 可组合
* 可解释
* 可仿真
* 可调参
* 可由 LLM 通过现象逐步发现

的规则系统。

World Engine 应能在完全没有 LLM 的情况下，根据：

```text
角色
+
身体结构
+
环境
+
操作参数
+
时间
```

确定性地计算：

```text
灵气吸收
灵气传输
五行转化
五行抑制
积累
逸散
压力
稳定性
损伤
适应
Sequence运行
Loop形成
筑基条件
```

LLM 永远不能修改这些公式。

---

# 2. 数值约定

## 2.1 五行顺序

所有五行数组统一：

```text
[木, 火, 土, 金, 水]
```

记为：

```text
e ∈ {wood, fire, earth, metal, water}
```

---

## 2.2 基础范围

大部分归一化属性使用：

```text
0.0 ~ 1.0
```

例如：

```text
Openness
Stability
Damage
Activity
Adaptation
```

亲和属性使用：

```text
0.5 ~ 1.5
```

其中：

```text
1.0 = 普通
>1.0 = 亲和
<1.0 = 排斥 / 不适合
```

---

# 3. Character 数据结构

```text
Character {
    affinity[5]

    control
    perception

    body_graph
}
```

## affinity

先天五行亲和：

```text
CharacterAffinity[e]
```

建议普通角色范围：

```text
0.75 ~ 1.25
```

特殊体质未来可以突破：

```text
0.50 ~ 1.60
```

---

## control

```text
0 ~ 1
```

影响：

* 实际流量与目标流量误差
* 大流量操作稳定性
* 参数控制精度

v0.1 可先用于流量误差：

```text
ControlErrorMax
=
0.20 × (1 - Control)
```

例如：

```text
Control = 0.5

最大误差约：
±10%
```

---

# 4. 穴位模型

```text
Acupoint {
    affinity[5]

    capacity
    throughput
    retention

    openness
    base_stability
    stability

    damage

    adaptation[5]

    qi[5]
    activity[5]

    previous_flow
}
```

---

# 5. 经脉模型

```text
Channel {
    from
    to

    base_conductance
    throughput

    damage
}
```

---

# 6. 环境模型

```text
Environment {
    qi[5]
    activity[5]

    temperature
}
```

v0.1 暂时不引入：

```text
湿度
压力
空间灵气流场
```

除非后续验证确实产生玩法价值。

---

# 7. 灵气吸收

角色通过穴位 i 从环境吸收属性 e。

基础公式：

```text
Intake[e]
=
EnvironmentQi[e]
× CharacterAffinity[e]
× AcupointAffinity[i,e]
× EffectiveOpenness[i]
× TechniqueIntensity
× BackPressure[i]
× ActivityFactor[e]
```

其中：

```text
EffectiveOpenness
=
Openness × (1 - Damage)
```

---

## 7.1 BackPressure

当前穴位总负载：

```text
Load[i]
=
Σ Qi[i,e]
```

负载比：

```text
LoadRatio[i]
=
Load[i] / Capacity[i]
```

反压：

```text
BackPressure[i]
=
1 / (1 + LoadRatio[i]^2)
```

特点：

```text
LoadRatio = 0
BackPressure = 1

LoadRatio = 0.5
BackPressure = 0.80

LoadRatio = 1
BackPressure = 0.50

LoadRatio = 2
BackPressure = 0.20
```

不会突然“容量满就无法吸收”。

而是越来越困难。

---

## 7.2 ActivityFactor

环境中的灵气活跃程度：

```text
ActivityFactor[e]
=
0.5 + 0.5 × EnvironmentActivity[e]
```

因此：

```text
Activity = 0
仍然可以吸收，但效率较低。

Activity = 1
完全发挥。
```

---

# 8. 灵气传输

从穴位 i 经 Channel 到穴位 j。

首先计算元素 e 的有效传导能力：

```text
Conductance[i,j,e]
=
BaseConductance[i,j]
× sqrt(
    EffectiveAffinity[i,e]
    × EffectiveAffinity[j,e]
)
× EffectiveOpenness[i]
× EffectiveOpenness[j]
× (1 - ChannelDamage)
```

其中：

```text
EffectiveAffinity[i,e]
=
AcupointAffinity[i,e]
× Adaptation[i,e]
```

---

## 8.1 ChannelCapacity

```text
ChannelCapacity[e]
=
min(
    ChannelThroughput,
    SourceAcupointThroughput,
    TargetAcupointThroughput
)
× Conductance[i,j,e]
```

---

## 8.2 实际流量

LLM / Protocol 请求：

```text
RequestedFlow[e]
```

考虑角色控制误差：

```text
ActualRequestedFlow[e]
=
RequestedFlow[e]
× (1 + ε)
```

其中：

```text
ε ∈ [-ControlErrorMax, +ControlErrorMax]
```

如果要求完全确定性，可使用：

```text
ε = deterministic_noise(seed, tick)
```

而不是随机系统级 RNG。

最终：

```text
ActualFlow[e]
=
min(
    ActualRequestedFlow[e],
    SourceQi[e],
    ChannelCapacity[e]
)
```

---

# 9. 传输损耗

定义：

```text
TransferEfficiency
=
clamp(
    Conductance,
    0,
    1
)
```

则：

```text
DeliveredQi
=
ActualFlow × TransferEfficiency
```

损耗：

```text
TransferLoss
=
ActualFlow - DeliveredQi
```

v0.1 中损耗分配：

```text
50% → Diffuse
50% → Heat / LocalStrain
```

以后可以再细化。

---

# 10. 五行相生

相生关系：

```text
木 → 火
火 → 土
土 → 金
金 → 水
水 → 木
```

在穴位 i 内：

```text
TransformAmount
=
SourceQi
× SourceActivity
× BaseShengRate
× LocalTransformAffinity
× TimeFactor
```

其中：

```text
LocalTransformAffinity
=
sqrt(
    EffectiveAffinity[source]
    × EffectiveAffinity[target]
)
```

v0.1：

```text
BaseShengRate = 0.04 / Tick
```

---

## 10.1 转化效率

```text
TargetGain
=
TransformAmount
× ShengEfficiency
```

默认：

```text
ShengEfficiency = 0.90
```

源减少：

```text
SourceQi -= TransformAmount
```

目标增加：

```text
TargetQi += TargetGain
```

剩余：

```text
TransformLoss
=
TransformAmount
× (1 - ShengEfficiency)
```

转化成：

```text
Heat / Strain
```

---

# 11. 五行相克

相克关系：

```text
木克土
土克水
水克火
火克金
金克木
```

v0.1 不直接删除被克元素数量。

优先压制：

```text
Activity
```

计算：

```text
Suppression
=
BaseKeRate
× CounterRatio
× CounterActivity
× LocalKeAffinity
```

其中：

```text
CounterRatio
=
CounterQi
/
(CounterQi + TargetQi + epsilon)
```

```text
LocalKeAffinity
=
sqrt(
    EffectiveAffinity[counter]
    × EffectiveAffinity[target]
)
```

默认：

```text
BaseKeRate = 0.15
```

然后：

```text
TargetActivity'
=
TargetActivity
× clamp(
    1 - Suppression,
    0,
    1
)
```

---

# 12. Activity 自然恢复

被压制的 Activity 不应该永久降低。

每 Tick：

```text
Activity(t+1)
=
Activity(t)
+
ActivityRecoveryRate
× (
    NaturalActivity
    - Activity(t)
)
```

默认：

```text
ActivityRecoveryRate = 0.10
```

---

# 13. 灵气逸散 Diffuse

每个穴位都有 Retention：

```text
0 ~ 1
```

自然逸散：

```text
DiffuseAmount[e]
=
Qi[e]
× BaseDiffuseRate
× (1 - Retention)
× Activity[e]
```

默认：

```text
BaseDiffuseRate = 0.02 / Tick
```

因此：

```text
Retention高
→ 更适合储存

Retention低
→ 更适合作为流动节点
```

---

# 14. 穴位压力

```text
Load
=
Σ Qi[e]
```

```text
LoadRatio
=
Load / Capacity
```

基础压力：

```text
Pressure
=
LoadRatio²
```

例如：

```text
0.5 → 0.25
0.8 → 0.64
1.0 → 1.0
1.2 → 1.44
```

---

# 15. Flow Shock

穴位本 Tick 总流量：

```text
CurrentFlow
```

上一 Tick：

```text
PreviousFlow
```

定义：

```text
FlowShock
=
abs(CurrentFlow - PreviousFlow)
/
max(Throughput, epsilon)
```

用于描述：

> 突然猛推一股灵气。

---

# 16. 五行 Compatibility

首先：

```text
QiRatio[e]
=
Qi[e] / TotalQi
```

定义当前穴位平均兼容度：

```text
Compatibility
=
Σ(
    QiRatio[e]
    × clamp(EffectiveAffinity[e], 0, 1)
)
```

这里暂时将：

```text
Affinity > 1
```

截断到 1。

因为 Compatibility 主要表达：

> 是否不适合。

高于 1 的亲和优势，应通过吸收、转化、适应体现。

---

# 17. Strain

压力、五行不适和流量冲击共同产生负荷。

```text
Overload
=
max(
    0,
    LoadRatio - 1
)
```

```text
AffinityStrain
=
max(
    0,
    1 - Compatibility
)
```

总 Strain：

```text
Strain
=
W_overload × Overload²
+
W_affinity × AffinityStrain²
+
W_shock × FlowShock²
+
TransformHeat
```

v0.1 默认：

```text
W_overload = 0.60
W_affinity = 0.20
W_shock = 0.20
```

`TransformHeat` 建议先归一化后进入：

```text
0 ~ 0.2
```

---

# 18. Stability

每个穴位当前稳定性：

```text
0 ~ 1
```

恢复量：

```text
Recovery
=
BaseRecovery
× (1 - Damage)
× EarthStabilization
```

默认：

```text
BaseRecovery = 0.01 / Tick
```

土性稳定影响：

```text
EarthStabilization
=
1
+
EarthQiRatio
× EarthActivity
× 0.20
```

最终：

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

---

# 19. Damage

当 Stability 足够低时开始产生结构损伤。

```text
DamageThreshold = 0.40
```

如果：

```text
Stability < 0.40
```

则：

```text
DamageGain
=
(0.40 - Stability)
× DamageRate
```

默认：

```text
DamageRate = 0.03
```

然后：

```text
Damage(t+1)
=
clamp(
    Damage(t) + DamageGain,
    0,
    1
)
```

---

# 20. Damage 的实际影响

损伤应该真正反馈到系统：

```text
EffectiveOpenness
=
Openness × (1 - Damage)
```

```text
EffectiveCapacity
=
Capacity × (1 - 0.5 × Damage)
```

```text
EffectiveThroughput
=
Throughput × (1 - Damage)
```

因此：

> 受伤不是减 HP。

而是改变身体运行能力。

---

# 21. 自然恢复

没有高负载时：

```text
if Pressure < 0.4
and Strain < 0.05:
```

允许：

```text
DamageRecovery
=
BaseDamageRecovery
× BodyRecoveryModifier
```

默认：

```text
BaseDamageRecovery = 0.001 / Tick
```

恢复应该远慢于受伤。

---

# 22. Adaptation

长期安全运行某种属性，可以提高穴位对此属性的后天适应。

只有满足：

```text
Stability >= 0.75

Pressure <= 0.8

QiFlow[e] > MinimumEffectiveFlow
```

才增加 Adaptation。

公式：

```text
AdaptationGain[e]
=
AdaptationRate
× SafeFlowRatio
× (
    AdaptationMax - Adaptation[e]
)
```

其中：

```text
SafeFlowRatio
=
min(
    QiFlow[e] / Throughput,
    1
)
```

默认：

```text
AdaptationRate = 0.0005 / Tick
AdaptationMax = 1.25
```

这意味着成长很慢。

---

# 23. Openness 成长

如果穴位长期在安全压力区运行：

```text
0.30 <= Pressure <= 0.75
```

并且：

```text
Stability >= 0.80
```

则：

```text
OpennessGain
=
OpenRate
× Pressure
× (1 - Openness)
```

默认：

```text
OpenRate = 0.0003 / Tick
```

所以：

> 完全不使用不会打开。

> 强行过载也不会更快打开。

存在一个适宜刺激区间。

---

# 24. 经脉 Adaptation

经脉暂时不增加五行独立 Adaptation。

长期安全使用：

```text
ChannelConductanceAdapt
```

定义额外系数：

```text
ChannelAdaptation
=
1.0 ~ 1.30
```

增量：

```text
ChannelAdaptGain
=
ChannelAdaptRate
× SafeFlowRatio
× (
    1.30 - ChannelAdaptation
)
```

默认：

```text
ChannelAdaptRate = 0.0003
```

然后：

```text
BaseConductanceEffective
=
BaseConductance
× ChannelAdaptation
```

---

# 25. Sequence 定义

```text
Sequence {
    steps[]

    cycle_count
    cadence
    guards
}
```

Step：

```text
Step {
    source
    target

    nature_filter?

    requested_amount
    requested_flow

    dwell_ticks
}
```

例如：

```text
丹田 → 青关
木性为主
Amount = 5
Flow = 1.5
Dwell = 1

青关 → 阳枢
Flow = 1.3

阳枢 → 土府
Flow = 1.0
Dwell = 2

土府 → 丹田
Flow = 0.9
```

---

# 26. 单周天有效率

定义一周天：

```text
CycleInput
=
本轮从丹田 / 环境投入的有效 Qi
```

最后重新回到目标核心节点：

```text
CycleOutput
```

循环效率：

```text
CycleEfficiency
=
CycleOutput / CycleInput
```

例如：

```text
Input = 10
Output = 7.8

CycleEfficiency = 0.78
```

---

# 27. Cycle Stability

一周天期间：

```text
CycleStability
=
平均所有参与节点 Stability
```

也记录：

```text
MinimumCycleStability
```

因为平均值可能掩盖局部危险。

---

# 28. 周天安全判定

v0.1：

```text
SuccessfulCycle =
CycleEfficiency >= 0.50

AND MinimumCycleStability >= 0.60

AND NewDamage <= DamageTolerance
```

默认：

```text
DamageTolerance = 0.005
```

这只是“成功跑完”。

不是“优秀功法”。

---

# 29. 功法 Protocol 的验证

一个 Technique 想从实验方案变成成熟 Protocol：

建议满足至少：

```text
SuccessfulCycles >= 20
```

并且：

```text
SuccessRate >= 95%
```

以及：

```text
AverageMinimumStability >= 0.75
```

并且：

```text
没有未知异常
```

才进入：

```text
Validated Protocol
```

---

# 30. Known Safe Range

一个成熟 Protocol 不应只保存单点参数。

例如：

```text
Flow = 1.8
```

而应保存：

```text
ValidatedFlowRange:
[1.5, 2.0]

Preferred:
1.8
```

Knowledge Boundary 示例：

```text
0 ~ 2.0
已验证

2.0 ~ 2.3
少量实验

>2.3
未知
```

Runtime：

> 只能在已验证区域自动运行。

LLM：

> 负责探索未知区域。

---

# 31. Loop 形成

这是炼气 → 筑基的核心。

Sequence 即使重复很多次，也不自动成为 Loop。

需要定义：

```text
Closure
RuleBalance
SelfSustain
DynamicStability
Internalization
```

---

# 32. Closure

如果路径：

```text
A → B → C → A
```

则：

```text
TopologyClosure = 1
```

否则：

```text
0
```

但还需要数量闭合。

定义：

```text
FlowClosure
=
min(
    CycleOutput / CycleInput,
    1
)
```

最终：

```text
Closure
=
TopologyClosure
× FlowClosure
```

---

# 33. RuleBalance

用于判断这个循环是不是：

```text
一路无限放大
```

还是：

```text
生成 / 消耗 / 转化 / 抑制 / 逸散
形成平衡
```

先定义每 Tick 的净系统变化：

```text
DeltaEnergy
=
Generated
+
ExternalInput
-
Consumed
-
Diffuse
-
Loss
```

归一化：

```text
EnergyImbalance
=
abs(DeltaEnergy)
/
max(
    TotalActiveEnergy,
    epsilon
)
```

再记录五行分布变化：

```text
NatureDrift
=
Σ abs(
    NatureRatio(t)
    -
    NatureRatio(t-N)
)
```

第一版：

```text
RuleBalance
=
clamp(
    1
    - W_energy × EnergyImbalance
    - W_nature × NatureDrift,
    0,
    1
)
```

默认：

```text
W_energy = 0.6
W_nature = 0.4
```

---

# 34. SelfSustain

关键定义：

> 当主动推动停止以后，这个循环还能继续多少。

测试方法：

正常运行稳定后：

```text
ControlInput = 0
ExternalPush = 0
```

观察后续。

定义：

```text
SelfSustain
=
AverageFlowAfterRelease
/
AverageFlowBeforeRelease
```

例如：

```text
停止主动控制前：
平均 Flow = 2.0

停止后：
平均 Flow = 1.7

SelfSustain = 0.85
```

这就是非常高的自维持。

---

# 35. Dynamic Stability

观察一个时间窗口：

```text
N = 50 Tick
```

记录：

```text
Flow
Pressure
NatureRatio
Stability
```

计算归一化波动：

```text
FlowVariance
PressureVariance
NatureVariance
StabilityVariance
```

可以先：

```text
DynamicInstability
=
0.25 × FlowVar
+
0.25 × PressureVar
+
0.25 × NatureVar
+
0.25 × StabilityVar
```

然后：

```text
DynamicStability
=
clamp(
    1 - DynamicInstability,
    0,
    1
)
```

具体方差归一化方式由实现统一处理。

---

# 36. Internalization

用于区分：

> 身体自己完成。

与：

> 靠阵法 / 丹药 / 外部法器强行维持。

定义：

```text
InternalContribution
=
来自角色自身穴位、经脉、丹田的有效贡献

ExternalContribution
=
来自外部设备、持续丹药、阵法的贡献
```

则：

```text
Internalization
=
InternalContribution
/
(
    InternalContribution
    +
    ExternalContribution
)
```

如果完全依赖自身：

```text
≈ 1
```

---

# 37. 筑基候选条件

v0.1 建议：

```text
Closure >= 0.80

RuleBalance >= 0.80

SelfSustain >= 0.70

DynamicStability >= 0.80

Internalization >= 0.80
```

并且：

```text
MinimumStability >= 0.70
```

持续：

```text
CandidateTicks >= 30
```

则进入：

> 触碰筑基。

---

# 38. 真正筑基条件

继续长期运行：

```text
Closure >= 0.90

RuleBalance >= 0.85

SelfSustain >= 0.85

DynamicStability >= 0.88

Internalization >= 0.90

MinimumStability >= 0.75
```

持续：

```text
StableTicks >= 100
```

然后：

```text
BodyPhase = FOUNDATION
```

这是 v0.1 调试值。

必须通过模拟器校准。

---

# 39. 筑基后的变化

筑基后不是直接：

```text
属性 +20%
```

而是 Engine 允许这个稳定结构注册成：

```text
AutonomousLoop
```

Loop：

```text
Loop {
    protocol
    target_flow
    guards
    stability_state
}
```

每 Tick 由 Runtime 自动推进。

不需要 LLM。

---

# 40. LLM 什么时候参与

## 必须调用

```text
首次感知未知现象
探索新穴位
测试未知参数区间
设计新 Sequence
分析失败
处理未见异常
尝试形成 Loop
优化功法
```

---

## 不需要调用

```text
成熟吸收 Protocol
成熟 Sequence
已有安全参数运行
正常周天
已知异常的 Guard 处理
筑基后的稳定 Loop
```

---

# 41. Exception Escalation

Runtime 首先：

```text
正常
→ 继续

进入已知警戒区
→ Guard自动调整

退出已知边界
→ Stop
→ EscalateToAgent
```

例如：

```text
Protocol正常 Flow:
1.5 ~ 2.0

当前系统要求：
2.25

超出已知安全区
```

Runtime 不应该自己尝试。

应该：

```text
pause
→ Agent
```

---

# 42. Perception Formula

LLM 不读取真实数值。

例如 Engine：

```text
Pressure = 0.66
```

Perception Resolver 根据：

```text
Perception
Experience
KnownKnowledge
```

产生感知。

简单 v0.1：

```text
PerceivedSignalStrength
=
ActualChange
× CharacterPerception
× Familiarity
```

其中：

```text
Familiarity
=
0.5 + 0.5 × KnowledgeConfidence
```

---

# 43. 感知分级

例如压力：

```text
Pressure < 0.25
→ 无明显感觉

0.25 ~ 0.45
→ 轻微充盈

0.45 ~ 0.65
→ 明显胀感

0.65 ~ 0.85
→ 强烈胀感 / 不适

0.85 ~ 1.0
→ 疼痛

> 1.0
→ 强烈疼痛 / 失稳风险
```

但真正输出还要经过 Perception。

低感知角色可能：

```text
0.70
```

只感受到：

> 有些不舒服。

高手则能判断：

> 下丹田压力正在持续上升。

---

# 44. Stability 感知

例如：

```text
>= 0.90
极其稳定

0.75 ~ 0.90
稳定

0.60 ~ 0.75
轻微波动

0.40 ~ 0.60
明显不稳

0.20 ~ 0.40
接近失控

< 0.20
结构崩溃
```

---

# 45. 一个完整数值实验

以下作为开发 Agent 的测试 Case。

## 环境

```text
EnvironmentQi:

木 = 10
火 = 1
土 = 1
金 = 0.5
水 = 4

Activity:
全部 = 1
```

---

## 角色

```text
CharacterAffinity:

木 = 1.25
火 = 0.80
土 = 0.90
金 = 0.75
水 = 1.10

Control = 0.70
Perception = 0.50
```

---

## 青关

```text
Affinity:

木 = 1.20
火 = 0.75
土 = 0.90
金 = 0.85
水 = 1.00

Capacity = 10
Throughput = 3

Retention = 0.40

Openness = 0.80

Stability = 0.95

Damage = 0

Adaptation:
全部 = 1
```

---

# 46. Tick 1：吸收

功法：

```text
TechniqueIntensity = 0.60
```

当前穴位为空：

```text
BackPressure = 1
```

木：

```text
10
× 1.25
× 1.20
× 0.80
× 0.60
× 1
× 1

= 7.20
```

火：

```text
1
× 0.80
× 0.75
× 0.80
× 0.60

= 0.288
```

土：

```text
1
× 0.90
× 0.90
× 0.80
× 0.60

= 0.389
```

金：

```text
0.5
× 0.75
× 0.85
× 0.80
× 0.60

= 0.153
```

水：

```text
4
× 1.10
× 1.00
× 0.80
× 0.60

= 2.112
```

进入青关总量：

```text
10.142
```

---

# 47. 青关立即进入高负载

因为：

```text
Capacity = 10
```

所以：

```text
LoadRatio = 10.142 / 10 = 1.0142
```

```text
Pressure
=
1.0142²
≈ 1.029
```

已经轻微超过承载。

这说明：

> intensity = 0.60 对当前角色其实偏高。

这个结果很重要。

不是所有第一次吸收都应该成功得非常舒服。

---

# 48. 下一 Tick BackPressure

```text
BackPressure
=
1 / (1 + 1.0142²)

≈ 0.493
```

如果继续同样吸收：

木本 Tick 只有约：

```text
7.20 × 0.493
≈ 3.55
```

吸收能力明显下降。

LLM 感知：

> 第一轮灵气涌入十分明显，但很快青关产生强烈充盈感，继续牵引时阻力显著增加。

---

# 49. Agent 调整

Agent可能决定：

```text
TechniqueIntensity:
0.60 → 0.25
```

并将青关灵气向丹田传输。

此时开始：

```text
吸收
+
传输
```

达到动态平衡。

这就是：

> 吐纳法最初的雏形。

---

# 50. 一条简单 Sequence

假设：

```text
Environment
→ 青关
→ 下丹田
→ 青关
```

Agent设计：

```text
吸收 intensity = 0.25

青关 → 丹田
Flow = 1.5

丹田 → 青关
Flow = 0.8
```

运行 10 周天。

Engine 记录：

```text
CycleEfficiency
MinimumStability
Pressure
Damage
NatureDrift
```

然后 LLM 才能决定：

> 这是不是一个可继续研究的功法。

---

# 51. 完整开发执行顺序

建议开发 Agent 按以下顺序实现测试。

### Test 1

单穴位：

```text
环境 → 穴位
```

验证：

```text
Intake
BackPressure
Pressure
Stability
```

---

### Test 2

两个穴位：

```text
A → B
```

验证：

```text
Conductance
ActualFlow
TransferLoss
```

---

### Test 3

单穴位五行：

```text
木 + 火 + 水
```

验证：

```text
Sheng
Ke
Activity
```

---

### Test 4

Sequence：

```text
A → B → C → A
```

验证：

```text
CycleEfficiency
CycleStability
```

---

### Test 5

长期运行：

```text
1000 Tick
```

验证：

```text
Adaptation
Openness
ChannelAdaptation
Damage
```

---

### Test 6

停止主动控制。

验证：

```text
SelfSustain
```

---

### Test 7

满足条件后：

```text
Sequence → Autonomous Loop
```

验证：

> 筑基。

---

# 52. 数值设计原则

所有当前数值：

```text
0.04
0.15
0.60
0.20
100 Tick
```

等都只是：

> v0.1 初始实验参数。

不属于世界不可变规则。

真正不可轻易修改的是：

```text
公式关系
数据结构
因果方向
```

例如：

```text
压力随负载非线性增加。
```

可以保留。

但：

```text
平方
```

将来可以调整。

---

# 53. 调参策略

不要直接人工凭感觉调。

开发 Agent 应建立 Monte Carlo / Simulation Test：

随机生成：

```text
1000 个角色
1000 套身体参数
若干环境
若干 Sequence
```

观察：

```text
完全无法修炼比例
过于容易稳定比例
Damage分布
筑基时间
不同亲和角色差异
不同路线差异
```

目标是避免：

```text
只有一种最优路线
```

或者：

```text
所有路线都差不多。
```

---

# 54. v0.1 最重要的数学目标

这套系统必须自然产生三类路线：

### 高效率 / 高风险

```text
Flow 高
Pressure 高
Growth 快
Damage Risk 高
```

### 中等平衡

```text
Efficiency 中
Stability 高
```

### 低效率 / 极稳定

```text
Flow 低
Damage 几乎没有
成长慢
```

如果这三个策略自然出现：

> 数值模型基本健康。

---

# 55. 第一阶段最终成功条件

在没有任何预置功法的情况下：

让不同角色、不同 Agent 自己探索。

最终自然产生：

```text
角色 A：
功法 A
效率高
火性积累严重

角色 B：
功法 B
速度慢
稳定极高

角色 C：
功法 C
五行循环复杂
SelfSustain 高
```

并且只有满足：

```text
Closure
+
RuleBalance
+
SelfSustain
+
DynamicStability
+
Internalization
```

的路线，才能最终形成：

```text
Stable Loop
```

进而：

> 筑基。

---

# 56. 核心总结

整个数值系统应始终满足：

> **公式简单。**

> **关系稳定。**

> **组合复杂。**

> **反馈连续。**

> **结果不预设。**

Engine 不定义：

```text
《青木诀》
《烈火心经》
《玄水功》
```

Engine 只定义：

```text
灵气如何进入
如何流动
如何变化
如何失衡
如何适应
如何形成稳定结构
```

LLM 与玩家根据这些结果：

> 创造功法。

因此整个系统最核心的职责仍然是：

```text
Engine
生产因果

Agent
发现规律

Player
决定方向
```

而在运行层：

> **Runtime 消费已知，LLM 探索未知。**
