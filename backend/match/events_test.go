package match

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"agentbattle/backend/agent"
	"agentbattle/backend/engine"
)

// —— EventClassifier 单元用例（design §3.2 去抖规则）——

// newTestPlayer builds a stand-in player entity at full form/qi.
func newTestPlayer() *engine.Entity {
	return &engine.Entity{
		ID: 1, Kind: "agent", Owner: "player",
		Integrity: 100, MaxIntegrity: 100, Battery: 100, MaxBattery: 100,
	}
}

// ev builds one engine event.
func ev(tick int64, e engine.EntityID, typ, text string) engine.Event {
	return engine.Event{Tick: tick, Type: typ, Entity: e, Text: text}
}

// feed runs one Classify round against classifier c (心跳隔离由调用方
// 通过 c.HeartbeatEvery 控制).
func feed(c *EventClassifier, p *engine.Entity, tick int64, evs []engine.Event, hostiles []engine.EntityID, combat bool) string {
	return c.Classify(ClassifyInput{
		Tick: tick, PlayerID: p.ID, Events: evs, Self: p,
		VisibleHostiles: hostiles, InCombat: combat,
	})
}

// 同途不重触发：order_set/move 不触发；order_done 只在订单完成那 tick
// 触发一次；抵达后行走中不再触发（每订单至多一次）。
func TestClassifierArrivalOncePerOrder(t *testing.T) {
	c := NewEventClassifier()
	c.HeartbeatEvery = 1000 // 隔离心跳
	p := newTestPlayer()
	one := func(tick int64, typ string) string {
		return feed(c, p, tick, []engine.Event{ev(tick, p.ID, typ, typ)}, nil, false)
	}
	if got := one(1, "order_set"); got != "" {
		t.Fatalf("order_set 不应触发，got %q", got)
	}
	for tk := int64(2); tk <= 5; tk++ { // 行走途中：move 事件零触发
		if got := one(tk, "move"); got != "" {
			t.Fatalf("tick %d：行走中不应触发，got %q", tk, got)
		}
	}
	if got := one(6, "order_done"); got != TriggerArrive {
		t.Fatalf("order_done 应触发 arrive，got %q", got)
	}
	// 同途不重触发：抵达后继续行走不再触发
	for tk := int64(7); tk <= 9; tk++ {
		if got := one(tk, "move"); got != "" {
			t.Fatalf("tick %d：抵达后行走不应触发，got %q", tk, got)
		}
	}
	// 新订单（新一次 order_done）→ 新的一次 arrive
	if got := one(10, "order_done"); got != TriggerArrive {
		t.Fatalf("新订单完成应再次触发 arrive，got %q", got)
	}
}

// order_blocked 连续 3 tick 才升级为事件；升级后计数归零。
func TestClassifierBlockedEscalation(t *testing.T) {
	c := NewEventClassifier()
	c.HeartbeatEvery = 1000
	p := newTestPlayer()
	one := func(tick int64, typ string) string {
		return feed(c, p, tick, []engine.Event{ev(tick, p.ID, typ, typ)}, nil, false)
	}
	if got := one(1, "order_blocked"); got != "" {
		t.Fatalf("第 1 tick 受阻不应触发，got %q", got)
	}
	if got := one(2, "order_blocked"); got != "" {
		t.Fatalf("第 2 tick 受阻不应触发，got %q", got)
	}
	if got := one(3, "order_blocked"); got != TriggerArrive {
		t.Fatalf("连续 3 tick 受阻应升级为 arrive，got %q", got)
	}
	if got := one(4, "order_blocked"); got != "" {
		t.Fatalf("升级后计数应归零（第 1 tick），got %q", got)
	}
	if got := one(5, "order_blocked"); got != "" {
		t.Fatalf("升级后计数应归零（第 2 tick），got %q", got)
	}
	if got := one(6, "order_blocked"); got != TriggerArrive {
		t.Fatalf("再次连续 3 tick 应再升级，got %q", got)
	}
	// 中断即重新计数
	if got := one(7, "move"); got != "" {
		t.Fatalf("行走应清零受阻计数，got %q", got)
	}
	if got := one(8, "order_blocked"); got != "" {
		t.Fatalf("中断后第 1 tick 受阻不应触发，got %q", got)
	}
}

// 遭遇：敌对实体首见触发一次；离开感知 ≥6 tick 重新武装。
func TestClassifierEncounterDebounce(t *testing.T) {
	c := NewEventClassifier()
	c.HeartbeatEvery = 1000
	p := newTestPlayer()
	mon := engine.EntityID(9)
	see := func(tick int64, visible bool) string {
		var vh []engine.EntityID
		if visible {
			vh = []engine.EntityID{mon}
		}
		return feed(c, p, tick, nil, vh, false)
	}
	if got := see(1, true); got != TriggerEncounter {
		t.Fatalf("首见应触发 encounter，got %q", got)
	}
	for tk := int64(2); tk <= 5; tk++ {
		if got := see(tk, true); got != "" {
			t.Fatalf("tick %d：持续可见不应重复触发，got %q", tk, got)
		}
	}
	for tk := int64(6); tk <= 10; tk++ { // 离域 1..5 tick：尚未重新武装
		if got := see(tk, false); got != "" {
			t.Fatalf("tick %d：离域未满 6 tick，got %q", tk, got)
		}
	}
	// tick 11：离感知已满 6 tick（5→11）→ 重新武装 → 再次触发
	if got := see(11, true); got != TriggerEncounter {
		t.Fatalf("离域 ≥6 tick 后应重新武装并再次触发，got %q", got)
	}
	if got := see(12, true); got != "" {
		t.Fatalf("重新触发后持续可见不应重复，got %q", got)
	}
}

// 遭遇只对首见的那只触发，其余敌对实体各自独立武装。
func TestClassifierEncounterPerEntity(t *testing.T) {
	c := NewEventClassifier()
	c.HeartbeatEvery = 1000
	p := newTestPlayer()
	a, b := engine.EntityID(9), engine.EntityID(10)
	if got := feed(c, p, 1, nil, []engine.EntityID{a}, false); got != TriggerEncounter {
		t.Fatalf("first sight of a should trigger, got %q", got)
	}
	// 同 tick 双首见：每 tick 至多一个触发——a 已见不重报，b 的首见
	// 在同 tick 补上报（a 无候选，b 是唯一武装者）。
	if got := feed(c, p, 2, nil, []engine.EntityID{a, b}, false); got != TriggerEncounter {
		t.Fatalf("b 的首见应在 tick 2 补报，got %q", got)
	}
	if got := feed(c, p, 3, nil, []engine.EntityID{a, b}, false); got != "" {
		t.Fatalf("both seen, should be quiet, got %q", got)
	}
}

// 状态越阈：边沿触发 + 10% 迟滞（battery<20 触发一次，回升到 22 前不重复）。
func TestClassifierThresholdHysteresis(t *testing.T) {
	c := NewEventClassifier()
	c.HeartbeatEvery = 1000
	p := newTestPlayer()
	batt := func(tick int64, v int) string {
		p.Battery = v
		return feed(c, p, tick, nil, nil, false)
	}
	if got := batt(1, 25); got != "" {
		t.Fatalf("battery 25 不应触发，got %q", got)
	}
	if got := batt(2, 19); got != TriggerThreshold+":battery" {
		t.Fatalf("battery 19 应触发 threshold:battery，got %q", got)
	}
	for tk, v := range map[int64]int{3: 20, 4: 21} { // 迟滞：回升到 22 前不重复
		if got := batt(tk, v); got != "" {
			t.Fatalf("battery %d（未回 22）不应重复触发，got %q", v, got)
		}
	}
	if got := batt(5, 22); got != "" {
		t.Fatalf("battery 22 重新武装但不触发，got %q", got)
	}
	if got := batt(6, 21); got != "" {
		t.Fatalf("battery 21 高于阈值不应触发，got %q", got)
	}
	if got := batt(7, 19); got != TriggerThreshold+":battery" {
		t.Fatalf("重新跌破 20 应再次触发，got %q", got)
	}

	// heat 与 integrity 各自独立边沿 + 迟滞
	c2 := NewEventClassifier()
	c2.HeartbeatEvery = 1000
	p2 := newTestPlayer()
	p2.Heat = 21
	if got := feed(c2, p2, 1, nil, nil, false); got != TriggerThreshold+":heat" {
		t.Fatalf("heat 21 应触发 threshold:heat，got %q", got)
	}
	p2.Heat = 22
	if got := feed(c2, p2, 2, nil, nil, false); got != "" {
		t.Fatalf("heat 22 未回落到 18 不应重复，got %q", got)
	}
	p2.Heat = 18
	if got := feed(c2, p2, 3, nil, nil, false); got != "" {
		t.Fatalf("heat 18 重新武装但不触发，got %q", got)
	}
	p2.Heat = 25
	if got := feed(c2, p2, 4, nil, nil, false); got != TriggerThreshold+":heat" {
		t.Fatalf("heat 重新越过 20 应再触发，got %q", got)
	}

	p2.Heat = 0
	p2.Integrity = 29
	if got := feed(c2, p2, 5, nil, nil, false); got != TriggerThreshold+":integrity" {
		t.Fatalf("integrity 29 应触发 threshold:integrity，got %q", got)
	}
	p2.Integrity = 31
	if got := feed(c2, p2, 6, nil, nil, false); got != "" {
		t.Fatalf("integrity 31（未回 33）不应重复，got %q", got)
	}
}

// 心跳：连续 8 tick 无事件恰好触发一次且归零计数（反思本身计为事件）。
func TestClassifierHeartbeat(t *testing.T) {
	c := NewEventClassifier() // 默认 HeartbeatEvery = 8
	p := newTestPlayer()
	one := func(tick int64) string {
		return feed(c, p, tick, nil, nil, false)
	}
	if got := one(0); got != TriggerHeartbeat {
		t.Fatalf("开局唤醒应为心跳（第 0 tick），got %q", got)
	}
	for tk := int64(1); tk <= 7; tk++ {
		if got := one(tk); got != "" {
			t.Fatalf("tick %d：心跳间隔内不应触发，got %q", tk, got)
		}
	}
	if got := one(8); got != TriggerHeartbeat {
		t.Fatalf("连续 8 tick 无触发应恰好触发一次心跳，got %q", got)
	}
	for tk := int64(9); tk <= 15; tk++ { // 反思已归零计数：重新数 8 tick
		if got := one(tk); got != "" {
			t.Fatalf("tick %d：反思后计数应已归零，got %q", tk, got)
		}
	}
	if got := one(16); got != TriggerHeartbeat {
		t.Fatalf("tick 16 应再次心跳，got %q", got)
	}
}

// 战斗：InCombat 的每个 tick 都触发。
func TestClassifierCombatEveryTick(t *testing.T) {
	c := NewEventClassifier()
	c.HeartbeatEvery = 1000
	p := newTestPlayer()
	for tk := int64(1); tk <= 5; tk++ {
		if got := feed(c, p, tk, nil, nil, true); got != TriggerCombat {
			t.Fatalf("tick %d：交战中应每 tick 触发 combat，got %q", tk, got)
		}
	}
	if got := feed(c, p, 6, nil, nil, false); got != "" {
		t.Fatalf("脱离交战且无事件不应触发，got %q", got)
	}
}

// 资源发现：懒激活 spawn 事件与首次 pickup 触发一次。
func TestClassifierResourceDiscovery(t *testing.T) {
	c := NewEventClassifier()
	c.HeartbeatEvery = 1000
	p := newTestPlayer()
	m1, m2 := engine.EntityID(9), engine.EntityID(10)
	if got := feed(c, p, 1, []engine.Event{ev(1, m1, "spawn", "懒激活")}, nil, false); got != TriggerResource {
		t.Fatalf("懒激活 spawn 应触发 resource，got %q", got)
	}
	if got := feed(c, p, 2, []engine.Event{ev(2, m2, "spawn", "懒激活")}, nil, false); got != TriggerResource {
		t.Fatalf("新 chunk 的新怪应再触发一次 resource，got %q", got)
	}
	if got := feed(c, p, 3, nil, nil, false); got != "" {
		t.Fatalf("无新信息不应触发，got %q", got)
	}
	if got := feed(c, p, 4, []engine.Event{ev(4, p.ID, "pickup", "取得元气结晶")}, nil, false); got != TriggerResource {
		t.Fatalf("首次 pickup 应触发 resource，got %q", got)
	}
	if got := feed(c, p, 5, nil, nil, false); got != "" {
		t.Fatalf("pickup 只触发一次，got %q", got)
	}
}

// 优先级与幂等：Guidance 压过 combat；同一事件流只消费一次。
func TestClassifierPriorityAndIdempotence(t *testing.T) {
	c := NewEventClassifier()
	c.HeartbeatEvery = 1000
	p := newTestPlayer()
	in := ClassifyInput{Tick: 1, PlayerID: p.ID, Self: p, InCombat: true}
	c.ArmGuidance()
	if got := c.Classify(in); got != TriggerGuidance {
		t.Fatalf("Guidance 应压过 combat，got %q", got)
	}
	in2 := in
	in2.Tick = 2
	if got := c.Classify(in2); got != TriggerCombat {
		t.Fatalf("Guidance 只消费一次，随后 combat 应触发，got %q", got)
	}
	if got := c.Classify(in2); got != "" {
		t.Fatalf("同一 tick 重复喂入应幂等返回空，got %q", got)
	}
}

// —— match 集成 ——

// Guidance 到达 → 下一 tick 必触发一次，且只触发一次。
func TestGuidanceTriggersNextTick(t *testing.T) {
	m := New("g-trig", 3, nil)
	if _, ok := m.Pilot.(*agent.LLMAgent); ok {
		t.Skip("LLM configured: scripted-pilot semantics only")
	}
	if err := m.AddGuidance("guide", "优先探路。"); err != nil {
		t.Fatalf("guide rejected: %v", err)
	}
	m.TickAuto()
	if len(m.TriggerLog) != 1 || m.TriggerLog[0] != TriggerGuidance {
		t.Fatalf("Guidance 应在下一 tick 恰好触发一次，log=%v", m.TriggerLog)
	}
	if v := m.View(); v.LastTrigger != TriggerGuidance {
		t.Fatalf("last_trigger = %q, want %q", v.LastTrigger, TriggerGuidance)
	}
	m.TickAuto() // 无新事件：安静（开局心跳已被 guidance 消费）
	if len(m.TriggerLog) != 1 {
		t.Fatalf("第二次 TickAuto 不应触发，log=%v", m.TriggerLog)
	}
}

// 整局触发序列抽查：脚本驾驶 MVP 一局，执行层行走 tick 不触发，
// arrive/resource 各就其位，且无相邻双触发（combat 除外）。
func TestMVPTriggerSequence(t *testing.T) {
	m := New("trig", 42, nil)
	if _, ok := m.Pilot.(*agent.LLMAgent); ok {
		t.Skip("LLM configured: scripted-pilot semantics only")
	}
	for m.Status == StatusRunning && m.W.Tick < m.TurnLimit {
		m.TickAuto()
	}
	if m.Status != StatusWon {
		t.Fatalf("expected win, got %s (%s)", m.Status, m.Summary())
	}
	var arrivals, resources int
	for i, tr := range m.TriggerLog {
		switch tr {
		case TriggerArrive:
			arrivals++
		case TriggerResource:
			resources++
		}
		// 相邻 tick 连续触发只允许发生在战斗层
		if i > 0 && m.TriggerLog[i-1] == tr && tr != TriggerCombat {
			t.Fatalf("non-combat trigger fired on consecutive ticks: %v", m.TriggerLog)
		}
	}
	if arrivals != 1 { // 取晶一程触发 arrive；回程抵达即胜利，当 tick 结算
		// 胜利（checkEnd）无需再花一次调用——这是事件驱动模型的预算红利
		t.Errorf("arrivals = %d, want 1 (log %v)", arrivals, m.TriggerLog)
	}
	if resources != 1 { // 首次 pickup 恰好一次
		t.Errorf("resources = %d, want 1 (log %v)", resources, m.TriggerLog)
	}
	t.Logf("MVP trigger log: %v (llm_calls=%d)", m.TriggerLog, m.llmCalls)
}

// countingPilot 是预算集成测试的计数器替身：包装 ScriptedAgent，统计
// "本应触发的 pilot 决策次数"（scripted 不真调用 LLM）。
type countingPilot struct {
	inner *agent.ScriptedAgent
	calls int
}

func (cp *countingPilot) Decide(obs *engine.Observation) []engine.Intent {
	return cp.inner.Decide(obs)
}

func (cp *countingPilot) DecideOrder(obs *engine.Observation) []engine.Intent {
	cp.calls++
	return cp.inner.DecideOrder(obs)
}

// 预算验收（评审 §7 补充要求）：ScriptedAgent 经计数器替身驾驶 V04 一局
// 60 回合，触发次数 ≤ 50。
func TestV04LLMBudget(t *testing.T) {
	m := NewV04("budget", 20260917, nil)
	inner, ok := m.Pilot.(*agent.ScriptedAgent)
	if !ok {
		t.Skip("LLM configured: budget counts trigger-driven pilot calls")
	}
	cp := &countingPilot{inner: inner}
	m.Pilot = cp
	for m.Status == StatusRunning && m.W.Tick < m.TurnLimit {
		m.TickAuto()
	}
	if cp.calls > 50 {
		t.Fatalf("预算超限：%d 次触发（上限 50）；序列 %v", cp.calls, m.TriggerLog)
	}
	if cp.calls != m.llmCalls {
		t.Errorf("llm_calls = %d，替身计数 = %d，应一致", m.llmCalls, cp.calls)
	}
	if v := m.View(); v.LLMCalls != cp.calls {
		t.Errorf("view llm_calls = %d, want %d", v.LLMCalls, cp.calls)
	}
	joined := strings.Join(m.TriggerLog, ",")
	for _, want := range []string{TriggerHeartbeat, TriggerResource} {
		if !strings.Contains(joined, want) {
			t.Errorf("V04 触发序列应包含 %s：got %v", want, m.TriggerLog)
		}
	}
	t.Logf("V04 预算：%d 回合触发 %d 次（≤50）；分布 %v",
		m.W.Tick, cp.calls, triggerHistogram(m.TriggerLog))
}

func triggerHistogram(log []string) map[string]int {
	h := map[string]int{}
	for _, tr := range log {
		h[tr]++
	}
	return h
}

// 去抖状态进重放：同 seed 两次运行，触发序列与事件流逐字节一致
// (design §2.5/§3.2)。
func TestV04TriggerSequenceDeterminism(t *testing.T) {
	run := func() ([]string, string) {
		m := NewV04("det", 20260917, nil)
		if _, ok := m.Pilot.(*agent.LLMAgent); ok {
			t.Skip("LLM configured: scripted-pilot semantics only")
		}
		for m.Status == StatusRunning && m.W.Tick < m.TurnLimit {
			m.TickAuto()
		}
		var b strings.Builder
		for _, e := range m.Events {
			fmt.Fprintf(&b, "[%d]%s#%d:%s\n", e.Tick, e.Type, e.Entity, e.Text)
		}
		return m.TriggerLog, b.String()
	}
	tl1, ev1 := run()
	tl2, ev2 := run()
	if !reflect.DeepEqual(tl1, tl2) {
		t.Fatalf("触发序列不一致：\nA=%v\nB=%v", tl1, tl2)
	}
	if ev1 != ev2 {
		t.Fatalf("事件流不一致：\n--- A ---\n%s\n--- B ---\n%s", ev1, ev2)
	}
}

// —— v0.5 WPB：equip 进预测口径（装配类意图 → "assemble"）——

func TestClassifyEquipAsAssemble(t *testing.T) {
	w := engine.NewWorldTerrain(1, nil)
	p := &engine.Entity{
		Kind: "agent", Owner: "player", Essence: engine.Wood,
		Integrity: 100, MaxIntegrity: 100, Battery: 100, MaxBattery: 100,
	}
	w.Spawn(p)
	m := &Match{W: w, PlayerID: p.ID}
	m.lastIntents = []engine.Intent{{
		Type:      "equip",
		Blueprint: []engine.CompSpec{{Type: engine.CEmitter}},
	}}
	if got := m.classifyIntents(); got != "assemble" {
		t.Fatalf("equip 预测口径 = %q, want %q", got, "assemble")
	}
}
