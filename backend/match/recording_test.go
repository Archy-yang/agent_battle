package match

// v0.7 录像与重放测试 (world_genesis §7 / §7.1)：
//
//   - V04 一局 60 回合（scripted pilot）：录制 → 重放到 15/30/60 三个
//     采样点 → 引擎状态逐字段一致、事件流一致；
//   - 含 Guidance 的局：Guidance 只影响 LLM 上下文（此处影响分类器触
//     发节奏），回放按记录的 Intent 流重演、不重新消化 Guidance；
//   - 篡改一帧意图 → ErrReplayMismatch → 端点 500 replay mismatch；
//   - 端点契约：GET /api/match/{id}/recording、POST /api/replay
//     （JSON 往返后仍可精确重演；不碰注册表）。

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"agentbattle/backend/engine"
)

// stateDump is a deterministic full dump of the engine-side state of a
// match: tick/status/entities（含 Standing 订单与组件）/materials/explored.
// 经济侧（Influence、预测）不入 dump：Guidance 计费不进引擎，回放局
// 经济中立（见 recording.go 文件头）。
func stateDump(m *Match) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "tick=%d status=%s seed=%d pack=%s\n", m.W.Tick, m.Status, m.Seed, m.PackName)
	for _, e := range m.W.SortedEntities() {
		fmt.Fprintf(&sb, "#%d %s %s %v 形%d/%d 热%d 储%d/%d 携%d",
			e.ID, e.Kind, e.Owner, e.Pos, e.Integrity, e.MaxIntegrity,
			e.Heat, e.Battery, e.MaxBattery, len(e.Carrying))
		if e.OrderTo != nil {
			fmt.Fprintf(&sb, " 订单→%v", *e.OrderTo)
		}
		for _, c := range e.Components {
			fmt.Fprintf(&sb, " [%s %s %v]", c.Type, c.Essence, c.Active)
		}
		sb.WriteByte('\n')
	}
	owners := make([]string, 0, len(m.W.Materials))
	for o := range m.W.Materials {
		owners = append(owners, o)
	}
	slices.Sort(owners)
	for _, o := range owners {
		inv := m.W.Materials[o]
		keys := make([]engine.Essence, 0, len(inv))
		for k := range inv {
			keys = append(keys, k)
		}
		slices.SortFunc(keys, func(a, b engine.Essence) int { return strings.Compare(a.String(), b.String()) })
		fmt.Fprintf(&sb, "物料[%s]:", o)
		for _, k := range keys {
			fmt.Fprintf(&sb, " %s×%d", k, inv[k])
		}
		sb.WriteByte('\n')
	}
	explored := make([]engine.Hex, 0, len(m.Explored))
	for p := range m.Explored {
		explored = append(explored, p)
	}
	sortHexes(explored)
	fmt.Fprintf(&sb, "explored=%v\n", explored)
	return sb.String()
}

// runAndSnapshot drives one match with TickAuto，在 snapshotTicks 逐字段
// 快照（状态 dump + 事件流副本），跑满 turns 或对局结束；hooks 在每个
// tick 之后按 tick 号触发（注入 Guidance 等干预）.
func runAndSnapshot(t *testing.T, m *Match, turns int64, snapshotTicks map[int64]bool,
	hooks ...func(tick int64)) (*Recording, map[int64]string, map[int64][]engine.Event) {
	t.Helper()
	wantState := map[int64]string{}
	wantEvents := map[int64][]engine.Event{}
	for i := int64(0); i < turns && m.Status == StatusRunning; i++ {
		m.TickAuto()
		for _, h := range hooks {
			h(m.W.Tick)
		}
		if snapshotTicks[m.W.Tick] {
			wantState[m.W.Tick] = stateDump(m)
			wantEvents[m.W.Tick] = append([]engine.Event(nil), m.Events...)
		}
	}
	return m.Recording(), wantState, wantEvents
}

// assertReplayMatches checks a replay at untilTick against the original
// snapshots: 状态逐字段一致、事件流一致.
func assertReplayMatches(t *testing.T, rec *Recording, untilTick int64,
	wantState map[int64]string, wantEvents map[int64][]engine.Event) *Match {
	t.Helper()
	rm, err := Replay(rec, untilTick)
	if err != nil {
		t.Fatalf("replay to tick %d failed: %v", untilTick, err)
	}
	if got, want := stateDump(rm), wantState[untilTick]; got != want {
		t.Fatalf("replay to tick %d state mismatch:\n--- replay ---\n%s\n--- original ---\n%s", untilTick, got, want)
	}
	if !slices.Equal(rm.Events, wantEvents[untilTick]) {
		t.Fatalf("replay to tick %d event stream mismatch: %d vs %d events",
			untilTick, len(rm.Events), len(wantEvents[untilTick]))
	}
	return rm
}

// TestRecordingReplayV04: 一局 60 回合（scripted pilot）→ 重放到
// 15/30/60 三个采样点，状态与事件流逐项一致；录像经 JSON 往返后仍可
// 精确重演（API 契约自证）。
//
// seed 42 依据（v0.9 懒怪战斗化重采样）：原 seed 7 的 scripted pilot 在
// 战斗化的懒激活怪围攻下 t=10 早夭（v0.8 起已知，见 match.go lazySpawns
// 注释）；1..400 升序扫描（scripted TickAuto、无夹具）中 6/11/16/29/42…
// 均跑满 60 回合，取项目惯用 seed 42——timeout 终局、60 帧完整、
// 重放窗口不缺角。
func TestRecordingReplayV04(t *testing.T) {
	m := NewV04("rec", 42, nil) // seed 42：scripted pilot 恰好跑满 60 回合（timeout）
	rec, wantState, wantEvents := runAndSnapshot(t, m, 60, map[int64]bool{15: true, 30: true, 60: true})

	if m.W.Tick != 60 {
		t.Fatalf("run ended early at tick %d (%s)", m.W.Tick, m.Status)
	}
	if len(rec.Frames) != 60 {
		t.Fatalf("recorded %d frames, want 60", len(rec.Frames))
	}
	for i, f := range rec.Frames {
		if f.Tick != int64(i+1) {
			t.Fatalf("frame %d has tick %d, want %d", i, f.Tick, i+1)
		}
	}
	autoWithIntents := 0
	for _, f := range rec.Frames {
		if f.Auto && len(f.PlayerIntents) > 0 {
			autoWithIntents++
		}
	}
	if autoWithIntents == 0 {
		t.Fatal("no auto frame with recorded intents — pilot decisions never recorded, replay would be vacuous")
	}

	for _, until := range []int64{15, 30, 60} {
		assertReplayMatches(t, rec, until, wantState, wantEvents)
	}

	// JSON 往返（GET recording 的传输形态）后重演仍精确一致.
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal recording: %v", err)
	}
	var back Recording
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal recording: %v", err)
	}
	rm := assertReplayMatches(t, &back, 60, wantState, wantEvents)
	if got, want := rm.View().Turn, int64(60); got != want {
		t.Fatalf("replayed view turn = %d, want %d", got, want)
	}
	t.Logf("60 帧录像（%d 字节 JSON）在 15/30/60 三个采样点逐字段重演一致", len(data))
}

// TestRecordingReplayWithGuidance: 带 Guidance 的局——Guidance 进入 LLM
// 上下文/分类器触发节奏，不影响引擎决定性；回放把意图流按记录重演，
// 不重新消化 Guidance（回放局 Guidance 为空、Influence 中立）。
func TestRecordingReplayWithGuidance(t *testing.T) {
	m := NewV04("g", 20260917, nil)
	rec, wantState, wantEvents := runAndSnapshot(t, m, 40, map[int64]bool{10: true, 40: true},
		withGuidance(t, m, 5, "guide", "优先生存，远离火迹。"),
		withGuidance(t, m, 20, "whisper", "注意储气。"),
	)

	if got := len(rec.Guidance); got != 2 {
		t.Fatalf("recording carries %d guidance entries, want 2", got)
	}
	for _, until := range []int64{10, 40} {
		rm := assertReplayMatches(t, rec, until, wantState, wantEvents)
		if len(rm.Guidance) != 0 {
			t.Fatalf("replay digested %d guidance entries; replay must not re-digest guidance", len(rm.Guidance))
		}
	}
	// 重放校验隐含：Guidance 改变过的决策 tick（被 ArmGuidance 唤醒的
	// 帧）已按记录的意图重演——存在被 Guidance 触发的自动帧.
	found := false
	for _, f := range rec.Frames {
		if f.Auto && len(f.PlayerIntents) > 0 {
			found = true
		}
	}
	if !found {
		t.Fatal("guidance never woke the pilot — test would be vacuous")
	}
}

// withGuidance 是 runAndSnapshot 的注入缝：在到达 atTick 后记一条
// Guidance（原局逐 tick 驱动时按序调用）.
func withGuidance(t *testing.T, m *Match, atTick int64, kind, text string) func(int64) {
	t.Helper()
	return func(tick int64) {
		if tick == atTick {
			if err := m.AddGuidance(kind, text); err != nil {
				t.Fatalf("add guidance at tick %d: %v", atTick, err)
			}
		}
	}
}

// TestReplayMismatchDetection: 篡改一帧的玩家意图 → 重演事件与记录的
// ResultingEvents 分歧 → ErrReplayMismatch → 端点 500 replay mismatch.
func TestReplayMismatchDetection(t *testing.T) {
	m := NewV04("rec", 31337, nil)
	rec, _, _ := runAndSnapshot(t, m, 30, nil)

	tampered := 0
	for i := range rec.Frames {
		if len(rec.Frames[i].PlayerIntents) > 0 {
			to := engine.Hex{Q: int(rec.Frames[i].Tick) + 9, R: -int(rec.Frames[i].Tick)} // 偏离原路线
			rec.Frames[i].PlayerIntents = []engine.Intent{{Type: "move", To: &to, Power: 600}}
			tampered = i + 1
			break
		}
	}
	if tampered == 0 {
		t.Fatal("no frame with intents to tamper")
	}
	if _, err := Replay(rec, 30); !errors.Is(err, ErrReplayMismatch) {
		t.Fatalf("tampered frame %d: got err %v, want ErrReplayMismatch", tampered, err)
	}

	// 端点形态：500 + {"error":"replay mismatch"}.
	w := httptest.NewRecorder()
	ServeReplay(w, httptest.NewRequest(http.MethodPost, "/api/replay", strings.NewReader(recJSON(t, rec, 30))))
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("ServeReplay code = %d, want 500 (body %s)", w.Code, w.Body.String())
	}
	if got := w.Body.String(); !strings.Contains(got, `"replay mismatch"`) {
		t.Fatalf("ServeReplay body = %s, want replay mismatch", got)
	}
	t.Logf("篡改第 %d 帧意图 → 重演校验在线捕获，端点返回 500 replay mismatch", tampered)
}

func recJSON(t *testing.T, rec *Recording, untilTick int64) string {
	t.Helper()
	data, err := json.Marshal(ReplayRequest{Recording: *rec, UntilTick: untilTick})
	if err != nil {
		t.Fatalf("marshal replay request: %v", err)
	}
	return string(data)
}

// TestRecordingAPI: GET /api/match/{id}/recording 与 POST /api/replay
// 的端点契约；重放不往注册表里塞任何 match.
func TestRecordingAPI(t *testing.T) {
	reg := NewRegistry(nil)
	m := reg.Create(31337, "v04")
	for i := 0; i < 5; i++ {
		m.TickAuto()
	}

	// GET recording.（PathValue 由 ServeMux 注入；直调 handler 时手动设置）
	req := httptest.NewRequest(http.MethodGet, "/api/match/"+m.ID+"/recording", nil)
	req.SetPathValue("id", m.ID)
	w := httptest.NewRecorder()
	reg.ServeRecording(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("ServeRecording code = %d (body %s)", w.Code, w.Body.String())
	}
	var rec Recording
	if err := json.Unmarshal(w.Body.Bytes(), &rec); err != nil {
		t.Fatalf("decode recording: %v", err)
	}
	if rec.ID != m.ID || rec.Seed != m.Seed || rec.Scenario != m.PackName {
		t.Fatalf("recording header = %s/%d/%s, want %s/%d/%s",
			rec.ID, rec.Seed, rec.Scenario, m.ID, m.Seed, m.PackName)
	}
	if len(rec.Frames) != 5 {
		t.Fatalf("recording has %d frames, want 5", len(rec.Frames))
	}

	// 未知 match → 404.
	req = httptest.NewRequest(http.MethodGet, "/api/match/nope/recording", nil)
	req.SetPathValue("id", "nope")
	w = httptest.NewRecorder()
	reg.ServeRecording(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown match code = %d, want 404", w.Code)
	}

	// POST /api/replay → 一次性 match，状态 = until_tick，不进注册表.
	w = httptest.NewRecorder()
	ServeReplay(w, httptest.NewRequest(http.MethodPost, "/api/replay",
		strings.NewReader(recJSON(t, &rec, 5))))
	if w.Code != http.StatusOK {
		t.Fatalf("ServeReplay code = %d (body %s)", w.Code, w.Body.String())
	}
	var view StateView
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode replay view: %v", err)
	}
	if view.Turn != 5 || view.Observation == nil {
		t.Fatalf("replay view turn=%d observation=%v, want turn 5 with observation", view.Turn, view.Observation)
	}
	if len(view.Events) != len(m.Events) {
		t.Fatalf("replayed %d events, original has %d", len(view.Events), len(m.Events))
	}
	if got := reg.Get(view.ID); got != nil {
		t.Fatal("replay leaked into the registry")
	}

	// 坏请求：未知场景 → 400；空体 → 400.
	w = httptest.NewRecorder()
	ServeReplay(w, httptest.NewRequest(http.MethodPost, "/api/replay",
		strings.NewReader(`{"seed":1,"scenario":"bogus","frames":[]}`)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bogus scenario code = %d, want 400", w.Code)
	}

	// until_tick 缺省（0）= 全程重演，不误报超出范围.
	w = httptest.NewRecorder()
	data, _ := json.Marshal(ReplayRequest{Recording: rec})
	ServeReplay(w, httptest.NewRequest(http.MethodPost, "/api/replay", bytes.NewReader(data)))
	if w.Code != http.StatusOK {
		t.Fatalf("default until_tick code = %d (body %s)", w.Code, w.Body.String())
	}
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil || view.Turn != 5 {
		t.Fatalf("default until_tick replay turn=%d err=%v, want 5", view.Turn, err)
	}
}
