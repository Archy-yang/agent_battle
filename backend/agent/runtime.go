package agent

// Agent Runtime —— v0.4 LLM 介入层 (design §3)。
//
// WP3 把决策时机从"每 tick"改为"每个值得判断的时刻"：match 侧的
// EventClassifier 判定本轮触发原因，经 SetTrigger 注入本 runtime；
// 触发决策一律走 DecideTriggered——system prompt 追加"触发语境"段，
// user payload 增加 trigger 字段。llm.go 的 Decide 是每 tick 常规决策
// 的历史入口，本波冻结不改（不改既有文件签名）。
//
// LLMAgent 的字段声明在 llm.go，触发状态由此处的注册表承载（键为
// agent 指针，生命周期与 agent 一致；每次决策消费即清除）。

import (
	"encoding/json"
	"log"
	"strings"
	"sync"

	"agentbattle/backend/engine"
)

// llmTriggers holds each LLMAgent's armed trigger reason (*LLMAgent →
// string). LLMAgent 结构体属于 llm.go（WP3 不改既有文件），触发状态
// 挂在包级注册表上.
var llmTriggers sync.Map

// SetTrigger arms the reason the agent is being woken for its next
// decision (design §3.2)：本轮是因为什么被唤醒。空串解除武装.
func (a *LLMAgent) SetTrigger(reason string) {
	if reason == "" {
		llmTriggers.Delete(a)
		return
	}
	llmTriggers.Store(a, reason)
}

// takeTrigger reads and consumes the armed trigger (一次决策一消费).
func (a *LLMAgent) takeTrigger() string {
	v, ok := llmTriggers.LoadAndDelete(a)
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// triggerSection renders the 触发语境 block appended to the system
// prompt：本轮因为什么被唤醒，并提示"本次可以只判断、不必移动"——
// 返回空 intents 时既有 move_to 订单仍由引擎逐 tick 执行 (design §3.1
// 执行层).
func triggerSection(reason string) string {
	if reason == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n【触发语境】本轮你不是按 tick 常规唤醒，而是被一个事件唤醒：trigger=" + reason + "。")
	b.WriteString("先回应这个触发点（判断、陈述、调整计划），再决定是否需要动作；返回空 intents 即保持既有订单继续行走。\n")
	switch {
	case reason == "heartbeat":
		b.WriteString("这是心跳反思（开局或连续多 tick 无新事件）：回顾目标与计划是否仍然成立，更新认知摘要；若无必要改变，保持原订单即可。")
	case reason == "combat":
		b.WriteString("这是战斗回合：交战期间每个 tick 都会唤醒你。优先判断还击、撤退或调整作用强度。")
		b.WriteString("本次响应 JSON 额外携带可选字段 \"battle_note\"：≤50 字、第一人称的战术意图解说")
		b.WriteString("（如\"它金甲厚重，我改打它的水行储气\"），会直接展示给玩家——只说判断与意图，不复述事件。")
	case reason == "encounter":
		b.WriteString("首次感知到敌对实体：先评估威胁（五行、距离、其质），再决定接近、绕行或备战。")
	case strings.HasPrefix(reason, "threshold:"):
		b.WriteString("你的状态越过了警戒阈值。本轮可以只判断、不必移动：评估风险、调整计划即可。")
	case reason == "guidance":
		b.WriteString("玩家刚刚给出干预（见 guidance 字段）。请优先消化玩家偏好，再决定行动。")
	case reason == "resource":
		b.WriteString("发现新的物料/资源信息（区块懒激活或首次拾取）：评估是否调整采集与组装计划。")
	case reason == "arrive":
		b.WriteString("抵达/受阻事件：订单已完成或连续受阻。请基于当前位置重新规划下一步。")
	}
	return b.String()
}

// DecideTriggered is the WP3 event-driven decision entry (design §3)：
// 与 Decide 同一观察↔意图协议，但 system prompt 追加触发语境、user
// payload 增加 trigger 字段。任何解析失败与 Decide 一样返回 nil（跳过
// 本轮，世界照常推进）。
func (a *LLMAgent) DecideTriggered(obs *engine.Observation) []engine.Intent {
	if obs == nil {
		return nil
	}
	reason := a.takeTrigger()
	userPayload := map[string]any{
		"observation": obs,
		"guidance":    a.guidance,
		"trigger":     reason,
	}
	raw, err := json.Marshal(userPayload)
	if err != nil {
		return nil
	}
	content := a.chat([]map[string]string{
		{"role": "system", "content": a.systemPrompt() + triggerSection(reason)},
		{"role": "user", "content": string(raw)},
	}, 0.2)
	intents, cognition, battleNote := parseDecision(content)
	if cognition != nil {
		a.last = cognition
	}
	a.lastBattleNote = battleNote // 每轮覆盖：本轮未携带即为空，不残留
	return intents
}

// parseDecision parses one LLM decision reply ("```json" 包裹容错)，
// 一并返回可选的 battle_note（战斗解说，combat 触发时才有意义）.
func parseDecision(content string) ([]engine.Intent, *Cognition, string) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, nil, ""
	}
	var parsed struct {
		Intents    []engine.Intent `json:"intents"`
		Cognition  *Cognition      `json:"cognition"`
		BattleNote string          `json:"battle_note"`
	}
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		log.Printf("[llm] unparseable intents: %v: %.200s", err, content)
		return nil, nil, ""
	}
	return parsed.Intents, parsed.Cognition, strings.TrimSpace(parsed.BattleNote)
}
