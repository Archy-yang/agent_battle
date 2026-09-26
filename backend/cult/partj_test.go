package cult

// partj_test.go —— Part J 被动环流引擎层测试（docs/specs/2026-09-22-partj-
// passive-circulation.md J7 测试计划 T-J4 守恒扩项 / T-J5 金手指复核）。
//
// T-J1/T-J2/T-J3/T-J6 在 ctmatch 包（会话层口径，真实 tick 流驱动）。

import (
	"testing"
)

// partjRingWorld 构造三穴单行素环（A→B→C→A，全顺流），诸穴等容等压，
// 环境有灵气（途经纳气可发生）。retention 打开逸散通道。
func partjRingWorld() *World {
	pt := func(name string, qi int64) Acupoint {
		return Acupoint{
			Name:       name,
			Affinity:   [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
			Capacity:   30000,
			Throughput: 10000,
			Retention:  900,
			Openness:   MicroOne,
			Stability:  950,
			Activity:   [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
			Adaptation: [ElementCount]int64{1000000, 1000000, 1000000, 1000000, 1000000},
			Qi:         [ElementCount]int64{qi, qi, qi, qi, qi},
		}
	}
	char := Character{
		Affinity:   [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Control:    1000,
		Perception: 500,
		BodyGraph: BodyGraph{
			// 驱动画像（0.5.0）：A 显著主导（12000 > 3000×2）——A 发起的
			// 核点逆流（C→A 反向）在 0.5.0 判据下依旧合法。
			Acupoints: []Acupoint{pt("A", 6000), pt("B", 6000), pt("C", 6000)},
			Channels: []Channel{
				{Name: "A→B", From: 0, To: 1, BaseConductance: 980, Throughput: 10000},
				{Name: "B→C", From: 1, To: 2, BaseConductance: 980, Throughput: 10000},
				{Name: "C→A", From: 2, To: 0, BaseConductance: 980, Throughput: 10000},
			},
		},
	}
	env := Environment{
		Qi:          [ElementCount]int64{3000, 3000, 3000, 3000, 3000},
		Activity:    [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Temperature: 1000,
	}
	w := NewWorld(char, env)
	// 驱动画像（0.5.0）：普通穴 3000，A 覆盖为 12000（全图唯一显著主导）。
	for i := range w.Character.BodyGraph.Acupoints {
		w.Character.BodyGraph.Acupoints[i].DriveCapacity = 3000
	}
	w.Character.BodyGraph.Acupoints[0].DriveCapacity = 12000
	return w
}

// TestPartJConservationEntrained（T-J4）：被动输运 + 途经纳气 + 主动引流
// + 逆流混合运行，逐 Tick 守恒恒等式（Part J 扩 Entrained 项）零残差。
func TestPartJConservationEntrained(t *testing.T) {
	w := partjRingWorld()
	seed := uint64(42)
	for i := 0; i < 60; i++ {
		seed = seed*6364136223846793005 + 1442695040888963407
		var reqs []FlowRequest
		if i%3 == 0 {
			// 偶发主动引流（A→B）与核点逆流（A 发起，C→A 反向）。
			reqs = append(reqs,
				FlowRequest{Channel: 0, Amount: [ElementCount]int64{800, 800, 800, 800, 800}},
				FlowRequest{Channel: 2, Reverse: true, Amount: [ElementCount]int64{400, 0, 0, 0, 0}},
			)
		}
		w.NoiseSeed = seed % 1000
		w.Ops = Ops{Requests: reqs} // 其余 Tick 纯被动环流
		rep := Advance(w)
		c := rep.Conservation
		net := c.QiAfter - c.QiBefore
		accounted := c.Intake - c.Diffuse - c.TransferLoss - c.ShengLoss - c.DriveWork + c.Entrained
		if net != accounted {
			t.Fatalf("tick %d 守恒破坏: ΔQi=%d, 归账=%d (intake=%d diffuse=%d transfer=%d sheng=%d reverse=%d entrained=%d)",
				i, net, accounted, c.Intake, c.Diffuse, c.TransferLoss, c.ShengLoss, c.DriveWork, c.Entrained)
		}
	}
}

// TestPartJEntrainedReportedAndCapped（T-J4 附属）：途经纳气随实际流量
// 发生并计入报告与守恒账；受 (1-LoadRatio) 抑制——穴位越满卷入越少。
func TestPartJEntrainedReportedAndCapped(t *testing.T) {
	// 有流必有纳气：给环一个初始压差（A 满、B/C 空），松手看被动流与纳气。
	w := partjRingWorld()
	w.Character.BodyGraph.Acupoints[0].Qi = [ElementCount]int64{18000, 18000, 18000, 18000, 18000}
	w.Character.BodyGraph.Acupoints[1].Qi = [ElementCount]int64{}
	w.Character.BodyGraph.Acupoints[2].Qi = [ElementCount]int64{}
	rep := Advance(w)
	if rep.Conservation.Entrained <= 0 {
		t.Fatalf("有被动流应有途经纳气: entrained=%d", rep.Conservation.Entrained)
	}
	var passive int64
	for _, tr := range rep.Transfers {
		for e := 0; e < ElementCount; e++ {
			passive += tr.Passive[e]
			if tr.Passive[e] > 0 && tr.Actual[e] != tr.Passive[e] {
				t.Fatalf("被动条目 Actual 应等于 Passive: %d vs %d", tr.Actual[e], tr.Passive[e])
			}
		}
	}
	if passive <= 0 {
		t.Fatal("压差驱动应有被动流")
	}

	// 满穴抑制：全部穴位顶满（LoadRatio→1）时纳气趋零。
	w2 := partjRingWorld()
	for i := range w2.Character.BodyGraph.Acupoints {
		w2.Character.BodyGraph.Acupoints[i].Qi = [ElementCount]int64{29000, 29000, 29000, 29000, 29000}
	}
	w2.Ops = Ops{}
	rep2 := Advance(w2)
	if rep2.Conservation.Entrained > rep.Conservation.Entrained/10 {
		t.Errorf("近满穴位的途经纳气应被 BackPressure 抑制: got %d (开放穴 %d)",
			rep2.Conservation.Entrained, rep.Conservation.Entrained)
	}
}

// TestPartJGoldenSingleAcupointUnchanged（T-J5 金手指复核）：单穴无经脉
// 世界（无压差、无流、无纳气路径）下，Part J 各新项恒等 0，stage_3 §46-48
// 黄金用例数值逐位不变。
func TestPartJGoldenSingleAcupointUnchanged(t *testing.T) {
	w := NewWorld(goldenCharacter(), goldenEnv())
	w.Ops = Ops{Intensity: []int64{600}}

	rep := Advance(w)
	p := &rep.Points[0]
	assertNear(t, "木吸收", p.Intake[Wood], 7200)
	assertNear(t, "水吸收", p.Intake[Water], 2112)
	if rep.Conservation.Entrained != 0 {
		t.Errorf("无经脉世界 Entrained 应为 0: %d", rep.Conservation.Entrained)
	}
	if len(rep.Transfers) != 0 {
		t.Errorf("无经脉世界不应有传输报告: %d", len(rep.Transfers))
	}
	for i := range p.Entrained {
		if p.Entrained[i] != 0 {
			t.Fatalf("单穴世界途经纳气应为 0")
		}
	}

	// 双 Tick 反压链不变（§48）。
	rep2 := Advance(w)
	assertNear(t, "Tick2 BackPressure", rep2.Points[0].BackPressure, 493)
	assertNear(t, "Tick2 木吸收", rep2.Points[0].Intake[Wood], 3550)
}

// TestPartJNoFlowNoPassive（T-J5 附属）：等压无流世界（诸穴同负载比、
// 无 FlowPrev）下被动输运恒零——金手指"无被动场景恒等 1 路径"。
func TestPartJNoFlowNoPassive(t *testing.T) {
	w := partjRingWorld()
	w.Ops = Ops{}
	for i := 0; i < 5; i++ {
		rep := Advance(w)
		for _, tr := range rep.Transfers {
			for e := 0; e < ElementCount; e++ {
				if tr.Passive[e] != 0 {
					t.Fatalf("tick %d: 等压无流世界出现被动流 %d（元素 %d）", i, tr.Passive[e], e)
				}
			}
		}
	}
}

// TestPartJInertiaRelayDecay（R4 惯性项）：无环境灵气、等压三穴环上，
// 预置连续环流的 FlowPrev 后松手——被动环流靠惯性接力延续若干 Tick
// （不是瞬间归零），且无源有耗，逐段衰减最终归零（不放大、不永动）。
// 全程守恒逐 Tick 核账（T-J4 口径）。
func TestPartJInertiaRelayDecay(t *testing.T) {
	w := partjRingWorld()
	w.Env.Qi = [ElementCount]int64{} // 关掉途经纳气，隔离出纯惯性衰减
	const seedFlow = 500
	for c := range w.Character.BodyGraph.Channels {
		w.Character.BodyGraph.Channels[c].FlowPrev = [ElementCount]int64{seedFlow, seedFlow, seedFlow, seedFlow, seedFlow}
	}
	w.Ops = Ops{}

	const totalTicks = 60
	flows := make([]int64, 0, totalTicks)
	for i := 0; i < totalTicks; i++ {
		rep := Advance(w)
		var f int64
		for _, tr := range rep.Transfers {
			for e := 0; e < ElementCount; e++ {
				if tr.Passive[e] > 0 && tr.Actual[e] != tr.Passive[e] {
					t.Fatalf("tick %d: 被动条目 Actual 应等于 Passive: %d vs %d", i, tr.Actual[e], tr.Passive[e])
				}
				f += tr.Passive[e]
			}
		}
		flows = append(flows, f)
		if i == 0 && f <= 0 {
			t.Fatal("惯性接力失效：松手首 Tick 环流即归零")
		}
		c := rep.Conservation
		if net, acc := c.QiAfter-c.QiBefore, c.Intake-c.Diffuse-c.TransferLoss-c.ShengLoss-c.DriveWork+c.Entrained; net != acc {
			t.Fatalf("tick %d 守恒破坏: ΔQi=%d 归账=%d", i, net, acc)
		}
	}

	// 有耗无源：环流不放大、不永动。流量被源穴储气份额封顶（KQiShare），
	// 随库存损耗单调回落——衰减时间常数由结构决定（J5：Conductance 越高
	// 衰减越慢），故不要求窗内归零，只要求单调回落且严格下降。
	if tail := flows[len(flows)-1]; tail >= flows[1] {
		t.Errorf("无源环流应随库存损耗回落: 尾流 %d >= 首窗 %d", tail, flows[1])
	}
	for i := 2; i < len(flows); i++ {
		if flows[i] > flows[i-1] {
			t.Errorf("tick %d: 无源环流应单调回落: %d -> %d", i, flows[i-1], flows[i])
			break
		}
	}
	// 不放大：接力瞬态受储气份额封顶，全程流量不超过初值的有界倍数。
	var maxF int64
	for _, f := range flows {
		if f > maxF {
			maxF = f
		}
	}
	initial := int64(seedFlow) * ElementCount * int64(len(w.Character.BodyGraph.Channels))
	if maxF > 2*initial {
		t.Errorf("惯性接力不应放大环流: max=%d > 2×初值 %d", maxF, initial)
	}
	t.Logf("惯性衰减序列（前 12 Tick）：%v", flows[:12])
}
