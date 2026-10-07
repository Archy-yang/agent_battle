package ctmatch

// receipt.go —— 意图回执（M1 铁律 6：意图回执律；SPIKE 结论一"静默丢弃"的对策）。
//
// 任何意图落地与否，会话层必须给 Agent 一条中性回执：无法执行 / 已被拒 /
// 已生效及其粗档后果方向。回执属接口语义而非世界真值：它只告诉 Agent
// "这份心意是否被天地接住、身体压力朝哪个方向去了"，绝不泄露公式、参数
// 与任何数值（SPIKE 三轮证实：静默丢弃是分身建立错误理论的最大失败源）。
//
// 措辞集中本文件词表（唯一出处，改手感只改这里）；Agent 可见视图一律经
// ReceiptView 渲染，全文零数字——wp4_test.go 以 llm_test.go 的隔离测试
// 风格锁死。

import (
	"strings"

	"agentbattle/backend/cult"
)

// ---------------------------------------------------------------------------
// 词表（回执措辞的唯一出处）
// ---------------------------------------------------------------------------

// 落定类与拒绝类的回执主词。
const (
	WordApplied  = "已生效"      // 意图已被天地接住
	WordRejected = "此路不通"     // 目标穴位/经脉不存在，意图无处落地
	WordBlocked  = "穴未开或满溢不入" // 意图交出了，身体却没有接纳
	WordUnclear  = "意图不明"     // 缺少必要说明（如未给出力度）
	WordNoted    = "已记下"      // 记账类意图（如 record_hypothesis）
	WordUnproven = "未足为凭"     // 试跑完毕但未达转正之准（M2 WP-M2-2）
)

// 落地后的粗档后果方向词：只给方向，不给数。
const (
	WordPressureUp   = "压力升"
	WordPressureDown = "压力降"
	WordPressureSame = "压力不变"
)

// 拒绝/阻断的定性补语。
const (
	WhyNoAcupoint  = "目标穴位不存在"
	WhyNoChannel   = "两穴之间没有经脉直接相连"
	WhyNoLevel     = "未说明力度"
	WhyNoFlow      = "未说明流量"
	WhyBadElement  = "所引的五行不明"
	WhyNotTakenIn  = "穴位未开，或已满溢，气入不进去"
	WhyNotBackflow = "逆经脉而行，唯丹田能推得动" // 修订包 A2：非汇点发起逆流
)

// ---------------------------------------------------------------------------
// 回执事件
// ---------------------------------------------------------------------------

// IntentReceipt 是一条意图回执：每 Tick 每意图恰一条。Tick 仅供会话层
// 记账；Agent 可见载荷是 ReceiptView 的渲染结果——零数字。
type IntentReceipt struct {
	Tick      int64
	Kind      IntentKind
	Summary   string // 定性意图摘要（如"于青关纳气（少量）"）
	Outcome   string // 词表：WordApplied / WordRejected / WordBlocked / WordUnclear / WordNoted
	Detail    string // 定性补语（可空），取自 Why* 词表
	Direction string // 压力方向词（仅动作类已生效时给出；其余为空）
}

// ReceiptView 是 Agent 可见的回执一行文：集中渲染，保证零数字。
func ReceiptView(r IntentReceipt) string {
	var b strings.Builder
	b.WriteString(r.Summary)
	b.WriteString("：")
	b.WriteString(r.Outcome)
	if r.Detail != "" {
		b.WriteString("（")
		b.WriteString(r.Detail)
		b.WriteString("）")
	}
	if r.Direction != "" {
		b.WriteString("，")
		b.WriteString(r.Direction)
	}
	return b.String()
}

// ReceiptViews 把一批回执渲染成 Agent 可见行文。
func ReceiptViews(rs []IntentReceipt) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, ReceiptView(r))
	}
	return out
}

// intentSummary 生成一条意图的定性摘要（零数字：显式流量只说"定量"，
// 档位与五行用词表）。
func intentSummary(it Intent) string {
	switch it.Kind {
	case IntentSense:
		return "静静感受"
	case IntentStop:
		return "收功止念"
	case IntentInspect:
		return "内视" + it.Acupoint
	case IntentAbsorb:
		return "于" + it.Acupoint + "纳气" + levelWord(it.Level)
	case IntentGuide:
		return "引气自" + it.From + "至" + it.To + elementWord(it.Element) + levelWord(it.Level)
	case IntentHypothesis:
		return "记下一条猜想"
	case IntentProtocolDraft:
		return "呈上一份功法草案"
	case IntentStartProtocol:
		return "请起功「" + it.Text + "」"
	}
	return "一份不明的心意"
}

func levelWord(l Level) string {
	switch l {
	case LevelTrace:
		return "（微微）"
	case LevelSmall:
		return "（少量）"
	case LevelMedium:
		return "（中等）"
	case LevelLarge:
		return "（大量）"
	}
	if l == LevelUnset {
		return "（定量）" // 显式流量：是"精确控制"，不是档位——摘要不给数
	}
	return ""
}

func elementWord(e int) string {
	if e < 0 || e >= cult.ElementCount {
		return ""
	}
	return "（" + [...]string{"木", "火", "土", "金", "水"}[e] + "行）"
}

// pressureDirection 比较 target 穴位本 Tick 与上一 Tick 的压力，给出
// 方向词（方向来自真值比较，但载荷上只有词——铁律 1/6 的边界在渲染层）。
func pressureDirection(cur, prev *cult.TickReport, idx int) string {
	if cur == nil || idx < 0 || idx >= len(cur.Points) {
		return WordPressureSame
	}
	p := cur.Points[idx].Pressure
	if prev != nil && idx < len(prev.Points) {
		q := prev.Points[idx].Pressure
		if p > q {
			return WordPressureUp
		}
		if p < q {
			return WordPressureDown
		}
	}
	return WordPressureSame
}

// ---------------------------------------------------------------------------
// Pilot 的回执/账本扩展（可选接口——既有 Pilot 实现不受影响）
// ---------------------------------------------------------------------------

// ReceiptAware 是 Pilot 的可选扩展：愿意接收"上一组意图的回执列表"。
// 会话层在唤醒 Pilot 时、于 Decide 之前调用；无新回执时沿用上一组
// （没有新意图，就没有新回执——"上一组"指最近一次交出的意图）。
type ReceiptAware interface {
	GiveReceipts(receipts []IntentReceipt)
}

// LedgerAware 是 Pilot 的可选扩展：愿意接收证据账本的定性视图
// （knowledge.go；三态事实凭据，修行笔记只是附注）。
type LedgerAware interface {
	GiveLedgerView(view []string)
}

// GiveReceipts 使 ScriptedPilot 可记录会话层给到的回执（测试断言用）。
func (p *ScriptedPilot) GiveReceipts(rs []IntentReceipt) {
	if p != nil {
		p.LastReceipts = rs
	}
}

// GiveLedgerView 使 ScriptedPilot 可记录会话层给到的账本视图（测试断言用）。
func (p *ScriptedPilot) GiveLedgerView(v []string) {
	if p != nil {
		p.LastLedger = v
	}
}
