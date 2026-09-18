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
