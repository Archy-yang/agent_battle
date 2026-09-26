package cult

// tick.go —— 每 Tick 的确定性结算（stage_3 §7-24）。
//
// 结算顺序固定：
//
//	§7  灵气吸收（BackPressure 用上一 Tick 吸收后负载比，§47→§48 链）
//	§8  灵气传输（传导/容量/控制误差 ε）
//	§9  传输损耗（50% Diffuse、50% 热 → 目标穴位 LocalStrain）
//	§10 相生转化（快照语义：本阶段所有转化量按阶段开始时的 Qi 计算）
//	§11 相克压活性（固定顺序、活值语义）
//	§12 活性恢复
//	§13 逸散
//	§14 压力（本 Tick 结束后负载）
//	§15 FlowShock
//	§16 Compatibility
//	§17 Strain
//	§18 Stability
//	§19 Damage
//	§21 自然恢复
//	§22 Adaptation
//	§23 Openness 成长
//	§24 经脉 Adaptation
//
// 全程定点整数，无浮点；同 seed 同 Ops 流重演逐位一致。

// Ops 是调用方（Runtime / 测试）交给引擎的本 Tick 操作。
// 引擎不解释意图——它只知道每个穴位以多大强度吸收、每条经脉请求多少流量。
type Ops struct {
	// Intensity[i] 是穴位 i 的功法强度 ∈ (0,1000]；0 表示本 Tick 不吸收。
	// 长度必须与穴位数一致（nil 视为全 0）。
	Intensity []int64
	// Requests 是本 Tick 的经脉流量请求，按给出顺序结算。
	Requests []FlowRequest
}

// FlowRequest 是一条经脉上按五行请求的流量（量，千分位）。
type FlowRequest struct {
	// Channel 是经脉在 BodyGraph.Channels 中的下标。
	Channel int
	// Amount[e] 是对元素 e 的请求流量；0 表示不请求。
	Amount [ElementCount]int64
	// Reverse 标记逆流传输（修订包 A2）：沿天然向反向行走。
	// 唯有核点（Capacity 最大的穴位，A1 v0.2 核点判据——丹田画像，B1）
	// 可发起；其余一律被引擎拒绝。
	Reverse bool
}

// World 是一次修炼仿真的全部状态。
type World struct {
	Character Character
	Env       Environment

	// RuleVersion 是产生本状态的底层规则版本（宪法 §18；cult.RuleVersion）。
	RuleVersion string

	// NoiseSeed 是控制误差 ε 的确定性噪声种子（stage_3 §8.2）。
	NoiseSeed uint64

	// Tick 是已完成的 Tick 数。
	Tick int64

	// Ops 是调用方为下一 Tick 设置的操作。
	Ops Ops

	// Last 是最近一次 Advance 的报告；尚未推进时为 nil。
	Last *TickReport

	// sensory 是感知层的滞回状态存档（境界文档 §14-15；percept.go）：
	// 各穴位的滚动窗口、滞回档位与档位时长。私有中间态——不进入任何
	// 运算、不被序列化（encoding/json 只编导出字段），但同 seed 重放
	// 时随相同的观察序列逐位一致（DeepEqual 测试锁死）。
	sensory *sensoryState
}

// NewWorld 组装一个最小世界：指定角色与环境，噪声种子默认 1。
//
// 归一化：适应字段（Acupoint.Adaptation、Channel.ChannelAdaptation）
// 若为 0 视作未初始化，重置为 1.0（微标度）——0 亲和会使传导/转化
// 全部失效，不属于合法初值。Openness=0、Activity=0 是合法状态，不动。
func NewWorld(char Character, env Environment) *World {
	for i := range char.BodyGraph.Acupoints {
		a := &char.BodyGraph.Acupoints[i]
		for e := 0; e < ElementCount; e++ {
			if a.Adaptation[e] == 0 {
				a.Adaptation[e] = MicroOne
			}
		}
	}
	for c := range char.BodyGraph.Channels {
		if char.BodyGraph.Channels[c].ChannelAdaptation == 0 {
			char.BodyGraph.Channels[c].ChannelAdaptation = MicroOne
		}
	}
	return &World{Character: char, Env: env, NoiseSeed: 1, RuleVersion: RuleVersion}
}

// ---------------------------------------------------------------------------
// Tick 报告（供测试与上层观察；不进入下一 Tick 的运算）
// ---------------------------------------------------------------------------

// TransferReport 是一条经脉本 Tick 的传输明细（§8-9 + 修订包 A2）。
type TransferReport struct {
	Channel     int
	From        int
	To          int
	Conductance [ElementCount]int64
	Capacity    [ElementCount]int64
	Requested   [ElementCount]int64
	Actual      [ElementCount]int64
	Delivered   [ElementCount]int64
	Loss        [ElementCount]int64

	// Reverse 标记本条为逆流传输（A2）：Actual 沿天然向反向行走
	// （从 To 端流向 From 端），From/To 仍为经脉天然端点。
	Reverse bool
	// Rejected 标记逆流请求被引擎拒绝（A2：发起者不是核点），
	// 此时无任何流量发生。
	Rejected bool
	// WorkCost[e] 是逆流做功代价（从发起汇点按元素扣除；A2）。
	WorkCost [ElementCount]int64

	// Passive[e] 是本条经脉本 Tick 的被动输运实际流量（Part J J2：压差
	// 驱动，沿天然向）。被动流走独立报告条目（无主动请求的经脉也会产生
	// 条目），与主动流共用 §9 损耗管线（守恒/损耗/炼化筛选取叠加后的
	// 实际流）。被动条目上 Actual[e] == Passive[e]（统一"实际流"消费口径：
	// 守恒、筑基采样、回执汇总都直接读 Actual）；Passive 字段单独归因，
	// 供 SelfSustain 测量区分"推动之流"与"自走之流"。
	Passive [ElementCount]int64
}

// AcupointReport 是一个穴位本 Tick 结束时的诊断快照。
type AcupointReport struct {
	Load            int64
	LoadRatio       int64
	Pressure        int64
	FlowShock       int64
	Compatibility   int64
	Strain          int64
	TransformHeat   int64
	Stability       int64
	Damage          int64
	CurrentFlow     int64
	BackPressure    int64 // 本 Tick 吸收时使用的反压
	PostIntakeLoad  int64 // 吸收后立即测得的负载（§47）
	PostIntakeRatio int64 // 吸收后负载比（§47，也是下一 Tick 反压的输入）
	ThermalLoad     int64 // 热负荷累积（修订包 A2/B4；散热衰减后含本 Tick 新热）
	Qi              [ElementCount]int64
	Activity        [ElementCount]int64
	Intake          [ElementCount]int64 // §7 环境吸收（主动纳入；途经纳气另列 Entrained）
	Entrained       [ElementCount]int64 // Part J J3 途经纳气（环境纳入，随实际流量逐穴计量）
}

// Conservation 是一次 Advance 的守恒归账（修订包 A3：任何总量变化必须
// 逐项可归因，无凭空增益）。恒等式（精确整数，逐项与运算用同一舍入值）：
//
//	QiAfter - QiBefore == Intake - Diffuse - TransferLoss - ShengLoss
//	                    - ReverseWork + Entrained
//
// 去向标注：Diffuse 与 TransferLoss 的一半 → 环境；TransferLoss 另一半、
// ShengLoss、ReverseWork → 热（穴位 heatAccum / ThermalLoad）。
// Entrained 是 Part J J3 途经纳气（来源=环境，随实际流量逐穴计量）。
type Conservation struct {
	QiBefore     int64 // Tick 开始时 Σ 穴位储气
	QiAfter      int64 // Tick 结束时 Σ 穴位储气
	Intake       int64 // Σ §7 环境输入（逐穴计量）
	Diffuse      int64 // Σ §13 逸散（去向=环境）
	TransferLoss int64 // Σ §9 传输损耗
	ShengLoss    int64 // Σ §10 转化损耗（去向=热）
	ReverseWork  int64 // Σ 逆流做功代价（A2，去向=热）
	Entrained    int64 // Σ 途经纳气（Part J J3，来源=环境）
}

// TickReport 是一次 Advance 的完整报告。
type TickReport struct {
	Tick         int64
	Transfers    []TransferReport
	Points       []AcupointReport
	Conservation Conservation
}

// ---------------------------------------------------------------------------
// 入口
// ---------------------------------------------------------------------------

// Advance 推进一个 Tick 并返回报告。纯确定性：同状态同 Ops 必得同结果。
func Advance(w *World) *TickReport {
	n := len(w.Character.BodyGraph.Acupoints)
	rep := &TickReport{Tick: w.Tick}
	w.Tick++

	backPressure := make([]int64, n)
	intake := make([][ElementCount]int64, n)
	postLoad := make([]int64, n)
	postRatio := make([]int64, n)

	// Refinement 导出量快照（修订包 D2：绝不落存储，从本 Tick 开始时的
	// 气构成与身体状态重算）。本 Tick 内所有 D4 乘子使用同一快照——
	// Refinement=0 时全部乘子 = 1，与旧公式行为一致。
	refines := make([]int64, n)
	loadSum, weightedRefine := int64(0), int64(0)
	for i := 0; i < n; i++ {
		a := &w.Character.BodyGraph.Acupoints[i]
		refines[i] = refineOf(a, &w.Character)
		ld := a.load()
		loadSum += ld
		weightedRefine += fpMul(refines[i], ld)
	}
	refineAvg := int64(0)
	if loadSum > 0 {
		refineAvg = fpDiv(weightedRefine, loadSum) // 负载加权的平均炼化度（D4-2 口径）
	}

	// 守恒归账（修订包 A3）。
	cons := &rep.Conservation
	for i := 0; i < n; i++ {
		cons.QiBefore += w.Character.BodyGraph.Acupoints[i].load()
	}

	// §7 BackPressure：由上一 Tick 吸收后的负载比决定（§47→§48 链式定义）。
	for i := 0; i < n; i++ {
		a := &w.Character.BodyGraph.Acupoints[i]
		lr2 := fpMul(a.lastLoadRatioMilli, a.lastLoadRatioMilli)
		backPressure[i] = fpDiv(One, One+lr2)
	}

	// §7 灵气吸收。
	for i := 0; i < n; i++ {
		a := &w.Character.BodyGraph.Acupoints[i]
		intensity := int64(0)
		if i < len(w.Ops.Intensity) {
			intensity = w.Ops.Intensity[i]
		}
		if intensity <= 0 {
			continue
		}
		effOpen := a.effectiveOpenness()
		for e := 0; e < ElementCount; e++ {
			actFactor := 500 + fpMul(500, w.Env.Activity[e]) // §7.2
			v := fpMul(w.Env.Qi[e], w.Character.Affinity[e])
			v = fpMul(v, a.Affinity[e])
			v = fpMul(v, effOpen)
			v = fpMul(v, intensity)
			v = fpMul(v, backPressure[i])
			v = fpMul(v, actFactor)
			if v <= 0 {
				continue
			}
			a.Qi[e] += v
			// 吸收不计入 FlowShock/QiFlow（见 body.go flowAccum 注释）。
			intake[i][e] = v
			cons.Intake += v
		}
	}

	// §47 吸收后立即测得的负载与压力（黄金用例口径）。
	for i := 0; i < n; i++ {
		a := &w.Character.BodyGraph.Acupoints[i]
		postLoad[i] = a.load()
		r := loadRatio(a, postLoad[i])
		postRatio[i] = r
		a.lastLoadRatioMilli = r // 下一 Tick 反压的输入
	}

	// §8-9 灵气传输与损耗（含修订包 A2 逆流做功、Part J 被动输运与途经纳气）。
	var entrainedArr [][ElementCount]int64
	rep.Transfers, entrainedArr = runTransfers(w, refines, postRatio)
	for _, tr := range rep.Transfers {
		for e := 0; e < ElementCount; e++ {
			cons.TransferLoss += tr.Loss[e]
			cons.ReverseWork += tr.WorkCost[e]
		}
	}
	// Part J J3：途经纳气归账（来源=环境，单独成项）。报告逐穴单列
	// Entrained（不并入 §7 Intake），守恒归因走 cons.Entrained。
	for i := 0; i < n; i++ {
		for e := 0; e < ElementCount; e++ {
			cons.Entrained += entrainedArr[i][e]
		}
	}

	// §10 相生转化。
	cons.ShengLoss = runSheng(w, refines)

	// §11 相克压活性。
	runKe(w)

	// §12 活性恢复。
	for i := 0; i < n; i++ {
		a := &w.Character.BodyGraph.Acupoints[i]
		for e := 0; e < ElementCount; e++ {
			a.Activity[e] += fpMul(ActivityRecoveryRate, NaturalActivity-a.Activity[e])
		}
	}

	// §13 逸散（修订包 D3：筛选机制——穴位按亲和"留与散"，
	// Retention_effective[i,e] = Retention[i] × AcupointAffinity[i,e]；
	// 亲和低的成分按比例逸散，去向=环境，计入守恒账。
	// D4-1：逸散 × (1 - Refine×KRefine)，Refine=0 时乘子=1）。
	for i := 0; i < n; i++ {
		a := &w.Character.BodyGraph.Acupoints[i]
		for e := 0; e < ElementCount; e++ {
			retEff := fpClamp(fpMul(a.Retention, a.refineAffinity(e)), 0, One)
			d := fpMul(fpMul(fpMul(a.Qi[e], BaseDiffuseRate), One-retEff), a.Activity[e])
			d = fpMul(d, One-fpMul(refines[i], KRefine))
			a.Qi[e] -= d
			cons.Diffuse += d
		}
	}

	// §14-§24 逐穴位状态结算。
	rep.Points = make([]AcupointReport, n)
	for i := 0; i < n; i++ {
		a := &w.Character.BodyGraph.Acupoints[i]
		r := &rep.Points[i]
		r.BackPressure = backPressure[i]
		r.PostIntakeLoad = postLoad[i]
		r.PostIntakeRatio = postRatio[i]
		for e := 0; e < ElementCount; e++ {
			r.Intake[e] = intake[i][e]
			r.Entrained[e] = entrainedArr[i][e]
		}

		// §14 压力。
		r.Load = a.load()
		r.LoadRatio = loadRatio(a, r.Load)
		r.Pressure = fpMul(r.LoadRatio, r.LoadRatio)

		// §15 FlowShock。
		effTp := a.effectiveThroughput()
		denom := effTp
		if denom < ShockEpsilon {
			denom = ShockEpsilon
		}
		r.CurrentFlow = a.flowAccum
		r.FlowShock = fpDiv(abs64(r.CurrentFlow-a.PreviousFlow), denom)

		// §16 Compatibility。
		r.Compatibility = compatibility(a, r.Load)

		// §17 Strain（D4-4：FlowShock_effective × (1 - Refine×KShock)，
		// 炼化气更抗冲击；报告中的 FlowShock 保持原值）。
		r.TransformHeat = fpMul(fpClamp(fpDiv(a.heatAccum, denom), 0, One), TransformHeatMax)
		overload := r.LoadRatio - One
		if overload < 0 {
			overload = 0
		}
		affStrain := One - r.Compatibility
		if affStrain < 0 {
			affStrain = 0
		}
		shockEff := fpMul(r.FlowShock, One-fpMul(refines[i], KShock))
		r.Strain = fpMul(WOverload, fpMul(overload, overload)) +
			fpMul(WAffinity, fpMul(affStrain, affStrain)) +
			fpMul(WShock, fpMul(shockEff, shockEff)) +
			r.TransformHeat

		// §18 Stability（D4-2：稳定性恢复 × (1 + Refine_avg×KStab)）。
		earthRatio := int64(0)
		if r.Load > 0 {
			earthRatio = fpDiv(a.Qi[Earth], r.Load)
		}
		earthStab := One + fpMul(fpMul(earthRatio, a.Activity[Earth]), EarthStabilizationBonus)
		recovery := fpMul(fpMul(BaseRecovery, One-a.Damage), earthStab)
		recovery = fpMul(recovery, One+fpMul(refineAvg, KStab))
		a.Stability = fpClamp(a.Stability+recovery-r.Strain, 0, One)
		r.Stability = a.Stability

		// §19 Damage。
		if a.Stability < DamageThreshold {
			gain := fpMul(DamageThreshold-a.Stability, DamageRate)
			a.Damage = fpClamp(a.Damage+gain, 0, One)
		}

		// §21 自然恢复。
		if r.Pressure < LowPressureThreshold && r.Strain < LowStrainThreshold {
			a.Damage = fpClamp(a.Damage-fpMul(BaseDamageRecovery, BodyRecoveryModifier), 0, One)
		}
		r.Damage = a.Damage

		// ThermalLoad（修订包 A2/B4）：旧值先按 HeatDissipation 散热衰减，
		// 再累加本 Tick 的转化损耗热（§9/§10 → heatAccum）与逆流做功热
		// （reverseHeatAccum）。只累积，不进任何公式（P5 标定项）。
		a.ThermalLoad = fpMul(a.ThermalLoad, One-HeatDissipation) + a.heatAccum + a.reverseHeatAccum
		r.ThermalLoad = a.ThermalLoad

		// §22 Adaptation。
		if a.Stability >= AdaptationSafeStability && r.Pressure <= AdaptationSafePressure {
			for e := 0; e < ElementCount; e++ {
				flow := a.elemFlowAccum[e]
				if flow <= MinimumEffectiveFlow {
					continue
				}
				sfr := fpDiv(flow, denom)
				if sfr > One {
					sfr = One
				}
				gap := AdaptationMax - int64(a.adaptationMilli(e))
				a.Adaptation[e] += AdaptationRate * sfr * gap / MicroOne
				if a.Adaptation[e] > AdaptationMax*1000 {
					a.Adaptation[e] = AdaptationMax * 1000
				}
			}
		}

		// §23 Openness 成长。
		if r.Pressure >= OpenSafePressureLo && r.Pressure <= OpenSafePressureHi && a.Stability >= OpenSafeStability {
			o := One - a.opennessMilli()
			if o > 0 {
				a.Openness += OpenRate * r.Pressure * o / MicroOne
				if a.Openness > MicroOne {
					a.Openness = MicroOne
				}
			}
		}

		// §15 的"上一 Tick 流量"滚动 + 清理本 Tick 累计。
		a.PreviousFlow = a.flowAccum
		a.flowAccum = 0
		a.heatAccum = 0
		a.reverseHeatAccum = 0
		a.elemFlowAccum = [ElementCount]int64{}

		r.Qi = a.Qi
		r.Activity = a.Activity
	}

	// §24 经脉 Adaptation。
	for c := range w.Character.BodyGraph.Channels {
		ch := &w.Character.BodyGraph.Channels[c]
		denom := ch.Throughput
		if denom < ShockEpsilon {
			denom = ShockEpsilon
		}
		sfr := fpDiv(ch.flowAccum, denom)
		if sfr > One {
			sfr = One
		}
		gap := ChannelAdaptMax - ch.ChannelAdaptation/1000
		ch.ChannelAdaptation += ChannelAdaptRate * sfr * gap / MicroOne
		if ch.ChannelAdaptation > ChannelAdaptMax*1000 {
			ch.ChannelAdaptation = ChannelAdaptMax * 1000
		}
		ch.flowAccum = 0
	}

	// 守恒归账收尾（修订包 A3）：Qi 在 §13 后不再变化，此处汇总。
	for i := 0; i < n; i++ {
		cons.QiAfter += w.Character.BodyGraph.Acupoints[i].load()
	}

	w.Last = rep
	return rep
}

// ---------------------------------------------------------------------------
// 阶段实现
// ---------------------------------------------------------------------------

// loadRatio 返回 Load/Capacity（千分位）；容量非正时视为 0。
func loadRatio(a *Acupoint, load int64) int64 {
	cap := a.effectiveCapacity()
	if cap <= 0 {
		return 0
	}
	return fpDiv(load, cap)
}

// CorePointOf 返回身体图的核点下标（修订包 A1 v0.2）：Capacity 最大的穴位。
//
// 结构判据而非类型枚举（宪法 §8）：丹田画像的特殊性——唯可发起逆流外推
// （A2）——全部落在其"Capacity 最高"的参数事实上，无 type 字段。
// 旧判据"纯汇点（无出向经脉）"在主环拓扑（A1 v0.2）下失效：丹田是环上的
// 泵，有出向边 丹→土。并列时取下标最小者，保证确定性。
// ctmatch 的协议编译/意图路径与本引擎核同用本函数，避免两份判据漂移。
func CorePointOf(g *BodyGraph) int {
	core := 0
	for i := 1; i < len(g.Acupoints); i++ {
		if g.Acupoints[i].Capacity > g.Acupoints[core].Capacity {
			core = i
		}
	}
	return core
}

// runTransfers 实现 §8-9（含修订包 A2 逆流做功、Part J 被动输运与途经纳气）。
//
// 逆流语义（A2/B1）：请求带 Reverse 标记时，Actual 沿经脉天然向反向行走
// （从 To 端流向 From 端）。唯有核点（Capacity 最大的穴位，A1 v0.2 核点
// 判据——B1 丹田画像，不设 type 字段——宪法 §8）可发起逆流；其余逆流请求
// 被拒绝（Rejected）——逆经脉而行，唯丹田能推得动，穴与穴之间不得逆流。
// 逆流在常规传输损耗之外另付做功代价：
//
//	WorkCost = Actual × BaseWork × (1 + LoadRatio_dst × KBack) × (1 - Conductance)
//	ReverseHeat = WorkCost × KHeat → 逆流目的地 ThermalLoad
//
// Part J J2 被动输运：全部主动请求结算完毕后，对每条经脉按天然气向追加
// 压差驱动的被动项（压差快照 = 本 Tick 吸收后的负载比，对全部经脉同源，
// 与经脉遍历顺序无关）：
//
//	passiveFlow = clamp(chanCap×KPassive×ΔP×Conductance   // 压差梯度项
//	                    + KInertia×inflow(源穴),          // R4 惯性接力项
//	                   0, min(chanCap×Conductance,        // §8.1 管线容量
//	                          源穴储气×KQiShare,           // 压而不泻封顶
//	                          管线余量))                   // 主动已占容量不计
//	ΔP = Pressure(a) - Pressure(b)（Pressure = LoadRatio²，吸收后口径）
//
// 被动项与主动请求叠加（J2），但只填同一 §8.1 管线的余量——总实际流
//（主动+被动）不超过管道容量，主动推力不被被动旁路放大（R1）。压差为负
// 则梯度项为零，惯性项沿天然向衰减延续（被动永不逆天然向）。惯性接力的
// 输入是"其他经脉流入源穴"的实际流量（上一 Tick 打底 + 本 Tick 送达 live
// 接力）；自环通道的自身回流不是来流，不计入（否则成做功无源的影子回声）。
// 纯压差项在闭环上会均衡化归零（环上压差快照同和为零，无泵不成环流），
// 惯性项即 R4 预案的 Persistent Flow State 第二阶段。被动流与主动流共用
// 同一 §9 损耗管线并同样触发途经纳气，使守恒、损耗、炼化筛选都作用于
// 叠加后的实际流；报告中以独立条目（Passive 字段）记账，与主动流可归因区分。
//
// Part J J3 途经纳气：每笔实际流量（主动/逆流/被动）在其接收端穴位卷入
// 环境气（来源=环境，守恒账 Entrained 项）：
//
//	entrained = F × Openness × EnvironmentQi × AcupointAffinity
//	            × KEntrain × (1 - LoadRatio)
//
// 返回传输报告与逐穴途经纳气量。
func runTransfers(w *World, refines []int64, postRatio []int64) ([]TransferReport, [][ElementCount]int64) {
	g := &w.Character.BodyGraph
	reports := make([]TransferReport, 0, len(w.Ops.Requests)+len(g.Channels))
	entrained := make([][ElementCount]int64, len(g.Acupoints))
	chanFlow := make([][ElementCount]int64, len(g.Channels))  // 本 Tick 逐经脉逐元素实际流量（→ FlowPrev）
	activeFlow := make([][ElementCount]int64, len(g.Channels)) // 本 Tick 逐经脉逐元素主动流量（被动让行判据）
	controlErrMax := fpMul(ControlErrorBase, One-w.Character.Control)

	// 途经纳气（Part J J3）：一笔流量 F 在接收端穴位 i（元素 e）卷入的环境气。
	entrainInto := func(i int, a *Acupoint, e int, flow int64) {
		if flow <= 0 || w.Env.Qi[e] <= 0 {
			return
		}
		v := fpMul(flow, a.effectiveOpenness())
		v = fpMul(v, w.Env.Qi[e])
		v = fpMul(v, a.effectiveAffinity(e))
		v = fpMul(v, KEntrain)
		v = fpMul(v, One-postRatio[i])
		if v <= 0 {
			return
		}
		a.Qi[e] += v
		entrained[i][e] += v
	}

	// 核点判据（修订包 A1 v0.2）：逆流发起资格 = 发起者（逆流请求的 To 端）
	// 是核点（CorePointOf，见上）。
	core := CorePointOf(g)

	for _, req := range w.Ops.Requests {
		if req.Channel < 0 || req.Channel >= len(g.Channels) {
			continue
		}
		ch := &g.Channels[req.Channel]
		if ch.From < 0 || ch.From >= len(g.Acupoints) || ch.To < 0 || ch.To >= len(g.Acupoints) {
			continue
		}
		tr := TransferReport{Channel: req.Channel, From: ch.From, To: ch.To, Reverse: req.Reverse}

		// 端点选取：顺流 From→To；逆流 To→From（A2）。
		src, dst := &g.Acupoints[ch.From], &g.Acupoints[ch.To]
		srcIdx, dstIdx := ch.From, ch.To
		if req.Reverse {
			if ch.To != core {
				// 发起者不是核点——逆经脉而行，唯丹田能推得动。
				tr.Rejected = true
				reports = append(reports, tr)
				continue
			}
			src, dst = &g.Acupoints[ch.To], &g.Acupoints[ch.From]
			srcIdx, dstIdx = ch.To, ch.From
		}

		chanAdapt := ch.ChannelAdaptation / 1000
		baseCond := fpMul(ch.BaseConductance, chanAdapt) // §24
		condFactor := fpMul(fpMul(src.effectiveOpenness(), dst.effectiveOpenness()), One-ch.Damage)
		chanCap := ch.Throughput
		if s := src.effectiveThroughput(); s < chanCap {
			chanCap = s
		}
		if d := dst.effectiveThroughput(); d < chanCap {
			chanCap = d
		}

		// D4-5：控制误差幅值 × (1 - Refine_src×KControl)。src 已按流向取对
		// （顺流源 = From 端，逆流源 = To 端）。
		controlErrEff := fpMul(controlErrMax, One-fpMul(refines[srcIdx], KControl))

		// A2：逆流目的地的即时负载比（推入越满，做功越贵）。
		// workUnit = BaseWork × (1 + LoadRatio_dst × KBack)；(1 - Conductance) 逐元素乘。
		workUnit := int64(0)
		if req.Reverse {
			lrDst := loadRatio(dst, dst.load())
			workUnit = fpMul(BaseWork, One+fpMul(lrDst, KBack))
		}

		for e := 0; e < ElementCount; e++ {
			tr.Requested[e] = req.Amount[e]
			if req.Amount[e] <= 0 {
				continue
			}
			// §8 Conductance。
			aff := fpSqrt(fpMul(src.effectiveAffinity(e), dst.effectiveAffinity(e)))
			cond := fpMul(fpMul(baseCond, aff), condFactor)
			tr.Conductance[e] = cond

			// §8.1 ChannelCapacity。
			cap := fpMul(chanCap, cond)
			tr.Capacity[e] = cap

			// §8.2 控制误差 ε = deterministic_noise(seed, tick, channel, e)。
			eps := noiseSigned(w.NoiseSeed, w.Tick, req.Channel, e) * controlErrEff / 1000
			requested := fpMul(req.Amount[e], One+eps)

			actual := requested
			if src.Qi[e] < actual {
				actual = src.Qi[e]
			}
			if cap < actual {
				actual = cap
			}
			if actual <= 0 {
				continue
			}

			// §9 传输损耗。
			eff := fpClamp(cond, 0, One)
			delivered := fpMul(actual, eff)
			loss := actual - delivered

			src.Qi[e] -= actual
			dst.Qi[e] += delivered
			dst.heatAccum += fpMul(loss, One-DiffuseShare) // 50% → 热

			// Part J J3：接收端途经纳气（主动流同样卷入环境气）。
			entrainInto(dstIdx, dst, e, actual)

			// A2 逆流做功：代价从发起汇点按元素扣除（扣除量不超过存量），
			// 对应热加到逆流目的地的 ThermalLoad 累计。
			if req.Reverse {
				cost := fpMul(actual, fpMul(workUnit, One-eff))
				if cost > src.Qi[e] {
					cost = src.Qi[e]
				}
				if cost > 0 {
					src.Qi[e] -= cost
					dst.reverseHeatAccum += fpMul(cost, KHeat)
				}
				tr.WorkCost[e] = cost
			}

			src.elemFlowAccum[e] += actual
			dst.elemFlowAccum[e] += delivered
			src.flowAccum += actual
			dst.flowAccum += delivered
			ch.flowAccum += actual
			chanFlow[req.Channel][e] += actual
			activeFlow[req.Channel][e] += actual

			tr.Actual[e] = actual
			tr.Delivered[e] = delivered
			tr.Loss[e] = loss
		}
		reports = append(reports, tr)
	}

	// Part J J2：被动输运——压差驱动的背景环流（与主动请求叠加，见上注释）。
	// 压差快照取本 Tick 吸收后的负载比（postRatio），与遍历顺序无关。
	// 惯性项（R4）按"输送带"语义：经脉的惯性输入 = 流入其源穴位的实际
	// 流量（节点入流：上一 Tick 流量打底 + 本 Tick 已到达的送达量live累加
	// ——经脉按下标序结算，主环拓扑下恰好沿行进方向逐段接力，环流连续
	// 不间断）。流转受源穴储气份额封顶（见 KQiShare），不会同 Tick 级联
	// 放大。
	nodeIn := make([][ElementCount]int64, len(g.Acupoints))
	for c := range g.Channels {
		ch := &g.Channels[c]
		if ch.To < 0 || ch.To >= len(g.Acupoints) {
			continue
		}
		for e := 0; e < ElementCount; e++ {
			nodeIn[ch.To][e] += ch.FlowPrev[e]
		}
	}
	for c := range g.Channels {
		ch := &g.Channels[c]
		if ch.From < 0 || ch.From >= len(g.Acupoints) || ch.To < 0 || ch.To >= len(g.Acupoints) {
			continue
		}
		pa, pb := postRatio[ch.From], postRatio[ch.To]
		dP := fpMul(pa, pa) - fpMul(pb, pb)
		// 压差只决定梯度项是否驱动（下述元素级判断）；惯性项不受压差门槛，
		// 否则均衡压差下整段经脉被跳过、环流断裂（弛豫振荡的根源）。
		src, dst := &g.Acupoints[ch.From], &g.Acupoints[ch.To]

		chanAdapt := ch.ChannelAdaptation / 1000
		baseCond := fpMul(ch.BaseConductance, chanAdapt)
		condFactor := fpMul(fpMul(src.effectiveOpenness(), dst.effectiveOpenness()), One-ch.Damage)
		chanCap := ch.Throughput
		if s := src.effectiveThroughput(); s < chanCap {
			chanCap = s
		}
		if d := dst.effectiveThroughput(); d < chanCap {
			chanCap = d
		}

		var tr TransferReport
		tr.Channel, tr.From, tr.To = c, ch.From, ch.To
		for e := 0; e < ElementCount; e++ {
			aff := fpSqrt(fpMul(src.effectiveAffinity(e), dst.effectiveAffinity(e)))
			cond := fpMul(fpMul(baseCond, aff), condFactor)
			tr.Conductance[e] = cond
			eff := fpClamp(cond, 0, One)
			pipeCap := fpMul(chanCap, cond) // 与主动流 §8.1 同一管线容量上限

			// 主动占道、被动让行（R1）：被动项只填 §8.1 管线在本元素上的
			// 余量（pipeCap − 主动实际流），与主动流共用同一容量上限——
			// 被动环流是压差均衡化的背景物理（J2"主动 Request 照旧叠加"），
			// 但总实际流不得超过管道容量，否则主动推力被被动旁路放大
			//（白嫖环流）。上游满压、下游有需求时余量输运正是被动项的
			//本职：压差在请求存在的 Tick 同样被均衡化，局部过压不积聚。
			// 压差梯度项（J2）：仅压差为正时驱动（压差为负不逆天然向）。
			var gf int64
			if dP > 0 {
				gf = fpMul(fpMul(fpMul(chanCap, KPassive), dP), cond)
			}
			// 惯性项（R4 第二阶段）：流入源穴位的实际流量沿行进方向衰减
			// 延续（输送带语义，沿天然向；live 入流含本 Tick 上游送达）。
			// 注意惯性项不受压差门槛——均衡压差下环流靠它延续。
			// 自环不接力：惯性输入只计"其他经脉流入源穴"的来流；自环的
			// 自身回流是它自己流量的延续而非新来流，计入即成做功无源的
			// 影子回声（自环又无压差可言，纯添一份损耗流）。
			inertIn := nodeIn[ch.From][e]
			if ch.To == ch.From {
				inertIn -= ch.FlowPrev[e]
			}
			inert := fpMul(KInertia, inertIn)
			// 源穴储气份额封顶：被动流转每 Tick 不得超过源穴存量的
			// KQiShare——压而不泻，防止流转过冲振荡（流量远大于储气时
			// 压差逐 Tick 反转，形成弛豫振荡打穿 §15/§18）。
			qiShare := fpMul(src.Qi[e], KQiShare)
			capPf := pipeCap
			if qiShare < capPf {
				capPf = qiShare
			}
			// 管道余量（R1）：本元素本 Tick 已被主动流占用的容量，被动项
			// 不得复用——总实际流（主动+被动）≤ §8.1 管线容量。
			if spare := pipeCap - activeFlow[c][e]; spare < capPf {
				capPf = spare
				if capPf < 0 {
					capPf = 0
				}
			}
			pf := fpClamp(gf+inert, 0, capPf)
			if pf > src.Qi[e] {
				pf = src.Qi[e]
			}
			if pf <= 0 {
				continue
			}

			// §9 损耗管线（与主动流同管线）。
			delivered := fpMul(pf, eff)
			loss := pf - delivered

			src.Qi[e] -= pf
			dst.Qi[e] += delivered
			dst.heatAccum += fpMul(loss, One-DiffuseShare)
			nodeIn[ch.To][e] += delivered // live 入流：本 Tick 送达可即段接力

			// Part J J3：接收端途经纳气（被动流同样卷入环境气）。
			entrainInto(ch.To, dst, e, pf)

			src.elemFlowAccum[e] += pf
			dst.elemFlowAccum[e] += delivered
			src.flowAccum += pf
			dst.flowAccum += delivered
			ch.flowAccum += pf
			chanFlow[c][e] += pf

			tr.Passive[e] = pf
			tr.Actual[e] = pf // Actual=被动实际流量（统一"实际流"口径）；Passive 单独归因
			tr.Delivered[e] = delivered
			tr.Loss[e] = loss
		}
		if tr.Passive[Wood] > 0 || tr.Passive[Fire] > 0 || tr.Passive[Earth] > 0 ||
			tr.Passive[Metal] > 0 || tr.Passive[Water] > 0 {
			reports = append(reports, tr)
		}
	}

	// R4：持久流量状态落存（下一 Tick 惯性项的输入；随 World 状态演进，
	// 同 seed 重放逐位一致）。
	for c := range g.Channels {
		g.Channels[c].FlowPrev = chanFlow[c]
	}
	return reports, entrained
}

// runSheng 实现 §10（快照语义：所有转化量按本阶段开始时的 Qi 计算）。
// D4-3：转化量 × (1 + Refine×KTransform)，Refine=0 时乘子=1。
// 返回本 Tick 转化损耗总量（§10.1 剩余 → 热，守恒归账用）。
func runSheng(w *World, refines []int64) int64 {
	total := int64(0)
	for i := range w.Character.BodyGraph.Acupoints {
		a := &w.Character.BodyGraph.Acupoints[i]
		var snap [ElementCount]int64
		copy(snap[:], a.Qi[:])

		for _, src := range shengOrder {
			dst := ShengDst[src]
			if snap[src] <= 0 {
				continue
			}
			localAff := fpSqrt(fpMul(a.effectiveAffinity(src), a.effectiveAffinity(dst)))
			transform := fpMul(fpMul(fpMul(fpMul(snap[src], a.Activity[src]), BaseShengRate), localAff), TimeFactor)
			transform = fpMul(transform, One+fpMul(refines[i], KTransform)) // D4-3
			if transform <= 0 {
				continue
			}
			gain := fpMul(transform, ShengEfficiency)
			a.Qi[src] -= transform
			a.Qi[dst] += gain
			a.heatAccum += transform - gain // §10.1 剩余 → 热
			total += transform - gain
		}
	}
	return total
}

// runKe 实现 §11（固定顺序、活值语义）。
func runKe(w *World) {
	for i := range w.Character.BodyGraph.Acupoints {
		a := &w.Character.BodyGraph.Acupoints[i]
		for counter := 0; counter < ElementCount; counter++ {
			target := KeTarget[counter]
			cq, tq := a.Qi[counter], a.Qi[target]
			if cq <= 0 || tq <= 0 {
				continue
			}
			ratio := fpDiv(cq, cq+tq+KeEpsilon)
			localAff := fpSqrt(fpMul(a.effectiveAffinity(counter), a.effectiveAffinity(target)))
			suppression := fpMul(fpMul(fpMul(BaseKeRate, ratio), a.Activity[counter]), localAff)
			factor := fpClamp(One-suppression, 0, One)
			a.Activity[target] = fpMul(a.Activity[target], factor)
		}
	}
}

// compatibility 实现 §16。空穴位视为完全兼容（避免空载惩罚）。
func compatibility(a *Acupoint, total int64) int64 {
	if total <= 0 {
		return One
	}
	compat := int64(0)
	for e := 0; e < ElementCount; e++ {
		ratio := fpDiv(a.Qi[e], total)
		aff := a.effectiveAffinity(e)
		if aff > One {
			aff = One // §16：>1 的亲和优势不进入 Compatibility
		}
		if aff < 0 {
			aff = 0
		}
		compat += fpMul(ratio, aff)
	}
	return compat
}

// ---------------------------------------------------------------------------
// 确定性噪声（stage_3 §8.2）
// ---------------------------------------------------------------------------

// noiseSigned 返回 (seed, tick, a, b) 决定的 [-1000,1000] 整数。
// 本包自含 splitmix64 终结器混 Hash，不依赖任何全局 RNG，
// 也不与调用方的随机数状态互相干扰。
func noiseSigned(seed uint64, tick int64, a, b int) int64 {
	h := seed ^ 0x9e3779b97f4a7c15
	h = mixCult(h ^ uint64(tick))
	h = mixCult(h ^ uint64(uint32(a))<<32 ^ uint64(uint32(b)))
	h = mixCult(h)
	return int64(h%2001) - 1000
}

// mixCult 是 splitmix64 的三轮移位-乘法终结器。
func mixCult(z uint64) uint64 {
	z ^= z >> 33
	z *= 0xff51afd7ed558ccd
	z ^= z >> 33
	z *= 0xc4ceb9fe1a85ec53
	z ^= z >> 33
	return z
}

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
