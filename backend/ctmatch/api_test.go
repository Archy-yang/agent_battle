package ctmatch

// api_test.go —— /api/cult/* HTTP 层测试（WP-5 验收）：
//   - 建会话 → tick → 视图含回执与定性观察；非 debug 载荷无真值；
//   - 玩家意图走会话入口并得回执（铁律 6）；止息后世界静息；
//   - ask 在无 LLM 配置时降级；llm 分身在未配置时建会话被拒；
//   - 功法起功出界 → 请示卡上浮，"止"可决之；
//   - 心跳唤醒 script 分身：常驻意图沿用，感知流持续滚动。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestMux 挂载与 worldd 相同的路由（PathValue 需要真实 ServeMux）。
func newTestMux(reg *CultRegistry) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/cult/session", reg.HandleCreate)
	mux.HandleFunc("GET /api/cult/session/{id}", reg.HandleView)
	mux.HandleFunc("POST /api/cult/session/{id}/intents", reg.HandleIntents)
	mux.HandleFunc("POST /api/cult/session/{id}/ask", reg.HandleAsk)
	mux.HandleFunc("POST /api/cult/session/{id}/guidance", reg.HandleGuidance)
	mux.HandleFunc("POST /api/cult/session/{id}/tick", reg.HandleTick)
	mux.HandleFunc("POST /api/cult/session/{id}/protocols", reg.HandleProtocols)
	mux.HandleFunc("POST /api/cult/session/{id}/protocols/{name}/start", reg.HandleStartProtocol)
	return mux
}

func post(t *testing.T, mux *http.ServeMux, path string, body any) (*httptest.ResponseRecorder, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, path, &buf)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode response %s: %v", rec.Body.String(), err)
	}
	return rec, out
}

func createSession(t *testing.T, mux *http.ServeMux, pilot, mode string) (string, *SessionView) {
	t.Helper()
	body := map[string]any{"seed": 7, "pilot": pilot}
	if mode != "" {
		body["mode"] = mode
	}
	rec, out := post(t, mux, "/api/cult/session", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: HTTP %d: %v", rec.Code, out)
	}
	data, _ := json.Marshal(out)
	var v SessionView
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("decode view: %v", err)
	}
	return v.ID, &v
}

func tick(t *testing.T, mux *http.ServeMux, id string, n int, intents []map[string]any) *SessionView {
	t.Helper()
	body := map[string]any{"ticks": n}
	if intents != nil {
		body["intents"] = intents
	}
	rec, out := post(t, mux, "/api/cult/session/"+id+"/tick", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("tick: HTTP %d: %v", rec.Code, out)
	}
	data, _ := json.Marshal(out)
	var v SessionView
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("decode view: %v", err)
	}
	return &v
}

// TestCultAPIScriptSessionFlow 建会话（tutorial：预载功法 + 固定走法
// 分身）→ 跑几十 tick → 视图含回执、定性观察、感知流；非 debug 载荷
// 无真值；?debug=1 在 AGENTBATTLE_DEV 开启时才有真值。
func TestCultAPIScriptSessionFlow(t *testing.T) {
	t.Setenv("AGENTBATTLE_DEV", "1")
	reg := NewCultRegistry()
	mux := newTestMux(reg)
	id, v0 := createSession(t, mux, "script", "tutorial")
	if v0.PilotKind != "script" || v0.Acuity != 1 {
		t.Fatalf("unexpected session header: %+v", v0)
	}
	// 8 穴 9 脉（A1 v0.2 主环拓扑：8 脉 + 新增 丹田→土府、阳枢→神庭，
	// 移除 阳枢→中庭、中庭→丹田）。
	if len(v0.Points) != 8 || len(v0.Body.Channels) != 9 {
		t.Fatalf("body graph not exposed: %d points, %d channels", len(v0.Points), len(v0.Body.Channels))
	}
	if len(v0.Protocols) != 1 || v0.Protocols[0].Status != StatusDraft {
		t.Fatalf("starter protocol missing: %+v", v0.Protocols)
	}

	v := tick(t, mux, id, 40, nil)
	if v.Tick != 40 {
		t.Fatalf("tick not advanced: %d", v.Tick)
	}
	// 感知流持续滚动（心跳唤醒 script 分身，常驻意图沿用）。
	if len(v.SenseStream) < 20 {
		t.Fatalf("sense stream too thin: %d lines", len(v.SenseStream))
	}
	// 回执出现（铁律 6）且为定性措辞。
	foundReceipt := false
	for _, r := range v.Receipts {
		if strings.Contains(r.Text, "已生效") || strings.Contains(r.Text, "于青关纳气") {
			foundReceipt = true
		}
		if strings.ContainsAny(r.Text, "0123456789") {
			t.Fatalf("receipt leaks numbers: %q", r.Text)
		}
	}
	if !foundReceipt {
		t.Fatalf("no applied receipt in view: %+v", v.Receipts)
	}
	// 定性观察：开度/稳固/损伤全部是词表词。
	wordIn := func(w string, words []string) bool {
		for _, x := range words {
			if w == x {
				return true
			}
		}
		return false
	}
	openWords := []string{"闭", "微开", "半开", "大开", "洞开"}
	stabWords := []string{"稳如磐石", "稳固", "微澜", "不稳", "将溃"}
	dmgWords := []string{"无损", "隐伤", "作痛", "重创", "脉裂"}
	for _, p := range v.Points {
		if !wordIn(p.Openness, openWords) || !wordIn(p.Stability, stabWords) || !wordIn(p.Damage, dmgWords) {
			t.Fatalf("point %s off vocabulary: %s/%s/%s", p.Name, p.Openness, p.Stability, p.Damage)
		}
		if p.PressureBand < 0 || p.PressureBand > 5 {
			t.Fatalf("pressure band out of range: %d", p.PressureBand)
		}
	}
	// 环境 chips 定性。
	if len(v.Env) != 5 {
		t.Fatalf("env chips: %d", len(v.Env))
	}
	for _, e := range v.Env {
		if e.Qi == "" || e.Activity == "" {
			t.Fatalf("env chip empty: %+v", e)
		}
	}
	// 非调试载荷无真值视图。
	if v.Debug != nil {
		t.Fatal("debug payload present without ?debug=1")
	}

	// ?debug=1 才有真值。
	req := httptest.NewRequest(http.MethodGet, "/api/cult/session/"+id+"?debug=1", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode debug view: %v", err)
	}
	if _, ok := raw["debug"]; !ok {
		t.Fatal("debug payload missing with ?debug=1")
	}
}

// TestCultAPIPlayerIntents 玩家意图走会话入口并得回执；止息后静息。
func TestCultAPIPlayerIntents(t *testing.T) {
	reg := NewCultRegistry()
	mux := newTestMux(reg)
	id, _ := createSession(t, mux, "script", "") // 缺省 mode=normal

	// 玩家：于金门纳气（微量）——走与 Pilot 相同的入口，记 FromPlayer。
	v := tick(t, mux, id, 1, []map[string]any{
		{"kind": "absorb", "target": "金门", "level": "trace"},
	})
	if len(v.MyIntents) == 0 || !strings.Contains(v.MyIntents[0].Summary, "金门") {
		t.Fatalf("player intent not journaled: %+v", v.MyIntents)
	}
	found := false
	for _, r := range v.Receipts {
		if strings.Contains(r.Text, "金门") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no receipt for player intent: %+v", v.Receipts)
	}

	// 玩家引用不存在的穴位：回执"此路不通"（会话层裁决，铁律 6）。
	v = tick(t, mux, id, 1, []map[string]any{
		{"kind": "absorb", "target": "不存在的穴", "level": "small"},
	})
	found = false
	for _, r := range v.Receipts {
		if strings.Contains(r.Text, "此路不通") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no rejection receipt: %+v", v.Receipts)
	}

	// 止息：常驻意图清空，世界静息（感知流只见平静）。
	v = tick(t, mux, id, 3, []map[string]any{{"kind": "stop"}})
	if v.Tick != 5 {
		t.Fatalf("tick mismatch: %d", v.Tick)
	}
	for _, l := range v.SenseStream {
		if l.Tick >= 45 && (strings.Contains(l.Text, "进入") || strings.Contains(l.Text, "加速")) {
			t.Fatalf("world still moving after stop: %q", l.Text)
		}
	}

	// 非法意图被拒。
	rec, out := post(t, mux, "/api/cult/session/"+id+"/tick",
		map[string]any{"intents": []map[string]any{{"kind": "absorb", "target": "青关", "level": "无理"}}})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad level not rejected: %d %v", rec.Code, out)
	}
}

// TestCultAPIAskDegraded 无 LLM 配置时 ask 降级；llm 分身建会话被拒。
func TestCultAPIAskDegraded(t *testing.T) {
	reg := NewCultRegistry()
	mux := newTestMux(reg)
	id, _ := createSession(t, mux, "script", "")

	rec, out := post(t, mux, "/api/cult/session/"+id+"/ask", map[string]any{"question": "今日身势如何？"})
	if rec.Code != http.StatusOK {
		t.Fatalf("ask should degrade gracefully, got %d", rec.Code)
	}
	if out["degraded"] != true || out["answer"] != "" {
		t.Fatalf("degraded ask payload: %v", out)
	}
	if s, _ := out["reason"].(string); s == "" {
		t.Fatal("degraded ask lacks reason")
	}

	// llm 分身在未配置环境下建会话被拒。
	rec, out = post(t, mux, "/api/cult/session", map[string]any{"pilot": "llm"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("llm pilot without config should 400, got %d", rec.Code)
	}
}

// TestCultAPIProtocolEscalation 起功 → 出界 → 请示卡上浮 → "止"决之。
func TestCultAPIProtocolEscalation(t *testing.T) {
	reg := NewCultRegistry()
	mux := newTestMux(reg)
	id, _ := createSession(t, mux, "script", "tutorial") // 起功用例需要预载《吐纳初诀》
	// 先止息：建立观测基准，同时清掉 script 分身的常驻动作意图
	//（否则玩家常驻意图会在下个 Tick 以"外意接管"暂停功法）。
	tick(t, mux, id, 2, []map[string]any{{"kind": "stop"}})

	// 起功（预载草创功法，有 validated_range，应能正常进入 Runtime）。
	rec, out := post(t, mux, "/api/cult/session/"+id+"/protocols/"+fmt.Sprintf("%s", "吐纳初诀")+"/start", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("start protocol: %d %v", rec.Code, out)
	}
	v := tick(t, mux, id, 6, nil)
	if v.Active != "吐纳初诀" {
		t.Fatalf("protocol not active: %q", v.Active)
	}
	if v.Trial == nil || v.Trial.Protocol != "吐纳初诀" {
		t.Fatalf("trial card missing: %+v", v.Trial)
	}

	// 收一份无 validated_range 的功法并起功：一出界即请示。
	yaml := `protocol: 越界诀
actions:
  - guide:
      from: 青关
      to: 中庭
      level: medium
cadence: 1
cycles: 3
`
	rec, out = post(t, mux, "/api/cult/session/"+id+"/protocols", map[string]any{"yaml": yaml})
	if rec.Code != http.StatusCreated {
		t.Fatalf("load protocol: %d %v", rec.Code, out)
	}
	rec, out = post(t, mux, "/api/cult/session/"+id+"/protocols/"+fmt.Sprintf("%s", "越界诀")+"/start", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("start 越界诀: %d %v", rec.Code, out)
	}
	v = tick(t, mux, id, 2, nil)
	if len(v.Requests) == 0 {
		t.Fatalf("no request card for out-of-boundary: %+v", v.Requests)
	}
	card := v.Requests[0]
	if card.Kind != "out_of_boundary" {
		t.Fatalf("unexpected request kind: %s", card.Kind)
	}
	if strings.ContainsAny(card.Text, "0123456789") {
		t.Fatalf("request card leaks numbers: %q", card.Text)
	}

	// "止"决请示：请示清空，功法不再活跃。
	v = tick(t, mux, id, 1, []map[string]any{{"kind": "stop"}})
	if len(v.Requests) != 0 {
		t.Fatalf("request not resolved by stop: %+v", v.Requests)
	}

	// 功法卡含路径/知识边界/自运行开关位。
	rec2 := httptest.NewRequest(http.MethodGet, "/api/cult/session/"+id, nil)
	rec3 := httptest.NewRecorder()
	mux.ServeHTTP(rec3, rec2)
	var raw struct {
		Protocols []ProtoView `json:"protocols"`
	}
	_ = json.Unmarshal(rec3.Body.Bytes(), &raw)
	var foundPath bool
	for _, p := range raw.Protocols {
		if p.Name == "吐纳初诀" && strings.Contains(p.Path, "青关") && strings.Contains(p.Path, "下丹田") {
			foundPath = true
		}
	}
	if !foundPath {
		t.Fatalf("protocol path missing: %+v", raw.Protocols)
	}
}

// TestCultAPIGuidance 传音入流并注入 llm 载荷（无 llm 时只标注不报错）。
func TestCultAPIGuidance(t *testing.T) {
	reg := NewCultRegistry()
	mux := newTestMux(reg)
	id, _ := createSession(t, mux, "script", "")
	rec, out := post(t, mux, "/api/cult/session/"+id+"/guidance",
		map[string]any{"kind": "guide", "text": "金门似有滞涩，先查明再动"})
	if rec.Code != http.StatusOK {
		t.Fatalf("guidance: %d %v", rec.Code, out)
	}
	var v SessionView
	data, _ := json.Marshal(out)
	_ = json.Unmarshal(data, &v)
	found := false
	for _, l := range v.SenseStream {
		if strings.Contains(l.Text, "【本尊·引导】") && strings.Contains(l.Text, "金门") {
			found = true
		}
	}
	if !found {
		t.Fatal("guidance not in sense stream")
	}
	// 空文本被拒。
	rec, _ = post(t, mux, "/api/cult/session/"+id+"/guidance", map[string]any{"text": "  "})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty guidance not rejected: %d", rec.Code)
	}
}

// TestCultAPIHypothesis 玩家登记假说 → 假说表出现（凭据分档词）。
func TestCultAPIHypothesis(t *testing.T) {
	reg := NewCultRegistry()
	mux := newTestMux(reg)
	id, _ := createSession(t, mux, "script", "")
	v := tick(t, mux, id, 1, []map[string]any{
		{"kind": "hypothesis", "text": "青关或偏亲木性"},
	})
	if len(v.Hypotheses) != 1 || v.Hypotheses[0].Text != "青关或偏亲木性" {
		t.Fatalf("hypothesis missing: %+v", v.Hypotheses)
	}
	if v.Hypotheses[0].Evidence != "尚无凭据" {
		t.Fatalf("evidence word: %q", v.Hypotheses[0].Evidence)
	}
	if v.Hypotheses[0].Status != HypUnverified {
		t.Fatalf("hypothesis status: %s", v.Hypotheses[0].Status)
	}
}

// TestCultAPIIsolation 非调试视图全树零真值字段：除记账数字（tick/band/
// 周天数）外不含五量与 qi 数值键。
func TestCultAPIIsolation(t *testing.T) {
	reg := NewCultRegistry()
	mux := newTestMux(reg)
	id, _ := createSession(t, mux, "script", "")
	tick(t, mux, id, 10, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/cult/session/"+id, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, banned := range []string{
		`"metrics"`, `"flow_scale"`, `"debug"`, `"capacity"`, `"throughput"`, `"intake"`,
	} {
		if strings.Contains(body, banned) {
			t.Fatalf("view leaks truth key %s", banned)
		}
	}
}

// TestCultAPIDebugGate debug 视图之门（F2）：默认环境（AGENTBATTLE_DEV
// 未设）请求 ?debug=1 必须静默忽略——无真值字段、也不报错；设 env 后
// ?debug=1 才携带真值载荷。
func TestCultAPIDebugGate(t *testing.T) {
	reg := NewCultRegistry()
	mux := newTestMux(reg)

	t.Run("default_env_silent", func(t *testing.T) {
		t.Setenv("AGENTBATTLE_DEV", "")
		id, _ := createSession(t, mux, "script", "")
		req := httptest.NewRequest(http.MethodGet, "/api/cult/session/"+id+"?debug=1", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("debug 查询应静默等同普通视图，got HTTP %d", rec.Code)
		}
		var raw map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
			t.Fatalf("decode view: %v", err)
		}
		if _, ok := raw["debug"]; ok {
			t.Fatal("默认环境下 ?debug=1 泄漏了 debug 载荷")
		}
		for _, banned := range []string{`"metrics"`, `"flow_scale"`} {
			if strings.Contains(rec.Body.String(), banned) {
				t.Fatalf("默认环境 debug 泄漏真值键 %s", banned)
			}
		}
	})

	t.Run("dev_env_enabled", func(t *testing.T) {
		t.Setenv("AGENTBATTLE_DEV", "1")
		id, _ := createSession(t, mux, "script", "")
		req := httptest.NewRequest(http.MethodGet, "/api/cult/session/"+id+"?debug=1", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		var raw map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
			t.Fatalf("decode debug view: %v", err)
		}
		if _, ok := raw["debug"]; !ok {
			t.Fatal("AGENTBATTLE_DEV=1 时 ?debug=1 应携带 debug 载荷")
		}
	})
}

// TestCultAPISessionMode 会话模式拆分（F3）：
//   - 缺省/tutorial/normal 三种取值的功法库行为；
//   - normal 的探索型 script 分身不预设答案：按回执驱动，感知流滚动、
//     纳气与试引导回执出现；
//   - 未知 mode 被拒。
func TestCultAPISessionMode(t *testing.T) {
	reg := NewCultRegistry()
	mux := newTestMux(reg)

	// 缺省（不给 mode）= normal：功法库为空。
	_, v0 := createSession(t, mux, "script", "")
	if len(v0.Protocols) != 0 {
		t.Fatalf("normal 会话不应预装功法: %+v", v0.Protocols)
	}

	// tutorial：预载《吐纳初诀》。
	_, vt := createSession(t, mux, "script", "tutorial")
	if len(vt.Protocols) != 1 || vt.Protocols[0].Name != "吐纳初诀" {
		t.Fatalf("tutorial 会话缺预装功法: %+v", vt.Protocols)
	}

	// 未知 mode 被拒。
	rec, out := post(t, mux, "/api/cult/session", map[string]any{"seed": 7, "pilot": "script", "mode": "sandbox"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("未知 mode 应 400: %d %v", rec.Code, out)
	}

	// normal 探索型分身：推进几十 tick，感知流滚动，回执含纳气落地与
	// 试引导（允许失败回执）。
	id, _ := createSession(t, mux, "script", "")
	v := tick(t, mux, id, 40, nil)
	if v.Tick != 40 {
		t.Fatalf("tick not advanced: %d", v.Tick)
	}
	if len(v.SenseStream) < 20 {
		t.Fatalf("normal sense stream too thin: %d lines", len(v.SenseStream))
	}
	foundAbsorb, foundGuide := false, false
	for _, r := range v.Receipts {
		if strings.ContainsAny(r.Text, "0123456789") {
			t.Fatalf("receipt leaks numbers: %q", r.Text)
		}
		if strings.Contains(r.Text, "纳气") {
			foundAbsorb = true
		}
		if strings.Contains(r.Text, "引气") || strings.Contains(r.Text, "引") {
			foundGuide = true
		}
	}
	if !foundAbsorb {
		t.Fatalf("探索型分身未试纳气: %+v", v.Receipts)
	}
	if !foundGuide {
		t.Fatalf("探索型分身未试引导: %+v", v.Receipts)
	}
}
