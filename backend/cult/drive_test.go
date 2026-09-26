package cult

// drive_test.go —— 阶段二「驱动的代价」验收测试（定稿
// docs/specs/2026-09-26-engine-drive-foundation-v1.md 阶段二验收；对应评审稿
// docs/cultivation_autonomous_loop_design_v0.2.md §26.3-§26.6）：
//
//	§26.3 驱动容量限幅：同请求下高驱动容量节点输出更大实际驱动、低者被
//	      限幅，不得用存储容量替代；
//	§26.4 做功守恒：有实际驱动必扣可用能源；无能源则实际驱动归零，无凭空
//	      做功；
//	§26.5 正常驱动低应变：安全驱动比下做功>0 而应变≈低；
//	§26.6 过驱：超安全比时应变↑、废热↑、稳定↓；
//	+ 核点判据迁移（定稿阶段二范围 1 / 裁决①）：核点 = 驱动容量显著主导
//	  者，与存储容量脱钩；无主导者则无核点；
//	+ 做功台账（评审稿 §15/§24.6）与内化度派生量；
//	+ 无驱动场景恒零（金手指：无驱动路径既有断言不变）。
//
// 测试世界（driveWorld）：双穴单向源→汇，满传导满开度、满控制（ε≡0）、
// 源汇火亲和为 0（断木→火相生，隔离 §10 转化热）、满保留（无逸散）——
// 驱动做功是唯一热源/唯一额外支出，逐项可归因。

import (
	"testing"
)

func driveAcupoint(name string, capacity, driveCap int64) Acupoint {
	return Acupoint{
		Name:            name,
		Affinity:        [ElementCount]int64{1000, 0, 1000, 1000, 1000}, // 火亲和 0：断相生
		Capacity:        capacity,
		Throughput:      10000,
		DriveCapacity:   driveCap,
		DriveEfficiency: 1000,
		Retention:       1000, // 满保留，隔离 §13
		Openness:        MicroOne,
		Stability:       950,
		Activity:        [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
	}
}

func driveWorld(srcDrive int64, srcQi int64) *World {
	char := Character{
		Affinity:   [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Control:    1000, // 满控制 → ε ≡ 0
		Perception: 500,
		BodyGraph: BodyGraph{
			Acupoints: []Acupoint{
				// 容量远大于储气（LR ≤ 0.4）：隔离 §17 过载应变——本世界的
				// 应变只来自驱动（过驱/突变），不来自负载过载。
				driveAcupoint("驱源", 1000000, srcDrive),
				driveAcupoint("受汇", 5000000, 10000),
			},
			Channels: []Channel{
				{Name: "驱源→受汇", From: 0, To: 1, BaseConductance: 1000, Throughput: 10000},
			},
		},
	}
	w := NewWorld(char, Environment{Temperature: 1000}) // 环境无灵气：无纳气项
	w.Character.BodyGraph.Acupoints[0].Qi[Wood] = srcQi
	return w
}

// guideOps 生成一 Tick 的单元素顺流请求。
func guideOps(amount int64) Ops {
	return Ops{Requests: []FlowRequest{{Channel: 0, Amount: [ElementCount]int64{amount, 0, 0, 0, 0}}}}
}

// TestDriveCapacityClampsActualDrive（§26.3）：同请求、同存储容量、同管线
// 容量下——高驱动容量节点输出更大实际驱动，低驱动容量节点被限幅到其驱动
// 容量；存储容量全程富余，证明限幅来自驱动维度而非存储维度。
func TestDriveCapacityClampsActualDrive(t *testing.T) {
	const request = int64(4000)

	hi := driveWorld(10000, 100000)
	hi.Ops = guideOps(request)
	trHi := Advance(hi).Transfers[0]
	if trHi.Rejected {
		t.Fatal("顺流请求不应被拒绝")
	}
	// 高驱动容量（10000 ≥ 请求 4000）：管线容量 10000 亦不 binding，
	// 实际驱动 = 完整请求。
	if trHi.Actual[Wood] != request {
		t.Errorf("高驱动容量实际驱动: got %d, want %d（请求应足额兑现）", trHi.Actual[Wood], request)
	}

	lo := driveWorld(1500, 100000)
	lo.Ops = guideOps(request)
	trLo := Advance(lo).Transfers[0]
	// 低驱动容量（1500 < 请求 4000）：实际驱动被限幅到驱动容量。
	if trLo.Actual[Wood] != 1500 {
		t.Errorf("低驱动容量限幅: got %d, want 1500（驱动容量是唯一 binding 项）", trLo.Actual[Wood])
	}
	if trLo.Capacity[Wood] != trHi.Capacity[Wood] {
		t.Errorf("两世界管线容量应相同: %d vs %d", trLo.Capacity[Wood], trHi.Capacity[Wood])
	}
	if lo.Character.BodyGraph.Acupoints[0].Capacity != hi.Character.BodyGraph.Acupoints[0].Capacity {
		t.Error("两世界存储容量应相同——限幅不得由存储容量解释")
	}
	if trLo.Actual[Wood] >= trHi.Actual[Wood] {
		t.Fatalf("同请求下高驱动容量应输出更大实际驱动: %d vs %d", trLo.Actual[Wood], trHi.Actual[Wood])
	}

	// 做功二次律：高驱动的做功严格更大（§10.1）。
	if trHi.DriveWork[Wood] <= trLo.DriveWork[Wood] {
		t.Errorf("实际驱动更大者做功应更大: hi=%d lo=%d", trHi.DriveWork[Wood], trLo.DriveWork[Wood])
	}
}

// TestDriveWorkConservation（§26.4）：有实际驱动必扣可用能源（做功=能源×1）
// ；能源不足以支付做功时实际驱动衰减；无能源则实际驱动归零。任何情形下
// 储量不为负、守恒逐项归账。
func TestDriveWorkConservation(t *testing.T) {
	// ① 充裕能源：实际驱动足额，做功从驱动穴位扣除。
	w := driveWorld(10000, 100000)
	w.Ops = guideOps(2000)
	before := w.Character.BodyGraph.Acupoints[0].Qi[Wood]
	rep := Advance(w)
	tr := rep.Transfers[0]
	if tr.Actual[Wood] != 2000 || tr.DriveWork[Wood] != 8 {
		t.Fatalf("实际驱动/做功: got %d/%d, want 2000/8（k_work×F² = 0.002×2²）", tr.Actual[Wood], tr.DriveWork[Wood])
	}
	after := w.Character.BodyGraph.Acupoints[0].Qi[Wood]
	// 源穴高压差会另有被动输运外流（势差自然释放，不付做功），一并归账。
	var passive int64
	for _, t2 := range rep.Transfers {
		passive += t2.Passive[Wood]
	}
	if d := before - after; d != tr.Actual[Wood]+tr.DriveWork[Wood]+passive {
		t.Errorf("储量净减应 = 实际驱动 + 做功 + 被动流: %d vs %d+%d+%d", d, tr.Actual[Wood], tr.DriveWork[Wood], passive)
	}
	if rep.Conservation.DriveWork != tr.DriveWork[Wood] {
		t.Errorf("守恒账驱动做功: got %d, want %d", rep.Conservation.DriveWork, tr.DriveWork[Wood])
	}
	checkConservation(t, rep)

	// ② 能源紧张：实际驱动衰减，使「流量+做功」恰不透支存量。
	w2 := driveWorld(10000, 2900)
	w2.Ops = guideOps(2900)
	rep2 := Advance(w2)
	tr2 := rep2.Transfers[0]
	if tr2.Actual[Wood] >= 2900 {
		t.Fatalf("能源不足时实际驱动应衰减: got %d", tr2.Actual[Wood])
	}
	if got := w2.Character.BodyGraph.Acupoints[0].Qi[Wood]; got < 0 {
		t.Fatalf("驱动穴位储量不得为负: %d", got)
	}
	checkConservation(t, rep2)

	// ③ 无能源：实际驱动归零，无凭空做功。
	w3 := driveWorld(10000, 0)
	w3.Ops = guideOps(2900)
	rep3 := Advance(w3)
	if rep3.Transfers[0].Actual[Wood] != 0 {
		t.Errorf("无能源实际驱动应归零: got %d", rep3.Transfers[0].Actual[Wood])
	}
	if rep3.Conservation.DriveWork != 0 {
		t.Errorf("无能源不应有做功: got %d", rep3.Conservation.DriveWork)
	}
	checkConservation(t, rep3)
}

// TestNormalDriveLowStrain（§26.5）：安全驱动比（0.3）内的平稳驱动——
// 做功 > 0（耗能）而驱动应变 = 0（无过驱、无突变）。「耗能」与「受伤」
// 是两个概念（评审稿 §10.2）。
func TestNormalDriveLowStrain(t *testing.T) {
	w := driveWorld(10000, 100000)
	w.Ops = guideOps(3000)
	Advance(w) // 首 Tick：突变项一次性计入（加力从无到有），不属平稳段
	prev := int64(0)
	for i := 0; i < 4; i++ {
		rep := Advance(w)
		p := &rep.Points[0]
		if p.DriveWork <= 0 {
			t.Errorf("tick %d: 平稳驱动应有做功（耗能）: %d", i, p.DriveWork)
		}
		if p.DriveStrain != 0 {
			t.Errorf("tick %d: 安全比内平稳驱动应变应为 0（无过驱、无突变）: %d", i, p.DriveStrain)
		}
		// 首 Tick 突变/冲击造成的瞬时回落后，平稳段稳定性只升不降。
		if prev > 0 && p.Stability < prev {
			t.Errorf("tick %d: 平稳驱动下稳定性应回升: %d -> %d", i, prev, p.Stability)
		}
		prev = p.Stability
	}
}

// TestOverdriveHeatAndStrain（§26.6）：同体同源双世界，温和（驱动比 0.3）
// vs 过驱（驱动比 1.0）平稳运行 30 Tick——过驱侧应变↑、废热（ThermalLoad）↑、
// 稳定↓。断相生与满保留使驱动做功是两世界唯一的差异热源。
func TestOverdriveHeatAndStrain(t *testing.T) {
	gentle := driveWorld(10000, 400000)
	over := driveWorld(10000, 400000)

	// 预热一 Tick（加力从无到有的突变属瞬态，不属平稳过驱段）。
	gentle.Ops = guideOps(3000)
	over.Ops = guideOps(10000)
	Advance(gentle)
	Advance(over)

	for i := 0; i < 30; i++ {
		gentle.Ops = guideOps(3000) // 驱动比 0.3
		over.Ops = guideOps(10000)  // 驱动比 1.0
		g := Advance(gentle)
		o := Advance(over)
		if g.Points[0].DriveStrain != 0 {
			t.Fatalf("tick %d: 温和侧应变应恒 0: %d", i, g.Points[0].DriveStrain)
		}
		// 前 10 Tick（损伤劣化尚未反噬驱动能力前）：过驱应变恒显著。
		// 此后过驱自毁（稳定耗尽 → 损伤 → 有效驱动容量缩水 → 实际驱动
		// 衰减）——过驱不可持续本身就是本命题的后果面，不再断言恒定应变。
		if i < 10 && o.Points[0].DriveStrain < 50 {
			t.Errorf("tick %d: 过驱侧应变应显著: %d（OverDrive²×1.5 ≈ 0.06）", i, o.Points[0].DriveStrain)
		}
	}

	gTL, oTL := gentle.Last.Points[0].ThermalLoad, over.Last.Points[0].ThermalLoad
	if oTL <= 2*gTL {
		t.Errorf("过驱废热应显著高于温和驱动: 过驱 %d vs 温和 %d", oTL, gTL)
	}
	gStab := gentle.Character.BodyGraph.Acupoints[0].Stability
	oStab := over.Character.BodyGraph.Acupoints[0].Stability
	if oStab >= gStab {
		t.Fatalf("过驱稳定性应被显著打压: 过驱 %d vs 温和 %d", oStab, gStab)
	}
	if gStab-oStab < 300 {
		t.Errorf("过驱与温和的稳定差距应显著: %d vs %d", oStab, gStab)
	}
}

// TestCoreCriterionDriveCapacityDominance（定稿阶段二范围 1 / 裁决①）：
// 核点判据与存储容量脱钩——存储最大者不是核点；驱动容量显著主导者是
// 核点并独享逆流发起资格；无显著主导者（含全零画像）则无核点。
func TestCoreCriterionDriveCapacityDominance(t *testing.T) {
	// 存储最大（巨储 500000）≠ 核点；驱动容量显著主导（强驱 12000 > 1000×2）= 核点。
	char := Character{
		Affinity:   [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Control:    1000,
		Perception: 500,
		BodyGraph: BodyGraph{
			Acupoints: []Acupoint{
				driveAcupoint("巨储", 500000, 1000),
				driveAcupoint("强驱", 1000, 12000),
			},
			Channels: []Channel{
				{Name: "巨储→强驱", From: 0, To: 1, BaseConductance: 900, Throughput: 10000},
				{Name: "强驱→巨储", From: 1, To: 0, BaseConductance: 900, Throughput: 10000},
			},
		},
	}
	w := NewWorld(char, Environment{Temperature: 1000})
	w.Character.BodyGraph.Acupoints[0].Qi[Wood] = 20000
	w.Character.BodyGraph.Acupoints[1].Qi[Wood] = 20000

	if core := CorePointOf(&w.Character.BodyGraph); core != 1 {
		t.Fatalf("核点应为驱动容量显著主导者（下标 1），got %d（存储最大者是下标 0）", core)
	}

	// 逆流发起资格随核点迁移：channel 0（巨储→强驱）的逆流发起端是强驱
	//（核点）→ 放行；channel 1（强驱→巨储）的逆流发起端是巨储（非核点）
	// → 拒绝。
	w.Ops = Ops{Requests: []FlowRequest{
		{Channel: 0, Reverse: true, Amount: [ElementCount]int64{2000, 0, 0, 0, 0}},
		{Channel: 1, Reverse: true, Amount: [ElementCount]int64{2000, 0, 0, 0, 0}},
	}}
	rep := Advance(w)
	if rep.Transfers[0].Rejected || rep.Transfers[0].WorkCost[Wood] <= 0 {
		t.Errorf("核点发起的逆流应放行并付做功: %+v", rep.Transfers[0])
	}
	if !rep.Transfers[1].Rejected {
		t.Errorf("非核点（存储最大者）发起的逆流应被拒绝: %+v", rep.Transfers[1])
	}
	checkConservation(t, rep)

	// 同构画像（驱动容量并列）：无显著主导者 → 无核点，逆流一律拒绝。
	w2 := driveWorld(5000, 100000)
	if core := CorePointOf(&w2.Character.BodyGraph); core != -1 {
		t.Errorf("无显著主导者的世界不应有核点: got %d", core)
	}
	w2.Ops = Ops{Requests: []FlowRequest{{Channel: 0, Reverse: true, Amount: [ElementCount]int64{1000, 0, 0, 0, 0}}}}
	if tr := Advance(w2).Transfers[0]; !tr.Rejected {
		t.Errorf("无核点世界的逆流应被拒绝: %+v", tr)
	}
}

// TestWorkLedgerAttribution（定稿阶段二范围 4；评审稿 §15/§24.6）：台账三分
// 归因 + 守恒项对账；自主做功本阶段恒零、内化度恒 0（派生量，不落存储）。
func TestWorkLedgerAttribution(t *testing.T) {
	w := partjRingWorld() // 三穴环：被动环流 + 主动引流混合
	w.Ops = Ops{Requests: []FlowRequest{
		{Channel: 0, Amount: [ElementCount]int64{800, 800, 800, 800, 800}},
	}}
	rep := Advance(w)
	led := &rep.Work

	if led.ManualWork <= 0 {
		t.Errorf("主动引流应记 ManualWork: %d", led.ManualWork)
	}
	if led.AutonomousWork != 0 {
		t.Errorf("自主做功本阶段应恒零（阶段三才引入）: %d", led.AutonomousWork)
	}
	if led.DriveWork != rep.Conservation.DriveWork {
		t.Errorf("台账驱动做功应与守恒账同源: %d vs %d", led.DriveWork, rep.Conservation.DriveWork)
	}
	if led.ExternalInput != rep.Conservation.Intake+rep.Conservation.ExchangeIn+rep.Conservation.Entrained {
		t.Errorf("环境流入对账: %d vs intake %d + exchangeIn %d + entrained %d",
			led.ExternalInput, rep.Conservation.Intake, rep.Conservation.ExchangeIn, rep.Conservation.Entrained)
	}
	if led.Loss != rep.Conservation.TransferLoss+rep.Conservation.Diffuse+rep.Conservation.ShengLoss {
		t.Errorf("损耗对账: %d", led.Loss)
	}
	if led.Generated != 0 {
		t.Errorf("本阶段无净生成: %d", led.Generated)
	}
	// 内化度 = 自主/(自主+主动)（评审稿 §15）：自主恒零 → 恒 0。
	if got := DeriveInternalization(led); got != 0 {
		t.Errorf("内化度应恒 0（自主做功未引入）: %d", got)
	}

	// 无驱动 Tick：台账主动/驱动做功归零（金手指：无驱动路径不生新账）。
	w.Ops = Ops{}
	rep2 := Advance(w)
	if rep2.Work.ManualWork != 0 || rep2.Work.DriveWork != 0 {
		t.Errorf("无驱动 Tick 台账应零: manual=%d drive=%d", rep2.Work.ManualWork, rep2.Work.DriveWork)
	}
	for i := range rep2.Points {
		if rep2.Points[i].DriveStrain != 0 {
			t.Errorf("无驱动 Tick 不应有驱动应变: point %d = %d", i, rep2.Points[i].DriveStrain)
		}
	}
	checkConservation(t, rep2)
}
