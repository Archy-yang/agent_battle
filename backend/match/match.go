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
	InfluenceStart  = 5.0
	InfluenceCap    = 5.0
	InfluenceRegen  = 1.0 // 每 10 回合
	RegenEvery      = 10
	CostWhisper     = 0.5 // 轻度建议，可忽略
	CostGuide       = 1.0 // 明确方向，重新评估
	CostDeepLink    = 2.0 // 重新评估整个战略
	CostInform      = 1.0 // 提供玩家推断
	CostPause       = 3.0 // 紧急暂停，每局一次
	PredictReward   = 0.5 // 猜对奖励
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

// Match is one session of one scenario pack.
type Match struct {
	ID        string
	Seed      uint64
	PackName  string
	W         *engine.World
	TurnLimit int64
	Spawn     engine.Vec2

	PlayerID   engine.EntityID
	UnknownID  engine.EntityID
	FragmentID engine.EntityID

	Pilot     agent.DecisionMaker
	PilotName string // "llm" | "scripted"

	Status string

	Explored   map[engine.Vec2]bool
	Events     []engine.Event
	Guidance   []Guidance
	Predictions []*Prediction
	pending    *Prediction
	QA         []QA
	Cognition  *agent.Cognition
	ReviewRes  *Review

	Influence  float64
	pausedOnce bool
	pauseNext  bool

	profile     *profile.Store
	unknownDir  engine.Vec2
	lastEvs     []engine.Event
	lastIntents []engine.Intent
}

// New creates a match of scenario/mvp-0.1. The pilot is the LLM agent
// when AGENTBATTLE_LLM_* is configured; the scripted agent otherwise.
func New(id string, seed uint64, prof *profile.Store) *Match {
	pack := scenario.MVP(seed)
	w, player, unknown, frag := pack.Genesis()
	m := &Match{
		ID: id, Seed: seed, PackName: pack.Name,
		W: w, TurnLimit: pack.TurnLimit, Spawn: pack.PlayerSpawn,
		PlayerID: player, UnknownID: unknown, FragmentID: frag,
		Explored:   map[engine.Vec2]bool{},
		Status:     StatusRunning,
		Influence:  InfluenceStart,
		profile:    prof,
		unknownDir: engine.Vec2{X: -1, Y: 0},
	}
	if cfg := agent.LLMFromEnv(); cfg != nil {
		p := agent.NewLLM(*cfg)
		if prof != nil {
			p.SetProfile(prof.P.Persona, prof.P.Lessons)
		}
		m.Pilot, m.PilotName = p, "llm"
	} else {
		m.Pilot = agent.NewScripted(pack.PlayerSpawn, pack.FragmentPos, pack.Passable, pack.W, pack.H)
		m.PilotName = "scripted"
	}
	return m
}

// Tick advances one turn with explicit player intents.
func (m *Match) Tick(intents []engine.Intent) {
	if m.Status != StatusRunning {
		return
	}
	evs := m.W.Advance(map[engine.EntityID][]engine.Intent{
		m.PlayerID:  intents,
		m.UnknownID: m.unknownAct(),
	})
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

// TickAuto advances one turn using the pilot's decision. Player
// guidance enters the LLM pilot's context (偏好，不是动作).
func (m *Match) TickAuto() {
	if m.Status != StatusRunning {
		return
	}
	if m.pauseNext { // 紧急暂停：Agent 本回合待命（strage_1 影响表）
		m.pauseNext = false
		m.Tick(nil)
		return
	}
	if la, ok := m.Pilot.(*agent.LLMAgent); ok {
		notes := make([]agent.GuidanceNote, 0, len(m.Guidance))
		for _, g := range m.Guidance {
			notes = append(notes, agent.GuidanceNote{Turn: g.Turn, Kind: g.Kind, Text: g.Text})
		}
		la.SetGuidance(notes)
	}
	obs := m.W.Observe(m.PlayerID, m.lastEvs)
	m.Tick(m.Pilot.Decide(obs))
	if la, ok := m.Pilot.(*agent.LLMAgent); ok {
		if c := la.LastCognition(); c != nil {
			m.Cognition = c
		}
	}
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
		case "move":
			return "move"
		case "transfer":
			return "pickup"
		case "assemble":
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
			"events": tail(m.Events, 60),
			"guidance": m.Guidance,
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
// stand-in for its Logic DSL (world_genesis §5.2): wander the outer
// ring; flee if the player gets close. Same seed → same walk.
func (m *Match) unknownAct() []engine.Intent {
	u := m.W.Entities[m.UnknownID]
	if u == nil {
		return nil
	}
	try := func(p engine.Vec2) bool {
		return m.W.InBounds(p) && m.W.At(p).Passable && m.W.EntityAt(p) == nil
	}
	if p := m.W.Entities[m.PlayerID]; p != nil && engine.Chebyshev(p.Pos, u.Pos) <= 2 {
		d := engine.Vec2{X: sign2(u.Pos.X - p.Pos.X), Y: sign2(u.Pos.Y - p.Pos.Y)}
		for _, cand := range []engine.Vec2{
			{X: u.Pos.X + d.X, Y: u.Pos.Y + d.Y},
			{X: u.Pos.X + d.X, Y: u.Pos.Y},
			{X: u.Pos.X, Y: u.Pos.Y + d.Y},
		} {
			if (cand != u.Pos) && try(cand) {
				return []engine.Intent{{Type: "move", To: &cand, Power: 600}}
			}
		}
	}
	for i := 0; i < 4; i++ {
		cand := engine.Vec2{X: u.Pos.X + m.unknownDir.X, Y: u.Pos.Y + m.unknownDir.Y}
		if try(cand) && m.onRing(cand) {
			return []engine.Intent{{Type: "move", To: &cand, Power: 400}}
		}
		m.unknownDir = rotate(m.unknownDir)
	}
	return nil
}

func (m *Match) onRing(p engine.Vec2) bool {
	return p.X == 0 || p.Y == 0 || p.X == m.W.W-1 || p.Y == m.W.H-1
}

func rotate(d engine.Vec2) engine.Vec2 {
	return engine.Vec2{X: -d.Y, Y: d.X}
}

func sign2(v int) int {
	if v > 0 {
		return 1
	}
	if v < 0 {
		return -1
	}
	return 0
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
