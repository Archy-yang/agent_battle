package match

// EventClassifier —— v0.4 LLM 介入层的事件分类器 (design §3.2)。
//
// 输入每 tick 的引擎事件流与 match 公开状态，输出本轮 LLM 触发原因
// （可空）。确定性、可测试：所有去抖状态都是 match 内的普通确定性
// 字段，重放不受影响（design §2.5：重放 = Engine + seed + Intent 流，
// 分类器只消费事件流与公开实体状态，不掷任何随机数）。
//
// 触发规则（design §3.2，优先级自上而下，每 tick 至多一个触发）：
//   guidance   玩家 Guidance 到达时置 pending 标记，下一 tick 必触发一次
//   combat     InCombat 的每个 tick（战斗是唯一"每步都贵"的层）
//   arrive     order_done 每订单至多一次（订单生命周期内引擎只发一次）；
//              order_blocked 连续 3 tick 才升级为事件
//   encounter  敌对实体首见触发一次；离开感知 ≥6 tick 重新武装
//   resource   进入含新 essence 物料信息的已探索 chunk（v0.4 简化：取材
//              于 V04 PendingSpawns 懒激活 spawn 事件 + 首次 pickup）
//   threshold  battery<20 / heat>20 / integrity<30，边沿触发 + 10% 迟滞
//              （须回到阈值 ±10% 才重新武装）
//   heartbeat  连续 8 tick 无触发 → 反思一次；反思本身计为事件，防连环
//
// 被更高优先级压过的候选不消费去抖状态（如遭遇到期不消除），下一 tick
// 仍可触发——事件不丢失，只是让位。

import (
	"agentbattle/backend/engine"
)

// 触发原因（传给 LLM 的 user payload 的 trigger 字段）。
const (
	TriggerArrive    = "arrive"    // 抵达层：order_done / order_blocked 升级
	TriggerEncounter = "encounter" // 敌对实体首见
	TriggerResource  = "resource"  // 资源发现（懒激活 / 首次拾取）
	TriggerHeartbeat = "heartbeat" // 心跳反思
	TriggerCombat    = "combat"    // 战斗层
	TriggerGuidance  = "guidance"  // 玩家干预到达
	// 状态越阈触发为 "threshold:battery" / "threshold:heat" /
	// "threshold:integrity"（TriggerThreshold 前缀）。
	TriggerThreshold = "threshold"
)

// 状态越阈阈值（design §3.1/§3.2；玩家 MaxBattery/MaxIntegrity = 100，
// 故 <20 即 20%：<30 即 30%）。heat 的引擎过热线是 25，>20 提前预警。
const (
	batteryLow   = 20 // battery < 20 触发
	heatHigh     = 20 // heat   > 20 触发
	integrityLow = 30 // integrity < 30 触发
)

// 10% 迟滞的复位线：触发后须回到该线才重新武装。
const (
	batteryReArm   = batteryLow + 2   // ≥22 重新武装
	heatReArm      = heatHigh - 2     // ≤18 重新武装
	integrityReArm = integrityLow + 3 // ≥33 重新武装
)

// 去抖参数（design §3.2）。
const (
	blockedEscalateAfter = 3 // order_blocked 连续 3 tick 升级为事件
	encounterReArmAfter  = 6 // 离开感知 ≥6 tick 重新武装
	HeartbeatEvery       = 8 // 连续 8 tick 无触发 → 心跳反思
)

// encounterState is one hostile entity's encounter debounce state.
type encounterState struct {
	armed        bool
	lastSeenTick int64
}

// EventClassifier holds the deterministic debounce state of the event
// intervention layer. One instance per match.
type EventClassifier struct {
	// HeartbeatEvery overrides the heartbeat interval when > 0
	//（测试用它隔离心跳；0 = 默认 HeartbeatEvery）。
	HeartbeatEvery int

	lastTriggerTick int64 // 上次触发（任何触发归零心跳计数）
	processedTick   int64 // 已消费的事件流 tick（幂等护栏）

	blockedStreak int // 连续 order_blocked 计数
	encounters    map[engine.EntityID]*encounterState
	pickupSeen    bool // 首次 pickup 只触发一次
	spawnSeen     map[engine.EntityID]bool

	batteryArmed   bool
	heatArmed      bool
	integrityArmed bool

	pendingGuidance bool // Guidance 到达：下一 tick 必触发
}

// NewEventClassifier creates the classifier with fresh debounce state.
// lastTriggerTick 初始化为 -HeartbeatEvery：开局第一 tick 即触发一次
// 心跳（"开局唤醒"），否则 Agent 要干等到第 8 tick 才做第一个决策。
func NewEventClassifier() *EventClassifier {
	return &EventClassifier{
		lastTriggerTick: -HeartbeatEvery,
		processedTick:   -1,
		encounters:      map[engine.EntityID]*encounterState{},
		spawnSeen:       map[engine.EntityID]bool{},
		batteryArmed:    true,
		heatArmed:       true,
		integrityArmed:  true,
	}
}

// ArmGuidance marks that one player Guidance has arrived：下一 tick 必
// 触发一次（design §3.1 Guidance 层）。
func (c *EventClassifier) ArmGuidance() { c.pendingGuidance = true }

// ClassifyInput is one tick's facts fed to the classifier.
type ClassifyInput struct {
	Tick            int64
	PlayerID        engine.EntityID
	Events          []engine.Event    // 本 tick 引擎事件流（含怪物/环境）
	Self            *engine.Entity    // 玩家实体（形散后 nil）
	VisibleHostiles []engine.EntityID // 感知半径内的敌对实体（确定序）
	InCombat        bool
}

// Classify consumes one tick's events and returns this round's trigger
// reason, or "" (不触发：执行层继续，0 次调用). 幂等：同一 tick 重复
// 喂入返回 ""。
func (c *EventClassifier) Classify(in ClassifyInput) string {
	if in.Tick <= c.processedTick {
		return "" // 同一事件流只消费一次
	}
	c.processedTick = in.Tick

	// —— 事件流扫描（去抖状态更新先于触发裁决，确定性）——
	done, blocked, pickup, spawn := false, false, false, false
	for _, ev := range in.Events {
		if ev.Entity == in.PlayerID {
			switch ev.Type {
			case "order_done":
				done = true // 引擎在订单生命周期内只发一次 → 每订单至多一次
			case "order_blocked":
				blocked = true
			case "pickup":
				pickup = true
			}
			continue
		}
		if ev.Type == "spawn" && !c.spawnSeen[ev.Entity] {
			c.spawnSeen[ev.Entity] = true // 懒激活 = 该 chunk 的新 essence 物料信息
			spawn = true
		}
	}
	escalate := false
	if blocked {
		c.blockedStreak++
		if c.blockedStreak >= blockedEscalateAfter {
			escalate = true
			c.blockedStreak = 0 // 升级即归零：再次升级需再连续 3 tick
		}
	} else {
		c.blockedStreak = 0
	}
	if pickup {
		c.pickupSeen = true
	}
	arriveCand := done || escalate
	resourceCand := spawn || pickup

	// —— 遭遇去抖：首见触发一次；离感知 ≥6 tick 重新武装 ——
	encounter := false
	var encID engine.EntityID
	for _, id := range in.VisibleHostiles {
		st := c.encounters[id]
		if st == nil {
			st = &encounterState{armed: true, lastSeenTick: in.Tick}
			c.encounters[id] = st
		}
		if !st.armed && in.Tick-st.lastSeenTick >= encounterReArmAfter {
			st.armed = true
		}
		st.lastSeenTick = in.Tick
		if st.armed && !encounter {
			encounter, encID = true, id
		}
	}

	// —— 状态越阈：边沿触发 + 10% 迟滞 ——
	thresh := ""
	if in.Self != nil {
		switch {
		case c.batteryArmed && in.Self.Battery < batteryLow:
			thresh = TriggerThreshold + ":battery"
		case c.heatArmed && in.Self.Heat > heatHigh:
			thresh = TriggerThreshold + ":heat"
		case c.integrityArmed && in.Self.Integrity < integrityLow:
			thresh = TriggerThreshold + ":integrity"
		}
		// 迟滞复位按当前值判定（与是否触发无关，确定性）
		if !c.batteryArmed && in.Self.Battery >= batteryReArm {
			c.batteryArmed = true
		}
		if !c.heatArmed && in.Self.Heat <= heatReArm {
			c.heatArmed = true
		}
		if !c.integrityArmed && in.Self.Integrity >= integrityReArm {
			c.integrityArmed = true
		}
	}

	// —— 心跳：连续 N tick 无触发 ——
	hb := c.HeartbeatEvery
	if hb <= 0 {
		hb = HeartbeatEvery
	}
	heartbeat := in.Tick-c.lastTriggerTick >= int64(hb)

	// —— 裁决：优先级取一 ——
	win := ""
	switch {
	case c.pendingGuidance:
		win = TriggerGuidance
	case in.InCombat:
		win = TriggerCombat
	case arriveCand:
		win = TriggerArrive
	case encounter:
		win = TriggerEncounter
	case resourceCand:
		win = TriggerResource
	case thresh != "":
		win = thresh
	case heartbeat:
		win = TriggerHeartbeat
	}
	if win == "" {
		return ""
	}

	// 触发本身的去抖效应：任何触发归零心跳计数；其余只消费胜出者。
	c.lastTriggerTick = in.Tick
	switch win {
	case TriggerGuidance:
		c.pendingGuidance = false
	case TriggerEncounter:
		if st := c.encounters[encID]; st != nil {
			st.armed = false
		}
	case TriggerThreshold + ":battery":
		c.batteryArmed = false
	case TriggerThreshold + ":heat":
		c.heatArmed = false
	case TriggerThreshold + ":integrity":
		c.integrityArmed = false
	}
	return win
}
