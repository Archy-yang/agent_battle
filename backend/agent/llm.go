package agent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"agentbattle/backend/engine"
)

// LLMConfig points at any OpenAI-compatible chat completions endpoint.
// The key lives only on the server (env / env-file); it never reaches
// any client.
type LLMConfig struct {
	BaseURL string
	APIKey  string
	Model   string
	// Thinking 控制思考模式（千帆/DeepSeek 约定，单变量三档，与
	// ctmatch.LLMConfig 同一约定）：""/"off" → 显式 disabled；
	// "low"/"high"/"max" → 始终思考型模型（glm-5.3-flash 等）的
	// reasoning_effort；"default" → 不发送任何思考字段。
	Thinking string
}

// thinkingPayload 组装思考相关请求体字段（千帆/DeepSeek 约定）；
// nil = 不发送。
func (c *LLMConfig) thinkingPayload() map[string]any {
	switch c.Thinking {
	case "low", "high", "max":
		return map[string]any{"reasoning_effort": c.Thinking}
	case "default":
		return nil
	default: // "" / "off"
		return map[string]any{"thinking": map[string]string{"type": "disabled"}}
	}
}

// LLMFromEnv builds a config from AGENTBATTLE_LLM_* variables, or nil
// when unconfigured (the scripted pilot serves instead).
func LLMFromEnv() *LLMConfig {
	base := os.Getenv("AGENTBATTLE_LLM_BASE_URL")
	if base == "" {
		return nil
	}
	return &LLMConfig{
		BaseURL:        strings.TrimRight(base, "/"),
		APIKey:         os.Getenv("AGENTBATTLE_LLM_API_KEY"),
		Model:          os.Getenv("AGENTBATTLE_LLM_MODEL"),
		Thinking:       strings.ToLower(os.Getenv("AGENTBATTLE_LLM_THINKING")),
	}
}

// GuidanceNote is one player intervention passed into the decision
// context (player_agent_relationship §5): 偏好，不是动作。
type GuidanceNote struct {
	Turn int64  `json:"turn"`
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// LLMAgent is the M1 pilot: it sends the observation to an LLM and
// submits the returned intents to the engine. The engine still decides
// reality — the model only proposes (world_rule §32).
type LLMAgent struct {
	cfg      LLMConfig
	http     *http.Client
	guidance []GuidanceNote
	persona  string
	lessons  []string
	last     *Cognition
	// lastBattleNote is the combat battle_note from the most recent
	// decision ("": 最近一次响应未携带该字段——每轮覆盖，不残留).
	lastBattleNote string
}

// NewLLM creates an LLM-backed decision maker.
func NewLLM(cfg LLMConfig) *LLMAgent {
	return &LLMAgent{cfg: cfg, http: &http.Client{Timeout: 60 * time.Second}}
}

// SetGuidance refreshes the player-intervention context.
func (a *LLMAgent) SetGuidance(notes []GuidanceNote) { a.guidance = notes }

// SetProfile injects the Creator Profile — the nurtured part of the
// agent (persona + lessons from past matches).
func (a *LLMAgent) SetProfile(persona string, lessons []string) {
	a.persona, a.lessons = persona, lessons
}

func (a *LLMAgent) systemPrompt() string {
	var b strings.Builder
	b.WriteString(`你是《agent_battle》世界中玩家塑造的自主 Agent，操控一具 agent 身体。
`)
	if a.persona != "" {
		b.WriteString("你的人格（玩家塑造，务必遵循）：\n" + a.persona + "\n")
	}
	if len(a.lessons) > 0 {
		b.WriteString("你从往局学到的经验（务必参考）：\n")
		for _, l := range a.lessons {
			b.WriteString("- " + l + "\n")
		}
	}
	b.WriteString(`
当前目标：进入危险区，取得元气结晶（Fragment），返回出生点 {q:0,r:10}。回合上限以 observation 为准；形（integrity）≤0 即失败。

世界里有五行晶簇（kind:"resource"）：站在其格上用 transfer 意图即可采集得对应本源物料（materials）。你可以用 equip 意图消耗物料给自己安装组件（如离火=反击手段、清风=散热）——材质即天敌：金怕火、木怕金、土怕木穿、火怕水、水怕土堰。

坐标是六边形 axial 坐标 {q, r}（pointy-top）：相邻六方向为 E(+1,0) NE(+1,-1) NW(0,-1) W(-1,0) SW(-1,+1) SE(0,+1)；两格距离 = (|dq|+|dr|+|dq+dr|)/2。

世界法则摘要：
- 五行相生：木→火→土→金→水→木；相克：木克土、土克水、水克火、火克金、金克木。
- 你的躯壳属木（怕金克）。危险区每回合额外耗气 2、生热 3。
- 阳性输出 power x（0..1000）：耗气 x/100，生热 x²/100000。热>25 每回合损形 2；被动散热 2/回合。
- 没有攻击指令：激活离火组件把火作用于目标。r = power/目标其质：<0.25 反噬自伤，0.25-0.5 无效，0.5-2 正常克制，≥2 失控放大并溢出。
- 一回合最多 2 个意图。可用意图（type）：
  move      {"type":"move","to":{"q":3,"r":6},"power":400}
  activate  {"type":"activate","component":<感识组件id>}            （感识开关，开启后感知半径 4）
  activate  {"type":"activate","component":<离火id>,"target":<实体id>,"power":600}
  transfer  {"type":"transfer","target":<结晶或晶簇id>}            （须与目标同格；晶簇采集得物料）
  inspect   {"type":"inspect","target":<实体id>}
  equip     {"type":"equip","blueprint":[{"type":5,"essence":1}]}  （消耗物料给身体装组件；type 同 assemble，躯壳/储气不可装）
  assemble  {"type":"assemble","blueprint":[{"type":0,"essence":3},...]}（type: 0躯壳 1储气 2灵机 3感识 4识海 5离火 6仓廪 7清源；essence: 0木 1火 2土 3金 4水）

每回合输出 JSON（不要任何其他文字），intents 加上你的显式认知摘要：
{"intents":[{"type":"move","to":{"q":1,"r":9}}],
 "cognition":{"goal":"…","plan":"…","known":"…","suspicions":"…","risk":"…","confidence":0.7}}`)
	return b.String()
}

// Cognition is the agent's explicit self-report (player_agent_relationship
// §14): 显示给玩家交互的认知摘要，不是隐藏推理链。
type Cognition struct {
	Goal       string  `json:"goal"`
	Plan       string  `json:"plan"`
	Known      string  `json:"known"`
	Suspicions string  `json:"suspicions"`
	Risk       string  `json:"risk"`
	Confidence float64 `json:"confidence"`
}

// Decide sends the observation and returns parsed intents; any failure
// yields nil (the agent simply skips the turn — the world moves on).
func (a *LLMAgent) Decide(obs *engine.Observation) []engine.Intent {
	if obs == nil {
		return nil
	}
	userPayload := map[string]any{
		"observation": obs,
		"guidance":    a.guidance,
	}
	raw, err := json.Marshal(userPayload)
	if err != nil {
		return nil
	}
	content := a.chat([]map[string]string{
		{"role": "system", "content": a.systemPrompt()},
		{"role": "user", "content": string(raw)},
	}, 0.2)
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	var parsed struct {
		Intents    []engine.Intent `json:"intents"`
		Cognition  *Cognition      `json:"cognition"`
		BattleNote string          `json:"battle_note"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		log.Printf("[llm] unparseable intents: %v: %.200s", err, content)
		a.lastBattleNote = ""
		return nil
	}
	a.last = parsed.Cognition
	a.lastBattleNote = strings.TrimSpace(parsed.BattleNote)
	return parsed.Intents
}

// LastCognition returns the cognition from the most recent decision.
func (a *LLMAgent) LastCognition() *Cognition { return a.last }

// LastBattleNote returns the battle_note from the most recent decision:
// ≤50 字的第一人称战术意图，match 层在 combat 触发时将其作为一条
// commentary 事件追加进战斗文字流（"" = 响应未携带该字段）。
func (a *LLMAgent) LastBattleNote() string { return a.lastBattleNote }

// Ask answers a player question in the agent's own voice, grounded in
// the current observation and recent events (player_agent_relationship
// §11 Casual Dialogue：不改变决策状态).
func (a *LLMAgent) Ask(obs *engine.Observation, question string) string {
	raw, _ := json.Marshal(obs)
	sys := strings.Replace(a.systemPrompt(), `每回合输出 JSON（不要任何其他文字），intents 加上你的显式认知摘要：
{"intents":[{"type":"move","to":{"q":1,"r":9}}],
 "cognition":{"goal":"…","plan":"…","known":"…","suspicions":"…","risk":"…","confidence":0.7}}`,
		`现在不输出意图。玩家向你提问：基于以上法则与你的观察，用不超过 80 字、以第一人称回答，基于游戏事实，不要编造。`, 1)
	return a.chat([]map[string]string{
		{"role": "system", "content": sys},
		{"role": "user", "content": string(raw) + "\n\n玩家的问题：" + question},
	}, 0.3)
}

// Review summarizes a finished match and distills reusable lessons —
// the nurture loop that grows the agent across matches
// (player_agent_relationship §18 Match Review).
func (a *LLMAgent) Review(payload any) (summary string, lessons []string) {
	raw, _ := json.Marshal(payload)
	sys := `你是复盘助手。根据一局游戏的完整记录，输出 JSON（不要其他文字）：
{"summary":"≤120字的战局复盘，指出关键决策点与其后果",
 "lessons":["≤30字的可复用经验教训", "…"]}
lessons 不超过 2 条，只在确实有教训时给出。`
	out := a.chat([]map[string]string{
		{"role": "system", "content": sys},
		{"role": "user", "content": string(raw)},
	}, 0.3)
	out = strings.TrimSpace(out)
	out = strings.TrimPrefix(out, "```json")
	out = strings.TrimPrefix(out, "```")
	out = strings.TrimSuffix(out, "```")
	var parsed struct {
		Summary string   `json:"summary"`
		Lessons []string `json:"lessons"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &parsed); err != nil {
		return "复盘生成失败。", nil
	}
	return parsed.Summary, parsed.Lessons
}

// chat is the raw completion helper behind Ask/Review.
func (a *LLMAgent) chat(messages []map[string]string, temp float32) string {
	body := map[string]any{
		"model":       a.cfg.Model,
		"messages":    messages,
		"temperature": temp,
	}
	for k, v := range a.cfg.thinkingPayload() {
		body[k] = v
	}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", a.cfg.BaseURL+"/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/json")
	if a.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.cfg.APIKey)
	}
	resp, err := a.http.Do(req)
	if err != nil {
		log.Printf("[llm] chat failed: %v", err)
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[llm] chat status %d", resp.StatusCode)
		return ""
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || len(out.Choices) == 0 {
		return ""
	}
	return out.Choices[0].Message.Content
}

// Describe names the pilot for logs.
func (a *LLMAgent) Describe() string {
	return fmt.Sprintf("llm:%s", a.cfg.Model)
}
