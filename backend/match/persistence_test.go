package match

// persistence_test.go —— WP-B 对局持久化测试：
//
//   - 落盘 → 重启（新建 Registry）→ 惰性恢复 → 引擎世界与元状态与原局
//     一致 → 继续 tick 与原局延续逐字节一致（含触发节奏、经济、预测、
//     录像续帧）；
//   - 损坏文件（坏 JSON / 缺录像）跳过，不阻塞其它对局；
//   - 重演不一致（篡改事件基准 / 帧序断裂）拒绝恢复，跳过并保留其余.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"agentbattle/backend/engine"
)

// assertSameMatch asserts two matches are the same match——引擎世界逐字
// 段、事件流、经济、干预、预测、触发节奏、录像帧全部一致.
func assertSameMatch(t *testing.T, a, b *Match, phase string) {
	t.Helper()
	if got, want := stateDump(a), stateDump(b); got != want {
		t.Fatalf("%s：引擎状态分歧：\n--- a ---\n%s\n--- b ---\n%s", phase, got, want)
	}
	if a.Influence != b.Influence {
		t.Fatalf("%s：Influence %v ≠ %v", phase, a.Influence, b.Influence)
	}
	if !slices.Equal(a.Events, b.Events) {
		t.Fatalf("%s：事件流分歧（%d vs %d 条）", phase, len(a.Events), len(b.Events))
	}
	if !slices.Equal(a.Guidance, b.Guidance) {
		t.Fatalf("%s：Guidance 分歧", phase)
	}
	if !reflect.DeepEqual(a.Predictions, b.Predictions) {
		t.Fatalf("%s：预测分歧：%v vs %v", phase, a.Predictions, b.Predictions)
	}
	if a.pausedOnce != b.pausedOnce || a.pauseNext != b.pauseNext {
		t.Fatalf("%s：暂停状态分歧（%v/%v vs %v/%v）", phase,
			a.pausedOnce, a.pauseNext, b.pausedOnce, b.pauseNext)
	}
	if !slices.Equal(a.TriggerLog, b.TriggerLog) || a.lastTrigger != b.lastTrigger {
		t.Fatalf("%s：触发节奏分歧：%v/%s vs %v/%s", phase,
			a.TriggerLog, a.lastTrigger, b.TriggerLog, b.lastTrigger)
	}
	if a.llmCalls != b.llmCalls {
		t.Fatalf("%s：pilot 调用计数 %d ≠ %d", phase, a.llmCalls, b.llmCalls)
	}
	if a.PilotName != b.PilotName {
		t.Fatalf("%s：pilot 名 %s ≠ %s", phase, a.PilotName, b.PilotName)
	}
	ra, rb := a.Recording(), b.Recording()
	if len(ra.Frames) != len(rb.Frames) {
		t.Fatalf("%s：录像帧数 %d ≠ %d", phase, len(ra.Frames), len(rb.Frames))
	}
	for i := range ra.Frames {
		if !reflect.DeepEqual(ra.Frames[i], rb.Frames[i]) {
			t.Fatalf("%s：录像第 %d 帧分歧", phase, i)
		}
	}
	if !slices.Equal(ra.Guidance, rb.Guidance) {
		t.Fatalf("%s：录像携带的 Guidance 分歧", phase)
	}
}

// TestPersistenceRoundTripAndContinuation: 落盘 → 重启（新建 Registry）
// → 惰性恢复 → 与原局一致 → 继续 tick 与原局延续一致（混合驱动：自动
// tick、手动意图、Guidance、预测、紧急暂停全部覆盖）。
func TestPersistenceRoundTripAndContinuation(t *testing.T) {
	dir := t.TempDir()
	reg1 := NewRegistryPersisted(nil, dir)
	m1 := reg1.Create(42, "v04")

	// 新局即刻落盘（tick 之前重启不丢档）。
	if _, err := os.Stat(filepath.Join(dir, m1.ID+".json")); err != nil {
		t.Fatalf("新局未落盘：%v", err)
	}

	for i := 0; i < 6; i++ {
		m1.TickAuto()
	}
	if err := m1.AddGuidance("guide", "优先生存，避免无谓战斗。"); err != nil {
		t.Fatalf("add guidance: %v", err)
	}
	m1.TickAuto()
	from := m1.W.Entities[m1.PlayerID].Pos // 手动 tick：显式意图进引擎
	to := from.Add(engine.HexDirs[1])
	m1.Tick([]engine.Intent{{Type: "move", To: &to, Power: 600}})
	if err := m1.Predict("move"); err != nil { // 未结算预测跨重启
		t.Fatalf("predict: %v", err)
	}
	m1.TickAuto()
	if err := m1.Pause(); err != nil { // pauseNext 挂起跨重启
		t.Fatalf("pause: %v", err)
	}

	// —— 重启：全新 Registry 扫描同一目录 ——
	reg2 := NewRegistryPersisted(nil, dir)
	m2 := reg2.Get(m1.ID)
	if m2 == nil {
		t.Fatal("重启后未恢复对局")
	}
	assertSameMatch(t, m1, m2, "恢复后")

	// —— 续玩：恢复局与原局延续一致（首个 auto tick 消耗挂起的暂停）——
	m1.TickAuto()
	m2.TickAuto()
	assertSameMatch(t, m1, m2, "暂停消耗后")
	for i := 0; i < 12; i++ {
		m1.TickAuto()
		m2.TickAuto()
	}
	assertSameMatch(t, m1, m2, "续玩 12 tick 后")

	// 恢复局继续落盘，且编号不与存量冲突.
	if _, err := os.Stat(filepath.Join(dir, m2.ID+".json")); err != nil {
		t.Fatalf("恢复局未续写落盘：%v", err)
	}
	fresh := reg2.Create(7, "v04")
	if fresh == nil || fresh.ID == m1.ID {
		t.Fatalf("重启后新建对局编号冲突：%s", fresh.ID)
	}
	t.Logf("对局 %s（%d tick）落盘→重启→恢复→续玩逐字节一致；新局编号 %s",
		m1.ID, m1.W.Tick, fresh.ID)
}

// TestPersistenceCorruptFileSkipped: 损坏文件（坏 JSON、缺录像）跳过并
// log，不阻塞其它对局的恢复.
func TestPersistenceCorruptFileSkipped(t *testing.T) {
	dir := t.TempDir()
	reg1 := NewRegistryPersisted(nil, dir)
	good := reg1.Create(7, "v04")
	good.TickAuto()

	if err := os.WriteFile(filepath.Join(dir, "m9001.json"), []byte("{ not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "m9002.json"),
		[]byte(`{"id":"m9002","scenario":"scenario/infinite-0.4"}`), 0o644); err != nil {
		t.Fatal(err) // 合法 JSON 但缺录像
	}

	reg2 := NewRegistryPersisted(nil, dir)
	if reg2.Get("m9001") != nil {
		t.Fatal("坏 JSON 文件应被跳过")
	}
	if reg2.Get("m9002") != nil {
		t.Fatal("缺录像的文件应被跳过")
	}
	if m := reg2.Get(good.ID); m == nil {
		t.Fatal("损坏文件不应阻塞好局的恢复")
	} else if m.W.Tick != good.W.Tick {
		t.Fatalf("恢复 tick = %d, want %d", m.W.Tick, good.W.Tick)
	}
}

// TestPersistenceReplayMismatchSkipped: 落盘内容被篡改（事件基准分歧 /
// 帧序断裂）→ 重演校验拒绝恢复、跳过该对局，好局不受影响.
func TestPersistenceReplayMismatchSkipped(t *testing.T) {
	dir := t.TempDir()
	reg1 := NewRegistryPersisted(nil, dir)
	a := reg1.Create(9, "v04")
	b := reg1.Create(11, "v04")
	for i := 0; i < 5; i++ {
		a.TickAuto()
		b.TickAuto()
	}

	// a：篡改一帧的事件基准 → ErrReplayMismatch。
	data, err := os.ReadFile(filepath.Join(dir, a.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var pm persistedMatch
	if err := json.Unmarshal(data, &pm); err != nil {
		t.Fatal(err)
	}
	pm.Recording.Frames[0].ResultingEvents = append(pm.Recording.Frames[0].ResultingEvents,
		engine.Event{Tick: 1, Type: "intent_rejected", Entity: 999, Text: "篡改的事件"})
	if data, err = json.Marshal(pm); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, a.ID+".json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// b：删掉中间一帧 → 帧序断裂。
	data, err = os.ReadFile(filepath.Join(dir, b.ID+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &pm); err != nil {
		t.Fatal(err)
	}
	if len(pm.Recording.Frames) < 3 {
		t.Fatalf("帧数不足，无法构造断序：%d", len(pm.Recording.Frames))
	}
	pm.Recording.Frames = append(pm.Recording.Frames[:1], pm.Recording.Frames[2:]...)
	if data, err = json.Marshal(pm); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, b.ID+".json"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	reg2 := NewRegistryPersisted(nil, dir)
	if reg2.Get(a.ID) != nil {
		t.Fatal("重演不一致的对局应被拒绝")
	}
	if reg2.Get(b.ID) != nil {
		t.Fatal("帧序断裂的对局应被拒绝")
	}
	// 其余对局不受影响：新建编号也已越过被拒对局.
	if fresh := reg2.Create(13, "v04"); fresh == nil || fresh.ID == a.ID || fresh.ID == b.ID {
		t.Fatalf("被拒对局之后新建编号异常：%s", fresh.ID)
	}
	t.Logf("篡改/断序对局 %s、%s 被拒并跳过，注册表其余功能正常", a.ID, b.ID)
}

// TestPersistenceFinishedMatchRestored: 终局对局（timeout）恢复后状态与
// 复盘俱在，且不再推进.
func TestPersistenceFinishedMatchRestored(t *testing.T) {
	dir := t.TempDir()
	reg1 := NewRegistryPersisted(nil, dir)
	m1 := reg1.Create(42, "v04") // seed 42：scripted pilot 恰好跑满 60 回合
	for i := 0; i < 60 && m1.Status == StatusRunning; i++ {
		m1.TickAuto()
	}
	if m1.Status != StatusTimeout || m1.ReviewRes == nil {
		t.Fatalf("原局未按预期终局：status=%s review=%v", m1.Status, m1.ReviewRes)
	}

	m2 := NewRegistryPersisted(nil, dir).Get(m1.ID)
	if m2 == nil {
		t.Fatal("终局对局未恢复")
	}
	assertSameMatch(t, m1, m2, "终局恢复后")
	m2.TickAuto() // 终局不再推进
	if m2.W.Tick != m1.W.Tick || m2.Status != StatusTimeout {
		t.Fatalf("终局对局被推进：tick=%d status=%s", m2.W.Tick, m2.Status)
	}
}
