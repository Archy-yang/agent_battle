package ctmatch

// foundation.go —— Loop 五量与筑基判定（stage_3 §31-39；叙事措辞对齐
// stage_2 §18-20）。
//
// 五量 Closure / RuleBalance / SelfSustain / DynamicStability /
// Internalization 以千分位定点整数在会话层滚动计算（观察窗 50 Tick）。
// 五量数值只进转录/调试视图（Metrics / LoopMetrics），绝不进 Agent
// 观察——Agent 只见到叙事事件（"触碰筑基门槛"→"筑基"）。
//
// v0.1 诚实口径（stage_3 §52：参数是实验值，公式关系不可变）：
//   - SelfSustain（§34）依赖"放开主动推动"的观察窗：会话层每 Tick 上报
//     本 Tick 是否有主动推动（吸收/引流 Ops）。整个窗口都在推动 → 无从
//     自持（零）；从未推动而气流仍在 → 完全自持。v0.1 引擎里流量只能由
//     Ops 驱动，真实自持要等 Loop 注册（Runtime 自推）后才可观测——
//     这正是 stage_3 §39 的次序，不提前造假。
//   - Internalization（§36）：v0.1 无阵法/丹药/法器等外物，外部贡献恒
//     为零，故恒为满值；器物篇引入外部供给时按 §36 展开。
//   - RuleBalance（§33）：能量失衡 = |Δ体内灵气总量| / 总量（滚动均值），
//     五行漂移 = 与窗首的五行占比差之和（滚动均值）。
//   - DynamicStability（§35）：流量/压力/五行/稳定性四序列的归一化方差
//     均值（var/mean²，千分位），1 减之。
//
// 判定（§37/§38）：候选阈值持续 30 Tick → 触碰筑基门槛（一次性事件）；
// 筑基阈值持续 100 Tick（且已触碰）→ 筑基，并把在跑的功法升格为
// AutonomousLoop（§39，Runtime 每 Tick 自推，零 LLM）。

import (
	"agentbattle/backend/cult"
)

// ---------------------------------------------------------------------------
// 事件种类与叙事词（stage_2 §20：不要一接近就宣布成功）
// ---------------------------------------------------------------------------

const (
	EventFoundationTouched     EventKind = "foundation_touched"     // 触碰筑基门槛
	EventFoundationEstablished EventKind = "foundation_established" // 筑基功成
	EventLoopRegistered        EventKind = "loop_registered"        // AutonomousLoop 注册（§39）
)

// 叙事词（进事件流与 Agent 可见转录；零数字）。
const (
	WordFoundationTouched     = "灵气行过周天并未散尽，复沿旧径自行流转，虽仍有起伏——此谓触碰筑基门槛"
	WordFoundationEstablished = "周天循环已能长期自行维持，不假外力——筑基功成"
)

// ---------------------------------------------------------------------------
// 每 Tick 样本（会话层从报告提取；测试可直接构造）
// ---------------------------------------------------------------------------

// FoundationSample 是一个 Tick 供给筑基判定的样本。字段为引擎真值的
// 会话层汇总，仅供内部计算与调试视图——绝不进 Agent 观察。
type FoundationSample struct {
	Tick          int64
	TopoClosure   int64                    // 0/1000：本 Tick 功法引导路径是否成环（stage_3 §32）
	Intake        int64                    // 本 Tick 自环境纳入的灵气总量
	Delivered     int64                    // 本 Tick 经脉送达的灵气总量
	ActualFlow    int64                    // 本 Tick 经脉实际流量的总量
	TotalQi       int64                    // 体内灵气总量
	MeanPressure  int64                    // 诸穴压力均值（千分位）
	MinStability  int64                    // 诸穴稳定性最小值（千分位）
	NatureRatio   [cult.ElementCount]int64 // 五行占比（×1000，合计≈1000）
	ControlActive bool                     // 本 Tick 是否有主动推动（吸收/引流 Ops）
}

// ---------------------------------------------------------------------------
// 阈值（stage_3 §37/§38；§52：调试值，须经模拟器校准）
// ---------------------------------------------------------------------------

const (
	CandClosure         = 800 // §37 Closure >= 0.80
	CandRuleBalance     = 800 // §37 RuleBalance >= 0.80
	CandSelfSustain     = 700 // §37 SelfSustain >= 0.70
	CandDynStability    = 800 // §37 DynamicStability >= 0.80
	CandInternalization = 800 // §37 Internalization >= 0.80
	CandMinStability    = 700 // §37 MinimumStability >= 0.70
	CandidateTicks      = 30  // §37 持续 30 Tick → 触碰筑基

	FoundClosure         = 900 // §38 Closure >= 0.90
	FoundRuleBalance     = 850 // §38 RuleBalance >= 0.85
	FoundSelfSustain     = 850 // §38 SelfSustain >= 0.85
	FoundDynStability    = 880 // §38 DynamicStability >= 0.88
	FoundInternalization = 900 // §38 Internalization >= 0.90
	FoundMinStability    = 750 // §38 MinimumStability >= 0.75
	StableTicks          = 100 // §38 持续 100 Tick → 筑基
)

const (
	foundationWindow = 50  // §35 观察窗 N=50 Tick
	WEnergy          = 600 // §33 W_energy 0.6
	WNature          = 400 // §33 W_nature 0.4
)

// ---------------------------------------------------------------------------
// LoopMetrics（调试视图；绝不进 Agent 观察）
// ---------------------------------------------------------------------------

// LoopMetrics 是五量与最低稳定性的当前值（千分位定点）。
type LoopMetrics struct {
	Closure          int64
	RuleBalance      int64
	SelfSustain      int64
	DynamicStability int64
	Internalization  int64
	MinStability     int64
}

// ---------------------------------------------------------------------------
// 判定器
// ---------------------------------------------------------------------------

// FoundationMonitor 滚动消费样本，产出候选/筑基判定。
type FoundationMonitor struct {
	samples []FoundationSample // 滚动窗口（最多 foundationWindow 个）

	candStreak   int // §37 条件连续满足的 Tick 数
	stableStreak int // §38 条件连续满足的 Tick 数
	Touched      bool
	Established  bool

	last LoopMetrics // 最近一次计算的五量（调试视图）
}

// NewFoundationMonitor 建判定器。
func NewFoundationMonitor() *FoundationMonitor { return &FoundationMonitor{} }

// Observe 收一个样本，做本 Tick 判定。touched/established 为 true 表示
// 本 Tick 恰好跨过对应门槛（一次性事件，由调用方转成叙事事件）。
func (m *FoundationMonitor) Observe(s FoundationSample) (touched, established bool) {
	m.samples = append(m.samples, s)
	if len(m.samples) > foundationWindow {
		m.samples = m.samples[len(m.samples)-foundationWindow:]
	}

	mets := m.compute(s)
	m.last = mets

	if m.candidateOK(mets) {
		m.candStreak++
	} else {
		m.candStreak = 0
	}
	if m.foundationOK(mets) {
		m.stableStreak++
	} else {
		m.stableStreak = 0
	}

	if !m.Touched && m.candStreak >= CandidateTicks {
		m.Touched = true
		touched = true
	}
	if m.Touched && !m.Established && m.stableStreak >= StableTicks {
		m.Established = true
		established = true
	}
	return touched, established
}

// Metrics 返回最近一次计算的五量（只进转录/调试视图）。
func (m *FoundationMonitor) Metrics() LoopMetrics { return m.last }

func (m *FoundationMonitor) candidateOK(mt LoopMetrics) bool {
	return mt.Closure >= CandClosure && mt.RuleBalance >= CandRuleBalance &&
		mt.SelfSustain >= CandSelfSustain && mt.DynamicStability >= CandDynStability &&
		mt.Internalization >= CandInternalization && mt.MinStability >= CandMinStability
}

func (m *FoundationMonitor) foundationOK(mt LoopMetrics) bool {
	return mt.Closure >= FoundClosure && mt.RuleBalance >= FoundRuleBalance &&
		mt.SelfSustain >= FoundSelfSustain && mt.DynamicStability >= FoundDynStability &&
		mt.Internalization >= FoundInternalization && mt.MinStability >= FoundMinStability
}

// compute 按观察窗计算五量（全部定点整数，确定性）。
func (m *FoundationMonitor) compute(cur FoundationSample) LoopMetrics {
	w := m.samples
	n := int64(len(w))
	mt := LoopMetrics{Internalization: One} // §36：v0.1 无外物，内源恒满

	// Closure（§32）= TopologyClosure × FlowClosure。
	// FlowClosure = min(送达/(纳入+实际推动), 1) 的滚动均值。
	var fcSum int64
	for _, s := range w {
		den := s.ActualFlow + s.Intake
		v := int64(0)
		if den > 0 {
			v = s.Delivered * One / den
			if v > One {
				v = One
			}
		}
		fcSum += v
	}
	flowClosure := divRound(fcSum, n)
	mt.Closure = fpMul(cur.TopoClosure, flowClosure)

	// RuleBalance（§33）= 1 − W_energy×EnergyImbalance − W_nature×NatureDrift。
	var eiSum, ndSum int64
	for i := range w {
		if i > 0 && w[i-1].TotalQi > 0 {
			d := w[i].TotalQi - w[i-1].TotalQi
			if d < 0 {
				d = -d
			}
			eiSum += d * One / w[i-1].TotalQi
		}
		if len(w) > 1 {
			for e := 0; e < cult.ElementCount; e++ {
				d := w[i].NatureRatio[e] - w[0].NatureRatio[e]
				if d < 0 {
					d = -d
				}
				ndSum += d
			}
		}
	}
	ei := divRound(eiSum, n) // ×1000 比率
	nd := divRound(ndSum, n) // Σ|Δ占比|（×1000）
	mt.RuleBalance = One - (ei*WEnergy+nd*WNature)/One
	if mt.RuleBalance < 0 {
		mt.RuleBalance = 0
	}

	// SelfSustain（§34）= 松手后均流 / 推动时均流。
	var pushSum, pushN, relSum, relN int64
	for _, s := range w {
		if s.ControlActive {
			pushSum += s.ActualFlow
			pushN++
		} else {
			relSum += s.ActualFlow
			relN++
		}
	}
	switch {
	case relN == 0:
		// 尚未放开过推动：无从自持（§34 需要松手观测）。
	case pushN == 0:
		// 全程没有主动推动而气流仍在——完全自持。
		if relSum > 0 {
			mt.SelfSustain = One
		}
	default:
		pushAvg := divRound(pushSum, pushN)
		relAvg := divRound(relSum, relN)
		if pushAvg > 0 {
			mt.SelfSustain = relAvg * One / pushAvg
			if mt.SelfSustain > One {
				mt.SelfSustain = One
			}
		}
	}

	// DynamicStability（§35）= 1 − 四序列归一化方差的均值。
	mt.DynamicStability = One - m.dynInstability()
	if mt.DynamicStability < 0 {
		mt.DynamicStability = 0
	}

	mt.MinStability = cur.MinStability
	return mt
}

// dynInstability 计算窗内流量/压力/五行/稳定性四序列的归一化方差均值。
func (m *FoundationMonitor) dynInstability() int64 {
	w := m.samples
	n := len(w)
	if n < 2 {
		return 0
	}
	flows := make([]int64, n)
	pressures := make([]int64, n)
	stabilities := make([]int64, n)
	natureScalars := make([]int64, n)
	var meanRatio [cult.ElementCount]int64
	for i, s := range w {
		flows[i] = s.ActualFlow
		pressures[i] = s.MeanPressure
		stabilities[i] = s.MinStability
		for e := 0; e < cult.ElementCount; e++ {
			meanRatio[e] += s.NatureRatio[e]
		}
	}
	for e := 0; e < cult.ElementCount; e++ {
		meanRatio[e] = divRound(meanRatio[e], int64(n))
	}
	for i, s := range w {
		var d int64
		for e := 0; e < cult.ElementCount; e++ {
			x := s.NatureRatio[e] - meanRatio[e]
			if x < 0 {
				x = -x
			}
			d += x
		}
		natureScalars[i] = d
	}
	di := (normVar(flows) + normVar(pressures) + normVar(natureScalars) + normVar(stabilities)) / 4
	if di > One {
		di = One
	}
	return di
}

// normVar 返回序列的归一化方差 var/mean²（×1000 比率，封顶 1000）。
func normVar(xs []int64) int64 {
	n := int64(len(xs))
	if n < 2 {
		return 0
	}
	var sum int64
	for _, x := range xs {
		sum += x
	}
	mean := divRound(sum, n)
	if mean <= 0 {
		for _, x := range xs {
			if x != 0 {
				return One // 均值为零却有起伏：波动记满
			}
		}
		return 0
	}
	var v int64
	for _, x := range xs {
		d := x - mean
		v += d * d
	}
	nv := divRound(v, n) * One / (mean * mean)
	if nv > One {
		nv = One
	}
	return nv
}

// divRound 四舍五入除法（b>0）。
func divRound(a, b int64) int64 {
	if b <= 0 {
		return 0
	}
	if a >= 0 {
		return (a + b/2) / b
	}
	return (a - b/2) / b
}

// ---------------------------------------------------------------------------
// 会话层接线
// ---------------------------------------------------------------------------

// topoClosure 判定一份 Protocol 的引导路径是否成环（stage_3 §32
// TopologyClosure：A→B→…→A 则满值，否则为零）。结果只有 0/1000，
// 与遍历顺序无关。
func topoClosure(p *Protocol) int64 {
	if p == nil {
		return 0
	}
	type edge struct{ from, to int }
	var edges []edge
	for _, st := range p.Steps {
		if st.Kind == StepGuide {
			edges = append(edges, edge{st.From, st.To})
		}
	}
	if len(edges) == 0 {
		return 0
	}
	adjOf := func(u int) []int {
		var out []int
		for _, e := range edges {
			if e.from == u {
				out = append(out, e.to)
			}
		}
		return out
	}
	color := map[int]int{} // 0 未访 1 在栈 2 完成
	var hasCycle bool
	var dfs func(u int)
	dfs = func(u int) {
		color[u] = 1
		for _, v := range adjOf(u) {
			switch color[v] {
			case 0:
				dfs(v)
			case 1:
				hasCycle = true
			}
		}
		color[u] = 2
	}
	var starts []int
	for _, e := range edges {
		if color[e.from] == 0 {
			starts = append(starts, e.from)
		}
	}
	for _, u := range starts {
		if !hasCycle {
			dfs(u)
		}
	}
	if hasCycle {
		return One
	}
	return 0
}

// buildFoundationSample 从本 Tick 报告提取样本（真值只在此处汇总为
// 内部计算输入，不外泄给 Agent）。
func (s *CultSession) buildFoundationSample(rep *cult.TickReport, controlActive bool) FoundationSample {
	sample := FoundationSample{Tick: rep.Tick, ControlActive: controlActive}
	if s.active != nil {
		sample.TopoClosure = topoClosure(s.active.proto)
	} else if s.lastProtocol != nil {
		sample.TopoClosure = topoClosure(s.lastProtocol)
	}
	n := int64(len(rep.Points))
	var pressSum int64
	minStab := int64(One)
	for i := range rep.Points {
		p := &rep.Points[i]
		for e := 0; e < cult.ElementCount; e++ {
			sample.Intake += p.Intake[e]
			sample.TotalQi += p.Qi[e]
		}
		pressSum += p.Pressure
		if p.Stability < minStab {
			minStab = p.Stability
		}
	}
	for ti := range rep.Transfers {
		tr := &rep.Transfers[ti]
		for e := 0; e < cult.ElementCount; e++ {
			sample.ActualFlow += tr.Actual[e]
			sample.Delivered += tr.Delivered[e]
		}
	}
	if n > 0 {
		sample.MeanPressure = divRound(pressSum, n)
	}
	// 五行占比：按穴累加每行 Qi，再折成占比（×1000）。
	if sample.TotalQi > 0 {
		var elemQi [cult.ElementCount]int64
		for i := range rep.Points {
			for e := 0; e < cult.ElementCount; e++ {
				elemQi[e] += rep.Points[i].Qi[e]
			}
		}
		for e := 0; e < cult.ElementCount; e++ {
			sample.NatureRatio[e] = elemQi[e] * One / sample.TotalQi
		}
	}
	sample.MinStability = minStab
	return sample
}

// observeFoundation 在每 Tick 结算后做筑基判定（§37/§38），并把跨门槛
// 转成叙事事件；筑基后将在跑的功法升格为 AutonomousLoop（§39）。
func (s *CultSession) observeFoundation(res *TickResult, controlActive bool) {
	if s.Foundation == nil || res.Report == nil {
		return
	}
	sample := s.buildFoundationSample(res.Report, controlActive)
	touched, _ := s.Foundation.Observe(sample)
	if touched {
		s.pushEvent(Event{Kind: EventFoundationTouched, Tick: s.World.Tick, Reason: WordFoundationTouched})
	}
	// 筑基事件只发一次（判定器状态是会话级事实，无论跨门槛发生在哪个
	// Tick——包括上层直接喂样本的情形）。
	if s.Foundation.Established && !s.establishedNoted {
		s.establishedNoted = true
		s.pushEvent(Event{Kind: EventFoundationEstablished, Tick: s.World.Tick, Reason: WordFoundationEstablished})
	}
	// §39：Engine 允许这个稳定结构注册成 AutonomousLoop。幂等：筑基后
	// 只注册一次；无可升格的功法（从未跑过 Runtime）则留待日后。
	if s.Foundation.Established && !s.loopEngaged && s.lastProtocol != nil {
		s.loopEngaged = true
		s.RegisterLoop(s.lastProtocol)
	}
}

// RegisterLoop 把一份 Protocol 升格为 AutonomousLoop（stage_3 §39）：
// Runtime 每 Tick 自推，不再受周天数上限约束，也不需要 LLM。
func (s *CultSession) RegisterLoop(p *Protocol) {
	if p == nil {
		return
	}
	s.active = &protocolState{proto: p, flowScale: One, loopMode: true}
	s.pushEvent(Event{
		Kind: EventLoopRegistered, Tick: s.World.Tick, Protocol: p.Name,
		Reason: "autonomous loop engaged; runtime self-drives the cycle every tick",
	})
}
