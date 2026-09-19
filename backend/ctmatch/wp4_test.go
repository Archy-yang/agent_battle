package ctmatch

// wp4_test.go —— WP-4 验收测试（知识系统 + 意图回执 + Protocol 固化闭环 +
// Loop/筑基判定）：
//
//	 1. 回执：无法落地意图必产出拒绝回执；回执载荷零数字（铁律 6）
//	 2. 证据三态：试过被拒 / 试过且成 / 从未试过（SPIKE 结论二）
//	 3. 假说：record_hypothesis 登记、状态与关联证据
//	 4. 固化闭环：20+ 周天 95% 成功率转正；未达标不转正；出界扩张被拒
//	 5. 同息并行 cadence（SPIKE 结论三）：吸收+引流同 Tick 编排
//	 6. 筑基：满足 stage_3 §37 触达"触碰筑基"、§38 筑基；阈值不足不误报
//	 7. Loop 注册：筑基后 Protocol 升格 AutonomousLoop
//	 8. 确定性：同 seed 同意图流（含知识系统）逐位一致

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"agentbattle/backend/cult"
)

// ---------------------------------------------------------------------------
// 1. 回执（铁律 6）
// ---------------------------------------------------------------------------

// receiptDigits 扫描一批回执视图里的 ASCII 数字。
func receiptDigits(lines []string) string {
	var bad []rune
	for _, l := range lines {
		for _, r := range l {
			if r >= '0' && r <= '9' {
				bad = append(bad, r)
			}
		}
	}
	return string(bad)
}

func TestReceiptsProducedAndZeroDigits(t *testing.T) {
	// 静环境：吸收必然"不入"，制造穴未开或满溢不入的回执。
	w := newTestWorld()
	for e := range w.Env.Qi {
		w.Env.Qi[e] = 0
	}
	s := NewSession(w, 43, nil)
	Tick(s, nil) // 静息，取观测基准

	intents := []Intent{
		{Kind: IntentAbsorb, Acupoint: "不存在", Level: LevelSmall},     // 此路不通
		{Kind: IntentGuide, From: "丹田", To: "阳枢", Level: LevelSmall}, // 此路不通（无经脉）
		{Kind: IntentAbsorb, Acupoint: "青关"},                         // 意图不明（无档位）
		{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall},      // 已生效但不入（环境无气）
		{Kind: IntentSense}, // 已生效
		{Kind: IntentHypothesis, Text: "青关或偏亲木性"}, // 已记下
	}
	res := Tick(s, intents)
	if len(res.Receipts) != len(intents) {
		t.Fatalf("每意图应恰一条回执：got %d want %d", len(res.Receipts), len(intents))
	}

	wantOutcomes := []string{WordNoted, WordRejected, WordRejected, WordUnclear, WordBlocked, WordApplied}
	for i, want := range wantOutcomes {
		if res.Receipts[i].Outcome != want {
			t.Errorf("回执 %d outcome = %q, want %q", i, res.Receipts[i].Outcome, want)
		}
	}
	views := ReceiptViews(res.Receipts)
	if bad := receiptDigits(views); bad != "" {
		t.Fatalf("回执载荷含数字 %q——铁律 6：回执不得出现任何数值", bad)
	}
	// 拒绝类补语词表必须在（假说回执在 Tick 开头先行给出，故下标后移）。
	if !strings.Contains(views[1], WhyNoAcupoint) || !strings.Contains(views[2], WhyNoChannel) {
		t.Errorf("拒绝补语缺失: %q / %q", views[1], views[2])
	}
	if !strings.Contains(views[4], WhyNotTakenIn) {
		t.Errorf("不入补语缺失: %q", views[4])
	}
}

func TestReceiptAppliedWithDirection(t *testing.T) {
	s := newTestSession(t, 47)
	res := Tick(s, []Intent{
		{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall},
		{Kind: IntentGuide, From: "丹田", To: "青关", Element: cult.Wood, Level: LevelMedium},
	})
	if len(res.Receipts) != 2 {
		t.Fatalf("receipts = %d", len(res.Receipts))
	}
	for i, r := range res.Receipts {
		if r.Outcome != WordApplied {
			t.Fatalf("回执 %d 应已生效: %+v", i, r)
		}
		if r.Direction == "" {
			t.Errorf("已生效回执应带压力方向词: %+v", r)
		}
		switch r.Direction {
		case WordPressureUp, WordPressureDown, WordPressureSame:
		default:
			t.Errorf("方向词不在词表: %q", r.Direction)
		}
	}
}

func TestPilotReceivesReceiptsOnWake(t *testing.T) {
	w := newTestWorld()
	pilot := &ScriptedPilot{Script: [][]Intent{{{Kind: IntentStop}}}}
	s := NewSession(w, 53, pilot)
	Tick(s, nil)

	// Tick 1：交出一条无处落地的意图 → 拒绝回执留存为"上一组"。
	Tick(s, []Intent{{Kind: IntentAbsorb, Acupoint: "太乙", Level: LevelSmall}})

	// Tick 2：出界升级（下一 Tick 唤醒 Pilot）。
	yaml := `
protocol: wake_receipt_v1
actions:
  - guide: {from: 丹田, to: 青关, element: wood, level: large}
cadence: 1
cycles: 1
validated_range:
  - channel: [丹田, 青关]
    element: wood
    min: 0
    max: 2000
`
	p, err := Compile([]byte(yaml), &w.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	s.LoadProtocol(p)
	if err := s.StartProtocol(p.Name); err != nil {
		t.Fatalf("start: %v", err)
	}
	if res := Tick(s, nil); !res.Escalated {
		t.Fatalf("expected boundary escalation")
	}

	// Tick 3：唤醒 Pilot——征询载荷携带上一组意图的回执。
	Tick(s, nil)
	if pilot.Calls != 1 {
		t.Fatalf("pilot calls = %d, want 1", pilot.Calls)
	}
	if len(pilot.LastReceipts) != 1 || pilot.LastReceipts[0].Outcome != WordRejected {
		t.Fatalf("pilot 未收到拒绝回执: %+v", pilot.LastReceipts)
	}
	if bad := receiptDigits(ReceiptViews(pilot.LastReceipts)); bad != "" {
		t.Errorf("回执载荷含数字 %q", bad)
	}
}

func TestLLMPilotPayloadCarriesReceiptsAndLedger(t *testing.T) {
	var user string
	srv := newFakeLLM(t, `{"cognition":"","intents":[]}`, func(req map[string]any) {
		msgs := req["messages"].([]any)
		user = msgs[1].(map[string]any)["content"].(string)
	})
	p := fakePilot(t, srv)
	p.GiveReceipts([]IntentReceipt{
		{Summary: "于青关纳气（少量）", Outcome: WordApplied, Direction: WordPressureUp},
		{Summary: "引气自丹田至阳枢", Outcome: WordRejected, Detail: WhyNoChannel},
	})
	p.GiveLedgerView([]string{"于青关纳气（少量）：试过且成，近见压力升", "猜想〔未验证〕青关或偏亲木性（尚无凭据）"})
	p.Decide(fakeObservation())
	if p.LastError != "" {
		t.Fatalf("不应报错: %s", p.LastError)
	}

	// 回执与账本段落必须零数字（观察 JSON 的 band 序号不在本段）。
	seg := user[strings.Index(user, "【你上一组意图的回执】"):strings.Index(user, "【当前的身体感受】")]
	if bad := receiptDigits([]string{seg}); bad != "" {
		t.Errorf("回执/账本载荷含数字 %q：\n%s", bad, seg)
	}
	for _, want := range []string{WordApplied, WordRejected, WordPressureUp, "试过且成", "猜想〔未验证〕"} {
		if !strings.Contains(user, want) {
			t.Errorf("载荷缺要素 %q", want)
		}
	}
	// 修行笔记降级为附注。
	if !strings.Contains(user, "附注") {
		t.Error("修行笔记应标注为附注")
	}
}

// ---------------------------------------------------------------------------
// 2. 证据三态（SPIKE 结论二）
// ---------------------------------------------------------------------------

func TestEvidenceLedgerTriState(t *testing.T) {
	l := NewEvidenceLedger()

	if st := l.StateOf("absorb@青关"); st != NeverTried {
		t.Fatalf("空账本应为从未试过, got %d", st)
	}
	if w := StateWord(NeverTried); w != "从未试过" {
		t.Errorf("StateWord(NeverTried) = %q", w)
	}

	l.Record(EvidenceEntry{Tick: 1, Action: "guide@丹田→阳枢", Summary: "引气自丹田至阳枢", Receipt: WordRejected})
	if st := l.StateOf("guide@丹田→阳枢"); st != TriedRejected {
		t.Errorf("被拒应为试过被拒, got %d", st)
	}
	l.Record(EvidenceEntry{Tick: 2, Action: "absorb@青关", Summary: "于青关纳气", Receipt: WordApplied, Outcome: WordPressureUp})
	if st := l.StateOf("absorb@青关"); st != TriedApplied {
		t.Errorf("已生效应为试过且成, got %d", st)
	}
	// 被"意图不明"打断不改变三态。
	l.Record(EvidenceEntry{Tick: 3, Action: "absorb@青关", Summary: "于青关纳气", Receipt: WordUnclear})
	if st := l.StateOf("absorb@青关"); st != TriedApplied {
		t.Errorf("意图不明不应覆盖三态, got %d", st)
	}
	// 未落地无方向。
	if d := l.lastOutcome("guide@丹田→阳枢"); d != "" {
		t.Errorf("被拒不应有方向词: %q", d)
	}

	view := l.AgentView()
	if bad := receiptDigits(view); bad != "" {
		t.Errorf("账本视图含数字 %q: %v", bad, view)
	}
	joined := strings.Join(view, "\n")
	for _, want := range []string{"试过被拒", "试过且成", "压力升"} {
		if !strings.Contains(joined, want) {
			t.Errorf("账本视图缺 %q: %v", want, view)
		}
	}
}

func TestEvidenceLedgerFromSession(t *testing.T) {
	s := newTestSession(t, 59)
	Tick(s, []Intent{{Kind: IntentGuide, From: "丹田", To: "阳枢", Element: -1, Level: LevelSmall}}) // 被拒
	Tick(s, []Intent{{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall}})                   // 且成

	if st := s.Ledger.StateOf("guide@丹田→阳枢"); st != TriedRejected {
		t.Errorf("会话账本：guide 应试过被拒, got %d", st)
	}
	if st := s.Ledger.StateOf("absorb@青关"); st != TriedApplied {
		t.Errorf("会话账本：absorb 应试过且成, got %d", st)
	}
	if st := s.Ledger.StateOf("absorb@丹田"); st != NeverTried {
		t.Errorf("会话账本：未做过的应为从未试过, got %d", st)
	}
}

// ---------------------------------------------------------------------------
// 3. 假说（stage_2 §26 第六接口 / §28）
// ---------------------------------------------------------------------------

func TestHypothesisIntentLifecycle(t *testing.T) {
	s := newTestSession(t, 61)
	res := Tick(s, []Intent{{Kind: IntentHypothesis, Text: "青关或偏亲木性"}})
	if len(res.Receipts) != 1 || res.Receipts[0].Outcome != WordNoted {
		t.Fatalf("record_hypothesis 应回执已记下: %+v", res.Receipts)
	}
	h := s.Ledger.RecordHypothesis("青关或偏亲木性", 1) // 重记去重
	if len(s.Ledger.Hypotheses) != 1 {
		t.Fatalf("同文重记不应新增: %d", len(s.Ledger.Hypotheses))
	}
	if h.Status != HypUnverified {
		t.Errorf("新假说应未验证: %s", h.Status)
	}

	// 后续证据累计（自登记以来）。
	Tick(s, []Intent{{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall}})
	Tick(s, []Intent{{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall}})
	if h.Evidence < 2 {
		t.Errorf("关联证据数应随账本增长: %d", h.Evidence)
	}

	if !s.Ledger.SetHypothesisStatus("青关或偏亲木性", HypSupported) {
		t.Fatal("SetHypothesisStatus 失败")
	}
	view := strings.Join(s.Ledger.AgentView(), "\n")
	if !strings.Contains(view, "猜想〔支持〕青关或偏亲木性") {
		t.Errorf("账本视图缺假说行: %s", view)
	}
	if bad := receiptDigits([]string{view}); bad != "" {
		t.Errorf("账本视图含数字 %q", bad)
	}
}

// ---------------------------------------------------------------------------
// 4. 固化闭环（stage_2 §29 / stage_3 §28-30）
// ---------------------------------------------------------------------------

// wp4World 是固化闭环/筑基场景的标定世界：穴位全开（传导无损耗）、
// 通道宽裕（首息流量冲击不至失稳）、环境木气稀薄（纳气恰好补足循环
// 损耗）。与 WP-3 的 testWorld 严格分开，互不影响。
func wp4Point(name string, cap, tp int64) cult.Acupoint {
	return cult.Acupoint{
		Name:       name,
		Affinity:   [cult.ElementCount]int64{1200, 800, 900, 750, 1000},
		Capacity:   cap,
		Throughput: tp,
		Retention:  400,
		Openness:   cult.MicroOne, // 全开：传导因子 1.0，送达 = 实际流量
		Stability:  950,
		Activity:   [cult.ElementCount]int64{1000, 1000, 1000, 1000, 1000},
	}
}

func wp4World() *cult.World {
	char := cult.Character{
		Affinity:   [cult.ElementCount]int64{1250, 800, 900, 750, 1100},
		Control:    700,
		Perception: 500,
		BodyGraph: cult.BodyGraph{
			Acupoints: []cult.Acupoint{
				wp4Point("丹田", 20000, 20000), // 宽吞吐：首息冲击不至失稳
				wp4Point("青关", 10000, 6000),
				wp4Point("阳枢", 5000, 2000),
			},
			Channels: []cult.Channel{
				{Name: "任脉下行", From: 0, To: 1, BaseConductance: 900, Throughput: 4000},
				{Name: "任脉上行", From: 1, To: 0, BaseConductance: 900, Throughput: 4000},
			},
		},
	}
	env := cult.Environment{
		Qi:          [cult.ElementCount]int64{3000, 200, 200, 200, 200}, // 木气稀薄
		Activity:    [cult.ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Temperature: 1000,
	}
	w := cult.NewWorld(char, env)
	w.Character.BodyGraph.Acupoints[0].Qi[cult.Wood] = 20000
	return w
}

func wp4Session(t *testing.T, seed uint64) *CultSession {
	t.Helper()
	s := NewSession(wp4World(), seed, nil)
	Tick(s, nil) // 静息，取观测基准
	return s
}

// parallelLoopYAML：同息并行闭环功法——纳气、进引、出引同一息编排
// （节律恒定 → 流量冲击趋零，供长试运行达标）。
const parallelLoopYAML = `
protocol: huantian_parallel_v1
actions:
  - absorb: {target: 青关, level: trace, duration: 1, parallel: 1}
  - guide: {from: 丹田, to: 青关, element: wood, amount: 3000, duration: 1, parallel: 1}
  - guide: {from: 青关, to: 丹田, element: wood, amount: 3000, duration: 1, parallel: 1}
cadence: 1
cycles: 25
validated_range:
  - channel: [丹田, 青关]
    element: wood
    min: 0
    max: 3000
  - channel: [青关, 丹田]
    element: wood
    min: 0
    max: 3000
`

func runParallelLoop(t *testing.T, cycles int) (*CultSession, *Protocol) {
	t.Helper()
	s := wp4Session(t, 67)
	yaml := strings.Replace(parallelLoopYAML, "cycles: 25", "cycles: "+itoa(cycles), 1)
	p, err := Compile([]byte(yaml), &s.World.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	s.LoadProtocol(p)
	if err := s.StartProtocol(p.Name); err != nil {
		t.Fatalf("start: %v", err)
	}
	for i := 0; i < cycles; i++ { // 一息一周天
		res := Tick(s, nil)
		if !res.Advanced {
			t.Fatalf("tick %d: engine not advanced (events=%+v)", i, s.Events)
		}
	}
	return s, p
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func TestProtocolValidationPromotion(t *testing.T) {
	s, p := runParallelLoop(t, 25)

	if p.Status != StatusValidated {
		t.Fatalf("25 周天全成应转正，status = %s；trial=%+v", p.Status, s.Trial.Record)
	}
	e := lastEventOf(s, EventProtocolValidated)
	if e == nil {
		t.Fatal("缺 protocol_validated 事件")
	}
	sum := s.Trial.Finish()
	if sum.SuccessfulCycles < ValidateMinCycles || sum.SuccessRate < ValidateMinRate || sum.AvgMinStability < ValidateMinAvgStab {
		t.Errorf("达标判定错误: %+v", sum)
	}
	for i, c := range s.Trial.Record.Cycles {
		if !c.Success {
			t.Errorf("周天 %d 意外失败: %+v", i, c)
		}
	}
	if len(s.Trial.Record.Cycles) != 25 {
		t.Errorf("逐周天记录数 = %d, want 25", len(s.Trial.Record.Cycles))
	}
}

func TestProtocolValidationNotPromoted(t *testing.T) {
	s, p := runParallelLoop(t, 10)
	if p.Status == StatusValidated {
		t.Fatalf("10 周天不应转正（successful=%d < %d）", s.Trial.Finish().SuccessfulCycles, ValidateMinCycles)
	}
	if lastEventOf(s, EventProtocolValidated) != nil {
		t.Error("不应有 protocol_validated 事件")
	}
	if sum := s.Trial.Finish(); sum.Qualified {
		t.Errorf("10 周天不应判定达标: %+v", sum)
	}
	_ = s
}

func TestExpandRange(t *testing.T) {
	s, p := runParallelLoop(t, 25)
	if p.Status != StatusValidated {
		t.Fatalf("测试前提：功法应已转正")
	}

	// 未转正的功法不得扩张。
	draft, err := Compile([]byte(validYAML), &s.World.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile draft: %v", err)
	}
	if err := s.ExpandRange(draft, 0, cult.Wood, 0, 2500); err == nil {
		t.Error("draft 功法扩张应被拒")
	}

	// 一步之内允许外扩。
	if err := s.ExpandRange(p, 0, cult.Wood, 0, 3400); err != nil {
		t.Fatalf("一步外扩被拒: %v", err)
	}
	if r := p.FindRange(0, cult.Wood); r.Max != 3400 {
		t.Errorf("扩张未生效: max=%d", r.Max)
	}

	// 出界扩张被拒（一次超过 RangeExpandStep）。
	if err := s.ExpandRange(p, 0, cult.Wood, 0, 4000); err == nil {
		t.Error("超过一步的外扩应被拒")
	}
	if r := p.FindRange(0, cult.Wood); r.Max != 3400 {
		t.Errorf("被拒的扩张不应生效: max=%d", r.Max)
	}
	if err := s.ExpandRange(p, 0, cult.Wood, 0, 0); err == nil {
		t.Error("max < min 应被拒")
	}
	// 无区间的通道不可扩张。
	if err := s.ExpandRange(p, 0, cult.Fire, 0, 100); err == nil {
		t.Error("无记录元素扩张应被拒")
	}
}

// ---------------------------------------------------------------------------
// 5. 同息并行 cadence（SPIKE 结论三）
// ---------------------------------------------------------------------------

func TestParallelCadence(t *testing.T) {
	s := newTestSession(t, 71)
	p, err := Compile([]byte(parallelLoopYAML), &s.World.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	// 三个步骤同组号相邻 → 单槽同息。
	if len(p.Slots) != 1 || len(p.Slots[0]) != 3 {
		t.Fatalf("slots = %+v, want 单槽三步", p.Slots)
	}
	s.LoadProtocol(p)
	if err := s.StartProtocol(p.Name); err != nil {
		t.Fatalf("start: %v", err)
	}

	res := Tick(s, nil)
	if !res.Advanced {
		t.Fatalf("engine not advanced: %+v", res.Events)
	}
	// 同一 Tick 内：青关既有进气（吸收）又有出气（引导）——纳引同息。
	if res.Report.Points[1].Intake[cult.Wood] <= 0 {
		t.Error("并行槽内吸收未生效")
	}
	found := false
	for _, tr := range res.Report.Transfers {
		if tr.Channel == 0 && tr.Actual[cult.Wood] > 0 {
			found = true
		}
	}
	if !found {
		t.Error("并行槽内引导未生效")
	}

	// 组内时长不一致必须拒绝编译。
	bad := `
protocol: bad_parallel_v1
actions:
  - absorb: {target: 青关, level: trace, duration: 1, parallel: 1}
  - guide: {from: 丹田, to: 青关, element: wood, level: large, duration: 2, parallel: 1}
cadence: 1
cycles: 1
`
	if _, err := Compile([]byte(bad), &s.World.Character.BodyGraph); err == nil {
		t.Error("并行组时长不一致应被拒绝编译")
	}
}

func TestTopoClosure(t *testing.T) {
	s := newTestSession(t, 73)
	closed, err := Compile([]byte(parallelLoopYAML), &s.World.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if got := topoClosure(closed); got != One {
		t.Errorf("闭环功法 topoClosure = %d, want %d", got, One)
	}
	open, err := Compile([]byte(validYAML), &s.World.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if got := topoClosure(open); got != 0 {
		t.Errorf("非闭环功法 topoClosure = %d, want 0", got)
	}
}

// ---------------------------------------------------------------------------
// 6. 筑基判定（stage_3 §37/§38，stage_2 §20 叙事）
// ---------------------------------------------------------------------------

// foundationGoodSample 构造满足 stage_3 §38（从而也满足 §37）的样本：
// 拓扑闭合、归返充足、五行与能量平稳、无人推动而气流自存、稳定性充足。
func foundationGoodSample(tick int64) FoundationSample {
	return FoundationSample{
		Tick:          tick,
		TopoClosure:   One,
		Delivered:     1000,
		ActualFlow:    1000,
		TotalQi:       20000,
		MeanPressure:  300,
		MinStability:  900,
		NatureRatio:   [cult.ElementCount]int64{200, 200, 200, 200, 200},
		ControlActive: false,
	}
}

func TestFoundationTouchAndEstablish(t *testing.T) {
	m := NewFoundationMonitor()
	touchedAt, establishedAt := int64(-1), int64(-1)
	for i := int64(1); i <= 200; i++ {
		touched, established := m.Observe(foundationGoodSample(i))
		if touched && touchedAt < 0 {
			touchedAt = i
		}
		if established && establishedAt < 0 {
			establishedAt = i
		}
	}
	if touchedAt != CandidateTicks {
		t.Errorf("触碰筑基应恰在第 %d Tick, got %d", CandidateTicks, touchedAt)
	}
	if establishedAt != StableTicks {
		t.Errorf("筑基应恰在第 %d Tick, got %d", StableTicks, establishedAt)
	}
	mt := m.Metrics()
	if mt.Closure < FoundClosure || mt.RuleBalance < FoundRuleBalance ||
		mt.SelfSustain < FoundSelfSustain || mt.DynamicStability < FoundDynStability ||
		mt.Internalization < FoundInternalization || mt.MinStability < FoundMinStability {
		t.Errorf("§38 五量应全部达标: %+v", mt)
	}
}

func TestFoundationNoFalsePositive(t *testing.T) {
	// 稳定性不足（§37 MinimumStability < 0.70）：永不误报。
	m := NewFoundationMonitor()
	for i := int64(1); i <= 300; i++ {
		s := foundationGoodSample(i)
		s.MinStability = 600
		if touched, established := m.Observe(s); touched || established {
			t.Fatalf("稳定性不足不应触门槛 (tick %d)", i)
		}
	}

	// 能量失衡（RuleBalance 不足）：总量大幅振荡（一路放大又一路溃散），
	// 永不误报。
	m2 := NewFoundationMonitor()
	for i := int64(1); i <= 300; i++ {
		s := foundationGoodSample(i)
		if i%2 == 1 {
			s.TotalQi = 38000
		} else {
			s.TotalQi = 20000
		}
		if touched, established := m2.Observe(s); touched || established {
			t.Fatalf("能量失衡不应触门槛 (tick %d)", i)
		}
	}

	// 主动推动从未松手（SelfSustain 无从观测）：永不误报。
	m3 := NewFoundationMonitor()
	for i := int64(1); i <= 300; i++ {
		s := foundationGoodSample(i)
		s.ControlActive = true
		if touched, established := m3.Observe(s); touched || established {
			t.Fatalf("未松手不应触门槛 (tick %d)", i)
		}
	}
}

func TestFoundationSessionNoFalsePositive(t *testing.T) {
	// 真实引擎里跑一个稳定的闭环功法：归返不足（FlowClosure < 0.8），
	// 不得误报筑基。
	s, p := runParallelLoop(t, 25)
	for i := 0; i < 20; i++ {
		Tick(s, nil)
	}
	if lastEventOf(s, EventFoundationTouched) != nil {
		t.Errorf("真实场景误报触碰筑基: metrics=%+v", s.Foundation.Metrics())
	}
	if s.Foundation.Touched {
		t.Errorf("FoundationMonitor 不应置位 Touched: %+v", s.Foundation.Metrics())
	}
	_ = p
}

// ---------------------------------------------------------------------------
// 7. Loop 注册（stage_3 §39）
// ---------------------------------------------------------------------------

func TestFoundationRegistersAutonomousLoop(t *testing.T) {
	s := wp4Session(t, 79)
	p, err := Compile([]byte(parallelLoopYAML), &s.World.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	s.LoadProtocol(p)
	s.lastProtocol = p // 模拟曾跑过此功法

	// 直接把判定器推到筑基（§37 30 Tick + §38 100 Tick）。
	for i := int64(1); !s.Foundation.Established; i++ {
		s.Foundation.Observe(foundationGoodSample(i))
		if i > 500 {
			t.Fatal("判定器未达筑基")
		}
	}
	Tick(s, nil) // 会话层发现已筑基 → 升格 AutonomousLoop

	if lastEventOf(s, EventFoundationEstablished) == nil {
		t.Fatal("缺 foundation_established 事件")
	}
	if lastEventOf(s, EventLoopRegistered) == nil {
		t.Fatal("缺 loop_registered 事件")
	}
	if s.ActiveProtocol() != p.Name {
		t.Fatalf("Loop 未接管: active=%q", s.ActiveProtocol())
	}
	if !s.active.loopMode {
		t.Error("loopMode 未置位")
	}
	// 周天无上限：远超 Cycles=25 仍自推。
	for i := 0; i < 40; i++ {
		res := Tick(s, nil)
		if !res.Advanced {
			t.Fatalf("loop tick %d 未推进: %+v", i, res.Events)
		}
	}
	if s.ActiveProtocol() != p.Name {
		t.Error("Loop 不应停止")
	}
	if lastEventOf(s, EventProtocolFinished) != nil {
		t.Error("Loop 不应有 protocol_finished 事件")
	}
}

func TestRegisterLoopManual(t *testing.T) {
	s := newTestSession(t, 83)
	short, err := Compile([]byte(`
protocol: short_v1
actions:
  - absorb: {target: 青关, level: trace, duration: 1}
cadence: 1
cycles: 2
`), &s.World.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	s.LoadProtocol(short)
	s.RegisterLoop(short)
	if lastEventOf(s, EventLoopRegistered) == nil {
		t.Fatal("缺 loop_registered 事件")
	}
	for i := 0; i < 5; i++ { // 远超 cycles=2，Loop 仍自推
		res := Tick(s, nil)
		if !res.Advanced {
			t.Fatalf("loop tick %d 未推进", i)
		}
	}
	if s.ActiveProtocol() != short.Name {
		t.Error("Loop 不应停止")
	}
}

// ---------------------------------------------------------------------------
// 8. 确定性：同 seed 同意图流（含知识系统）逐位一致
// ---------------------------------------------------------------------------

func TestDeterminismWithKnowledgeSystem(t *testing.T) {
	script := [][]Intent{
		{{Kind: IntentSense}},
		{{Kind: IntentHypothesis, Text: "青关或偏亲木性"}},
		{{Kind: IntentAbsorb, Acupoint: "不存在", Level: LevelSmall}}, // 拒绝回执
		{{Kind: IntentAbsorb, Acupoint: "青关", Level: LevelSmall}},
		{{Kind: IntentGuide, From: "丹田", To: "青关", Element: cult.Wood, Level: LevelMedium}},
		{{Kind: IntentGuide, From: "丹田", To: "阳枢", Element: -1, Level: LevelSmall}}, // 被拒
		{{Kind: IntentHypothesis, Text: "纳气后青关压力升"}},
		{{Kind: IntentStop}},
		nil, nil, nil, nil,
	}

	type run struct {
		world          *cult.World
		reports        [][]byte
		receipts       [][]IntentReceipt
		ledger         []string
		foundationLoop LoopMetrics
		intentLog      []IntentLogEntry
	}
	do := func() *run {
		s := NewSession(newTestWorld(), 4242, nil)
		s.Observe = func(x *cult.World) *Observation { return cult.Observe(x, 1) }
		r := &run{}
		for _, intents := range script {
			res := Tick(s, intents)
			b, err := json.Marshal(res.Report)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			r.reports = append(r.reports, b)
			r.receipts = append(r.receipts, res.Receipts)
		}
		r.world = s.World
		r.ledger = s.LedgerView()
		r.foundationLoop = s.Foundation.Metrics()
		r.intentLog = s.IntentLog
		return r
	}

	r1, r2 := do(), do()
	if !reflect.DeepEqual(r1.world, r2.world) {
		t.Fatal("worlds diverge (incl. unexported state)")
	}
	for i := range r1.reports {
		if string(r1.reports[i]) != string(r2.reports[i]) {
			t.Fatalf("tick %d reports diverge byte-wise", i)
		}
		if !reflect.DeepEqual(r1.receipts[i], r2.receipts[i]) {
			t.Fatalf("tick %d receipts diverge", i)
		}
	}
	if !reflect.DeepEqual(r1.ledger, r2.ledger) {
		t.Fatalf("ledger views diverge: %v vs %v", r1.ledger, r2.ledger)
	}
	if !reflect.DeepEqual(r1.foundationLoop, r2.foundationLoop) {
		t.Fatalf("foundation metrics diverge: %+v vs %+v", r1.foundationLoop, r2.foundationLoop)
	}
	if !reflect.DeepEqual(r1.intentLog, r2.intentLog) {
		t.Fatal("intent logs diverge")
	}
}
