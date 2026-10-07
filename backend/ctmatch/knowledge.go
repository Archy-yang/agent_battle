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
//
// M2 WP-M2-3（探索僵局对策）：账本另供 UntriedDirections——基于分身
// 自己的尝试记录推导"已知名所之间、但从未试过"的方向，供僵局转向提示
// 与账本凸显使用。只回喂分身自己说过的名字，不泄露任何未探明事实。

import (
	"strings"
)

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
	Why     string // 拒绝/阻断补语词（receipt.go Why*；内部记账用，不进
	// Agent 视图）。UntriedDirections 依它过滤"目标穴位不存在"的条目——
	// 分身念错的名字（幻觉穴名）不回喂、不进已知名所。
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
// 固化台账（M2 WP-M2-2：功法试跑达标转正即固化在案，成为可复用的功法）
// ---------------------------------------------------------------------------

// SolidEntry 是一条固化记录：试运行达标转正的功法（stage_2 §29
// 知识→功法、stage_3 §29）。固化语义：进台账、成为可复用之基；同名
// 草案此后不可覆盖（draft.go 的草案校验把关）。
type SolidEntry struct {
	Name string // 功法名
	Path string // 定性路径行（protocolPath，"青关 → 下丹田"式）
}

// ---------------------------------------------------------------------------
// 账本
// ---------------------------------------------------------------------------

// EvidenceLedger 是证据账本 + 假说表 + 固化台账。全部操作确定性：
// 同输入序列必得同视图（重演一致性的组成部分）。
type EvidenceLedger struct {
	Entries    []EvidenceEntry
	Hypotheses []*Hypothesis
	Solidified []SolidEntry

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

// Solidify 把一门已转正的功法记入固化台账（幂等：同名不重记）。
func (l *EvidenceLedger) Solidify(name, path string) {
	for _, e := range l.Solidified {
		if e.Name == name {
			return
		}
	}
	l.Solidified = append(l.Solidified, SolidEntry{Name: name, Path: path})
}

// ---------------------------------------------------------------------------
// 未试方向（M2 WP-M2-3：僵局转向的证据凸显）
// ---------------------------------------------------------------------------

// UntriedDirections 基于分身自己的尝试记录，推导"已知名所之间、但从未
// 试过"的方向（M2 WP-M2-3：转向提示与账本凸显的数据源）。
//
// 感知隔离律的边界：
//   - 穴位名只取分身亲自交出过、且回执补语不是"目标穴位不存在"的名字——
//     分身自己说过的，才可以说回去；幻觉穴名（WhyNoAcupoint）一律过滤。
//   - 经脉存在性一律不做断言：建议"引气自乙至甲"不是说那条路通，只是
//     提示它从未试过——试了才有回执（Engine 决定实际结果）。
//   - 输出确定性：全部按账本首次尝试/首现序排列，map 只作去重查存，
//     绝不作遍历序（重演一致性的组成部分）。
func (l *EvidenceLedger) UntriedDirections() []string {
	// 已知穴位（按证据条目序的首现序）。
	var known []string
	seenPt := make(map[string]bool)
	for i := range l.Entries {
		e := &l.Entries[i]
		if e.Why == WhyNoAcupoint {
			continue // 回执说过"目标穴位不存在"的名字，不回喂
		}
		var pts []string
		if rest, ok := strings.CutPrefix(e.Action, "absorb@"); ok {
			pts = []string{rest}
		} else if rest, ok := strings.CutPrefix(e.Action, "guide@"); ok {
			if j := strings.Index(rest, "→"); j >= 0 {
				pts = []string{rest[:j], rest[j+len("→"):]}
			}
		}
		for _, p := range pts {
			if p != "" && !seenPt[p] {
				seenPt[p] = true
				known = append(known, p)
			}
		}
	}

	var out []string
	seenSug := make(map[string]bool)
	// 一、未试的引气方向：已试通路的反向（去程走通之后，回程常是
	// 下一个该问的问题；此路不通的通路其反向同样值得一试）。
	for _, key := range l.order {
		rest, ok := strings.CutPrefix(key, "guide@")
		if !ok {
			continue
		}
		j := strings.Index(rest, "→")
		if j < 0 {
			continue
		}
		from, to := rest[:j], rest[j+len("→"):]
		if from == to {
			continue
		}
		// 该通路至少要有一次"名字真实"的尝试（Why 过滤），两端穴位
		// 才算分身已知的实有所在。
		real := false
		for i := range l.Entries {
			if l.Entries[i].Action == key && l.Entries[i].Why != WhyNoAcupoint {
				real = true
				break
			}
		}
		if !real {
			continue
		}
		sug := "引气自" + to + "至" + from
		if !seenSug[sug] && l.StateOf("guide@"+to+"→"+from) == NeverTried {
			seenSug[sug] = true
			out = append(out, sug)
		}
	}
	// 二、未试的纳气处：分身已知的穴位里，还没纳过气的。
	for _, p := range known {
		sug := "于" + p + "纳气"
		if !seenSug[sug] && l.StateOf("absorb@"+p) == NeverTried {
			seenSug[sug] = true
			out = append(out, sug)
		}
	}
	return out
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
	for _, e := range l.Solidified {
		out = append(out, "功法〔已固化〕"+e.Name+"："+e.Path)
	}
	return out
}
