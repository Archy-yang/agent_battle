# 修炼引擎：Sequence → Autonomous Loop → Foundation 设计规范 v0.2

> 文档定位：引擎级正式设计规范 / v0.1 的完整替代版本  
> 适用范围：`backend/cult`、`backend/ctmatch`，以及后续所有依赖能量流、反馈、自组织结构的玩法系统  
> 核心目标：让“筑基”成为底层动力学自然产生的稳定自主结构，而不是次数、经验条、脚本自动化或境界后注册的功能。

---

## 0. v0.2 版本结论

v0.2 在 v0.1 的核心骨架上继续下沉，并正式固定以下结论：

1. **先有 Autonomous Loop，后有 Foundation。**
2. **SelfSustain 测的是“脱离意识控制后还能维持多少”，不是“能量是否永远不衰减”。**
3. **世界中 Flow 的来源必须至少拆成 Passive / Autonomous / Manual 三类驱动。**
4. **FeedbackCoupling 是控制关系，不是能量来源。它只能决定“何时、向哪里响应”，不能凭空产生 Work。**
5. **环境吸收不做成修炼专用 `PassiveIntake`，升级为通用 `BoundaryExchange / EnvironmentalFlux`。**
6. **StorageCapacity、Throughput、Conductance、DriveCapacity 是四个不同维度，不得继续混用。**
7. **驱动不是免费的：任何主动或自主 Drive 都必须消耗可用能量；只有超过安全区或剧烈突变时才额外产生明显 Strain。**
8. **FeedbackCoupling 的形成不按次数增长，也不采用“ManualDrive 越大成长越快”的简单乘法。采用 Teacher / Error Learning。**
9. **Engine 只允许 Local TransitionCoupling，不允许 PathCoupling。路径、周天、功法只能存在于 Knowledge / Protocol / Blueprint 层。**
10. **FoundationEvaluator 永远只是观察者，不能反向注入动力、注册 Loop 或改变世界物理。**

---

## 1. 背景与当前矛盾

当前修炼引擎已经具备：

- 穴位 / 经脉图；
- 灵气数量与五行性质；
- Flow / Throughput / Conductance；
- Pressure / Thermal / Strain / Stability / Damage；
- Adaptation / Openness / ChannelAdaptation；
- Perception / Sensory Model；
- Protocol Runtime；
- Foundation 五量评估。

但如果 Flow 的唯一来源仍然是显式 `Ops.Request / guide()`，则会产生逻辑死锁：

```text
Foundation 需要 SelfSustain 足够高
        ↓
SelfSustain 要求停止 Manual Control 后仍然有 Flow
        ↓
当前没有 Request 就没有 Flow
        ↓
Release Test 必然得到接近 0
        ↓
真实世界永远无法自然筑基
```

如果用：

```text
FoundationEstablished
→ RegisterLoop(protocol)
→ 自动运行
```

来解决，则因果顺序被颠倒。

正确顺序必须是：

```text
长期稳定训练
→ 局部反馈关系形成
→ 多个局部反馈组合并闭合
→ ManualDrive = 0 后仍能运行
→ Engine 观察到稳定自主结构
→ FoundationEvaluator 判定达标
→ 上层文明把这个状态称为“筑基”
```

---

## 2. 顶层设计原则

### 2.1 LLM 决定意图，Engine 决定现实

LLM / 玩家可以决定：

- 想尝试哪种路径；
- 想施加什么方向、什么强度的 ManualDrive；
- 想维持多久；
- 是否停止控制；
- 根据感知提出什么假设和实验。

LLM 不得决定：

- 某个 Coupling 是否形成；
- AutonomousDrive 是否存在；
- SelfSustain 数值；
- 是否筑基；
- 能量是否守恒；
- Loop 是否继续运行。

这些全部由 Engine 根据当前 World State 派生。

### 2.2 自维持不等于永动

`Self-Sustaining` 的正式语义是：

> 在没有持续意识控制的情况下，结构仍能产生足够的内部反馈，使有组织的 Flow 延续。

它并不意味着：

- 不消耗 Qi；
- 不需要能源；
- 不发生损耗；
- 可以无限运行。

自主 Loop 可以依靠：

- 身体已有储备；
- 环境交换输入；
- 内部 Transform 释放的可用能量；

同时受到：

- Diffusion；
- TransferLoss；
- DriveWork；
- ThermalLoss；
- Damage；

等约束。

### 2.3 新增的是通用 Primitive，不是 LoopSystem

禁止新增：

```text
Technique.IsLoop
Acupoint.AutoRun
FoundationEstablished → RegisterLoop
PathCoupling("丹田→青关→阳枢→土府→丹田")
```

应新增或正式化的是：

- `BoundaryExchange`
- `Potential`
- `PassiveDrive`
- `DriveCapacity`
- `FeedbackCoupling`
- `AutonomousCommand`
- `DriveResolver`
- `AutonomousDrive`
- `WorkLedger`

这些 Primitive 必须可被身体、阵法、法器、丹炉、建筑等共同复用。

---

## 3. Sequence、Autonomous Sequence 与 Autonomous Loop

### 3.1 Sequence

炼气早期：

```text
丹田
  ↓ ManualDrive
青关
  ↓ ManualDrive
阳枢
  ↓ ManualDrive
土府
  ↓ ManualDrive
丹田
```

每一段都依赖：

```text
Intent / Protocol
      ↓
ManualCommand
      ↓
DriveResolver
      ↓
ManualDrive
      ↓
Flow
```

停止控制后，如果内部不存在其他 Drive，Flow 会自然衰减或停止。

### 3.2 Autonomous Sequence

长期稳定训练后，局部输入输出关系可能形成反馈：

```text
IncomingFlow(A → B)
      ↓
B 局部状态变化
      ↓
FeedbackCoupling(B)
      ↓
AutonomousCommand(B → C)
      ↓
DriveResolver
      ↓
AutonomousDrive(B → C)
```

当只有部分 Transition 已经内化时，这是 Autonomous Sequence，不是 Loop。

### 3.3 Autonomous Loop

只有多个 Local TransitionCoupling 在运行时形成闭合反馈图：

```text
      ┌──────────────────┐
      ↓                  │
丹田 → 青关 → 阳枢 → 土府
  ↑                      │
  └──────────────────────┘
```

并满足：

```text
ManualDrive = 0
```

后仍可持续运行，才是 Autonomous Loop。

### 3.4 Foundation

Engine 只识别：

```text
StableAutonomousStructure exists
```

“筑基”只是上层文明对满足指定稳定条件的 Autonomous Loop 的命名。

> **先有 Loop，后有筑基。**

---

## 4. 基础物理量必须拆分

以下四个维度必须独立存在，不得继续混用：

### 4.1 StorageCapacity

```text
StorageCapacity
= 当前 Carrier / Node 能稳定容纳多少 Qi / Energy
```

它只描述“能装多少”。

### 4.2 Throughput

```text
Throughput
= 单位时间允许安全通过多少 Flow
```

它只描述“能通过多快”。

### 4.3 Conductance

```text
Conductance
= 存在势差时，这条连接有多容易产生 Flow
```

它描述阻力 / 传导性。

### 4.4 DriveCapacity

```text
DriveCapacity
= 该节点 / 结构能够主动对流动施加多少驱动力
```

它描述“能够做多少主动功”。

四者举例：

| 结构 | StorageCapacity | Throughput | Conductance | DriveCapacity |
|---|---:|---:|---:|---:|
| 丹田 | 高 | 中高 | 由连接决定 | 高 |
| 普通穴位 | 低~中 | 低~中 | 由亲和与通道决定 | 低 |
| 灵石 | 高 | 中 | 可高 | 接近 0 |
| 阵法核心 | 中 | 高 | 高 | 高 |
| 丹炉泵结构 | 低 | 高 | 高 | 很高 |

因此不得再使用：

```text
Capacity 最大节点 = 唯一主动核心
```

逆势输运是否可能，应该由 `DriveCapacity + AvailableEnergy + DriveEfficiency` 决定。

---

## 5. BoundaryExchange：环境与 Carrier 的统一交换规则

v0.1 中缺失的 `PassiveIntake` 在 v0.2 不作为修炼专用能力存在，而升级为通用边界交换：

```text
BoundaryExchange / EnvironmentalFlux
```

### 5.1 基本公式

对节点 `i`、性质 `e`：

```text
ExchangeFlow[i,e]
=
Permeability[i,e]
× EffectiveOpenness[i]
× Affinity[i,e]
× (EnvironmentPotential[e] - InternalPotential[i,e])
× ActivityFactor[e]
```

允许正负双向：

```text
ExchangeFlow > 0  → 环境进入 Carrier
ExchangeFlow < 0  → Carrier 向环境泄出
```

### 5.2 与主动吐纳的关系

主动 `absorb()` 不应创建另一套世界物理。

它只是通过 ManualDrive / Control 改变：

- `Permeability`
- `EffectiveOpenness`
- 边界势差
- 局部 Drive

从而放大本来就存在的 BoundaryExchange。

### 5.3 统一的好处

同一规则未来可表达：

- 人体自然吐纳；
- 灵石缓慢吸收环境灵气；
- 聚灵结构改变局部环境势；
- 材料散热；
- 丹炉与外界交换；
- 密闭容器抑制泄露。

因此不要永久保留一个只属于人体的 `PassiveIntake`。

---

## 6. Potential 与 PassiveDrive

### 6.1 NodePotential

v0.2 最小实现可以：

```text
Potential[i] = PressurePotential[i]
```

以后扩展为：

```text
Potential[i]
=
PressurePotential
+ ActivityPotential
+ NaturePotential
+ ExternalFieldPotential
```

### 6.2 PassiveDrive

相连节点之间的自然驱动：

```text
PassiveDrive(i → j)
=
Conductance(i,j)
× max(0, Potential[i] - Potential[j])
```

对于可双向 Channel，可分别计算两个方向。

### 6.3 PassiveDrive 的意义

即使：

```text
ManualDrive = 0
AutonomousDrive = 0
```

世界也允许因为势差自然发生变化。

这使 Engine 从“命令驱动传输器”升级为真正的动力学系统。

---

## 7. FeedbackCoupling：控制关系，而不是能量来源

这是 v0.2 最重要的强约束之一。

### 7.1 正式定义

`FeedbackCoupling` 表示：

> 当某个局部输入模式出现时，结构已经学会产生某个后续响应命令。

它是 `Sensor → Control Mapping`，不是能源。

### 7.2 MUST NOT：Coupling 不得直接产生 Work

禁止：

```text
IncomingFlow
→ Coupling
→ 直接增加 Energy / Qi / Work
```

正确流程必须是：

```text
IncomingFlow / LocalState
        ↓
FeedbackCoupling
        ↓
AutonomousCommand
        ↓
DriveResolver
        ↓
检查 DriveCapacity
检查 AvailableEnergy
计算 DriveWork
产生 Heat / Strain
        ↓
ActualAutonomousDrive
```

因此：

```text
AutonomousCommand ≠ AutonomousDrive
```

Feedback 只决定“响应意图”，真正 Drive 必须经过物理约束。

### 7.3 Local TransitionCoupling

建议数据语义：

```text
FeedbackCoupling {
    input_transition
    output_transition
    strength
    latency
    tolerance
    learned_response
}
```

例如：

```text
Input:
    丹田 → 青关的稳定来流

Output:
    青关 → 阳枢的驱动响应
```

这是局部的：

```text
IncomingTransition → OutgoingTransition
```

而不是整条功法。

---

## 8. 为什么禁止 PathCoupling

不允许 Engine 存在：

```text
PathCoupling {
    丹田 → 青关 → 阳枢 → 土府 → 丹田
}
```

原因是它重新把：

```text
周天 / 功法 / 路线
```

这些上层概念写回底层 Engine。

正确层级：

```text
World Engine
    Local TransitionCoupling
            ↓
多个局部 Coupling 组合
            ↓
Emergent Loop
            ↓
Knowledge / Protocol 识别为 Path
            ↓
玩家 / LLM 命名为某某功法
```

Path 可以存在于：

- Knowledge；
- Technique Blueprint；
- Protocol；
- UI；

但不能成为 World Physics 的基础对象。

### 8.1 “多段训练会不会太慢”不是架构理由

多个 Transition 可以在同一个连续流动过程中并行学习。

例如：

```text
Tick 1~10   A → B 学习
Tick 8~20   B → C 学习
Tick 17~30  C → D 学习
```

时间尺度通过 `AdaptRate`、安全窗与仿真调参解决，不通过 PathCoupling 绕过底层抽象。

---

## 9. 三类驱动与统一 DriveResolver

正式定义：

```text
TotalDrive
=
PassiveDrive
+ AutonomousDrive
+ ManualDrive
```

### 9.1 ManualDrive

来源：

- 玩家 Intent；
- LLM Action；
- 炼气期 Protocol Runtime 的主动操作。

### 9.2 PassiveDrive

来源：

```text
Potential Gradient
```

无意识参与。

### 9.3 AutonomousDrive

来源：

```text
FeedbackCoupling
→ AutonomousCommand
→ DriveResolver
```

它依赖已经形成的内部反馈，但仍受能量和结构能力限制。

### 9.4 统一 DriveResolver

所有主动驱动最终必须进入同一套约束：

```text
ResolveDrive(
    command,
    DriveCapacity,
    AvailableEnergy,
    DriveEfficiency,
    CurrentState
)
```

输出：

```text
ActualDrive
DriveWork
WasteHeat
DriveStrain
```

Manual 与 Autonomous 不得走两套不同的“魔法通道”。

---

## 10. 驱动的能量代价与 Strain

### 10.1 Drive 不是免费的

定义：

```text
DriveRatio
=
ActualDrive / max(EffectiveDriveCapacity, ε)
```

驱动消耗：

```text
DriveWork
=
k_work
× ActualDrive²
× Δt
/ max(DriveEfficiency, ε)
```

`DriveWork` 必须从真实可用能源中扣除。

例如：

- QiReserve；
- StoredEnergy；
- Transform 释放的可用能量。

### 10.2 正常驱动不等于持续受伤

只有超出安全区或变化过快才产生明显额外 Strain：

```text
OverDrive
=
max(0, DriveRatio - SafeDriveRatio)
```

```text
DriveStrain
=
k_over × OverDrive²
+ k_ramp × DriveChange²
```

因此：

```text
温和稳定驱动
→ 消耗能量
→ Strain 很低
```

```text
过强 / 突变驱动
→ DriveWork ↑
→ WasteHeat ↑
→ DriveStrain ↑
→ Stability ↓
```

这可以自然产生“火候”与“用力过猛”的体验，而不需要独立的火候玩法规则。

---

## 11. Flow Solver

Drive 决定“为什么流动”，Flow Solver 决定“实际能流多少”。

最小实现：

```text
RequestedFlow
=
f(TotalDrive)
```

v0.2 可先取近似：

```text
RequestedFlow ≈ TotalDrive
```

实际流量：

```text
ActualFlow
=
min(
    RequestedFlow,
    SourceQi,
    EffectiveChannelCapacity
)
```

其中继续保留已有：

- Conductance；
- Throughput；
- Damage；
- Affinity；
- TransferEfficiency；
- TransferLoss；
- FlowShock。

Potential / Feedback / Drive 不替换传输模型，只重新定义 Flow 的驱动力来源。

---

## 12. FeedbackCoupling 如何形成：Teacher / Error Learning

### 12.1 禁止次数成长

禁止：

```text
if CycleFinished:
    Coupling += 0.01
```

也禁止：

```text
ManualDrive 越大
→ Coupling 增长越快
```

Coupling 必须学习的是：

> 在某种输入状态出现时，什么样的输出响应能够稳定复现当前受控行为。

### 12.2 输入信号

```text
InputSignal
=
clamp(
    IncomingFlow / max(Throughput, ε),
    0,
    1
)
```

可以对 InputSignal 使用 EMA 降噪。

### 12.3 TeacherGate

ManualDrive 的作用是告诉系统：

> 当前输出是一次“受控教学样本”。

定义：

```text
TeacherGate
=
saturate(
    ManualDrive / TeacherDriveReference
)
```

要求：

- ManualDrive 太低：没有足够清晰的教学信号；
- 达到正常教学强度：TeacherGate 接近 1；
- 继续猛推：TeacherGate 不再增加。

因此不会出现“用力十倍、内化十倍快”。

### 12.4 TargetResponse

身体要学习的不是“Coupling 数值”，而是稳定输入输出关系。

```text
TargetResponse
=
StableOutgoingFlow
/
max(IncomingFlow, ε)
```

### 12.5 当前预测

```text
PredictedResponse
=
CouplingStrength
× InputSignal
```

### 12.6 学习误差

```text
Error
=
TargetResponse
- PredictedResponse
```

### 12.7 SafetyGate

```text
SafetyGate
=
Stability
× Compatibility
× (1 - Damage)
× (1 - OverStrain)
```

所有量需 clamp 到合法范围。

### 12.8 Coupling 更新

```text
ΔCoupling
=
AdaptRate
× TeacherGate
× InputSignal
× SafetyGate
× Error
```

然后：

```text
Coupling(t+1)
=
clamp(
    Coupling(t) + ΔCoupling,
    CouplingMin,
    CouplingMax
)
```

### 12.9 该公式的意义

早期：

```text
Coupling 很低
→ Error 大
→ 学习较快
```

中期：

```text
自主响应开始接近目标
→ Error 下降
→ 学习自然变慢
```

后期：

```text
PredictedResponse ≈ TargetResponse
→ Error ≈ 0
→ Coupling 稳定
```

猛推不会增加 TeacherGate，反而可能通过：

```text
Strain ↑
Stability ↓
SafetyGate ↓
```

降低学习效率。

> 稳定、准确、可重复的控制，比暴力冲击更容易被身体内化。

---

## 13. AutonomousCommand 与 AutonomousDrive

### 13.1 反馈首先产生 Command

```text
AutonomousCommand
=
f(
    InputSignal,
    CouplingStrength,
    LocalState,
    LearnedResponse,
    Tolerance
)
```

最小实现可近似：

```text
AutonomousCommand
=
CouplingStrength
× InputSignal
× LearnedResponse
```

### 13.2 Command 必须经过 DriveResolver

```text
ActualAutonomousDrive
=
ResolveDrive(
    AutonomousCommand,
    DriveCapacity,
    AvailableEnergy,
    DriveEfficiency
)
```

所以：

```text
CouplingStrength = 1
```

并不意味着一定能输出足够 Drive。

如果：

- 能量不足；
- DriveCapacity 太低；
- 结构损伤；
- 当前状态超出 tolerance；

则实际 AutonomousDrive 仍然会下降或失败。

这确保 FeedbackCoupling 不能变成“隐藏魔法能量”。

---

## 14. 能量来源与环境条件

自主 Loop 可以依赖多种能源组合。

### 14.1 内部储备

```text
QiReserve ↓
```

Loop 可以在没有外部输入时短期维持，直至储备耗尽。

### 14.2 BoundaryExchange

当环境势高于内部势时：

```text
Environment
→ BoundaryExchange
→ Body / Carrier
```

可补偿 LoopLoss。

### 14.3 内部 Transform

五行 / 性质变化可能释放可用能量或提高某段 DriveEfficiency。

### 14.4 长期维持条件

长期稳定并不要求：

```text
SelfSustain = 1
```

而要求整体能量账目合理：

```text
ExternalInput
+ GeneratedUsableEnergy
≈
DriveWork
+ Diffuse
+ TransferLoss
+ OtherLoss
```

环境变化因此会自然影响同一门功法：

```text
洞天福地 → 可能长期稳定
普通环境 → 缓慢衰减
绝灵区域 → 最终停止
```

不需要额外做“地图修炼效率 +X%”。

---

## 15. WorkLedger 与 Internalization

每 Tick 记录：

```text
PassiveWork
AutonomousWork
ManualWork
```

注意：`PassiveWork` 代表势差自然释放的有效做功，不是“凭空能量”。

Internalization 正式定义：

```text
Internalization
=
AutonomousWork
/
max(
    AutonomousWork + ManualWork,
    ε
)
```

解释：

- 0：完全依赖意识；
- 0.5：主动与自主各占一半；
- 1：该结构无需持续外部控制。

`PassiveWork` 不计入 Internalization 分母，因为它描述世界自然势差，不代表身体是否学会了反馈控制。

---

## 16. SelfSustain：必须来自真实 Release Test

SelfSustain 的正式语义建议明确为：

> **Control Self-Sustain：停止 Manual Control 后，原有组织化 Flow 能维持多少。**

它不负责判断能量是否长期平衡。

### 16.1 Baseline Window

在稳定主动运行状态下采样：

```text
AverageOrganizedFlowBeforeRelease
```

推荐窗口：例如 50 Tick，具体通过仿真调参。

### 16.2 Release

设置：

```text
ManualDrive = 0
```

不得：

- 清空 Flow；
- 停止 Engine；
- 关闭 PassiveDrive；
- 关闭 AutonomousDrive；
- 直接停止所有 Protocol 物理状态。

### 16.3 Release Window

继续正常模拟 N Tick：

```text
AverageOrganizedFlowAfterRelease
```

### 16.4 计算

```text
SelfSustain
=
AverageOrganizedFlowAfterRelease
/
max(AverageOrganizedFlowBeforeRelease, ε)
```

例如：

```text
Before = 1.80
After  = 1.58
SelfSustain = 0.878
```

这才是真实的“放手后还能维持多少”。

---

## 17. Closure

### 17.1 TopologyClosure

仅表示当前运行路径在图上形成闭合：

```text
TopologyClosure ∈ {0,1}
```

### 17.2 FlowClosure

```text
FlowClosure
=
ReturningFlow
/
max(InitialOutgoingFlow, ε)
```

### 17.3 Closure

```text
Closure
=
TopologyClosure
× clamp(FlowClosure, 0, 1)
```

因此：

```text
图上闭合，但只回来 20%
```

不能视为高质量闭环。

---

## 18. DynamicStability

高 SelfSustain 不等于稳定。

例如：

```text
1.8 → 2.7 → 0.5 → 3.1 → 0.2
```

平均流量可能很高，但属于强烈振荡。

继续使用：

```text
DynamicInstability
=
0.25 × (
    FlowVar
  + PressureVar
  + NatureVar
  + StabilityVar
)
```

```text
DynamicStability
=
clamp(1 - DynamicInstability, 0, 1)
```

因此可以自然出现：

> 气确实自己动起来了，但结构正在失控。

这不是筑基，而可能是失稳 / 走火趋势。

---

## 19. RuleBalance 与能量平衡

SelfSustain 只回答：

> 是否不依赖意识控制。

RuleBalance 回答：

> 这个结构是否在规则上可持续、是否正在快速耗干自己、性质是否持续漂移。

定义：

```text
DeltaEnergy
=
Generated
+ ExternalInput
- Consumed
- DriveWork
- Diffuse
- TransferLoss
- OtherLoss
```

```text
EnergyImbalance
=
abs(DeltaEnergy)
/
max(TotalActiveEnergy, ε)
```

NatureDrift：

```text
NatureDrift
=
Σ abs(
    NatureRatio(t)
  - NatureRatio(t-N)
)
```

RuleBalance：

```text
RuleBalance
=
clamp(
    1
    - 0.6 × EnergyImbalance
    - 0.4 × NatureDrift,
    0,
    1
)
```

可以出现：

```text
SelfSustain = 0.92
RuleBalance = 0.40
```

语义是：

> 它确实会自己运行，但正在快速耗干或持续失衡。

不能筑基。

---

## 20. Foundation 判定

筑基由以下派生指标共同决定：

```text
Closure
RuleBalance
SelfSustain
DynamicStability
Internalization
MinimumStability
StableTicks
```

v0.2 候选阈值继续沿用 v0.1 作为校准起点：

```text
Closure >= 0.90
RuleBalance >= 0.85
SelfSustain >= 0.85
DynamicStability >= 0.88
Internalization >= 0.90
MinimumStability >= 0.75
StableTicks >= 100
```

这些值是：

> **调参起点，不是世界哲学常数。**

最终需要通过大量 Simulation / Monte Carlo 校准。

### 20.1 StableTicks 的语义

`StableTicks` 只用于证明：

> 当前指标不是一个瞬时碰巧达标的状态。

它不是：

```text
修炼经验
境界进度
次数门槛
```

### 20.2 正式判定

```text
if StableAutonomousStructure satisfies FoundationCriteria:
    BodyPhase = FOUNDATION
```

禁止：

```text
cycle_count >= N → FOUNDATION
progress >= 100 → FOUNDATION
RegisterLoop() → FOUNDATION
```

---

## 21. Protocol 在筑基前后的角色

### 21.1 炼气期

Protocol 是主动控制程序：

```text
guide 丹田 → 青关
guide 青关 → 阳枢
guide 阳枢 → 土府
guide 土府 → 丹田
```

Runtime 是操作者。

### 21.2 内化阶段

部分 TransitionCoupling 形成后：

```text
某些步骤 → AutonomousDrive
其余步骤 → ManualDrive
```

Runtime 可以：

- 补充尚未内化的 ManualDrive；
- 监控 Guard；
- 降低强度；
- 触发 Release Test；
- 异常时暂停。

### 21.3 筑基后

Protocol 不再是每 Tick 驱动身体的脚本。

它变成：

> 对一个已经存在的自主结构的 Blueprint / 运行边界 / 监控策略。

Runtime 负责：

- 启停；
- 安全监控；
- 环境变化下的策略调整；
- 异常升级；
- 越出 Knowledge Boundary 时唤醒 LLM。

禁止：

```text
FoundationEstablished → RegisterLoop(protocol)
```

---

## 22. Perception 与玩家体验

底层不得直接暴露：

```text
Coupling = 0.74
AutonomousDrive = 1.23
DriveCapacity = 2.1
SelfSustain = 0.81
```

Sensory Model 应映射为：

```text
“进入青关后，气息比过去更容易继续向前。”

“这一段似乎已经不再需要你持续牵引。”

“你松开心念之后，它仍自行运行了片刻。”

“流动虽然还在继续，但丹田里的储备正在明显下降。”

“这一轮几乎不需用力，但局部热意反而在累积。”
```

LLM 看到的是这些 Perception，然后进行：

```text
Interpretation
→ Hypothesis
→ Experiment
```

例如：

> 某些局部路径似乎已经形成自发反馈。下一轮可以降低主动引导，在阳枢后完全放手观察。

LLM 不能直接读取 Coupling 或宣布“已经筑基”。

---

## 23. Engine Tick v0.2

建议 Tick 演化为：

```text
1. 更新 Environment

2. 计算 EnvironmentPotential / Boundary 条件

3. 执行 BoundaryExchange
   - Environment ↔ Carrier

4. 计算各节点 NodePotential

5. 根据 Potential Gradient 计算 PassiveDrive

6. 根据 PreviousFlow / LocalState / FeedbackCoupling
   生成 AutonomousCommand

7. 从玩家 / LLM / Protocol Runtime 获取 ManualCommand

8. 分别进入 DriveResolver
   - 检查 DriveCapacity
   - 检查 AvailableEnergy
   - 计算 DriveWork
   - 计算 WasteHeat
   - 计算 DriveStrain

9. 得到：
   PassiveDrive
   AutonomousDrive
   ManualDrive

10. 合并：
    TotalDrive = Passive + Autonomous + Manual

11. Flow Solver

12. Transfer

13. Transform / Sheng / Ke / Diffuse / Bind / Release

14. 更新：
    Pressure
    Thermal
    Strain
    Stability
    Damage

15. 根据真实输入/输出样本更新 Local TransitionCoupling
    - TeacherGate
    - Error
    - SafetyGate

16. 更新 Adaptation / Openness / ChannelAdaptation

17. 记录 WorkLedger
    - PassiveWork
    - AutonomousWork
    - ManualWork
    - DriveWork
    - ExternalInput
    - Loss

18. 产生 Raw Events

19. Sensory Model → PerceptToken

20. FoundationEvaluator 旁路计算派生指标

21. 进入下一 Tick
```

关键约束：

> **FoundationEvaluator 只能读取状态，不得向任何一步写入 Drive、Coupling、Flow 或能力。**

---

## 24. 建议数据结构

以下是逻辑示意，不要求逐字实现。

### 24.1 NodeDynamics

```go
type NodeDynamics struct {
    Potential        Fixed
    StorageCapacity  Fixed
    Throughput       Fixed
    DriveCapacity    Fixed
    DriveEfficiency  Fixed
    AvailableEnergy  Fixed
}
```

### 24.2 BoundaryExchangeState

```go
type BoundaryExchangeState struct {
    Permeability [5]Fixed
    Flux         [5]Fixed
}
```

### 24.3 FeedbackCoupling

```go
type FeedbackCoupling struct {
    InputChannelID  string
    OutputChannelID string

    Strength        Fixed
    LearnedResponse Fixed
    Tolerance       Fixed
    LatencyTicks    int

    LastInputSignal Fixed
    LastError       Fixed
}
```

### 24.4 DriveBreakdown

```go
type DriveBreakdown struct {
    Passive    Fixed
    Autonomous Fixed
    Manual     Fixed
}
```

### 24.5 DriveResult

```go
type DriveResult struct {
    RequestedDrive Fixed
    ActualDrive    Fixed
    WorkCost       Fixed
    WasteHeat      Fixed
    Strain         Fixed
}
```

### 24.6 WorkLedger

```go
type WorkLedger struct {
    PassiveWork     Fixed
    AutonomousWork  Fixed
    ManualWork      Fixed

    DriveWork       Fixed
    ExternalInput   Fixed
    Generated       Fixed
    Loss            Fixed
}
```

### 24.7 底层结构禁止字段

禁止加入：

```text
IsFoundation
IsCultivationLoop
TechniqueName
RealmName
PathName
IsDantianSpecial
```

底层只描述物理和反馈状态。

---

## 25. 当前代码迁移建议

### P0-1：删除“筑基后注册 Loop”的反向因果

当前逻辑：

```text
FoundationEstablished
→ RegisterLoop(lastProtocol)
```

目标：

```text
Engine 自主形成闭环
→ FoundationEvaluator 识别
→ BodyPhase 更新
```

### P0-2：Flow 不再依赖 Request 才存在

`Ops.Requests` 改成 ManualCommand 的来源，而不是 Flow 的唯一入口。

### P0-3：Release Test 只关闭 ManualDrive

Release 时：

```text
ManualDrive = 0
```

世界模拟、BoundaryExchange、PassiveDrive、AutonomousDrive 全部继续。

### P0-4：正式引入 DriveCapacity

删除任何：

```text
Capacity 最大节点 = 主动核心
```

的隐式规则。

### P0-5：FeedbackCoupling 不得直接输出 Drive

必须：

```text
Coupling
→ AutonomousCommand
→ DriveResolver
→ ActualAutonomousDrive
```

### P1-1：加入 BoundaryExchange

优先替换修炼专用的被动吸收思路，并逐步统一 Diffuse / Leak / Intake 的边界交换逻辑。

### P1-2：引入 Teacher / Error Learning

不要沿用简单：

```text
TrainingSignal × (CouplingMax - Coupling)
```

改为“输入 → 教学输出 → 预测误差 → 安全门”的局部学习。

### P1-3：记录 WorkLedger

为 Internalization、RuleBalance 和能量审计提供真实来源。

### P1-4：只实现 Local TransitionCoupling

不实现 PathCoupling。

### P2：统一 DriveWork / Thermal / Strain

让“驱动成本、发热、用力过猛”由同一底层链路产生，为后续阵法 / 法器 / 丹炉复用。

---

## 26. 测试规范

### 26.1 BoundaryExchange 测试

给定：

```text
EnvironmentPotential > InternalPotential
Permeability > 0
ManualDrive = 0
```

应有：

```text
Environment → Carrier Flux > 0
```

反向势差时应允许泄出。

### 26.2 PassiveFlow 测试

```text
Potential[A] > Potential[B]
ManualDrive = 0
Coupling = 0
```

必须产生：

```text
PassiveFlow A → B > 0
```

### 26.3 DriveCapacity 测试

相同 Command：

- 高 DriveCapacity 节点能够输出更大 ActualDrive；
- 低 DriveCapacity 节点被限幅；
- 不得通过 StorageCapacity 间接代替 DriveCapacity。

### 26.4 DriveWork 能量守恒测试

有 ActualDrive 时：

```text
AvailableEnergy 必须下降
```

若无可用能量：

```text
ActualDrive 应下降或为 0
```

不得凭空产生 Work。

### 26.5 正常驱动低 Strain 测试

安全 DriveRatio 下：

```text
DriveWork > 0
DriveStrain ≈ 低
```

证明“耗能”与“受伤”没有被混成一个概念。

### 26.6 过驱测试

```text
DriveRatio > SafeDriveRatio
```

应出现：

```text
DriveStrain ↑
WasteHeat ↑
Stability ↓
```

### 26.7 未内化 Sequence 测试

```text
Coupling ≈ 0
ManualDrive = 0
```

应快速衰减或停止。

### 26.8 TeacherGate 饱和测试

超过正常教学强度后继续提高 ManualDrive：

```text
TeacherGate 不应继续线性增长
```

避免暴力操作获得更快学习。

### 26.9 Error Learning 测试

当 PredictedResponse 逐渐接近 TargetResponse：

```text
|Error| ↓
|ΔCoupling| ↓
```

最终自然收敛。

### 26.10 不稳定训练抑制测试

当：

```text
Stability ↓
Damage ↑
OverStrain ↑
```

必须：

```text
SafetyGate ↓
Coupling 学习变慢或停止
```

### 26.11 局部内化测试

A → B → C 中仅：

```text
Input A→B
Output B→C
```

形成高 Coupling：

- 没有 A→B 来流时，B→C 不得凭空启动；
- 有符合 tolerance 的来流时，B→C 应产生 AutonomousCommand；
- 没有能源时，Command 不能变成有效 Drive。

### 26.12 非闭合 Autonomous Sequence

局部段落全部可以自动运行，但最终不能返回起点：

```text
Closure < threshold
Foundation = false
```

### 26.13 Release Test

稳定闭环下：

```text
ManualDrive = 0
```

应继续正常模拟并计算真实 SelfSustain。

### 26.14 无环境补充的能量耗尽

自主 Loop：

- 可依靠内部储备运行；
- Reserve 持续下降；
- 最终 Flow 衰减；
- 不得永动。

### 26.15 环境维持

若：

```text
BoundaryExchangeInput
+ GeneratedUsableEnergy
≈ LoopLoss + DriveWork
```

允许长期稳定。

### 26.16 高自持低平衡

```text
SelfSustain 高
RuleBalance 低
```

必须：

```text
Foundation = false
```

### 26.17 高自持低动态稳定

构造振荡 Flow：

```text
SelfSustain 高
DynamicStability 低
```

必须：

```text
Foundation = false
```

### 26.18 次数无关性

两个角色：

- 修炼次数不同；
- 当前 World / Body / Coupling / Energy State 完全相同。

则：

```text
FoundationMetrics 必须完全相同
```

证明境界不是次数经验条。

### 26.19 局部而非 PathCoupling 测试

Engine 状态中不得存在：

```text
完整周天路径耦合值
```

Loop 必须可以由多个 Local TransitionCoupling 组合检测得到。

### 26.20 Deterministic Replay

相同：

```text
seed
rule_version
initial_world
intent_stream
```

必须得到完全一致的：

```text
BoundaryExchange
FeedbackCoupling
DriveWork
Flow
FoundationMetrics
Events
```

---

## 27. 仿真与时间尺度校准

Coupling 学习速度、筑基时长不能靠拍脑袋设定。

建议 Monte Carlo / Batch Simulation：

```text
1000+ Character
随机：
  Affinity
  StorageCapacity
  Throughput
  Conductance
  DriveCapacity
  DriveEfficiency
  Environment
  Route
  Flow
  Dwell
```

重点观测：

- 第一个局部 Coupling 何时出现；
- 何时出现 20% / 50% / 80% AutonomousWork；
- 哪些路线永远无法闭合；
- 哪些路线 SelfSustain 高但 RuleBalance 低；
- 哪些角色在低灵气环境无法维持；
- 哪些路线容易过热 / 振荡 / 损伤；
- 筑基时间分布是否过于集中；
- 是否存在单一全局最优路线。

需要调节：

```text
AdaptRate
TeacherDriveReference
SafeDriveRatio
DriveEfficiency
CouplingMax
Tolerance
BoundaryExchangeScale
Foundation Thresholds
```

目标不是：

> 所有人固定 30 分钟筑基。

而是：

> 不同身体、环境、路线产生合理且可感知的时间尺度差异，同时不存在明显的唯一答案。

---

## 28. 已知风险与防回退禁令

### 28.1 风险：FeedbackCoupling 退化成经验值

禁止：

```text
运行一次 +1
成功一轮 +0.01
在线时间越长 Coupling 越高
```

必须由真实输入输出学习得到。

### 28.2 风险：Feedback 变成隐藏能源

禁止：

```text
Coupling 越高 → 凭空输出更多能量
```

Coupling 只产生 Command，Drive 必须扣真实能源。

### 28.3 风险：PathCoupling 把功法写回底层

禁止在 Engine 存储整条周天“熟练度 / 耦合度”。

### 28.4 风险：DriveCapacity 再次被丹田语义绑死

禁止：

```text
if node == Dantian
```

或：

```text
capacity 最大 → drive 特权
```

### 28.5 风险：BoundaryExchange 变成免费吸气

交换必须受：

- 外部势；
- 内部势；
- Permeability；
- Openness；
- BackPressure；
- Affinity；

约束，不能无条件增加 Qi。

### 28.6 风险：SelfSustain 被误解为无限续航

SelfSustain 只测 Control Independence。

长期可持续性必须由 RuleBalance / EnergyBalance 决定。

### 28.7 风险：FoundationEvaluator 反向修改世界

它只能读取：

```text
State / Events / Ledger
```

不能：

- 注册 Loop；
- 增加 Coupling；
- 增加 DriveCapacity；
- 补充能量；
- 修复 Stability。

---

## 29. 对未来系统的复用

### 29.1 阵法

```text
阵眼 A 收到能量
→ Local FeedbackCoupling
→ 阵眼 B AutonomousCommand
→ DriveResolver
→ 实际能量传递
```

### 29.2 法器

```text
Sensor State
→ FeedbackCoupling
→ Actuator Command
→ DriveResolver
```

### 29.3 丹炉

```text
温度 / 压力变化
→ FeedbackCoupling
→ 调整阀门 / 导灵结构
→ 消耗真实能量
```

### 29.4 建筑

```text
环境变化
→ Local Feedback
→ 自动响应结构
```

因此 v0.2 不是在“补筑基玩法”，而是在世界底层正式加入：

> **能量可以跨边界交换；局部结构可以学习输入到输出的反馈映射；多个局部反馈可以组合成稳定自组织结构。**

---

## 30. 最终不可违反的 Invariants

1. **没有次数筑基。**
2. **没有进度条筑基。**
3. **没有“先筑基再注册 Loop”。**
4. **Flow 不依赖 Request 才允许存在。**
5. **SelfSustain 必须来自真实 Release Test。**
6. **SelfSustain 只表示脱离意识控制，不表示能源无限。**
7. **FeedbackCoupling 是控制映射，不是能量来源。**
8. **AutonomousCommand 必须经过 DriveResolver 才能成为 AutonomousDrive。**
9. **任何实际 Drive 都必须受 DriveCapacity 与 AvailableEnergy 约束。**
10. **任何实际 Drive 都必须记录 DriveWork。**
11. **正常驱动可以耗能但低 Strain；过驱 / 突变才显著增加 Strain。**
12. **环境被动吸收必须走 BoundaryExchange，不得无条件增量。**
13. **Coupling 不按次数增长。**
14. **Coupling 的学习不得奖励无限增大的 ManualDrive。**
15. **Engine 只允许 Local TransitionCoupling，不允许 PathCoupling。**
16. **Path / 周天 / 功法属于 Knowledge / Protocol / Blueprint 层。**
17. **Internalization 必须由实际 ManualWork / AutonomousWork 派生。**
18. **Autonomous Loop 仍然受能量守恒、损耗、环境与稳定性约束。**
19. **FoundationEvaluator 只能观察，不能注入任何世界能力。**
20. **LLM 只能根据 Perception 推测结构变化，不能读取隐藏真值或宣布境界事实。**

---

## 31. 一句话定义

> **Sequence 是意识逐段施加控制的行为程序；FeedbackCoupling 是身体在稳定教学样本中学会的局部输入→响应关系；Autonomous Loop 是这些局部反馈在真实能量约束下闭合后自然持续的动力学结构；Foundation 只是这个结构达到稳定阈值后的文明命名。**
