// Package match glues a scenario pack, the engine and agent policies
// into one playable session: turn loop, fog memory (v0.1 session-level
// stand-in for agent-owned memory, world_rule §21), guidance log and
// win/lose adjudication (world_genesis §8).
package match

import (
	"fmt"

	"agentbattle/backend/agent"
	"agentbattle/backend/engine"
	"agentbattle/backend/scenario"
)

// Status values of a match.
const (
	StatusRunning = "running"
	StatusWon     = "won"
	StatusLost    = "lost"
	StatusTimeout = "timeout"
)

// Guidance is one player intervention (player_agent_relationship §5):
// goal / preference / concern / information — never an action.
type Guidance struct {
	Turn int64  `json:"turn"`
	Kind string `json:"kind"`
	Text string `json:"text"`
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

	Pilot agent.DecisionMaker // scripted v0.1; LLM-backed in M1

	Explored map[engine.Vec2]bool
	Events   []engine.Event
	Guidance []Guidance
	Status   string

	unknownDir engine.Vec2
	lastEvs    []engine.Event
}

// New creates a match of scenario/mvp-0.1 with a scripted pilot.
func New(id string, seed uint64) *Match {
	pack := scenario.MVP(seed)
	w, player, unknown, frag := pack.Genesis()
	return &Match{
		ID: id, Seed: seed, PackName: pack.Name,
		W: w, TurnLimit: pack.TurnLimit, Spawn: pack.PlayerSpawn,
		PlayerID: player, UnknownID: unknown, FragmentID: frag,
		Pilot: agent.NewScripted(pack.PlayerSpawn, pack.FragmentPos, pack.Passable, pack.W, pack.H),
		Explored: map[engine.Vec2]bool{},
		Status:   StatusRunning,
		unknownDir: engine.Vec2{X: -1, Y: 0}, // 沿北环向西游荡
	}
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
	m.Events = append(m.Events, evs...)
	if obs := m.W.Observe(m.PlayerID, nil); obs != nil { // 形散后无感知
		for _, p := range obs.Visible {
			m.Explored[p] = true
		}
	}
	m.checkEnd()
}

// TickAuto advances one turn using the pilot's decision.
func (m *Match) TickAuto() {
	if m.Status != StatusRunning {
		return
	}
	obs := m.W.Observe(m.PlayerID, m.lastEvs)
	m.Tick(m.Pilot.Decide(obs))
}

// Observe returns the player agent's current perception.
func (m *Match) Observe() *engine.Observation {
	return m.W.Observe(m.PlayerID, m.lastEvs)
}

// AddGuidance records a player intervention (v0.1: log only; M1 feeds
// it into the LLM agent's decision context).
func (m *Match) AddGuidance(kind, text string) {
	m.Guidance = append(m.Guidance, Guidance{Turn: m.W.Tick, Kind: kind, Text: text})
}

func (m *Match) checkEnd() {
	p := m.W.Entities[m.PlayerID]
	if p != nil && carries(p, m.FragmentID) && p.Pos == m.Spawn {
		m.Status = StatusWon
		return
	}
	if p == nil || p.Integrity <= 0 {
		m.Status = StatusLost
		return
	}
	if m.W.Tick >= m.TurnLimit {
		m.Status = StatusTimeout
	}
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
	// 避险：玩家靠近则远离
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
	// 游荡外环
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
	return engine.Vec2{X: -d.Y, Y: d.X} // 逆时针 90°
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
