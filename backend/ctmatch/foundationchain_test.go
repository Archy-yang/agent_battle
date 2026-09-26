package ctmatch

// foundationchain_test.go —— 阶段三收口「筑基判定全链」验收测试
//（docs/specs/2026-09-26-engine-drive-foundation-v1.md 阶段三范围；评审稿
// docs/cultivation_autonomous_loop_design_v0.2.md 测试规范）：
//
//	§26.12 非闭合不筑基（闭合分层 §17：拓扑闭合 × 流量闭合）
//	§26.16 高自持低规则平衡不筑基（§19）
//	§26.17 高自持低动态稳定不筑基（§18，构造振荡流场景）
//	§26.18 次数无关性（§20.1：StableTicks 不是修炼次数/进度条）
//	§28.7 / 不变量 19 评估器只读：判定链不得向世界写入任何状态
//	§21.3  筑基后协议移交已内化段（不再逐节拍驱动）
//
// 真实松手全链（引气→炼气→反复行环→松手→自然筑基）由 partj_test.go 的
// T-J6 覆盖；本文件的单判定器用例按 wp4_test.go 既有口径直接构造
// FoundationSample（deprecated 喂样路径仅测试兼容用）。

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"agentbattle/backend/cult"
)

// ---------------------------------------------------------------------------
// §26.12 非闭合不筑基
// ---------------------------------------------------------------------------

// TestClosureRequiresFlowReturn（§26.12/§17）：拓扑闭合但流量不闭合——
// 环首出去了 1000、只回来 150（15%）：图上闭合但只回小部分不算高质量
// 闭环，Closure = 1.0 × 0.15 远低于候选阈值，永不触碰/筑基。
func TestClosureRequiresFlowReturn(t *testing.T) {
	m := NewFoundationMonitor()
	for i := int64(1); i <= 300; i++ {
		s := foundationGoodSample(i)
		s.CycleOut = 1000
		s.CycleBack = 150 // 回流 15%
		if touched, established := m.Observe(s); touched || established {
			t.Fatalf("§26.12: 流量闭合不足不应触门槛 (tick %d, metrics %+v)", i, m.Metrics())
		}
	}
	if mt := m.Metrics(); mt.Closure >= CandClosure {
		t.Fatalf("§26.12: 低回流 Closure 应低于候选阈值, got %d", mt.Closure)
	}

	// 对照组：回流追平初发流（95%）时同样的流能正常走完判定链——
	// 证明上一段拒绝的是「不回流」，不是别的量。
	m2 := NewFoundationMonitor()
	established := false
	for i := int64(1); i <= 130; i++ {
		s := foundationGoodSample(i)
		s.CycleOut = 1000
		s.CycleBack = 950
		if _, established = m2.Observe(s); established {
			break
		}
	}
	if !established {
		t.Fatalf("§26.12 对照组: 高回流样本应可筑基, metrics %+v", m2.Metrics())
	}
}

// chainYAML 生成一条"纳气 + 无回路的链式引导"功法（丹田→土府→中庭→阳枢，
// 有去无回——§26.12 的拓扑不闭合形态）。
func chainYAML(cycles int64) string {
	chain := [][2]string{
		{"下丹田", "土府"},
		{"土府", "中庭"},
		{"中庭", "阳枢"},
	}
	var b strings.Builder
	fmt.Fprintf(&b, "protocol: chain_no_closure_v1\nactions:\n")
	fmt.Fprintf(&b, "  - absorb: {target: 青关, level: trace, duration: 1, parallel: 1}\n")
	for _, e := range chain {
		fmt.Fprintf(&b, "  - guide: {from: %s, to: %s, element: \"wood\", amount: 2000, duration: 1, parallel: 1}\n",
			e[0], e[1])
	}
	fmt.Fprintf(&b, "cadence: 1\ncycles: %d\nvalidated_range:\n", cycles)
	for _, e := range chain {
		fmt.Fprintf(&b, "  - channel: [%s, %s]\n    element: \"wood\"\n    min: 0\n    max: 2000\n", e[0], e[1])
	}
	return b.String()
}

// TestNonClosingLoopNeverFounds（§26.12，真实 tick 流）：链式功法有去无回，
// 局部段落行环推动正常，但流不能返回起点——拓扑闭合 0 → Closure 恒 0，
// 推动与松手全程不触碰、不筑基。
func TestNonClosingLoopNeverFounds(t *testing.T) {
	const cycles = 200
	w := SpikeWorld()
	w.Character.BodyGraph.Acupoints[0].Qi[cult.Wood] = 60000
	s := NewSession(w, 17, nil)
	p, err := Compile([]byte(chainYAML(cycles)), &s.World.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	s.LoadProtocol(p)
	Tick(s, nil)
	if err := s.StartProtocol(p.Name); err != nil {
		t.Fatalf("start: %v", err)
	}

	// 推动相：链路有流（场景前提），但 Closure 必须恒 0。
	var pushFlow int64
	for i := 0; i < cycles; i++ {
		res := Tick(s, nil)
		if !res.Advanced {
			t.Fatalf("链式推动 tick %d 未推进: %+v", i, res.Events)
		}
		pushFlow += tickTotalFlow(res.Report)
		if s.Foundation.Established || s.Foundation.Touched {
			t.Fatalf("§26.12: 非闭合链路不得触碰筑基 (tick %d)", i)
		}
	}
	if pushFlow <= 0 {
		t.Fatal("测试前提失效：推动期应有流量")
	}

	// 松手相：继续正常模拟（§16 松手只清手动驱动，世界不中断），
	// 流不能返回起点 → 永不筑基。
	for i := 0; i < 120; i++ {
		Tick(s, nil)
		if s.Foundation.Established || s.Foundation.Touched {
			t.Fatalf("§26.12: 松手期非闭合链路不得触碰/筑基 (tick %d)", i)
		}
	}
	if mt := s.Foundation.Metrics(); mt.Closure != 0 {
		t.Fatalf("§26.12: 非闭合链路 Closure 应恒 0, got %d", mt.Closure)
	}
}

// ---------------------------------------------------------------------------
// §26.16 高自持低规则平衡不筑基
// ---------------------------------------------------------------------------

// TestHighSelfSustainLowRuleBalance（§26.16/§19）：松手后流量几乎不衰减
// （SelfSustain 高），但能量账大幅失衡——总量急纳急泄、入不敷出与反噬
// 交替（"确实自己会转，但在快速耗干自己"）→ RuleBalance 远低于阈值，
// 永不筑基。
func TestHighSelfSustainLowRuleBalance(t *testing.T) {
	m := NewFoundationMonitor()
	established := false
	for i := int64(1); i <= 300; i++ {
		s := foundationGoodSample(i)
		if i <= 30 {
			// 推动相：手动驱动维持 1000 的组织化流。
			s.ControlActive = true
			s.ManualFlow = 1000
			s.ActualFlow = 1000
		} else {
			// 松手相：流几乎不衰减（95% 自存）。
			s.ControlActive = false
			s.ActualFlow = 950
		}
		// 能量账失衡：总量在 38000/20000 间大幅振荡（每 Tick |Δ|/总量
		// 高企——§19 EnergyImbalance 持续拉低 RuleBalance）。
		if i%2 == 1 {
			s.TotalQi = 38000
		} else {
			s.TotalQi = 20000
		}
		if _, established = m.Observe(s); established {
			t.Fatalf("§26.16: 高自持低平衡不得筑基 (tick %d)", i)
		}
	}
	mt := m.Metrics()
	if mt.SelfSustain < FoundSelfSustain {
		t.Fatalf("§26.16 前提失效: SelfSustain=%d 应达标", mt.SelfSustain)
	}
	if mt.RuleBalance >= FoundRuleBalance {
		t.Fatalf("§26.16 前提失效: RuleBalance=%d 应不达标", mt.RuleBalance)
	}
	if m.Touched {
		t.Error("§26.16: 候选门槛也不应触碰（能量失衡一路压到候选线下）")
	}
}

// ---------------------------------------------------------------------------
// §26.17 高自持低动态稳定不筑基
// ---------------------------------------------------------------------------

// TestHighSelfSustainLowDynamicStability（§26.17/§18）：构造振荡流——
// 松手后流量在 3000/100 间剧烈振荡，均值高于推动窗手动驱动（SelfSustain
// 满值），但流量方差把 DynamicStability 压到候选线以下 → 永不触碰/筑基
// （"气确实自己动起来了，但结构正在失控"）。
func TestHighSelfSustainLowDynamicStability(t *testing.T) {
	m := NewFoundationMonitor()
	established := false
	for i := int64(1); i <= 300; i++ {
		s := foundationGoodSample(i)
		if i <= 30 {
			s.ControlActive = true
			s.ManualFlow = 1000
			s.ActualFlow = 1000
		} else {
			s.ControlActive = false
			// 振荡流：3900/100 交替（周期 2），均值 2000 ≥ 手动驱动 1000。
			if i%2 == 0 {
				s.ActualFlow = 3900
				s.CycleOut = 3900
				s.CycleBack = 3705
			} else {
				s.ActualFlow = 100
				s.CycleOut = 100
				s.CycleBack = 95
			}
		}
		if _, established = m.Observe(s); established {
			t.Fatalf("§26.17: 高自持低动态稳定不得筑基 (tick %d)", i)
		}
	}
	mt := m.Metrics()
	if mt.SelfSustain < FoundSelfSustain {
		t.Fatalf("§26.17 前提失效: SelfSustain=%d 应达标", mt.SelfSustain)
	}
	if mt.DynamicStability >= CandDynStability {
		t.Fatalf("§26.17 前提失效: DynamicStability=%d 应不达标", mt.DynamicStability)
	}
	if m.Touched {
		t.Error("§26.17: 候选门槛也不应触碰（振荡一路压到候选线下）")
	}
}

// TestOscillationDetectedInRealStream（§26.17 真实流对照）：真实引擎上
// 交替猛推/缓推构造振荡流场景，四方差联合判定必须把 DynamicStability
// 压到候选线以下——公式对真实振荡敏感，不只对人工样本。
func TestOscillationDetectedInRealStream(t *testing.T) {
	w := SpikeWorld()
	w.Character.BodyGraph.Acupoints[0].Qi[cult.Wood] = 60000
	s := NewSession(w, 5, nil)
	Tick(s, nil) // 静息基准

	var flows []int64
	for i := 0; i < 60; i++ {
		amt := int64(3000)
		if i%2 == 1 {
			amt = 200
		}
		res := Tick(s, []Intent{{
			Kind: IntentGuide, From: "下丹田", To: "土府",
			Element: cult.Wood, Amount: amt,
		}})
		if !res.Advanced {
			t.Fatalf("振荡推动 tick %d 未推进: %+v", i, res.Events)
		}
		var f int64
		for ti := range res.Report.Transfers {
			f += res.Report.Transfers[ti].Actual[cult.Wood]
		}
		flows = append(flows, f)
	}
	var fmin, fmax int64 = 1 << 62, 0
	for _, f := range flows {
		if f < fmin {
			fmin = f
		}
		if f > fmax {
			fmax = f
		}
	}
	if fmax < 3*fmin || fmax <= 0 {
		t.Fatalf("测试前提失效：流未显著振荡 (min=%d max=%d)", fmin, fmax)
	}
	if mt := s.Foundation.Metrics(); mt.DynamicStability >= CandDynStability {
		t.Fatalf("§26.17: 真实振荡流的 DynamicStability 应低于候选线, got %d", mt.DynamicStability)
	}
}

// ---------------------------------------------------------------------------
// §26.18 次数无关性
// ---------------------------------------------------------------------------

// TestFoundationCountIndependence（§26.18/§20.1）：两个身体当前
// World/Body/Coupling/Energy 状态完全相同（同 seed 同流逐位重演），
// 只有功法记录的周天总数不同（300 vs 900——"修炼次数"的载体）。
// 判定链只消费样本流，不消费任何次数：报告、五量、跨门槛 Tick 必须
// 逐位相同。这是防"次数经验条"回潮的回归锁。
func TestFoundationCountIndependence(t *testing.T) {
	run := func(cycles int64) (*CultSession, []string, int) {
		s := NewSession(PartJCalibrationWorld(), 7, nil)
		p, err := Compile([]byte(partjRingYAML("", 300, cycles)), &s.World.Character.BodyGraph)
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		s.LoadProtocol(p)
		Tick(s, nil)
		if err := s.StartProtocol(p.Name); err != nil {
			t.Fatalf("start: %v", err)
		}
		for i := 0; i < 150; i++ { // 推动相：两个功法都未跑满（次数未兑现）
			if res := Tick(s, nil); !res.Advanced {
				t.Fatalf("推动 tick %d 未推进: %+v", i, res.Events)
			}
		}
		Tick(s, []Intent{{Kind: IntentStop}}) // 同 Tick 停机 → 松手
		var reports []string
		establishedAt := -1
		for i := 0; i < 260; i++ {
			res := Tick(s, nil)
			b, err := json.Marshal(res.Report)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			reports = append(reports, string(b))
			if s.Foundation.Established && establishedAt < 0 {
				establishedAt = i
			}
		}
		return s, reports, establishedAt
	}

	a, ra, ta := run(300)
	b, rb, tb := run(900)
	if ta != tb {
		t.Fatalf("§26.18: 筑基 Tick 不同: %d vs %d", ta, tb)
	}
	t.Logf("§26.18: 筑基发生在松手后第 %d Tick（两身体同刻）", ta)
	if a.Foundation.Touched != b.Foundation.Touched ||
		a.Foundation.Established != b.Foundation.Established {
		t.Fatal("§26.18: 跨门槛事实不一致")
	}
	if !reflect.DeepEqual(a.Foundation.Metrics(), b.Foundation.Metrics()) {
		t.Fatalf("§26.18: 五量不同: %+v vs %+v", a.Foundation.Metrics(), b.Foundation.Metrics())
	}
	for i := range ra {
		if ra[i] != rb[i] {
			t.Fatalf("§26.18: tick %d 世界报告不同——次数泄入了世界演化", i)
		}
	}
	if !reflect.DeepEqual(a.World, b.World) {
		t.Fatal("§26.18: 世界终态不同")
	}
}

// ---------------------------------------------------------------------------
// §28.7 / 不变量 19 评估器只读
// ---------------------------------------------------------------------------

// copyOps 深拷贝一份 Ops（切片独立）。
func copyOps(ops cult.Ops) cult.Ops {
	out := cult.Ops{}
	out.Intensity = append([]int64(nil), ops.Intensity...)
	out.Requests = append([]cult.FlowRequest(nil), ops.Requests...)
	return out
}

// TestEvaluatorReadOnly（评审稿 §28.7 / §23 关键约束 / 不变量 19）：
// 判定链（buildFoundationSample → Observe → 判定 → 事件/升格）不得向世界
// 写入任何驱动/耦合/流量/能力。三条证据链：
//  1. 会话正常推进（评估链每 Tick 运行一次）的世界，与同一 ops 流的
//     纯引擎重演（无会话、无判定）逐位一致；
//  2. 每 Tick 对评估链超量重复调用 6 次（高频采样的评估器）不改变世界
//     ——若评估链有任何写效应，重复调用必然放大可见；
//  3. 全程报告逐字节一致。
func TestEvaluatorReadOnly(t *testing.T) {
	const (
		cycles     = 40
		releaseN   = 8
		extraCalls = 5 // 正常 1 次 + 额外 5 次 = 6 倍评估频率
	)

	// 参照世界：同构初值（SpikeWorld 确定性构造 + 同一预填），同噪声种子
	//（NewSession 会覆写 World.NoiseSeed，参照必须对齐），并补上 newSession
	// 里那一次静息基准 Tick（空 ops）。
	wRef := SpikeWorld()
	wRef.Character.BodyGraph.Acupoints[0].Qi[cult.Wood] = 60000
	wRef.NoiseSeed = 13
	wRef.Ops = cult.Ops{}
	cult.Advance(wRef)

	newSession := func() *CultSession {
		w := SpikeWorld()
		w.Character.BodyGraph.Acupoints[0].Qi[cult.Wood] = 60000
		s := NewSession(w, 13, nil)
		p, err := Compile([]byte(partjRingYAML("wood", 2000, cycles)), &s.World.Character.BodyGraph)
		if err != nil {
			t.Fatalf("compile: %v", err)
		}
		s.LoadProtocol(p)
		Tick(s, nil)
		if err := s.StartProtocol(p.Name); err != nil {
			t.Fatalf("start: %v", err)
		}
		return s
	}

	a := newSession()
	b := newSession()
	for i := 0; i < cycles+releaseN; i++ {
		resA := Tick(a, nil)
		resB := Tick(b, nil)

		// 参照重演：取会话本 Tick 实际使用的 ops，直接驱动纯引擎。
		wRef.Ops = copyOps(a.World.Ops)
		repRef := cult.Advance(wRef)

		ba, err := json.Marshal(resA.Report)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		br, err := json.Marshal(repRef)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if string(ba) != string(br) {
			t.Fatalf("证据 1: tick %d 会话报告与纯引擎重演不一致——判定链外存在写路径", i)
		}

		// 证据 2：对评估链超量重复采样。
		ctrl := opsHasControl(b.World.Ops) ||
			(resB.Report != nil && resB.Report.Work.ManualWork > 0)
		for k := 0; k < extraCalls; k++ {
			b.observeFoundation(resB, ctrl)
		}

		if string(ba) != mustJSON(t, resB.Report) {
			t.Fatalf("证据 3: tick %d 高频评估会话的报告出现分歧", i)
		}
	}
	if !reflect.DeepEqual(a.World, wRef) {
		t.Fatal("证据 1: 会话世界与纯引擎重演终态不一致——评估链写了世界")
	}
	if !reflect.DeepEqual(a.World, b.World) {
		t.Fatal("证据 2: 高频评估改变了世界状态——评估器非只读")
	}
	if a.Foundation.Established || b.Foundation.Established {
		t.Fatal("场景前提失效：本场景不应筑基（升格路径未触发）")
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// ---------------------------------------------------------------------------
// §21.3 筑基后协议移交已内化段
// ---------------------------------------------------------------------------

// TestLoopHandoverInternalizedSegments（§21.3，主会话裁决 2026-09-26）：
// AutonomousLoop 模式下，出边段耦合强度达移交门槛 → Runtime 不再对该段
// 发主动请求（流由耦合自主命令续推，ManualWork=0 → 不计入控制）；未达
// 门槛的段照常发请求（教学继续，§21.2）。
func TestLoopHandoverInternalizedSegments(t *testing.T) {
	s := NewSession(PartJCalibrationWorld(), 7, nil)
	p, err := Compile([]byte(partjRingYAML("wood", 300, 5)), &s.World.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	s.LoadProtocol(p)
	Tick(s, nil)
	s.RegisterLoop(p) // §39 升格：loopMode（测试直接驱动判定器以外的入口）
	if s.active == nil || !s.active.loopMode {
		t.Fatal("测试前提失效：RegisterLoop 应进入 loopMode")
	}

	// 未内化（耦合强度全零——NewWorld 初值）：Runtime 照常发引导请求。
	ops, ev := s.active.runtimeOps(s)
	if ev != nil {
		t.Fatalf("未内化段不应出界: %+v", ev)
	}
	if len(ops.Requests) == 0 {
		t.Fatal("未内化段应仍由 Runtime 驱动（教学期语义，§21.2）")
	}

	// 已内化：环上全部出边耦合达移交门槛 → 引导请求全部消失。
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
