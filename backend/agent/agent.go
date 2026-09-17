package agent

import "agentbattle/backend/engine"

// DecisionMaker turns an Observation into Intents — the AgentProtocol
// in its purest form (world_rule §12). LLM-backed implementations land
// in M1; everything here is deterministic and testable.
type DecisionMaker interface {
	Decide(obs *engine.Observation) []engine.Intent
}

// ScriptedAgent is the v0.1 player pilot: a deterministic MVP policy
// justified by 旧地图先验 (world_genesis §3.3) — it knows the terrain
// and the Fragment's approximate location, walks there, picks it up
// and returns to spawn. It exercises the full loop without an LLM.
type ScriptedAgent struct {
	Spawn    engine.Vec2
	Target   engine.Vec2
	Passable func(engine.Vec2) bool
	W, H     int
}

// NewScripted builds a pilot for scenario/mvp-0.1.
func NewScripted(spawn, fragment engine.Vec2, passable func(engine.Vec2) bool, w, h int) *ScriptedAgent {
	return &ScriptedAgent{Spawn: spawn, Target: fragment, Passable: passable, W: w, H: h}
}

func (a *ScriptedAgent) Decide(obs *engine.Observation) []engine.Intent {
	if obs == nil {
		return nil
	}
	self := obs.Self

	carrying := len(self.Carrying) > 0
	if carrying && self.Pos == a.Spawn {
		return nil // 胜利由对局层判定
	}
	if !carrying && self.Pos == a.Target {
		for _, seen := range obs.Entities {
			if seen.Kind == "fragment" && seen.Pos == self.Pos {
				return []engine.Intent{{Type: "transfer", Target: seen.ID}}
			}
		}
		return nil
	}
	goal := a.Target
	if carrying {
		goal = a.Spawn
	}
	if step, ok := a.nextStep(self.Pos, goal, obs); ok {
		return []engine.Intent{{Type: "move", To: &step, Power: 400}}
	}
	return nil
}

// nextStep is a deterministic greedy walk: prefer the axis with the
// larger remaining delta; sidestep if blocked.
func (a *ScriptedAgent) nextStep(from, goal engine.Vec2, obs *engine.Observation) (engine.Vec2, bool) {
	dx, dy := goal.X-from.X, goal.Y-from.Y
	primary, secondary := engine.Vec2{}, engine.Vec2{}
	if abs(dx) >= abs(dy) {
		primary = engine.Vec2{X: sign(dx), Y: 0}
		secondary = engine.Vec2{X: 0, Y: sign(dy)}
	} else {
		primary = engine.Vec2{X: 0, Y: sign(dy)}
		secondary = engine.Vec2{X: sign(dx), Y: 0}
	}
	for _, d := range []engine.Vec2{primary, secondary} {
		if d.X == 0 && d.Y == 0 {
			continue
		}
		cand := engine.Vec2{X: from.X + d.X, Y: from.Y + d.Y}
		if a.Passable(cand) && !a.blocked(cand, obs) {
			return cand, true
		}
	}
	return engine.Vec2{}, false
}

func (a *ScriptedAgent) blocked(p engine.Vec2, obs *engine.Observation) bool {
	for _, seen := range obs.Entities {
		if seen.Kind == "fragment" {
			continue
		}
		if seen.Pos == p {
			return true
		}
	}
	return false
}

func sign(v int) int {
	if v > 0 {
		return 1
	}
	if v < 0 {
		return -1
	}
	return 0
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
