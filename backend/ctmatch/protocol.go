package ctmatch

// protocol.go —— 功法 Protocol 的编译、静态校验与 Runtime 执行
// （stage_2 §35-37、stage_3 §25/§29-30/§41）。
//
// 职责边界：
//   - Compile 是"Agent 写的功法必须过引擎静态校验"的执行点：
//     穴位/经脉存在性、路径连通性、参数域，非法即拒绝并给出理由。
//   - Runtime 在已验证区（ValidatedRange）内全自动执行 Protocol，
//     零 LLM（stage_2 §33 L1 层）。
//   - Knowledge Boundary（stage_2 §37 / stage_3 §41）：请求流量超出
//     validated_range → 不尝试，暂停并产出 OutOfBoundary 升级事件，
//     交给上层决定是否唤醒 Pilot。Runtime 消费已知，LLM 探索未知。

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"agentbattle/backend/cult"
)

// ---------------------------------------------------------------------------
// 结构定义
// ---------------------------------------------------------------------------

// Metric 是 Guard/前置条件引用的穴位报告指标（cult.TickReport 字段）。
type Metric string

const (
	MetricPressure    Metric = "pressure"     // §14 压力 = LoadRatio²
	MetricStrain      Metric = "strain"       // §17 应变（痛觉的真值来源）
	MetricStability   Metric = "stability"    // §18 稳定性
	MetricDamage      Metric = "damage"       // §19 损伤
	MetricFlowShock   Metric = "flow_shock"   // §15 流量冲击
	MetricLoadRatio   Metric = "load_ratio"   // §14 负载比
	MetricCurrentFlow Metric = "current_flow" // §15 当前流量
)

// Cond 是一个可评估条件：某穴位的某指标与阈值比较。
type Cond struct {
	PointIdx int    // 穴位下标（编译期解析）
	Metric   Metric // 指标名
	Op       string // ">="、">"、"<="、"<"
	Value    int64  // 阈值（与 cult 包标度一致：比率千分位）
}

// GuardAction 是 Guard 触发后的动作（stage_2 §35 guards）。
type GuardAction string

const (
	GuardStop       GuardAction = "stop"        // 停：暂停 Protocol，升级给 Agent
	GuardReduceFlow GuardAction = "reduce_flow" // 降：后续 Tick 流量按 ReduceFlowFactor 缩减
)

// ReduceFlowFactor 是每次 reduce_flow 的缩减系数（千分位，-20%/次）。
// 累积缩减到 MinFlowScale 后仍压不住，会经由边界/异常路径升级。
const ReduceFlowFactor = 800

// MinFlowScale 是 flowScale 的下限（千分位）；缩到此处仍触发 Guard
// 视为无法处理，按 stop 升级。
const MinFlowScale = 200

// Guard 是一条"条件 → 动作"。
type Guard struct {
	When Cond
	Do   GuardAction
}

// StepKind 是协议步骤种类（stage_2 §35 actions；stage_3 §25 Step）。
type StepKind string

const (
	StepAbsorb StepKind = "absorb"
	StepGuide  StepKind = "guide"
)

// Step 是协议的一个步骤。粗档位（Level）或显式参数（Intensity/Amount）
// 二选一；Duration 是本步骤持续 Tick 数（stage_3 §25 dwell_ticks），
// 0 表示用协议级 Cadence。
type Step struct {
	Kind StepKind

	// absorb
	PointIdx  int   // 目标穴位下标（编译期解析）
	Intensity int64 // 显式吸收强度（千分位比率；Level 非空时忽略）

	// guide
	From    int // 源穴位下标
	To      int // 目标穴位下标
	Channel int // From→To 经脉下标（编译期解析，路径连通性校验的产物）
	Element int // 元素：cult.Wood..Water；-1 = 混合（五均分）
	Amount  int64

	Level    Level
	Duration int
}

// ValidatedRange 是一条经脉单元素的已验证流量区间（stage_3 §30）。
// Element 为 -1 表示该经脉全元素同区间。Max == 0 视为"未验证"。
type ValidatedRange struct {
	Channel int
	Element int
	Min     int64
	Max     int64
}

// Protocol 是编译后的功法：确定性、可被 Runtime 零 LLM 执行。
type Protocol struct {
	Name          string
	Preconditions []Cond // 启动前检查（stage_2 §35 preconditions）
	Steps         []Step // 动作序列；跑满一遍为一周天
	Guards        []Guard
	Cadence       int // 步骤默认持续 Tick 数（stage_2 §17 cadence）
	Cycles        int // 周天数（跑多少遍 Steps）
	Ranges        []ValidatedRange
}

// FindRange 返回某经脉某元素的已验证区间；无记录时返回 Max=0（未验证）。
func (p *Protocol) FindRange(channel, elem int) ValidatedRange {
	for _, r := range p.Ranges {
		if r.Channel == channel && (r.Element == elem || r.Element == -1) {
			return r
		}
	}
	return ValidatedRange{Channel: channel, Element: elem}
}

// ---------------------------------------------------------------------------
// 编译（stage_2 §35 Protocol Compiler）
// ---------------------------------------------------------------------------

// yaml 协议文件结构（对齐 stage_2 §35 示例，外加 stage_3 §25 的
// cadence/cycles 与 §30 的 validated_range）。
type yamlCond struct {
	Point  string `yaml:"point"`
	Metric string `yaml:"metric"`
	Op     string `yaml:"op"`
	Value  int64  `yaml:"value"`
}

type yamlAbsorb struct {
	Target    string `yaml:"target"`
	Level     string `yaml:"level"`
	Intensity int64  `yaml:"intensity"`
	Duration  int    `yaml:"duration"`
}

type yamlGuide struct {
	From     string `yaml:"from"`
	To       string `yaml:"to"`
	Element  string `yaml:"element"`
	Level    string `yaml:"level"`
	Amount   int64  `yaml:"amount"`
	Duration int    `yaml:"duration"`
}

type yamlStep struct {
	Absorb *yamlAbsorb `yaml:"absorb"`
	Guide  *yamlGuide  `yaml:"guide"`
}

type yamlGuard struct {
	When yamlCond `yaml:"when"`
	Do   string   `yaml:"do"`
}

type yamlRange struct {
	Channel []string `yaml:"channel"` // [from, to]
	Element string   `yaml:"element"`
	Min     int64    `yaml:"min"`
	Max     int64    `yaml:"max"`
}

type yamlProtocol struct {
	Protocol       string      `yaml:"protocol"`
	Preconditions  []yamlCond  `yaml:"preconditions"`
	Actions        []yamlStep  `yaml:"actions"`
	Guards         []yamlGuard `yaml:"guards"`
	Cadence        int         `yaml:"cadence"`
	Cycles         int         `yaml:"cycles"`
	ValidatedRange []yamlRange `yaml:"validated_range"`
}

// Compile 解析并静态校验一份 Protocol YAML（针对指定身体图）。
// 穴位/经脉存在性、路径连通性（guide 的 from→to 必须是图中的直接经脉）、
// 参数域（档位/强度/流量/阈值）任一非法即拒绝，错误信息给出全部理由。
func Compile(data []byte, g *cult.BodyGraph) (*Protocol, error) {
	var y yamlProtocol
	if err := yaml.Unmarshal(data, &y); err != nil {
		return nil, fmt.Errorf("protocol yaml: %w", err)
	}
	return compileYAML(&y, g)
}

func compileYAML(y *yamlProtocol, g *cult.BodyGraph) (*Protocol, error) {
	var errs []string
	add := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }

	p := &Protocol{Name: y.Protocol, Cadence: y.Cadence, Cycles: y.Cycles}
	if p.Name == "" {
		add("protocol: name is required")
	}
	if p.Cadence < 0 {
		add("cadence: must be >= 0")
	}
	if p.Cadence == 0 {
		p.Cadence = 1
	}
	if p.Cycles <= 0 {
		p.Cycles = 1
	}

	nameIdx := pointIndex(g)

	// 前置条件。
	for i, c := range y.Preconditions {
		cond, err := compileCond(&c, nameIdx)
		if err != nil {
			add("preconditions[%d]: %v", i, err)
			continue
		}
		p.Preconditions = append(p.Preconditions, cond)
	}

	// 动作序列。
	for i, s := range y.Actions {
		switch {
		case s.Absorb != nil && s.Guide != nil:
			add("actions[%d]: absorb and guide are mutually exclusive", i)
		case s.Absorb != nil:
			st, err := compileAbsorb(s.Absorb, nameIdx, p.Cadence)
			if err != nil {
				add("actions[%d]: %v", i, err)
				continue
			}
			p.Steps = append(p.Steps, st)
		case s.Guide != nil:
			st, err := compileGuide(s.Guide, g, nameIdx, p.Cadence)
			if err != nil {
				add("actions[%d]: %v", i, err)
				continue
			}
			p.Steps = append(p.Steps, st)
		default:
			add("actions[%d]: must specify absorb or guide", i)
		}
	}
	if len(p.Steps) == 0 {
		add("actions: at least one step is required")
	}

	// Guards。
	for i, gr := range y.Guards {
		cond, err := compileCond(&gr.When, nameIdx)
		if err != nil {
			add("guards[%d]: %v", i, err)
			continue
		}
		switch GuardAction(gr.Do) {
		case GuardStop, GuardReduceFlow:
		default:
			add("guards[%d]: unknown action %q (want stop|reduce_flow)", i, gr.Do)
			continue
		}
		p.Guards = append(p.Guards, Guard{When: cond, Do: GuardAction(gr.Do)})
	}

	// 已验证流量区间（stage_3 §30）。
	for i, r := range y.ValidatedRange {
		if len(r.Channel) != 2 {
			add("validated_range[%d]: channel must be [from, to]", i)
			continue
		}
		ch, err := findChannel(g, r.Channel[0], r.Channel[1], nameIdx)
		if err != nil {
			add("validated_range[%d]: %v", i, err)
			continue
		}
		elem, err := parseElement(r.Element)
		if err != nil {
			add("validated_range[%d]: %v", i, err)
			continue
		}
		if r.Min < 0 || r.Max < r.Min {
			add("validated_range[%d]: need 0 <= min <= max", i)
			continue
		}
		p.Ranges = append(p.Ranges, ValidatedRange{Channel: ch, Element: elem, Min: r.Min, Max: r.Max})
	}

	if len(errs) > 0 {
		return nil, fmt.Errorf("protocol %q rejected: %s", y.Protocol, strings.Join(errs, "; "))
	}
	return p, nil
}

func compileAbsorb(a *yamlAbsorb, nameIdx map[string]int, cadence int) (Step, error) {
	idx, ok := nameIdx[a.Target]
	if !ok {
		return Step{}, fmt.Errorf("absorb: unknown acupoint %q", a.Target)
	}
	lv, err := ParseLevel(a.Level)
	if err != nil {
		return Step{}, fmt.Errorf("absorb: %v", err)
	}
	if lv == LevelUnset && (a.Intensity <= 0 || a.Intensity > One) {
		return Step{}, fmt.Errorf("absorb: need level or intensity in (0,1]")
	}
	if lv != LevelUnset && a.Intensity != 0 {
		return Step{}, fmt.Errorf("absorb: level and intensity are mutually exclusive")
	}
	dur := a.Duration
	if dur < 0 {
		return Step{}, fmt.Errorf("absorb: negative duration")
	}
	return Step{Kind: StepAbsorb, PointIdx: idx, Level: lv, Intensity: a.Intensity, Duration: dur}, nil
}

func compileGuide(gd *yamlGuide, g *cult.BodyGraph, nameIdx map[string]int, cadence int) (Step, error) {
	from, ok := nameIdx[gd.From]
	if !ok {
		return Step{}, fmt.Errorf("guide: unknown acupoint %q", gd.From)
	}
	to, ok := nameIdx[gd.To]
	if !ok {
		return Step{}, fmt.Errorf("guide: unknown acupoint %q", gd.To)
	}
	ch, err := findChannel(g, gd.From, gd.To, nameIdx)
	if err != nil {
		return Step{}, err // 路径连通性：断路径在此拒绝
	}
	elem, err := parseElement(gd.Element)
	if err != nil {
		return Step{}, fmt.Errorf("guide: %v", err)
	}
	lv, err := ParseLevel(gd.Level)
	if err != nil {
		return Step{}, fmt.Errorf("guide: %v", err)
	}
	if lv == LevelUnset && gd.Amount <= 0 {
		return Step{}, fmt.Errorf("guide: need level or positive amount")
	}
	if lv != LevelUnset && gd.Amount != 0 {
		return Step{}, fmt.Errorf("guide: level and amount are mutually exclusive")
	}
	dur := gd.Duration
	if dur < 0 {
		return Step{}, fmt.Errorf("guide: negative duration")
	}
	return Step{Kind: StepGuide, From: from, To: to, Channel: ch, Element: elem,
		Level: lv, Amount: gd.Amount, Duration: dur}, nil
}

func compileCond(c *yamlCond, nameIdx map[string]int) (Cond, error) {
	idx, ok := nameIdx[c.Point]
	if !ok {
		return Cond{}, fmt.Errorf("unknown acupoint %q", c.Point)
	}
	m := Metric(c.Metric)
	switch m {
	case MetricPressure, MetricStrain, MetricStability, MetricDamage,
		MetricFlowShock, MetricLoadRatio, MetricCurrentFlow:
	default:
		return Cond{}, fmt.Errorf("unknown metric %q", c.Metric)
	}
	switch c.Op {
	case ">=", ">", "<=", "<":
	default:
		return Cond{}, fmt.Errorf("unknown op %q (want >= > <= <)", c.Op)
	}
	if c.Value < 0 || c.Value > 100*One {
		return Cond{}, fmt.Errorf("value %d out of domain [0,100]", c.Value)
	}
	return Cond{PointIdx: idx, Metric: m, Op: c.Op, Value: c.Value}, nil
}

func pointIndex(g *cult.BodyGraph) map[string]int {
	m := make(map[string]int, len(g.Acupoints))
	for i, a := range g.Acupoints {
		m[a.Name] = i
	}
	return m
}

// findChannel 返回 from→to 的直接经脉下标；不存在即路径不连通。
// v0.1 只允许单跳步骤（stage_3 §25 的 Sequence 本就以相邻穴对成步），
// 多跳路径由 Agent 在协议里拆成多步。
func findChannel(g *cult.BodyGraph, from, to string, nameIdx map[string]int) (int, error) {
	fi, ok := nameIdx[from]
	if !ok {
		return -1, fmt.Errorf("unknown acupoint %q", from)
	}
	ti, ok := nameIdx[to]
	if !ok {
		return -1, fmt.Errorf("unknown acupoint %q", to)
	}
	if fi == ti {
		return -1, fmt.Errorf("guide: from and to are the same acupoint %q", from)
	}
	for c, ch := range g.Channels {
		if ch.From == fi && ch.To == ti {
			return c, nil
		}
	}
	return -1, fmt.Errorf("guide: no channel %q -> %q (path not connected)", from, to)
}

// parseElement 解析元素名（英文/中文）；""、"mixed"、"all" 表示混合（-1）。
func parseElement(s string) (int, error) {
	switch strings.ToLower(s) {
	case "", "mixed", "all", "混", "混合", "全部":
		return -1, nil
	case "wood", "木":
		return cult.Wood, nil
	case "fire", "火":
		return cult.Fire, nil
	case "earth", "土":
		return cult.Earth, nil
	case "metal", "金":
		return cult.Metal, nil
	case "water", "水":
		return cult.Water, nil
	}
	return -1, fmt.Errorf("unknown element %q", s)
}

// ---------------------------------------------------------------------------
// Runtime（L1；全自动、零 LLM，stage_2 §33）
// ---------------------------------------------------------------------------

// protocolState 是一个正在运行/暂停的 Protocol 的执行状态。
type protocolState struct {
	proto      *Protocol
	step       int   // 当前步骤下标
	tickInStep int   // 当前步骤已执行 Tick 数
	cycle      int   // 已完成的周天数
	flowScale  int64 // Guard reduce_flow 的累积缩减（千分位，1000 = 不缩）
	paused     bool  // 暂停（Guard stop / 出界 / 未知异常）
	done       bool  // 跑满 Cycles
}

// stepDuration 返回当前步骤的有效持续 Tick 数。
func (st *protocolState) stepDuration() int {
	d := st.proto.Steps[st.step].Duration
	if d == 0 {
		d = st.proto.Cadence
	}
	if d <= 0 {
		d = 1
	}
	return d
}

// runtimeOps 产出当前 Tick 的 Ops，并推进步骤/周天游标。
// 返回 nil Event 表示可以推进引擎；返回 Event 表示出界
// （Knowledge Boundary），本 Tick 引擎不得推进。
func (st *protocolState) runtimeOps(s *CultSession) (cult.Ops, *Event) {
	g := &s.World.Character.BodyGraph
	step := st.proto.Steps[st.step]
	ops := cult.Ops{Intensity: make([]int64, len(g.Acupoints))}

	switch step.Kind {
	case StepAbsorb:
		it := step.Intensity
		if step.Level != LevelUnset {
			it = IntensityForLevel(step.Level)
		}
		ops.Intensity[step.PointIdx] = it

	case StepGuide:
		flow := step.Amount
		if step.Level != LevelUnset {
			flow = FlowForLevel(step.Level, g, step.From, step.To, step.Channel)
		}
		flow = fpMul(flow, st.flowScale)
		// Knowledge Boundary（stage_2 §37 / stage_3 §41）：
		// 请求超出已验证区间 → 不尝试，暂停并升级。
		if ev := st.checkBoundary(s, step, flow); ev != nil {
			return ops, ev
		}
		req := cult.FlowRequest{Channel: step.Channel}
		if step.Element >= 0 {
			req.Amount[step.Element] = flow
		} else {
			for e := 0; e < cult.ElementCount; e++ {
				req.Amount[e] = fpMul(flow, 200) // 混合：每元素 0.2，合计 1.0
			}
		}
		ops.Requests = append(ops.Requests, req)
	}

	// 推进游标（本 Tick 已产出 Ops；下一 Tick 用下一步骤）。
	st.tickInStep++
	if st.tickInStep >= st.stepDuration() {
		st.tickInStep = 0
		st.step++
		if st.step >= len(st.proto.Steps) {
			st.step = 0
			st.cycle++
			if st.cycle >= st.proto.Cycles {
				st.done = true
			}
		}
	}
	return ops, nil
}

// checkBoundary 校验请求流量是否落在已验证区间内。
// 无 validated_range 记录视为未验证（Max=0）：任何正流量都出界。
func (st *protocolState) checkBoundary(s *CultSession, step Step, flow int64) *Event {
	r := st.proto.FindRange(step.Channel, step.Element)
	out := flow > r.Max || (r.Min > 0 && flow < r.Min)
	if !out {
		return nil
	}
	st.paused = true
	return &Event{
		Kind:     EventOutOfBoundary,
		Protocol: st.proto.Name,
		Reason: fmt.Sprintf("requested flow %d outside validated range [%d,%d] (channel %d, element %d); runtime does not explore unknowns, escalating to agent",
			flow, r.Min, r.Max, step.Channel, step.Element),
		Detail: map[string]int64{
			"channel": int64(step.Channel), "element": int64(step.Element),
			"flow": flow, "min": r.Min, "max": r.Max,
		},
	}
}

// evaluateCond 对报告评估一个条件。
func evaluateCond(c Cond, rep *cult.TickReport) int64 {
	if c.PointIdx < 0 || c.PointIdx >= len(rep.Points) {
		return 0
	}
	p := &rep.Points[c.PointIdx]
	switch c.Metric {
	case MetricPressure:
		return p.Pressure
	case MetricStrain:
		return p.Strain
	case MetricStability:
		return p.Stability
	case MetricDamage:
		return p.Damage
	case MetricFlowShock:
		return p.FlowShock
	case MetricLoadRatio:
		return p.LoadRatio
	case MetricCurrentFlow:
		return p.CurrentFlow
	}
	return 0
}

func condTrue(c Cond, rep *cult.TickReport) bool {
	v := evaluateCond(c, rep)
	switch c.Op {
	case ">=":
		return v >= c.Value
	case ">":
		return v > c.Value
	case "<=":
		return v <= c.Value
	case "<":
		return v < c.Value
	}
	return false
}

// runGuards 在每 Tick 结算后检查 guards（stage_2 §36 已知轻微异常→自动调整）。
// 返回是否有 Guard 触发、以及会话事件。
func (st *protocolState) runGuards(s *CultSession, rep *cult.TickReport) (fired bool) {
	for _, g := range st.proto.Guards {
		if !condTrue(g.When, rep) {
			continue
		}
		fired = true
		switch g.Do {
		case GuardReduceFlow:
			if st.flowScale > MinFlowScale {
				st.flowScale = fpMul(st.flowScale, ReduceFlowFactor)
				s.pushEvent(Event{
					Kind: EventGuardAdjusted, Protocol: st.proto.Name,
					Reason: fmt.Sprintf("guard triggered (%s %s %d at %s): flow scaled to %d/1000",
						g.When.Metric, g.When.Op, g.When.Value, s.World.Character.BodyGraph.Acupoints[g.When.PointIdx].Name, st.flowScale),
					Detail: map[string]int64{"metric_value": evaluateCond(g.When, rep), "threshold": g.When.Value, "flow_scale": st.flowScale},
				})
				continue
			}
			// 已缩到底仍触发 → 无法处理，按 stop 升级。
			fallthrough
		case GuardStop:
			st.paused = true
			s.pushEvent(Event{
				Kind: EventGuardAdjusted, Protocol: st.proto.Name,
				Reason: fmt.Sprintf("guard stop (%s %s %d at %s): protocol paused, escalating to agent",
					g.When.Metric, g.When.Op, g.When.Value, s.World.Character.BodyGraph.Acupoints[g.When.PointIdx].Name),
				Detail: map[string]int64{"metric_value": evaluateCond(g.When, rep), "threshold": g.When.Value},
			})
		}
	}
	return fired
}

// fpMul 定点乘（千分位，四舍五入），与 cult 包标度一致。
// 本包自含一份：cult 的实现未导出，且本包只做轻微换算，不重复引擎公式。
func fpMul(a, b int64) int64 {
	p := a * b
	if p >= 0 {
		return (p + One/2) / One
	}
	return (p - One/2) / One
}
