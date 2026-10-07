package ctmatch

// draft_test.go —— 功法编译端到端（M2 WP-M2-2）单元验收：
//
//	1. 定性回执：每类编译错误的草案回执零数字、零 ASCII（机器词），
//	   且措辞可指导修订（指明哪一段、错在何处）
//	2. 机读口径不回归：CompileError.Error() 保持既有措辞（转录/测试锁定）
//	3. 草案录入与起功：档位词界限（max_level）编译期经既有换算定格
//	4. 固化语义：试跑达标转正即固化在册（进台账）；已固化功法不可被
//	   同名草案覆盖
//	5. 试跑总结回执：未达标定性告知原因并唤醒分身
//	6. 确定性与移交门衔接：草案路径编译的功法与直编路径逐位一致；
//	   loopMode 移交门语义不受草案路径影响

import (
	"strings"
	"testing"

	"agentbattle/backend/cult"
)

// draftWorld 在 wp4 标定世界上加一条单行经脉 丹田→阳枢（阳维），
// 供「无通路」与「逆流」两类编译错误构造；其余参数与 wp4World 相同
// （环功法可试跑达标）。
func draftWorld() *cult.World {
	w := wp4World()
	g := &w.Character.BodyGraph
	g.Channels = append(g.Channels,
		cult.Channel{Name: "阳维", From: 0, To: 2, BaseConductance: 900, Throughput: 4000})
	return w
}

func draftSession(t *testing.T, seed uint64) *CultSession {
	t.Helper()
	s := NewSession(draftWorld(), seed, nil)
	Tick(s, nil) // 静息，取观测基准
	return s
}

// submitDraft 经会话入口提交一份草案，返回其回执。
func submitDraft(t *testing.T, s *CultSession, yaml string) IntentReceipt {
	t.Helper()
	res := Tick(s, []Intent{{Kind: IntentProtocolDraft, Text: yaml}})
	if len(res.Receipts) == 0 {
		t.Fatalf("草案提交应恰有一条回执")
	}
	return res.Receipts[0]
}

// assertAgentText 锁感知隔离律：定性回执行零数字、零 ASCII 字母（机器词
// 必然带字母；测试用中文穴位名，故定性行内不应出现任何 ASCII）。
func assertAgentText(t *testing.T, where, line string) {
	t.Helper()
	for _, r := range line {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			t.Fatalf("%s：定性回执含数字/机器词字符 %q（全文：%s）", where, string(r), line)
		}
	}
}

// ---------------------------------------------------------------------------
// 1. 定性回执：每类编译错误
// ---------------------------------------------------------------------------

// draftErrCase 是一类编译错误：草案、回执中必须出现的定性指路片段。
type draftErrCase struct {
	name string
	yaml string
	want string // 定性回执必须包含的措辞（全部零数字零 ASCII）
}

var draftErrCases = []draftErrCase{
	{"无名", `
protocol: ""
actions:
  - absorb: {target: 青关, level: small}
`, "还没有名字"},
	{"无动作", `
protocol: 空架子
`, "没有一段可行的行功"},
	{"纳气落点未辨识", `
protocol: 错落点
actions:
  - absorb: {target: 玄府, level: small}
`, "尚未辨识"},
	{"引气来处未辨识", `
protocol: 错来处
actions:
  - guide: {from: 玄府, to: 丹田, level: small}
`, "尚未辨识"},
	{"两穴无通路", `
protocol: 断路诀
actions:
  - guide: {from: 青关, to: 阳枢, level: small}
`, "没有相连的经脉"},
	{"逆流非丹田发起", `
protocol: 强逆诀
actions:
  - guide: {from: 阳枢, to: 丹田, level: small}
`, "唯丹田推得动"},
	{"五行不识", `
protocol: 乱行诀
actions:
  - guide: {from: 丹田, to: 青关, element: 光, level: small}
`, "五行之列"},
	{"纳气未说明力度", `
protocol: 无力诀
actions:
  - absorb: {target: 青关}
`, "总得说明一份"},
	{"引气未说明力度", `
protocol: 无流诀
actions:
  - guide: {from: 丹田, to: 青关}
`, "总得说明一份"},
	{"力度与定数并举", `
protocol: 两全诀
actions:
  - absorb: {target: 青关, level: small, intensity: 300}
`, "只可写其一"},
	{"一段纳引并举", `
protocol: 兼差诀
actions:
  - absorb: {target: 青关, level: small}
    guide: {from: 丹田, to: 青关, level: small}
`, "只做得一件事"},
	{"息数为负", `
protocol: 倒息诀
actions:
  - absorb: {target: 青关, level: small, duration: -1}
`, "不该是负数"},
	{"档位词不识", `
protocol: 怪档诀
actions:
  - absorb: {target: 青关, level: huge}
`, "四档之内"},
	{"同息并行息数不一", `
protocol: 乱拍诀
actions:
  - absorb: {target: 青关, level: trace, duration: 1, parallel: 1}
  - guide: {from: 丹田, to: 青关, level: small, duration: 2, parallel: 1}
`, "同起同息"},
	{"护卫处置不识", `
protocol: 怪卫诀
actions:
  - absorb: {target: 青关, level: small}
guards:
  - when: {point: 青关, metric: pressure, op: ">=", value: 900}
    do: explode
`, "「即止」与「缓行」"},
	{"护卫所察不识", `
protocol: 怪察诀
actions:
  - absorb: {target: 青关, level: small}
guards:
  - when: {point: 青关, metric: mood, op: ">=", value: 900}
    do: stop
`, "无从察起"},
	{"起功条件穴位未辨识", `
protocol: 怪条诀
preconditions:
  - point: 玄府
    metric: stability
    op: ">="
    value: 900
actions:
  - absorb: {target: 青关, level: small}
`, "起功条件有误"},
	{"界限穴位未辨识", `
protocol: 怪界诀
actions:
  - guide: {from: 丹田, to: 青关, level: small}
validated_range:
  - channel: [玄府, 丹田]
    max_level: large
`, "尚未辨识"},
	{"界限无通路", `
protocol: 界断诀
actions:
  - guide: {from: 丹田, to: 青关, level: small}
validated_range:
  - channel: [青关, 阳枢]
    max_level: large
`, "并没有相连的经脉"},
	{"界限五行不识", `
protocol: 界乱诀
actions:
  - guide: {from: 丹田, to: 青关, level: small}
validated_range:
  - channel: [丹田, 青关]
    element: 光
    max_level: large
`, "五行之列"},
	{"界限上下颠倒", `
protocol: 界倒诀
actions:
  - guide: {from: 丹田, to: 青关, level: small}
validated_range:
  - channel: [丹田, 青关]
    min: 5000
    max: 100
`, "下限高过了上限"},
	{"界限定数与档位并举", `
protocol: 界双诀
actions:
  - guide: {from: 丹田, to: 青关, level: small}
validated_range:
  - channel: [丹田, 青关]
    max: 2000
    max_level: large
`, "只可写其一"},
	{"界限档位词不识", `
protocol: 界怪档诀
actions:
  - guide: {from: 丹田, to: 青关, level: small}
validated_range:
  - channel: [丹田, 青关]
    max_level: huge
`, "四档之内"},
	{"字句读不懂", `
protocol: [破架
`, "没能读懂"},
}

func TestDraftRejectionReceiptsQualitative(t *testing.T) {
	s := draftSession(t, 73)
	for _, tc := range draftErrCases {
		rc := submitDraft(t, s, tc.yaml)
		if rc.Outcome != WordRejected {
			t.Errorf("%s：应被拒（%s），got %q（%s）", tc.name, WordRejected, rc.Outcome, rc.Detail)
			continue
		}
		view := ReceiptView(rc)
		assertAgentText(t, tc.name, view)
		if !strings.Contains(rc.Detail, tc.want) {
			t.Errorf("%s：回执应含定性指路语 %q，got %q", tc.name, tc.want, rc.Detail)
		}
		if !strings.Contains(rc.Summary, "功法草案") {
			t.Errorf("%s：回执摘要应指明是草案：%q", tc.name, rc.Summary)
		}
	}
	// 被拒的草案一律不入功法册。
	if len(s.Protocols) != 0 {
		t.Errorf("被拒草案不应入册：%v", s.Protocols)
	}
}

// TestDraftRejectionCitesSegment：多段草案出错时，回执按"第几段"指路
// （任务口径示例：「第三段引用了你尚未辨识的穴位」）。
func TestDraftRejectionCitesSegment(t *testing.T) {
	s := draftSession(t, 79)
	yaml := `
protocol: 三错诀
actions:
  - absorb: {target: 青关, level: small}
  - guide: {from: 丹田, to: 青关, level: small}
  - guide: {from: 青关, to: 玄府, level: small}
`
	rc := submitDraft(t, s, yaml)
	if rc.Outcome != WordRejected {
		t.Fatalf("应被拒: %+v", rc)
	}
	if !strings.Contains(rc.Detail, "第三段") || !strings.Contains(rc.Detail, "尚未辨识") {
		t.Errorf("回执应指明第三段引用未辨识穴位：%q", rc.Detail)
	}
	assertAgentText(t, "三错诀", ReceiptView(rc))
}

// TestDraftEmptyText：空草案给"意图不明"回执。
func TestDraftEmptyText(t *testing.T) {
	s := draftSession(t, 83)
	rc := submitDraft(t, s, "   ")
	if rc.Outcome != WordUnclear {
		t.Fatalf("空草案应意图不明: %+v", rc)
	}
	assertAgentText(t, "空草案", ReceiptView(rc))
}

// ---------------------------------------------------------------------------
// 2. 机读口径不回归（CompileError.Error() 保持既有措辞）
// ---------------------------------------------------------------------------

func TestCompileErrorMachineTextStable(t *testing.T) {
	g := &draftWorld().Character.BodyGraph
	badRev := `
protocol: bad_reverse_v1
actions:
  - guide: {from: 阳枢, to: 丹田, element: wood, level: small}
`
	_, err := Compile([]byte(badRev), g)
	if err == nil || !strings.Contains(err.Error(), "core point") {
		t.Errorf("逆流资格机读措辞不回归: %v", err)
	}
	badEdge := `
protocol: bad_edge_v1
actions:
  - guide: {from: 青关, to: 阳枢, element: wood, level: small}
`
	_, err = Compile([]byte(badEdge), g)
	if err == nil || !strings.Contains(err.Error(), "path not connected") {
		t.Errorf("路径不连通机读措辞不回归: %v", err)
	}
	var ce *CompileError
	if !asCompileError(err, &ce) || len(ce.Agent) == 0 {
		t.Fatalf("编译错误应携带定性行: %v", err)
	}
	assertAgentText(t, "机读对照", strings.Join(ce.Agent, "；"))
}

func asCompileError(err error, target **CompileError) bool {
	if e, ok := err.(*CompileError); ok {
		*target = e
		return true
	}
	return false
}

// ---------------------------------------------------------------------------
// 3. 草案录入与起功（档位词界限）
// ---------------------------------------------------------------------------

// goodDraftYAML 是分身表象可达的合法草案：全程只有档位词与寻常计数，
// 界限（validated_range）用力度档表达（M2 WP-M2-2：分身无需捏造世界之数）。
const goodDraftYAML = `
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

func TestDraftAcceptedThenStarted(t *testing.T) {
	s := draftSession(t, 89)

	rc := submitDraft(t, s, goodDraftYAML)
	if rc.Outcome != WordApplied {
		t.Fatalf("合法草案应录入: %+v", rc)
	}
	assertAgentText(t, "录入回执", ReceiptView(rc))
	p, ok := s.Protocols["周天初环"]
	if !ok {
		t.Fatal("草案应入功法册")
	}
	if p.Status != StatusDraft {
		t.Errorf("录入时应为草创: %s", p.Status)
	}
	// max_level: large 经既有档位换算定格（两条经脉有效通量同为 4000）。
	wantMax := FlowForLevel(LevelLarge, &s.World.Character.BodyGraph, 0, 1, 0)
	if r := p.FindRange(0, cult.Wood); r.Max != wantMax || r.Max == 0 {
		t.Errorf("档位界限应定格为档位换算值: max=%d want=%d", r.Max, wantMax)
	}

	// 起功试跑。
	res := Tick(s, []Intent{{Kind: IntentStartProtocol, Text: "周天初环"}})
	if len(res.Receipts) == 0 || res.Receipts[0].Outcome != WordApplied {
		t.Fatalf("起功应已生效: %+v", res.Receipts)
	}
	if s.ActiveProtocol() != "周天初环" {
		t.Fatalf("起功后应有活跃功法: %q", s.ActiveProtocol())
	}
	assertAgentText(t, "起功回执", ReceiptView(res.Receipts[0]))
}

func TestStartProtocolIntentErrors(t *testing.T) {
	s := NewSession(draftWorld(), 97, nil) // 未静息：无观测基准

	// 未指明功法名。
	res := Tick(s, []Intent{{Kind: IntentStartProtocol, Text: "  "}})
	if res.Receipts[0].Outcome != WordUnclear {
		t.Errorf("未指明名号应意图不明: %+v", res.Receipts[0])
	}
	// 功法册里没有。
	res = Tick(s, []Intent{{Kind: IntentStartProtocol, Text: "无此功法"}})
	if res.Receipts[0].Outcome != WordRejected || !strings.Contains(res.Receipts[0].Detail, "没有这份功法") {
		t.Errorf("无名功法应被拒: %+v", res.Receipts[0])
	}
	// 尚无静观为凭：未经任何一息推进的会话，起功被拒（前置条件需观测基准）。
	s2 := NewSession(draftWorld(), 99, nil)
	p2, err := Compile([]byte(goodDraftYAML), &s2.World.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	s2.LoadProtocol(p2)
	res = Tick(s2, []Intent{{Kind: IntentStartProtocol, Text: "周天初环"}})
	if res.Receipts[0].Outcome != WordRejected || !strings.Contains(res.Receipts[0].Detail, "静观为凭") {
		t.Errorf("无观测基准起功应被拒: %+v", res.Receipts[0])
	}
	assertAgentText(t, "起功拒绝", ReceiptView(res.Receipts[0]))
}

// ---------------------------------------------------------------------------
// 4. 固化语义
// ---------------------------------------------------------------------------

// runDraftTrial 提交草案、起功、跑满周天数，返回会话。
func runDraftTrial(t *testing.T, s *CultSession, yaml string, cycles int) IntentReceipt {
	t.Helper()
	if rc := submitDraft(t, s, yaml); rc.Outcome != WordApplied {
		t.Fatalf("草案应录入: %+v", rc)
	}
	res := Tick(s, []Intent{{Kind: IntentStartProtocol, Text: "周天初环"}})
	if res.Receipts[0].Outcome != WordApplied {
		t.Fatalf("起功应已生效: %+v", res.Receipts[0])
	}
	var last IntentReceipt
	for i := 0; i < cycles; i++ {
		r := Tick(s, nil)
		for _, rc := range r.Receipts {
			if strings.Contains(rc.Summary, "试跑已毕") {
				last = rc
			}
		}
	}
	return last
}

func TestTrialQualifiedSolidifies(t *testing.T) {
	s := draftSession(t, 101)
	last := runDraftTrial(t, s, goodDraftYAML, 25)
	if last.Summary == "" {
		t.Fatal("试跑行满应有总结回执")
	}
	if last.Outcome != WordApplied || !strings.Contains(last.Detail, "固化在册") {
		t.Errorf("达标试跑总结应为固化: %+v", last)
	}
	assertAgentText(t, "固化总结", ReceiptView(last))
	p := s.Protocols["周天初环"]
	if p.Status != StatusValidated {
		t.Fatalf("达标应转正: %s", p.Status)
	}
	if lastEventOf(s, EventProtocolValidated) == nil {
		t.Error("缺 protocol_validated 事件")
	}
	view := strings.Join(s.Ledger.AgentView(), "\n")
	if !strings.Contains(view, "功法〔已固化〕周天初环") {
		t.Errorf("台账应有固化记录: %s", view)
	}
	assertAgentText(t, "台账", view)

	// 固化后同名草案不可覆盖；另起新名可以。
	res := Tick(s, []Intent{{Kind: IntentProtocolDraft, Text: goodDraftYAML}})
	if res.Receipts[0].Outcome != WordRejected || !strings.Contains(res.Receipts[0].Detail, "已固化在册") {
		t.Errorf("已固化功法不可被同名草案覆盖: %+v", res.Receipts[0])
	}
	revised := strings.Replace(goodDraftYAML, "周天初环", "周天再环", 1)
	res = Tick(s, []Intent{{Kind: IntentProtocolDraft, Text: revised}})
	if res.Receipts[0].Outcome != WordApplied {
		t.Errorf("另起新名的草案应可录入: %+v", res.Receipts[0])
	}
}

func TestTrialUnprovenConclusionWakesPilot(t *testing.T) {
	s := draftSession(t, 103)
	pilot := &ScriptedPilot{} // 只为承接回执（脚本恒空，不交意图）
	s.Pilot = pilot
	short := strings.Replace(goodDraftYAML, "cycles: 25", "cycles: 5", 1)
	last := runDraftTrial(t, s, short, 5)
	if last.Outcome != WordUnproven {
		t.Fatalf("周天太少应未足为凭: %+v", last)
	}
	if !strings.Contains(last.Detail, "周天太少") {
		t.Errorf("总结回执应定性告知原因: %+v", last)
	}
	assertAgentText(t, "未足为凭", ReceiptView(last))
	if p := s.Protocols["周天初环"]; p.Status != StatusDraft {
		t.Errorf("未达标不转正: %s", p.Status)
	}
	// 总结回执既已产生，分身于其后第一个唤醒时机收到它（试跑已毕 →
	// 唤醒分身定夺固化或修订；回执已随 keepReceipts 留存为"上一组"）。
	Tick(s, nil)
	if len(pilot.LastReceipts) == 0 || !strings.Contains(pilot.LastReceipts[len(pilot.LastReceipts)-1].Summary, "试跑已毕") {
		t.Fatalf("分身应收到试跑总结回执: %+v", pilot.LastReceipts)
	}
	for _, v := range ReceiptViews(pilot.LastReceipts) {
		assertAgentText(t, "唤醒载荷", v)
	}
}

// ---------------------------------------------------------------------------
// 5. 确定性与移交门衔接
// ---------------------------------------------------------------------------

// TestDraftPathDeterministic：同一份功法，直编路径与草案路径产出的会话
// 推进逐位一致——草案提交只是录入通道，不触碰世界语义。
func TestDraftPathDeterministic(t *testing.T) {
	direct := draftSession(t, 107)
	p, err := Compile([]byte(goodDraftYAML), &direct.World.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	direct.LoadProtocol(p)
	Tick(direct, nil) // 与草案路径的"提交息"对齐（草案提交本身占用一息）
	direct.StartProtocol(p.Name)

	byDraft := draftSession(t, 107)
	if rc := submitDraft(t, byDraft, goodDraftYAML); rc.Outcome != WordApplied {
		t.Fatalf("草案应录入: %+v", rc)
	}
	byDraft.StartProtocol("周天初环")

	var ra, rb []*cult.TickReport
	for i := 0; i < 8; i++ {
		res := Tick(direct, nil)
		ra = append(ra, res.Report)
		res = Tick(byDraft, nil)
		rb = append(rb, res.Report)
	}
	ba, _ := MarshalReports(ra)
	bb, _ := MarshalReports(rb)
	if string(ba) != string(bb) {
		t.Error("草案路径与直编路径的推进应逐位一致")
	}
}

// TestDraftProtocolLoopHandover（§21.3 衔接）：经草案路径录入并升格
// AutonomousLoop 的功法，移交门语义与直编路径完全一致——已内化段停发
// 请求，未内化段照常教学。
func TestDraftProtocolLoopHandover(t *testing.T) {
	s := NewSession(PartJCalibrationWorld(), 7, nil)
	yaml := partjRingYAML("wood", 300, 5)
	res := Tick(s, []Intent{{Kind: IntentProtocolDraft, Text: yaml}})
	if res.Receipts[0].Outcome != WordApplied {
		t.Fatalf("草案应录入: %+v", res.Receipts[0])
	}
	p := s.Protocols["zhutian_partj_v1"]
	if p == nil {
		t.Fatal("功法应入册")
	}
	Tick(s, nil)
	s.RegisterLoop(p)
	if s.active == nil || !s.active.loopMode {
		t.Fatal("RegisterLoop 应进入 loopMode")
	}

	// 未内化：Runtime 照常发引导请求。
	ops, ev := s.active.runtimeOps(s)
	if ev != nil {
		t.Fatalf("未内化段不应出界: %+v", ev)
	}
	if len(ops.Requests) == 0 {
		t.Fatal("未内化段应仍由 Runtime 驱动（教学期语义，§21.2）")
	}

	// 已内化：移交门生效，引导请求全部消失。
	g := &s.World.Character.BodyGraph
	for i := range g.Acupoints {
		for k := range g.Acupoints[i].Couplings {
			for e := 0; e < cult.ElementCount; e++ {
				g.Acupoints[i].Couplings[k].Strength[e] = feedHandoverStrength
			}
		}
	}
	ops2, ev2 := s.active.runtimeOps(s)
	if ev2 != nil {
		t.Fatalf("移交段不应出界: %+v", ev2)
	}
	if len(ops2.Requests) != 0 {
		t.Fatalf("§21.3: 已内化段应移交身体，仍发 %d 条请求", len(ops2.Requests))
	}
}
