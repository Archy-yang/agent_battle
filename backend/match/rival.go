package match

// rival.go —— v0.9 敌对 Agent（rival）调度协议 (docs/world_genesis.md §5.2)。
//
// 三铁律的 match 层落实：
//   一、敌人是 Entity：rival 的每个意图都进 engine.Advance 同一条物理链，
//       无特例物理（身体由 scenario.V05/PromoteRival 装配，本文件只出意图）。
//   二、敌人没有特权 AI：它只见 engine.Observe(rivalID) 给它的感知——
//       信息边界与玩家对称（它看不到玩家看不到的）；调度与玩家同构——
//       独立 EventClassifier 实例（rivalCls）判定本 tick 是否值得一次
//       决策，无触发不调用；触发时唤醒挂在 RivalPilot 上的任意
//       DecisionMaker（与玩家 pilot 完全同接口，Agent Protocol 对称性）。
//   三、敌人是诚实的：意图逐帧记录进录像（Frame.RivalIntents），回放
//       按记录重演、不重跑（LLM rival 与玩家同规则，world_genesis §7）。
//
// LLM 接入点：ScriptedRival 先行；LLM rival 只需实现 rivalPilot（=
// agent.DecisionMaker，建议同时实现 orderPilot 以复用引擎执行层的
// move_to 逐 tick 步进）并赋给 Match.RivalPilot——调度、感知、录像、
// 重放各层对两种实现无差别。

import (
	"agentbattle/backend/agent"
	"agentbattle/backend/engine"
)

// rivalPilot is the rival's decision interface —— 与玩家 pilot 同接口
// （Agent Protocol 对称性，world_rule §12）：Observation 进、Intent 出。
// 接受任意 DecisionMaker：ScriptedRival 先行，LLM rival 经同一接入点
// 挂入（Match.RivalPilot），match 不感知差别。
type rivalPilot = agent.DecisionMaker

// rivalAttackPower is the scripted rival's melee emitter power（相邻
// 攻击功率 500，介于游荡者 400 与哨卫 550 之间）。
const rivalAttackPower = 500

// ScriptedRival is the v0.9 aggressive stand-in pilot（LLM rival 的确
// 定性替身，与玩家侧 ScriptedAgent 同级的旧地图先验策略，world_genesis
// §3.3）：向玩家方向推进（先验目标 = 玩家出生点，与 ScriptedAgent 知道
// Fragment 位置同构）、感知内玩家相邻即攻击（power 500）、形 <40% 撤退
// 回出生锚点。全部判定只消费观察，同局面同输出。
type ScriptedRival struct {
	Home     engine.Hex // 出生锚点：撤退目的地
	Target   engine.Hex // 玩家出生点（旧地图先验）
	Passable func(engine.Hex) bool
}

// NewScriptedRival builds the aggressive scripted pilot for a pack.
func NewScriptedRival(home, playerSpawn engine.Hex, passable func(engine.Hex) bool) *ScriptedRival {
	return &ScriptedRival{Home: home, Target: playerSpawn, Passable: passable}
}

// Decide is the plain DecisionMaker entry —— 与 DecideOrder 同一策略
// （rival 不依赖触发也能应答，协议对称性）。
func (r *ScriptedRival) Decide(obs *engine.Observation) []engine.Intent {
	return r.DecideOrder(obs)
}

// DecideOrder is the event-driven entry (design §3.1 执行层)：以 move_to
// 常驻订单说话——引擎逐 tick 自动步进，两次事件之间零次决策；相邻战斗
// 期间 combat 每 tick 触发，逐 tick 出 activate 攻击。
func (r *ScriptedRival) DecideOrder(obs *engine.Observation) []engine.Intent {
	if obs == nil {
		return nil
	}
	self := obs.Self
	// 形 <40%：撤退回锚点（已在锚点则驻足蓄息，nil = 无输出）。
	if self.Integrity*5 < self.MaxIntegrity*2 {
		if self.Pos == r.Home {
			return nil
		}
		to := r.Home
		return []engine.Intent{{Type: "move_to", To: &to}}
	}
	var intents []engine.Intent
	// 感识未开且气足：先开感识（读迹有代价，world_laws 律六；半径 1→4，
	// 更早看见玩家）。
	if !self.SensorActive && self.Battery > 40 {
		if c := selfComponent(self, engine.CSensor.String()); c != nil {
			intents = append(intents, engine.Intent{Type: "activate", Component: c.ID})
		}
	}
	// 感知内的玩家：相邻即攻击（离火 power 500）；否则贴上去。
	if foe, ok := perceivedFoe(obs); ok {
		if foe.Distance <= 1 {
			if c := selfComponent(self, engine.CEmitter.String()); c != nil {
				return append(intents, engine.Intent{
					Type: "activate", Component: c.ID, Target: foe.ID, Power: rivalAttackPower,
				})
			}
		}
		to := foe.Pos
		return append(intents, engine.Intent{Type: "move_to", To: &to})
	}
	// 未见玩家：向先验目标推进（执行层订单，引擎按静态地形 BFS 步进）。
	to := r.Target
	return append(intents, engine.Intent{Type: "move_to", To: &to})
}

// selfComponent finds one of the agent's own components by type name.
func selfComponent(self engine.SelfView, typ string) *engine.ComponentView {
	for i := range self.Components {
		if self.Components[i].Type == typ {
			return &self.Components[i]
		}
	}
	return nil
}

// perceivedFoe returns the player entity as the rival perceives it
// （信息边界对称：rival 只消费 Observe 给它的实体表；Owner "player" 是
// 它唯一的天敌，fragment/晶簇/同类不构成目标）。平局取 id 小者。
func perceivedFoe(obs *engine.Observation) (engine.SeenEntity, bool) {
	var best engine.SeenEntity
	found := false
	for _, seen := range obs.Entities {
		if seen.Kind != "agent" || seen.Owner != "player" {
			continue
		}
		if !found || seen.ID < best.ID {
			best, found = seen, true
		}
	}
	return best, found
}

// scheduleRival runs the rival's event-driven scheduler for the tick
// about to be advanced: 独立 classifier 消费上一 tick 的事件流与 rival
// 的感知/状态 → 触发才唤醒 pilot；无触发返回 nil（执行层既有 move_to
// 订单由引擎照走，与玩家同规则，0 次调用）。
func (m *Match) scheduleRival() []engine.Intent {
	if m.RivalID == 0 || m.RivalPilot == nil || m.Status != StatusRunning {
		return nil
	}
	if m.W.Entities[m.RivalID] == nil {
		return nil // 形散：不再调度
	}
	trigger := m.classifyRivalTick()
	if trigger == "" {
		return nil
	}
	m.rivalTriggerLog = append(m.rivalTriggerLog, trigger)
	obs := m.W.Observe(m.RivalID, m.lastEvs) // 对称信息边界
	if op, ok := m.RivalPilot.(orderPilot); ok {
		return op.DecideOrder(obs)
	}
	return m.RivalPilot.Decide(obs)
}

// classifyRivalTick feeds the rival's own classifier: 触发条件以 rival
// 的感知/状态为准——Self = rival 实体（threshold 通道）、VisibleHostiles
// = rival 感知内的玩家（encounter 通道）、InCombat = rival 视角。与玩家
// 分类器 m.cls 是两个实例，去抖状态互不污染。
func (m *Match) classifyRivalTick() string {
	return m.rivalCls.Classify(ClassifyInput{
		Tick:            m.W.Tick,
		PlayerID:        m.RivalID, // classifier 语义中的"self"
		Events:          m.classifierEvents(),
		Self:            m.W.Entities[m.RivalID],
		VisibleHostiles: m.rivalVisibleFoes(),
		InCombat:        m.rivalInCombat(),
	})
}

// rivalVisibleFoes lists the ids hostile to the rival (即玩家) inside
// the rival's perception radius (被动 1，感识开启 4).
func (m *Match) rivalVisibleFoes() []engine.EntityID {
	rv := m.W.Entities[m.RivalID]
	p := m.W.Entities[m.PlayerID]
	if rv == nil || p == nil {
		return nil
	}
	radius := 1
	if s := rv.FindComponent(engine.CSensor); s != nil && s.Active {
		radius = 4
	}
	if engine.HexDist(rv.Pos, p.Pos) <= radius {
		return []engine.EntityID{m.PlayerID}
	}
	return nil
}

// rivalInCombat reports combat from the rival's perspective: 玩家进入
// HexDist 2，或上一 tick 的事件流中 rival/玩家被 attack/backlash/spill
// 作用（与 InCombat 的玩家视角同构）。
func (m *Match) rivalInCombat() bool {
	rv := m.W.Entities[m.RivalID]
	p := m.W.Entities[m.PlayerID]
	if rv != nil && p != nil && engine.HexDist(rv.Pos, p.Pos) <= 2 {
		return true
	}
	for _, ev := range m.lastEvs {
		switch ev.Type {
		case "attack", "backlash", "spill":
			if ev.Entity == m.RivalID || ev.Entity == m.PlayerID {
				return true
			}
		}
	}
	return false
}

// v0.9 回放注册：V05 录像可重放（replayConstructors 定义于
// recording.go；init 注册避免回改既有注册表字面量）。
func init() {
	replayConstructors["v05"] = NewV05
	replayConstructors["scenario/infinite-0.5"] = NewV05
}
