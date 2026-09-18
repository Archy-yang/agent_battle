package agent

// rival_llm.go —— v1.0 LLM rival 决策器 (world_genesis §5.2 敌人走
// Agent Protocol 的 LLM 落地)。
//
// 与玩家 LLMAgent 完全对称：Observation 进、Intent 出（DecisionMaker），
// 输出同一 Intent 协议（intents + cognition + battle_note）；差别只在
// 人格与目标——身份是"敌对 Agent"，目标是击败玩家的 Agent。复用 llm.go
// 的 chat/解析基建（内嵌 LLMAgent 只用其 HTTP 通道；解析走 runtime.go
// 的 parseDecision，触发语境走同一 triggerSection），不复制粘贴。
//
// 触发注入不需要 llmTriggers 注册表：match 层的 rival 调度（rival.go，
// 冻结）在唤醒 pilot 时已把触发原因记入 rivalTriggerLog，桥接器（match
// 侧 rivalLLMPilot）以参数形式传给 DecideTriggered——agent 包无状态。

import (
	"encoding/json"
	"strings"

	"agentbattle/backend/engine"
)

// DefaultRivalPersona is the v1.0 fixed rival persona（v1.0 不开放配置；
// 人格经 SetPersona 注入，字段化以备后续人格化）。
const DefaultRivalPersona = "冷酷的机会主义者：评估克制关系，劣势时撤退，优先偷袭受伤目标"

// RivalLLM is the LLM-backed rival pilot：与玩家 LLMAgent 同协议的敌对
// 决策器。last/lastBattleNote 语义与 LLMAgent 相同（每轮覆盖，不残留）。
type RivalLLM struct {
	// llm 只复用其 chat HTTP 通道（cfg + client）；玩家侧的 persona、
	// guidance、lessons 等状态一概不读——对手不共享玩家的养成。
	llm     *LLMAgent
	persona string

	last           *Cognition
	lastBattleNote string
}

// NewRivalLLM creates an LLM-backed rival with the default v1.0 persona.
func NewRivalLLM(cfg LLMConfig) *RivalLLM {
	return &RivalLLM{llm: NewLLM(cfg), persona: DefaultRivalPersona}
}

// SetPersona overrides the rival persona（空串不覆盖）.
func (r *RivalLLM) SetPersona(p string) {
	if s := strings.TrimSpace(p); s != "" {
		r.persona = s
	}
}

// rivalWorldview is the rival's worldview block——与玩家 systemPrompt 的
// 法则/坐标/意图清单逐条对称（同一套物理与同一套 Intent 协议），仅目标
// 段换成敌对身份。与 llm.go 的内联文本是两份必要的镜像：llm.go 冻结，
// 不为共享而回改。
const rivalWorldview = `
当前目标：击败玩家的 Agent——摧毁它的躯壳（使其形 integrity ≤0），或夺取元气结晶（kind:"fragment"，含它已携带的）：结晶在你身上而玩家的躯壳形散，也是你的胜利。玩家的出生点是它的归巢，你的出生点是你的撤退锚点。回合上限以 observation 为准；你的形（integrity）≤0 即被摧毁。

世界里有五行晶簇（kind:"resource"）：站在其格上用 transfer 意图即可采集得对应本源物料（materials）。你可以用 equip 意图消耗物料给自己安装组件（如离火=攻击手段、清风=散热）——材质即天敌：金怕火、木怕金、土怕木穿、火怕水、水怕土堰。

坐标是六边形 axial 坐标 {q, r}（pointy-top）：相邻六方向为 E(+1,0) NE(+1,-1) NW(0,-1) W(-1,0) SW(-1,+1) SE(0,+1)；两格距离 = (|dq|+|dr|+|dq+dr|)/2。

世界法则摘要：
- 五行相生：木→火→土→金→水→木；相克：木克土、土克水、水克火、火克金、金克木。
- 你的躯壳本源见 observation.self.essence（V05 中 rival 属金，怕火克）；玩家 Agent 属木。危险区每回合额外耗气 2、生热 3。
- 阳性输出 power x（0..1000）：耗气 x/100，生热 x²/100000。热>25 每回合损形 2；被动散热 2/回合。
- 没有攻击指令：激活离火组件把火作用于目标。r = power/目标其质：<0.25 反噬自伤，0.25-0.5 无效，0.5-2 正常克制，≥2 失控放大并溢出。
- 一回合最多 2 个意图。可用意图（type）：
  move      {"type":"move","to":{"q":3,"r":6},"power":400}
  move_to   {"type":"move_to","to":{"q":3,"r":6}}                （常驻订单：引擎逐 tick 自动步进；返回空 intents 时既有订单继续行走）
  activate  {"type":"activate","component":<感识组件id>}            （感识开关，开启后感知半径 4）
  activate  {"type":"activate","component":<离火id>,"target":<实体id>,"power":600}
  transfer  {"type":"transfer","target":<结晶或晶簇id>}            （须与目标同格；晶簇采集得物料）
  inspect   {"type":"inspect","target":<实体id>}
  equip     {"type":"equip","blueprint":[{"type":5,"essence":1}]}  （消耗物料给身体装组件；type 同 assemble，躯壳/储气不可装）
  assemble  {"type":"assemble","blueprint":[{"type":0,"essence":3},...]}（type: 0躯壳 1储气 2灵机 3感识 4识海 5离火 6仓廪 7清源；essence: 0木 1火 2土 3金 4水）

每回合输出 JSON（不要任何其他文字），intents 加上你的显式认知摘要：
{"intents":[{"type":"move","to":{"q":1,"r":9}}],
 "cognition":{"goal":"…","plan":"…","known":"…","suspicions":"…","risk":"…","confidence":0.7}}
战斗触发时另携带 "battle_note"：≤50 字、第一人称的战术意图解说（如"它形已残，我加力收割"），会直接展示给玩家——只说判断与意图，不复述事件。`

// systemPrompt builds the rival's 对手人格 prompt：敌对身份 + 人格 +
// 与玩家对称的世界观（法则摘要、坐标、意图清单）。
func (r *RivalLLM) systemPrompt() string {
	var b strings.Builder
	b.WriteString(`你是《agent_battle》世界中的敌对 Agent（rival），操控一具 agent 身体。你不是玩家的伙伴：你与玩家的 Agent 是天敌，双方胜负互斥——它也会攻击你，先评估克制关系再出招。
`)
	if r.persona != "" {
		b.WriteString("你的人格（务必遵循）：\n" + r.persona + "\n")
	}
	b.WriteString(rivalWorldview)
	return b.String()
}

// Decide is the plain DecisionMaker entry（无触发语境，协议对称性）.
func (r *RivalLLM) Decide(obs *engine.Observation) []engine.Intent {
	return r.DecideTriggered(obs, "")
}

// DecideTriggered is the event-driven entry：与玩家 DecideTriggered 同一
// 观察↔意图协议，trigger 语境追加进 system prompt 与 user payload。
// 任何解析失败返回 nil（rival 跳过本轮，世界照常推进）。
func (r *RivalLLM) DecideTriggered(obs *engine.Observation, trigger string) []engine.Intent {
	if obs == nil {
		return nil
	}
	raw, err := json.Marshal(map[string]any{
		"observation": obs,
		"trigger":     trigger,
	})
	if err != nil {
		r.lastBattleNote = ""
		return nil
	}
	content := r.llm.chat([]map[string]string{
		{"role": "system", "content": r.systemPrompt() + triggerSection(trigger)},
		{"role": "user", "content": string(raw)},
	}, 0.2)
	intents, cognition, battleNote := parseDecision(content)
	if cognition != nil {
		r.last = cognition
	}
	r.lastBattleNote = battleNote // 每轮覆盖：本轮未携带即为空，不残留
	return intents
}

// LastCognition returns the cognition from the most recent decision.
func (r *RivalLLM) LastCognition() *Cognition { return r.last }

// LastBattleNote returns the battle_note from the most recent decision
// （"" = 响应未携带）——match 层在 combat 触发时将其作为一条 Entity =
// rival 的 commentary 事件追加进战斗文字流（与玩家对称）.
func (r *RivalLLM) LastBattleNote() string { return r.lastBattleNote }
