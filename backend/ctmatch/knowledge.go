package ctmatch

// knowledge.go —— 知识系统（stage_2 §28-29）。
//
// Agent 不应只有普通聊天记忆：这里维护独立的证据账本与假说表。
//
// SPIKE 结论二"稀疏反馈下迷信累积"的对策：证据结构必须区分三态——
//   试过且成 / 试过被拒 / 从未试过
// 假说校验以证据账本为准；修行笔记（LLM cognition 回喂，llm.go）保留
// 但降级为附注——账本才是事实凭据。
//
// Agent 可见视图（AgentView）零数字：三态用词、后果方向用词、假说证据
// 多寡用分档词，绝不出现计数。

// ---------------------------------------------------------------------------
// 证据条目
// ---------------------------------------------------------------------------

// EvidenceEntry 记录一次"动作 → 观察到的后果"：
// 意图摘要（定性）、回执词（receipt.go 词表）、后果方向词。
type EvidenceEntry struct {
	Tick    int64  // 内部记账用；不进 Agent 视图
	Action  string // 行动键（ActionKey），如 "absorb@青关"、"guide@丹田→青关"
	Summary string // 定性意图摘要（零数字）
	Receipt string // 回执词：WordApplied / WordRejected / WordBlocked / WordUnclear
	Outcome string // 后果方向词（WordPressureUp/Down/Same；未落地时为空）
}

// ActionKey 把意图归约为账本的行动键。非动作意图返回空串（不入账）。
func ActionKey(it Intent) string {
	switch it.Kind {
	case IntentAbsorb:
		return "absorb@" + it.Acupoint
	case IntentGuide:
		return "guide@" + it.From + "→" + it.To
	}
	return ""
}

// ---------------------------------------------------------------------------
// 三态查询（SPIKE 结论二的核心结构）
// ---------------------------------------------------------------------------

// TrialState 是一条路的三态：从未试过 / 试过被拒 / 试过且成。
type TrialState int

const (
	NeverTried    TrialState = iota // 从未试过
	TriedRejected                   // 试过被拒（此路不通 / 穴未开或满溢不入）
	TriedApplied                    // 试过且成
)

// StateWord 是三态的定性措辞（进 Agent 视图，零数字）。
func StateWord(st TrialState) string {
	switch st {
	case TriedApplied:
		return "试过且成"
	case TriedRejected:
		return "试过被拒"
	}
	return "从未试过"
}

// ---------------------------------------------------------------------------
// 假说（stage_2 §26 第六接口 record_hypothesis 的落点）
// ---------------------------------------------------------------------------

// HypothesisStatus 是假说的验证状态。
type HypothesisStatus string

const (
	HypUnverified HypothesisStatus = "未验证"
	HypSupported  HypothesisStatus = "支持"
	HypDoubtful   HypothesisStatus = "存疑"
	HypRefuted    HypothesisStatus = "证伪"
)

// Hypothesis 是 Agent 登记的一条猜想。Evidence 是关联证据数（自登记以来
// 账本新增的证据条数；内部统计——Agent 视图里只以分档词出现）。
type Hypothesis struct {
	Text     string
	Status   HypothesisStatus
	Evidence int
	Tick     int64
}

// evidenceWord 把证据数折成定性分档词（视图零数字）。
func evidenceWord(n int) string {
	switch {
	case n <= 0:
		return "尚无凭据"
	case n < 3:
		return "已有初步凭据"
	case n < 6:
		return "凭据渐多"
	default:
		return "凭据累累"
	}
}

// ---------------------------------------------------------------------------
// 账本
// ---------------------------------------------------------------------------

// EvidenceLedger 是证据账本 + 假说表。全部操作确定性：同输入序列必得
// 同视图（重演一致性的组成部分）。
type EvidenceLedger struct {
	Entries    []EvidenceEntry
	Hypotheses []*Hypothesis

	order []string // 行动键的首次尝试顺序（视图排序依据，不用 map 遍历）
}

// NewEvidenceLedger 建空账本。
func NewEvidenceLedger() *EvidenceLedger { return &EvidenceLedger{} }

// Record 收一条证据；同时给登记在前的假说累计凭据（自其登记以来的
// 新证据都算它的检验经历——假说由证据印证或推翻，笔记不算）。
func (l *EvidenceLedger) Record(e EvidenceEntry) {
	if e.Action == "" {
		return
	}
	l.Entries = append(l.Entries, e)
	first := true
	for _, k := range l.order {
		if k == e.Action {
			first = false
			break
		}
	}
	if first {
		l.order = append(l.order, e.Action)
	}
	for _, h := range l.Hypotheses {
		if e.Tick >= h.Tick {
			h.Evidence++
		}
	}
}

// StateOf 查询一条行动键的三态。"意图不明"不计入尝试（那不是路，
// 是没把话说清）；"此路不通"与"穴未开或满溢不入"都算试过被拒。
func (l *EvidenceLedger) StateOf(key string) TrialState {
	if key == "" {
		return NeverTried
	}
	tried, applied := false, false
	for i := range l.Entries {
		e := &l.Entries[i]
		if e.Action != key {
			continue
		}
		switch e.Receipt {
		case WordApplied:
			tried, applied = true, true
		case WordRejected, WordBlocked:
			tried = true
		}
	}
	switch {
	case applied:
		return TriedApplied
	case tried:
		return TriedRejected
	}
	return NeverTried
}

// lastOutcome 返回该行动键最近一次"试过且成"的后果方向词（无则空串）。
func (l *EvidenceLedger) lastOutcome(key string) string {
	for i := len(l.Entries) - 1; i >= 0; i-- {
		e := &l.Entries[i]
		if e.Action == key && e.Receipt == WordApplied {
			return e.Outcome
		}
	}
	return ""
}

// labelOf 返回该行动键的定性标签（首次尝试的摘要）。
func (l *EvidenceLedger) labelOf(key string) string {
	for i := range l.Entries {
		if l.Entries[i].Action == key {
			return l.Entries[i].Summary
		}
	}
	return key
}

// RecordHypothesis 登记一条猜想（stage_2 §26 record_hypothesis）。
// 同文重记不新增（账本替 Agent 去重），返回既有/新建的假说。
func (l *EvidenceLedger) RecordHypothesis(text string, tick int64) *Hypothesis {
	for _, h := range l.Hypotheses {
		if h.Text == text {
			return h
		}
	}
	h := &Hypothesis{Text: text, Status: HypUnverified, Tick: tick}
	l.Hypotheses = append(l.Hypotheses, h)
	return h
}

// SetHypothesisStatus 由上层（实验/校验流程）设定假说状态。
func (l *EvidenceLedger) SetHypothesisStatus(text string, st HypothesisStatus) bool {
	for _, h := range l.Hypotheses {
		if h.Text == text {
			h.Status = st
			return true
		}
	}
	return false
}

// AgentView 是给 Pilot 征询载荷的定性视图：先列走过的路（三态 + 最近
// 后果方向），再列猜想。全文零数字。
func (l *EvidenceLedger) AgentView() []string {
	var out []string
	for _, key := range l.order {
		st := l.StateOf(key)
		line := l.labelOf(key) + "：" + StateWord(st)
		if st == TriedApplied {
			if d := l.lastOutcome(key); d != "" {
				line += "，近见" + d
			}
		}
		out = append(out, line)
	}
	for _, h := range l.Hypotheses {
		out = append(out, "猜想〔"+string(h.Status)+"〕"+h.Text+"（"+evidenceWord(h.Evidence)+"）")
	}
	return out
}
