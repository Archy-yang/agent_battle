# 修真世界：境界结构与感知映射规则

> 开发指导规范 v0.1
>
> 本文只收敛两类已经讨论到足以指导实现的规则：
> 1. 境界如何由底层结构变化产生，尤其是炼气 -> 筑基 -> 金丹，以及更高境界应如何继续定义。
> 2. Engine 数值如何在不依赖 LLM 润色的情况下，确定性映射为“微热、充盈、胀痛、刺痛、滞涩、顺畅”等角色感知。

---

## 1. 文档状态约定

本文中的规则分三类：

- **[固定原则]**：当前架构必须遵守，不应随数值调优轻易改变。
- **[v0.1 参数]**：为了让模拟器能运行而给出的初始阈值或权重，后续必须通过仿真调整。
- **[待定义]**：方向已确定，但具体结构尚未最终设计，不应直接硬编码成正式规则。

最重要的边界：

> **境界不是经验等级，而是 Engine 中新稳定结构的出现。**
>
> **感知文本不是 LLM 创作，而是数值状态经过确定性 Sensory Model 映射后的结果。**

---

# Part A. 境界结构规则

## 2. 境界系统的根本定义

### 2.1 [固定原则] Realm = Stable Structure Phase

境界用于描述角色体内“当前存在哪一种稳定结构”，而不是描述：

- 累计修炼次数；
- 经验值；
- 总灵气量；
- 战斗力；
- 玩家点击了某个突破按钮。

可将境界理解为物理系统的相态：水、冰、蒸汽不是 1/2/3 级，而是同一底层物质处于不同稳定结构。

因此，每增加一个境界，开发必须先回答：

> **这个境界在 Engine 里新增了什么上一境界不存在的、可以独立检测的稳定结构？**

如果答案只是“灵气更多、身体更强、攻击更高”，则该境界不应作为独立底层相态存在。

### 2.2 Engine 概念与世界观名称分离

Engine 可以只保存结构状态：

```text
DISPERSED_SEQUENCE
SELF_SUSTAINING_LOOP
STABLE_ENERGY_CORE
...
```

修真文明再把它们命名为：

```text
炼气
筑基
金丹
...
```

这样上层世界观命名不会污染底层规则。

---

## 3. 炼气：Sequence Phase

### 3.1 [固定原则] 炼气不是“低等级”，而是“尚无自维持结构”

炼气阶段角色已经可以：

- 感知灵气；
- 吸收灵气；
- 在穴位和经脉间主动引导；
- 储存灵气；
- 反复执行一段运行路线；
- 将稳定操作固化为 Protocol Runtime。

但是角色体内仍不存在可以自行运转的稳定周天。

基本特征：

```text
ControlInput > 0  -> Sequence 可继续
ControlInput = 0  -> Flow 快速衰减并停止
```

### 3.2 Sequence 与自动 Protocol 不应混淆

炼气期已经允许已知动作自动执行。

例如：

```text
《青关纳气法》
环境 -> 青关 -> 丹田
```

可以由 Runtime 自动重复，而无需 LLM 每 Tick 参与。

但这仍然不等于筑基，因为它依赖的是外部执行器持续“再次发出动作”，而不是身体内部结构自行续行。

因此必须区分：

```text
Protocol Automation
= Runtime 自动重复 Sequence

Autonomous Loop
= 即使停止主动动作，Body State 自己继续演化出下一轮 Flow
```

---

## 4. 筑基：Stable Self-Sustaining Loop

### 4.1 [固定原则] 筑基的结构定义

筑基表示角色体内第一次形成：

> **闭合、符合世界底层规则、可自维持、长期动态稳定，并且主要由身体自身承担的灵气循环结构。**

筑基不是开放一个 `loop()` API，也不是重复某条路线 N 次后的奖励。

真实因果链应为：

```text
反复安全运行 Sequence
        ↓
Openness / Adaptation / Conductance 等身体状态变化
        ↓
Loss 降低、Flow Closure 提高、反馈关系改变
        ↓
SelfSustain / DynamicStability 等指标逐渐改变
        ↓
结构跨过稳定相变条件
        ↓
SELF_SUSTAINING_LOOP
```

### 4.2 筑基五个核心判据

#### A. Closure

路线首先必须拓扑闭合：

```text
A -> B -> C -> A
```

定义：

```text
TopologyClosure = 1 if path returns to origin/core else 0
FlowClosure = min(CycleOutput / CycleInput, 1)
Closure = TopologyClosure * FlowClosure
```

Closure 高表示一轮运行后，大部分有效流能够重新返回系统，而不是持续外泄。

#### B. RuleBalance

闭环还必须符合底层变化规则，不能只是“几何上首尾相连”。

例如一个回路如果每轮都持续 Amplify 火性，最终一定发散，不能筑基。

v0.1 可定义：

```text
DeltaEnergy = Generated + ExternalInput - Consumed - Diffuse - Loss
EnergyImbalance = abs(DeltaEnergy) / max(TotalActiveEnergy, epsilon)

NatureDrift = sum(abs(NatureRatio(t) - NatureRatio(t-N)))

RuleBalance = clamp(
    1 - 0.6 * EnergyImbalance - 0.4 * NatureDrift,
    0,
    1
)
```

权重 0.6 / 0.4 属于 [v0.1 参数]。

#### C. SelfSustain

筑基最关键的实验：**停止主动推动。**

```text
ControlInput = 0
ExternalPush = 0
```

然后观察循环是否继续。

```text
SelfSustain = AverageFlowAfterRelease / AverageFlowBeforeRelease
```

SelfSustain 越高，表示循环越不依赖主动意识持续推动。

#### D. DynamicStability

系统不能只在某一 Tick 恰好满足条件，而必须在时间窗口内不发散。

观察至少包括：

```text
Flow
Pressure
NatureRatio
Stability
```

v0.1：

```text
DynamicInstability =
    0.25 * FlowVariance
  + 0.25 * PressureVariance
  + 0.25 * NatureVariance
  + 0.25 * StabilityVariance

DynamicStability = clamp(1 - DynamicInstability, 0, 1)
```

这里的 Variance 应使用归一化后的窗口波动值。

#### E. Internalization

借助外部阵法、法器或持续药效形成的循环，不应直接等同筑基。

```text
Internalization =
    InternalContribution
    / (InternalContribution + ExternalContribution)
```

主要贡献应来自：

- 自身穴位；
- 自身经脉；
- 丹田；
- 后天 Adaptation；
- 自身形成的稳定反馈关系。

### 4.3 [v0.1 参数] “触碰筑基”与“真正筑基”

为了让体验连续，应区分两个阶段。

**Foundation Candidate / 触碰筑基：**

```text
Closure >= 0.80
RuleBalance >= 0.80
SelfSustain >= 0.70
DynamicStability >= 0.80
Internalization >= 0.80
MinimumStability >= 0.70
CandidateTicks >= 30
```

此时只能产生感知：

> 停止主动引导以后，气息没有立刻停下，而是自行续行了一段时间。

不能直接宣布筑基。

**Foundation / 真正筑基：**

```text
Closure >= 0.90
RuleBalance >= 0.85
SelfSustain >= 0.85
DynamicStability >= 0.88
Internalization >= 0.90
MinimumStability >= 0.75
StableTicks >= 100
```

满足后 Engine 才允许：

```text
BodyPhase = SELF_SUSTAINING_LOOP
```

所有阈值属于 [v0.1 参数]，但“必须通过长期稳定结构判定，而不是通过次数判定”属于 [固定原则]。

### 4.4 修炼次数的真实作用

“运行了多少周天”不能直接进入境界判定。

重复修炼只能通过改变真实身体状态间接影响筑基：

```text
AdaptationGain[e]
= AdaptationRate
* SafeFlowRatio
* (AdaptationMax - Adaptation[e])
```

```text
OpennessGain
= OpenRate
* Pressure
* (1 - Openness)
```

以及：

```text
ChannelAdaptation ↑
Conductance ↑
TransferLoss ↓
Damage / Recovery 改变
```

最终这些状态变化才使：

```text
Closure ↑
SelfSustain ↑
DynamicStability ↑
```

因此：

> **次数不是门槛，次数只是物理状态演化所经过的时间。**

---

## 5. 金丹：Stable Energy Core

### 5.1 [固定原则] 金丹必须是 Loop 中出现的新稳定结构

筑基后的新境界不能只是“Loop 更快”。

金丹定义为：

> **在已经稳定的基础循环中，形成一个可以长期存在、具有高密度、高相干性、自保持能力的独立能量核心。**

Engine 应真实创建结构：

```text
EnergyCore {
    mass
    effective_volume
    density
    coherence
    stability
    self_retention
    composition[5]
    energy
}
```

其中：

```text
Density = CoreQiAmount / EffectiveVolume
```

### 5.2 核心不是“丹田灵气很多”

必须区分：

```text
大量分散灵气 != EnergyCore
```

只有当局部能量满足：

- 持续聚集；
- 结构相干；
- 停止输入后仍能保持；
- 内部波动不快速崩解；
- 与基础 Loop 保持稳定交换；

才视为真正 Core。

### 5.3 [v0.1 参数] 金丹候选判定

前置条件：

```text
BodyPhase == SELF_SUSTAINING_LOOP
```

候选判据可先使用：

```text
CoreDensity >= DensityCritical        # 示例初值可取 8.0
Coherence >= 0.85
CoreStability >= 0.85
SelfRetention >= 0.90
CoreStableTicks >= 100
```

SelfRetention 可定义为：

```text
SelfRetention = CoreEnergyAfterIsolationWindow / CoreEnergyBeforeIsolation
```

测试时暂时降低或停止外部输入，观察核心是否自行散掉。

满足后：

```text
BodyPhase = STABLE_ENERGY_CORE
```

修真文明称其为“金丹”。

### 5.4 同境界不要求同结构参数

两个角色都可以是金丹，但 Core 完全不同：

```text
角色 A: 木 / 水组成占主导，稳定、恢复倾向明显
角色 B: 火 / 土组成占主导，释放强、压力高
```

境界只说明“存在同类结构”，不说明结构完全同质。

---

## 6. 元婴及更高境界的设计方法

### 6.1 [固定原则] 不按传统名词反推机制

不要先写：

```text
元婴 -> 某技能
化神 -> 某属性
```

再想办法把规则凑进去。

正确顺序必须是：

```text
底层规则产生了哪一种新的稳定结构？
        ↓
这种结构与上一阶段有什么质变？
        ↓
它能否被 Engine 独立检测？
        ↓
世界观再给它命名
```

### 6.2 [待定义] 元婴候选方向

目前只确定一种值得继续研究的方向，但尚未定为正式规则：

> 在 Stable Energy Core 内部形成第二层可长期保持的自组织结构；该结构可能具有自己的内部循环、状态保持能力，甚至成为后续“独立信息/意识载体”的基础。

可以暂时理解为：

```text
Sequence
   ↓
Stable Loop          -> 筑基
   ↓
Stable Core          -> 金丹
   ↓
Second-order Persistent Structure inside Core
                      -> 元婴候选方向
```

但具体需要哪些状态、是否涉及 Information、Memory、Secondary Loop，目前均属于 [待定义]。

### 6.3 更高境界的统一审核问题

任何新境界必须回答：

1. Engine 中新增了什么实体或结构？
2. 它为什么不能在上一境界长期存在？
3. 它的形成条件能否由现有底层规则自然推导？
4. 它是否可以通过数值状态和稳定性测试被确定性检测？
5. 如果去掉“境界名称”，该结构是否仍然有物理意义？

只要其中大部分回答不了，就不应该进入底层实现。

---

# Part B. Sensory Model：数字如何变成“微热、胀痛、刺痛”

## 7. 感知系统的职责边界

### 7.1 [固定原则] LLM 不生成基础感觉

错误链路：

```text
Engine 数值 -> LLM -> “这里感觉微热”
```

正确链路：

```text
World Engine Numeric State
        ↓
Derived Physical Signals
        ↓
Sensory Model
        ↓
Percept Tokens
        ↓
Deterministic Text/UI Renderer
        ↓
玩家 + LLM
```

LLM 只能解释：

> “为什么会热？”

不能决定：

> “这里应该热。”

### 7.2 身体感觉与五行性质必须分离

至少分成两类感知：

**Body Sensation：**

```text
Thermal      热/冷
Fullness     充盈
Distension   胀
Pain         痛
Flow         流动
Obstruction  滞涩
Vibration    震颤
Tension      拉扯/紧绷
```

**Nature Signature：**

```text
Growth       生发
Activation   活跃
Stability    厚重/稳定
Convergence  锐利/收敛
Flowing      流动/适应
```

“温热”不等于“火属性”。

火性可以通过底层转化导致 ThermalLoad 上升，最终表现为热；但热感来自 ThermalLoad，而不是字符串 `element == FIRE`。

---

## 8. Sensory Model 总体流水线

```text
Numeric State
   ↓
Signal Derivation
   - ThermalLoad
   - Pressure / LoadRatio
   - FlowEfficiency
   - Strain
   - Damage
   - FlowShock
   - Nature tendency
   ↓
Temporal Analysis
   - delta
   - trend
   - duration
   - moving window
   ↓
Threshold + Hysteresis
   ↓
PerceptToken
   ↓
Deterministic Renderer
   ↓
Observation Buffer
   ├─ UI
   └─ LLM Cognition
```

UI 与 LLM 应基于同一 Observation Boundary，LLM 默认不获得额外上帝视角。

---

## 9. Thermal：为什么会“微热 / 温热 / 灼热”

### 9.1 [固定原则] 先生成热负荷，再映射感觉

穴位维护：

```text
ThermalLoad
```

其变化来自真实过程：

```text
HeatGenerated =
    TransformLossHeat
  + TransferFrictionHeat
  + ActiveReleaseHeat
  + ExternalHeat
```

```text
ThermalLoad(t+1)
= ThermalLoad(t)
+ HeatGenerated
- HeatDissipation
```

其中五行只通过底层机制影响热生成或散热，例如：

```text
火性高 Activity -> 某些 Release / Transform 更活跃 -> HeatGenerated 可能上升
水性流动增强 -> 某些条件下 HeatDissipation 可能上升
```

但 Sensory Model 不直接检查“火”来输出“热”。

### 9.2 [v0.1 参数] 热感阈值

定义：

```text
ThermalDelta = LocalThermalLoad - BaselineThermalLoad
```

示例映射：

| ThermalDelta | 感知 |
|---:|---|
| < 0.05 | 无明显热感 |
| 0.05 - 0.15 | 微温 |
| 0.15 - 0.30 | 温热 |
| 0.30 - 0.50 | 明显发热 |
| 0.50 - 0.75 | 灼热 |
| > 0.75 | 灼痛风险 |

实际输出还必须结合趋势：

```text
ThermalDelta = 0.25, dThermal/dt ≈ 0
-> 保持稳定温热

ThermalDelta = 0.25, dThermal/dt >> 0
-> 热意正在明显增强
```

---

## 10. Fullness 与 Distension：为什么会“充盈 / 发胀 / 胀痛”

### 10.1 Fullness 表达“这里正在积累东西”

```text
Load = sum(Qi[e])
LoadRatio = Load / Capacity
```

Fullness 可以主要依据 LoadRatio。

示例：

| LoadRatio | 感知 |
|---:|---|
| < 0.20 | 空 / 无明显存在感 |
| 0.20 - 0.40 | 轻微充盈 |
| 0.40 - 0.65 | 明显充盈 |
| 0.65 - 0.85 | 饱满 |
| > 0.85 | 接近承载上限 |

### 10.2 Distension 表达机械压力和扩张感

已经定义：

```text
Pressure = LoadRatio^2
```

为了体现快速上涨比缓慢上涨更容易被察觉：

```text
DistensionSignal
= Pressure
+ K_pressure_delta * max(0, dPressure/dt)
```

`K_pressure_delta` 属于 [v0.1 参数]。

示例映射：

| DistensionSignal | 感知 |
|---:|---|
| < 0.20 | 无明显胀感 |
| 0.20 - 0.40 | 轻微充盈/撑感 |
| 0.40 - 0.60 | 明显发胀 |
| 0.60 - 0.80 | 强烈胀感 |
| 0.80 - 1.00 | 胀痛 |
| > 1.00 | 强烈胀痛 / 失稳风险 |

因此“充盈”和“胀”不能用同一个字段代替。

---

## 11. Pain：为什么是“刺痛 / 胀痛 / 灼痛”

### 11.1 PainSignal 来源

v0.1：

```text
PainSignal =
    0.35 * Strain
  + 0.35 * Damage
  + 0.20 * FlowShock
  + 0.10 * Overload
```

权重属于 [v0.1 参数]。

### 11.2 痛感类型由主要成因决定

不要只输出 `pain = 0.7`。

可以根据 dominant cause 分类：

```text
Pressure / Overload 主导 -> 胀痛
FlowShock 主导          -> 刺痛
ThermalLoad 主导        -> 灼痛
Damage 长时间主导       -> 隐痛 / 持续痛
Tension 主导            -> 拉扯痛 / 紧绷痛
```

这使玩家可以通过感觉反推系统发生了什么。

---

## 12. Flow / Obstruction：为什么会“顺畅 / 滞涩”

定义：

```text
FlowEfficiency = ActualFlow / max(RequestedFlow, epsilon)
```

再结合有效传导和流量冲击：

```text
Smoothness = clamp(
    FlowEfficiency
    * ConductanceNormalized
    * (1 - FlowShock),
    0,
    1
)
```

示例映射：

| Smoothness | 感知 |
|---:|---|
| > 0.85 | 极为顺畅 |
| 0.65 - 0.85 | 顺畅 |
| 0.45 - 0.65 | 略有阻滞 |
| 0.25 - 0.45 | 明显滞涩 |
| < 0.25 | 难以推动 |

也可以定义：

```text
Obstruction = 1 - Smoothness
```

---

## 13. Nature Signature：如何感知“生发 / 活跃”，但不知道这是木火

五行知识应分成：

```text
World Truth       -> Nature[5]
Sensory Signature -> 生发 / 活跃 / 厚重 / 锐利 / 流动
Cognitive Label   -> 木 / 火 / 土 / 金 / 水
```

早期角色可能得到：

> 一股柔和、生发的气息。

而不是：

> 木属性 73%。

只有当角色已有足够知识后，LLM/Knowledge System 才能解释：

> 这种“生发”特征很可能对应木性。

因此：

> **Nature Signature 是感知，五行命名是知识。**

---

## 14. 时间维度：感觉必须包含 Trend 与 Duration

Sensory Model 不应只看瞬时值。

每个信号至少维护：

```text
value
previous_value
delta
trend
duration
window_mean
window_variance
```

例如同样 Thermal = 0.28：

```text
过去 10 Tick 一直约 0.28
-> 保持稳定温热
```

```text
0.10 -> 0.15 -> 0.21 -> 0.28
-> 热意正在持续增强
```

这种差异对于 LLM 判断“继续 / 减速 / 停止”非常重要。

---

## 15. Hysteresis：避免感知在阈值附近抖动

不能简单写：

```text
0.149 -> 微温
0.151 -> 温热
0.148 -> 微温
```

否则 UI 和 Observation 会频繁跳变。

应使用状态滞回。例如：

```text
进入 WARM：ThermalDelta >= 0.17
退出 WARM：ThermalDelta <= 0.12
```

每个连续感知状态都应考虑类似机制。

这是 [固定实现原则]，具体上下阈值属于 [v0.1 参数]。

---

## 16. Pattern Resolver：确定性识别复合感觉

在单通道感觉之上，可以增加不依赖 LLM 的 Pattern Resolver。

示例：

```text
Pressure ↑ + Flow ↓
-> BACKPRESSURE
-> “灵气仍在积累，但继续推动越来越困难。”
```

```text
FlowShock ↑ + SharpPain ↑
-> FLOW_IMPACT
-> “气息突然冲击经路，引发短促刺痛。”
```

```text
Thermal ↑ + NatureActivation ↑ + Stability ↓
-> RUNAWAY_ACTIVATION
-> “局部热意持续增强，波动也开始变得不稳定。”
```

Pattern Resolver 仍属于确定性规则层，不调用 LLM。

---

## 17. PerceptToken 数据结构

建议不要让 Sensory Model 直接生成自然语言，而是先生成结构化 Token：

```json
{
  "location": "qingguan",
  "signals": {
    "fullness": {
      "level": "moderate",
      "trend": "rising"
    },
    "distension": {
      "level": "mild",
      "trend": "rising"
    },
    "thermal": {
      "level": "warm",
      "trend": "stable"
    },
    "flow": {
      "level": "smooth",
      "trend": "stable"
    },
    "pain": {
      "level": "none"
    }
  },
  "nature_signature": {
    "growth": "strong",
    "activation": "moderate"
  },
  "patterns": []
}
```

然后由确定性模板层生成：

> 青关有明显充盈感，局部保持温热，气息流动总体顺畅，没有明显疼痛。

---

## 18. 确定性文本模板

第一版完全不需要为了“文采”调用 LLM。

示例模板：

```text
thermal.mild        -> “微微发热”
thermal.warm        -> “有明显温热感”
thermal.hot         -> “局部灼热”

fullness.mild       -> “有轻微充盈感”
distension.moderate -> “出现明显胀感”

pain.sharp          -> “传来短促刺痛”
pain.burning        -> “出现灼痛”

flow.smooth         -> “气息流动顺畅”
flow.blocked        -> “气息明显滞涩”
```

组合时遵循固定优先级：

```text
位置
-> 主要身体感觉
-> 流动情况
-> 疼痛/危险
-> 趋势
```

避免同一 Tick 输出十几条碎片感知。

---

## 19. LLM 最终应该收到什么

LLM 不接收：

```text
WoodQi = 8.72
Pressure = 0.641
FireActivity = 0.82
```

正常情况下接收：

```text
[身体感知]
位置：青关
- 有明显充盈感，仍在缓慢增强
- 局部保持微温
- 气息流动总体顺畅
- 没有明显疼痛

[性质感知]
- 生发感明显
- 活跃感略有提高

[相比上一周天]
- 充盈感上升
- 热感基本不变
- 流动没有明显恶化
```

LLM 的职责才从这里开始：

```text
Interpretation
Hypothesis
Experiment Design
Next Intent
```

例如：

> 当前压力虽然增加，但没有明显失稳迹象，可以保持流量再运行一轮，并观察充盈趋势是否继续。

这是认知判断，不是 World Truth。

---

# Part C. 两套系统如何连接

## 20. 感知是境界变化的“玩家可见面”

境界公式完全在 Engine 内计算：

```text
Closure
RuleBalance
SelfSustain
DynamicStability
Internalization
```

玩家和 LLM 不应该直接看到这些精确值作为主要游戏界面。

相反，结构变化通过 Sensory Model 表现出来。

例如 SelfSustain 逐渐提高：

```text
低：
“你一松开心念，气息便迅速衰减。”

中：
“你停止推动以后，气息仍自行向前续行了一小段。”

高：
“即使你不再主动推动，周天仍保持原有节奏继续运行。”
```

这三句话必须由 SelfSustain / Flow Decay 等确定性条件映射，而不是 LLM 自己决定。

### 20.1 筑基体验的正确因果

```text
Engine 数值真正满足相变条件
        ↓
BodyPhase 发生变化
        ↓
Sensory Model 检测到新的持续模式
        ↓
玩家感知：松手后周天仍自行运行
        ↓
LLM 理解：我们可能已经形成真正的自循环
        ↓
世界观层称其为“筑基”
```

不能反过来让 LLM 宣布“我筑基了”，再修改 Engine 状态。

---

## 21. 开发模块建议

建议拆成以下模块：

```text
world-engine/
  body_graph
  essence_flow
  transform
  stability
  damage
  adaptation

realm-resolver/
  closure
  rule_balance
  self_sustain
  dynamic_stability
  internalization
  core_detector

sensory-model/
  signal_derivation
  temporal_analysis
  threshold_hysteresis
  pattern_resolver
  nature_signature

perception/
  percept_token
  visibility_filter
  familiarity_filter
  deterministic_renderer

agent-runtime/
  observation_buffer
  knowledge
  protocol_runtime
  novelty_detector
  llm_escalation
```

Realm Resolver 与 Sensory Model 都不能依赖 LLM。

---

## 22. 必须有的测试

### 22.1 Realm Tests

至少覆盖：

1. Sequence 重复 10000 次但 SelfSustain 始终低，不得筑基。
2. 某 Tick 瞬时所有指标过线，但随后迅速发散，不得筑基。
3. 外部阵法维持的稳定 Loop，Internalization 不足，不得筑基。
4. 停止主动控制后长期稳定，且五项指标满足条件，才能进入 Foundation。
5. 灵气很多但没有高相干稳定 Core，不得进入金丹。
6. Core 停止输入后快速散掉，不得进入金丹。

### 22.2 Sensory Tests

至少覆盖：

1. 相同 Pressure，不同 dPressure/dt，应产生不同胀感趋势。
2. Thermal 在阈值附近小幅波动时，不应“微温/温热”反复跳变。
3. 高 FlowShock + 低 Damage，应偏向“刺痛”而不是“持续隐痛”。
4. 高 LoadRatio + 低 Pain，应先表现“充盈/胀”，不能直接“疼痛”。
5. 相同五行性质但不同 ThermalLoad，不应都输出同样“热”。
6. Nature Signature 可输出“生发”，但在角色未掌握五行知识时不能直接输出“木”。
7. 玩家 UI 与 LLM Observation 对同一事实应保持一致的信息边界。

---

## 23. 最终不可违反的规则

1. **境界由结构相变决定，不由经验或次数决定。**
2. **次数只能通过改变身体真实状态，间接影响境界条件。**
3. **筑基 = Stable Self-Sustaining Loop。**
4. **金丹 = 在 Stable Loop 中产生 Stable Energy Core。**
5. **更高境界必须首先定义新的 Engine 稳定结构，再赋予修真名称。**
6. **LLM 不参与境界判定。**
7. **LLM 不决定角色是否感觉热、胀、痛或顺畅。**
8. **所有基础感觉由 Numeric State -> Sensory Model -> PerceptToken 确定性产生。**
9. **身体感觉与五行知识分离：热不是火，生发也不等于角色已经知道“木”。**
10. **LLM 从 Observation 开始负责理解、假设、实验设计与下一步 Intent。**
11. **已知过程交给 Runtime；未知、异常、创造再唤醒 LLM。**
12. **Engine 生产因果，Sensory Model 生产感觉，LLM 生产解释与知识。**
