package cult

// boundary_test.go —— 阶段二收尾块「势差版统一边界交换」验收测试（定稿
// docs/specs/2026-09-26-engine-drive-foundation-v1.md 裁决② + 阶段二范围
// 1/2/4；对应评审稿 docs/cultivation_autonomous_loop_design_v0.2.md）：
//
//	§26.1  双向交换：环境势高 → 净纳入 > 0；环境势低 → 泄出 > 0；
//	       两种方向均在无意识参与（ManualDrive=0）下成立；
//	§28.5  防免费吸气：无势差时零净流；交换受势差/通透性/开放度/
//	       反压/亲和逐项约束，不能无条件增加 Qi；
//	D6/P1  反压抑制：体内势 ≥ 环境势时纳入恒零（满则吸不进，烧蚀闭环
//	       的引擎级保证；cultmc P1 全流程复测另行通过）；
//	§26.14 无补给耗尽：隔绝环境的循环，储备持续下降、流衰减、不永动；
//	§26.15 环境匹配维持：环境势与损耗率匹配时循环长期稳定；
//	守恒   含边界交换净流入（Intake + ExchangeIn − Diffuse）的恒等式
//	       逐 tick 零残差。

import (
	"testing"
)

// boundaryOps 生成三穴环的等量环流请求（每 Tick 每脉 1000/元素）。
func boundaryOps() Ops {
	return Ops{Requests: []FlowRequest{
		{Channel: 0, Amount: [ElementCount]int64{1000, 1000, 1000, 1000, 1000}},
		{Channel: 1, Amount: [ElementCount]int64{1000, 1000, 1000, 1000, 1000}},
		{Channel: 2, Amount: [ElementCount]int64{1000, 1000, 1000, 1000, 1000}},
	}}
}

// totalQiOf 汇总世界全部穴位储气。
func totalQiOf(w *World) int64 {
	var s int64
	for i := range w.Character.BodyGraph.Acupoints {
		s += w.Character.BodyGraph.Acupoints[i].load()
	}
	return s
}

// totalFlowOf 汇总一份报告的实际流量。
func totalFlowOf(rep *TickReport) int64 {
	var s int64
	for _, tr := range rep.Transfers {
		for e := 0; e < ElementCount; e++ {
			s += tr.Actual[e]
		}
	}
	return s
}

// TestBoundaryExchangeBidirectional（评审稿 §26.1）：给定势差与通透性，
// 无意识参与下环境 → Carrier 净纳入 > 0；反向势差时 Carrier → 环境
// 泄出 > 0。意识参与只放大纳入量，不改变方向性（§5.2）。
func TestBoundaryExchangeBidirectional(t *testing.T) {
	// ① 环境势高（3000/元素）+ 体内势为零 + 无意识（Intensity 全 0）
	// → 被动纳入 > 0，意识增量恒 0（回执口径）。
	w := partjRingWorld()
	for i := range w.Character.BodyGraph.Acupoints {
		w.Character.BodyGraph.Acupoints[i].Qi = [ElementCount]int64{}
	}
	w.Ops = Ops{} // ManualDrive = 0：无强度、无请求
	rep := Advance(w)
	if rep.Conservation.ExchangeIn <= 0 {
		t.Fatalf("§26.1: 环境势高时无意识也应净纳入: ExchangeIn=%d", rep.Conservation.ExchangeIn)
	}
	if rep.Conservation.Intake != 0 {
		t.Errorf("无意识时意识驱动增量应恒 0: %d", rep.Conservation.Intake)
	}
	if rep.Conservation.Diffuse != 0 {
		t.Errorf("体内势为零不应有泄出: %d", rep.Conservation.Diffuse)
	}
	if rep.Conservation.QiAfter <= rep.Conservation.QiBefore {
		t.Errorf("§26.1: 纳入应使总量上升: %d -> %d", rep.Conservation.QiBefore, rep.Conservation.QiAfter)
	}
	checkConservation(t, rep)

	// ② 环境势低（绝灵环境）+ 体内有气 + 无意识 → 泄出 > 0（去向=环境，
	// 原 §13 逸散语义），无任何纳入。
	w2 := partjRingWorld()
	w2.Env.Qi = [ElementCount]int64{}
	w2.Ops = Ops{}
	rep2 := Advance(w2)
	if rep2.Conservation.Diffuse <= 0 {
		t.Fatalf("§26.1: 环境势低时无意识也应泄出: Diffuse=%d", rep2.Conservation.Diffuse)
	}
	if rep2.Conservation.ExchangeIn != 0 || rep2.Conservation.Intake != 0 {
		t.Errorf("绝灵环境不应有任何纳入: exchangeIn=%d intake=%d", rep2.Conservation.ExchangeIn, rep2.Conservation.Intake)
	}
	if rep2.Conservation.QiAfter >= rep2.Conservation.QiBefore {
		t.Errorf("§26.1: 泄出应使总量下降: %d -> %d", rep2.Conservation.QiBefore, rep2.Conservation.QiAfter)
	}
	checkConservation(t, rep2)

	// ③ 意识参与（§5.2 主动吐纳 = 放大既有交换）：同世界同环境，
	// intensity 500 的纳入总量应严格大于纯被动纳入。
	w3 := partjRingWorld()
	for i := range w3.Character.BodyGraph.Acupoints {
		w3.Character.BodyGraph.Acupoints[i].Qi = [ElementCount]int64{}
	}
	it := make([]int64, len(w3.Character.BodyGraph.Acupoints))
	for i := range it {
		it[i] = 500
	}
	w3.Ops = Ops{Intensity: it}
	rep3 := Advance(w3)
	passiveTotal := rep.Conservation.ExchangeIn
	drivenTotal := rep3.Conservation.Intake + rep3.Conservation.ExchangeIn
	if rep3.Conservation.Intake <= 0 {
		t.Errorf("意识驱动应产生纳入增量（回执口径）: %d", rep3.Conservation.Intake)
	}
	if drivenTotal <= passiveTotal {
		t.Errorf("意识驱动应放大纳入: 被动 %d vs 驱动 %d", passiveTotal, drivenTotal)
	}
	// 放大口径与 §5.2 一致：perm = KPermBase + KPermDrive×0.5，放大后
	// 总量应约为被动量的 (2 + 996×0.5)/2 = 250 倍量级（下界 100 倍防呆）。
	if drivenTotal < passiveTotal*100 {
		t.Errorf("意识放大倍数异常: 被动 %d vs 驱动 %d", passiveTotal, drivenTotal)
	}
	checkConservation(t, rep3)
}

// TestBoundaryExchangeNoGapNoFlow（评审稿 §28.5 防免费吸气）：交换受
// 势差/通透性/开放度/亲和/反压约束——
//
//	无势差（体内势 = 环境势）→ 纳入、泄出、意识增量全部为零，零净流；
//	有效开放度为零（闭穴）→ 无纳入；
//	亲和为零 → 无纳入。
//
// 不能无条件增加 Qi。
func TestBoundaryExchangeNoGapNoFlow(t *testing.T) {
	// ① 零势差：穴位存量逐元素精确等于环境势。
	w := partjRingWorld()
	for i := range w.Character.BodyGraph.Acupoints {
		w.Character.BodyGraph.Acupoints[i].Qi = [ElementCount]int64{3000, 3000, 3000, 3000, 3000}
	}
	w.Ops = Ops{}
	rep := Advance(w)
	if rep.Conservation.ExchangeIn != 0 || rep.Conservation.Intake != 0 {
		t.Errorf("§28.5: 无势差不应有纳入: exchangeIn=%d intake=%d", rep.Conservation.ExchangeIn, rep.Conservation.Intake)
	}
	if rep.Conservation.Diffuse != 0 {
		t.Errorf("§28.5: 无势差不应有泄出（势差为零）: %d", rep.Conservation.Diffuse)
	}
	checkConservation(t, rep)

	// ② 闭穴（Openness=0 → 有效开放度 0）：有势差也无纳入。
	w2 := partjRingWorld()
	for i := range w2.Character.BodyGraph.Acupoints {
		w2.Character.BodyGraph.Acupoints[i].Qi = [ElementCount]int64{}
		w2.Character.BodyGraph.Acupoints[i].Openness = 0
	}
	it := make([]int64, len(w2.Character.BodyGraph.Acupoints))
	for i := range it {
		it[i] = 1000
	}
	w2.Ops = Ops{Intensity: it}
	rep2 := Advance(w2)
	if rep2.Conservation.ExchangeIn != 0 || rep2.Conservation.Intake != 0 {
		t.Errorf("§28.5: 开度为零不应有纳入: exchangeIn=%d intake=%d", rep2.Conservation.ExchangeIn, rep2.Conservation.Intake)
	}
	checkConservation(t, rep2)

	// ③ 亲和为零：有势差、有开度也无纳入。
	w3 := partjRingWorld()
	for i := range w3.Character.BodyGraph.Acupoints {
		w3.Character.BodyGraph.Acupoints[i].Qi = [ElementCount]int64{}
		w3.Character.BodyGraph.Acupoints[i].Affinity = [ElementCount]int64{}
	}
	w3.Character.Affinity = [ElementCount]int64{}
	w3.Ops = Ops{Intensity: it}
	rep3 := Advance(w3)
	if rep3.Conservation.ExchangeIn != 0 || rep3.Conservation.Intake != 0 {
		t.Errorf("§28.5: 亲和为零不应有纳入: exchangeIn=%d intake=%d", rep3.Conservation.ExchangeIn, rep3.Conservation.Intake)
	}
	checkConservation(t, rep3)
}

// TestBackpressureNearFullNoIntake（D6 烧蚀动机闭环的引擎级保证 / P1）：
// 体内势 ≥ 环境势时纳入恒零——即使意识强度拉满也吸不进；存量低于环境势
// 时纳入量随存量上升单调收敛（势差闭合 + 反压抑制双通道，满则难纳）。
func TestBackpressureNearFullNoIntake(t *testing.T) {
	// ① 近满：木存量 3600 = 环境势 3600 → 势差闭合，满强度吸收也纳入为 0。
	w := partjRingWorld()
	w.Env.Qi = [ElementCount]int64{3600, 0, 0, 0, 0} // 只留木势，隔离变量
	for i := range w.Character.BodyGraph.Acupoints {
		a := &w.Character.BodyGraph.Acupoints[i]
		a.Qi = [ElementCount]int64{}
		a.Qi[Wood] = 3600 // 体内势 = 环境势
		a.Retention = 1000 // 关闭泄出，隔离变量
	}
	it := make([]int64, len(w.Character.BodyGraph.Acupoints))
	for i := range it {
		it[i] = 1000 // 意识强度拉满
	}
	w.Ops = Ops{Intensity: it}
	rep := Advance(w)
	for i := range rep.Points {
		if rep.Points[i].Intake[Wood] != 0 || rep.Points[i].ExchangeIn[Wood] != 0 {
			t.Fatalf("D6: 体内势=环境势时仍吸进了（点 %d: intake=%d exchangeIn=%d）——满则吸不进被破坏",
				i, rep.Points[i].Intake[Wood], rep.Points[i].ExchangeIn[Wood])
		}
	}

	// ② 超满：木存量 5000 > 环境势 3600 → 势差为负，纳入仍为 0。
	for i := range w.Character.BodyGraph.Acupoints {
		a := &w.Character.BodyGraph.Acupoints[i]
		a.Qi = [ElementCount]int64{}
		a.Qi[Wood] = 5000
	}
	rep2 := Advance(w)
	for i := range rep2.Points {
		if rep2.Points[i].Intake[Wood] != 0 || rep2.Points[i].ExchangeIn[Wood] != 0 {
			t.Fatalf("D6: 超满穴位仍吸进了（点 %d）", i)
		}
	}

	// ③ 单调收敛：存量越接近环境势，纳入越少（500 / 1500 / 2500 三档）。
	prevTotal := int64(1 << 62)
	for _, stock := range []int64{500, 1500, 2500} {
		ws := partjRingWorld()
		ws.Env.Qi = [ElementCount]int64{3600, 0, 0, 0, 0}
		for i := range ws.Character.BodyGraph.Acupoints {
			a := &ws.Character.BodyGraph.Acupoints[i]
			a.Qi = [ElementCount]int64{}
			a.Qi[Wood] = stock
			a.Retention = 1000
		}
		ws.Ops = Ops{Intensity: it}
		rs := Advance(ws)
		var tot int64
		for i := range rs.Points {
			tot += rs.Points[i].Intake[Wood] + rs.Points[i].ExchangeIn[Wood]
		}
		if tot >= prevTotal {
			t.Fatalf("D6: 纳入应随存量上升单调下降: stock=%d 纳入 %d ≥ 前档 %d", stock, tot, prevTotal)
		}
		prevTotal = tot
	}
}

// TestIsolatedLoopDepletion（评审稿 §26.14 无补给耗尽）：隔绝环境（绝灵）
// 的循环——储备持续下降、流量衰减、最终趋零，不得永动。逐 tick 守恒核账。
func TestIsolatedLoopDepletion(t *testing.T) {
	w := partjRingWorld()
	w.Env.Qi = [ElementCount]int64{} // 绝灵：无任何环境补给（纳入/纳气恒零）

	const ticks = 250
	initial := totalQiOf(w)
	var firstFlow int64
	for i := 0; i < ticks; i++ {
		w.Ops = boundaryOps()
		rep := Advance(w)
		checkConservation(t, rep)

		if rep.Conservation.ExchangeIn != 0 || rep.Conservation.Entrained != 0 {
			t.Fatalf("tick %d: 绝灵环境不应有环境流入: exchangeIn=%d entrained=%d",
				i, rep.Conservation.ExchangeIn, rep.Conservation.Entrained)
		}
		// 储备持续下降：只有泄出/损耗，没有任何补给。
		if total := totalQiOf(w); total >= initial {
			t.Fatalf("tick %d: 绝灵循环储备应持续低于初值: %d ≥ %d", i, total, initial)
		}
		f := totalFlowOf(rep)
		if i == 0 {
			firstFlow = f
		}
		if i == ticks-1 {
			if total := totalQiOf(w); total >= initial/10 {
				t.Errorf("§26.14: 储备应被循环损耗大幅消耗: 末值 %d ≥ 初值 1/10 (%d)", total, initial)
			}
			if f >= firstFlow/3 {
				t.Errorf("§26.14: 流量应随储备枯竭衰减: 末流 %d ≥ 首流 1/3 (%d)", f, firstFlow)
			}
			t.Logf("无补给循环: 总量 %d → %d，流 %d → %d（%d ticks）", initial, totalQiOf(w), firstFlow, f, ticks)
		}
	}
}

// TestEnvironmentMatchedSustain（评审稿 §26.15 环境维持）：环境势与损耗率
// 匹配时，同一循环可长期稳定——总量收敛到稳定带（不再下降也不发散）、
// 流量持续存在。逐 tick 守恒核账。
func TestEnvironmentMatchedSustain(t *testing.T) {
	w := partjRingWorld() // 环境势 3000/元素，与环损耗量级匹配
	initial := totalQiOf(w)

	const ticks = 500
	totals := make([]int64, 0, ticks)
	for i := 0; i < ticks; i++ {
		w.Ops = boundaryOps()
		rep := Advance(w)
		checkConservation(t, rep)
		totals = append(totals, totalQiOf(w))
	}

	// ① 不枯竭：末段总量远高于绝灵场景的枯竭线，且循环仍在流转。
	const windowStart = ticks - 100
	win := totals[windowStart:]
	lo, hi := win[0], win[0]
	for _, v := range win {
		if v < lo {
			lo = v
		}
		if v > hi {
			hi = v
		}
	}
	if lo < initial/2 {
		t.Errorf("§26.15: 环境匹配下循环不应枯竭: 末窗最低 %d < 初值一半 %d", lo, initial/2)
	}
	// ② 稳定带：末 100 tick 总量波动 < 1%（实测 ~0.15%——环流与交换的
	// 常规呼吸起伏；定点世界里的「长期稳定」）。
	if spread := hi - lo; spread*100 > hi {
		t.Errorf("§26.15: 末窗总量波动超 1%%: %d ~ %d (spread %d)", lo, hi, spread)
	}
	// ③ 流量持续：末 Tick 环流仍存在（循环活着，不是死水）。
	w.Ops = boundaryOps()
	last := Advance(w)
	if f := totalFlowOf(last); f <= 0 {
		t.Errorf("§26.15: 末 Tick 循环流量应 > 0: %d", f)
	}
	checkConservation(t, last)
	t.Logf("环境匹配循环: 总量 %d → 稳定带 [%d, %d]", initial, lo, hi)
}
