package cult

import (
	"reflect"
	"testing"
)

// 标度备忘：所有断言值为千分位（1.0 = 1000）。
// stage_3 手算值为十进制小数，换算 = 小数 × 1000；
// 允许误差 ±1（千分位最后一位），来自逐对 fpMul 四舍五入。

// goldenEnv / goldenCharacter / goldenAcupoint 复现 stage_3 §45 的配置。
func goldenEnv() Environment {
	return Environment{
		Qi:          [ElementCount]int64{10000, 1000, 1000, 500, 4000}, // 木10 火1 土1 金0.5 水4
		Activity:    [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Temperature: 1000,
	}
}

func goldenCharacter() Character {
	return Character{
		Affinity:   [ElementCount]int64{1250, 800, 900, 750, 1100}, // §45
		Control:    700,
		Perception: 500,
		BodyGraph: BodyGraph{
			Acupoints: []Acupoint{goldenAcupoint("青关")},
		},
	}
}

func goldenAcupoint(name string) Acupoint {
	return Acupoint{
		Name:          name,
		Affinity:      [ElementCount]int64{1200, 750, 900, 850, 1000}, // §45
		Capacity:      10000,                                          // 10
		Throughput:    3000,                                           // 3
		Retention:     400,                                            // 0.40
		Openness:      800000,                                         // 0.80（微标度）
		BaseStability: 950,
		Stability:     950, // 0.95
		// 驱动画像（0.5.0 [v0.1 参数]）：10000 ≥ 本世界各通道 large 档流量，
		// 驱动容量不限幅——黄金用例只受 §8.1 管线容量约束（单穴世界无经脉，
		// 本字段不参与任何运算）。
		DriveCapacity: 10000,
		Damage:        0,
		Adaptation:    [ElementCount]int64{1000000, 1000000, 1000000, 1000000, 1000000},
		Activity:      [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
	}
}

func assertNear(t *testing.T, what string, got, want int64) {
	t.Helper()
	if got-want > 1 || want-got > 1 {
		t.Errorf("%s: got %d, want %d (±1)", what, got, want)
	}
}

// TestGoldenCase 复现 stage_3 §46-48 的手算例子（黄金用例）。
//
// 0.6.0 金手指复核（势差版统一边界交换，定稿裁决②）——逐项归因：
//   - 第一 Tick 体内势为零 → 势差 = 环境密度，且 KPermBase +
//     KPermDrive×0.6 ≈ 0.002+0.598 = 0.60 = 旧 §7 intensity 档，乘法链同序
//     → 边界交换纳入总量与旧 §7 吸收量逐位一致（见下方总量断言）。
//   - 报告口径变化：Intake 现在是「意识驱动放大的纳入增量」
//     （perm = KPermBase+KPermDrive×0.6 = 0.60 中的 KPermDrive 份额），
//     被动通透性基线单列 ExchangeIn（KPermBase=0.002 的份额，量级极小）。
//     两者之和 = 旧 §7 值。
//   - 第二 Tick 起势差参与：纳入总量 ∝ (环境势 − 体内势)，不再是
//     旧 §7 的「与环境密度成正比、与体内存量无关」——Tick1 已纳入的
//     7200 木经相生转化后存量 7014，势差压到 ≈2986，叠加反压 0.493
//     → 木纳入总量 1062（旧 §7 值为 3550）。
func TestGoldenCase(t *testing.T) {
	w := NewWorld(goldenCharacter(), goldenEnv())
	w.Ops = Ops{Intensity: []int64{600}} // §46：intensity 0.60

	// ---- Tick 1 ----
	rep := Advance(w)
	p := &rep.Points[0]

	// §46 各行纳入总量（手算 → 千分位；0.6.0 起为 Intake+ExchangeIn 之和，
	// 数值与旧 §7 吸收量逐位一致）：
	//   木 10×1.25×1.20×0.80×0.60×1×1 = 7.20
	//   火 1×0.80×0.75×0.80×0.60      = 0.288
	//   土 1×0.90×0.90×0.80×0.60      = 0.389
	//   金 0.5×0.75×0.85×0.80×0.60    = 0.153
	//   水 4×1.10×1.00×0.80×0.60      = 2.112
	wantTotal := [ElementCount]int64{7200, 288, 389, 153, 2112}
	for e := 0; e < ElementCount; e++ {
		assertNear(t, "纳入总量", p.Intake[e]+p.ExchangeIn[e], wantTotal[e])
	}
	// 归因分解：被动基线 = gap × KPermBase(0.002)×其余因子（木 24、水 7），
	// 意识驱动增量 = 总量 − 被动基线（perm(600) − KPermBase(2) 的份额）。
	assertNear(t, "木意识驱动增量", p.Intake[Wood], 7176)
	assertNear(t, "木被动基线", p.ExchangeIn[Wood], 24)
	assertNear(t, "水意识驱动增量", p.Intake[Water], 2105)
	assertNear(t, "水被动基线", p.ExchangeIn[Water], 7)

	total := int64(0)
	for e := 0; e < ElementCount; e++ {
		total += p.Intake[e] + p.ExchangeIn[e]
	}
	// 引擎逐元素四舍五入后总和 = 10142，与手算 10.142 一致。
	if total != 10142 {
		t.Errorf("纳入总量: got %d, want 10142", total)
	}

	// §47：LoadRatio = 10.142/10 = 1.0142；Pressure = 1.0142² ≈ 1.029。
	assertNear(t, "交换后 LoadRatio", p.PostIntakeRatio, 1014)
	assertNear(t, "交换后 Pressure", fpMul(p.PostIntakeRatio, p.PostIntakeRatio), 1028)

	// ---- Tick 2 ----
	rep2 := Advance(w)
	p2 := &rep2.Points[0]

	// §48：BackPressure = 1/(1+1.0142²) ≈ 0.493（反压链不受 0.6.0 影响）。
	assertNear(t, "Tick2 BackPressure", p2.BackPressure, 493)
	// 0.6.0 重算（归因见函数头）：木纳入总量 = 势差 2986 × 0.60(perm)
	// × 1.25 × 1.20 × 0.80 × 0.493(反压) ≈ 1062（意识增量 1058 +
	// 被动基线 4）。
	assertNear(t, "Tick2 木纳入总量", p2.Intake[Wood]+p2.ExchangeIn[Wood], 1062)
	assertNear(t, "Tick2 木意识增量", p2.Intake[Wood], 1058)
	assertNear(t, "Tick2 木被动基线", p2.ExchangeIn[Wood], 4)
}

// TestGolden_TenTicks 冒烟：连续 10 Tick 状态不出界（§49 调低强度）。
func TestGolden_TenTicks(t *testing.T) {
	w := NewWorld(goldenCharacter(), goldenEnv())
	for i := 0; i < 10; i++ {
		w.Ops = Ops{Intensity: []int64{250}}
		rep := Advance(w)
		p := &rep.Points[0]
		if p.Stability < 0 || p.Stability > 1000 {
			t.Fatalf("tick %d: stability 越界 %d", i, p.Stability)
		}
		for e := 0; e < ElementCount; e++ {
			if p.Qi[e] < 0 || p.Activity[e] < 0 || p.Activity[e] > 1000 {
				t.Fatalf("tick %d: 元素 %d 状态越界 qi=%d act=%d", i, e, p.Qi[e], p.Activity[e])
			}
		}
	}
}

// TestSingleAcupoint_IntakePressureStability 是 §51 Test 1：
// 单穴位吸收，验证 Intake / BackPressure / Pressure / Stability 演化。
func TestSingleAcupoint_IntakePressureStability(t *testing.T) {
	// 反压链：负载逐 Tick 累积 → BackPressure 严格下降（§7.1）。
	w := NewWorld(goldenCharacter(), goldenEnv())
	prevBP := int64(2000)
	prevPressure := int64(0)
	for i := 0; i < 3; i++ {
		w.Ops = Ops{Intensity: []int64{600}}
		rep := Advance(w)
		p := &rep.Points[0]
		if p.BackPressure >= prevBP {
			t.Errorf("tick %d: BackPressure 应下降: %d -> %d", i, prevBP, p.BackPressure)
		}
		prevBP = p.BackPressure
		if p.Pressure <= prevPressure {
			t.Errorf("tick %d: Pressure 应上升: %d -> %d", i, prevPressure, p.Pressure)
		}
		prevPressure = p.Pressure
	}

	// §18：轻度吸收（应变 < 恢复）下 Stability 保持并回升。
	w = NewWorld(goldenCharacter(), goldenEnv())
	w.Ops = Ops{Intensity: []int64{600}}
	s1 := Advance(w).Points[0].Stability
	w.Ops = Ops{Intensity: []int64{0}}
	w.Ops = Ops{}
	s2 := Advance(w).Points[0].Stability
	if s2 < s1 {
		t.Errorf("轻度负载下 Stability 不应下降: %d -> %d", s1, s2)
	}

	// 重载（容量 2000，负载比 ≈ 5）→ §17 Overload² 主导 →
	// Stability 被打穿，§19 产生损伤。
	heavy := goldenCharacter()
	heavy.BodyGraph.Acupoints[0].Capacity = 2000
	w = NewWorld(heavy, goldenEnv())
	stab := int64(10000)
	damaged := false
	for i := 0; i < 5; i++ {
		w.Ops = Ops{Intensity: []int64{600}}
		p := Advance(w).Points[0]
		if p.Stability > stab {
			t.Errorf("重载下 Stability 应下降: %d -> %d", stab, p.Stability)
		}
		stab = p.Stability
		if p.Damage > 0 {
			damaged = true
		}
	}
	if !damaged {
		t.Error("持续 5 倍过载应产生损伤")
	}
}

// TestTwoAcupoints_Transfer 是 §51 Test 2：双穴位 A→B，
// 验证 Conductance / ActualFlow / TransferLoss（§8-9）。
func TestTwoAcupoints_Transfer(t *testing.T) {
	newPoint := func(name string, qi int64) Acupoint {
		return Acupoint{
			Name:       name,
			Affinity:   [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
			Capacity:   50000,
			Throughput: 3000,
			// 驱动画像（0.5.0）：10000 ≫ 请求流量——本用例隔离 §8 传输
			// 公式，驱动容量不得成为限幅项（限幅语义另测）。
			DriveCapacity: 10000,
			Retention:     1000, // 不逸散，隔离变量
			Openness:      1000000,
			Stability:     1000,
			Activity:      [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
			Qi:            [ElementCount]int64{qi, 0, 0, 0, 0},
		}
	}
	char := Character{
		Affinity:   [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Control:    1000, // 满控制 → ε ≡ 0，请求流量不被扰动
		Perception: 500,
		BodyGraph: BodyGraph{
			Acupoints: []Acupoint{newPoint("丹田", 5000), newPoint("青关", 0)},
			Channels: []Channel{
				{Name: "丹田→青关", From: 0, To: 1, BaseConductance: 500, Throughput: 2000},
			},
		},
	}
	w := NewWorld(char, Environment{Temperature: 1000})
	w.Ops = Ops{Requests: []FlowRequest{{Channel: 0, Amount: [ElementCount]int64{1000, 0, 0, 0, 0}}}}

	rep := Advance(w)
	tr := rep.Transfers[0]

	// §8 Conductance = 0.5 × sqrt(1×1) × 1×1 × (1-0) = 0.5。
	assertNear(t, "Conductance", tr.Conductance[Wood], 500)
	// §8.1 Capacity = min(2000, 3000, 3000) × 0.5 = 1000。
	assertNear(t, "ChannelCapacity", tr.Capacity[Wood], 1000)
	// §8.2 ActualFlow = min(1000×(1+0), 5000, 1000) = 1000。
	assertNear(t, "ActualFlow", tr.Actual[Wood], 1000)
	// §9 Delivered = 1000 × clamp(0.5,0,1) = 500；TransferLoss = 500。
	assertNear(t, "DeliveredQi", tr.Delivered[Wood], 500)
	assertNear(t, "TransferLoss", tr.Loss[Wood], 500)

	// 库存（同 Tick 相生快照也作用）。Part J（被动输运/途经纳气）重算归因：
	// 新物理引入两笔潜在增量——①途经纳气（来源=环境，随实际流逐穴计量），
	// 本场景 Environment.Qi 全零，纳气恒 0；②被动输运（来源=体内流转，
	// 沿天然向、只填 §8.1 管线余量），本场景请求 1000 恰等于管线容量
	//（min(2000,3000,3000)×0.5=1000），管道余量为零——丹田→青关虽存在
	// 压差（ΔP=Pressure(LR²)=10），被动项亦无余量可用，故实际流全由主动
	// 请求构成。0.5.0 重算归因（定稿阶段二「驱动的代价」）：主动驱动另付
	// 统一驱动求解做功 DriveWork = k_work×Actual²/效率 = 0.002×1²=0.002
	//（千分位 2，去向=热、从驱动穴位扣除）——做功从真实能源扣，无凭空
	// 驱动（评审稿 §10.1/§26.4）：源 5000−1000(流出)−2(做功)−160(木→火
	// 转化) = 3838；目标 500−20 = 480；木合计 = 4318（主动传输损耗 500
	// 离开木池：250 归环境、250 归热；做功 2 与转化为火非损失项同账）。
	if got := rep.Points[0].Qi[Wood]; got != 3838 {
		t.Errorf("源穴位木: got %d, want 3838", got)
	}
	if got := rep.Points[1].Qi[Wood]; got != 480 {
		t.Errorf("目标穴位木: got %d, want 480", got)
	}
	total := rep.Points[0].Qi[Wood] + rep.Points[1].Qi[Wood]
	if total != 4318 {
		t.Errorf("木合计: got %d, want 4318", total)
	}

	// Part J 叠加对照（全新世界，与上半场状态隔离）：请求减半（500）后
	// 管道余量 500，压差梯度项照常驱动被动流、与主动流同走 §9 损耗管线：
	// 被动流 = chanCap×KPassive×ΔP×cond = 2000×0.3×0.01×0.5 = 3（千分位），
	// 送达 2、损耗 1（远小于余量——梯度受压差平方律限制，此处仅为叠加与
	// 归账验证）。守恒（0.5.0 重算归因：主动驱动另付做功 0.002×0.5²≈0.001，
	// 千分位 1）：源 5000−500(主动流出)−3(被动)−1(做功)−180(木→火) = 4316；
	// 目标 250+2−10 = 242；ΔQi = −442 = −251(传输损耗) −190(转化损耗)
	// −1(驱动做功)，环境气为零 → 无纳气项，全部可归因。被动流不付做功
	//（势差自然释放，评审稿 §15）——做功只随主动请求发生。
	char3 := Character{
		Affinity:   [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Control:    1000,
		Perception: 500,
		BodyGraph: BodyGraph{
			Acupoints: []Acupoint{newPoint("丹田", 5000), newPoint("青关", 0)},
			Channels: []Channel{
				{Name: "丹田→青关", From: 0, To: 1, BaseConductance: 500, Throughput: 2000},
			},
		},
	}
	w3 := NewWorld(char3, Environment{Temperature: 1000})
	w3.Ops = Ops{Requests: []FlowRequest{{Channel: 0, Amount: [ElementCount]int64{500, 0, 0, 0, 0}}}}
	rep3 := Advance(w3)
	var passive int64
	for _, tr := range rep3.Transfers {
		passive += tr.Passive[Wood]
	}
	if passive != 3 {
		t.Errorf("余量输运: 被动流 got %d, want 3", passive)
	}
	if got := rep3.Points[0].Qi[Wood]; got != 4316 {
		t.Errorf("余量输运源穴木: got %d, want 4316", got)
	}
	if got := rep3.Points[1].Qi[Wood]; got != 242 {
		t.Errorf("余量输运目标穴木: got %d, want 242", got)
	}
	c3 := rep3.Conservation
	if net, acc := c3.QiAfter-c3.QiBefore, c3.Intake-c3.Diffuse-c3.TransferLoss-c3.ShengLoss-c3.DriveWork+c3.Entrained; net != acc {
		t.Errorf("余量输运守恒破坏: ΔQi=%d 归账=%d", net, acc)
	}
	if rep3.Conservation.Entrained != 0 {
		t.Errorf("无环境灵气时途经纳气应为 0: %d", rep3.Conservation.Entrained)
	}

	// 满传导对照：Delivered = Actual、Loss = 0。
	char.BodyGraph.Channels[0].BaseConductance = 1000
	w2 := NewWorld(char, Environment{Temperature: 1000})
	w2.Ops = Ops{Requests: []FlowRequest{{Channel: 0, Amount: [ElementCount]int64{1000, 0, 0, 0, 0}}}}
	tr2 := Advance(w2).Transfers[0]
	if tr2.Delivered[Wood] != tr2.Actual[Wood] {
		t.Errorf("满传导 Delivered 应等于 Actual: %d vs %d", tr2.Delivered[Wood], tr2.Actual[Wood])
	}
	if tr2.Loss[Wood] != 0 {
		t.Errorf("满传导 Loss 应为 0: got %d", tr2.Loss[Wood])
	}
}

// TestSingleAcupoint_ShengKe 是 §51 Test 3：单穴位木+火+水，
// 验证相生转化、相克压活性、活性恢复（§10-12）。
func TestSingleAcupoint_ShengKe(t *testing.T) {
	char := Character{
		Affinity:   [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Control:    1000,
		Perception: 500,
		BodyGraph: BodyGraph{Acupoints: []Acupoint{{
			Name:       "五行试穴",
			Affinity:   [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
			Capacity:   100000,
			Throughput: 3000,
			Retention:  1000, // 不逸散，隔离变量
			Openness:   1000000,
			Stability:  1000,
			Activity:   [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
			Qi:         [ElementCount]int64{2000, 2000, 0, 0, 2000}, // 木+火+水
		}}},
	}
	w := NewWorld(char, Environment{Temperature: 1000})
	w.Ops = Ops{}

	rep := Advance(w)
	p := &rep.Points[0]

	// §10 相生（快照语义，亲和/活性全 1）：
	// 每源转化 2000×0.04 = 80，目标得 80×0.9 = 72。
	assertNear(t, "木转化后", p.Qi[Wood], 2000-80+72) // 木→火 −80，水→木 +72
	assertNear(t, "火转化后", p.Qi[Fire], 2000+72-80) // 木→火 +72，火→土 −80
	assertNear(t, "土转化后", p.Qi[Earth], 72)        // 火→土 +72
	assertNear(t, "水转化后", p.Qi[Water], 2000-80)   // 水→木 −80
	if p.Qi[Metal] != 0 {
		t.Errorf("金不应被生成: got %d", p.Qi[Metal])
	}

	// §11 相克（活值顺序：木克土 → 土克水 → 水克火）+ §12 恢复（同 Tick +10%）：
	// 土：压制 = 0.15×(1992/2065) ≈ 145 → 活性 1000×0.855 = 855 → 恢复后 870。
	assertNear(t, "土活性（被木克+恢复）", p.Activity[Earth], 870)
	// 火：水克火，压制 ≈ 74 → 活性 926 → 恢复后 933。
	assertNear(t, "火活性（被水克+恢复）", p.Activity[Fire], 933)
	// 金无生成无克制（火克金需金存在），活性保持满。
	if p.Activity[Metal] != 1000 {
		t.Errorf("金活性不应变化: got %d", p.Activity[Metal])
	}

	// §12 恢复：清空灵气（克制消失）后活性单调趋向满值。
	for i := range w.Character.BodyGraph.Acupoints {
		w.Character.BodyGraph.Acupoints[i].Qi = [ElementCount]int64{}
	}
	w.Ops = Ops{}
	prev := Advance(w).Points[0]
	for i := 0; i < 5; i++ {
		cur := Advance(w).Points[0]
		for e := 0; e < ElementCount; e++ {
			if cur.Activity[e] < prev.Activity[e] {
				t.Errorf("无克制时活性应单调恢复: 元素%d %d -> %d", e, prev.Activity[e], cur.Activity[e])
			}
			if cur.Activity[e] > 1000 {
				t.Fatalf("活性越过满值: %d", cur.Activity[e])
			}
		}
		prev = cur
	}
	if prev.Activity[Earth] <= 870 {
		t.Errorf("恢复 6 Tick 后土活性应高于 870: got %d", prev.Activity[Earth])
	}
}

// TestDamageAndRecovery 验证 §19/§21：低稳定性生损伤、低压低应变下缓慢自愈。
func TestDamageAndRecovery(t *testing.T) {
	char := goldenCharacter()
	pt := &char.BodyGraph.Acupoints[0]
	pt.Capacity = 2000  // 吸收 10.1 → 负载比 ≈ 5，稳定性速崩
	pt.Retention = 1000 // 不逸散，维持重载
	pt.Stability = 1000

	w := NewWorld(char, goldenEnv())
	w.Ops = Ops{Intensity: []int64{600}}

	peak := int64(0)
	for i := 0; i < 5; i++ {
		if d := Advance(w).Points[0].Damage; d > peak {
			peak = d
		}
	}
	if peak == 0 {
		t.Fatal("持续 5 倍过载应产生损伤")
	}

	// 清空负载并隔绝环境（0.6.0：环境有灵气时身体会被动吐纳、静息压力
	// 停在环境势水平，§21 恢复条件无法满足——「静养」语义 = 无负载 +
	// 无环境交换）→ §21 条件（Pressure<0.4 且 Strain<0.05）满足，
	// 稳定性以 0.01/Tick 回升；越过 0.40 阈值后损伤开始净恢复。
	for i := range w.Character.BodyGraph.Acupoints {
		w.Character.BodyGraph.Acupoints[i].Qi = [ElementCount]int64{}
	}
	w.Env.Qi = [ElementCount]int64{}
	w.Ops = Ops{}
	recovered := peak
	for i := 0; i < 400; i++ {
		if d := Advance(w).Points[0].Damage; d < recovered {
			recovered = d
			break
		}
	}
	if recovered >= peak {
		t.Errorf("损伤未恢复: 峰值 %d, 现 %d", peak, recovered)
	}
}

// TestGrowth_LongRun 验证 §22/§23/§24：安全运行下 Adaptation、Openness、
// 经脉 Adaptation 缓慢成长（stage_3 §51 Test 5 的引擎核部分）。
//
// 场景：低强度吸收 + 五行平衡外流（流向丹田汇点）。BackPressure 使
// 负载比自稳定在 ≈0.67（压力 ≈0.45-0.57，恰在 §23 适宜压力带内），
// 稳定性保持满值 → 三个成长通道全程开启。
func TestGrowth_LongRun(t *testing.T) {
	char := goldenCharacter()
	pt := &char.BodyGraph.Acupoints[0]
	pt.Capacity = 12000
	pt.Throughput = 20000
	pt.Retention = 1000
	pt.Stability = 1000
	pt.Qi = [ElementCount]int64{5540, 155, 210, 80, 1815} // 预填，压力即入适宜带
	sink := goldenAcupoint("丹田汇")
	sink.Capacity = 100000
	sink.Throughput = 20000
	sink.Retention = 1000
	sink.Stability = 1000
	char.BodyGraph.Acupoints = append(char.BodyGraph.Acupoints, sink)
	char.BodyGraph.Channels = []Channel{{
		Name: "青关→丹田", From: 0, To: 1,
		BaseConductance: 1000, Throughput: 20000,
	}}
	w := NewWorld(char, goldenEnv())

	startAdapt := w.Character.BodyGraph.Acupoints[0].Adaptation[Wood]
	startOpen := w.Character.BodyGraph.Acupoints[0].Openness
	startChan := w.Character.BodyGraph.Channels[0].ChannelAdaptation

	for i := 0; i < 500; i++ {
		w.Ops = Ops{
			Intensity: []int64{60, 0},
			Requests:  []FlowRequest{{Channel: 0, Amount: [ElementCount]int64{420, 18, 25, 10, 130}}},
		}
		Advance(w)
	}

	after := &w.Character.BodyGraph.Acupoints[0]
	if after.Adaptation[Wood] <= startAdapt {
		t.Errorf("木适应应增长: %d -> %d", startAdapt, after.Adaptation[Wood])
	}
	if after.Openness <= startOpen {
		t.Errorf("Openness 应增长: %d -> %d", startOpen, after.Openness)
	}
	if w.Character.BodyGraph.Channels[0].ChannelAdaptation <= startChan {
		t.Errorf("经脉适应应增长: %d -> %d", startChan, w.Character.BodyGraph.Channels[0].ChannelAdaptation)
	}
	// §22/§23 的"缓慢"：500 Tick 内不得长满。
	if after.Openness >= MicroOne {
		t.Errorf("Openness 500 Tick 内不应到顶: %d", after.Openness)
	}
	if after.Adaptation[Wood] >= AdaptationMax*1000 {
		t.Errorf("木适应 500 Tick 内不应到顶: %d", after.Adaptation[Wood])
	}
}

// TestDeterminism_1000Ticks 同参数 + 同种子跑 1000 Tick 两次，状态逐位一致。
func TestDeterminism_1000Ticks(t *testing.T) {
	build := func() *World {
		char := goldenCharacter()
		char.BodyGraph.Channels = []Channel{
			{Name: "自环", From: 0, To: 0, BaseConductance: 700, Throughput: 2000},
		}
		return NewWorld(char, goldenEnv())
	}
	run := func(w *World) {
		for i := 0; i < 1000; i++ {
			w.Ops = Ops{
				Intensity: []int64{int64(100 + (i%7)*100)}, // 变强度
				Requests: []FlowRequest{{
					Channel: 0,
					Amount:  [ElementCount]int64{int64(500 + (i%5)*200), 0, 0, 0, 0},
				}},
			}
			Advance(w) // Control=0.70 → 控制误差噪声全程生效
		}
	}
	w1, w2 := build(), build()
	run(w1)
	run(w2)
	if w1.Tick != w2.Tick {
		t.Fatalf("Tick 计数不一致: %d vs %d", w1.Tick, w2.Tick)
	}
	if !reflect.DeepEqual(w1.Character, w2.Character) {
		t.Fatal("1000 Tick 后角色状态逐位不一致")
	}
	if !reflect.DeepEqual(w1.Last, w2.Last) {
		t.Fatal("末 Tick 报告逐位不一致")
	}
	// 换种子必须产生不同轨迹（证明噪声确实参与运算）。
	w3 := build()
	w3.NoiseSeed = 42
	run(w3)
	if reflect.DeepEqual(w1.Character, w3.Character) {
		t.Fatal("不同噪声种子产生了完全相同的状态——噪声未生效")
	}
}
