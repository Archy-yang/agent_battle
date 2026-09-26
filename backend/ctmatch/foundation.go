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
//   - SelfSustain（§34，Part J J5 测量协议 + §16 真实松手语义；判定口径
//     foundation-metrics-v2，2026-09-26 主会话裁决）：推动窗口 W1 = 推动相
//     （ControlActive）的手动驱动均流；松手窗口 W2 = 其后连续
//     FoundationReleaseWindow tick 全无请求的均流（非控制期有组织的实际流
//     = 被动 + 惯性 + 自主——非控制 Tick 上 ManualWork 恒 0，全部实际流
//     只能来自这三者）。SelfSustain = W2均流 / W1均流：控制所维持的流量
//     中，松手后仍能自存多少。
//     ControlActive（2026-09-26 主会话裁决）= 本 Tick 台账存在手动驱动做功
//     （ManualWork > 0，玩家意图与协议的主动操作都表现为手动驱动）或存在
//     主动请求。自主驱动与被动流永不计入控制——已内化的自主环在无人推动
//     时继续流，恰恰是自持测量的真实对象；协议筑基后转为监控/守卫/异常
//     升级（评审稿 §21.3），不再逐节拍驱动已内化段（protocol.go 移交门），
//     若协议仍对已内化转移发主动请求，那段流就是手动流、计入控制，
//     自持测量不受污染。
//     直接构造 FoundationSample 喂样本的口径仅测试兼容用（deprecated）：
//     生产判定路径的样本一律来自 buildFoundationSample（真实 tick 流）。
//   - Closure（§17 闭合分层）：Closure = TopologyClosure × FlowClosure。
//     FlowClosure（§17.2）= 回流 / 初发流——引导环首出边的实际流量中，
//     沿环末回边送达起点的份额（滚动均值）。图上闭合但只回小部分不算
//     高质量闭环（§17.3）。
//   - Internalization（§36）：v0.1 无阵法/丹药/法器等外物，外部贡献恒
//     为零，故恒为满值；器物篇引入外部供给时按 §36 展开。
//   - RuleBalance（§33/§19）：能量失衡 = |Δ体内灵气总量| / 总量（滚动
//     均值；§19 EnergyImbalance，Generated=0 时与守恒账逐项口径一致），
//     五行漂移 = 与窗首的五行占比差之和（滚动均值）。
//   - DynamicStability（§35/§18）：流量/压力/五行/稳定性四序列的归一化
//     方差均值（var/mean²，千分位），1 减之。高自持但剧烈振荡不算稳定。
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
//
// 生产路径：样本一律由 buildFoundationSample 从真实 tick 报告生成。
// 直接构造本结构人工喂样本仅供测试兼容（deprecated）——筑基判定链路
// （T-J6）不得依赖人工喂样。
type FoundationSample struct {
	Tick          int64
	TopoClosure   int64                    // 0/1000：本 Tick 功法引导路径是否成环（stage_3 §32）
	Intake        int64                    // 本 Tick 自环境纳入的灵气总量（含 Part J 途经纳气）
	Delivered     int64                    // 本 Tick 经脉送达的灵气总量
	ActualFlow    int64                    // 本 Tick 经脉实际流量的总量（主动 + 被动）
	ManualFlow    int64                    // 本 Tick 手动驱动实际流量合计（台账 ManualWork；SelfSustain 分母的推动窗输入）
	CycleOut      int64                    // 本 Tick 引导环首出边的实际流量（§17.2 初发流；未成环为 0）
	CycleBack     int64                    // 本 Tick 沿环末回边送达起点的流量（§17.2 回流；未成环为 0）
	TotalQi       int64                    // 体内灵气总量
	MeanPressure  int64                    // 诸穴压力均值（千分位）
	MinStability  int64                    // 诸穴稳定性最小值（千分位）
	NatureRatio   [cult.ElementCount]int64 // 五行占比（×1000，合计≈1000）
	ControlActive bool                     // 本 Tick 是否有主动控制（主会话裁决 2026-09-26：台账 ManualWork > 0 或存在主动请求；自主驱动与被动流永不计入）
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

	// NormVarFloor 是 §35 归一化方差的序列幅值下限（Part J 标定 [v0.1 参数]）：
	// 均值低于此值的序列（如五行占比偏差的整数舍入抖动）不参与失稳计量。
	NormVarFloor = 5
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

	// Part J J5 测量状态机（随样本流滚动，确定性）：
	w1Sum, w1N int64 // 当前推动相（ControlActive）的手动驱动流量累计（SelfSustain 分母输入）
	w1Avg      int64 // 最近完成的推动相手动驱动均流（W1 基线；0 = 尚无）
	everPushed bool  // 本会话是否出现过推动相（人工喂样兼容口径的判据）

	releaseRun int64 // 当前连续松手（无请求）tick 数
	relRunSum  int64 // 当前松手相流量累计（不断窗，legacy 判据用）
	relWinSum  int64 // W2 窗口流量累计（前 FoundationReleaseWindow 个松手 tick）
	relWinN    int64 // W2 窗口已收样本数（封顶后冻结——长松手期比值恒定，供 streak 累积）

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

	// Part J J5：推动相 / 松手相状态机（判定口径 foundation-metrics-v2：
	// 分母只累计手动驱动——被动/自主流不是控制做的功，不进分母）。
	if s.ControlActive {
		m.everPushed = true
		m.w1Sum += s.ManualFlow
		m.w1N++
		m.releaseRun, m.relRunSum, m.relWinSum, m.relWinN = 0, 0, 0, 0
	} else {
		if m.w1N > 0 { // 推动相刚结束：结算 W1 基线（跨观察窗持久，供长松手期使用）
			m.w1Avg = divRound(m.w1Sum, m.w1N)
			m.w1Sum, m.w1N = 0, 0
		}
		m.releaseRun++
		m.relRunSum += s.ActualFlow
		if m.relWinN < cult.FoundationReleaseWindow {
			m.relWinSum += s.ActualFlow
			m.relWinN++
		}
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

	// Closure（§17 闭合分层）= TopologyClosure × FlowClosure。
	// FlowClosure（§17.2）= 回流 / 初发流：引导环首出边的实际流量中，沿
	// 环末回边送达起点的份额（逐 Tick 比值 clamp[0,1] 后的滚动均值）。
	// 图上闭合但只回小部分不算高质量闭环（§17.3）：回流为 0 或初发流为 0
	// （环上无流）都记 0。
	var fcSum int64
	for _, s := range w {
		v := int64(0)
		if s.CycleOut > 0 {
			v = s.CycleBack * One / s.CycleOut
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

	// SelfSustain（§34 + §16 真实松手语义；判定口径 foundation-metrics-v2）：
	// W1 = 最近完成的推动相手动驱动均流（会话级基线，跨观察窗持久）；
	// W2 = 松手后前 FoundationReleaseWindow 个全无请求 tick 的均流
	//（非控制期有组织的实际流 = 被动 + 惯性 + 自主）。
	// SelfSustain = W2均流 / W1均流：控制所维持的流量中松手后仍自存多少。
	// 推动中（无松手观测）恒 0。
	// everPushed=false 的 legacy 分支兼容人工喂样口径（deprecated，仅测试）：
	// 全程从未推动而气流仍在 → 完全自持。生产路径（真实 tick 流）只要
	// 出现过推动相就走 W2/W1 口径，松手流必须真正追上手动驱动流才计满。
	switch {
	case !m.everPushed:
		if m.releaseRun > 0 && m.relRunSum > 0 {
			mt.SelfSustain = One
		}
	case m.releaseRun > 0 && m.relWinN > 0 && m.w1Avg > 0:
		w2Avg := divRound(m.relWinSum, m.relWinN)
		mt.SelfSustain = w2Avg * One / m.w1Avg
		if mt.SelfSustain > One {
			mt.SelfSustain = One
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
// 序列整体幅值低于 NormVarFloor 时视为无波动（返 0）：Part J 的稳定环流
// 下五行占比偏差只剩整数舍入抖动（±1 千分位），var/mean² 对近零均值序列
// 病态发散，不应记为失稳。
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
	if mean < NormVarFloor {
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

// topoCycle 从一份 Protocol 的引导边中判定是否成环（stage_3 §32
// TopologyClosure），并在成环时提取环的首出边与回边（§17.2 流量闭合的
// 测量点）。确定性与遍历顺序无关：按步骤顺序 DFS，取首个发现的回边
// 所在环。返回 closed 与 (outCh, backCh)——outCh 是环首出边（初发流
// 离开环起点所走的经脉），backCh 是环末回边（流沿它送达环起点）；
// 自环（单边 A→A）两者为同一条经脉。closed=false 时后两个值无意义。
func topoCycle(p *Protocol) (closed bool, outCh, backCh int) {
	if p == nil {
		return false, 0, 0
	}
	type edge struct{ from, to, ch int }
	var edges []edge
	for _, st := range p.Steps {
		if st.Kind == StepGuide {
			edges = append(edges, edge{st.From, st.To, st.Channel})
		}
	}
	if len(edges) == 0 {
		return false, 0, 0
	}
	color := map[int]int{} // 0 未访 1 在栈 2 完成
	parent := map[int]edge{}
	var cycFrom, cycTo, cycCh int
	var found bool
	var dfs func(u int)
	dfs = func(u int) {
		color[u] = 1
		for _, e := range edges {
			if e.from != u {
				continue
			}
			switch color[e.to] {
			case 0:
				parent[e.to] = e
				dfs(e.to)
			case 1:
				if !found {
					// 回边 u→e.to：e.to 在栈上（是 u 的祖先）→ 环起点。
					found, cycFrom, cycTo, cycCh = true, u, e.to, e.ch
				}
			}
		}
		color[u] = 2
	}
	for _, e := range edges {
		if color[e.from] == 0 {
			dfs(e.from)
		}
	}
	if !found {
		return false, 0, 0
	}
	backCh = cycCh
	if cycFrom == cycTo { // 自环：同一条边既出又回
		return true, cycCh, cycCh
	}
	// 沿 parent 链从 cycFrom 回溯到环起点 cycTo，得环的边序列
	// cycTo→…→cycFrom；末步回溯到的边 = 离开环起点的初发边。
	var firstEdge edge
	cur := cycFrom
	for guard := 0; cur != cycTo && guard <= len(edges); guard++ {
		e, ok := parent[cur]
		if !ok {
			return false, 0, 0 // 防御：parent 链断裂（不应发生）
		}
		firstEdge = e
		cur = e.from
	}
	if cur != cycTo {
		return false, 0, 0 // 防御：回溯未达环起点
	}
	return true, firstEdge.ch, backCh
}

// topoClosure 判定一份 Protocol 的引导路径是否成环（stage_3 §32
// TopologyClosure：A→B→…→A 则满值，否则为零）。结果只有 0/1000，
// 与遍历顺序无关。
func topoClosure(p *Protocol) int64 {
	if closed, _, _ := topoCycle(p); closed {
		return One
	}
	return 0
}

// buildFoundationSample 从本 Tick 报告提取样本（真值只在此处汇总为
// 内部计算输入，不外泄给 Agent）。
func (s *CultSession) buildFoundationSample(rep *cult.TickReport, controlActive bool) FoundationSample {
	sample := FoundationSample{Tick: rep.Tick, ControlActive: controlActive}
	proto := (*Protocol)(nil)
	if s.active != nil {
		proto = s.active.proto
	} else {
		proto = s.lastProtocol
	}
	if closed, outCh, backCh := topoCycle(proto); closed {
		sample.TopoClosure = One
		// §17.2 流量闭合测量点：环首出边的实际流量（初发流）与环末回边
		// 送达环起点的流量（回流）。只计顺流条目——逆流条目的 Actual 沿
		// 天然向反向行走，方向与本口径相反（见 TransferReport.Reverse）。
		for ti := range rep.Transfers {
			tr := &rep.Transfers[ti]
			if tr.Reverse {
				continue
			}
			for e := 0; e < cult.ElementCount; e++ {
				if tr.Channel == outCh {
					sample.CycleOut += tr.Actual[e]
				}
				if tr.Channel == backCh {
					sample.CycleBack += tr.Delivered[e]
				}
			}
		}
	}
	sample.ManualFlow = rep.Work.ManualWork
	n := int64(len(rep.Points))
	var pressSum int64
	minStab := int64(One)
	for i := range rep.Points {
		p := &rep.Points[i]
		for e := 0; e < cult.ElementCount; e++ {
			sample.Intake += p.Intake[e] + p.Entrained[e] // 环境纳入 = §7 吸收 + Part J 途经纳气
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
			// Actual 含主动、自主与被动实际流量（被动/自主条目
			// Actual==Passive/Autonomous，Part J / 0.7.0：松手期的非控制
			// 流由此进入 SelfSustain 测量）。
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
// controlActive 按主会话裁决（2026-09-26）由会话层给出：本 Tick 台账存在
// 手动驱动做功（ManualWork > 0）或存在主动请求；自主驱动与被动流永不
// 计入控制（详见 FoundationSample.ControlActive 注释）。
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
