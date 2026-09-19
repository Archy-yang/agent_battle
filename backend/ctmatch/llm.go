package ctmatch

// llm.go —— 分身 LLM Pilot（M1 SPIKE；stage_2 §26-27、§33-37）。
//
// 感知隔离律（M1 计划 §0 铁律 1）在本文件的落点：
//   - system prompt 只含三件事：处境、意图手册（五意图 + 粗档位语义）、
//     输出 JSON 格式。绝无公式、参数数值、阈值数字、任何"某量 ≥x 就如何"
//     的规则表述（llm_test.go 用"system prompt 全文无数字"测试锁死）。
//   - 观察载荷只来自 cult.Observe 的定性 JSON（acuity 由上层注入）。
//   - LLM 输出的 cognition 原文记为修行笔记（Agent Knowledge，非 World
//     Truth），后续唤醒时原样回喂——它只知道自己的心得，从不接触真值。
//
// 本文件自带最小 OpenAI 兼容 chat/completions 客户端（仅 stdlib），
// 读 AGENTBATTLE_LLM_BASE_URL / _API_KEY / _MODEL 环境变量；任一缺失时
// NewLLMPilot 返回 nil，上层回落桩（ScriptedPilot）。API key 只进内存
// 与请求头，绝不写日志、绝不硬编码。

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"agentbattle/backend/cult"
)

// LLMConfig 是 OpenAI 兼容服务的连接配置。
type LLMConfig struct {
	BaseURL string // 如 https://api.example.com/v1（自动补 /chat/completions）
	APIKey  string
	Model   string
	// Thinking 控制思考模式（千帆/DeepSeek 约定，单变量三档）：
	//   "" / "off"     → 请求体显式 {"thinking":{"type":"disabled"}}
	//                    （DeepSeek 系列服务端默认 enabled，须显式关）
	//   "low"/"high"/"max" → 始终思考型模型（如 glm-5.3-flash，不支持
	//                    关闭，只接受程度参数）→ reasoning_effort=<档>
	//   "default"      → 不发送任何思考字段，走服务端默认
	Thinking string
}

// thinkingPayload 按配置组装思考相关请求体字段；nil = 不发送。
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

const (
	// LLMBudgetDefault 是 SPIKE 的默认调用预算（节省预算：升级/不适唤醒
	// + 心跳征询共用此池）。
	LLMBudgetDefault = 40

	llmMaxNotes     = 12  // 修行笔记最多保留的条数（滚动）
	llmNoteMaxRunes = 600 // 单条笔记截断长度
	llmMaxIntents   = 12  // 单次响应最多采纳的意图数
	llmTimeout      = 90 * time.Second
)

// LoadLLMConfig 从环境变量读 LLM 配置；任一项缺失返回 nil。
func LoadLLMConfig() *LLMConfig {
	cfg := &LLMConfig{
		BaseURL:        strings.TrimRight(os.Getenv("AGENTBATTLE_LLM_BASE_URL"), "/"),
		APIKey:         os.Getenv("AGENTBATTLE_LLM_API_KEY"),
		Model:          os.Getenv("AGENTBATTLE_LLM_MODEL"),
		Thinking:       strings.ToLower(os.Getenv("AGENTBATTLE_LLM_THINKING")),
	}
	if cfg.BaseURL == "" || cfg.APIKey == "" || cfg.Model == "" {
		return nil
	}
	return cfg
}

// LLMPilot 是 Pilot 接口的真 LLM 实现（L2 分身）。
// 调用预算（MaxCalls）耗尽后 Decide 恒返回 nil——上层沿用上一组意图。
type LLMPilot struct {
	cfg    *LLMConfig
	client *http.Client

	// MaxCalls 是调用预算；Calls 是已发起的调用次数（含失败——失败也
	// 耗预算，防止对故障端点无限重试）。
	MaxCalls int
	Calls    int

	// Notes 是修行笔记：历次 cognition 原文（滚动保留最近 llmMaxNotes 条）。
	// SPIKE 结论二：笔记只是附注，证据账本（knowledge.go）才是事实凭据。
	Notes []string

	// lastReceipts / lastLedger 是会话层经可选接口（ReceiptAware /
	// LedgerAware）给到的上一组意图回执与账本定性视图，随观察一并进
	// 征询载荷——全部为定性措辞、零数字（铁律 1/6）。
	lastReceipts []IntentReceipt
	lastLedger   []string

	// pendingGuidance 是本尊传音（WP-5 /guidance）：消费于下次
	// buildUserMessage，消费即清。
	pendingGuidance []string

	// LastCognition / LastError / LastRaw 是最近一次 Decide 的诊断信息
	//（供转录与测试；不含任何密钥）。
	LastCognition string
	LastError     string
	LastRaw       string
}

// NewLLMPilot 按环境变量构造 LLMPilot；未配置时返回 nil，让上层回落桩。
func NewLLMPilot() *LLMPilot {
	cfg := LoadLLMConfig()
	if cfg == nil {
		return nil
	}
	return newLLMPilotWithConfig(cfg)
}

func newLLMPilotWithConfig(cfg *LLMConfig) *LLMPilot {
	return &LLMPilot{
		cfg:      cfg,
		client:   &http.Client{Timeout: llmTimeout},
		MaxCalls: LLMBudgetDefault,
	}
}

// BudgetLeft 返回剩余调用预算。
func (p *LLMPilot) BudgetLeft() int {
	if p == nil {
		return 0
	}
	return p.MaxCalls - p.Calls
}

// llmSystemPrompt 是分身的全部"先天认知"：处境、意图手册、输出格式。
// 铁律 1：这里没有任何数字——llm_test.go 锁死。
const llmSystemPrompt = `你是一位修士的分身。本尊沉睡未醒，只留下你独自在一件静室中修行。
你完全不知道修炼的法门：没有功法，没有口诀，也没有人教过你。你只能凭身体的感受摸索。

你的心愿，是让自身气血活起来：吐故纳新、有进有出、生生不息——而不是把某一处
越胀越满，胀到撕裂。满而不泄是死水，通而不息才是修行。

你的身体由若干穴位组成，穴位之间有经脉相连，灵气可以循经脉在穴位之间流动。
天地灵气分五行：木、火、土、金、水。静室里各行的浓淡与活跃、你身体各穴位的
胀满与虚空、温热与清凉、通畅与阻滞、开合、稳固与损伤——这些感受，就是你全部的依据。
不要去猜任何数字：数字不存在于你的世界，感受就是感受。

每息（每个时机）你可以下达一组意图，天生意志决定它们的真实效果；
效果如何，你会收到回执，也能从下一息的感受里去体会。可用的意图如下：

- sense：静静感受，不做任何事。
- absorb：以某个穴位吸纳天地灵气。字段：target（穴位名）、level（力度粗档）。
- guide：把灵气从一处穴位引向另一处穴位（两者须有经脉直接相连）。
  字段：from、to（穴位名）、可选 element（木、火、土、金、水之一；省略则不挑属性）、
  level（流量粗档）。
- stop：停下一切。
- inspect：内视某个穴位。字段：target（穴位名）。
- record_hypothesis：把你此刻的一条猜想记入证据账本（如「青关或偏亲木性」）。
  字段：text（猜想原文）。

关于这组意图，还有几件事你需要知道：

- 一息之内的多条意图会同时生效——你尽可以一面在一处纳气，一面在别处引气，
  让纳气与引气在同一息里咬合成节律。
- 引气能否落地，只系于两穴之间有没有直接相连的经脉。element 是你挑引哪一行
  的气，不是开门的钥匙；穴位叫什么名字，也不代表路就通向哪里。
- 每息之后你会收到上一组意图的回执：能否落地，以及落地之后身体压力的方向
  （压力升、压力降或压力不变）。回执若说「此路不通」或「穴未开或满溢不入」，
  就换个源头、去处、力度或方式再试，不要对着同一堵墙反复推。
- 经脉的走向无人告知你——哪两口之间有路，只有真的引一次才知道。
  把探明通路当作与纳气同样要紧的功课。
- 你的证据账本会替你记住哪些路从未试过、试过被拒、试过且成。猜想请交
  record_hypothesis 登记，由账本里的证据来印证或推翻；修行笔记只是附注，
  证据才是凭据。
- 只有真正纳气、引气时，身体才会有新的感受；静观与内视不会带来新信息。
- 状态之语的次序：开度由闭到开是「闭、微开、半开、大开、洞开」；
  稳固由强到弱是「稳如磐石、稳固、微澜、不稳、将溃」；
  损伤由轻到重是「无损、隐伤、作痛、重创、脉裂」。

力度粗档只有四档：trace（极微）、small（少量）、medium（中等）、large（大量）。

每息你只能输出一个 JSON 对象，除此之外一个字都不要输出。格式如下：
{"cognition": "你此刻的心得：你对身体、灵气与法则的新认识、猜测与下一步打算",
 "intents": [{"kind": "absorb", "target": "某穴位", "level": "small"},
             {"kind": "guide", "from": "某穴位", "to": "另一穴位", "element": "木", "level": "medium"},
             {"kind": "record_hypothesis", "text": "你的猜想原文"}]}
intents 也可以是空数组。cognition 会被记入你的修行笔记（附注），下次唤醒时原样还给你。`

// Decide 实现 Pilot：把观察（定性 JSON）与修行笔记交给 LLM，
// 解析其 JSON 响应为意图。任何失败都安全回落（返回 nil），不惊动引擎。
func (p *LLMPilot) Decide(obs *Observation) []Intent {
	if p == nil || p.cfg == nil {
		return nil
	}
	if p.Calls >= p.MaxCalls {
		p.LastError = "调用预算已用尽，分身不再被唤醒"
		return nil
	}
	p.LastError, p.LastCognition, p.LastRaw = "", "", ""
	p.Calls++

	content, err := p.chat(llmSystemPrompt, p.buildUserMessage(obs))
	if err != nil {
		p.LastError = err.Error()
		return nil
	}
	p.LastRaw = truncateRunes(content, 2000)
	resp, err := parseLLMResponse(content)
	if err != nil {
		p.LastError = err.Error()
		return nil
	}
	if c := strings.TrimSpace(resp.Cognition); c != "" {
		p.LastCognition = c
		p.addNote(c)
	}
	return resp.toIntents()
}

// GiveReceipts 实现 ReceiptAware：接收会话层给到的"上一组意图的回执"。
func (p *LLMPilot) GiveReceipts(rs []IntentReceipt) {
	if p != nil {
		p.lastReceipts = rs
	}
}

// GiveLedgerView 实现 LedgerAware：接收证据账本的定性视图。
func (p *LLMPilot) GiveLedgerView(v []string) {
	if p != nil {
		p.lastLedger = v
	}
}

// GiveGuidance 记一条本尊传音（WP-5：玩家文本注入下次征询载荷的
// guidance 段；kind 只作标注——轻语/引导/深谈）。消费于下次
// buildUserMessage（即下次 Decide），消费即清。
func (p *LLMPilot) GiveGuidance(kind, text string) {
	if p != nil {
		p.pendingGuidance = append(p.pendingGuidance, kind+"："+text)
	}
}

// askSystemPrompt 是向分身提问的应答口径：与修行同源——只凭感受与
// 账本作答，定性、不超过八十字、绝无数字。
const askSystemPrompt = `你是一位修士的分身，本尊此刻唤你问话。
你只凭自己的感受（定性之词）与证据账本作答：说得出的说，说不出的直说不知。
不许猜任何数字——数字不存在于你的世界。回答不超过八十字，只输出答话正文，不要 JSON。`

// Ask 向分身提问（WP-5 /ask）：载荷 = 证据账本 + 最新定性观察 + 问题，
// 全部定性措辞、零数字（铁律 1）。未配置或调用失败返回错误，由上层降级。
func (p *LLMPilot) Ask(obs *Observation, ledger []string, question string) (string, error) {
	if p == nil || p.cfg == nil {
		return "", fmt.Errorf("LLM 未配置")
	}
	var b strings.Builder
	b.WriteString("【你的证据账本】\n")
	if len(ledger) == 0 {
		b.WriteString("（还是空的。）\n")
	}
	for _, line := range ledger {
		b.WriteString("- " + line + "\n")
	}
	b.WriteString("\n【当前的身体感受】\n")
	data, err := json.Marshal(obs)
	if err != nil {
		data = []byte("{}")
	}
	b.Write(data)
	b.WriteString("\n\n本尊问你：" + question)
	return p.chat(askSystemPrompt, b.String())
}

// buildUserMessage 组装用户消息：传音 + 回执 + 证据账本 + 修行笔记（附注）+ 定性观察。
func (p *LLMPilot) buildUserMessage(obs *Observation) string {
	var b strings.Builder
	if len(p.pendingGuidance) > 0 {
		b.WriteString("【本尊传音】\n")
		for _, g := range p.pendingGuidance {
			b.WriteString("- " + g + "\n")
		}
		b.WriteString("\n")
		p.pendingGuidance = nil
	}
	b.WriteString("【你上一组意图的回执】\n")
	if len(p.lastReceipts) == 0 {
		b.WriteString("（尚无回执——你还没有交出过意图。）\n")
	}
	for _, r := range p.lastReceipts {
		b.WriteString("- " + ReceiptView(r) + "\n")
	}
	b.WriteString("\n【你的证据账本】\n")
	if len(p.lastLedger) == 0 {
		b.WriteString("（还是空的——许多路你从未试过。）\n")
	}
	for _, line := range p.lastLedger {
		b.WriteString("- " + line + "\n")
	}
	b.WriteString("\n【你的修行笔记（附注——证据账本才是凭据）】\n")
	if len(p.Notes) == 0 {
		b.WriteString("（还是一片空白——你一无所知。）\n")
	}
	for _, n := range p.Notes {
		b.WriteString("- " + n + "\n")
	}
	b.WriteString("\n【当前的身体感受】\n")
	data, err := json.Marshal(obs)
	if err != nil {
		data = []byte("{}")
	}
	b.Write(data)
	b.WriteString("\n\n回顾回执、账本、笔记与感受，决定接下来一息的意图，并写下新的心得。只输出 JSON。")
	return b.String()
}

// chat 调 OpenAI 兼容 chat/completions，返回首条回复文本。
func (p *LLMPilot) chat(system, user string) (string, error) {
	endpoint := p.cfg.BaseURL
	if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/chat/completions"
	}
	body := map[string]any{
		"model": p.cfg.Model,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"temperature": 0.7,
	}
	for k, v := range p.cfg.thinkingPayload() {
		body[k] = v
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("HTTP %d，响应不是合法 JSON: %w", resp.StatusCode, err)
	}
	if parsed.Error != nil && parsed.Error.Message != "" {
		return "", fmt.Errorf("HTTP %d，服务端错误: %s", resp.StatusCode, parsed.Error.Message)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("HTTP %d，响应无 choices", resp.StatusCode)
	}
	return parsed.Choices[0].Message.Content, nil
}

// ---------------------------------------------------------------------------
// LLM 响应解析
// ---------------------------------------------------------------------------

type llmIntent struct {
	Kind    string `json:"kind"`
	Target  string `json:"target"`
	From    string `json:"from"`
	To      string `json:"to"`
	Element string `json:"element"`
	Level   string `json:"level"`
	Text    string `json:"text"` // record_hypothesis 的猜想原文
}

type llmResponse struct {
	Cognition string      `json:"cognition"`
	Intents   []llmIntent `json:"intents"`
}

// parseLLMResponse 从回复文本中提取 JSON 对象（容忍 markdown 代码围栏
// 与前后闲话——但 system prompt 已要求只输出 JSON）。
func parseLLMResponse(content string) (*llmResponse, error) {
	trimmed := strings.TrimSpace(content)
	// 去掉 ``` 围栏行。
	if strings.HasPrefix(trimmed, "```") {
		if i := strings.IndexByte(trimmed, '\n'); i >= 0 {
			trimmed = trimmed[i+1:]
		}
		if i := strings.LastIndex(trimmed, "```"); i >= 0 {
			trimmed = trimmed[:i]
		}
		trimmed = strings.TrimSpace(trimmed)
	}
	start := strings.IndexByte(trimmed, '{')
	end := strings.LastIndexByte(trimmed, '}')
	if start < 0 || end <= start {
		return nil, fmt.Errorf("回复中没有 JSON 对象: %q", truncateRunes(content, 120))
	}
	var r llmResponse
	if err := json.Unmarshal([]byte(trimmed[start:end+1]), &r); err != nil {
		return nil, fmt.Errorf("JSON 解析失败: %w", err)
	}
	return &r, nil
}

// toIntents 把 LLM 的意图列表换成本包 Intent。宽容解析：不认识的档位
// 回落 small，不挑属性按混合处理；不认识的 kind 丢弃。
// 引用不存在穴位/经脉的意图由会话层丢弃（EventInvalidIntent）——
// 上层转录也会记录，这里不做世界相关校验（本包不持有身体图）。
func (r *llmResponse) toIntents() []Intent {
	out := make([]Intent, 0, len(r.Intents))
	for _, li := range r.Intents {
		var it Intent
		switch strings.ToLower(strings.TrimSpace(li.Kind)) {
		case "sense":
			it.Kind = IntentSense
		case "stop":
			it.Kind = IntentStop
		case "inspect":
			it.Kind = IntentInspect
			it.Acupoint = strings.TrimSpace(li.Target)
		case "absorb":
			it.Kind = IntentAbsorb
			it.Acupoint = strings.TrimSpace(li.Target)
			it.Level = parseLevelLenient(li.Level)
		case "guide":
			it.Kind = IntentGuide
			it.From = strings.TrimSpace(li.From)
			it.To = strings.TrimSpace(li.To)
			it.Element = parseElementLenient(li.Element)
			it.Level = parseLevelLenient(li.Level)
		case "record_hypothesis", "hypothesis":
			it.Kind = IntentHypothesis
			it.Text = strings.TrimSpace(li.Text)
		default:
			continue
		}
		out = append(out, it)
		if len(out) >= llmMaxIntents {
			break
		}
	}
	return out
}

// parseLevelLenient 解析档位词，失败回落 small（新手最不该用力过猛）。
func parseLevelLenient(s string) Level {
	if l, err := ParseLevel(strings.ToLower(strings.TrimSpace(s))); err == nil {
		return l
	}
	return LevelSmall
}

// parseElementLenient 解析五行词；空/不认识 → 混合。
func parseElementLenient(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "":
		return -1
	case "wood", "木":
		return cult.Wood
	case "fire", "火":
		return cult.Fire
	case "earth", "土":
		return cult.Earth
	case "metal", "金":
		return cult.Metal
	case "water", "水":
		return cult.Water
	}
	return -1
}

// addNote 追加修行笔记（截断 + 滚动保留）。
func (p *LLMPilot) addNote(c string) {
	p.Notes = append(p.Notes, truncateRunes(c, llmNoteMaxRunes))
	if len(p.Notes) > llmMaxNotes {
		p.Notes = p.Notes[len(p.Notes)-llmMaxNotes:]
	}
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
