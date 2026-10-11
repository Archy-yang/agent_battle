package cult

// percept.go —— PerceptToken 中间层（境界文档 Part B §14-18 + 修订包 Part F）。
//
// 感知流水线（§8）：数值状态 → 信号派生 → 时间分析（趋势/时长，8 Tick
// 滚动窗口）→ 阈值 + 滞回 → PerceptToken → 确定性模板渲染 → Observation。
//
// 本文件产出的是结构化 token（§17）：每个信号 {level, trend, duration}、
// 痛感分型、nature 签名、炼化档位与复合模式。token 不导出到
// Observation——Observation 仍纯定性文本（感知隔离律），token 只是
// 渲染的内部中间层。
//
// 滞回（§15，[固定实现原则]）：连续感知状态用进入/退出双阈值，raw 值
// 在 [exit[b], enter[b]] 死区内时保持原档，杜绝阈值附近反复跳档。
// 全部阈值为 [v0.1 参数]。
//
// 滞回状态（sensoryState）存档在 World 的私有字段上，只被本文件的
// 信号更新写入、被 Observe 逐 Tick 推进（每 Tick 备忘一次，同一 Tick
// 反复观察复用同一 token——确定性 + 幂等）。同 seed 重放时两份状态
// 逐位一致（DeepEqual 测试锁死）。全程定点整数，无浮点。

// ---------------------------------------------------------------------------
// 滞回阈值表（[v0.1 参数]；enter = 进入该档下限，exit = 跌回上一档上限）
// ---------------------------------------------------------------------------

// hystBand 是一档滞回阈值（千分位）。
type hystBand struct {
	enter int64
	exit  int64
}

// fullnessBands 是充盈档（LoadRatio，§10.1：<0.20 无 / 0.20-0.40 轻微 /
// 0.40-0.65 明显 / 0.65-0.85 饱满 / >0.85 近上限），死区 0.05。
var fullnessBands = []hystBand{
	{}, {200, 150}, {400, 350}, {650, 600}, {850, 800},
}

// distensionBands 是胀感档（DistensionSignal，§10.2：<0.20 无 / 0.20-0.40
// 轻微撑感 / 0.40-0.60 明显发胀 / 0.60-0.80 强烈胀感 / 0.80-1.00 胀痛 /
// >1.00 强烈胀痛·失稳风险），死区 0.05。
var distensionBands = []hystBand{
	{}, {200, 150}, {400, 350}, {600, 550}, {800, 750}, {1000, 950},
}

// thermalBands 是热感档（ThermalLoad − 基线 0，§9.2：<0.05 无明显热感 /
// 0.05-0.15 微温 / 0.15-0.30 温热 / 0.30-0.50 明显发热 / 0.50-0.75 灼热 /
// >0.75 灼痛风险）。温热档即 §15 示例：进入 ≥0.17、退出 ≤0.12。
var thermalBands = []hystBand{
	{}, {50, 20}, {170, 120}, {320, 270}, {520, 470}, {770, 720},
}

// flowBands 是阻滞档（obstruction，§12 的体验面：顺畅 → 难以推动），
// 死区 ~0.08。
var flowBands = []hystBand{
	{}, {250, 180}, {500, 400}, {750, 650},
}

// painBands 是痛感强度档（PainSignal，§11.1），死区 ~0.05。
var painBands = []hystBand{
	{}, {120, 80}, {300, 250}, {600, 550},
}

// ---------------------------------------------------------------------------
// 信号参数（[v0.1 参数]）
// ---------------------------------------------------------------------------

const (
	// pressureDeltaGain 是 K_pressure_delta（§10.2）：DistensionSignal =
	// Pressure + K × max(0, dPressure/dt)。快速上涨比缓慢上涨更易察觉。
	pressureDeltaGain = 250 // 0.25

	// hystWindow 是趋势滚动窗口（§14 moving window，v0.1 取 8 Tick）。
	hystWindow = 8

	// 趋势判定死区（raw − 窗口均值，千分位）：小于该值视为"稳定"。
	trendEpsFullness   = 15
	trendEpsDistension = 20
	trendEpsThermal    = 20
	trendEpsFlow       = 30
	trendEpsPain       = 40

	// flowShockGain 是流量冲击对阻滞感的贡献（obstruction）。
	flowShockGain = 500 // 0.5
	// flowBackpressureAdd 是反压过高时的阻滞加成（两级：<0.4 / <0.25）。
	flowBackpressureAdd  = 300
	flowBackpressureAdd2 = 200

	// painDullHalf 是隐痛判型的持续性门槛：Damage 需连续 ≥2 Tick 出现，
	// 否则其"成因贡献"减半——高频冲击 + 低损伤判刺痛而非隐痛（§11.2）。
	painDullMinTicks = 2

	// refineMid / refineHigh 是炼化三档分界（修订包 Part F，[v0.1 参数]）：
	// <0.30 低（驳杂）/ 0.30-0.70 中（驯顺）/ ≥0.70 高（凝练）。
	refineMid  = 300
	refineHigh = 700
)

// 趋势（§14）。
const (
	trendFalling = -1
	trendStable  = 0
	trendRising  = 1
)

// 复合模式（§16 Pattern Resolver，确定性规则，不调用 LLM）。
const (
	patternBackpressure = iota // 胀感上升 + 推动受阻 → 反压
	patternFlowImpact          // 冲击型刺痛 → 气息冲击经路
	patternRunaway             // 热升 + 稳定降 → 失稳前兆
	patternCount
)

// ---------------------------------------------------------------------------
// Token 结构（内部中间层；不导出到 Observation）
// ---------------------------------------------------------------------------

// signalToken 是一个连续信号的本 Tick 感知结果（§17 signals × {level,
// trend} + duration）。
type signalToken struct {
	level    int // 滞回档
	trend    int // trendFalling / trendStable / trendRising
	duration int // 连续处于当前档的 Tick 数（≥1）
}

// painKind 是痛感的主导成因（§11.2）。0=无报告，1..5 与
// painKindWords 对齐。
type painKind int

const (
	painNone       painKind = iota // 无报告（世界未推进）
	painDistension                 // 胀痛（Pressure / Overload）
	painSharp                      // 刺痛（FlowShock）
	painBurning                    // 灼痛（ThermalLoad / 转化热）
	painDull                       // 隐痛（Damage 持续）
	painTension                    // 拉扯痛（五行不合张力）
)

// pointToken 是一个穴位的 PerceptToken（§17）。
type pointToken struct {
	Fullness   signalToken // 充盈（LoadRatio，§10.1）
	Distension signalToken // 胀（Pressure + K×dPressure，§10.2）
	Thermal    signalToken // 热（ThermalLoad，§9）
	Flow       signalToken // 阻滞（obstruction，§12）
	Pain       signalToken // 痛（PainSignal，§11.1）
	Kind       painKind    // 痛感主导成因（§11.2）
	Nature     int         // nature 签名：五行下标，-1 = 辨不出
	Refine     int         // 炼化三档：0 低 / 1 中 / 2 高（修订包 Part F）
	Patterns   []int       // 命中的复合模式（§16）
}

// ---------------------------------------------------------------------------
// 滞回状态（World 私有存档；参与确定性重放）
// ---------------------------------------------------------------------------

// signalRing 是一个信号的滚动窗口（最多 hystWindow 个历史 raw 值，
// vals[hystWindow-1] 为最新）。
type signalRing struct {
	vals [hystWindow]int64
	n    int
}

func (r *signalRing) push(v int64) {
	if r.n < hystWindow {
		r.n++
	}
	copy(r.vals[:], r.vals[1:])
	r.vals[hystWindow-1] = v
}

// mean 返回窗口均值（空窗口 = 0）。
func (r *signalRing) mean() int64 {
	if r.n == 0 {
		return 0
	}
	sum := int64(0)
	for i := hystWindow - r.n; i < hystWindow; i++ {
		sum += r.vals[i]
	}
	return sum / int64(r.n)
}

// last 返回最近一次 push 的值（空窗口 = 0）。
func (r *signalRing) last() int64 {
	if r.n == 0 {
		return 0
	}
	return r.vals[hystWindow-1]
}

// pointMemory 是一个穴位的感知记忆：各信号的滚动窗口 + 滞回档位 +
// 档位持续时长 + 持续损伤计数。
type pointMemory struct {
	load     signalRing
	pressure signalRing
	dist     signalRing
	thermal  signalRing
	flow     signalRing
	pain     signalRing

	lvFull, lvDist, lvThermal, lvFlow, lvPain int
	durFull, durDist, durThermal, durFlow     int
	durPain                                   int
	damageTicks                               int // Damage 连续 > 0 的 Tick 数

	// tokens 是本 Tick 备忘的 PerceptToken（tick 相同的重复观察复用）。
	tokens pointToken
}

// sensoryState 是全部穴位的感知记忆 + 本 Tick 的 token 备忘。
type sensoryState struct {
	tick   int64
	points []pointMemory
}

// hystStep 是滞回状态机：raw 升破 enter 跳档（一次可跨多档），跌破
// exit 逐档回落，死区内保持原档（§15）。
func hystStep(level int, raw int64, bands []hystBand) int {
	for b := len(bands) - 1; b > level; b-- {
		if raw >= bands[b].enter {
			return b
		}
	}
	for level > 0 && raw <= bands[level].exit {
		level--
	}
	return level
}

// updateSignal 完成一个信号的本 Tick 时间分析：窗口均值 → 趋势 →
// 入窗 → 滞回 → 时长。
func updateSignal(r *signalRing, level, dur *int, raw int64, bands []hystBand, trendEps int64) signalToken {
	trend := trendStable
	if r.n > 0 {
		if d := raw - r.mean(); d > trendEps {
			trend = trendRising
		} else if d < -trendEps {
			trend = trendFalling
		}
	}
	r.push(raw)
	prev := *level
	*level = hystStep(*level, raw, bands)
	if *level == prev {
		*dur++
	} else {
		*dur = 1
	}
	return signalToken{level: *level, trend: trend, duration: *dur}
}

// sensoryTokens 返回本 Tick 全部穴位的 PerceptToken。每 Tick 只计算
// 一次（按 w.Tick 备忘）：同一 Tick 的后续 Observe 复用同一 token，
// 因此 Observe 保持确定性且逐次深度相等；滞回状态随观察推进。
func sensoryTokens(w *World) []pointToken {
	if w.sensory == nil {
		w.sensory = &sensoryState{}
	}
	st := w.sensory
	n := len(w.Character.BodyGraph.Acupoints)
	if st.tick == w.Tick && len(st.points) == n {
		tokens := make([]pointToken, n)
		for i := range st.points {
			tokens[i] = st.points[i].tokens
		}
		return tokens
	}
	if len(st.points) != n {
		st.points = make([]pointMemory, n)
	}
	for i := range st.points {
		a := &w.Character.BodyGraph.Acupoints[i]
		st.points[i].update(a, w, i)
	}
	st.tick = w.Tick
	tokens := make([]pointToken, n)
	for i := range st.points {
		tokens[i] = st.points[i].tokens
	}
	return tokens
}

// update 派生一个穴位本 Tick 的全部信号并推进滞回状态。
func (m *pointMemory) update(a *Acupoint, w *World, idx int) {
	var r AcupointReport
	hasReport := w.Last != nil && idx < len(w.Last.Points)
	if hasReport {
		r = w.Last.Points[idx]
	}

	// 充盈 / 胀（§10）：无报告时用现场状态计算，口径与报告一致。
	lr := int64(0)
	pressure := int64(0)
	if hasReport {
		lr = r.LoadRatio
		pressure = r.Pressure
	} else {
		lr = loadRatio(a, a.load())
		pressure = fpMul(lr, lr)
	}
	m.tokens.Fullness = updateSignal(&m.load, &m.lvFull, &m.durFull, lr, fullnessBands, trendEpsFullness)
	// dPressure/dt：本 Tick 压力 − 上一观察压力（千分位/Tick）。首拍无
	// 先前状态 → 无增量信息（不把"从无到有"计为上涨）。
	var dP int64
	if m.pressure.n > 0 {
		dP = pressure - m.pressure.last()
		if dP < 0 {
			dP = 0
		}
	}
	dist := pressure + fpMul(pressureDeltaGain, dP)
	m.tokens.Distension = updateSignal(&m.dist, &m.lvDist, &m.durDist, dist, distensionBands, trendEpsDistension)
	m.pressure.push(pressure)

	// 热（§9）：热感只来自热负荷，不看属性名（§7.2）。ThermalLoad 是
	// 穴位累积状态，报告与现场同口径。
	thermal := a.ThermalLoad
	if hasReport {
		thermal = r.ThermalLoad
	}
	m.tokens.Thermal = updateSignal(&m.thermal, &m.lvThermal, &m.durThermal, thermal, thermalBands, trendEpsThermal)

	// 阻滞 / 痛：只在有报告（世界推进过）时更新——停滞世界不积累痛感。
	if hasReport {
		m.tokens.Flow = updateSignal(&m.flow, &m.lvFlow, &m.durFlow, obstructionOf(&r), flowBands, trendEpsFlow)
		pain, kind := m.painOf(&r)
		m.tokens.Pain = updateSignal(&m.pain, &m.lvPain, &m.durPain, pain, painBands, trendEpsPain)
		m.tokens.Kind = kind
	} else {
		m.tokens.Flow = signalToken{level: m.lvFlow, trend: trendStable, duration: m.durFlow}
		m.tokens.Pain = signalToken{level: m.lvPain, trend: trendStable, duration: m.durPain}
	}

	// nature 签名（§13）：与神识无关的信号层判定，神识门槛在渲染层。
	if dom, load := dominantElement(a.Qi); load >= natureMinLoad {
		m.tokens.Nature = dom
	} else {
		m.tokens.Nature = -1
	}

	// 炼化三档（修订包 Part F；D2 导出量，绝不落存储）。
	refine := DeriveRefinement(w, idx)
	switch {
	case refine >= refineHigh:
		m.tokens.Refine = 2
	case refine >= refineMid:
		m.tokens.Refine = 1
	default:
		m.tokens.Refine = 0
	}

	m.tokens.Patterns = resolvePatterns(a, &m.tokens)
}

// obstructionOf 把流量/反压折算成阻滞感（§12 的体验面，千分位 0..1000）：
// 流量冲击越猛越滞涩；吸收反压过低（上一负载比过高）时叠加两级加成。
func obstructionOf(r *AcupointReport) int64 {
	shock := r.FlowShock
	if shock > One {
		shock = One
	}
	obs := fpMul(flowShockGain, shock)
	if r.BackPressure < blockedFeel {
		obs += flowBackpressureAdd
	}
	if r.BackPressure < blockedFeel/2 {
		obs += flowBackpressureAdd2
	}
	if obs > One {
		obs = One
	}
	return obs
}

// painOf 计算 PainSignal（§11.1 [v0.1 权重]）并判主导成因（§11.2）：
//
//	PainSignal = 0.35×Strain + 0.35×Damage + 0.20×FlowShock + 0.10×Overload
//
// 成因贡献取自 Strain 的逐项拆解（§17 公式重算，报告字段同口径）：
// 过载项 → 胀痛、冲击项 → 刺痛、转化热 + 热负荷应变（0.8.0，慢通道
// 长程积热的灼感） → 灼痛、亲和失配项 → 拉扯痛；
// Damage 单独计隐痛，但需连续 painDullMinTicks Tick 持续才按全权重
// 参与判型（高频冲击 + 低损伤 → 刺痛而非隐痛）。并列取优先序
// 胀 > 刺 > 灼 > 隐 > 扯（确定性）。
func (m *pointMemory) painOf(r *AcupointReport) (int64, painKind) {
	ov := r.LoadRatio - One
	if ov < 0 {
		ov = 0
	}
	affStrain := One - r.Compatibility
	if affStrain < 0 {
		affStrain = 0
	}
	shock := r.FlowShock
	if shock > One {
		shock = One
	}

	cDist := fpMul(WOverload, fpMul(ov, ov)) + fpMul(100, ov)
	cSharp := fpMul(WShock, fpMul(shock, shock)) + fpMul(200, shock)
	cBurn := r.TransformHeat + r.HeatLoadStrain
	if r.Damage > 0 {
		m.damageTicks++
	} else {
		m.damageTicks = 0
	}
	cDull := fpMul(350, r.Damage)
	if m.damageTicks < painDullMinTicks {
		cDull /= 2
	}
	cTension := fpMul(WAffinity, fpMul(affStrain, affStrain))

	pain := fpMul(350, r.Strain) + fpMul(350, r.Damage) + fpMul(200, shock) + fpMul(100, ov)

	kind := painDistension
	best := cDist
	if cSharp > best {
		kind, best = painSharp, cSharp
	}
	if cBurn > best {
		kind, best = painBurning, cBurn
	}
	if cDull > best {
		kind, best = painDull, cDull
	}
	if cTension > best {
		kind, best = painTension, cTension
	}
	return pain, kind
}

// resolvePatterns 是 Pattern Resolver（§16）：确定性复合感觉识别。
func resolvePatterns(a *Acupoint, tok *pointToken) []int {
	patterns := []int{}
	if tok.Distension.level >= 2 && tok.Distension.trend == trendRising && tok.Flow.level >= 1 {
		patterns = append(patterns, patternBackpressure)
	}
	if tok.Pain.level >= 1 && tok.Kind == painSharp {
		patterns = append(patterns, patternFlowImpact)
	}
	if tok.Thermal.level >= 2 && tok.Thermal.trend == trendRising && stabilityBand(a.Stability) >= 3 {
		patterns = append(patterns, patternRunaway)
	}
	return patterns
}

// ---------------------------------------------------------------------------
// 词表（token → 定性文本；零数字）
// ---------------------------------------------------------------------------

// thermalWords 是热感档措辞（thermalBands 1..5；境界文档 §9.2）。
var thermalWords = [6]string{"", "微温", "温热", "明显发热", "灼热", "灼痛之虞"}

// painKindWords[kind-1][level] 是痛感分型措辞（§11.2；kind 与 painKind
// 常量 1..5 对齐——下标 0 是胀痛行；level 0 = 无痛不渲染）。
var painKindWords = [5][4]string{
	{"", "胀痛隐现", "阵阵胀痛", "胀痛难忍"},   // 胀痛（压力/过载）
	{"", "偶有刺痛", "阵阵刺痛", "刺痛剧烈"},   // 刺痛（流量冲击）
	{"", "隐有灼痛", "灼痛阵阵", "灼痛难当"},   // 灼痛（热负荷）
	{"", "隐隐作痛", "隐痛持续", "隐痛沉重"},   // 隐痛（持续损伤）
	{"", "微有拉扯之感", "拉扯作痛", "拉扯痛甚"}, // 拉扯痛（五行不合）
}

// refineWords 是炼化三档措辞（修订包 Part F）。
var refineWords = [3]string{
	"气机驳杂，偶有逸散之感",   // 低：驳杂、逸散感
	"气感渐驯，推动阻力减小",   // 中：驯顺、阻力减小
	"气机凝练，几乎不再自行散去", // 高：凝练、几乎不散
}

// patternWords 是复合模式措辞（§16）。
var patternWords = [patternCount]string{
	"灵气仍在积累，继续推动越发吃力", // BACKPRESSURE
	"气息骤然冲击经路，引发短促刺痛", // FLOW_IMPACT
	"热意持续增强，波动也开始不稳",  // RUNAWAY_ACTIVATION
}

// trendWord 从 token 选出最显著的一条趋势措辞（§18 模板优先级：趋势
// 放最后；单穴最多一条，避免碎片化）。
func trendWord(tok *pointToken) string {
	switch {
	case tok.Distension.trend == trendRising && tok.Distension.level >= 1:
		return "胀意仍在增强"
	case tok.Thermal.trend == trendRising && tok.Thermal.level >= 1:
		return "热意渐盛"
	case tok.Fullness.trend == trendRising && tok.Fullness.level >= 1:
		return "充盈仍在缓缓积聚"
	case tok.Thermal.trend == trendFalling && tok.Thermal.level >= 2:
		return "热势渐退"
	case tok.Distension.trend == trendFalling && tok.Distension.level >= 1:
		return "胀意渐缓"
	default:
		return ""
	}
}
