package ctmatch

// api.go —— /api/cult/* HTTP 层（M1 WP-5）。
//
// 职责：
//   - 内存 registry 持有修炼会话（持久化不在本 WP）；
//   - 把会话状态渲染成"玩家界面视图"：全部定性词 / band 序号 / 记账数字
//     （Tick、周天转数），真值（五量数值、qi 数值）只在 ?debug=1 下输出
//     ——感知隔离律（M1 计划 §0 铁律 1）在 API 层同样成立；
//   - 玩家意图走与 Pilot 相同的会话入口（ctmatch.Tick），记 FromPlayer；
//   - 心跳唤醒 Pilot（节奏与 cultspike 一致），升级唤醒由会话层自理。
//
// 世界构造复用 spikeworld.go 的 SpikeWorld（cmd/cultspike 同源）。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"

	"agentbattle/backend/cult"
)

// ---------------------------------------------------------------------------
// 世界构造（SPIKE 同源）
// ---------------------------------------------------------------------------

// starterProtocolYAML 是新会话预载的草创功法：给功法/参悟两屏一个
// 可视的起点（起功后走 Runtime，护卫与出界请示都有真实演示）。
const starterProtocolYAML = `protocol: 吐纳初诀
actions:
  - absorb:
      target: 青关
      level: small
      duration: 2
  - guide:
      from: 青关
      to: 下丹田
      level: large
      duration: 2
guards:
  - when: {point: 青关, metric: pressure, op: ">=", value: 850}
    do: stop
cadence: 1
cycles: 10
validated_range:
  - channel: [青关, 下丹田]
    element: mixed
    min: 0
    max: 8000
`

// ---------------------------------------------------------------------------
// Pilot：脚本循环桩（"script" 分身）
// ---------------------------------------------------------------------------

// loopPilot 是 script 分身。两种画像（会话 mode 决定）：
//   - tutorial：每次被征询都交出同一组朴素吐纳意图（吸收+引流，粗档位
//     可达的稳定路径）——演示/测试用的预置走法；
//   - normal（探索型）：驱动层策略，不读世界真值、不预设答案——交替
//     静观 / 在不同可感之处小量纳气 / 偶尔在已知穴位之间试引导，成败
//     全凭回执反应（允许"此路不通"）。本尊止息后不再出招。
type loopPilot struct {
	intents []Intent // tutorial 画像的固定意图组
	explore bool     // true = normal 会话的探索型策略
	stopped bool

	points []string // 探索型：观察到的可感穴位名（来自定性观察，非世界真值）
	step   int      // 探索型：被征询的次数（决定策略轮换）
}

func (p *loopPilot) Decide(obs *Observation) []Intent {
	if p == nil || p.stopped {
		return nil
	}
	if p.explore {
		return p.exploreDecide(obs)
	}
	out := make([]Intent, len(p.intents))
	copy(out, p.intents)
	return out
}

// exploreDecide 是探索型策略：静观 / 轮换纳气 / 轮换试引导三班倒。
// 一切输入只来自定性观察（穴位名是观察词，非世界真值）；引导走没走通
// 由下一息的回执告知，这里不预判。
func (p *loopPilot) exploreDecide(obs *Observation) []Intent {
	if obs != nil {
		for _, pt := range obs.Points {
			known := false
			for _, n := range p.points {
				if n == pt.Name {
					known = true
					break
				}
			}
			if !known {
				p.points = append(p.points, pt.Name)
			}
		}
	}
	p.step++
	round := p.step % 3
	switch {
	case round == 1 && len(p.points) > 0:
		// 在不同可感之处小量纳气（轮换）。
		tgt := p.points[(p.step/3)%len(p.points)]
		return []Intent{{Kind: IntentAbsorb, Acupoint: tgt, Level: LevelSmall}}
	case round == 2 && len(p.points) >= 2:
		// 偶尔在已知穴位之间试引导（轮换相邻对，小量，允许失败回执）。
		i := (p.step / 3) % len(p.points)
		j := (i + 1) % len(p.points)
		return []Intent{{Kind: IntentGuide, From: p.points[i], To: p.points[j], Element: -1, Level: LevelSmall}}
	default:
		return []Intent{{Kind: IntentSense}}
	}
}

// scriptIntents 是 tutorial 分身的默认画像：主纳穴少量吸收 + 混合大量引流。
func scriptIntents() []Intent {
	return []Intent{
		{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall},
		{Kind: IntentGuide, From: "青关", To: "下丹田", Element: -1, Level: LevelLarge},
	}
}

// ---------------------------------------------------------------------------
// 会话句柄
// ---------------------------------------------------------------------------

// cultHandle 是 registry 里的一次修炼会话及其界面簿记（感知流、玩家
// 意图志、待决请示）。
type cultHandle struct {
	id     string
	sess   *CultSession
	pilot  Pilot
	kind   string // "llm" | "script"
	script *loopPilot

	currentIntents []Intent // 常驻意图（Pilot 最近一次交出的 / 玩家动作叠加）

	stream    []StreamLine  // 感知流（含回执/事件行，旧→新）
	receipts  []ReceiptLine // 回执行（旧→新）
	playerLog []PlayerLine  // 玩家意图志（FromPlayer）
	pending   []RequestCard // 待决请示（升级事件）
	reqSeq    int
}

// StreamLine 是感知流的一行。
type StreamLine struct {
	Tick int64  `json:"tick"`
	Kind string `json:"kind"` // sense | receipt | event | voice
	Text string `json:"text"`
}

// ReceiptLine 是一条回执的界面行（ReceiptView 渲染，零数字）。
type ReceiptLine struct {
	Tick int64  `json:"tick"`
	Text string `json:"text"`
}

// PlayerLine 是玩家意图志的一行（FromPlayer 记录）。
type PlayerLine struct {
	Tick    int64  `json:"tick"`
	Summary string `json:"summary"` // intentSummary 的定性摘要
}

// RequestCard 是一张"分身请示"卡（升级事件上浮）。
type RequestCard struct {
	ID   string `json:"id"`
	Kind string `json:"kind"` // guard_stop | out_of_boundary | unknown_anomaly
	Text string `json:"text"`
	Tick int64  `json:"tick"`
}

// ---------------------------------------------------------------------------
// 视图类型（玩家界面载荷：定性词 / band / 记账数字）
// ---------------------------------------------------------------------------

// EnvChip 是环境一行五行的定性 chip。
type EnvChip struct {
	Element  string `json:"element"`
	Qi       string `json:"qi"`
	Activity string `json:"activity"`
}

// PointView 是一个穴位的定性视图（全部来自 cult.Observe 的 Observation）。
type PointView struct {
	Name         string   `json:"name"`
	Feelings     []string `json:"feelings"`
	Flow         string   `json:"flow"`
	Nature       []string `json:"nature"`
	Openness     string   `json:"openness"`
	Stability    string   `json:"stability"`
	Damage       string   `json:"damage"`
	PressureBand int      `json:"pressure_band"` // 0-5 档序号（供 UI 色带）
}

// ChannelView 是一条经脉的结构视图（拓扑，非真值）。
type ChannelView struct {
	Name string `json:"name"`
	From string `json:"from"`
	To   string `json:"to"`
}

// BodyView 是身体图（8 穴位 + 经脉拓扑）。
type BodyView struct {
	Points   []string      `json:"points"`
	Channels []ChannelView `json:"channels"`
}

// HypView 是一张假说卡。
type HypView struct {
	Text     string           `json:"text"`
	Status   HypothesisStatus `json:"status"`
	Evidence string           `json:"evidence"` // 分档词（零数字）
}

// RangeView 是一条已验证流量区间（功法配置，非世界真值）。
type RangeView struct {
	Channel string `json:"channel"`
	Element string `json:"element"`
	Min     int64  `json:"min"`
	Max     int64  `json:"max"`
}

// ProtoView 是一张功法卡。
type ProtoView struct {
	Name    string         `json:"name"`
	Status  ProtocolStatus `json:"status"`
	Path    string         `json:"path"`
	Guards  []string       `json:"guards"`
	Ranges  []RangeView    `json:"ranges"`
	Active  bool           `json:"active"`
	Paused  bool           `json:"paused"`
	Loop    bool           `json:"loop"`
	Cycles  int            `json:"cycles"`
	Current int            `json:"current"` // 已跑完的周天数（记账）
}

// CycleLine 是一周天的试运行记录行。
type CycleLine struct {
	Cycle   int    `json:"cycle"`
	Success bool   `json:"success"`
	FailWhy string `json:"fail_why,omitempty"`
}

// TrialView 是当前试跑卡。
type TrialView struct {
	Protocol string      `json:"protocol"`
	Cycle    int         `json:"cycle"` // 正在跑第几周天
	Total    int         `json:"total"`
	Loop     bool        `json:"loop"`
	Recent   []CycleLine `json:"recent"`
}

// FiveView 是筑基五关之一（定性词 + band 0-4 供进度条）。
type FiveView struct {
	Key  string `json:"key"`
	Word string `json:"word"`
	Band int    `json:"band"`
}

// DebugPoint 是调试视图的一个穴位真值（仅 ?debug=1）。
type DebugPoint struct {
	Name      string `json:"name"`
	Qi        int64  `json:"qi"`
	Stability int64  `json:"stability"`
	Damage    int64  `json:"damage"`
}

// DebugView 是调试载荷（真值；仅 ?debug=1 时出现，前端不消费）。
type DebugView struct {
	Metrics   LoopMetrics  `json:"metrics"`
	Points    []DebugPoint `json:"points"`
	FlowScale int64        `json:"flow_scale"`
}

// SessionView 是一次修炼会话的玩家界面视图。
type SessionView struct {
	ID           string        `json:"id"`
	Seed         uint64        `json:"seed"`
	RuleVersion  string        `json:"rule_version"` // 产生本会话状态的底层规则版本（cult.RuleVersion；非世界真值，不属泄漏）
	Tick         int64         `json:"tick"`
	Acuity       int           `json:"acuity"`
	Realm        string        `json:"realm"`
	PilotKind    string        `json:"pilot_kind"`
	LLMAvailable bool          `json:"llm_available"`
	Env          []EnvChip     `json:"env"`
	Points       []PointView   `json:"points"`
	Body         BodyView      `json:"body"`
	SelfReport   string        `json:"self_report"`
	Cognitions   []string      `json:"cognitions"`
	SenseStream  []StreamLine  `json:"sense_stream"`
	Receipts     []ReceiptLine `json:"receipts"`
	MyIntents    []PlayerLine  `json:"my_intents"`
	Requests     []RequestCard `json:"requests"`
	Ledger       []string      `json:"ledger"`
	Hypotheses   []HypView     `json:"hypotheses"`
	Protocols    []ProtoView   `json:"protocols"`
	Active       string        `json:"active"`
	Trial        *TrialView    `json:"trial,omitempty"`
	Foundation   []FiveView    `json:"foundation"`
	Debug        *DebugView    `json:"debug,omitempty"`
}

// ---------------------------------------------------------------------------
// registry
// ---------------------------------------------------------------------------

// CultRegistry 是修炼会话的内存 registry 与 HTTP 处理器集合。
type CultRegistry struct {
	mu       sync.Mutex
	sessions map[string]*cultHandle
	seq      int
}

// NewCultRegistry 建空 registry。
func NewCultRegistry() *CultRegistry {
	return &CultRegistry{sessions: make(map[string]*cultHandle)}
}

// Create 建一次修炼会话。pilotKind："script"（默认）或 "llm"（未配置
// LLM 时报错）。mode："normal"（默认）或 "tutorial"：
//   - tutorial：预载《吐纳初诀》（青关→丹田）+ script 分身固定走法，
//     供演示/测试；
//   - normal：功法库为空，script 分身走探索型策略（不预设答案）。
//
// seed 为 0 时取 1（确定性默认）。
func (r *CultRegistry) Create(seed uint64, pilotKind, mode string) (*SessionView, error) {
	if pilotKind == "" {
		pilotKind = "script"
	}
	switch pilotKind {
	case "script", "llm":
	default:
		return nil, fmt.Errorf("未知分身类型 %q（可选 script | llm）", pilotKind)
	}
	if mode == "" {
		mode = "normal"
	}
	switch mode {
	case "normal", "tutorial":
	default:
		return nil, fmt.Errorf("未知会话模式 %q（可选 normal | tutorial）", mode)
	}

	w := SpikeWorld()
	var pilot Pilot
	var script *loopPilot
	if pilotKind == "llm" {
		lp := NewLLMPilot()
		if lp == nil {
			return nil, fmt.Errorf("LLM 未配置（AGENTBATTLE_LLM_BASE_URL/_API_KEY/_MODEL），无法请 llm 分身")
		}
		lp.MaxCalls = LLMBudgetDefault
		pilot = lp
	} else if mode == "tutorial" {
		script = &loopPilot{intents: scriptIntents()}
		pilot = script
	} else {
		script = &loopPilot{explore: true}
		pilot = script
	}

	sess := NewSession(w, seed, pilot)
	const acuity = 1 // 初见措辞（感知隔离律：观察只出定性词）
	sess.Observe = func(x *cult.World) *Observation { return cult.Observe(x, acuity) }

	// 仅 tutorial 预载草创功法（编译失败不阻断建会话——功法屏留空即可）；
	// normal 功法库为空，门径由分身自己摸索。
	if mode == "tutorial" {
		if p, err := Compile([]byte(starterProtocolYAML), &w.Character.BodyGraph); err == nil {
			sess.LoadProtocol(p)
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.seq++
	h := &cultHandle{
		id:     fmt.Sprintf("cult-%04d", r.seq),
		sess:   sess,
		pilot:  pilot,
		kind:   pilotKind,
		script: script,
	}
	h.currentIntents = nil
	r.sessions[h.id] = h
	return h.view(), nil
}

// get 取会话句柄。
func (r *CultRegistry) get(id string) *cultHandle {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.sessions[id]
}

// ---------------------------------------------------------------------------
// 心跳与推进
// ---------------------------------------------------------------------------

// WakePilotNextTick 让会话在下一个 Tick 开始时唤醒 Pilot（带最新观察
// 征询意图）。上层心跳征询（API 层）的入口：与会话层升级唤醒共用同一
// 路径（ ReceiptAware/LedgerAware 载荷照给）。
func (s *CultSession) WakePilotNextTick() { s.lastEscalated = true }

// heartbeatDue 是心跳节奏（与 cultspike 一致）：前 120 Tick 每 5 Tick
// 一询（探路期密集决策），之后每 40 Tick 一询。
func heartbeatDue(t int64) bool {
	if t <= 0 {
		return false
	}
	if t <= 120 {
		return t%5 == 0
	}
	return t%40 == 0
}

// step 推进 n 个 Tick。升级唤醒由会话层自理；心跳到点则置唤醒标记。
// 每 Tick 后把 Pilot 新交出的意图固化为常驻意图（与 cultspike 的
// "意图沿用直到下次征询"同构），并append 感知流。
func (h *cultHandle) step(n int) {
	if n <= 0 {
		n = 1
	}
	if n > 500 {
		n = 500
	}
	for i := 0; i < n; i++ {
		t := h.sess.World.Tick
		if h.sess.Pilot != nil && heartbeatDue(t) {
			h.sess.WakePilotNextTick()
		}
		res := Tick(h.sess, h.currentIntents)
		h.absorbPilotDecisions(t)
		h.appendTickLines(res)
	}
}

// absorbPilotDecisions 扫意图日志：本 Tick 由 Pilot 交出的非空意图组
// 取代常驻意图（Pilot 沉默则沿用——分身入定不语）。
func (h *cultHandle) absorbPilotDecisions(t int64) {
	var latest []Intent
	for _, e := range h.sess.IntentLog {
		if e.Tick == t && e.FromPilot {
			latest = append(latest, e.Intent) // 同组意图按序累积
		} else if e.Tick > t {
			break
		}
	}
	if len(latest) > 0 {
		h.currentIntents = latest
	}
}

// appendTickLines 把一个 Tick 的观察/回执/事件折成感知流行。
func (h *cultHandle) appendTickLines(res *TickResult) {
	t := h.sess.World.Tick
	g := &h.sess.World.Character.BodyGraph
	for _, ev := range res.Events {
		h.stream = append(h.stream, StreamLine{Tick: t, Kind: "event", Text: eventText(ev, g)})
		if card := requestCardFor(ev); card != nil {
			h.reqSeq++
			card.ID = fmt.Sprintf("req-%d", h.reqSeq)
			card.Tick = t
			h.pending = append(h.pending, *card)
		}
	}
	for _, rc := range res.Receipts {
		h.receipts = append(h.receipts, ReceiptLine{Tick: t, Text: ReceiptView(rc)})
		h.stream = append(h.stream, StreamLine{Tick: t, Kind: "receipt", Text: "【回执】" + ReceiptView(rc)})
	}
	if line := senseLine(res.Observation); line != "" {
		h.stream = append(h.stream, StreamLine{Tick: t, Kind: "sense", Text: line})
	}
	// 簿记滚动上限：感知流与回执各留最近 160 条。
	if len(h.stream) > 160 {
		h.stream = h.stream[len(h.stream)-160:]
	}
	if len(h.receipts) > 160 {
		h.receipts = h.receipts[len(h.receipts)-160:]
	}
}

// senseLine 由定性观察合成一行感知文（零数字）。
func senseLine(obs *Observation) string {
	if obs == nil {
		return ""
	}
	var parts []string
	for _, p := range obs.Points {
		hot := p.PressureBand >= 2 || p.Damage != "无损" || p.Flow != "停滞" || len(p.Feelings) > 1
		if !hot {
			continue
		}
		feel := strings.Join(p.Feelings, "·")
		if feel == "" {
			feel = "无明显感觉"
		}
		parts = append(parts, fmt.Sprintf("%s：%s，气机%s", p.Name, feel, p.Flow))
	}
	if len(parts) == 0 {
		return "周身平静，气机沉寂。"
	}
	return strings.Join(parts, "；") + "。"
}

// eventText 把会话事件翻译成定性界面文（Event.Reason 含真值数字，
// 一律不外泄——铁律 1）。
func eventText(e Event, g *cult.BodyGraph) string {
	switch e.Kind {
	case EventGuardAdjusted:
		if strings.HasPrefix(e.Reason, "guard stop") {
			return "护卫叫停：" + e.Protocol + "暂歇，候本尊示下"
		}
		return "护卫调息：" + e.Protocol + "的气流已自行放缓"
	case EventOutOfBoundary:
		return "欲引的气流越出「" + e.Protocol + "」已验证的边界，分身不敢妄进，特此请示"
	case EventUnknownAnomaly:
		name := "某穴"
		if i, ok := e.Detail["acupoint"]; ok && g != nil {
			name = pointName(int(i), g)
		}
		return "异象：" + name + "似生新伤，其故难明，已停功请示"
	case EventProtocolStarted:
		return "起功：「" + e.Protocol + "」开始自行运转"
	case EventProtocolPaused:
		return "「" + e.Protocol + "」暂歇（另有心意接管行功）"
	case EventProtocolFinished:
		return "「" + e.Protocol + "」行满周天，功成收势"
	case EventProtocolValidated:
		return "「" + e.Protocol + "」试运行达标，由草创入正册"
	case EventPreconditionFail:
		return "「" + e.Protocol + "」起功未成：此刻身势不合其所求"
	case EventInvalidIntent:
		return "一份心意无处落地，已散去"
	case EventFoundationTouched:
		return e.Reason
	case EventFoundationEstablished:
		return e.Reason
	case EventLoopRegistered:
		return "周天自成循环：「" + e.Protocol + "」入自运行，此后不假分身之思"
	}
	return "隐隐有所触动。"
}

// requestCardFor 把升级事件折成请示卡；非升级事件返回 nil。
func requestCardFor(e Event) *RequestCard {
	switch e.Kind {
	case EventGuardAdjusted:
		if !strings.HasPrefix(e.Reason, "guard stop") {
			return nil
		}
		return &RequestCard{Kind: "guard_stop", Text: "「" + e.Protocol + "」触护卫而停（气机过盛）。是否再行功？"}
	case EventOutOfBoundary:
		return &RequestCard{Kind: "out_of_boundary", Text: "「" + e.Protocol + "」欲引之气越出已验证边界。准其试探（有伤脉之险），抑或止之？"}
	case EventUnknownAnomaly:
		return &RequestCard{Kind: "unknown_anomaly", Text: "「" + e.Protocol + "」行功中生出分身无法解释的异象（似有新伤）。是否续行？"}
	}
	return nil
}

// ---------------------------------------------------------------------------
// 意图解析（玩家 JSON → ctmatch.Intent）
// ---------------------------------------------------------------------------

// playerIntent 是玩家意图的 JSON 形态。
type playerIntent struct {
	Kind    string `json:"kind"` // sense|absorb|guide|stop|inspect|hypothesis|resume
	Target  string `json:"target"`
	From    string `json:"from"`
	To      string `json:"to"`
	Element string `json:"element"`
	Level   string `json:"level"`
	Text    string `json:"text"`
}

var elementByName = map[string]int{
	"wood": cult.Wood, "木": cult.Wood,
	"fire": cult.Fire, "火": cult.Fire,
	"earth": cult.Earth, "土": cult.Earth,
	"metal": cult.Metal, "金": cult.Metal,
	"water": cult.Water, "水": cult.Water,
}

func parsePlayerIntent(p playerIntent) (Intent, bool, error) {
	it := Intent{Element: -1}
	oneShot := false
	switch strings.ToLower(strings.TrimSpace(p.Kind)) {
	case "sense":
		it.Kind, oneShot = IntentSense, true
	case "stop":
		it.Kind, oneShot = IntentStop, true
	case "inspect":
		it.Kind, oneShot = IntentInspect, true
		it.Acupoint = strings.TrimSpace(p.Target)
	case "hypothesis", "record_hypothesis":
		it.Kind, oneShot = IntentHypothesis, true
		it.Text = strings.TrimSpace(p.Text)
	case "absorb":
		it.Kind = IntentAbsorb
		it.Acupoint = strings.TrimSpace(p.Target)
		lv, err := ParseLevel(p.Level)
		if err != nil {
			return it, false, fmt.Errorf("absorb: %v", err)
		}
		it.Level = lv
	case "guide":
		it.Kind = IntentGuide
		it.From = strings.TrimSpace(p.From)
		it.To = strings.TrimSpace(p.To)
		if e, ok := elementByName[strings.ToLower(strings.TrimSpace(p.Element))]; ok {
			it.Element = e
		}
		lv, err := ParseLevel(p.Level)
		if err != nil {
			return it, false, fmt.Errorf("guide: %v", err)
		}
		it.Level = lv
	case "resume":
		// 恢复被护卫/出界暂停的功法（请示卡的"准"）。会话层特批。
		it.Kind, oneShot = IntentSense, true
	default:
		return it, false, fmt.Errorf("未知意图 %q", p.Kind)
	}
	return it, oneShot, nil
}

// applyPlayerIntents 落一批玩家意图：感知/止/内视/假说入队（一次性）；
// 动作意图并入常驻意图（同穴覆盖吸收、同路覆盖引流，其余追加）；
// 止同时清空常驻集并叫停 script 分身；准恢复暂停的功法并清请示。
func (h *cultHandle) applyPlayerIntents(list []playerIntent) error {
	t := h.sess.World.Tick
	var queued []Intent
	for _, pj := range list {
		it, oneShot, err := parsePlayerIntent(pj)
		if err != nil {
			return err
		}
		switch {
		case pj.Kind == "resume":
			h.sess.ResumeProtocol()
			h.pending = nil
			h.stream = append(h.stream, StreamLine{Tick: t, Kind: "voice", Text: "【本尊】准。分身续行前功。"})
			h.playerLog = append(h.playerLog, PlayerLine{Tick: t, Summary: "准许分身续行"})
		case it.Kind == IntentStop:
			h.currentIntents = nil
			if h.script != nil {
				h.script.stopped = true
			}
			h.pending = nil // "止"决一切待决请示
			queued = append(queued, it)
			h.playerLog = append(h.playerLog, PlayerLine{Tick: t, Summary: intentSummary(it)})
		case oneShot:
			queued = append(queued, it)
			h.playerLog = append(h.playerLog, PlayerLine{Tick: t, Summary: intentSummary(it)})
		default:
			// 动作意图：并入常驻集（覆盖同类，其余追加）。
			h.currentIntents = mergeIntent(h.currentIntents, it)
			h.playerLog = append(h.playerLog, PlayerLine{Tick: t, Summary: intentSummary(it)})
			// 玩家接管行功即重新唤起 script 分身。
			if h.script != nil {
				h.script.stopped = false
			}
		}
	}
	if len(queued) > 0 {
		h.sess.Queue = append(h.sess.Queue, queued...)
	}
	if len(h.playerLog) > 80 {
		h.playerLog = h.playerLog[len(h.playerLog)-80:]
	}
	return nil
}

// mergeIntent 把 it 并入常驻意图：同穴吸收覆盖、同路引流覆盖，其余追加。
func mergeIntent(cur []Intent, it Intent) []Intent {
	out := cur[:0:0]
	replaced := false
	for _, e := range cur {
		same := false
		switch {
		case it.Kind == IntentAbsorb && e.Kind == IntentAbsorb && e.Acupoint == it.Acupoint:
			same = true
		case it.Kind == IntentGuide && e.Kind == IntentGuide && e.From == it.From && e.To == it.To:
			same = true
		}
		if same && !replaced {
			out = append(out, it)
			replaced = true
			continue
		}
		if same && replaced {
			continue
		}
		out = append(out, e)
	}
	if !replaced {
		out = append(out, it)
	}
	return out
}

// ---------------------------------------------------------------------------
// 视图渲染
// ---------------------------------------------------------------------------

// view 渲染玩家界面视图（registry 锁外调用：handle 只被单请求写）。
func (h *cultHandle) view() *SessionView {
	s := h.sess
	v := &SessionView{
		ID:          h.id,
		Seed:        s.Seed,
		RuleVersion: s.World.RuleVersion,
		Tick:        s.World.Tick,
		Acuity:      1,
		Realm:       realmWord(s.Foundation),
		PilotKind:   h.kind,
		Active:      s.ActiveProtocol(),
	}

	// LLM 配置是否可用（分身是否请得动）。
	if lp, ok := s.Pilot.(*LLMPilot); ok {
		v.LLMAvailable = lp.cfg != nil
	} else {
		v.LLMAvailable = false
	}

	// 定性观察（现场重算：纯函数，确定性）。
	if s.Observe != nil {
		if obs := s.Observe(s.World); obs != nil {
			v.Env = envChips(obs.Env)
			for _, p := range obs.Points {
				v.Points = append(v.Points, PointView{
					Name: p.Name, Feelings: p.Feelings, Flow: p.Flow,
					Nature: p.Nature, Openness: p.Openness,
					Stability: p.Stability, Damage: p.Damage,
					PressureBand: p.PressureBand,
				})
			}
		}
	}

	// 身体图拓扑。
	g := &s.World.Character.BodyGraph
	v.Body.Points = make([]string, len(g.Acupoints))
	for i, a := range g.Acupoints {
		v.Body.Points[i] = a.Name
	}
	for _, c := range g.Channels {
		from, to := "", ""
		if c.From >= 0 && c.From < len(g.Acupoints) {
			from = g.Acupoints[c.From].Name
		}
		if c.To >= 0 && c.To < len(g.Acupoints) {
			to = g.Acupoints[c.To].Name
		}
		v.Body.Channels = append(v.Body.Channels, ChannelView{Name: c.Name, From: from, To: to})
	}

	// 感知流/回执/玩家志/请示：新置顶（倒序拷贝）。
	v.SenseStream = reverseLines(h.stream)
	v.Receipts = reverseReceipts(h.receipts)
	v.MyIntents = reversePlayers(h.playerLog)
	v.Requests = append([]RequestCard(nil), h.pending...)

	// 分身自述（LLM 修行笔记，新→旧）。
	if lp, ok := s.Pilot.(*LLMPilot); ok && lp != nil {
		for i := len(lp.Notes) - 1; i >= 0; i-- {
			v.Cognitions = append(v.Cognitions, lp.Notes[i])
		}
		if len(v.Cognitions) > 0 {
			v.SelfReport = v.Cognitions[0]
		}
	}
	if v.SelfReport == "" {
		v.SelfReport = "（分身入定，未留言。）"
	}

	// 证据账本与假说表（定性视图）。
	v.Ledger = s.Ledger.AgentView()
	for _, hyp := range s.Ledger.Hypotheses {
		v.Hypotheses = append(v.Hypotheses, HypView{
			Text: hyp.Text, Status: hyp.Status, Evidence: evidenceWord(hyp.Evidence),
		})
	}

	// 功法库。
	for _, p := range sortedProtocols(s) {
		pv := ProtoView{
			Name: p.Name, Status: p.Status, Path: protocolPath(p, g),
			Cycles: p.Cycles, Loop: false,
		}
		for _, gd := range p.Guards {
			pv.Guards = append(pv.Guards, guardText(gd, g))
		}
		for _, rg := range p.Ranges {
			pv.Ranges = append(pv.Ranges, RangeView{
				Channel: channelName(rg.Channel, g), Element: elementLabel(rg.Element),
				Min: rg.Min, Max: rg.Max,
			})
		}
		if s.active != nil && s.active.proto == p {
			pv.Active = true
			pv.Paused = s.active.paused
			pv.Loop = s.active.loopMode
			pv.Current = s.active.cycle
		}
		v.Protocols = append(v.Protocols, pv)
	}

	// 试跑卡。
	if s.Trial != nil && s.active != nil && s.active.proto == s.Trial.proto {
		tv := &TrialView{
			Protocol: s.Trial.Record.Protocol,
			Cycle:    len(s.Trial.Record.Cycles) + 1,
			Total:    s.Trial.proto.Cycles,
			Loop:     s.active.loopMode,
		}
		rec := s.Trial.Record.Cycles
		for i := len(rec) - 1; i >= 0 && len(tv.Recent) < 5; i-- {
			tv.Recent = append(tv.Recent, CycleLine{Cycle: rec[i].Cycle, Success: rec[i].Success, FailWhy: rec[i].FailWhy})
		}
		v.Trial = tv
	}

	// 筑基五关（五量 → 定性词 + band）。
	v.Foundation = foundationView(s.Foundation)

	normalizeNilSlices(v)
	return v
}

// normalizeNilSlices 把视图里的 nil 切片规整为空数组（nil 序列化为
// null，前端 .length 会炸——契约一律给 []）。
func normalizeNilSlices(v *SessionView) {
	if v.Env == nil {
		v.Env = []EnvChip{}
	}
	if v.Points == nil {
		v.Points = []PointView{}
	}
	for i := range v.Points {
		if v.Points[i].Feelings == nil {
			v.Points[i].Feelings = []string{}
		}
		if v.Points[i].Nature == nil {
			v.Points[i].Nature = []string{}
		}
	}
	if v.Body.Channels == nil {
		v.Body.Channels = []ChannelView{}
	}
	if v.Cognitions == nil {
		v.Cognitions = []string{}
	}
	if v.SenseStream == nil {
		v.SenseStream = []StreamLine{}
	}
	if v.Receipts == nil {
		v.Receipts = []ReceiptLine{}
	}
	if v.MyIntents == nil {
		v.MyIntents = []PlayerLine{}
	}
	if v.Requests == nil {
		v.Requests = []RequestCard{}
	}
	if v.Ledger == nil {
		v.Ledger = []string{}
	}
	if v.Hypotheses == nil {
		v.Hypotheses = []HypView{}
	}
	if v.Protocols == nil {
		v.Protocols = []ProtoView{}
	}
	for i := range v.Protocols {
		if v.Protocols[i].Guards == nil {
			v.Protocols[i].Guards = []string{}
		}
		if v.Protocols[i].Ranges == nil {
			v.Protocols[i].Ranges = []RangeView{}
		}
	}
	if v.Foundation == nil {
		v.Foundation = []FiveView{}
	}
}

// reverseLines / reverseReceipts / reversePlayers：新消息置顶。
func reverseLines(in []StreamLine) []StreamLine {
	out := make([]StreamLine, len(in))
	for i, l := range in {
		out[len(in)-1-i] = l
	}
	return out
}
func reverseReceipts(in []ReceiptLine) []ReceiptLine {
	out := make([]ReceiptLine, len(in))
	for i, l := range in {
		out[len(in)-1-i] = l
	}
	return out
}
func reversePlayers(in []PlayerLine) []PlayerLine {
	out := make([]PlayerLine, len(in))
	for i, l := range in {
		out[len(in)-1-i] = l
	}
	return out
}

func sortedProtocols(s *CultSession) []*Protocol {
	names := make([]string, 0, len(s.Protocols))
	for n := range s.Protocols {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]*Protocol, 0, len(names))
	for _, n := range names {
		out = append(out, s.Protocols[n])
	}
	return out
}

// envChips 把环境感知折成五行 chips（"木气浓郁"→ 元素 木 + 词 浓郁）。
func envChips(e EnvSense) []EnvChip {
	out := make([]EnvChip, cult.ElementCount)
	names := [...]string{"木", "火", "土", "金", "水"}
	for i := 0; i < cult.ElementCount; i++ {
		q := e.Qi[i]
		a := e.Activity[i]
		out[i] = EnvChip{
			Element:  names[i],
			Qi:       trimPrefix2(q),
			Activity: trimPrefix2(a),
		}
	}
	return out
}

// trimPrefix2 去掉观察词的两个前缀字（"木气"/"木性"），留下档位词。
func trimPrefix2(s string) string {
	r := []rune(s)
	if len(r) > 2 {
		return string(r[2:])
	}
	return s
}

// realmWord 是境界定性词（筑基判定器的会话级事实，零数字）。
func realmWord(f *FoundationMonitor) string {
	if f == nil {
		return "炼气"
	}
	switch {
	case f.Established:
		return "筑基"
	case f.Touched:
		return "炼气·门槛已触"
	}
	return "炼气"
}

// fiveWords 是筑基五关的定性措辞（弱→强，对齐设计稿措辞族）。
var fiveWords = []string{"未启", "微萌", "渐衡", "将成", "已圆"}

// fiveBand 把五量（千分位）折成 0-4 档（band 是允许的输出）。
func fiveBand(v int64) int {
	switch {
	case v <= 0:
		return 0
	case v < 250:
		return 1
	case v < 500:
		return 2
	case v < 750:
		return 3
	default:
		return 4
	}
}

// foundationView 渲染筑基五关（stage_3 §31-36 五量 → 定性）。
func foundationView(f *FoundationMonitor) []FiveView {
	if f == nil {
		return nil
	}
	m := f.Metrics()
	return []FiveView{
		{Key: "循环", Word: fiveWords[fiveBand(m.Closure)], Band: fiveBand(m.Closure)},
		{Key: "衡定", Word: fiveWords[fiveBand(m.RuleBalance)], Band: fiveBand(m.RuleBalance)},
		{Key: "自持", Word: fiveWords[fiveBand(m.SelfSustain)], Band: fiveBand(m.SelfSustain)},
		{Key: "守静", Word: fiveWords[fiveBand(m.DynamicStability)], Band: fiveBand(m.DynamicStability)},
		{Key: "内化", Word: fiveWords[fiveBand(m.Internalization)], Band: fiveBand(m.Internalization)},
	}
}

// protocolPath 由步骤合成路径行（功法配置，非真值）。
func protocolPath(p *Protocol, g *cult.BodyGraph) string {
	var parts []string
	last := ""
	appendPoint := func(n string) {
		if n != "" && n != last {
			parts = append(parts, n)
			last = n
		}
	}
	first := true
	for i := range p.Steps {
		st := &p.Steps[i]
		switch st.Kind {
		case StepAbsorb:
			if first {
				parts = append(parts, "环境")
				last = "环境"
			}
			appendPoint(pointName(st.PointIdx, g))
		case StepGuide:
			appendPoint(pointName(st.From, g))
			appendPoint(pointName(st.To, g))
		}
		first = false
	}
	if len(parts) == 0 {
		return "（未定路径）"
	}
	return strings.Join(parts, " → ")
}

func pointName(i int, g *cult.BodyGraph) string {
	if i >= 0 && i < len(g.Acupoints) {
		return g.Acupoints[i].Name
	}
	return "?"
}

func channelName(i int, g *cult.BodyGraph) string {
	if i >= 0 && i < len(g.Channels) {
		return g.Channels[i].Name
	}
	return "?"
}

func elementLabel(e int) string {
	if e < 0 || e >= cult.ElementCount {
		return "混合"
	}
	return [...]string{"木", "火", "土", "金", "水"}[e]
}

// guardText 把护卫折成定性一句话（条件数值是功法配置，但措辞从简）。
func guardText(gd Guard, g *cult.BodyGraph) string {
	act := "即止"
	if gd.Do == GuardReduceFlow {
		act = "减速"
	}
	metric := map[Metric]string{
		MetricPressure: "压高", MetricStrain: "劳损", MetricStability: "稳落",
		MetricDamage: "伤生", MetricFlowShock: "震荡", MetricLoadRatio: "载重",
		MetricCurrentFlow: "流急",
	}[gd.When.Metric]
	if metric == "" {
		metric = "有异"
	}
	return metric + act + "（" + pointName(gd.When.PointIdx, g) + "）"
}

// withDebug 在视图上挂调试载荷（真值；?debug=1 专用）。
func (h *cultHandle) withDebug(v *SessionView) *SessionView {
	s := h.sess
	d := &DebugView{Metrics: s.Foundation.Metrics(), FlowScale: s.FlowScale()}
	for i := range s.World.Character.BodyGraph.Acupoints {
		a := &s.World.Character.BodyGraph.Acupoints[i]
		q := int64(0)
		for e := 0; e < cult.ElementCount; e++ {
			q += a.Qi[e]
		}
		d.Points = append(d.Points, DebugPoint{Name: a.Name, Qi: q, Stability: a.Stability, Damage: a.Damage})
	}
	v.Debug = d
	return v
}

// ---------------------------------------------------------------------------
// HTTP 处理器（worldd 挂载）
// ---------------------------------------------------------------------------

func writeCultJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeCultErr(w http.ResponseWriter, status int, msg string) {
	writeCultJSON(w, status, map[string]string{"error": msg})
}

// HandleCreate POST /api/cult/session
// body: {"seed": 1, "pilot": "script"|"llm", "mode": "normal"|"tutorial"}
// mode 缺省 normal（功法库为空、探索型 script 分身）；tutorial 为演示
// 模式（预载《吐纳初诀》+ 固定走法）。
func (r *CultRegistry) HandleCreate(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Seed  uint64 `json:"seed"`
		Pilot string `json:"pilot"`
		Mode  string `json:"mode"`
	}
	_ = json.NewDecoder(req.Body).Decode(&body) // 空 body 允许
	v, err := r.Create(body.Seed, body.Pilot, body.Mode)
	if err != nil {
		writeCultErr(w, http.StatusBadRequest, err.Error())
		return
	}
	writeCultJSON(w, http.StatusCreated, v)
}

// debugEnabled 报告 debug 视图是否放行：仅当环境变量 AGENTBATTLE_DEV
// 为 "1" 或 "true" 时，?debug=1 才携带真值载荷；否则 debug 查询被静默
// 忽略（不报错——不暴露 debug 机制的存在）。默认关闭。
func debugEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AGENTBATTLE_DEV"))) {
	case "1", "true":
		return true
	}
	return false
}

// HandleView GET /api/cult/session/{id}[?debug=1]
func (r *CultRegistry) HandleView(w http.ResponseWriter, req *http.Request) {
	h := r.get(req.PathValue("id"))
	if h == nil {
		writeCultErr(w, http.StatusNotFound, "cult session not found")
		return
	}
	v := h.view()
	if debugEnabled() && req.URL.Query().Get("debug") == "1" {
		v = h.withDebug(v)
	}
	writeCultJSON(w, http.StatusOK, v)
}

// HandleTick POST /api/cult/session/{id}/tick
// body: {"ticks": 1, "intents": [...]}——intents 先落地（FromPlayer），
// 再推进；推进中心跳/升级自动唤醒 Pilot。
func (r *CultRegistry) HandleTick(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Ticks   int            `json:"ticks"`
		Intents []playerIntent `json:"intents"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		writeCultErr(w, http.StatusBadRequest, "bad tick body: "+err.Error())
		return
	}
	r.mu.Lock()
	h := r.sessions[req.PathValue("id")]
	if h == nil {
		r.mu.Unlock()
		writeCultErr(w, http.StatusNotFound, "cult session not found")
		return
	}
	if len(body.Intents) > 0 {
		if err := h.applyPlayerIntents(body.Intents); err != nil {
			r.mu.Unlock()
			writeCultErr(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	h.step(body.Ticks)
	v := h.view()
	r.mu.Unlock()
	writeCultJSON(w, http.StatusOK, v)
}

// HandleIntents POST /api/cult/session/{id}/intents
// body: {"intents": [...]}——只落意图不推进（下个 Tick 生效）。
func (r *CultRegistry) HandleIntents(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Intents []playerIntent `json:"intents"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || len(body.Intents) == 0 {
		writeCultErr(w, http.StatusBadRequest, "bad intents")
		return
	}
	r.mu.Lock()
	h := r.sessions[req.PathValue("id")]
	if h == nil {
		r.mu.Unlock()
		writeCultErr(w, http.StatusNotFound, "cult session not found")
		return
	}
	if err := h.applyPlayerIntents(body.Intents); err != nil {
		r.mu.Unlock()
		writeCultErr(w, http.StatusBadRequest, err.Error())
		return
	}
	v := h.view()
	r.mu.Unlock()
	writeCultJSON(w, http.StatusOK, v)
}

// HandleGuidance POST /api/cult/session/{id}/guidance
// body: {"kind": "whisper"|"guide"|"deeplink", "text": "..."}——本 WP
// 简化为注入 Pilot 下次征询载荷的 guidance 段（llm 分身），三级只影响标注。
func (r *CultRegistry) HandleGuidance(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Kind string `json:"kind"`
		Text string `json:"text"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || strings.TrimSpace(body.Text) == "" {
		writeCultErr(w, http.StatusBadRequest, "bad guidance")
		return
	}
	kindWord := map[string]string{"whisper": "轻语", "guide": "引导", "deeplink": "深谈"}[body.Kind]
	if kindWord == "" {
		kindWord = "传音"
	}
	r.mu.Lock()
	h := r.sessions[req.PathValue("id")]
	if h == nil {
		r.mu.Unlock()
		writeCultErr(w, http.StatusNotFound, "cult session not found")
		return
	}
	if lp, ok := h.sess.Pilot.(*LLMPilot); ok && lp != nil {
		lp.GiveGuidance(kindWord, body.Text)
	}
	h.stream = append(h.stream, StreamLine{
		Tick: h.sess.World.Tick, Kind: "voice",
		Text: "【本尊·" + kindWord + "】" + body.Text,
	})
	v := h.view()
	r.mu.Unlock()
	writeCultJSON(w, http.StatusOK, v)
}

// HandleAsk POST /api/cult/session/{id}/ask
// body: {"question": "..."}——LLM 可用时向分身提问；否则降级应答。
func (r *CultRegistry) HandleAsk(w http.ResponseWriter, req *http.Request) {
	var body struct {
		Question string `json:"question"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || strings.TrimSpace(body.Question) == "" {
		writeCultErr(w, http.StatusBadRequest, "bad question")
		return
	}
	r.mu.Lock()
	h := r.sessions[req.PathValue("id")]
	if h == nil {
		r.mu.Unlock()
		writeCultErr(w, http.StatusNotFound, "cult session not found")
		return
	}
	lp, ok := h.sess.Pilot.(*LLMPilot)
	if !ok || lp == nil || lp.cfg == nil {
		r.mu.Unlock()
		writeCultJSON(w, http.StatusOK, map[string]any{
			"answer": "", "degraded": true,
			"reason": "分身无法应答：未接灵机（LLM 未配置）。它仍在凭感知修行。",
		})
		return
	}
	obs := (*Observation)(nil)
	if h.sess.Observe != nil {
		obs = h.sess.Observe(h.sess.World)
	}
	ans, err := lp.Ask(obs, h.sess.Ledger.AgentView(), body.Question)
	v := h.view()
	r.mu.Unlock()
	if err != nil {
		writeCultJSON(w, http.StatusOK, map[string]any{"answer": "", "degraded": true, "reason": "分身一时语塞：" + err.Error()})
		return
	}
	writeCultJSON(w, http.StatusOK, map[string]any{"answer": ans, "degraded": false, "view": v})
}

// HandleProtocols POST /api/cult/session/{id}/protocols
// body: {"yaml": "..."}——编译并收录一份功法（静态校验，非法即拒）。
func (r *CultRegistry) HandleProtocols(w http.ResponseWriter, req *http.Request) {
	var body struct {
		YAML string `json:"yaml"`
	}
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil || strings.TrimSpace(body.YAML) == "" {
		writeCultErr(w, http.StatusBadRequest, "bad protocol body (want yaml)")
		return
	}
	r.mu.Lock()
	h := r.sessions[req.PathValue("id")]
	if h == nil {
		r.mu.Unlock()
		writeCultErr(w, http.StatusNotFound, "cult session not found")
		return
	}
	p, err := Compile([]byte(body.YAML), &h.sess.World.Character.BodyGraph)
	if err != nil {
		r.mu.Unlock()
		writeCultErr(w, http.StatusBadRequest, err.Error())
		return
	}
	h.sess.LoadProtocol(p)
	h.stream = append(h.stream, StreamLine{
		Tick: h.sess.World.Tick, Kind: "event",
		Text: "「" + p.Name + "」录入功法册（草创）。",
	})
	v := h.view()
	r.mu.Unlock()
	writeCultJSON(w, http.StatusCreated, v)
}

// HandleStartProtocol POST /api/cult/session/{id}/protocols/{name}/start
// 起功（需至少推进过一个 Tick：前置条件要有观测基准）。
func (r *CultRegistry) HandleStartProtocol(w http.ResponseWriter, req *http.Request) {
	r.mu.Lock()
	h := r.sessions[req.PathValue("id")]
	if h == nil {
		r.mu.Unlock()
		writeCultErr(w, http.StatusNotFound, "cult session not found")
		return
	}
	name := req.PathValue("name")
	if err := h.sess.StartProtocol(name); err != nil {
		r.mu.Unlock()
		writeCultErr(w, http.StatusBadRequest, err.Error())
		return
	}
	// 起功事件由会话层记 EventProtocolStarted，下个 Tick 随事件流入流——
	// 此处不再重复写感知流。
	v := h.view()
	r.mu.Unlock()
	writeCultJSON(w, http.StatusOK, v)
}
