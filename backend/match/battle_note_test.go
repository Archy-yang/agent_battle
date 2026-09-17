package match

// battle_note 战斗解说测试（combat 触发的 LLM 响应 → commentary 事件）：
//
//   - 协议解析：combat 触发的假响应体解析出 battle_note → LastBattleNote；
//     下一轮响应未携带 / 不可解析时覆盖为空，不残留上一轮；
//   - match 接线：combat tick 的 battle_note 成为一条 Type "commentary"
//     的解说事件——只进展示流（Events / Commentary / StateView），不进
//     引擎事件流（lastEvs）、不进录像帧的 ResultingEvents；>50 字兜底
//     截断；
//   - 重放一致性：帧只记引擎事件 → 含解说的局仍可逐帧精确重演；重放局
//     不重跑 LLM，故不伪造解说（展示流 = 原局展示流滤掉 commentary）；
//   - 全路径：TickAuto 真实接敌（InCombat → 分类器裁决 combat）同样接
//     线；非 combat 触发不产生解说。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"agentbattle/backend/agent"
	"agentbattle/backend/engine"
)

// newFakeLLM spins one OpenAI-compatible chat endpoint whose reply
// content is the JSON string at *content——测试改写 pointee 即可改变
// agent 的下一次决策。
func newFakeLLM(t *testing.T, content string) (*httptest.Server, *string) {
	t.Helper()
	cur := &content
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": *cur}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	t.Cleanup(srv.Close)
	return srv, cur
}

// decisionJSON renders one pilot decision reply (intents + cognition +
// 可选 battle_note) as the chat message content.
func decisionJSON(t *testing.T, intents []map[string]any, cognition map[string]any, note string) string {
	t.Helper()
	d := map[string]any{"intents": intents, "cognition": cognition}
	if note != "" {
		d["battle_note"] = note
	}
	b, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal decision: %v", err)
	}
	return string(b)
}

// newFakePilotMatch builds a v04 match whose pilot is an LLM agent wired
// to the fake endpoint (不装载 profile，纯单测).
func newFakePilotMatch(t *testing.T, seed uint64, content string) (*Match, *string) {
	t.Helper()
	srv, cur := newFakeLLM(t, content)
	m := NewV04("note", seed, nil)
	m.Pilot = agent.NewLLM(agent.LLMConfig{BaseURL: srv.URL, Model: "fake"})
	m.PilotName = "llm"
	return m, cur
}

// assertNoCommentaryIn fails the test if any commentary event appears in
// evs（引擎事件流 / 录像帧必须保持纯净）.
func assertNoCommentaryIn(t *testing.T, evs []engine.Event, where string) {
	t.Helper()
	for _, ev := range evs {
		if ev.Type == "commentary" {
			t.Fatalf("%s: commentary leaked into the engine-side stream: %+v", where, ev)
		}
	}
}

// TestBattleNoteParse: combat 触发的响应解析出 battle_note；未携带与
// 不可解析的响应覆盖为空（LastBattleNote 不残留上一轮）。
func TestBattleNoteParse(t *testing.T) {
	const note = "它金甲厚重，我改打它的水行储气"
	m, content := newFakePilotMatch(t, 42, decisionJSON(t,
		[]map[string]any{{"type": "wait"}},
		map[string]any{"goal": "活下来", "confidence": 0.6}, note))
	la := m.Pilot.(*agent.LLMAgent)

	la.SetTrigger(TriggerCombat)
	intents := la.DecideTriggered(m.Observe())
	if len(intents) != 1 || intents[0].Type != "wait" {
		t.Fatalf("intents = %+v, want one wait intent", intents)
	}
	if got := la.LastBattleNote(); got != note {
		t.Fatalf("LastBattleNote = %q, want %q", got, note)
	}

	// 下一轮响应未携带 battle_note → 覆盖为空.
	*content = decisionJSON(t, nil, map[string]any{"goal": "再探", "confidence": 0.5}, "")
	la.SetTrigger(TriggerCombat)
	_ = la.DecideTriggered(m.Observe())
	if got := la.LastBattleNote(); got != "" {
		t.Fatalf("absent field kept stale note %q", got)
	}

	// 不可解析的响应 → 同样清空.
	*content = "这不是 JSON"
	la.SetTrigger(TriggerCombat)
	_ = la.DecideTriggered(m.Observe())
	if got := la.LastBattleNote(); got != "" {
		t.Fatalf("unparseable reply kept stale note %q", got)
	}
}

// TestCommentaryWiringAndReplayCompat: combat tick 的 battle_note 成为
// 一条 commentary 事件（只进展示流），含解说的局仍可精确重演.
func TestCommentaryWiringAndReplayCompat(t *testing.T) {
	const note = "金狼在侧，我先拉开半格距离再打"
	m, content := newFakePilotMatch(t, 42, decisionJSON(t, nil,
		map[string]any{"goal": "侦察", "confidence": 0.5}, note))

	m.pilotTick(m.Pilot.(*agent.LLMAgent), TriggerCombat)
	if got := len(m.Commentary); got != 1 {
		t.Fatalf("commentary count = %d, want 1", got)
	}
	ev := m.Commentary[0]
	if ev.Type != "commentary" || ev.Text != note || ev.Entity != m.PlayerID || ev.Tick != m.W.Tick {
		t.Fatalf("commentary event = %+v, want {commentary %q by player at tick %d}", ev, note, m.W.Tick)
	}
	if last := m.Events[len(m.Events)-1]; last.Type != "commentary" {
		t.Fatalf("display stream tail = %+v, want commentary", last)
	}
	assertNoCommentaryIn(t, m.lastEvs, "engine event stream (lastEvs)")
	if n := len(m.rec.Frames); n == 0 {
		t.Fatal("no frame recorded for the combat tick")
	} else {
		assertNoCommentaryIn(t, m.rec.Frames[n-1].ResultingEvents, "recorded frame")
	}

	// StateView 追加：Commentary 字段 + 展示流里混入 commentary.
	v := m.View()
	if len(v.Commentary) != 1 || v.Commentary[0].Text != note {
		t.Fatalf("view commentary = %+v, want one %q", v.Commentary, note)
	}
	inEvents := false
	for _, e := range v.Events {
		if e.Type == "commentary" {
			inEvents = true
		}
	}
	if !inEvents {
		t.Fatal("commentary missing from the display stream in StateView.Events")
	}

	// >50 字兜底截断.
	*content = decisionJSON(t, nil, map[string]any{"confidence": 0.5}, strings.Repeat("金", 60))
	m.pilotTick(m.Pilot.(*agent.LLMAgent), TriggerCombat)
	got := m.Commentary[len(m.Commentary)-1].Text
	if want := strings.Repeat("金", battleNoteMax); got != want {
		t.Fatalf("overlong note truncated to %d runes, want %d", len([]rune(got)), battleNoteMax)
	}

	// 重放一致性：帧只记引擎事件 → 含解说的局逐帧精确重演；重放局不重跑
	// LLM，故不伪造解说——其展示流 = 原局展示流滤掉 commentary.
	rm, err := Replay(m.Recording(), m.W.Tick)
	if err != nil {
		t.Fatalf("replay of commentary-bearing recording: %v", err)
	}
	if len(rm.Commentary) != 0 {
		t.Fatalf("replay fabricated %d commentary events; LLM must not be re-run", len(rm.Commentary))
	}
	var origEngine []engine.Event
	for _, e := range m.Events {
		if e.Type != "commentary" {
			origEngine = append(origEngine, e)
		}
	}
	if !slices.Equal(rm.Events, origEngine) {
		t.Fatalf("replayed stream diverges from the original engine stream: %d vs %d events",
			len(rm.Events), len(origEngine))
	}
	t.Logf("含 %d 条解说的 %d 帧录像精确重演；解说只活在展示流", len(m.Commentary), len(m.rec.Frames))
}

// TestCommentaryViaTickAutoCombat: 真实接敌（未知体挪到相邻格 →
// InCombat → 分类器裁决 combat）走 TickAuto 全路径同样接线；非 combat
// 触发不产生解说。
func TestCommentaryViaTickAutoCombat(t *testing.T) {
	const note = "它贴脸了，先把作用功率压到反噬线以下"
	m, _ := newFakePilotMatch(t, 7, decisionJSON(t, nil,
		map[string]any{"goal": "周旋", "confidence": 0.4}, note))

	p := m.W.Entities[m.PlayerID]
	u := m.W.Entities[m.UnknownID]
	placed := false
	for _, dir := range engine.HexDirs { // 未知体挪到玩家相邻的空格
		cand := p.Pos.Add(dir)
		if m.W.At(cand).Passable && m.W.EntityAt(cand) == nil {
			u.Pos = cand
			placed = true
			break
		}
	}
	if !placed || !m.InCombat() {
		t.Fatal("could not stage an adjacent hostile for the combat branch")
	}

	m.TickAuto()
	if m.lastTrigger != TriggerCombat {
		t.Fatalf("trigger = %q, want combat", m.lastTrigger)
	}
	if got := len(m.Commentary); got != 1 {
		t.Fatalf("TickAuto combat produced %d commentary events, want 1", got)
	}
	if m.Commentary[0].Text != note {
		t.Fatalf("commentary text = %q, want %q", m.Commentary[0].Text, note)
	}
	assertNoCommentaryIn(t, m.lastEvs, "engine event stream (lastEvs)")
	assertNoCommentaryIn(t, m.rec.Frames[len(m.rec.Frames)-1].ResultingEvents, "recorded frame")

	// 再推一 tick：若触发不再是 combat，解说不得新增.
	m.TickAuto()
	if m.lastTrigger != TriggerCombat && len(m.Commentary) != 1 {
		t.Fatalf("non-combat tick appended commentary (%d total)", len(m.Commentary))
	}
}
