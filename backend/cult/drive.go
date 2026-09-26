package cult

// drive.go —— 统一驱动求解与做功台账（定稿
// docs/specs/2026-09-26-engine-drive-foundation-v1.md 阶段二「驱动的代价」；
// 公式来源：评审稿 docs/cultivation_autonomous_loop_design_v0.2.md
// §9 统一 DriveResolver、§10 驱动的能量代价与 Strain、§15 WorkLedger、
// §24.4-§24.6 数据结构示意）。
//
// 统一驱动求解的管线（定稿阶段二范围 2）：
//
//	请求驱动 → 校验驱动容量与可用能量 → 计算做功 → 产出废热与应变 → 实际驱动
//
// 本阶段接入该管线的驱动源只有手动驱动（玩家意图 / 协议运行，Ops.Requests）；
// 自主驱动（评审稿 §9.3，FeedbackCoupling → AutonomousCommand）属阶段三，
// 届时走同一管线（评审稿：Manual 与 Autonomous 不得走两套「魔法通道」）。
// 被动输运（Part J J2）是势差自然释放，无意识参与、不付做功（评审稿 §15：
// PassiveWork 不是凭空能量，也不进内化度分母），不经本求解。
//
// 做功的代价分层（评审稿 §10，定点整数化）：
//
//	顺流驱动：DriveWork = k_work × ActualDrive² × Δt / DriveEfficiency
//	逆势驱动：A2 公式结构原样（BaseWork × (1 + LoadRatio_dst × KBack)
//	          × (1 − Conductance)），归属迁移为求解器的逆势代价输出
//	          （定稿裁决①：不再是独立通道）
//	应变：    DriveStrain = k_over × OverDrive² + k_ramp × ΔDriveRatio²
//	          （只有超安全驱动比或加力突变才显著——温和稳定驱动耗能但低应变）
//
// 全程定点整数，无浮点；同 seed 重放逐位一致。

// WorkLedger 是一次 Advance 的做功台账（评审稿 §15/§24.6；定稿阶段二范围 4）。
// 每 Tick 记录被动/自主/手动三分做功归因与驱动的能量代价、环境流入、损耗。
// 台账只进 TickReport（供测试与上层审计），不回写世界状态。
type WorkLedger struct {
	// PassiveWork 是被动输运的实际流量合计（势差自然释放的有效做功，
	// 评审稿 §15：不代表身体控制，不进内化度分母）。
	PassiveWork int64
	// AutonomousWork 是自主驱动的实际驱动合计。本阶段恒零——自主驱动
	// （FeedbackCoupling → AutonomousCommand → DriveResolver）属阶段三。
	AutonomousWork int64
	// ManualWork 是手动驱动（玩家意图 / 协议运行）的实际驱动合计。
	ManualWork int64
	// DriveWork 是驱动的能量代价合计（守恒账 DriveWork 同源）：顺流做功
	// （§10.1 二次律）+ 逆势做功（A2）。去向=热。
	DriveWork int64
	// ExternalInput 是环境流入合计（§7 边界交换纳入总量 = 意识驱动
	// 增量 Intake + 被动基线 ExchangeIn，0.6.0 + Part J J3 途经纳气）。
	// 与守恒账 ExternalInput = cons.Intake + cons.ExchangeIn +
	// cons.Entrained 同源（评审稿 §14.4 能量平衡的环境侧口径）。
	ExternalInput int64
	// Generated 是内部生成（评审稿 §19 DeltaEnergy 的 Generated 项）。
	// 本阶段恒零：相生转化是元素互转（效率 0.9 的损耗已入 ShengLoss），
	// 无净生成；未来「Transform 释放可用能」类原语入此项。
	Generated int64
	// Loss 是损耗合计（传输损耗 + 逸散 + 转化损耗）。
	Loss int64
}

// DeriveInternalization 返回内化度（千分位；评审稿 §15）：
//
//	Internalization = AutonomousWork / max(AutonomousWork + ManualWork, ε)
//
// 0 = 完全依赖意识；1 = 无需持续外部控制。PassiveWork 不进分母（世界
// 自然势差不代表反馈控制）。派生量：只从台账现算，绝不落存储（与
// Refinement 同纪律，修订包 D2）。
func DeriveInternalization(led *WorkLedger) int64 {
	denom := led.AutonomousWork + led.ManualWork
	if denom <= 0 {
		return 0
	}
	return fpDiv(led.AutonomousWork, denom)
}

// driveWorkOf 计算一笔顺流驱动的做功代价（评审稿 §10.1 定点实现）：
//
//	DriveWork = k_work × ActualDrive² × Δt / DriveEfficiency（Δt = 1 Tick）
//
// ActualDrive 与返回值均为千分位量。k_work×F² 在千分位标度下 =
// fpMul(KDriveWork, fpMul(F, F))（fpMul(F,F) = F²/1000）。
func driveWorkOf(actual, efficiency int64) int64 {
	if actual <= 0 {
		return 0
	}
	return fpMul(KDriveWork, fpMul(actual, fpDiv(actual, efficiency)))
}

// driveEnergyFit 把请求驱动量按「驱动量 + 做功 ≤ 可用能量」收敛（评审稿
// §9.4 校验可用能量 / §26.4 无能源则实际驱动衰减为零）。workOf 必须对
// actual 单调不减（两处调用点的做功公式均满足）。收敛一步修正：
//
//	work₀ = workOf(actual)；若 actual + work₀ 超出可用能量，
//	actual' = max(0, available − work₀) ≤ actual
//	⇒ actual' + workOf(actual') = available − work₀ + workOf(actual')
//	                                 ≤ available（workOf 单调不减）
//
// 衰减而非精确求解：世界语义是「推不动的部分自然泄掉」，不承诺最优。
func driveEnergyFit(actual, available int64, workOf func(int64) int64) int64 {
	if actual <= 0 {
		return 0
	}
	work := workOf(actual)
	if actual+work <= available {
		return actual
	}
	fit := available - work
	if fit < 0 {
		fit = 0
	}
	if fit > actual {
		fit = actual
	}
	return fit
}

// driveStrainOf 计算一路驱动的应变分量（评审稿 §10.2 定点实现）：
//
//	OverDrive = max(0, DriveRatio − SafeDriveRatio)
//	Strain    = k_over × OverDrive² + k_ramp × max(0, ΔRatio)²
//
// ΔRatio 只计加力方向：骤然松手的流量回落已由 §15 FlowShock 管辖，
// 不双重惩罚（世界语义：突变加力才打稳定，松手是「撤力」不是「用力」）。
// ratio / prev 均为千分位比。
func driveStrainOf(ratio, prev int64) int64 {
	over := ratio - SafeDriveRatio
	if over < 0 {
		over = 0
	}
	s := fpMul(KDriveOver, fpMul(over, over))
	d := ratio - prev
	if d > 0 {
		s += fpMul(KDriveRamp, fpMul(d, d))
	}
	return s
}
