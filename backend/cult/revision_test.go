package cult

import (
	"reflect"
	"testing"
)

// revision_test.go —— 修订包（docs/specs/2026-09-21-stage3-revision-circulation-
// refinement.md）新增测试：T1 守恒（A3）、T2 逆流拒绝（A2）、T4 Refinement
// 导出性（D2）、T5 身体跌变（D2）、T-pulse 逆流热脉冲（A2/B4）。
//
// 测试世界（revCharacter/revEnv/revWorld）：三穴两脉的极简拓扑——
//
//	下丹田(0) ← 青关(1) ← 神庭(2)
//	channel 0: 青关→下丹田；channel 1: 神庭→青关
//
// 下丹田是核点（A1 v0.2 核点判据：Capacity 最大穴位，B1 丹田画像）。
// 满 Control 使控制误差 ε ≡ 0，隔离变量。

func revAcupoint(name string, qi [ElementCount]int64) Acupoint {
	return Acupoint{
		Name:       name,
		Affinity:   [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Capacity:   100000,
		Throughput: 5000,
		Retention:  1000, // 满保留，隔离 D3 筛选变量
		Openness:   MicroOne,
		Stability:  1000,
		Activity:   [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Qi:         qi,
	}
}

func revWorld() *World {
	char := Character{
		Affinity:   [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Control:    1000, // 满控制 → ε ≡ 0
		Perception: 500,
		BodyGraph: BodyGraph{
			Acupoints: []Acupoint{
				revAcupoint("下丹田", [ElementCount]int64{5000, 0, 0, 0, 0}),
				revAcupoint("青关", [ElementCount]int64{}),
				revAcupoint("神庭", [ElementCount]int64{}),
			},
			Channels: []Channel{
				{Name: "青关→下丹田", From: 1, To: 0, BaseConductance: 950, Throughput: 5000},
				{Name: "神庭→青关", From: 2, To: 1, BaseConductance: 950, Throughput: 5000},
			},
		},
	}
	env := Environment{
		Qi:          [ElementCount]int64{3000, 1000, 1000, 500, 2000},
		Activity:    [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Temperature: 1000,
	}
	w := NewWorld(char, env)
	// 丹田画像（B1）：Capacity 全图最高（A1 v0.2 核点判据的唯一结构依据）。
	w.Character.BodyGraph.Acupoints[0].Capacity = 200000
	return w
}

// checkConservation 断言一个 Tick 报告的守恒恒等式（修订包 A3：精确整数，
// 无凭空增益；Part J J3 恒等式扩 Entrained 项）。
func checkConservation(t *testing.T, rep *TickReport) {
	t.Helper()
	c := rep.Conservation
	net := c.QiAfter - c.QiBefore
	accounted := c.Intake - c.Diffuse - c.TransferLoss - c.ShengLoss - c.ReverseWork + c.Entrained
	if net != accounted {
		t.Errorf("tick %d 守恒破坏: ΔQi=%d, 归账=%d (intake=%d diffuse=%d transfer=%d sheng=%d reverse=%d entrained=%d)",
			rep.Tick, net, accounted, c.Intake, c.Diffuse, c.TransferLoss, c.ShengLoss, c.ReverseWork, c.Entrained)
	}
}

// TestConservation_MultiTick（T1，A3）：吸收 + 顺流 + 逆流 + 转化 + 逸散
// 混合运行的 30 Tick 内，每 Tick 总量变化逐项归账，无凭空增益。
func TestConservation_MultiTick(t *testing.T) {
	w := revWorld()
	w.Character.BodyGraph.Acupoints[1].Retention = 500 // 打开逸散通道，覆盖 §13
	w.Character.BodyGraph.Acupoints[1].Qi = [ElementCount]int64{2000, 500, 500, 0, 500}

	for i := 0; i < 30; i++ {
		w.Ops = Ops{
			Intensity: []int64{0, 300, 0}, // 青关持续纳气
			Requests: []FlowRequest{
				{Channel: 0, Amount: [ElementCount]int64{800, 100, 100, 0, 100}},          // 顺流 青关→丹田
				{Channel: 0, Reverse: true, Amount: [ElementCount]int64{500, 0, 0, 0, 0}}, // 逆流 丹田→青关
			},
		}
		checkConservation(t, Advance(w))
	}
}

// TestReverse_DantianOnly（T2，A2）：唯有丹田（核点画像，A1 v0.2 核点判据）
// 可发起逆流；非核点发起的逆流传输在引擎层被拒绝（Rejected，无任何流量）。
// 会话层回执属后续任务，此处只锁引擎语义。
func TestReverse_DantianOnly(t *testing.T) {
	w := revWorld()
	w.Ops = Ops{Requests: []FlowRequest{
		// 合法：发起者下丹田是核点（Capacity 全图最大；沿 青关→丹田 反向行走）。
		{Channel: 0, Reverse: true, Amount: [ElementCount]int64{2000, 0, 0, 0, 0}},
		// 非法：发起者青关不是核点（Capacity 低于丹田）——穴与穴之间不得逆流。
		{Channel: 1, Reverse: true, Amount: [ElementCount]int64{500, 0, 0, 0, 0}},
	}}
	rep := Advance(w)

	if len(rep.Transfers) != 2 {
		t.Fatalf("应有两条传输报告: %d", len(rep.Transfers))
	}
	tr := rep.Transfers[0]
	if tr.Rejected {
		t.Fatal("丹田发起的逆流不应被拒绝")
	}
	if tr.Actual[Wood] != 2000 || tr.Delivered[Wood] != 1900 {
		t.Errorf("逆流 Actual/Delivered: %d/%d, want 2000/1900 (cond 0.95)", tr.Actual[Wood], tr.Delivered[Wood])
	}
	if tr.WorkCost[Wood] <= 0 {
		t.Errorf("逆流应产生做功代价: got %d", tr.WorkCost[Wood])
	}
	// 做功代价期望值：2000 × 0.2 × (1+0) × (1-0.95) = 20。
	if tr.WorkCost[Wood] != 20 {
		t.Errorf("做功代价: got %d, want 20", tr.WorkCost[Wood])
	}

	rej := rep.Transfers[1]
	if !rej.Rejected {
		t.Error("非丹田发起的逆流应被拒绝")
	}
	if rej.Actual[Wood] != 0 || rej.Delivered[Wood] != 0 {
		t.Errorf("被拒绝的逆流不应有任何流量: actual=%d delivered=%d", rej.Actual[Wood], rej.Delivered[Wood])
	}
	if got := w.Character.BodyGraph.Acupoints[2].Qi[Wood]; got != 0 {
		t.Errorf("被拒绝的逆流不应改变神庭存量: got %d", got)
	}

	// 存量核对（含 §10 相生转化的同 Tick 快照结算；Refinement=0 → 乘子=1；
	// Part J J3：逆流入库 1900 在接收端青关卷入环境纳气 240
	// = 2000×3.0(环境木密度)×0.04，转化快照按 2140 计）：
	//   丹田：5000 − 逆流 2000 − 做功 20 = 2980 → 木→火转化 119（2980×0.04）；
	//   青关：入库 1900 + 途经纳气 240 = 2140 → 转化 86（2140×0.04）。
	if got := w.Character.BodyGraph.Acupoints[0].Qi[Wood]; got != 2980-119 {
		t.Errorf("丹田存量: got %d, want %d", got, 2980-119)
	}
	if got := w.Character.BodyGraph.Acupoints[1].Qi[Wood]; got != 2140-86 {
		t.Errorf("青关存量: got %d, want %d", got, 2140-86)
	}

	// 逆流参与的 Tick 同样守恒。
	checkConservation(t, rep)
}

// TestReverse_ThermalLoadPulse（T-pulse，A2/B4）：逆流使目的地 ThermalLoad
// 上升（对照世界无逆流则为 0）；清空储气停止热源后，散热使其单调回落
// （衰减到整数舍入地板 ≤5）。
func TestReverse_ThermalLoadPulse(t *testing.T) {
	w := revWorld()
	w.Ops = Ops{Requests: []FlowRequest{
		{Channel: 0, Reverse: true, Amount: [ElementCount]int64{4000, 0, 0, 0, 0}},
	}}
	rep := Advance(w)
	peak := rep.Points[1].ThermalLoad // 目的地 = 青关
	if peak <= 0 {
		t.Fatalf("逆流后目的地 ThermalLoad 应上升: got %d", peak)
	}

	// 对照：除无逆流请求外完全相同的世界，目的地 ThermalLoad 应为 0
	// （逆流做功热是热源，不是呼吸性背景）。
	ctrl := revWorld()
	ctrl.Ops = Ops{}
	if got := Advance(ctrl).Points[1].ThermalLoad; got != 0 {
		t.Errorf("无逆流对照世界目的地 ThermalLoad 应为 0: got %d", got)
	}

	// 清空储气（停掉 §10 转化热源）→ 纯散热：单调不升，回落到近零。
	w.Ops = Ops{}
	last := peak
	for i := 0; i < 60; i++ {
		for j := range w.Character.BodyGraph.Acupoints {
			w.Character.BodyGraph.Acupoints[j].Qi = [ElementCount]int64{}
		}
		cur := Advance(w).Points[1].ThermalLoad
		if cur > last {
			t.Fatalf("散热阶段 ThermalLoad 不应回升: %d -> %d", last, cur)
		}
		last = cur
	}
	if last > 5 {
		t.Errorf("散热 60 Tick 后 ThermalLoad 应近零（舍入地板 ≤5）: got %d", last)
	}
}

// TestRefinement_Derived（T4，D2）：导出量——同状态重算一致、不写状态；
// 高亲和构成 vs 均匀构成单调（均匀≈0，全主行=1000，中间构成单调升）。
func TestRefinement_Derived(t *testing.T) {
	// 金手指标定亲和（有梯度）：affEff = {1500, 600, 810, 638, 1100}，
	// MatchMin = 930，MatchMax = 1500。
	char := goldenCharacter()
	w := NewWorld(char, goldenEnv())
	pt := &w.Character.BodyGraph.Acupoints[0]

	// 均匀构成 → Match = MatchMin → Refine = 0。
	pt.Qi = [ElementCount]int64{1000, 1000, 1000, 1000, 1000}
	rUniform := DeriveRefinement(w, 0)
	if rUniform != 0 {
		t.Errorf("均匀构成 Refine 应为 0: got %d", rUniform)
	}

	// 全木 → Match = MatchMax → Refine = 1000。
	pt.Qi = [ElementCount]int64{5000, 0, 0, 0, 0}
	rAll := DeriveRefinement(w, 0)
	if rAll != 1000 {
		t.Errorf("全木构成 Refine 应为 1000: got %d", rAll)
	}

	// 木/水配比单调：木占比 0.4 → 0.6 → 0.8，Refine 严格递增。
	prev := int64(-1)
	for _, woodShare := range []int64{400, 600, 800} {
		waterShare := 1000 - woodShare
		pt.Qi = [ElementCount]int64{}
		pt.Qi[Wood] = 5 * woodShare
		pt.Qi[Water] = 5 * waterShare
		r := DeriveRefinement(w, 0)
		if r <= prev {
			t.Errorf("木占比 %d: Refine 应递增: %d -> %d", woodShare, prev, r)
		}
		prev = r
	}

	// 导出性：重算一致，且不写 World 状态。
	pt.Qi = [ElementCount]int64{5000, 0, 0, 0, 0}
	before := *pt
	r1 := DeriveRefinement(w, 0)
	r2 := DeriveRefinement(w, 0)
	if r1 != r2 {
		t.Errorf("同状态重算不一致: %d vs %d", r1, r2)
	}
	if !reflect.DeepEqual(*pt, before) {
		t.Error("DeriveRefinement 修改了穴位状态——导出量不得落存储")
	}
}

// TestRefinement_BodyDrift（T5，D2）：身体跌变即重算——降低穴位亲和或
// 提高损伤后，同一构成的 Refine 不升（主行构成下严格下降）。
func TestRefinement_BodyDrift(t *testing.T) {
	newW := func() *World {
		w := NewWorld(goldenCharacter(), goldenEnv())
		w.Character.BodyGraph.Acupoints[0].Qi = [ElementCount]int64{5000, 0, 0, 0, 0}
		return w
	}
	base := DeriveRefinement(newW(), 0)
	if base != 1000 {
		t.Fatalf("基线（全木、无损）Refine 应为 1000: got %d", base)
	}

	// 降低穴位主行亲和：木亲和 1200 → 600（affEff 向量变形 → Refine 严格下降）。
	w := newW()
	w.Character.BodyGraph.Acupoints[0].Affinity[Wood] = 600
	if r := DeriveRefinement(w, 0); r >= base {
		t.Errorf("降低主行亲和后 Refine 应下降: %d -> %d", base, r)
	}

	// 提高损伤：Damage 0 → 500。损伤对诸元素亲和是同一比例 (1-Damage)，
	// 对纯构成的重算值是尺度不变的——静态口径"不升"即通过；损伤对
	// Refine 的实质作用通道是 D3 筛选弱化（留与散的比例漂移 → 构成
	// 随时间变驳杂），见下方 D3 后果断言。
	w = newW()
	w.Character.BodyGraph.Acupoints[0].Damage = 500
	if r := DeriveRefinement(w, 0); r > base {
		t.Errorf("提高损伤后 Refine 不应上升: %d -> %d", base, r)
	}

	// D3 后果（损伤的身体跌变通道）：损伤削弱筛选（Retention_effective
	// 随 (1-Damage) 缩水）→ 逸散更多、吸收更少 → 同样操作下存气更少。
	run := func(damage int64) int64 {
		wr := newW()
		wr.Character.BodyGraph.Acupoints[0].Damage = damage
		for i := 0; i < 10; i++ {
			wr.Ops = Ops{Intensity: []int64{600}}
			Advance(wr)
		}
		return wr.Character.BodyGraph.Acupoints[0].load()
	}
	if got := run(500); got >= run(0) {
		t.Errorf("损伤身体的留气能力不应更高: 损伤 %d vs 完好 %d", got, run(0))
	}
}
