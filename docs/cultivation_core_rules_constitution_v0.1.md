# 修真 Sandbox Engine：底层规则与组合规则设计宪法 v0.1

## 1. 文档定位

本文档用于规定整个项目最高层的设计原则。

它不定义某一个具体玩法，也不定义某一本功法、某一种丹药、某一种阵法或某一类装备。

它只回答一个问题：

> **这个世界最底层允许什么存在、什么变化、什么组合；上层玩法应如何从这些基础规则中自然产生。**

后续所有系统设计——修炼、功法、炼丹、炼器、阵法、建筑、战斗、宗门、生产、探索——都必须优先服从本文档。

---

## 2. 项目最核心的设计目标

本项目不是通过不断新增“玩法系统”来扩展内容，而是通过不断完善一套统一的世界底层规则，让新的玩法可以从规则组合中自然出现。

核心目标：

```text
少量稳定的底层规则
        ↓
可组合的实体 / 结构 / 过程
        ↓
复杂且不可完全预设的上层结果
        ↓
玩家 + LLM 发现、利用并命名这些结果
```

因此必须始终坚持：

> **规则少而稳定，组合多而开放。**

以及：

> **Engine 定义因果，不定义玩法名称。**

---

## 3. 为什么要这样设计

传统游戏通常采用：

```text
功法系统
炼丹系统
炼器系统
阵法系统
建筑系统
战斗系统
```

每个系统分别设计规则。

这种方式的问题是：

1. 系统之间重复定义大量相似逻辑；
2. 不同玩法难以真正互相作用；
3. 每增加一个境界、职业或玩法都需要继续新增大量特殊规则；
4. 玩家只能在开发者预设的“解法空间”内组合；
5. LLM 最终只能成为内容生成器，而无法真正研究世界。

本项目目标相反：

```text
统一底层规则
    ↓
人体是一个结构
丹炉是一个结构
阵法是一个结构
法器是一个结构
建筑也是一个结构
    ↓
它们只是不同 Carrier / Node / Channel / Material / Process 的组合
```

如果不同玩法能复用同一套底层规则，那么它们就能自然产生跨系统影响。

---

## 4. 类比：Minecraft 式设计思想

本项目应更接近 Minecraft 的设计精神，而不是大量独立玩法模块。

Minecraft 不需要定义：

```text
自动门系统
升降机系统
计算器系统
自动农场系统
```

它只需要定义：

```text
方块
空间占据
信号传递
活塞位移
重力
流体
碰撞
材料性质
```

玩家最终组合出开发者没有逐个写死的复杂结构。

本项目同理。

Engine 不应该定义：

```text
聚灵阵
飞剑
回春丹
火系功法
洞府聚灵室
```

Engine 应定义：

```text
能量怎样流动
材料怎样承载
结构怎样连接
通道怎样传输
状态怎样转化
稳定结构怎样形成
环境怎样影响过程
控制信号怎样改变过程
```

最终某个结构持续把环境灵气汇聚到中心，玩家把它命名为“聚灵阵”。

“聚灵阵”是文明和玩家的概念，不是底层物理的类型。

---

## 5. 三层架构

整个世界建议长期维持三层抽象。

### 5.1 Layer 1：World Physics / 底层世界规则

回答：

> 世界中的东西怎样存在、变化和相互作用？

这一层必须尽量稳定，修改成本最高。

当前候选基础概念：

```text
Space
Time
Matter / Material
Essence / Energy
Nature
Carrier
Channel
Structure
Field / Environment
Threshold
Information / Control
```

当前候选基础算子：

```text
Generate
Consume
Transfer
Transform
Amplify
Suppress
Accumulate
Diffuse
Bind
Release
Stabilize
Destabilize
```

这层不应该出现：

```text
功法
丹药
飞剑
阵法
宗门建筑
```

这些都属于上层语义。

---

### 5.2 Layer 2：Composition Grammar / 组合语法

回答：

> 基础世界对象怎样组合成复杂系统？

这是项目目前最需要继续补齐的一层。

建议优先抽象：

```text
Entity
Component
Port
Connection
Node
Channel
Container
Structure
Process
Controller
Blueprint
```

其中：

- Entity：世界中可独立存在的对象；
- Component：对象内部可组合的功能单元；
- Port：组件允许连接、传输或交互的位置；
- Connection：组件、节点之间的连接关系；
- Container：允许承载其他对象或物质；
- Structure：多个组件形成的稳定整体；
- Process：按时间发生的一系列状态变化；
- Controller：对 Process 施加控制逻辑；
- Blueprint：描述结构如何被组合，而不是直接描述“这是什么玩法”。

这一层决定世界是否真正具有 Sandbox 能力。

---

### 5.3 Layer 3：Emergent Meaning / 上层涌现语义

回答：

> 玩家和文明把一个已经成立的结构称为什么？

例如 Engine 看到：

```text
5 个 Node
闭合 Channel
中心形成持续负压
环境 Essence 稳定流向中心
```

Engine 只知道这些事实。

玩家可能把它命名为：

```text
聚灵阵
```

同理，一个物体具备：

```text
高结构强度
高金性
可储存 Essence
可将 Essence 定向释放至刃部
```

玩家可以把它叫：

```text
飞剑
灵剑
能量刃
```

Engine 不需要知道这些名称之间的文化差异。

---

## 6. 一个新玩法出现时，先问“是否需要新规则”

任何新增玩法需求都必须经过以下检查。

假设需求是：

> 增加飞剑。

禁止第一反应：

```text
新增 FlyingSwordSystem
新增 flying_sword_damage
新增 flying_sword_speed
新增 flying_sword_level
```

应先拆解：

```text
飞剑需要什么？

Material
Structure
Energy Storage
Channel
Directional Release
Control
Force / Motion
```

然后检查：

> 现有基础规则是否已经可以表达？

只有确实存在无法表达的基础现象时，才考虑新增 Primitive 或 Operator。

---

## 7. 新增底层规则的准入条件

任何新的 Primitive / Operator 都必须至少满足以下一项：

1. 能被多个不同上层系统复用；
2. 能产生新的可观察因果结果；
3. 现有规则无法自然表达该现象；
4. 不加入它会导致大量上层特殊 case；
5. 它足够基础，不能继续拆成现有概念组合。

如果一个规则只服务于某个单独玩法，应优先认为它不应该进入底层。

错误示例：

```text
AlchemySuccessRate
FlyingSwordDamageBonus
FoundationPassiveBonus
FormationDamageType
```

更合理的底层表达：

```text
Material Stability
Transform Efficiency
Energy Density
Flow
Pressure
Structure Integrity
Control Precision
```

---

## 8. 禁止“玩法名称驱动底层代码”

底层代码中应尽量避免：

```text
if item.type == 'pill'
if skill.type == 'cultivation'
if structure.type == 'formation'
if realm == 'foundation'
```

优先表达为：

```text
if carrier.can_store(...)
if structure.is_closed_loop(...)
if material.stability < threshold
if process.self_sustain >= threshold
```

也就是说：

> 底层判断应基于属性与结构，而不是基于策划名称。

---

## 9. 当前“修炼系统”如何符合这一思想

现阶段已经建立的修炼模型正是第一块验证。

角色身体被抽象为：

```text
Node = 穴位 / 丹田
Channel = 经脉
Carrier = 身体
Essence = 灵气
Nature = 五行倾向
Structure = 身体稳定结构
```

功法不是 Skill ID，而是：

```text
Path
+ Amount
+ Flow
+ Dwell
+ Cadence
+ Guards
```

也就是一个可重复 Process / Protocol。

筑基也不是：

```text
经验值满
```

而是：

```text
Sequence
→ Stable Self-Sustaining Loop
```

这证明“境界”也可以成为底层结构变化的上层名称。

---

## 10. 阵法如何复用同一套规则

修炼：

```text
穴位 = Node
经脉 = Channel
身体 = Carrier
```

阵法：

```text
阵眼 = Node
阵纹 = Channel
阵基 = Carrier / Structure
```

如果形成稳定闭合能量流：

```text
Stable Loop
```

那么从 Engine 看，人体筑基与阵法成型可能共享大量相同数学结构。

不同仅仅是：

```text
Carrier 不同
Material 不同
控制方式不同
环境约束不同
```

因此不得为阵法重新复制一整套独立“能量循环规则”。

---

## 11. 炼丹如何复用同一套规则

禁止：

```text
药材 A + 药材 B + 火 10 秒
= 回春丹
```

应表达为：

```text
Material A
Material B
Temperature
Pressure
Container
Time
Transform
Release
Bind
Stabilize
```

过程可能是：

```text
材料结构破坏
→ 活性成分释放
→ Essence 发生转化
→ 不同成分重新 Bind
→ 形成新的 Stable Structure
```

最后这个新结构进入人体后，会改变某些底层 Process。

玩家把它叫“回春丹”。

Engine 不需要预先知道“回春丹”是什么。

---

## 12. 炼器如何复用同一套规则

一个法器可以表示为：

```text
Material
+ Structure
+ Components
+ Ports
+ Channels
+ Energy Storage
+ Controller
```

例如一把灵剑可能产生的能力，不来自：

```text
weapon.damage += 30%
```

而来自：

```text
材料本身高结构强度
+ 高能量传导率
+ 刃部具有定向释放结构
+ 使用者可以通过 Channel 注入 Essence
```

因此其攻击能力是结构结果，而非固定类型奖励。

---

## 13. 建造如何复用同一套规则

未来建筑系统不应只支持预设建筑类型。

底层只需要知道：

```text
Material
Position
Volume
Connection
Support
Seal
Environment Interaction
Energy / Essence Flow
```

由此可能自然产生：

```text
普通房屋
密室
聚灵室
隔热室
炼丹房
阵法建筑
储能设施
```

建筑名称属于玩家语义。

---

## 14. LLM 在这个体系中的位置

LLM 不负责定义世界规则。

LLM 的角色是：

```text
观察
理解
提出假设
设计实验
组合已有 Primitive
创建 Blueprint / Protocol
解释结果
命名结果
```

因此整体关系：

```text
World Rules
    ↓
Simulation
    ↓
Observation
    ↓
LLM / Player
    ↓
Hypothesis / Design
    ↓
Composition
    ↓
World Rules 验证
```

必须避免：

```text
LLM 说“这个阵法应该成功”
→ 系统就成功
```

正确方式：

```text
LLM 提交阵法结构
→ Engine 根据节点、材料、连接、流量、稳定性计算
→ 结构成立或崩溃
```

---

## 15. “知识”与“世界真相”必须分离

World Truth：

```text
由 Engine 持有
```

Agent Knowledge：

```text
由观察和实验形成
可能正确
可能部分正确
可能错误
```

因此玩家 / LLM 可以：

```text
发现规律
误解规律
修正理论
创造不同流派
```

同一个底层世界可以自然产生不同文明知识体系。

这是 Sandbox + LLM 组合的关键价值。

---

## 16. 规则应优先产生连续结果，而不是离散奖励

不推荐：

```text
木系功法 +20% 回血
筑基 +30% 灵力
金丹技能伤害 +50%
```

优先：

```text
木性倾向提高某些 Generate / Growth Process 的效率
稳定 Loop 降低重复控制和传输损耗
Stable Core 提高局部能量密度、自保持能力与可调用储量
```

上层的“更强”应是这些真实结构变化产生的结果。

---

## 17. 复杂性来源

世界复杂性必须主要来自：

```text
简单规则
×
大量实体
×
拓扑关系
×
不同材料参数
×
不同环境
×
时间演化
×
反馈循环
```

而不是：

```text
一个巨大公式
+ 大量特殊条件
+ 大量技能脚本
```

这是整个项目未来可维护性的核心。

---

## 18. Rule Versioning

底层规则一旦进入正式世界，应尽量保持稳定。

建议所有世界状态记录：

```text
rule_version
```

例如：

```text
world_rule_version = 0.1.3
```

修改底层公式时必须考虑：

1. 旧存档是否仍可重放；
2. 旧功法 / Blueprint 是否会失效；
3. 是否需要 Migration；
4. 是否只是调参，还是因果规则变化。

区分：

```text
Parameter Tuning
```

与：

```text
Rule Semantic Change
```

后者应极其谨慎。

---

## 19. Deterministic Simulation

对于相同：

```text
World State
Action / Intent
Rule Version
Seed
```

Engine 应尽量产生相同结果。

这样可以支持：

```text
Replay
Debug
Balance Test
Agent Evaluation
Bug Reproduction
```

随机性可以存在，但必须是：

```text
Seeded Randomness
```

而不是不可重现的随机行为。

---

## 20. 新系统开发检查清单

未来任何开发 Agent 开始实现新玩法前，都必须回答以下问题。

### 20.1 这个玩法的底层实体是什么？

例如：

```text
Material?
Entity?
Structure?
Process?
```

### 20.2 使用了哪些已有 Primitive？

尽量复用已有概念。

### 20.3 使用了哪些已有 Operator？

例如：

```text
Transfer
Transform
Bind
Stabilize
```

### 20.4 是否真的需要新的底层规则？

如果需要，说明为什么现有规则无法表达。

### 20.5 新规则是否至少可以被两个以上不同系统复用？

如果不能，优先降级为上层逻辑。

### 20.6 Engine 是否知道了不应该知道的“玩法名称”？

如果代码开始判断：

```text
Pill
Formation
Sword
CultivationSkill
```

必须重新审视抽象。

### 20.7 这个结果是否可以从底层状态自然推导？

如果只能依赖策划脚本强行赋值，说明底层可能缺少概念。

---

## 21. 判断一个抽象是否正确的三个问题

每一层都应检查：

### 问题一

> 它是否比上一层更基础？

### 问题二

> 下一层能否由它组合或涌现出来？

### 问题三

> 最终世界中的对象，能否一路反向追溯到底层规则？

例如：

```text
聚灵阵
↓
稳定汇聚结构
↓
Node + Channel + Field
↓
Transfer + Pressure + Stabilize
↓
World Physics
```

如果无法追溯，说明中间可能存在硬编码玩法逻辑。

---

## 22. 当前阶段优先级

当前不应该立即完整开发：

```text
炼丹系统
炼器系统
阵法系统
建筑系统
```

下一阶段更合理的工作是完善 Composition Grammar。

优先验证以下 Primitive 是否足够通用：

```text
Entity
Material
Component
Port
Connection
Container
Structure
Process
Controller
Blueprint
```

然后用它们做概念验证：

```text
人体
功法
阵法
丹炉
简单法器
简单建筑
```

如果同一套抽象能够自然表达这些对象，说明方向正确。

---

## 23. 最高优先级禁止事项

开发过程中应持续禁止以下倾向：

### 禁止 1：为了赶功能直接新增玩法专用规则

例如：

```text
formation_bonus
pill_success_rate
foundation_attack_bonus
```

### 禁止 2：让 LLM 直接创造世界事实

LLM 只能提出 Intent / Blueprint / Protocol。

### 禁止 3：让上层名称渗透到底层 Engine

底层应关心属性、连接、状态和过程。

### 禁止 4：用等级数字替代结构变化

境界、法器品质、阵法水平，应尽量对应真实结构差异。

### 禁止 5：为了“丰富”不断新增参数

新增参数必须产生可观察、可决策的策略差异。

---

## 24. 中心架构图

```text
                 World Physics
                       │
       ┌───────────────┼───────────────┐
       │               │               │
    Matter          Essence        Information
       │               │               │
       └───────┬───────┴───────┬───────┘
               │               │
          Structure         Process
               │               │
               └───────┬───────┘
                       │
              Composition Grammar
                       │
        Entity / Component / Port
        Connection / Container
        Controller / Blueprint
                       │
        ┌──────────────┼───────────────┐
        │              │               │
      身体            物品            建筑
        │              │               │
      功法            法器            阵法
        │              │               │
      筑基            丹炉            洞府
        │              │               │
        └──────────────┼───────────────┘
                       │
               Emergent Meaning
                       │
              Player + LLM 命名
```

---

## 25. 最终中心思想

整个项目未来所有开发决策都应围绕以下几句话展开。

> **底层只定义世界如何运行，不定义玩家应该怎么玩。**

> **上层玩法应尽量是底层规则组合出来的结果，而不是独立脚本。**

> **Engine 生产因果，玩家与 LLM 发现因果、组合因果、利用因果。**

> **同一个规则应尽可能同时支撑修炼、炼丹、炼器、阵法、建筑和战斗。**

> **当一个新玩法需要新增系统时，首先怀疑的应该不是“缺少玩法代码”，而是“现有 Primitive 是否缺少一个真正基础的概念”。**

项目真正想构建的不是一组修真玩法，而是：

> **一个能够自然产生修真玩法的世界。**
