// Package match glues a scenario pack, the engine and agent policies
// into one playable session: turn loop, Influence economy, guidance,
// prediction, ask, end-of-match review, and win/lose adjudication
// (strage_1 MVP + player_agent_relationship).
package match

import (
	"errors"
	"fmt"

	"agentbattle/backend/agent"
	"agentbattle/backend/engine"
	"agentbattle/backend/profile"
	"agentbattle/backend/scenario"
)

// Status values of a match.
const (
	StatusRunning = "running"
	StatusWon     = "won"
	StatusLost    = "lost"
	StatusTimeout = "timeout"
)

// Influence economy (strage_1 §四 数值表 v0.1).
const (
	InfluenceStart = 5.0
	InfluenceCap   = 5.0
	InfluenceRegen = 1.0 // 每 10 回合
	RegenEvery     = 10
	CostWhisper    = 0.5 // 轻度建议，可忽略
	CostGuide      = 1.0 // 明确方向，重新评估
	CostDeepLink   = 2.0 // 重新评估整个战略
	CostInform     = 1.0 // 提供玩家推断
	CostPause      = 3.0 // 紧急暂停，每局一次
	PredictReward  = 0.5 // 猜对奖励
)

// Guidance kinds → costs. 玩家永远发偏好，不发动作。
var GuidanceCosts = map[string]float64{
	"whisper":     CostWhisper,
	"guide":       CostGuide,
	"deeplink":    CostDeepLink,
	"information": CostInform,
}

// Guidance is one player intervention (player_agent_relationship §5).
type Guidance struct {
	Turn int64  `json:"turn"`
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// Prediction is the player's guess about the agent's next action
// (strage_1 五动作：Predict，猜对 +0.5).
type Prediction struct {
	Turn     int64  `json:"turn"`
	Choice   string `json:"choice"`
	Resolved bool   `json:"resolved"`
	Actual   string `json:"actual,omitempty"`
	Hit      bool   `json:"hit"`
}

// Prediction choices and their matchers.
var PredictionChoices = []string{"move", "sense", "attack", "pickup", "assemble", "wait"}

// QA is one player question and the agent's answer.
type QA struct {
	Turn     int64  `json:"turn"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// Review is the end-of-match summary + distilled lessons
// (player_agent_relationship §18).
type Review struct {
	Summary string   `json:"summary"`
	Lessons []string `json:"lessons"`
}

// v04PackName selects the v0.4 infinite-world pack: only there does the
// chunk lazy-activation protocol run (scenario/infinite.go).
const v04PackName = "scenario/infinite-0.4"

// Match is one session of one scenario pack.
type Match struct {
	ID        string
	Seed      uint64
	PackName  string
	W         *engine.World
	TurnLimit int64
	Spawn     engine.Hex

	PlayerID   engine.EntityID
	UnknownID  engine.EntityID
	FragmentID engine.EntityID
	// MonsterIDs lists the hand-placed monsters (v0.2); empty for mvp.
	MonsterIDs []engine.EntityID

	Pilot     agent.DecisionMaker
	PilotName string // "llm" | "scripted"

	Status string

	Explored    map[engine.Hex]bool
	Events      []engine.Event
	Guidance    []Guidance
	Predictions []*Prediction
	pending     *Prediction
	QA          []QA
	Cognition   *agent.Cognition
	ReviewRes   *Review

	Influence  float64
	pausedOnce bool
	pauseNext  bool

	// activatedChunks 是 v0.4 怪物懒激活的"已激活 chunk 集合"——调用方
	// 持有的确定性状态（design §2.5：纯函数 + 调用方持状态），进重放
	// 语义：同 seed + 同 Intent 流 → 同一激活序列。scenario 侧的
	// PendingSpawns 是纯函数，不持任何跨对局状态.
	activatedChunks map[[2]int]bool

	// —— v0.4 LLM 介入层 (design §3) ——
	// cls 的全部去抖状态是确定性字段；TriggerLog 记录每次触发原因，
	// lastTrigger 是最近一次触发（View.last_trigger），llmCalls 是本局
	// pilot 决策调用计数（LLM pilot 下即 LLM 调用数，预算口径 ≤50/局）。
	cls         *EventClassifier
	TriggerLog  []string
	lastTrigger string
	llmCalls    int

	profile     *profile.Store
	unknownDir  engine.Hex
	lastEvs     []engine.Event
	// lastGatherEvs 是上一 tick 采集结算生成的分类器镜像事件（gather →
	// pickup，见 resolveGathers）；只进 classifyTick 的喂入，不进事件流.
	lastGatherEvs []engine.Event
	lastIntents   []engine.Intent
}

// New creates a match of scenario/mvp-0.1. The pilot is the LLM agent
// when AGENTBATTLE_LLM_* is configured; the scripted agent otherwise.
func New(id string, seed uint64, prof *profile.Store) *Match {
	return newMatch(id, seed, prof, scenario.MVP(seed))
}

// NewV02 creates a match of scenario/v02-0.2: the hex-translated v0.2
// field with three hand-placed monsters in addition to the Unknown Agent.
func NewV02(id string, seed uint64, prof *profile.Store) *Match {
	return newMatch(id, seed, prof, scenario.V02(seed))
}

// NewV04 creates a match of scenario/infinite-0.4: the v0.4 minimal
// infinite-world pack (design §2.4).
func NewV04(id string, seed uint64, prof *profile.Store) *Match {
	return newMatch(id, seed, prof, scenario.V04(seed))
}

func newMatch(id string, seed uint64, prof *profile.Store, pack *scenario.Pack) *Match {
	w, player, unknown, frag, monsters := pack.Genesis()
	m := &Match{
		ID: id, Seed: seed, PackName: pack.Name,
		W: w, TurnLimit: pack.TurnLimit, Spawn: pack.PlayerSpawn,
		PlayerID: player, UnknownID: unknown, FragmentID: frag,
		MonsterIDs: monsters,
		Explored:   map[engine.Hex]bool{},
		Status:     StatusRunning,
		Influence:  InfluenceStart,
		profile:    prof,
		cls:        NewEventClassifier(),
		unknownDir: engine.HexDirs[0], // E：六方向固定顺序游荡

		activatedChunks: map[[2]int]bool{}, // v0.4 懒激活：调用方持状态
	}
	if cfg := agent.LLMFromEnv(); cfg != nil {
		p := agent.NewLLM(*cfg)
		if prof != nil {
			p.SetProfile(prof.P.PersonaText(), prof.P.EnabledLessons())
		}
		m.Pilot, m.PilotName = p, "llm"
	} else {
		m.Pilot = agent.NewScripted(pack.PlayerSpawn, pack.FragmentPos, pack.Passable)
		m.PilotName = "scripted"
	}
	return m
}

// Tick advances one turn with explicit player intents.
func (m *Match) Tick(intents []engine.Intent) {
	if m.Status != StatusRunning {
		return
	}
	acts := m.monstersAct()
	// v0.5 WPA 经济循环：指向晶簇的 transfer 不进引擎（引擎 resolveTransfer
	// 只认 fragment），在 match 层于推进后结算采集
	order, gathers := m.splitGathers(intents)
	acts[m.PlayerID] = order
	evs := m.W.Advance(acts)
	evs = append(evs, m.resolveGathers(gathers)...)
	evs = append(evs, m.lazySpawns()...) // v0.4 怪物/晶簇懒激活进事件流
	m.lastEvs = evs
	m.lastIntents = intents
	m.Events = append(m.Events, evs...)
	if obs := m.W.Observe(m.PlayerID, nil); obs != nil { // 形散后无感知
		for _, p := range obs.Visible {
			m.Explored[p] = true
		}
	}
	m.regenInfluence()
	m.judgePending()
	m.checkEnd()
}

// lazySpawns runs the v0.4 chunk lazy-activation protocol
// (scenario/infinite.go 文件头【match 接入点】): 在玩家所在 chunk 的 3×3
// 激活圈内覆盖尚未激活的 chunk，把出怪/出晶簇 chunk 的激活事件（Entity
// 回填为新实体 id 后）并入本回合事件流，并把覆盖到的 chunk 键记入
// activatedChunks（无论是否出怪/晶簇——"只激活一次"对 chunk 成立）。
// PendingSpawns 是纯函数，激活判定状态完全由 m.activatedChunks 持有，
// 懒激活是 (seed, chunk, activated 演化) 的确定结果，重放不受影响
// (design §2.5)。仅 v0.4 包接线。
func (m *Match) lazySpawns() []engine.Event {
	if m.PackName != v04PackName {
		return nil
	}
	p := m.W.Entities[m.PlayerID]
	if p == nil {
		return nil
	}
	if m.activatedChunks == nil {
		m.activatedChunks = map[[2]int]bool{}
	}
	var evs []engine.Event
	for _, cs := range scenario.PendingSpawns(m.W, p.Pos, m.activatedChunks) {
		m.activatedChunks[[2]int{cs.QC, cs.RC}] = true
		for _, ps := range []*scenario.PendingSpawn{cs.Spawn, cs.Resource} {
			if ps == nil {
				continue // 覆盖到位但该 chunk 无此物
			}
			e := m.W.Spawn(ps.Entity)
			ev := ps.Event
			ev.Entity = e.ID // 实体 id 在 Spawn 后才存在
			evs = append(evs, ev)
		}
	}
	return evs
}

// splitGathers partitions the player's intents into engine-bound intents
// and resource-gather requests：transfer 指向 Kind "resource" 的实体时，
// 引擎 resolveTransfer 会以"只能搬取元气结晶"拒绝（engine 冻结）——晶
// 簇采集在 match 层结算，剥离后不进引擎。指向 fragment 的 transfer 与
// 其余意图原样放行。
func (m *Match) splitGathers(intents []engine.Intent) (rest, gathers []engine.Intent) {
	for _, in := range intents {
		if in.Type == "transfer" {
			if t := m.W.Entities[in.Target]; t != nil && t.Kind == "resource" {
				gathers = append(gathers, in)
				continue
			}
		}
		rest = append(rest, in)
	}
	return rest, gathers
}

// resolveGathers settles the stripped gather requests（在 Advance 之后结
// 算：晶簇可能被本 tick 的溢出波及形散，先推进后采集）. 成功：实体从
// 世界移除、物料入库（w.Materials["player"]）、事件 "gather"。
//
// 同时为事件分类器生成 pickup 镜像事件（m.lastGatherEvs）：events.go 冻
// 结，resource 触发通道只认 Type "pickup" 的 pickupSeen 去抖——镜像让
// 首次采集晶簇与首次拾取结晶协同共用同一个"首次拾取"。镜像只进
// classifyTick 的喂入，不进事件流与观察。
func (m *Match) resolveGathers(gathers []engine.Intent) []engine.Event {
	m.lastGatherEvs = nil
	var evs []engine.Event
	for _, in := range gathers {
		p := m.W.Entities[m.PlayerID]
		if p == nil {
			return evs // 形散：无从采集
		}
		res := m.W.Entities[in.Target]
		switch {
		case res == nil || res.Kind != "resource":
			evs = append(evs, engine.Event{Tick: m.W.Tick, Type: "intent_rejected",
				Entity: p.ID, Text: "晶簇已消散，无从采集"})
		case res.Pos != p.Pos:
			evs = append(evs, engine.Event{Tick: m.W.Tick, Type: "intent_rejected",
				Entity: p.ID, Text: "必须站在晶簇所在格"})
		default:
			ess := res.Essence
			delete(m.W.Entities, res.ID)
			inv := m.W.Materials["player"]
			if inv == nil {
				inv = map[engine.Essence]int{}
				m.W.Materials["player"] = inv
			}
			inv[ess]++
			evs = append(evs, engine.Event{Tick: m.W.Tick, Type: "gather", Entity: p.ID,
				Text: fmt.Sprintf("采集 %s×1，仓库存料 %d", ess, inv[ess])})
			m.lastGatherEvs = append(m.lastGatherEvs,
				engine.Event{Tick: m.W.Tick, Type: "pickup", Entity: p.ID})
		}
	}
	return evs
}

// orderPilot is a pilot that speaks in standing move_to orders (执行层,
// design §3.1/§3.4 条 1)：引擎逐 tick 自动步进，两次事件之间零次决策；
// pilot 只在 EventClassifier 唤醒时被咨询。
type orderPilot interface {
	agent.DecisionMaker
	DecideOrder(obs *engine.Observation) []engine.Intent
}

// TickAuto advances one turn under the v0.4 four-layer event-driven
// model (design §3)：EventClassifier 判定本 tick 是否值得一次决策。
//
// 不触发 → 不调用 pilot，仅推进引擎（执行层订单照走、怪物照动、环境
// 照常），0 次调用；触发 → 唤醒 pilot，触发原因经 SetTrigger 进入
// LLM 的触发语境与 user payload（scripted pilot 以 move_to 订单应答）。
func (m *Match) TickAuto() {
	if m.Status != StatusRunning {
		return
	}
	if m.pauseNext { // 紧急暂停：Agent 本回合待命（strage_1 影响表）
		m.pauseNext = false
		m.Tick(nil)
		return
	}
	trigger := m.classifyTick()
	if trigger == "" { // 执行层：0 次调用
		m.Tick(nil)
		return
	}
	m.lastTrigger = trigger
	m.TriggerLog = append(m.TriggerLog, trigger)
	m.llmCalls++ // 计数口径：本局 pilot 决策调用数（LLM pilot 下即 LLM 调用数）

	if la, ok := m.Pilot.(*agent.LLMAgent); ok {
		la.SetGuidance(m.guidanceNotes())
		la.SetTrigger(trigger)
		obs := m.W.Observe(m.PlayerID, m.lastEvs)
		m.Tick(la.DecideTriggered(obs))
		if c := la.LastCognition(); c != nil {
			m.Cognition = c
		}
		return
	}
	obs := m.W.Observe(m.PlayerID, m.lastEvs)
	if op, ok := m.Pilot.(orderPilot); ok {
		m.Tick(op.DecideOrder(obs))
		return
	}
	m.Tick(m.Pilot.Decide(obs))
}

// classifyTick feeds the classifier with the facts of the tick just
// ended（真实事件流 + 采集的 pickup 镜像，见 resolveGathers）.
func (m *Match) classifyTick() string {
	return m.cls.Classify(ClassifyInput{
		Tick:            m.W.Tick,
		PlayerID:        m.PlayerID,
		Events:          m.classifierEvents(),
		Self:            m.W.Entities[m.PlayerID],
		VisibleHostiles: m.visibleHostiles(),
		InCombat:        m.InCombat(),
	})
}

// classifierEvents is the classifier's feed for the last tick: the real
// event stream plus the gather→pickup mirror events.
func (m *Match) classifierEvents() []engine.Event {
	if len(m.lastGatherEvs) == 0 {
		return m.lastEvs
	}
	return append(append([]engine.Event{}, m.lastEvs...), m.lastGatherEvs...)
}

// visibleHostiles lists the hostile ids inside the player's current
// perception radius (被动 1，感识开启 4)，按 hostiles() 固定序。
func (m *Match) visibleHostiles() []engine.EntityID {
	p := m.W.Entities[m.PlayerID]
	if p == nil {
		return nil
	}
	radius := 1
	if s := p.FindComponent(engine.CSensor); s != nil && s.Active {
		radius = 4
	}
	var out []engine.EntityID
	for _, id := range m.hostiles() {
		if e := m.W.Entities[id]; e != nil && engine.HexDist(p.Pos, e.Pos) <= radius {
			out = append(out, id)
		}
	}
	return out
}

func (m *Match) guidanceNotes() []agent.GuidanceNote {
	notes := make([]agent.GuidanceNote, 0, len(m.Guidance))
	for _, g := range m.Guidance {
		notes = append(notes, agent.GuidanceNote{Turn: g.Turn, Kind: g.Kind, Text: g.Text})
	}
	return notes
}

// Observe returns the player agent's current perception.
func (m *Match) Observe() *engine.Observation {
	return m.W.Observe(m.PlayerID, m.lastEvs)
}

// AddGuidance records and charges one player intervention.
func (m *Match) AddGuidance(kind, text string) error {
	cost, ok := GuidanceCosts[kind]
	if !ok {
		return fmt.Errorf("未知的 Guidance 类型 %q", kind)
	}
	if m.Influence < cost {
		return fmt.Errorf("Influence 不足（需 %.1f，余 %.1f）", cost, m.Influence)
	}
	m.Influence -= cost
	m.Guidance = append(m.Guidance, Guidance{Turn: m.W.Tick, Kind: kind, Text: text})
	m.cls.ArmGuidance() // Guidance 层：下一 tick 必触发一次（design §3.1）
	return nil
}

// Pause executes the once-per-match emergency pause (3.0).
func (m *Match) Pause() error {
	if m.pausedOnce {
		return errors.New("紧急暂停每局只能使用一次")
	}
	if m.Influence < CostPause {
		return fmt.Errorf("Influence 不足（需 %.1f，余 %.1f）", CostPause, m.Influence)
	}
	m.Influence -= CostPause
	m.pausedOnce = true
	m.pauseNext = true
	return nil
}

// Predict registers the player's guess about the agent's next action.
func (m *Match) Predict(choice string) error {
	valid := false
	for _, c := range PredictionChoices {
		if c == choice {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("未知的预测选项 %q", choice)
	}
	if m.pending != nil && !m.pending.Resolved {
		m.pending.Choice = choice // 覆盖未结算的预测
		return nil
	}
	m.pending = &Prediction{Turn: m.W.Tick, Choice: choice}
	m.Predictions = append(m.Predictions, m.pending)
	return nil
}

// Ask lets the player question the agent (免费；不改变决策状态).
func (m *Match) Ask(question string) (string, error) {
	la, ok := m.Pilot.(*agent.LLMAgent)
	if !ok {
		return "", errors.New("Ask 需要 LLM pilot")
	}
	ans := la.Ask(m.Observe(), question)
	if ans == "" {
		return "", errors.New("Agent 暂时无法回答")
	}
	m.QA = append(m.QA, QA{Turn: m.W.Tick, Question: question, Answer: ans})
	return ans, nil
}

func (m *Match) regenInfluence() {
	if m.W.Tick%RegenEvery == 0 && m.Influence < InfluenceCap {
		m.Influence = minF(InfluenceCap, m.Influence+InfluenceRegen)
	}
}

// judgePending settles the pending prediction against what the agent
// actually tried to do this tick.
func (m *Match) judgePending() {
	p := m.pending
	if p == nil || p.Resolved || p.Turn >= m.W.Tick {
		return
	}
	p.Resolved = true
	p.Actual = m.classifyIntents()
	if p.Actual == p.Choice {
		p.Hit = true
		m.Influence = minF(InfluenceCap, m.Influence+PredictReward)
	}
	m.pending = nil
}

func (m *Match) classifyIntents() string {
	player := m.W.Entities[m.PlayerID]
	if player == nil {
		return "wait"
	}
	if len(m.lastIntents) == 0 {
		return "wait"
	}
	for _, in := range m.lastIntents {
		switch in.Type {
		case "move", "move_to": // WP3：订单也是"移动"的预测口径
			return "move"
		case "transfer":
			return "pickup"
		case "assemble":
			return "assemble"
		case "equip": // WPB：equip 是装配类意图（与 assemble 同级），预测口径并入同类创造行为
			return "assemble"
		case "activate":
			if c := player.Component(in.Component); c != nil {
				if c.Type == engine.CSensor {
					return "sense"
				}
				if c.Type == engine.CEmitter {
					return "attack"
				}
			}
		}
	}
	return "other"
}

func (m *Match) checkEnd() {
	p := m.W.Entities[m.PlayerID]
	if p != nil && carries(p, m.FragmentID) && p.Pos == m.Spawn {
		m.Status = StatusWon
	} else if p == nil || p.Integrity <= 0 {
		m.Status = StatusLost
	} else if m.W.Tick >= m.TurnLimit {
		m.Status = StatusTimeout
	} else {
		return
	}
	m.buildReview()
}

// buildReview runs once at match end: LLM 复盘 + 经验教训入 Profile
// （养成的不只是角色，还有驱动角色的 Agent）。
func (m *Match) buildReview() {
	rev := &Review{Summary: m.basicSummary(), Lessons: nil}
	if la, ok := m.Pilot.(*agent.LLMAgent); ok {
		payload := map[string]any{
			"status": m.Status, "turn": m.W.Tick,
			"events":      tail(m.Events, 60),
			"guidance":    m.Guidance,
			"predictions": m.Predictions,
		}
		s, lessons := la.Review(payload)
		if s != "" {
			rev.Summary = s
		}
		rev.Lessons = lessons
		if m.profile != nil && len(lessons) > 0 {
			m.profile.AddLessons(lessons)
		}
	}
	m.ReviewRes = rev
}

func (m *Match) basicSummary() string {
	p := m.W.Entities[m.PlayerID]
	integ := 0
	if p != nil {
		integ = p.Integrity
	}
	return fmt.Sprintf("%s：第 %d 回合结束，Agent 余形 %d，共发生事件 %d 条。",
		m.Status, m.W.Tick, integ, len(m.Events))
}

func tail[T any](s []T, n int) []T {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func carries(e *engine.Entity, id engine.EntityID) bool {
	for _, c := range e.Carrying {
		if c == id {
			return true
		}
	}
	return false
}

// unknownAct is the Unknown Agent's v0.1 behaviour — a deterministic
// stand-in for its Logic DSL (world_genesis §5.2): wander the infinite
// plain along a rotating hex direction; flee if the player gets close.
// Same seed → same walk.
func (m *Match) unknownAct() []engine.Intent {
	u := m.W.Entities[m.UnknownID]
	if u == nil {
		return nil
	}
	try := func(p engine.Hex) bool {
		return m.W.At(p).Passable && m.W.EntityAt(p) == nil
	}
	if p := m.W.Entities[m.PlayerID]; p != nil && engine.HexDist(p.Pos, u.Pos) <= 2 {
		// 逃离：按"远离玩家的距离"降序尝试六邻格（固定次序打破平局）。
		cands := engine.HexStepToward(u.Pos, p.Pos)
		for i := 5; i >= 0; i-- {
			if cand := cands[i]; cand != u.Pos && try(cand) {
				return []engine.Intent{{Type: "move", To: &cand, Power: 600}}
			}
		}
	}
	for i := 0; i < 6; i++ {
		cand := u.Pos.Add(m.unknownDir)
		if try(cand) {
			return []engine.Intent{{Type: "move", To: &cand, Power: 400}}
		}
		m.unknownDir = rotate(m.unknownDir)
	}
	return nil
}

// hostiles lists every hostile entity id (the Unknown Agent + all
// monsters), in deterministic order.
func (m *Match) hostiles() []engine.EntityID {
	ids := make([]engine.EntityID, 0, 1+len(m.MonsterIDs))
	if m.UnknownID != 0 {
		ids = append(ids, m.UnknownID)
	}
	return append(ids, m.MonsterIDs...)
}

// monstersAct gathers each hostile entity's intents for one tick: the
// Unknown Agent keeps its v0.1 behaviour, monsters follow the M3.1
// Logic-DSL stand-in below. Same seed → same intents.
func (m *Match) monstersAct() map[engine.EntityID][]engine.Intent {
	acts := map[engine.EntityID][]engine.Intent{}
	if a := m.unknownAct(); len(a) > 0 {
		acts[m.UnknownID] = a
	}
	for _, id := range m.MonsterIDs {
		if a := m.monsterAct(id); len(a) > 0 {
			acts[id] = a
		}
	}
	return acts
}

// monsterAct is the v0.2 monster behaviour — a deterministic stand-in
// for its Logic DSL (world_genesis §5.2 铁律二：敌人走 Agent Protocol).
// At most one intent per tick:
//
//	hex 相邻        → 攻击：activate 离火作用于玩家（power 500）
//	距离 ≤ 4        → 追击：朝玩家方向移动一格（不可穿障碍，可入危险区）
//	距离 > 4        → 游荡：Rng 驱动，原地停留概率 1/3
//
// (the 4-cell band closes the spec's 3..4 gap: chase up to and
// including distance 4, wander strictly beyond).
func (m *Match) monsterAct(id engine.EntityID) []engine.Intent {
	mon := m.W.Entities[id]
	if mon == nil {
		return nil
	}
	p := m.W.Entities[m.PlayerID]
	if p == nil {
		return nil
	}
	try := func(pos engine.Hex) bool { // 不可穿障碍；危险区可入（Passable）
		return m.W.At(pos).Passable && m.W.EntityAt(pos) == nil
	}
	// 相邻（HexDist == 1，六方向等价）：攻击
	if engine.Adjacent(p.Pos, mon.Pos) {
		if em := mon.FindComponent(engine.CEmitter); em != nil {
			return []engine.Intent{{Type: "activate", Component: em.ID, Target: p.ID, Power: 500}}
		}
	}
	switch dist := engine.HexDist(p.Pos, mon.Pos); {
	case dist <= 4: // 追击：贪心六方向（HexDist 递减排布，固定次序破平局）
		for _, cand := range engine.HexStepToward(mon.Pos, p.Pos) {
			if cand != mon.Pos && try(cand) {
				return []engine.Intent{{Type: "move", To: &cand, Power: 400}}
			}
		}
		return nil // 被地形卡死：本回合驻足
	default: // 游荡（dist > 4）
		if m.W.Rng.Intn(3) == 0 { // 原地停留概率 1/3
			return nil
		}
		start := m.W.Rng.Intn(6)
		for i := 0; i < 6; i++ {
			cand := mon.Pos.Add(engine.HexDirs[(start+i)%6])
			if try(cand) {
				return []engine.Intent{{Type: "move", To: &cand, Power: 400}}
			}
		}
		return nil
	}
}

// InCombat reports whether the player is in a fight this tick: any
// hostile within HexDist 2, or a player/hostile acted upon by an
// attack, backlash or spill in the most recent tick's events.
func (m *Match) InCombat() bool {
	p := m.W.Entities[m.PlayerID]
	if p != nil {
		for _, id := range m.hostiles() {
			if e := m.W.Entities[id]; e != nil && engine.HexDist(p.Pos, e.Pos) <= 2 {
				return true
			}
		}
	}
	hostile := map[engine.EntityID]bool{m.PlayerID: true, m.UnknownID: true}
	for _, id := range m.MonsterIDs {
		hostile[id] = true
	}
	for _, ev := range m.lastEvs {
		switch ev.Type {
		case "attack", "backlash", "spill":
			if hostile[ev.Entity] {
				return true
			}
		}
	}
	return false
}

// rotate advances a hex direction to the next in the fixed HexDirs
// order (E→NE→NW→W→SW→SE→E).
func rotate(d engine.Hex) engine.Hex {
	for i, dir := range engine.HexDirs {
		if dir == d {
			return engine.HexDirs[(i+1)%6]
		}
	}
	return engine.HexDirs[0]
}

// Summary returns a compact one-line state for logs and tests.
func (m *Match) Summary() string {
	p := m.W.Entities[m.PlayerID]
	pos, integ, batt := "—", 0, 0
	if p != nil {
		pos, integ, batt = fmt.Sprintf("%v", p.Pos), p.Integrity, p.Battery
	}
	return fmt.Sprintf("turn=%d status=%s player=%s 形=%d 气=%d", m.W.Tick, m.Status, pos, integ, batt)
}
