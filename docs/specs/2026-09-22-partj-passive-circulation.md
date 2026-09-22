# Part J：Sequence 物理地变成 Loop —— 被动环流设计 v0.1

> 状态：待评审。背景：外部代码审查（2026-09-22）确认筑基死锁——传输全由
> Ops.Requests 驱动、ControlActive=Requests>0、AutonomousLoop 筑基后才注册，
> 故松手必无流、SelfSustain 恒 0、真实流程无法自然筑基。本文给出引擎级修复设计。
>
> 原则红线（审查者与本项目一致）：**不允许把 Protocol Runtime 运行伪装成身体自持**。

---

## J1 诊断：当前引擎是命令驱动的传输器

```text
现状：Flow = f(Ops.Requests)
      松手（Requests=0）→ Flow=0 → SelfSustain=0 → 筑基不可达
      RegisterLoop 仅在 Established 后 → 先有鸡还是先有蛋
```

缺失的是"世界在没有命令时也会自己动"的物理。

## J2 被动输运（Passive Transport）

每 tick 在主动 Requests 之外，新增压力梯度驱动的被动项：

```text
对每条经脉 ch（天然向 a→b）：
  ΔP = Pressure(a) - Pressure(b)          // Pressure 由 LoadRatio 派生（现状）
  passiveFlow(ch) = clamp(K_passive × ΔP × Conductance(ch), 0, Throughput)
  方向：沿天然向（压差为负则被动流为零——被动项永不逆天然向）
```

- **主动 Request 照旧叠加**；逆流仍是逆天然向，唯有核点可发起、需做功（A2 不变）
- 被动项使压差均衡化：死端/树杈会停滞归零；**单向环上的压差可持续传播**
  ——主环拓扑从"设计选择"升格为"物理必然"，没有环就没有自持

## J3 途经纳气（Entrained Intake）

气行经穴位时卷入少量环境气（让"边过边纳"成为真话，并为自持循环供给能量）：

```text
对每条有实际流量 F 的经脉，其接收端穴位 i：
  entrained[i,e] = F × Openness(i) × EnvironmentQi(e)
                   × AcupointAffinity(i,e) × K_entrain × (1 - LoadRatio(i))
```

- 来源=环境，逐项计入守恒账（A3 恒等式扩一项），不破守恒
- 量级受 BackPressure 抑制——丹田满时途经纳气也难，P1 烧蚀闭环保持成立

## J4 ControlActive 重定义

```text
ControlActive(t) = 本 tick 存在玩家意图请求 OR Protocol Runtime 在执行
被动流不计入控制——Runtime 跑协议仍是 ControlActive（不做假自持）
```

## J5 SelfSustain 测量协议（替换现 foundation.go 的喂样口径）

```text
推动窗口 W1：正常行环（有请求）的均流
松手窗口 W2：其后连续 N tick 全无请求的均流（纯被动环流）
SelfSustain = mean(Flow in W2) / mean(Flow in W1)
```

- 衰减时间常数由身体结构决定：Conductance/Adaptation 越高（反复安全行环的
  结果），被动环流衰减越慢 → 比值越高
- **次数不是门槛，次数只是结构演化经过的时间**（境界文档 §4.4 原样成立）
- 筑基判据五项不变；变化的是 SelfSustain 第一次有了真实来源

## J6 因果链核对（境界文档 §4.1 逐环对应）

```text
反复安全运行 Sequence
  → Openness/Adaptation/Conductance 上升（现有机制）
  → Loss 下降 + 途经纳气维持梯度（J2/J3）
  → 松手后被动环流衰减变慢（J2）
  → SelfSustain 上升（J5）
  → 结构跨过相变条件 → SELF_SUSTAINING_LOOP（现有判据）
```

## J7 测试计划

```text
T-J1 死锁回归：脚本分身全程"行环 N 轮 → 全松手"，SelfSustain 随 N 单调上升
     （旧引擎恒 0——本测试在修复前必须 FAIL）
T-J2 无环不自持：砍掉一条环边成树，松手后流归零（主环物理必然性）
T-J3 假自持拒绝：Protocol Runtime 持续运行时 ControlActive=true，
     不得计入 SelfSustain 分子
T-J4 守恒扩项：ΔQi 恒等式纳入 entrained 项，逐 tick 零残差
T-J5 金手指复核：新增 K_passive/K_entrain 在无被动场景（无压差/无流）下恒等 1 路径，
     原值断言预期不变；若变，逐项归因
T-J6 里程碑链：M1/M2/M3 判定改为由真实松手流程触发（移除人工喂样口径的测试依赖）
```

## J8 参数（全部 [v0.1 参数]，cultmc 标定复用）

```text
K_passive    被动输运强度（初值 0.3 量级）
K_entrain    途经纳气系数（初值 0.05 量级——远小于主动纳气）
W2 长度      松手观察窗（初值 30 tick）
```

标定命题（cultmc 新增/改造）：

```text
P7 衰减-结构关系：Conductance/Adaptation 越高，松手衰减时间常数越长（单调）
P8 环必要性：同参数下主环 vs 断环（树）的松手均流比（环显著更高）
P9 筑基可达性：完整游戏流程（引气→炼气→反复行环→松手）中，
   SelfSustain 跨 0.85 的 seed 占比 ≥ 某阈值——这是 P0 死锁修复的总验收
```

## J9 落地改动清单

```text
cult/tick.go      被动输运项 + 途经纳气项 + ControlActive 重定义 + 守恒扩项
cult/params.go    K_passive/K_entrain/W2
ctmatch/foundation.go  SelfSustain 采样改 J5 口径；RegisterLoop 时机复核
                   （Established 后注册 Loop 不变，但筑基判定不再依赖它）
ctmatch/cultspike 里程碑流程与 cultmc 场景改造（T-J6/P7-P9）
设计稿/前端        无改动（松手交互已有；「行至半途松开心念」卡片天然对上）
```

## J10 风险

- R1 被动流可能让"顺流白嫖"——不推也有流，降低炼气期操作感 → K_passive 初值
  取小，且被动流不受控、会均衡化，玩家仍需主动调度；标定 P7 观察
- R2 途经纳气使总量虚增 → K_entrain 小 + BackPressure 抑制 + P1 复测
- R3 SelfSustain 窗口口径变化影响既有筑基测试喂样 → T-J6 全链重验
