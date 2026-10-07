package ctmatch

// llm_test.go —— LLMPilot 的合同测试（假端点，不触网）：
//
//	1. 未配置环境变量 → NewLLMPilot 返回 nil（回落桩）
//	2. 感知隔离律：system prompt 全文无数字、无规则表述（铁律 1 的锁）
//	3. 观察 JSON 进入用户载荷；cognition 记为修行笔记
//	4. 意图解析：absorb/guide/stop/inspect/sense、元素与档位宽容映射
//	5. 预算耗尽与垃圾响应的安全回落

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agentbattle/backend/cult"
)

// newFakeLLM 起一个 OpenAI 兼容假端点，回固定 content，并把收到的请求体
// 交给 inspect 回调。
func newFakeLLM(t *testing.T, content string, inspect func(req map[string]any)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("请求体不是 JSON: %v", err)
		} else if inspect != nil {
			inspect(body)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": content}},
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func fakePilot(t *testing.T, srv *httptest.Server) *LLMPilot {
	t.Helper()
	p := newLLMPilotWithConfig(&LLMConfig{BaseURL: srv.URL, APIKey: "test-key", Model: "fake-model"})
	p.MaxCalls = 5
	return p
}

func fakeObservation() *Observation {
	return &Observation{
		Tick: 7,
		Env:  EnvSense{Qi: []string{"木气流转", "火气微茫", "土气微茫", "金气微茫", "水气流转"}, Activity: []string{"木性沛然", "火性沛然", "土性沛然", "金性沛然", "水性沛然"}},
		Points: []AcupointSense{
			{Name: "青关", Feelings: []string{"有些充盈", "温热"}, Flow: "进入", Nature: nil,
				Openness: "大开", Stability: "稳固", Damage: "无损", PressureBand: 2},
		},
	}
}

func TestNewLLMPilotUnconfigured(t *testing.T) {
	t.Setenv("AGENTBATTLE_LLM_BASE_URL", "")
	t.Setenv("AGENTBATTLE_LLM_API_KEY", "")
	t.Setenv("AGENTBATTLE_LLM_MODEL", "")
	if NewLLMPilot() != nil {
		t.Fatal("未配置时 NewLLMPilot 应返回 nil 让上层回落桩")
	}
}

// TestLLMPromptIsolation 是感知隔离律（M1 铁律 1）的执行测试：
// system prompt 全文不得出现任何 ASCII 数字（公式/参数值/阈值数字
// 必然带数字），也不得出现"某量≥x"式规则表述的代词。
func TestLLMPromptIsolation(t *testing.T) {
	var system string
	srv := newFakeLLM(t, `{"cognition":"初见","intents":[]}`, func(req map[string]any) {
		msgs := req["messages"].([]any)
		system = msgs[0].(map[string]any)["content"].(string)
	})
	p := fakePilot(t, srv)
	p.Decide(fakeObservation())

	if system == "" {
		t.Fatal("未捕获到 system prompt")
	}
	for _, r := range system {
		if r >= '0' && r <= '9' {
			t.Fatalf("system prompt 含数字 %q——铁律 1：不得出现参数值/阈值/公式片段", r)
		}
	}
	low := strings.ToLower(system)
	for _, banned := range []string{"loadratio", "load_ratio", "capacity", "throughput", "retention", "intensity", "0.", ">=", "≥"} {
		if strings.Contains(low, banned) {
			t.Fatalf("system prompt 含疑似规则表述 %q", banned)
		}
	}
	// 手册五意图与四档位必须在。
	for _, want := range []string{"sense", "absorb", "guide", "stop", "inspect", "trace", "small", "medium", "large", "木", "火", "土", "金", "水"} {
		if !strings.Contains(low, want) {
			t.Fatalf("system prompt 缺意图手册要素 %q", want)
		}
	}
}

func TestLLMDecideParsesIntents(t *testing.T) {
	content := "好的，我的打算：\n```json\n" +
		`{"cognition":"木气最盛，先以青关少量吸纳。","intents":[` +
		`{"kind":"absorb","target":"青关","level":"small"},` +
		`{"kind":"guide","from":"青关","to":"下丹田","element":"wood","level":"medium"},` +
		`{"kind":"guide","from":"青关","to":"中庭","level":"large"},` +
		`{"kind":"stop"},{"kind":"inspect","target":"青关"},{"kind":"sense"},` +
		`{"kind":"absorb","target":"神庭","level":"huge"}]}` +
		"\n```"
	srv := newFakeLLM(t, content, nil)
	p := fakePilot(t, srv)

	intents := p.Decide(fakeObservation())
	if p.LastError != "" {
		t.Fatalf("不应报错: %s", p.LastError)
	}
	if len(intents) != 7 {
		t.Fatalf("应解析出 7 条意图, got %d", len(intents))
	}
	if intents[0].Kind != IntentAbsorb || intents[0].Acupoint != "青关" || intents[0].Level != LevelSmall {
		t.Errorf("absorb 解析错误: %+v", intents[0])
	}
	if intents[1].Kind != IntentGuide || intents[1].Element != cult.Wood || intents[1].Level != LevelMedium {
		t.Errorf("guide(wood) 解析错误: %+v", intents[1])
	}
	if intents[2].Element != -1 || intents[2].Level != LevelLarge {
		t.Errorf("guide(混合) 解析错误: %+v", intents[2])
	}
	if intents[3].Kind != IntentStop || intents[4].Kind != IntentInspect || intents[5].Kind != IntentSense {
		t.Errorf("stop/inspect/sense 解析错误: %+v %+v %+v", intents[3], intents[4], intents[5])
	}
	if intents[6].Level != LevelSmall {
		t.Errorf("不认识的档位应回落 small: %+v", intents[6])
	}
	if p.LastCognition == "" || len(p.Notes) != 1 {
		t.Errorf("cognition 应记为修行笔记: %q %v", p.LastCognition, p.Notes)
	}

	// 笔记应回喂到下一次调用的用户载荷。
	var user string
	srv2 := newFakeLLM(t, `{"cognition":"","intents":[]}`, func(req map[string]any) {
		msgs := req["messages"].([]any)
		user = msgs[1].(map[string]any)["content"].(string)
	})
	p2 := fakePilot(t, srv2)
	p2.Notes = []string{"木气最盛，先以青关少量吸纳。"}
	p2.Decide(fakeObservation())
	if !strings.Contains(user, "木气最盛") {
		t.Error("修行笔记未回喂到用户载荷")
	}
	if !strings.Contains(user, "青关") || !strings.Contains(user, "有些充盈") {
		t.Error("观察定性词未进入用户载荷")
	}
}

func TestLLMBudgetAndGarbage(t *testing.T) {
	srv := newFakeLLM(t, "这不是 JSON。", nil)
	p := fakePilot(t, srv)
	p.MaxCalls = 2

	if got := p.Decide(fakeObservation()); got != nil {
		t.Error("垃圾响应应安全回落 nil")
	}
	if p.LastError == "" {
		t.Error("垃圾响应应记录错误")
	}
	if got := p.Decide(fakeObservation()); got != nil {
		t.Error("预算耗尽应回落 nil")
	}
	if p.Calls != 2 {
		t.Fatalf("第二次调用应已发起（失败也计预算）: calls=%d", p.Calls)
	}
	if got := p.Decide(fakeObservation()); got != nil {
		t.Error("预算耗尽应回落 nil")
	}
	if !strings.Contains(p.LastError, "预算") {
		t.Errorf("预算耗尽应明示: %q", p.LastError)
	}
	if p.BudgetLeft() != 0 || p.Calls != 2 {
		t.Errorf("失败调用也应计入预算: calls=%d left=%d", p.Calls, p.BudgetLeft())
	}
}

// ---------------------------------------------------------------------------
// 功法草案（M2 WP-M2-2）：提示词表象、意图解析、定性回执回流
// ---------------------------------------------------------------------------

// TestLLMPromptDraftSurface 锁 R3 重排后的草案表象：分身须知"协议草案
// 怎么写"的可探索格架（键名/规矩），但不泄公式与参数——全文零数字，
// 且不出现规则词表（复用 TestLLMPromptIsolation 的禁词口径）。
func TestLLMPromptDraftSurface(t *testing.T) {
	var system string
	srv := newFakeLLM(t, `{"cognition":"","intents":[]}`, func(req map[string]any) {
		msgs := req["messages"].([]any)
		system = msgs[0].(map[string]any)["content"].(string)
	})
	p := fakePilot(t, srv)
	p.Decide(fakeObservation())

	if system == "" {
		t.Fatal("未捕获到 system prompt")
	}
	for _, r := range system {
		if r >= '0' && r <= '9' {
			t.Fatalf("system prompt 含数字 %q——铁律 1", r)
		}
	}
	low := strings.ToLower(system)
	for _, banned := range []string{"loadratio", "load_ratio", "capacity", "throughput", "retention", "intensity", "0.", ">=", "≥"} {
		if strings.Contains(low, banned) {
			t.Fatalf("system prompt 含疑似规则表述 %q", banned)
		}
	}
	// 草案表象要素：两个新意图 + 草案格架的关键键名。
	for _, want := range []string{"give_protocol_draft", "start_protocol", "protocol:", "actions:", "validated_range:", "max_level", "cadence:", "cycles:", "parallel:", "固化在册"} {
		if !strings.Contains(system, want) {
			t.Fatalf("system prompt 缺草案表象要素 %q", want)
		}
	}
}

// TestLLMParseDraftIntents：give_protocol_draft / start_protocol 的解析。
func TestLLMParseDraftIntents(t *testing.T) {
	content := "按心得行事：\n```json\n" +
		`{"cognition":"把摸到的路写成草案。","intents":[` +
		`{"kind":"give_protocol_draft","text":"protocol: 周天初环\nactions:\n  - absorb: {target: 青关, level: trace}"},` +
		`{"kind":"start_protocol","name":"周天初环"},` +
		`{"kind":"start_protocol","text":"备选名"}]}` +
		"\n```"
	srv := newFakeLLM(t, content, nil)
	p := fakePilot(t, srv)

	intents := p.Decide(fakeObservation())
	if p.LastError != "" {
		t.Fatalf("不应报错: %s", p.LastError)
	}
	if len(intents) != 3 {
		t.Fatalf("应解析出 3 条意图, got %d", len(intents))
	}
	if intents[0].Kind != IntentProtocolDraft || !strings.Contains(intents[0].Text, "周天初环") {
		t.Errorf("草案意图解析错误: %+v", intents[0])
	}
	if intents[1].Kind != IntentStartProtocol || intents[1].Text != "周天初环" {
		t.Errorf("起功意图解析错误: %+v", intents[1])
	}
	if intents[2].Kind != IntentStartProtocol || intents[2].Text != "备选名" {
		t.Errorf("起功意图应回落 text 字段: %+v", intents[2])
	}
}

// TestLLMDraftReceiptFeedsBack：会话 + 假端点全链——LLM 分身提交含错
// 草案，会话编译拒绝并给出定性回执；下一息回执照常回流到征询载荷
// （零数字、零机器词，可指导修订）。
func TestLLMDraftReceiptFeedsBack(t *testing.T) {
	var user string
	call := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("请求体不是 JSON: %v", err)
		}
		msgs := body["messages"].([]any)
		if n := len(msgs); n >= 2 {
			user = msgs[1].(map[string]any)["content"].(string)
		}
		call++
		content := `{"cognition":"试着写一门功法","intents":[{"kind":"give_protocol_draft","text":"protocol: 冒进诀\nactions:\n  - absorb: {target: 玄府, level: small}"}]}`
		if call > 1 {
			content = `{"cognition":"按回执再修","intents":[]}`
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": content}}},
		})
	}))
	t.Cleanup(srv.Close)

	w := draftWorld()
	p := newLLMPilotWithConfig(&LLMConfig{BaseURL: srv.URL, APIKey: "test-key", Model: "fake-model"})
	p.MaxCalls = 4
	s := NewSession(w, 127, p)
	s.Observe = func(x *cult.World) *Observation { return cult.Observe(x, 1) }

	// 息一：唤醒分身 → 分身提交含错草案 → 定性拒绝回执。
	s.WakePilotNextTick()
	Tick(s, nil)
	if _, ok := s.Protocols["冒进诀"]; ok {
		t.Fatal("被拒草案不应入册")
	}
	// 息二：唤醒 → 回执回流到征询载荷。
	s.WakePilotNextTick()
	Tick(s, nil)

	if !strings.Contains(user, "尚未辨识") || !strings.Contains(user, "玄府") {
		t.Errorf("定性回执应回流到征询载荷并指明错处：\n%s", user)
	}
	seg := user[strings.Index(user, "【你上一组意图的回执】"):]
	if i := strings.Index(seg, "【你的证据账本】"); i >= 0 {
		seg = seg[:i]
	}
	if bad := receiptDigits([]string{seg}); bad != "" {
		t.Errorf("回执载荷含数字 %q", bad)
	}
	for _, r := range seg {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			t.Errorf("回执载荷含 ASCII（机器词）%q", string(r))
		}
	}
}

// 思考模式配置（2026-09-19，单变量三档）：off=显式关闭（DeepSeek 系）；
// low/high/max=始终思考型模型（glm-5.3-flash 等）的程度参数；default=不发送。
func TestLLMThinkingPayload(t *testing.T) {
	off := &LLMConfig{}
	body := off.thinkingPayload()
	if body["thinking"].(map[string]string)["type"] != "disabled" {
		t.Errorf("默认应显式关闭思考，got %v", body)
	}
	if _, ok := body["reasoning_effort"]; ok {
		t.Errorf("关闭时不应带 reasoning_effort")
	}
	eff := (&LLMConfig{Thinking: "low"}).thinkingPayload()
	if eff["reasoning_effort"] != "low" {
		t.Errorf("low 应映射 reasoning_effort，got %v", eff)
	}
	if d := (&LLMConfig{Thinking: "default"}).thinkingPayload(); d != nil {
		t.Errorf("default 应不发送任何字段，got %v", d)
	}
	// 环境变量注入路径
	t.Setenv("AGENTBATTLE_LLM_BASE_URL", "http://x/v1")
	t.Setenv("AGENTBATTLE_LLM_API_KEY", "k")
	t.Setenv("AGENTBATTLE_LLM_MODEL", "m")
	t.Setenv("AGENTBATTLE_LLM_THINKING", "LOW") // 大小写不敏感
	cfg := LoadLLMConfig()
	if cfg.Thinking != "low" {
		t.Errorf("THINKING 应归一化为小写，got %q", cfg.Thinking)
	}
}
