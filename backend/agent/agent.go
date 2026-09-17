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
	Spawn    engine.Hex
	Target   engine.Hex
	Passable func(engine.Hex) bool
}

// NewScripted builds a pilot for a scenario pack.
func NewScripted(spawn, fragment engine.Hex, passable func(engine.Hex) bool) *ScriptedAgent {
	return &ScriptedAgent{Spawn: spawn, Target: fragment, Passable: passable}
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

// nextStep is a deterministic greedy walk over the six hex directions:
// candidates ordered by ascending HexDist to the goal (fixed tie order
// by HexDirs), first passable and unblocked candidate wins. On open
// terrain every step closes the distance by exactly 1.
func (a *ScriptedAgent) nextStep(from, goal engine.Hex, obs *engine.Observation) (engine.Hex, bool) {
	for _, cand := range engine.HexStepToward(from, goal) {
		if a.Passable(cand) && !a.blocked(cand, obs) {
			return cand, true
		}
	}
	return engine.Hex{}, false
}

func (a *ScriptedAgent) blocked(p engine.Hex, obs *engine.Observation) bool {
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
