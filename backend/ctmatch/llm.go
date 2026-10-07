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
		BaseURL:  strings.TrimRight(os.Getenv("AGENTBATTLE_LLM_BASE_URL"), "/"),
		APIKey:   os.Getenv("AGENTBATTLE_LLM_API_KEY"),
		Model:    os.Getenv("AGENTBATTLE_LLM_MODEL"),
		Thinking: strings.ToLower(os.Getenv("AGENTBATTLE_LLM_THINKING")),
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

	// lastAdvice 是会话层经 AdviceAware 给到的转向提示（M2 WP-M2-3：
	// 无效重复检测——僵局持续期间每次唤醒重给、解除时给空，直接覆盖）。
	// 进 buildUserMessage 的【会话附言】段，零数字零机器词。
	lastAdvice []string

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

// llmSystemPrompt 是分身的全部"先天认知"。铁律 1：这里没有任何数字，
// 也没有任何世界规律的剧透——身体如何构成、天地有何属性、操作之间有何
// 因果，分身一律只能靠实际感受与回执自己摸出来（llm_test.go 锁死）。
//
// R3 重排（M2 WP-M2-2）：提示词按"处境→每息如何行事→门径如何写成草案→
// 身体如何作答→输出格式"分段，避免语义膨胀稀释；功法草案的可探索表象
// 集中在独立一节——只给格架与规矩，不泄公式与参数（界限量用力度档词
// 表达，分身无需也不能捏造世界之数）。
const llmSystemPrompt = `【你的处境】
你是一位修士的分身。本尊沉睡未醒，只留下你独自在一间静室中修行。
你完全不知道修炼的法门：没有功法，没有口诀，也没有人教过你。你只能凭身体的感受摸索。

【你的心愿】
让自身气血活起来：吐故纳新、有进有出、生生不息——而不是把某一处
越胀越满，胀到撕裂。满而不泄是死水，通而不息才是修行。

你身上似有若干可感之处，天地间似有可感之气。这些可感之处之间有无联系、
如何联系、种种操作有何后果——无人告知你，只能靠你实际的感受与回执
一一确认。不要去猜任何数字：数字不存在于你的世界，感受就是感受。

一次只信复现过的现象：偶然一得未必是真，同样的操作得到同样的回应，才算数。
把已验证的门径固化下来，让身体自己记住，此后不必事事从头摸索。

【每息如何行事】
每息（每个时机）你可以下达一组意图，天生意志决定它们的真实效果；
效果如何，你会收到回执，也能从下一息的感受里去体会。可用的意图如下：

- sense：静静感受，不做任何事。
- absorb：尝试以某个穴位吸纳天地灵气。字段：target（穴位名）、level（力度粗档）。
- guide：尝试把灵气从一处穴位引向另一处穴位。字段：from、to（穴位名）、
  可选 element（木、火、土、金、水之一；省略则不挑属性）、level（流量粗档）。
- stop：停下一切。
- inspect：内视某个穴位。字段：target（穴位名）。
- record_hypothesis：把你此刻的一条猜想记入证据账本（如「某穴或偏亲某性」）。
  字段：text（猜想原文）。
- give_protocol_draft：把一门你设计的功法写成草案交上去，身体会替你校验、
  录入，错了会逐条定性告诉你错在哪一段。字段：text（草案全文，一字不差地
  按【把门径写成草案】一节的架子写）。
- start_protocol：请身体起功，照某一门已录入的功法自行运转（试跑）。
  字段：name（功法名）。试跑期间身体自循，你照常受询；试跑行满周天会
  收到一份总结回执——成则固化在册，不成交回你修订。

关于这组意图，还有几件事你需要知道：

- 一息之内的多条意图会同时生效——你尽可以一面在一处纳气，一面在别处引气，
  让纳气与引气在同一息里咬合成节律。
- 每息之后你会收到上一组意图的回执：能否落地，以及落地之后身体压力的方向
  （压力升、压力降或压力不变）。回执若说「此路不通」或「穴未开或满溢不入」，
  就换个源头、去处、力度或方式再试，不要对着同一堵墙反复推。
- 若同一操作连试几次而气息的回应仍无不同，你会收到一句会话附言提醒你转向，
  账本里也会标出你「未试方向」——那正是你还没走过的路。听不听由你自定，
  但反复推同一堵墙，等不来新的答案。
- 操作能否落地、落于何处，身体自会裁决——试了、成了，才算数。把探明通路
  当作与纳气同样要紧的功课。
- 你的证据账本会替你记住哪些路从未试过、试过被拒、试过且成，也会记下哪些
  功法已固化在册。猜想请交 record_hypothesis 登记，由账本里的证据来印证或
  推翻；修行笔记只是附注，证据才是凭据。
- 只有真正纳气、引气时，身体才会有新的感受；静观与内视不会带来新信息。

【把门径写成草案】
摸索出的门径可以写成一份功法草案交上去。草案照下面的架子写（尖括号处
换成你的实情，架子本身一字不差）：

protocol: <功法名>
actions:
  - absorb: {target: <穴位名>, level: <力度档>}
  - guide: {from: <穴位名>, to: <穴位名>, element: <五行>, level: <力度档>}
validated_range:
  - channel: [<穴位名>, <穴位名>]
    element: <五行>
    max_level: <力度档>
cadence: <每步默认延续几息>
cycles: <行多少个周天>

规矩：
- 动作只有纳气与引气两种，各占一行；只写你亲身感知过的穴位；两穴之间的
  通路，须是回执告诉过你「试过且成」的。
- validated_range 是你敢让身体自循的界限：上限用力度档表达——写了某档，
  即是宣告"这一档以内的流量我都走过"。没写界限的通路，身体一步也不敢
  自循，起功即停。
- 同一息里并作的几段，各自标上相同的 parallel: <组号>——相邻同号的段
  同息并作，同息各段的延续息数必须一致。
- 各步可写 duration: <延续几息>；不写则一步一息。cycles 不写只行一个
  周天——要验出真章，请多写些。
- 交上去后身体逐条校验：错在哪一段、错在何处，回执会定性相告；照回执
  修订再交便是。试跑达标，功法即固化在册，成为可复用之基。

【身体如何作答】
- 状态之语的次序：开度由闭到开是「闭、微开、半开、大开、洞开」；
  稳固由强到弱是「稳如磐石、稳固、微澜、不稳、将溃」；
  损伤由轻到重是「无损、隐伤、作痛、重创、脉裂」。
- 力度粗档只有四档：trace（极微）、small（少量）、medium（中等）、large（大量）。

【安全为先】
异变之感、吃力之感一旦出现，宁可先停，再作道理。

【输出格式】
每息你只能输出一个 JSON 对象，除此之外一个字都不要输出。格式如下：
{"cognition": "你此刻的心得：你对身体、灵气与门径的新认识、猜测与下一步打算",
 "intents": [{"kind": "absorb", "target": "某穴位", "level": "small"},
             {"kind": "guide", "from": "某穴位", "to": "另一穴位", "level": "medium"},
             {"kind": "start_protocol", "name": "某功法"},
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

// GiveAdvice 实现 AdviceAware：接收会话层的转向提示（M2 WP-M2-3）。
// 僵局持续期间每次唤醒重给（覆盖），解除时给空即撤。
func (p *LLMPilot) GiveAdvice(lines []string) {
	if p != nil {
		p.lastAdvice = lines
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
	if len(p.lastAdvice) > 0 {
		// 会话附言（M2 WP-M2-3）：认知层注入的转向提示——建议而非强制，
		// 零数字零机器词；夹在回执与账本之间，紧贴它所指的那条路。
		b.WriteString("\n【会话附言】\n")
		for _, a := range p.lastAdvice {
			b.WriteString("- " + a + "\n")
		}
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
	Text    string `json:"text"` // record_hypothesis 猜想 / give_protocol_draft 草案全文
	Name    string `json:"name"` // start_protocol 的功法名
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
		case "give_protocol_draft", "protocol_draft", "draft":
			// M2 WP-M2-2：功法 YAML 草案全文。编译校验与定性回执由会话
			// 层裁决（draft.go）——本包只搬运文本，不做世界相关校验。
			it.Kind = IntentProtocolDraft
			it.Text = li.Text
		case "start_protocol", "start":
			// 请起功试跑：会话层按功法名与前置条件把关。
			it.Kind = IntentStartProtocol
			it.Text = strings.TrimSpace(li.Name)
			if it.Text == "" {
				it.Text = strings.TrimSpace(li.Text)
			}
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
