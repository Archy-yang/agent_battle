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
}

// LLMFromEnv builds a config from AGENTBATTLE_LLM_* variables, or nil
// when unconfigured (the scripted pilot serves instead).
func LLMFromEnv() *LLMConfig {
	base := os.Getenv("AGENTBATTLE_LLM_BASE_URL")
	if base == "" {
		return nil
	}
	return &LLMConfig{
		BaseURL: strings.TrimRight(base, "/"),
		APIKey:  os.Getenv("AGENTBATTLE_LLM_API_KEY"),
		Model:   os.Getenv("AGENTBATTLE_LLM_MODEL"),
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
}

// NewLLM creates an LLM-backed decision maker.
func NewLLM(cfg LLMConfig) *LLMAgent {
	return &LLMAgent{cfg: cfg, http: &http.Client{Timeout: 60 * time.Second}}
}

// SetGuidance refreshes the player-intervention context.
func (a *LLMAgent) SetGuidance(notes []GuidanceNote) { a.guidance = notes }

const systemPrompt = `你是《agent_battle》世界中玩家塑造的自主 Agent，操控一具 agent 身体。
目标：进入中央火阳过盛的危险区，取得元气结晶（Fragment），返回出生点 {x:1,y:10}。30 回合上限；形（integrity）≤0 即失败。

世界法则摘要：
- 五行相生：木→火→土→金→水→木；相克：木克土、土克水、水克火、火克金、金克木。
- 你的躯壳属木（怕金克）。危险区每回合额外耗气 2、生热 3。
- 阳性输出 power x（0..1000）：耗气 x/100，生热 x²/100000。热>25 每回合损形 2；被动散热 2/回合。
- 没有攻击指令：激活离火组件把火作用于目标。r = power/目标其质：<0.25 反噬自伤，0.25-0.5 无效，0.5-2 正常克制，≥2 失控放大并溢出。
- 一回合最多 2 个意图。可用意图（type）：
  move      {"type":"move","to":{"x":3,"y":6},"power":400}
  activate  {"type":"activate","component":<感识组件id>}            （感识开关，开启后感知半径 4）
  activate  {"type":"activate","component":<离火id>,"target":<实体id>,"power":600}
  transfer  {"type":"transfer","target":<结晶id>}                  （须与结晶同格）
  inspect   {"type":"inspect","target":<实体id>}
  assemble  {"type":"assemble","blueprint":[{"type":0,"essence":3},...]}（type: 0躯壳 1储气 2灵机 3感识 4识海 5离火 6仓廪 7清源；essence: 0木 1火 2土 3金 4水）

只输出 JSON，不要任何其他文字：
{"intents":[{"type":"move","to":{"x":1,"y":9}}]}`

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
	body := map[string]any{
		"model": a.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": string(raw)},
		},
		"temperature": 0.2,
	}
	buf, _ := json.Marshal(body)
	req, err := http.NewRequest("POST", a.cfg.BaseURL+"/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return nil
	}
	req.Header.Set("Content-Type", "application/json")
	if a.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.cfg.APIKey)
	}
	resp, err := a.http.Do(req)
	if err != nil {
		log.Printf("[llm] request failed: %v", err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[llm] status %d", resp.StatusCode)
		return nil
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || len(out.Choices) == 0 {
		log.Printf("[llm] bad response: %v", err)
		return nil
	}
	content := out.Choices[0].Message.Content
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	var parsed struct {
		Intents []engine.Intent `json:"intents"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		log.Printf("[llm] unparseable intents: %v: %.200s", err, content)
		return nil
	}
	return parsed.Intents
}

// Describe names the pilot for logs.
func (a *LLMAgent) Describe() string {
	return fmt.Sprintf("llm:%s", a.cfg.Model)
}
