package cult

// feedback_test.go —— 阶段三第一块「局部反馈耦合」验收测试（定稿
// docs/specs/2026-09-26-engine-drive-foundation-v1.md 阶段三验收；对应评审稿
// docs/cultivation_autonomous_loop_design_v0.2.md）：
//
//	§26.7  未内化段松手即衰减：耦合≈0、无主动驱动 → 无自主接续，流衰减；
//	§26.8  教师门饱和：超教学强度继续加力，学习不加速（猛推不加速学习）；
//	§26.9  误差收敛：预测逼近目标时更新量自然趋零；
//	§26.10 失稳抑制：稳定↓/损伤↑/过应变↑ → 安全门↓、学习停止；
//	§26.11 局部内化：仅内化段产生自主命令；无来流不凭空启动；
//	       无能源命令不成驱动；受控流不伪装自主（T-J3 语义）；
//	§26.19 局部而非整径耦合：引擎状态中不存在整条周天路径的耦合值；
//	§26.4  延伸——做功守恒：自主驱动必扣真实能源、必付做功、必入台账；
//	§26.20 确定性重放：同 seed 逐位一致，含耦合状态；
//	感知映射：反馈形成有定性感知词（零数字零机器词）。
//
// 测试世界（feedChainWorld）：三穴单向链 甲→乙→丙，满传导、满控制（ε≡0）、
// 火亲和为 0（断相生）、满保留 + 绝灵环境（无纳入/纳气/泄出）——耦合学习
// 与自主驱动是唯一变量，逐项可归因。

import (
	"reflect"
	"strings"
	"testing"
)

func feedAcupoint(name string, capacity, driveCap, qi int64) Acupoint {
	a := Acupoint{
		Name:            name,
		Affinity:        [ElementCount]int64{1000, 0, 1000, 1000, 1000}, // 火亲和 0：断相生
		Capacity:        capacity,
		Throughput:      10000,
		DriveCapacity:   driveCap,
		DriveEfficiency: 1000,
		Retention:       1000, // 满保留：绝灵环境零泄出
		Openness:        MicroOne,
		Stability:       950,
		Activity:        [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
	}
	a.Qi[Wood] = qi
	return a
}

// feedChainWorld 构造三穴单向链 甲→乙→丙（大库容：教学期不枯竭、压力梯度
// 温和），绝灵环境。
func feedChainWorld() *World {
	char := Character{
		Affinity:   [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Control:    1000, // 满控制 → ε ≡ 0（教学样本确定性）
		Perception: 500,
		BodyGraph: BodyGraph{
			Acupoints: []Acupoint{
				feedAcupoint("甲", 4000000, 12000, 2000000),
				feedAcupoint("乙", 1000000, 5000, 400000),
				feedAcupoint("丙", 5000000, 5000, 0),
			},
			Channels: []Channel{
				{Name: "甲→乙", From: 0, To: 1, BaseConductance: 1000, Throughput: 10000},
				{Name: "乙→丙", From: 1, To: 2, BaseConductance: 1000, Throughput: 10000},
			},
		},
	}
	return NewWorld(char, Environment{Temperature: 1000}) // 绝灵
}

// feedChain4World 构造四穴单向链 甲→乙→丙→丁（§26.11/§26.19 局部性用）。
func feedChain4World() *World {
	char := Character{
		Affinity:   [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Control:    1000,
		Perception: 500,
		BodyGraph: BodyGraph{
			Acupoints: []Acupoint{
				feedAcupoint("甲", 4000000, 12000, 2000000),
				feedAcupoint("乙", 1000000, 5000, 400000),
				feedAcupoint("丙", 1000000, 5000, 0),
				feedAcupoint("丁", 5000000, 5000, 0),
			},
			Channels: []Channel{
				{Name: "甲→乙", From: 0, To: 1, BaseConductance: 1000, Throughput: 10000},
				{Name: "乙→丙", From: 1, To: 2, BaseConductance: 1000, Throughput: 10000},
				{Name: "丙→丁", From: 2, To: 3, BaseConductance: 1000, Throughput: 10000},
			},
		},
	}
	return NewWorld(char, Environment{Temperature: 1000})
}

// feedCoupleOf 取穴位 node 上（进边 ic → 出边 oc）的耦合指针。
func feedCoupleOf(w *World, node, ic, oc int) *FeedbackCoupling {
	for k := range w.Character.BodyGraph.Acupoints[node].Couplings {
		cp := &w.Character.BodyGraph.Acupoints[node].Couplings[k]
		if cp.InChannel == ic && cp.OutChannel == oc {
			return cp
		}
	}
	return nil
}

// teachChain 以恒定教学样本（进边 + 出边双请求）行 N Tick。
func teachChain(w *World, ticks int, amount int64) {
	reqs := []FlowRequest{
		{Channel: 0, Amount: [ElementCount]int64{amount, 0, 0, 0, 0}},
		{Channel: 1, Amount: [ElementCount]int64{amount, 0, 0, 0, 0}},
	}
	for i := 0; i < ticks; i++ {
		w.Ops = Ops{Requests: reqs}
		Advance(w)
	}
}

// autonomousFlowOf 汇总一份报告的自主驱动实际流量。
func autonomousFlowOf(rep *TickReport) int64 {
	var s int64
	for _, tr := range rep.Transfers {
		for e := 0; e < ElementCount; e++ {
			s += tr.Autonomous[e]
		}
	}
	return s
}

// TestFeedUninternalizedReleaseDecays（§26.7 + §26.4 延伸）：未内化（耦合
// 全零）的身体松手后无任何自主接续——惯性能维持的流只会衰减，不会续推；
// 已内化的身体松手后出现自主驱动、必付做功、必入台账、内化度满值。
func TestFeedUninternalizedReleaseDecays(t *testing.T) {
	// 未内化：等压三穴 + 预置惯性环流，直接松手——耦合全零，无自主接续。
	u := feedChainWorld()
	for i, qi := range []int64{2000000, 500000, 2500000} { // 全部 LR = 0.5：零势差
		u.Character.BodyGraph.Acupoints[i].Qi[Wood] = qi
	}
	for c := range u.Character.BodyGraph.Channels {
		u.Character.BodyGraph.Channels[c].FlowPrev = [ElementCount]int64{1000, 0, 0, 0, 0}
	}
	var uFlows []int64
	for i := 0; i < 40; i++ {
		u.Ops = Ops{}
		rep := Advance(u)
		checkConservation(t, rep)
		if af := autonomousFlowOf(rep); af != 0 {
			t.Fatalf("§26.7: 未内化身体松手后出现自主驱动 %d（tick %d）", af, i)
		}
		if rep.Work.AutonomousWork != 0 {
			t.Fatalf("§26.7: 未内化身体松手后台账自主做功应恒零: %d（tick %d）", rep.Work.AutonomousWork, i)
		}
		if got := DeriveInternalization(&rep.Work); got != 0 {
			t.Fatalf("§26.7: 未内化身体内化度应恒 0: %d（tick %d）", got, i)
		}
		uFlows = append(uFlows, totalFlowOf(rep))
	}
	// 无自主接续：惯性环流无源有耗，只能衰减（对照 Part J 惯性接力）。
	if uFlows[len(uFlows)-1] >= uFlows[0] {
		t.Errorf("§26.7: 未内化松手流应衰减: 首 %d → 末 %d", uFlows[0], uFlows[len(uFlows)-1])
	}

	// 已内化：教学 300 Tick 后松手。
	tr := feedChainWorld()
	teachChain(tr, 300, 2000)
	cp := feedCoupleOf(tr, 1, 0, 1) // 乙穴（进边 甲→乙，出边 乙→丙）
	if cp == nil || cp.Strength[Wood] < 800 {
		t.Fatalf("测试前提失效：教学后乙穴耦合应成形, got %+v", cp)
	}
	sawAuto := false
	for i := 0; i < 40; i++ {
		tr.Ops = Ops{}
		rep := Advance(tr)
		checkConservation(t, rep)
		af := autonomousFlowOf(rep)
		if af > 0 {
			sawAuto = true
			if rep.Work.AutonomousWork != af {
				t.Errorf("tick %d: 自主做功应与自主实际流量同源: %d vs %d", i, rep.Work.AutonomousWork, af)
			}
			if got := DeriveInternalization(&rep.Work); got != 1000 {
				t.Errorf("tick %d: 松手期（无主动请求）内化度应满值: %d", i, got)
			}
		}
	}
	if !sawAuto {
		t.Fatal("§26.7 对照：已内化身体松手后应产生自主驱动")
	}

	// 做功守恒（§26.4 延伸）：自主驱动 Tick 的驱动做功全部来自统一驱动
	// 求解（守恒账 DriveWork 与条目对账），自主驱动必扣乙穴真实储气。
	tr2 := feedChainWorld()
	teachChain(tr2, 300, 2000)
	before := tr2.Character.BodyGraph.Acupoints[1].Qi[Wood]
	var rep *TickReport
	for i := 0; i < 40; i++ {
		tr2.Ops = Ops{}
		rep = Advance(tr2)
		if autonomousFlowOf(rep) > 0 {
			break
		}
	}
	if rep == nil || autonomousFlowOf(rep) == 0 {
		t.Fatal("测试前提失效：松手窗内未出现自主驱动")
	}
	var autoWork int64
	for _, tr := range rep.Transfers {
		for e := 0; e < ElementCount; e++ {
			if tr.Autonomous[e] > 0 && tr.DriveWork[e] <= 0 {
				t.Errorf("自主驱动应支付 §10.1 做功: entry %+v", tr)
			}
			autoWork += tr.DriveWork[e]
		}
	}
	if rep.Conservation.DriveWork != autoWork {
		t.Errorf("松手 Tick 守恒账驱动做功应全部来自自主驱动: %d vs %d", rep.Conservation.DriveWork, autoWork)
	}
	after := tr2.Character.BodyGraph.Acupoints[1].Qi[Wood]
	if after >= before {
		t.Errorf("自主驱动必扣真实能源：乙穴储量应下降: %d -> %d", before, after)
	}
	checkConservation(t, rep)
}

// TestFeedTeacherGateSaturates（§26.8）：教师门饱和——超过教学强度继续
// 加力，学习不加速（目标响应被钳制 + 门饱和 → 学习更新逐位相同）。
func TestFeedTeacherGateSaturates(t *testing.T) {
	// 门函数：达到教学强度即满值，继续猛推不增长。
	if got := teacherGateOf(1000); got != 500 {
		t.Errorf("半教学强度门应 0.5: %d", got)
	}
	if got := teacherGateOf(2000); got != 1000 {
		t.Errorf("教学强度门应饱和到 1.0: %d", got)
	}
	if got := teacherGateOf(8000); got != 1000 {
		t.Errorf("四倍猛推门不应超过 1.0: %d（禁止「用力十倍内化十倍」）", got)
	}

	// 一步学习：同来流、同初始耦合，教学出流 2×ref 与 8×ref——目标响应
	// 均被 FeedResponseMax 钳制、门均饱和 → 强度更新逐位相同。
	step := func(manualOut int64) (strength, learned int64) {
		w := feedChainWorld()
		ts := &teachSamples{
			chanDelivered: make([][ElementCount]int64, 2),
			manualOut:     make([][ElementCount]int64, 2),
		}
		ts.chanDelivered[0][Wood] = 2000  // 进边来流固定
		ts.manualOut[1][Wood] = manualOut // 教学出流（变量）
		rep := &TickReport{Points: make([]AcupointReport, 3)}
		rep.Points[1] = AcupointReport{Stability: 1000, Compatibility: 1000}
		updateFeedbackCouplings(w, rep, ts)
		cp := feedCoupleOf(w, 1, 0, 1)
		return cp.Strength[Wood], cp.LearnedResponse[Wood]
	}
	s1, l1 := step(2 * TeacherDriveRef) // 2 倍猛推：目标 2.0（恰在钳制线上）
	s8, l8 := step(8 * TeacherDriveRef) // 8 倍猛推：目标钳制后同为 2.0
	if s1 != s8 || l1 != l8 {
		t.Errorf("§26.8: 猛推不应加速学习: 强度 %d vs %d, 习得响应 %d vs %d", s1, s8, l1, l8)
	}
	if s1 == 0 {
		t.Fatal("测试前提失效：饱和档应仍有学习量")
	}

	// 对照：半强度教学（门 0.5、目标 0.5）的更新量显著更小——门与误差
	// 确实在参与（不是恒等通道）。
	sHalf, _ := step(TeacherDriveRef / 2)
	if sHalf >= s1 {
		t.Errorf("半强度教学更新应小于饱和档: %d vs %d", sHalf, s1)
	}
}

// TestFeedErrorConverges（§26.9）：误差驱动收敛——预测逼近目标后单步
// 更新量自然趋零，耦合稳定（不按次数成长：有误差才有更新）。
func TestFeedErrorConverges(t *testing.T) {
	w := feedChainWorld()
	cp := feedCoupleOf(w, 1, 0, 1)
	if cp == nil {
		t.Fatal("测试前提失效：乙穴应有耦合")
	}
	const teachTicks = 400
	prev := int64(0)
	var deltaEarly, deltaLate int64
	var errEarly, errLate int64
	for i := 0; i < teachTicks; i++ {
		teachChain(w, 1, 2000)
		cur := cp.Strength[Wood]
		d := cur - prev
		switch i {
		case 19:
			deltaEarly, errEarly = d, cp.LastError[Wood]
		case teachTicks - 1:
			deltaLate, errLate = d, cp.LastError[Wood]
		}
		prev = cur
	}
	if prev <= 0 {
		t.Fatal("教学应使耦合成长")
	}
	if errEarly < 100 {
		t.Errorf("教学早期误差应显著: %d", errEarly)
	}
	// 收敛：预测逼近目标 → 误差与单步更新量同步趋零（§12.9）。
	absLate, absEarly := errLate, errEarly
	if absLate < 0 {
		absLate = -absLate
	}
	if absEarly < 0 {
		absEarly = -absEarly
	}
	if absLate*10 > absEarly {
		t.Errorf("§26.9: 后期误差应收敛到早期的 1/10 以内: %d vs %d", absLate, absEarly)
	}
	if deltaLate < 0 {
		deltaLate = -deltaLate
	}
	if deltaEarly < 0 {
		deltaEarly = -deltaEarly
	}
	if deltaLate*10 > deltaEarly {
		t.Errorf("§26.9: 后期单步更新量应趋零: %d vs %d", deltaLate, deltaEarly)
	}
}

// TestFeedInstabilitySuppressesLearning（§26.10）：失稳抑制——过驱教学
// （应变↑ → 稳定↓ → 损伤↑）使安全门塌落、学习停止；温和教学正常成形。
func TestFeedInstabilitySuppressesLearning(t *testing.T) {
	// 安全门：各分量单调抑制。
	if got := safetyGateOf(1000, 1000, 0, 0); got != 1000 {
		t.Errorf("健康状态安全门应满值: %d", got)
	}
	if got := safetyGateOf(500, 1000, 0, 0); got != 500 {
		t.Errorf("稳定减半 → 门减半: %d", got)
	}
	if got := safetyGateOf(1000, 1000, 300, 0); got != 700 {
		t.Errorf("损伤 0.3 → 门 ×0.7: %d", got)
	}
	if got := safetyGateOf(1000, 1000, 0, 1000); got != 0 {
		t.Errorf("过应变满值 → 门归零: %d", got)
	}

	// 世界级：同来流同教学，健康身体 vs 失稳带伤的身体——安全门塌落的
	// 身体学不进去（§26.10 的世界语义）。
	healthy := feedChainWorld()
	injured := feedChainWorld()
	// 失稳前提白盒构造：稳定被打落、结构带伤（对应 Stability↓ / Damage↑）。
	injured.Character.BodyGraph.Acupoints[1].Stability = 200
	injured.Character.BodyGraph.Acupoints[1].Damage = 500
	for i := 0; i < 20; i++ {
		teachChain(healthy, 1, 2000)
		teachChain(injured, 1, 2000)
	}
	hc := feedCoupleOf(healthy, 1, 0, 1)
	ic := feedCoupleOf(injured, 1, 0, 1)
	if hc == nil || ic == nil {
		t.Fatal("测试前提失效：耦合缺失")
	}
	if hc.Strength[Wood] <= 0 {
		t.Errorf("健康身体教学应成形: %d", hc.Strength[Wood])
	}
	if ic.Strength[Wood]*5 > hc.Strength[Wood] {
		t.Errorf("§26.10: 失稳应抑制学习: 失稳 %d vs 健康 %d", ic.Strength[Wood], hc.Strength[Wood])
	}
}

// TestFeedLocalInternalization（§26.11/§26.19）：局部内化——耦合是「进边 →
// 出边」的局部转移：无来流不得凭空启动；有容差内来流产生自主命令；无能源
// 命令不成驱动；受控流不伪装自主（T-J3）；未内化的转移不自响应。
func TestFeedLocalInternalization(t *testing.T) {
	seedCoupling := func(w *World) {
		cp := feedCoupleOf(w, 1, 0, 1)
		if cp == nil {
			t.Fatal("测试前提失效：乙穴耦合缺失")
		}
		cp.Strength[Wood] = 1000 // 已内化（白盒置位，学习路径另测）
		cp.LearnedResponse[Wood] = 1000
		cp.LastInputSignal[Wood] = 0 // 容差带 [0, 0.5]：来流 ≤ 半通量即可识别
	}

	// ① 无来流：耦合强度再高也不能凭空启动乙→丙（三穴等压零势差、零
	//    惯性，进边持续零来流）。
	w1 := feedChainWorld()
	seedCoupling(w1)
	for i, qi := range []int64{2000000, 500000, 2500000} { // 全部 LR = 0.5
		w1.Character.BodyGraph.Acupoints[i].Qi[Wood] = qi
	}
	for i := 0; i < 10; i++ {
		w1.Ops = Ops{}
		rep := Advance(w1)
		if af := autonomousFlowOf(rep); af != 0 {
			t.Fatalf("§26.11: 无来流时出现自主驱动 %d——凭空启动（tick %d）", af, i)
		}
		for _, tr := range rep.Transfers {
			if tr.Channel == 1 && tr.Actual[Wood] != 0 {
				t.Fatalf("§26.11: 无来流时乙→丙不得有任何流动（凭空启动）: %d", tr.Actual[Wood])
			}
		}
	}
	if cp := feedCoupleOf(w1, 1, 0, 1); cp.Strength[Wood] != 1000 || cp.LearnedResponse[Wood] != 1000 {
		t.Fatalf("§12.1: 无样本耦合不得自行变化: %+v", cp)
	}

	// ② 有容差内来流：甲→乙送达来流（Tick1 记入信号），乙→丙无请求 →
	//    下一 Tick 自主命令经统一驱动求解成真实驱动。
	w2 := feedChainWorld()
	seedCoupling(w2)
	w2.Ops = Ops{Requests: []FlowRequest{{Channel: 0, Amount: [ElementCount]int64{2000, 0, 0, 0, 0}}}}
	Advance(w2) // Tick1：来流入乙（「来流之后」的一拍延迟）
	w2.Ops = Ops{}
	rep2 := Advance(w2) // Tick2：乙沿出边自行续推
	af := autonomousFlowOf(rep2)
	if af == 0 {
		t.Fatal("§26.11: 有容差内来流应产生自主命令并成驱动")
	}
	found := false
	for _, tr := range rep2.Transfers {
		if tr.Channel == 1 && tr.Autonomous[Wood] > 0 {
			found = true
			if tr.Actual[Wood] != tr.Autonomous[Wood] {
				t.Errorf("自主条目 Actual 应等于 Autonomous: %d vs %d", tr.Actual[Wood], tr.Autonomous[Wood])
			}
		}
	}
	if !found {
		t.Fatal("自主流量应归因在乙→丙条目上")
	}
	if rep2.Work.AutonomousWork != af {
		t.Errorf("自主做功应入台账: %d vs %d", rep2.Work.AutonomousWork, af)
	}
	checkConservation(t, rep2)

	// ③ 无能源：来流信号仍在（上一 Tick 历史），但乙穴零储能 → 命令
	//    不成驱动、不透支。
	w3 := feedChainWorld()
	seedCoupling(w3)
	w3.Ops = Ops{Requests: []FlowRequest{{Channel: 0, Amount: [ElementCount]int64{2000, 0, 0, 0, 0}}}}
	Advance(w3)                                      // Tick1：来流入历史
	w3.Character.BodyGraph.Acupoints[0].Qi[Wood] = 0 // 断甲（无被动补给）
	w3.Character.BodyGraph.Acupoints[1].Qi[Wood] = 0 // 掏空乙：无能源
	w3.Ops = Ops{}
	rep3 := Advance(w3)
	if af := autonomousFlowOf(rep3); af != 0 {
		t.Fatalf("§26.11: 无能源时命令不得成驱动: %d", af)
	}
	if got := w3.Character.BodyGraph.Acupoints[1].Qi[Wood]; got != 0 {
		t.Fatalf("§26.11: 无能源时乙穴不得透支: %d", got)
	}
	if rep3.Work.AutonomousWork != 0 {
		t.Errorf("无能源不应有自主做功: %d", rep3.Work.AutonomousWork)
	}
	checkConservation(t, rep3)

	// ④ 受控流不伪装自主（T-J3 语义）：进边有来流历史、但出边被主动
	//    请求占用时不产生自主。
	w4 := feedChainWorld()
	seedCoupling(w4)
	w4.Ops = Ops{Requests: []FlowRequest{{Channel: 0, Amount: [ElementCount]int64{2000, 0, 0, 0, 0}}}}
	Advance(w4) // Tick1：来流入历史
	w4.Ops = Ops{Requests: []FlowRequest{{Channel: 1, Amount: [ElementCount]int64{2000, 0, 0, 0, 0}}}}
	rep4 := Advance(w4) // Tick2：乙→丙被手动占用——不得再有自主
	for _, tr := range rep4.Transfers {
		if tr.Autonomous[Wood] > 0 {
			t.Fatalf("T-J3: 出边被主动请求占用时不得产生自主驱动: %+v", tr)
		}
	}

	// ⑤ 局部性（§26.19）：四穴链只内化乙段——丙段未受教学永不自响应，
	//    松手后自主只出现在乙→丙，绝不在丙→丁。
	w5 := feedChain4World()
	reqs := []FlowRequest{
		{Channel: 0, Amount: [ElementCount]int64{2000, 0, 0, 0, 0}},
		{Channel: 1, Amount: [ElementCount]int64{2000, 0, 0, 0, 0}},
	}
	for i := 0; i < 300; i++ {
		w5.Ops = Ops{Requests: reqs}
		Advance(w5)
	}
	cc := feedCoupleOf(w5, 2, 1, 2) // 丙穴耦合（进边 乙→丙，出边 丙→丁）
	if cc == nil {
		t.Fatal("测试前提失效：丙穴耦合缺失")
	}
	for e := 0; e < ElementCount; e++ {
		if cc.Strength[e] != 0 {
			t.Fatalf("§26.11: 丙段未受教学不得内化: strength[%d]=%d", e, cc.Strength[e])
		}
	}
	sawB := false
	for i := 0; i < 30; i++ {
		w5.Ops = Ops{}
		rep5 := Advance(w5)
		for _, tr := range rep5.Transfers {
			if tr.Autonomous[Wood] > 0 {
				switch tr.Channel {
				case 1:
					sawB = true
				case 2:
					t.Fatalf("§26.19: 未内化的丙→丁出现自主驱动: %+v", tr)
				}
			}
		}
	}
	if !sawB {
		t.Fatal("测试前提失效：内化的乙→丙松手期应自主")
	}
	// 结构局部性（§26.19）：全部耦合都只引用本穴的进边/出边各一条——
	// 引擎状态中不存在整条周天路径的耦合值。
	g := &w5.Character.BodyGraph
	for i := range g.Acupoints {
		inDeg, outDeg := 0, 0
		for c := range g.Channels {
			if g.Channels[c].To == i {
				inDeg++
			}
			if g.Channels[c].From == i {
				outDeg++
			}
		}
		cps := g.Acupoints[i].Couplings
		seen := map[[2]int]bool{}
		count := 0
		for k := range cps {
			cp := &cps[k]
			if !cp.validFor(g, i) {
				t.Fatalf("§26.19: 耦合端点不落在本穴: node %d %+v", i, cp)
			}
			if cp.InChannel == cp.OutChannel {
				t.Fatalf("§26.19: 自转移不成耦合: node %d %+v", i, cp)
			}
			if seen[[2]int{cp.InChannel, cp.OutChannel}] {
				t.Fatalf("§26.19: 耦合重复: node %d %+v", i, cp)
			}
			seen[[2]int{cp.InChannel, cp.OutChannel}] = true
			count++
		}
		if count > inDeg*outDeg {
			t.Fatalf("§26.19: 耦合数超过进边×出边: node %d", i)
		}
	}
}

// TestFeedDeterminism（§26.20）：同 seed 重放逐位一致——含耦合学习状态
// （含私有来流历史）与自主驱动输出。
func TestFeedDeterminism(t *testing.T) {
	build := func() *World {
		w := feedChainWorld()
		w.Character.Control = 700 // 控制误差噪声全程参与
		return w
	}
	run := func(w *World) {
		for i := 0; i < 60; i++ {
			teachChain(w, 1, 2000)
		}
		for i := 0; i < 30; i++ {
			w.Ops = Ops{}
			Advance(w)
		}
	}
	w1, w2 := build(), build()
	run(w1)
	run(w2)
	if !reflect.DeepEqual(w1.Character, w2.Character) {
		t.Fatal("耦合状态（含来流历史）在重放下不一致——确定性被破坏")
	}
	if !reflect.DeepEqual(w1.Last, w2.Last) {
		t.Fatal("末 Tick 报告（含自主驱动条目）逐位不一致")
	}
	if autonomousFlowOf(w1.Last) == 0 {
		t.Fatal("测试前提失效：重放场景应含自主驱动")
	}
}

// TestFeedPerceptionWords（评审稿 §22 感知映射）：反馈形成有定性感知词；
// 零数字、零机器词；未内化不预 render（感知隔离律）。
func TestFeedPerceptionWords(t *testing.T) {
	// 未内化：无可感反馈词。
	fresh := feedChainWorld()
	Advance(fresh)
	for i := range fresh.Character.BodyGraph.Acupoints {
		obs := Observe(fresh, 3)
		for _, f := range obs.Points[i].Feelings {
			if strings.Contains(f, "自行续上") || strings.Contains(f, "自行接续") {
				t.Fatalf("未内化的身体不应出现反馈感知词: %q", f)
			}
		}
	}

	// 已内化且有来流：定性词出现。
	tr := feedChainWorld()
	teachChain(tr, 300, 2000)
	for i := 0; i < 5; i++ {
		tr.Ops = Ops{}
		Advance(tr) // 松手期保持来流，反馈词可见
	}
	obs := Observe(tr, 3)
	found := false
	for _, f := range obs.Points[1].Feelings {
		if strings.Contains(f, "自行续上") || strings.Contains(f, "自行接续") {
			found = true
		}
		for _, r := range "0123456789" {
			if strings.ContainsRune(f, r) {
				t.Errorf("感知词不得含数字: %q", f)
			}
		}
	}
	if !found {
		t.Fatalf("§22: 内化穴位的感知应出现「自行续上/接续」类定性词: %+v", obs.Points[1].Feelings)
	}
}
