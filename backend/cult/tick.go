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
}

// World 是一次修炼仿真的全部状态。
type World struct {
	Character Character
	Env       Environment

	// NoiseSeed 是控制误差 ε 的确定性噪声种子（stage_3 §8.2）。
	NoiseSeed uint64

	// Tick 是已完成的 Tick 数。
	Tick int64

	// Ops 是调用方为下一 Tick 设置的操作。
	Ops Ops

	// Last 是最近一次 Advance 的报告；尚未推进时为 nil。
	Last *TickReport
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
	return &World{Character: char, Env: env, NoiseSeed: 1}
}

// ---------------------------------------------------------------------------
// Tick 报告（供测试与上层观察；不进入下一 Tick 的运算）
// ---------------------------------------------------------------------------

// TransferReport 是一条经脉本 Tick 的传输明细（§8-9）。
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
	Qi              [ElementCount]int64
	Activity        [ElementCount]int64
	Intake          [ElementCount]int64
}

// TickReport 是一次 Advance 的完整报告。
type TickReport struct {
	Tick      int64
	Transfers []TransferReport
	Points    []AcupointReport
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

	// §8-9 灵气传输与损耗。
	rep.Transfers = runTransfers(w)

	// §10 相生转化。
	runSheng(w)

	// §11 相克压活性。
	runKe(w)

	// §12 活性恢复。
	for i := 0; i < n; i++ {
		a := &w.Character.BodyGraph.Acupoints[i]
		for e := 0; e < ElementCount; e++ {
			a.Activity[e] += fpMul(ActivityRecoveryRate, NaturalActivity-a.Activity[e])
		}
	}

	// §13 逸散。
	for i := 0; i < n; i++ {
		a := &w.Character.BodyGraph.Acupoints[i]
		keep := One - a.Retention
		for e := 0; e < ElementCount; e++ {
			d := fpMul(fpMul(fpMul(a.Qi[e], BaseDiffuseRate), keep), a.Activity[e])
			a.Qi[e] -= d
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

		// §17 Strain。
		r.TransformHeat = fpMul(fpClamp(fpDiv(a.heatAccum, denom), 0, One), TransformHeatMax)
		overload := r.LoadRatio - One
		if overload < 0 {
			overload = 0
		}
		affStrain := One - r.Compatibility
		if affStrain < 0 {
			affStrain = 0
		}
		r.Strain = fpMul(WOverload, fpMul(overload, overload)) +
			fpMul(WAffinity, fpMul(affStrain, affStrain)) +
			fpMul(WShock, fpMul(r.FlowShock, r.FlowShock)) +
			r.TransformHeat

		// §18 Stability。
		earthRatio := int64(0)
		if r.Load > 0 {
			earthRatio = fpDiv(a.Qi[Earth], r.Load)
		}
		earthStab := One + fpMul(fpMul(earthRatio, a.Activity[Earth]), EarthStabilizationBonus)
		recovery := fpMul(fpMul(BaseRecovery, One-a.Damage), earthStab)
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

// runTransfers 实现 §8-9。
func runTransfers(w *World) []TransferReport {
	g := &w.Character.BodyGraph
	reports := make([]TransferReport, 0, len(w.Ops.Requests))
	controlErrMax := fpMul(ControlErrorBase, One-w.Character.Control)

	for _, req := range w.Ops.Requests {
		if req.Channel < 0 || req.Channel >= len(g.Channels) {
			continue
		}
		ch := &g.Channels[req.Channel]
		if ch.From < 0 || ch.From >= len(g.Acupoints) || ch.To < 0 || ch.To >= len(g.Acupoints) {
			continue
		}
		src := &g.Acupoints[ch.From]
		dst := &g.Acupoints[ch.To]
		tr := TransferReport{Channel: req.Channel, From: ch.From, To: ch.To}

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
			eps := noiseSigned(w.NoiseSeed, w.Tick, req.Channel, e) * controlErrMax / 1000
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
			src.elemFlowAccum[e] += actual
			dst.elemFlowAccum[e] += delivered
			src.flowAccum += actual
			dst.flowAccum += delivered
			ch.flowAccum += actual

			tr.Actual[e] = actual
			tr.Delivered[e] = delivered
			tr.Loss[e] = loss
		}
		reports = append(reports, tr)
	}
	return reports
}

// runSheng 实现 §10（快照语义：所有转化量按本阶段开始时的 Qi 计算）。
func runSheng(w *World) {
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
			if transform <= 0 {
				continue
			}
			gain := fpMul(transform, ShengEfficiency)
			a.Qi[src] -= transform
			a.Qi[dst] += gain
			a.heatAccum += transform - gain // §10.1 剩余 → 热
		}
	}
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
