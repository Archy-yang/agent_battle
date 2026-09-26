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
// 0 表示用协议级 Cadence。Parallel 是同息并行组号（SPIKE 结论三
// "纳引相位割裂"的对策，stage_2 §17 cadence 的节奏语义扩展）：非零且
// 相邻步骤组号相同者同一息编排（吸收+引流同 Tick 生效），0 表示独立成息。
type Step struct {
	Kind StepKind

	// absorb
	PointIdx  int   // 目标穴位下标（编译期解析）
	Intensity int64 // 显式吸收强度（千分位比率；Level 非空时忽略）

	// guide
	From    int // 源穴位下标（声明流向的发起端）
	To      int // 目标穴位下标（声明流向的目的端）
	Channel int // 经脉下标（编译期解析，边序列校验的产物）
	// Reverse 标记本步为逆流边（修订包 C1/A2）：声明流向与经脉天然向
	// 相反（实际气从 To 端流向 From 端——即 From=发起端）。编译期已校验
	// 发起端是核点（Capacity 最大穴位，A1 v0.2 核点判据——唯有丹田可发起
	// 外推，B1）；运行期映射 FlowRequest{Reverse:true}，引擎仍保留最终否决权。
	Reverse bool
	Element int // 元素：cult.Wood..Water；-1 = 混合（五均分）
	Amount  int64

	Level    Level
	Duration int
	Parallel int // 同息并行组号（0 = 独立步骤；>0 且相邻同号 = 同息）
}

// ValidatedRange 是一条经脉单元素的已验证流量区间（stage_3 §30）。
// Element 为 -1 表示该经脉全元素同区间。Max == 0 视为"未验证"。
type ValidatedRange struct {
	Channel int
	Element int
	Min     int64
	Max     int64
}

// ProtocolStatus 是功法的生命周期状态（stage_2 §29 知识→功法、
// stage_3 §29 功法验证）：实验方案（draft）→ 达标转正（validated）。
type ProtocolStatus string

const (
	StatusDraft     ProtocolStatus = "draft"     // 实验方案：尚未通过试运行验证
	StatusValidated ProtocolStatus = "validated" // 正法：试运行达标（stage_3 §29）
)

// Protocol 是编译后的功法：确定性、可被 Runtime 零 LLM 执行。
type Protocol struct {
	Name          string
	Status        ProtocolStatus // 生命周期：draft → validated（试运行达标自动转正）
	Preconditions []Cond         // 启动前检查（stage_2 §35 preconditions）
	Steps         []Step         // 动作序列；跑满一遍为一周天
	Slots         [][]int        // 同息编排：每槽是同 Tick 并行的 Step 下标组（SPIKE 结论三）
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
	Parallel  int    `yaml:"parallel"` // 同息并行组号（SPIKE 结论三）
}

type yamlGuide struct {
	From     string `yaml:"from"`
	To       string `yaml:"to"`
	Element  string `yaml:"element"`
	Level    string `yaml:"level"`
	Amount   int64  `yaml:"amount"`
	Duration int    `yaml:"duration"`
	Parallel int    `yaml:"parallel"` // 同息并行组号（SPIKE 结论三）
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
// 穴位/经脉存在性、边序列合法性（修订包 C1：每条 guide 边须是图中真实
// 经脉，任一方向；逆流边只允许核点发起）、参数域（档位/强度/流量/
// 阈值）任一非法即拒绝，错误信息给出全部理由。
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

	p := &Protocol{Name: y.Protocol, Cadence: y.Cadence, Cycles: y.Cycles, Status: StatusDraft}
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
			st, err := compileAbsorb(s.Absorb, nameIdx)
			if err != nil {
				add("actions[%d]: %v", i, err)
				continue
			}
			st.Parallel = s.Absorb.Parallel
			p.Steps = append(p.Steps, st)
		case s.Guide != nil:
			st, err := compileGuide(s.Guide, g, nameIdx)
			if err != nil {
				add("actions[%d]: %v", i, err)
				continue
			}
			st.Parallel = s.Guide.Parallel
			p.Steps = append(p.Steps, st)
		default:
			add("actions[%d]: must specify absorb or guide", i)
		}
	}
	if len(p.Steps) == 0 {
		add("actions: at least one step is required")
	}

	// 同息并行编排（SPIKE 结论三：吸收-引流同息并行，化解纳引相位割裂）。
	slots, err := buildSlots(p.Steps, p.Cadence)
	if err != nil {
		add("actions: %v", err)
	} else {
		p.Slots = slots
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
		ch, err := resolveRangeChannel(g, r.Channel, nameIdx)
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

func compileAbsorb(a *yamlAbsorb, nameIdx map[string]int) (Step, error) {
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

func compileGuide(gd *yamlGuide, g *cult.BodyGraph, nameIdx map[string]int) (Step, error) {
	from, ok := nameIdx[gd.From]
	if !ok {
		return Step{}, fmt.Errorf("guide: unknown acupoint %q", gd.From)
	}
	to, ok := nameIdx[gd.To]
	if !ok {
		return Step{}, fmt.Errorf("guide: unknown acupoint %q", gd.To)
	}
	// 边序列校验（修订包 C1）：每条边须是真实经脉（任一方向）；
	// 逆流边（声明向与天然向相反）只允许核点发起（A2，结构判据同引擎核）。
	ch, rev, found := edgeFor(g, from, to)
	if !found {
		if from == to {
			return Step{}, fmt.Errorf("guide: from and to are the same acupoint %q", gd.From)
		}
		return Step{}, fmt.Errorf("guide: no channel %q -> %q (path not connected)", gd.From, gd.To)
	}
	if rev && !isCorePoint(g, from) {
		return Step{}, fmt.Errorf("guide: reverse edge %q -> %q denied: only the core point may push against the natural flow", gd.From, gd.To)
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
	return Step{Kind: StepGuide, From: from, To: to, Channel: ch, Reverse: rev, Element: elem,
		Level: lv, Amount: gd.Amount, Duration: dur}, nil
}

// resolveRangeChannel 解析 validated_range 的 channel [from, to]：
// 与 guide 步骤同用 edgeFor（任一方向，含逆流边）。
func resolveRangeChannel(g *cult.BodyGraph, names []string, nameIdx map[string]int) (int, error) {
	fi, ok1 := nameIdx[names[0]]
	ti, ok2 := nameIdx[names[1]]
	if !ok1 {
		return -1, fmt.Errorf("unknown acupoint %q", names[0])
	}
	if !ok2 {
		return -1, fmt.Errorf("unknown acupoint %q", names[1])
	}
	ch, _, found := edgeFor(g, fi, ti)
	if !found {
		return -1, fmt.Errorf("guide: no channel %q -> %q (path not connected)", names[0], names[1])
	}
	return ch, nil
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

// edgeFor 在身体图中解析一条声明方向的边（修订包 C1：协议路径=边序列，
// 每条边 {from,to} 须是 BodyGraph 真实经脉，任一方向皆可）：
// 先找天然向 fi→ti（顺流边）；再找 ti→fi（逆流边，reverse=true——实际气
// 从 fi 端推向 ti 端）。两向皆无或 fi==ti 则 found=false。
// 注意：逆流边的发起资格（核点，A1 v0.2/A2）由调用方另行校验——编译期与
// 意图路径都用 isCorePoint，与引擎核 runTransfers 同用 cult.CorePointOf，
// 判据单点实现，无漂移。
func edgeFor(g *cult.BodyGraph, fi, ti int) (channel int, reverse, found bool) {
	if fi == ti {
		return -1, false, false
	}
	for c := range g.Channels {
		if g.Channels[c].From == fi && g.Channels[c].To == ti {
			return c, false, true
		}
	}
	for c := range g.Channels {
		if g.Channels[c].From == ti && g.Channels[c].To == fi {
			return c, true, true
		}
	}
	return -1, false, false
}

// isCorePoint 报告节点 i 是否为核点（修订包 A1 v0.2：Capacity 最大的穴位）——
// 丹田画像的结构判据（B1，无 type 字段——宪法 §8）。实现单点在
// cult.CorePointOf，本包不复制判据逻辑；旧"纯汇点（无出向经脉）"判据在
// 主环拓扑下失效（丹田有出向边 丹→土）。唯有核点可发起逆流外推（修订包 A2）。
func isCorePoint(g *cult.BodyGraph, i int) bool {
	return i == cult.CorePointOf(g)
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
// 同息并行编排（SPIKE 结论三：cadence 表达"同息并行"而非仅交替步骤）
// ---------------------------------------------------------------------------

// buildSlots 把步骤序列折成同息编排槽：Parallel==0 的步骤独立成槽；
// 相邻同组号（>0）的步骤并入同一槽——同起同息、同 Tick 生效。
// 同槽各步的有效持续时长必须一致，否则拒绝编译。
func buildSlots(steps []Step, cadence int) ([][]int, error) {
	var slots [][]int
	for i := range steps {
		p := steps[i].Parallel
		if p < 0 {
			return nil, fmt.Errorf("parallel group number must be >= 0 (step %d)", i)
		}
		if n := len(slots); p > 0 && n > 0 && steps[slots[n-1][0]].Parallel == p {
			slots[n-1] = append(slots[n-1], i)
			continue
		}
		slots = append(slots, []int{i})
	}
	for si, sl := range slots {
		if len(sl) < 2 {
			continue
		}
		d := effDuration(steps[sl[0]], cadence)
		for _, idx := range sl[1:] {
			if effDuration(steps[idx], cadence) != d {
				return nil, fmt.Errorf("parallel group at slot %d: steps sharing one breath must share one duration", si)
			}
		}
	}
	return slots, nil
}

// effDuration 是步骤的有效持续 Tick 数（0 → 协议级 cadence）。
func effDuration(s Step, cadence int) int {
	if s.Duration == 0 {
		return cadence
	}
	if s.Duration < 0 {
		return 1
	}
	return s.Duration
}

// slotDuration 返回第 i 槽的有效持续 Tick 数。
func (p *Protocol) slotDuration(i int) int {
	if i < 0 || i >= len(p.Slots) {
		return 1
	}
	return effDuration(p.Steps[p.Slots[i][0]], p.Cadence)
}

// ---------------------------------------------------------------------------
// Runtime（L1；全自动、零 LLM，stage_2 §33）
// ---------------------------------------------------------------------------

// protocolState 是一个正在运行/暂停的 Protocol 的执行状态。
type protocolState struct {
	proto      *Protocol
	slot       int   // 当前槽下标（同息并行组或独立步骤）
	tickInSlot int   // 当前槽已执行 Tick 数
	cycle      int   // 已完成的周天数
	flowScale  int64 // Guard reduce_flow 的累积缩减（千分位，1000 = 不缩）
	paused     bool  // 暂停（Guard stop / 出界 / 未知异常）
	done       bool  // 跑满 Cycles
	loopMode   bool  // AutonomousLoop（stage_3 §39）：周天无上限，Runtime 每 Tick 自推
}

// runtimeOps 产出当前 Tick 的 Ops（同槽各步并行合流），并推进槽/周天
// 游标。返回 nil Event 表示可以推进引擎；返回 Event 表示出界
// （Knowledge Boundary），本 Tick 引擎不得推进。
func (st *protocolState) runtimeOps(s *CultSession) (cult.Ops, *Event) {
	g := &s.World.Character.BodyGraph
	ops := cult.Ops{Intensity: make([]int64, len(g.Acupoints))}

	for _, si := range st.proto.Slots[st.slot] {
		step := st.proto.Steps[si]
		switch step.Kind {
		case StepAbsorb:
			it := step.Intensity
			if step.Level != LevelUnset {
				it = IntensityForLevel(step.Level)
			}
			ops.Intensity[step.PointIdx] = it

		case StepGuide:
			// §21.3（主会话裁决 2026-09-26）：筑基后（AutonomousLoop 模式）
			// 协议不再是逐 Tick 驱动身体的脚本——已内化的段移交身体，由
			// 反馈耦合的自主命令续推；协议保留的角色是监控/守卫/异常升级。
			// 未移交的段照常发请求——若协议仍对已内化转移发主动请求，
			// 那段流就是手动流、计入控制，自持测量不受污染。
			if st.loopMode && feedHandoverTaken(s, step) {
				break
			}
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
			req := cult.FlowRequest{Channel: step.Channel, Reverse: step.Reverse}
			if step.Element >= 0 {
				req.Amount[step.Element] = flow
			} else {
				for e := 0; e < cult.ElementCount; e++ {
					req.Amount[e] = fpMul(flow, 200) // 混合：每元素 0.2，合计 1.0
				}
			}
			ops.Requests = append(ops.Requests, req)
		}
	}

	// 推进游标（本 Tick 已产出 Ops；下一 Tick 用下一槽）。
	st.tickInSlot++
	if st.tickInSlot >= st.proto.slotDuration(st.slot) {
		st.tickInSlot = 0
		st.slot++
		if st.slot >= len(st.proto.Slots) {
			st.slot = 0
			st.cycle++
			if st.cycle >= st.proto.Cycles && !st.loopMode {
				st.done = true
			}
		}
	}
	return ops, nil
}

// feedHandoverStrength 是筑基后（AutonomousLoop 模式）Runtime 向身体移交
// 已内化段的耦合强度门槛（评审稿 §21.3；主会话裁决 2026-09-26）。
// 取 0.8 的理由 [v0.1 参数，调参起点]：耦合按强度比例复现教学出流
// （命令 = 强度 × 来流 × 习得响应），强度 0.8 时移交后环流量保有约八成
// 教学水平、来流信号仍落在教学容差带（FeedTolerance=0.5）内——更低的
// 门槛会让移交后的流贴着容差带缘运行，稍有波动即「认不出」而断流。
// 待评审稿 §27 Monte Carlo 标定。
const feedHandoverStrength = 800

// feedHandoverTaken 判定一份引导步骤的出边段是否已被身体接管（§21.3
// 移交门，仅 loopMode 下调用）：step.From 穴位上存在出边为 step.Channel
// 的有效耦合，且步骤请求的每个元素的耦合强度都达到移交门槛。混合元素
// 步骤要求全元素达标——部分内化的段仍由意识驱动补足（§21.2）。
func feedHandoverTaken(s *CultSession, step Step) bool {
	g := &s.World.Character.BodyGraph
	if step.From < 0 || step.From >= len(g.Acupoints) ||
		step.Channel < 0 || step.Channel >= len(g.Channels) {
		return false
	}
	if g.Channels[step.Channel].From != step.From {
		return false // 出边已不从未穴流出（图谱被改造）：移交门不认
	}
	var best [cult.ElementCount]int64
	for k := range g.Acupoints[step.From].Couplings {
		cp := &g.Acupoints[step.From].Couplings[k]
		if cp.OutChannel != step.Channel {
			continue
		}
		for e := 0; e < cult.ElementCount; e++ {
			if cp.Strength[e] > best[e] {
				best[e] = cp.Strength[e]
			}
		}
	}
	if step.Element >= 0 {
		return best[step.Element] >= feedHandoverStrength
	}
	for e := 0; e < cult.ElementCount; e++ {
		if best[e] < feedHandoverStrength {
			return false
		}
	}
	return true
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

// ---------------------------------------------------------------------------
// 试运行与固化闭环（stage_2 §29 知识→功法；stage_3 §28-30）
// ---------------------------------------------------------------------------

// EventProtocolValidated 是试运行达标转正事件（stage_3 §29）。
const EventProtocolValidated EventKind = "protocol_validated"

// 周天成功与转正阈值（千分位；§52：实验值，公式关系不可变）。
const (
	CycleMinEfficiency   = 500 // §28 SuccessfulCycle: CycleEfficiency >= 0.50
	CycleMinStability    = 600 // §28 MinimumCycleStability >= 0.60
	CycleDamageTolerance = 5   // §28 DamageTolerance 0.005（×1000）
	ValidateMinCycles    = 20  // §29 SuccessfulCycles >= 20
	ValidateMinRate      = 950 // §29 SuccessRate >= 95%
	ValidateMinAvgStab   = 750 // §29 AverageMinimumStability >= 0.75
	RangeExpandStep      = 500 // §30 validated_range 一次允许的外扩幅度（量，千分位）
)

// CycleRecord 是一周天的试运行记录（成功/失败原因均为定性词；数值字段
// 只进转录/调试视图）。
type CycleRecord struct {
	Cycle        int
	Success      bool
	FailWhy      string // 定性失败原因："归返不足"/"稳定性不足"/"新增损伤"
	Efficiency   int64  // 调试视图：CycleEfficiency（×1000）
	MinStability int64  // 调试视图：CycleStability——路径诸穴稳定性最小值（×1000）
	Heat         int64  // 调试视图：HeatAccumulation——路径 ThermalLoad 变化和（修订包 C1）
}

// TrialRecord 是一次 Runtime 试跑的逐周天记录。
type TrialRecord struct {
	Protocol       string
	Cycles         []CycleRecord
	UnknownAnomaly bool // 试运行期间出现过未知异常（§29：转正要求没有未知异常）
}

// TrialSummary 是试跑结束后的达标判定（stage_3 §29）。
type TrialSummary struct {
	SuccessfulCycles int
	TotalCycles      int
	SuccessRate      int64 // ×1000（调试视图）
	AvgMinStability  int64 // ×1000（调试视图）
	UnknownAnomaly   bool
	Qualified        bool // 是否满足转正条件
}

// TrialMonitor 伴随一次 Runtime 试跑，逐周天记录成功与失败原因。
// 周天边界 = 各槽有效时长之和（与 Runtime 游标同构）。
//
// 评估沿路径累计（修订包 C1：周天=边序列，路径评估不是集合评估）：
// 效率与稳定性只统计协议边序列覆盖的穴位/经脉，热账只算路径穴位。
type TrialMonitor struct {
	proto       *Protocol
	cycleLen    int
	tickInCycle int
	cycleNo     int
	input       int64 // 本周天累计：纳入 + 实际推动（§26 CycleInput 的 v0.1 口径）
	output      int64 // 本周天累计：送达（§26 CycleOutput 的 v0.1 口径）
	minStab     int64
	damageStart int64
	pathPts     []int        // 路径穴位（吸收目标 + 各边两端，去重，编译顺序）
	pathChs     map[int]bool // 路径经脉（各边编译解析出的通道）
	heatStart   []int64      // 周天起点各路径穴位的热负荷（HeatAccumulation 基准）
	Record      TrialRecord
}

// newTrialMonitor 为一份 Protocol 建试跑记录器。
func newTrialMonitor(p *Protocol) *TrialMonitor {
	cl := 0
	for i := range p.Slots {
		cl += p.slotDuration(i)
	}
	seen := map[int]bool{}
	var pts []int
	chs := map[int]bool{}
	for i := range p.Steps {
		st := &p.Steps[i]
		cand := []int{}
		switch st.Kind {
		case StepAbsorb:
			cand = append(cand, st.PointIdx)
		case StepGuide:
			cand = append(cand, st.From, st.To)
			chs[st.Channel] = true
		}
		for _, idx := range cand {
			if idx >= 0 && !seen[idx] {
				seen[idx] = true
				pts = append(pts, idx)
			}
		}
	}
	return &TrialMonitor{proto: p, cycleLen: cl, minStab: One,
		pathPts: pts, pathChs: chs, Record: TrialRecord{Protocol: p.Name}}
}

// ObserveTick 累积一个引擎 Tick 的报告；跨过周天边界即结一周天账。
func (t *TrialMonitor) ObserveTick(rep *cult.TickReport) {
	if t == nil || t.cycleLen <= 0 || rep == nil {
		return
	}
	if t.tickInCycle == 0 {
		t.minStab = One
		t.input, t.output = 0, 0
		t.damageStart = 0
		for i := range rep.Points {
			if rep.Points[i].Damage > t.damageStart {
				t.damageStart = rep.Points[i].Damage
			}
		}
		// 周天起点热负荷（HeatAccumulation = 路径 ThermalLoad 变化和，C1）。
		t.heatStart = make([]int64, len(t.pathPts))
		for k, pt := range t.pathPts {
			if pt < len(rep.Points) {
				t.heatStart[k] = rep.Points[pt].ThermalLoad
			}
		}
	}
	// 路径穴位：纳入累计 + CycleStability（路径各穴稳定性最小值，C1）。
	for _, pt := range t.pathPts {
		if pt >= len(rep.Points) {
			continue
		}
		p := &rep.Points[pt]
		for e := 0; e < cult.ElementCount; e++ {
			t.input += p.Intake[e]
		}
		if p.Stability < t.minStab {
			t.minStab = p.Stability
		}
	}
	// 路径经脉：实际推动与送达累计（含逆流边——Actual/Delivered 已由
	// 引擎按实际流向给出）。
	for ti := range rep.Transfers {
		tr := &rep.Transfers[ti]
		if !t.pathChs[tr.Channel] {
			continue
		}
		for e := 0; e < cult.ElementCount; e++ {
			t.input += tr.Actual[e]
			t.output += tr.Delivered[e]
		}
	}
	t.tickInCycle++
	if t.tickInCycle >= t.cycleLen {
		t.closeCycle(rep)
		t.tickInCycle = 0
		t.cycleNo++
	}
}

// closeCycle 结一周天的账（stage_3 §28 周天成功判定 + 修订包 C1 路径评估）。
func (t *TrialMonitor) closeCycle(rep *cult.TickReport) {
	eff := int64(0)
	if t.input > 0 {
		eff = t.output * One / t.input
		if eff > One {
			eff = One
		}
	}
	// HeatAccumulation（C1）：路径各穴 ThermalLoad 自周天起点的变化和。
	heat := int64(0)
	if len(t.heatStart) == len(t.pathPts) {
		for k, pt := range t.pathPts {
			if pt < len(rep.Points) {
				heat += rep.Points[pt].ThermalLoad - t.heatStart[k]
			}
		}
	}
	dmgEnd := int64(0)
	for i := range rep.Points {
		if rep.Points[i].Damage > dmgEnd {
			dmgEnd = rep.Points[i].Damage
		}
	}
	newDamage := dmgEnd-t.damageStart > CycleDamageTolerance
	ok := eff >= CycleMinEfficiency && t.minStab >= CycleMinStability && !newDamage
	cr := CycleRecord{Cycle: t.cycleNo, Success: ok, Efficiency: eff, MinStability: t.minStab, Heat: heat}
	switch {
	case ok:
	case eff < CycleMinEfficiency:
		cr.FailWhy = "归返不足：投入多而归返少"
	case t.minStab < CycleMinStability:
		cr.FailWhy = "稳定性不足：循环中有的穴位晃得厉害"
	default:
		cr.FailWhy = "新增损伤"
	}
	t.Record.Cycles = append(t.Record.Cycles, cr)
}

// Finish 在试跑结束时做达标判定（stage_3 §29：SuccessfulCycles >= 20
// 且 SuccessRate >= 95% 且 AverageMinimumStability >= 0.75 且无未知异常）。
func (t *TrialMonitor) Finish() TrialSummary {
	var sum TrialSummary
	sum.TotalCycles = len(t.Record.Cycles)
	for _, c := range t.Record.Cycles {
		if c.Success {
			sum.SuccessfulCycles++
		}
		sum.AvgMinStability += c.MinStability
	}
	if sum.TotalCycles > 0 {
		sum.SuccessRate = int64(sum.SuccessfulCycles) * One / int64(sum.TotalCycles)
		sum.AvgMinStability = divRound(sum.AvgMinStability, int64(sum.TotalCycles))
	}
	sum.UnknownAnomaly = t.Record.UnknownAnomaly
	sum.Qualified = sum.SuccessfulCycles >= ValidateMinCycles &&
		sum.SuccessRate >= ValidateMinRate &&
		sum.AvgMinStability >= ValidateMinAvgStab &&
		!sum.UnknownAnomaly
	return sum
}

// ExpandRange 在回执确认安全后小步扩张已验证流量区间（stage_3 §30
// Known Safe Range 的"逐步扩张"）。只允许已转正（validated）的功法、
// 且一次外扩不超过 RangeExpandStep——Runtime 只小步放宽已知安全区，
// 更远的未知仍留给 LLM 探索。
func (s *CultSession) ExpandRange(p *Protocol, channel, element int, newMin, newMax int64) error {
	if p == nil {
		return errf("expand: nil protocol")
	}
	if p.Status != StatusValidated {
		return errf("protocol %q is not validated yet; range expansion denied", p.Name)
	}
	ri := -1
	for i := range p.Ranges {
		if p.Ranges[i].Channel == channel && p.Ranges[i].Element == element {
			ri = i
			break
		}
	}
	if ri < 0 {
		return errf("protocol %q has no validated range for channel %d element %d", p.Name, channel, element)
	}
	r := &p.Ranges[ri]
	if newMin < 0 || newMax < newMin {
		return errf("expand: need 0 <= min <= max, got [%d,%d]", newMin, newMax)
	}
	if newMin > r.Min || newMax < r.Max {
		return errf("expand: only widening is allowed (current [%d,%d]); contraction denied", r.Min, r.Max)
	}
	if newMin < r.Min-RangeExpandStep {
		return errf("expand: min %d reaches below %d by more than one safe step; expansion denied", newMin, r.Min)
	}
	if newMax > r.Max+RangeExpandStep {
		return errf("expand: max %d exceeds %d by more than one safe step; expansion denied", newMax, r.Max)
	}
	r.Min, r.Max = newMin, newMax
	return nil
}
