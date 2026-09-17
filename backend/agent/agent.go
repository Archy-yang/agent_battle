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
	// 相邻即撤（演示策略，v0.6 三轮新增）：感知内有 HexDist ≤1 的敌对
	// 实体 → 朝"远离该敌"的方向走一步（HexStepToward 定序，被挡则照
	// 常走下方逻辑），本 tick 不做其他事。让演示策略具备脱离能力——
	// 无此规则时被咬住即无法脱离（怪物同速且蓄息后不再自瘫）。拾取/
	// 交付分支已在其上：transfer 免费即时，优先结算不因撤离延迟。
	if foe, ok := adjacentHostile(obs); ok {
		away := engine.Hex{Q: 2*self.Pos.Q - foe.Pos.Q, R: 2*self.Pos.R - foe.Pos.R} // 远离敌一格
		for _, cand := range engine.HexStepToward(self.Pos, away) {
			if engine.HexDist(cand, foe.Pos) > 1 && a.Passable(cand) && !a.blocked(cand, obs) {
				to := cand
				return []engine.Intent{{Type: "move", To: &to, Power: 400}}
			}
		}
	}
	// 喘气启发（演示策略，world-laws/0.2 蓄息律）：气线以下返回 nil
	// 原地蓄息——携不携晶统一蓄到 40（v0.6 三轮：回程阈值 60→40，回
	// 程涓流 1 步/3 tick 是二轮"携晶超时"的主因；断气自愈由蓄息律兜
	// 底）。nil 意图 = 本 tick 无任何输出，引擎按蓄息律回气
	// （RestRegen=2/tick）。
	if self.Battery < 40 {
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

// DecideOrder is the WP3 event-driven entry for the scripted pilot
// (design §3.1 执行层)：它以 move_to 常驻订单说话——引擎逐 tick 自动
// 步进，两次事件之间零次决策；pilot 只在 EventClassifier 唤醒时被咨询
// （抵达、受阻升级、心跳……）。在结晶格搬运，携晶回出生点后待命
// （胜利由对局层判定）。
func (a *ScriptedAgent) DecideOrder(obs *engine.Observation) []engine.Intent {
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
	to := goal // move_to 订单：引擎按静态地形 BFS 逐 tick 步进，受阻自动等待
	return []engine.Intent{{Type: "move_to", To: &to}}
}

// adjacentHostile reports one hostile entity at HexDist ≤1 inside the
// observation, deterministically chosen (lowest entity id wins on ties).
// Hostile = 怪物（Kind "monster"）与 Unknown Agent（Kind "agent" 且非
// player 所有）；fragment 与晶簇不构成威胁。
func adjacentHostile(obs *engine.Observation) (engine.SeenEntity, bool) {
	var best engine.EntityID = -1
	var foe engine.SeenEntity
	for _, seen := range obs.Entities {
		hostile := seen.Kind == "monster" || (seen.Kind == "agent" && seen.Owner != "player")
		if !hostile || seen.Distance > 1 {
			continue
		}
		if best == -1 || seen.ID < best {
			best, foe = seen.ID, seen
		}
	}
	return foe, best != -1
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
