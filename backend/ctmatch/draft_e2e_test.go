package ctmatch

// draft_e2e_test.go —— 功法编译端到端主验收（M2 WP-M2-2）：脚本分身驱动
// 完整链路（禁止真实 LLM）：
//
//	感知身体 → 提交含故意错误的草案 → 吃到定性回执 → 修订 → 编译通过
//	→ 请起功试跑（真实行环，协议运行期回执照常回流）→ 试跑总结回执
//	→ 固化在案。
//
// 场景 B 验证"分身依据试跑回执决定修订"：第一门草案周天太少未足为凭，
// 分身依总结回执修订（加多周天）再试，直至达标固化。
//
// 分身是纯脚本状态机（ScriptedPilot 同族，无 LLM、无世界真值读取）：
// 决策输入只有观察（定性）与回执（定性视图），推进判定全部依据回执
// 措辞——即"分身依据回执行动"这一闭环本身被测试。

import (
	"strings"
	"testing"

	"agentbattle/backend/cult"
)

// e2eBadDraft 含三类故意错误（各占一段，回执应逐条定性指路）：
// 第一段纳气落点未辨识；第二段两穴无通路；第三段逆流非丹田发起。
const e2eBadDraft = `
protocol: 冒进诀
actions:
  - absorb: {target: 玄府, level: small}
  - guide: {from: 青关, to: 阳枢, level: medium}
  - guide: {from: 阳枢, to: 丹田, level: medium}
`

// e2eGoodDraft 是修订后的草案：修订点——落点改为已辨识的青关、
// 通路改为回执确认过的 丹田↔青关、删去逆流段。界限用力度档表达。
const e2eGoodDraft = `
protocol: 周天初环
actions:
  - absorb: {target: 青关, level: trace, parallel: 1}
  - guide: {from: 丹田, to: 青关, element: wood, level: medium, parallel: 1}
  - guide: {from: 青关, to: 丹田, element: wood, level: medium, parallel: 1}
cadence: 1
cycles: 25
validated_range:
  - channel: [丹田, 青关]
    element: wood
    max_level: large
  - channel: [青关, 丹田]
    element: wood
    max_level: large
`

// e2eShortDraft 是场景 B 的第一门草案：架子成立、能跑，但周天太少。
var e2eShortDraft = strings.Replace(e2eGoodDraft, "cycles: 25", "cycles: 5", 1)

// e2eDraftsA 是场景 A 的修订链：含错稿 → 修订稿。
var e2eDraftsA = []string{e2eBadDraft, e2eGoodDraft}

// e2eDraftsB 是场景 B 的修订链：短稿（试跑未足为凭）→ 长稿。
var e2eDraftsB = []string{e2eShortDraft, e2eGoodDraft}

// 脚本分身的修行阶段。
type e2eStage int

const (
	stagePerceive   e2eStage = iota // 感知身体
	stageSubmitBad                  // 交第一份草案（含错）
	stageLearn                      // 等定性回执
	stageSubmitGood                 // 依回执修订再交
	stageWaitStart                  // 等录入回执
	stageStart                      // 请起功试跑
	stageRun                        // 入定候跑（协议运行期，回执照常回流）
	stageDone                       // 见总结回执，功成
)

// draftE2EPilot 是脚本分身：每息至多交一组意图，阶段推进只看回执措辞
// （ReceiptAware 载荷），绝不读世界真值。drafts 是它的修订链：被拒即
// 依回执换下一稿（"依据回执修订"这一决策本身由回执措辞驱动）。
type draftE2EPilot struct {
	stage    e2eStage
	drafts   []string // 修订链：第一稿（含错）→ 修订稿
	draftIdx int

	seenReceipts []IntentReceipt // 历次唤醒收到的回执（断言用）
	runMark      int             // 起功时的回执游标（stageRun 只认其后的总结）
	seenObs      bool            // 是否收到过定性观察
	obsPoints    int             // 观察里的可感之处数
}

func (p *draftE2EPilot) draft() string { return p.drafts[p.draftIdx] }

func (p *draftE2EPilot) GiveReceipts(rs []IntentReceipt) {
	if p != nil {
		p.seenReceipts = append(p.seenReceipts, rs...)
	}
}

func (p *draftE2EPilot) GiveLedgerView(v []string) {}

// latestDraftReceipt 取最近一条草案相关回执（无则 nil）。
func (p *draftE2EPilot) latestDraftReceipt() *IntentReceipt {
	for i := len(p.seenReceipts) - 1; i >= 0; i-- {
		rc := &p.seenReceipts[i]
		if strings.Contains(rc.Summary, "功法草案") {
			return rc
		}
	}
	return nil
}

func (p *draftE2EPilot) latestSummary(word string) *IntentReceipt {
	for i := len(p.seenReceipts) - 1; i >= 0; i-- {
		rc := &p.seenReceipts[i]
		if strings.Contains(rc.Summary, word) {
			return rc
		}
	}
	return nil
}

func (p *draftE2EPilot) Decide(obs *Observation) []Intent {
	if obs != nil {
		p.seenObs = true
		if n := len(obs.Points); n > p.obsPoints {
			p.obsPoints = n
		}
	}
	switch p.stage {
	case stagePerceive:
		p.stage = stageSubmitBad
		return []Intent{{Kind: IntentSense}}
	case stageSubmitBad:
		p.stage = stageLearn
		return []Intent{{Kind: IntentProtocolDraft, Text: p.draft()}}
	case stageLearn:
		rc := p.latestDraftReceipt()
		if rc == nil {
			return nil
		}
		if rc.Outcome == WordRejected {
			// 吃到定性回执：依回执修订——换下一稿，重走校验。
			p.draftIdx++
			p.stage = stageSubmitGood
			return nil
		}
		if rc.Outcome == WordApplied {
			// 稿已录入：径请起功试跑。
			p.stage = stageStart
		}
		return nil
	case stageSubmitGood:
		if rc := p.latestDraftReceipt(); rc != nil && rc.Outcome == WordApplied {
			p.stage = stageWaitStart
			return nil
		}
		// 未录入（首息或修订稿仍被拒）：交当前稿再试。
		return []Intent{{Kind: IntentProtocolDraft, Text: p.draft()}}
	case stageWaitStart:
		if rc := p.latestDraftReceipt(); rc != nil && rc.Outcome == WordApplied {
			p.stage = stageStart
		}
		return nil
	case stageStart:
		if name := draftName(p.draft()); name != "" {
			p.stage = stageRun
			p.runMark = len(p.seenReceipts) // 旧试跑的总结自此不作数
			return []Intent{{Kind: IntentStartProtocol, Text: name}}
		}
		return nil
	case stageRun:
		for i := p.runMark; i < len(p.seenReceipts); i++ {
			if strings.Contains(p.seenReceipts[i].Summary, "试跑已毕") {
				p.stage = stageDone
				return nil
			}
		}
		return nil // 入定候跑：协议自循，静观回执照常回流
	}
	return nil
}

// draftName 从草案文本取功法名（脚本分身的记忆，非世界真值）。
func draftName(yaml string) string {
	for _, line := range strings.Split(yaml, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "protocol:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "protocol:"))
		}
	}
	return ""
}

// runE2E 驱动脚本分身走完整链路：每息唤醒分身（心跳口径），直至分身
// 宣告功成或 tick 预算耗尽。
func runE2E(t *testing.T, s *CultSession, p *draftE2EPilot, maxTicks int) {
	t.Helper()
	for i := 0; i < maxTicks && p.stage != stageDone; i++ {
		s.WakePilotNextTick() // 心跳征询（与 api 层 heartbeatDue 同一入口语义）
		Tick(s, nil)
	}
}

func newE2ESession(t *testing.T, seed uint64, p *draftE2EPilot) *CultSession {
	t.Helper()
	s := NewSession(draftWorld(), seed, p)
	const acuity = 1 // 初见措辞（感知隔离律：观察只出定性词）
	s.Observe = func(x *cult.World) *Observation { return cult.Observe(x, acuity) }
	return s
}

// TestScriptedPilotDraftE2E 是本工作包的主验收：脚本分身走完
// 感知 → 错误草案 → 定性回执 → 修订 → 编译通过 → 试跑 → 固化 全链。
func TestScriptedPilotDraftE2E(t *testing.T) {
	pilot := &draftE2EPilot{drafts: e2eDraftsA}
	s := newE2ESession(t, 109, pilot)
	runE2E(t, s, pilot, 60)

	if pilot.stage != stageDone {
		t.Fatalf("分身未走完全链（停在阶段 %d，tick %d）", pilot.stage, s.World.Tick)
	}

	// 链路各步证据。

	// 1. 感知身体：分身收到过定性观察（可感之处）。
	if !pilot.seenObs || pilot.obsPoints == 0 {
		t.Fatalf("分身应先感知身体：obs=%v points=%d", pilot.seenObs, pilot.obsPoints)
	}

	// 2. 错误草案吃到定性回执：三类错误逐条指路，零数字零机器词。
	var badRC *IntentReceipt
	for i := range pilot.seenReceipts {
		rc := &pilot.seenReceipts[i]
		if rc.Outcome == WordRejected && strings.Contains(rc.Summary, "冒进诀") {
			badRC = rc
		}
	}
	if badRC == nil {
		t.Fatalf("错误草案应吃到拒绝回执：%+v", pilot.seenReceipts)
	}
	for _, want := range []string{"第一段", "尚未辨识", "没有相连的经脉", "唯丹田推得动"} {
		if !strings.Contains(badRC.Detail, want) {
			t.Errorf("定性回执缺指路语 %q：%q", want, badRC.Detail)
		}
	}

	// 3. 修订稿编译通过并起功试跑；4. 总结回执固化。
	startRC := pilot.latestSummary("请起功")
	if startRC == nil || startRC.Outcome != WordApplied {
		t.Fatalf("起功应已生效：%+v", pilot.seenReceipts)
	}
	doneRC := pilot.latestSummary("试跑已毕")
	if doneRC == nil || doneRC.Outcome != WordApplied || !strings.Contains(doneRC.Detail, "固化在册") {
		t.Fatalf("试跑总结应为固化：%+v", doneRC)
	}

	// 5. 世界侧事实：功法转正、入台账。
	p := s.Protocols["周天初环"]
	if p == nil || p.Status != StatusValidated {
		t.Fatalf("功法应达标转正：%+v", p)
	}
	if lastEventOf(s, EventProtocolValidated) == nil {
		t.Error("缺 protocol_validated 事件")
	}
	view := strings.Join(s.Ledger.AgentView(), "\n")
	if !strings.Contains(view, "功法〔已固化〕周天初环") {
		t.Errorf("台账应有固化记录：%s", view)
	}

	// 6. 感知隔离律全链锁：分身收过的每一行回执零数字、零 ASCII。
	for i, rc := range pilot.seenReceipts {
		assertAgentText(t, "E2E 回执流", ReceiptView(rc))
		_ = i
	}
}

// TestScriptedPilotTrialRevisionE2E（场景 B）：第一门草案能跑但周天太少
// 未足为凭——分身依总结回执修订（加多周天）再试，直至达标固化。
func TestScriptedPilotTrialRevisionE2E(t *testing.T) {
	pilot := &draftE2EPilot{drafts: e2eDraftsB}
	s := newE2ESession(t, 113, pilot)

	// 场景 B 的修订链：短稿 → 未足为凭 → 长稿。用两段驱动。
	// 第一段：交短稿、起功、跑完、吃总结回执。
	runE2E(t, s, pilot, 60)
	if pilot.stage != stageDone {
		t.Fatalf("第一门功法应走完试跑（停在阶段 %d）", pilot.stage)
	}
	first := pilot.latestSummary("试跑已毕")
	if first == nil || first.Outcome != WordUnproven || !strings.Contains(first.Detail, "周天太少") {
		t.Fatalf("短稿试跑应未足为凭：%+v", first)
	}
	if p := s.Protocols["周天初环"]; p.Status != StatusDraft {
		t.Fatalf("未足为凭不转正：%s", p.Status)
	}

	// 第二段：依回执修订——同门功法加多周天，重走录入、试跑、固化。
	pilot.seenReceipts = nil
	pilot.stage = stageSubmitGood
	pilot.draftIdx = 1
	runE2E(t, s, pilot, 60)
	if pilot.stage != stageDone {
		t.Fatalf("修订稿应走完全链（停在阶段 %d）", pilot.stage)
	}
	done := pilot.latestSummary("试跑已毕")
	if done == nil || done.Outcome != WordApplied {
		t.Fatalf("修订稿试跑应固化：%+v", pilot.seenReceipts)
	}
	if p := s.Protocols["周天初环"]; p.Status != StatusValidated {
		t.Fatalf("修订稿应转正：%s", p.Status)
	}
	if view := strings.Join(s.Ledger.AgentView(), "\n"); !strings.Contains(view, "功法〔已固化〕周天初环") {
		t.Errorf("台账应有固化记录：%s", view)
	}
	for _, rc := range pilot.seenReceipts {
		assertAgentText(t, "场景 B 回执流", ReceiptView(rc))
	}
}
