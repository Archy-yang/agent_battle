# World Rule v0.1

## 1. 核心目标

这个世界不是给玩家直接操作的，而是给智能体生存和创造的。

整个系统遵循一个最基本的边界：

> **LLM 决定意图，World Engine 决定现实。**

LLM 可以提出：

* 我要去某个位置
* 我要观察这个物体
* 我要把这些组件组合起来
* 我要制造一个侦察设备
* 我要设置一套自动触发逻辑
* 我要攻击 / 逃跑 / 交易 / 欺骗
* 我要尝试创造一种游戏里此前不存在的设备

但 LLM 无权直接决定：

* 自己是否成功
* 产生多少伤害
* 物品有什么属性
* 自己有没有死亡
* 世界状态发生了什么
* 一个新设计是否符合世界规律

这些永远由 World Engine 判断。

---

# 2. 长期设计原则

World Rule 应满足六个原则：

### 2.1 Stable — 稳定

世界最底层规则不随 LLM 更换而改变。

GPT、Claude、本地模型以及未来更强模型进入的都是同一个世界。

---

### 2.2 Composable — 可组合

尽量少设计：

* 职业
* 武器类型
* 技能
* 固定道具
* 固定配方

更多设计：

* 属性
* Component
* Connection
* Transformation

复杂事物通过组合产生。

---

### 2.3 Deterministic — 可判定

任何 LLM 行为最终必须能够转换成 Engine 可以判定的结构。

例如：

```text
“我要制造一个高速、低可探测性的侦察机器人。”
```

最终必须变成：

```text
Blueprint
 ├─ Light Frame
 ├─ Battery
 ├─ Motor
 ├─ Sensor
 ├─ Processor
 └─ Signal Dampener
```

然后由 Engine 计算最终性能。

---

### 2.4 Discoverable — 可理解

规则必须足够清晰，让 LLM 可以通过：

```text
observe
inspect
query_rule
experiment
```

逐渐理解这个世界。

不应该要求模型预先掌握几百页游戏规则。

---

### 2.5 Emergent — 允许涌现

游戏中最有价值的玩法应该是：

> 设计者没有明确设计过，但符合底层规则，所以允许存在。

例如某个玩家创造出一种：

* 能缓慢移动
* 能监听通信
* 很难被发现
* 被发现后自毁

的东西。

Engine 不需要知道它叫：

> 间谍机器人。

它只是一个符合规则的 Entity。

---

### 2.6 Evolvable — 接口可演化

世界规律稳定，但 Agent 能力层允许不断升级。

今天模型可能只能：

```text
move
use
assemble
```

未来模型可能可以：

```text
设计 Blueprint
设计控制程序
设计多级机器
协调多个子 Agent
建立生产流水线
建立复杂社会组织
```

World Engine 不应该因此重构。

---

# 3. 世界由六种 Primitive 构成

第一版建议只有六个真正底层的概念。

---

## 3.1 Space

一切 Entity 都存在于空间中。

拥有：

```text
position
orientation
size
volume
velocity
```

第一版可以简化成 Grid / Hex。

未来可以扩展：

```text
2D Grid
→ 连续2D
→ 3D
→ 不同世界空间
```

空间影响：

* 距离
* 移动
* 碰撞
* 射程
* 探测
* 信号
* 遮挡

---

# 3.2 Matter

世界中的物体由 Matter 构成。

第一版不模拟真实化学元素。

材料只需要几个抽象属性：

```text
mass
strength
density
conductivity
thermal_resistance
```

例如：

```text
Light Alloy
Mass: 1
Strength: 2

Heavy Alloy
Mass: 3
Strength: 7
```

这样 Engine 并不知道：

> 钢、钛、碳纤维。

它只理解材料属性。

未来可以不断增加 Material。

---

# 3.3 Energy

世界所有主动行为最终消耗 Energy。

包括：

```text
移动
探测
计算
通信
制造
攻击
维修
加工
```

统一模型：

```text
Energy Source
      ↓
Energy Storage
      ↓
Energy Consumer
```

例如：

```text
Generator
   ↓
Battery
   ↓
Motor
```

Engine 只需要理解 Energy Flow。

---

# 3.4 Structure

Structure 表示一个物体还能不能维持当前结构。

核心属性：

```text
Integrity
Mass
Strength
Load
```

受到外力、能量或者高温以后：

```text
Integrity ↓
```

如果：

```text
Integrity <= 0
```

Entity 或 Component 被破坏。

因此“HP”只是 Structure 在 UI 层的一种表现。

世界规则本身甚至没有：

```text
HP
```

只有：

```text
Structural Integrity
```

---

# 3.5 Information

这是整个游戏非常重要的一种资源。

世界真实状态：

```text
World State
```

永远只存在于 Engine。

Agent 得到的是：

```text
Observation
```

因此：

```text
Reality ≠ Agent Knowledge
```

Agent 的知识来自：

```text
Sensor
Communication
Memory
Inference
```

所以 Agent 可以：

* 看错
* 不知道
* 被欺骗
* 信息过期
* 做错误推断

但 World Engine 不会错。

---

# 3.6 Time

世界按 Tick 推进。

例如：

```text
Turn 17
```

每个 Tick：

```text
Collect Intent
↓
Validate
↓
Resolve
↓
Physics Update
↓
Generate Observation
```

第一版采用回合制。

但底层应该定义成 Tick，而不是 Turn。

未来就可以支持：

```text
Turn Based
Real Time
Asynchronous World
Persistent World
```

而不改变世界模型。

---

# 4. 世界唯一的对象：Entity

世界中不存在：

```text
玩家
机器人
武器
门
无人机
炮塔
汽车
陷阱
```

这些底层全部都是：

```text
Entity
```

基本结构：

```text
Entity {
    id

    transform
    material

    components[]
    connections[]

    state

    owner?
}
```

区别只是挂载了不同 Component。

---

# 5. Component 是能力的最小单位

Component 不代表“物品”。

它代表：

> 一种局部物理能力。

第一版可以从大约 10 类开始。

```text
Frame
Storage
EnergySource
Battery
Motor
Sensor
Emitter
Processor
Communication
Cooling
```

后续增加 Component 不需要改变 Entity 模型。

---

## Component 必须拥有 Port

这是整个组合系统非常关键的一点。

例如：

```text
Battery

Input:
    energy_in

Output:
    energy_out
```

Motor：

```text
Input:
    energy

Output:
    motion
```

Sensor：

```text
Input:
    energy
    signal

Output:
    information
```

Processor：

```text
Input:
    information
    energy

Output:
    control
```

那么一个设备实际上就是：

```text
Battery.energy_out
        ↓
Motor.energy

Battery.energy_out
        ↓
Sensor.energy

Sensor.information
        ↓
Processor.information

Processor.control
        ↓
Motor.control
```

本质是一个：

> **Component Graph**

---

# 6. 所有能力本质上都是 Transformation

World Engine 可以定义一种非常通用的模型：

```text
Input → Transformation → Output
```

例如：

Motor：

```text
Energy → Motion
```

Sensor：

```text
Energy + Environment
→ Information
```

Emitter：

```text
Energy
→ Radiation / Projectile
```

Generator：

```text
Fuel / Environment
→ Energy
```

Cooling：

```text
Heat
→ Environment
```

Processor：

```text
Energy + Information
→ Control Signal
```

这样以后增加新 Component，非常容易。

---

# 7. 不定义 Weapon

这是一个重要原则。

World Engine 第一版最好没有：

```text
Weapon
DamageSkill
Attack
```

而只有：

```text
Energy
Mass
Velocity
Temperature
Collision
Radiation
Integrity
```

假设一个组件能够：

```text
Energy
↓
高速 Projectile
```

Projectile 撞到 Entity：

```text
kinetic_energy
↓
Structure
↓
Integrity Loss
```

玩家自然会把它叫：

> 枪。

但是在底层：

> 枪不是基本概念。

---

# 8. Heat 应作为第一个重要扩展属性

虽然 Primitive 可以先保持六个，但建议第一版 Simulation 就保留 Heat。

因为它非常容易形成取舍：

```text
Power ↑
↓
Heat ↑
```

比如一个高功率 Motor：

```text
移动快
但是发热高
```

Sensor：

```text
探测距离大
但是耗能和发热高
```

Emitter：

```text
输出高
但是需要 Cooling
```

这样很多 Build 会自然产生。

---

# 9. Signal 是第二个重要扩展属性

很多世界行为都可以统一为 Signal：

```text
光
无线电
声音
热辐射
主动扫描
```

不同 Sensor：

```text
Optical Sensor
Thermal Sensor
Radio Sensor
Acoustic Sensor
```

只是：

> 对不同 Signal 敏感。

这样 Stealth 就不需要成为：

```text
stealth = 80
```

而可以自然变成：

```text
降低某类 Signal
```

例如：

```text
低温设备
→ Thermal Signal ↓

通信静默
→ Radio Signal ↓

低速移动
→ Acoustic Signal ↓
```

这比一个“隐身数值”更容易产生策略。

---

# 10. Agent 本身也是 Entity

Agent 在世界中没有特权。

例如：

```text
Agent Body

Frame
Battery
Motor
Sensor
Processor
Communication
```

其中：

```text
Processor
```

对应外部 LLM。

可以理解成：

```text
World Entity
      ↓
Processor Component
      ↓
Agent Runtime
      ↓
LLM
```

于是模型就是这个世界中某个 Processor 的“智能实现”。

非常自然。

---

# 11. Agent Runtime 与世界彻底解耦

这一层必须单独设计。

```text
LLM

↑

Agent Runtime

↑

Agent Protocol

↑

World Engine
```

World Engine 永远不应该知道：

```text
GPT
Claude
Gemini
Qwen
Local Model
```

它只知道：

```text
AgentProtocol
```

---

# 12. AgentProtocol v1

Engine 给 Agent：

```text
Observation
Capability Manifest
Rules Reference
Events
```

Agent 返回：

```text
Intent[]
```

例如：

```json
{
  "intents": [
    {
      "type": "move",
      "target": [12, 8]
    },
    {
      "type": "activate",
      "component": "sensor_3",
      "parameters": {
        "direction": "north"
      }
    }
  ]
}
```

Engine 再：

```text
Validate
↓
Simulate
↓
Resolve
```

---

# 13. 不把 Tool Schema 固化成世界规则

这是为了未来模型变化，非常重要。

今天 LLM 可能适合：

```text
JSON Function Calling
```

未来模型可能直接生成：

```text
DSL
```

甚至直接操作：

```text
World Graph
```

所以不要写成：

```text
World Engine = Tool API
```

应该是：

```text
World Engine
    ↑
Intent Protocol
    ↑
Tool Adapter
```

今天可以有：

```text
OpenAI Tool Adapter
Anthropic Tool Adapter
MCP Adapter
JSON Adapter
```

以后换协议也不影响游戏。

---

# 14. Agent 的基础 Intent

第一版建议保持非常少：

```text
inspect
move
manipulate
connect
disconnect
configure
activate
transfer
communicate
assemble
disassemble
```

注意：

```text
attack
heal
craft_weapon
build_robot
```

都不是 Primitive Intent。

---

# 15. Blueprint —— 创造世界最核心的数据结构

LLM 想创造一个东西，最终必须提交：

```text
Blueprint
```

Blueprint 本质也是 Graph。

例如：

```text
Scout Device

Components:

Light Frame
Battery
Motor
Sensor
Processor

Connections:

Battery → Motor
Battery → Sensor
Battery → Processor

Sensor → Processor
Processor → Motor
```

然后 Engine 进行：

```text
Validate Ports
↓
Validate Physical Constraints
↓
Validate Available Materials
↓
Calculate Mass
↓
Calculate Energy
↓
Calculate Heat
↓
Calculate Performance
↓
Instantiate Entity
```

LLM 负责设计。

Engine 负责实现。

---

# 16. Blueprint 不应该只能来自预置配方

可以有三种层级。

### Level 1：Template

适合弱模型或者新人：

```text
Scout Template
```

只调整参数。

---

### Level 2：Component Assembly

模型自己组合 Component。

这是游戏早期核心。

---

### Level 3：Free Blueprint

强模型可以设计完整 Graph。

未来 LLM 越来越强，就逐渐开放这一层。

因此：

> 模型能力增强不会破坏游戏，反而会逐渐解锁更深的世界操作能力。

---

# 17. Logic —— 给创造物赋予行为

这是未来扩展潜力最大的部分之一。

Entity 可以拥有：

```text
Logic Component
```

但绝不能允许执行任意系统代码。

应该使用 World DSL。

例如：

```text
WHEN sensor.detect(entity)
AND entity.owner != self.owner
AND distance < 5
THEN activate(emitter)
```

或者：

```text
WHEN battery.energy < 10%
THEN return(home)
```

这就可以自然形成：

* 自动炮塔
* 无人机
* 陷阱
* 门禁
* 采矿机
* 警报系统

---

# 18. World DSL 要比 LLM 生命周期更长

今天可能用：

```text
JSON Logic
```

以后可能升级为：

```text
Graph Program
Behavior Tree
Event System
```

所以逻辑层同样应该抽象：

```text
Behavior Definition
        ↓
Behavior Runtime
```

而不是把脚本语言焊死。

---

# 19. Observation 是独立系统

LLM 永远不能拿完整 World State。

Engine 内部：

```text
World State
```

经过：

```text
Sensor Simulation
Visibility
Signal
Occlusion
Noise
Memory Policy
```

才能产生：

```text
Observation
```

例如：

```text
Object #381

distance: ~8
type: unknown structure
heat: medium
movement: none

confidence: 0.72
```

这允许未来做非常丰富的信息战争。

---

# 20. Observation 同样不能绑死文本

今天：

```text
Structured JSON
```

以后模型可能更擅长：

```text
地图图像
视觉画面
声音
3D场景
```

因此 Engine 应该内部维护：

```text
Observation Model
```

然后不同 Adapter 转换成：

```text
JSON Adapter
Text Adapter
Image Adapter
Vision Adapter
Multimodal Adapter
```

这样未来多模态模型变强以后，可以直接让 Agent：

> “看到世界。”

而不需要重写世界本身。

---

# 21. Memory 不属于 World Engine

这一点也必须明确。

世界只负责：

```text
Reality
```

Agent Runtime 负责：

```text
Memory
```

例如：

```text
Turn 21:
enemy_7 seen at D4
```

到了 Turn 40：

Engine 不会继续告诉 Agent：

> enemy_7 曾经在那里。

是否记得，由 Agent 自己决定。

这使得不同 Memory System 本身也可以成为玩家 Build 的一部分。

---

# 22. Agent Capability Manifest

为了支持未来模型，Engine 每一轮应该能告诉 Agent：

```text
你当前能做什么。
```

但不是枚举所有合法动作。

而是：

```text
Capabilities

Movement
Assembly
Communication
Programming
Sensor Access
Manipulation
```

例如：

```json
{
  "capabilities": {
    "move": true,
    "assembly": {
      "max_components": 8
    },
    "logic": {
      "complexity_limit": 12
    },
    "communication": true
  }
}
```

这样将来可以根据：

* 世界阶段
* 设备性能
* 比赛模式
* 模型等级

动态控制 Agent 能力。

---

# 23. 最重要的未来兼容机制：Complexity Budget

未来模型一定会越来越强。

如果规则完全开放，可能发生：

> 2026 年模型只能造小车，2030 年模型一回合设计完整工业体系。

所以不能靠限制“模型能想到什么”。

要限制：

> **世界允许它一次实现多少复杂度。**

例如：

```text
Blueprint Complexity

Component Count
Connection Count
Logic Nodes
Energy Requirement
Manufacturing Time
Required Processor Capacity
```

这意味着：

模型再聪明，也只能在世界资源约束下创造。

强模型的优势变成：

> 用相同资源创造更好的方案。

而不是：

> 强模型可以突破游戏规则。

---

# 24. Processor Capacity

甚至可以把 LLM 的能力映射成世界内资源。

比如 Processor Component 拥有：

```text
Compute Capacity
Memory Capacity
Control Channels
```

它并不是直接等于 Token。

而是一种游戏资源。

例如：

```text
Processor Level 1

最多控制：
1 个自身 Entity

Logic Complexity:
10

Memory Interface:
small
```

以后：

```text
Processor Level 5
```

可以：

```text
控制多个 Entity
运行复杂 Logic
设计复杂 Blueprint
```

这可以成为非常漂亮的科技成长线。

---

# 25. 未来 LLM 的能力可以按五个阶段开放

这套世界最好从一开始就支持下面的演化。

## Stage 1 — Action Agent

模型只能：

```text
观察
移动
使用已有设备
```

---

## Stage 2 — Creator Agent

可以：

```text
组合 Component
制造 Blueprint
```

---

## Stage 3 — Programmer Agent

可以：

```text
给 Entity 编写 Logic
```

---

## Stage 4 — System Designer

可以：

```text
制造机器
制造生产链
让机器制造机器
```

---

## Stage 5 — Civilization Agent

最终可能：

```text
建立工厂
建立通信网络
建立组织
建立经济
创造自己的子 Agent
```

但底下仍然遵循：

```text
Matter
Energy
Space
Structure
Information
Time
```

完全不需要换世界。

---

# 26. 子 Agent 未来也可以自然出现

以后完全可能允许一个 Agent 制造：

```text
Processor Component
```

然后挂载另一个模型。

于是世界里出现：

```text
Main Agent
    ↓
Scout Agent
    ↓
Worker Agent
```

但这不是现在需要实现的。

我们只需要保证：

> Processor 本身就是一种 Component。

以后自然能接进去。

---

# 27. Rule Discovery

我认为不要把完整规则一次塞给 LLM。

应该提供：

```text
query_rule(topic)
inspect(component)
simulate_blueprint(blueprint)
```

例如 Agent 可以问：

```text
query_rule("motor")
```

得到：

```text
Motor converts Energy into Motion.

Output depends on:
- power
- mass
- load

High power creates additional Heat.
```

甚至可以：

```text
simulate_blueprint()
```

得到：

```text
Estimated:

Mass: 17
Max speed: 3
Energy consumption: 8/tick
Heat generation: 4/tick
```

这就像工程设计。

---

# 28. Simulation 和 Reality 分开

为了鼓励创造，可以提供一个虚拟测试接口：

```text
simulate_blueprint
```

它不会改变世界。

而：

```text
assemble
```

才真正消耗：

```text
Matter
Energy
Time
```

这样 LLM 可以：

```text
设计
↓
模拟
↓
调整
↓
制造
```

这本身就是很好的 Agent 工作流。

---

# 29. Rule Versioning

这是为了未来非常重要的一层。

每场游戏绑定：

```text
Ruleset Version
```

例如：

```text
world-rule/1.0
component-set/3.2
simulation/2.1
```

比赛开始以后固定。

这样以后：

```text
新模型
新 Component
新物理规则
```

都不会破坏历史 Replay 和公平性。

Replay 可以重新跑出完全相同结果。

---

# 30. Engine 最终可以拆成五个稳定模块

```text
                  WORLD ENGINE

┌─────────────────────────────────┐
│ 1. Entity / Component Graph     │
└─────────────────────────────────┘
                ↓
┌─────────────────────────────────┐
│ 2. Rule / Simulation Engine     │
│ Space Energy Matter Signal ...  │
└─────────────────────────────────┘
                ↓
┌─────────────────────────────────┐
│ 3. Intent Resolver              │
│ Validate → Resolve              │
└─────────────────────────────────┘
                ↓
┌─────────────────────────────────┐
│ 4. Observation Engine           │
│ Reality → Perception            │
└─────────────────────────────────┘
                ↓
┌─────────────────────────────────┐
│ 5. Agent Protocol               │
│ Observation ↔ Intent            │
└─────────────────────────────────┘
```

其中前四层尽可能长期稳定。

Agent Protocol 可以不断升级。

---

# 31. v0.1 最小实现范围

第一版千万不要实现整个梦想。

只做：

### Primitive

```text
Space
Matter
Energy
Structure
Information
Time
```

### Extended Properties

```text
Heat
Signal
```

### Component

```text
Frame
Battery
Motor
Sensor
Processor
Emitter
Storage
Cooling
```

### Action

```text
inspect
move
connect
disconnect
activate
assemble
disassemble
transfer
```

### World

```text
小型二维地图
少量资源
固定 Tick
```

然后只验证一件事情：

> **LLM 能否理解这些规则并创造出我们没有预先设计过，但确实有实际用途的 Entity？**

比如让模型面对：

```text
目标：

进入一个危险区域，
找到目标，
获得信息，
并尽量避免自身损坏。
```

只给它基本组件。

看看不同模型会不会造出：

```text
高速侦察车
固定监控器
一次性探测器
诱饵
远程传感装置
```

如果能出现这些东西：

**这个世界模型就成立了。**

---

# 32. 整个游戏最核心的不变量

无论未来模型发展到什么程度，建议始终不改变四句话：

> **世界有唯一真实状态。**

> **Agent 只能通过感知了解世界。**

> **Agent 只能通过合法 Intent 改变世界。**

> **一切结果由世界规则计算，而不是由 Agent 描述。**

只要这四条一直成立，那么即使未来 LLM 的能力比现在强几十倍，这个游戏仍然成立。

模型越强，只会表现为：

> 更会理解这个世界。

> 更会利用这个世界。

> 更会创造这个世界里以前没人想到的东西。

而不会变成：

> 更会绕过游戏规则。
