package cult

// percept_test.go —— PerceptToken 中间层测试（境界文档 §22.2 的 cult 层
// 最小集 + 修订包 T6 + 滞回重放等价）。
//
// 覆盖：
//   a. 同 Pressure 不同 dPressure/dt → 趋势不同（§22.2-1）；
//   b. Thermal 在阈值附近小幅波动 → 滞回保证不反复跳档（§22.2-2/§15）；
//   c. 高 FlowShock + 低 Damage → 刺痛；持续 Damage → 隐痛（§22.2-3/§11.2）；
//   d. 高 LoadRatio + 低 Pain → 充盈/胀在前，不直接疼痛（§22.2-4）；
//   e. 相同构成不同 ThermalLoad → 热词不同——热来自热负荷不是属性名
//      （§22.2-5/§7.2）；
//   f. 炼化三档 → 对应定性词；构成/身体变化 → 词随重算变化
//      （修订包 Part F / T6）；
//   g. 滞回状态存档参与确定性重放（DeepEqual）；
//   h. 引擎纯度：Observe 不影响 Advance 结果；token 每 Tick 备忘幂等。

import (
	"reflect"
	"strings"
	"testing"
)

// tokenWorld 构造单穴位世界（均匀亲和 + 满控制，隔离变量）。
func tokenWorld() *World {
	a := Acupoint{
		Name:       "青关",
		Affinity:   [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Capacity:   10000,
		Throughput: 3000,
		Retention:  500,
		Openness:   800000,
		Stability:  950,
	}
	return NewWorld(Character{
		Affinity:   [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Control:    1000,
		Perception: 1000,
		BodyGraph:  BodyGraph{Acupoints: []Acupoint{a}},
	}, Environment{})
}

// baseReport 是一个"无事发生"的报告基线（BackPressure/Compatibility 满，
// 无冲击无损伤），各用例按需覆盖字段。
func baseReport() AcupointReport {
	return AcupointReport{
		BackPressure:  One,
		Compatibility: One,
	}
}

// step 模拟一次"推进 + 观察"：替换 w.Last 并前移 w.Tick（与 Advance 的
// 顺序一致），返回本 Tick 的穴位 token。
func step(w *World, rep AcupointReport) pointToken {
	w.Last = &TickReport{Tick: w.Tick, Points: []AcupointReport{rep}}
	w.Tick++
	return sensoryTokens(w)[0]
}

// rendered 把当前世界渲染成定性文本并拼接 Feelings（便于断言措辞）。
func rendered(w *World, acuity int) string {
	obs := Observe(w, acuity)
	return strings.Join(obs.Points[0].Feelings, "|")
}

// TestTrend_SamePressureDifferentDelta（a / §22.2-1）：相同终值压力，
// 上涨序列与平稳序列产出不同的胀感趋势词。
func TestTrend_SamePressureDifferentDelta(t *testing.T) {
	// 平稳：压力恒为 650（LoadRatio ≈ 806 恒定）。
	wFlat := tokenWorld()
	for i := 0; i < 3; i++ {
		rep := baseReport()
		rep.Pressure = 650
		rep.LoadRatio = 806
		rep.PostIntakeRatio = 806 // §47 口径：吸收后负载比（渲染用）
		tok := step(wFlat, rep)
		if i == 2 && (tok.Distension.trend != trendStable || tok.Fullness.trend != trendStable) {
			t.Errorf("平稳序列末 Tick 趋势应稳定: dist=%d full=%d", tok.Distension.trend, tok.Fullness.trend)
		}
	}
	flatText := rendered(wFlat, 2)
	if strings.Contains(flatText, "胀意仍在增强") {
		t.Errorf("平稳序列不应报增强趋势: %s", flatText)
	}

	// 上涨：压力 500 → 575 → 650（同终值，不同的 dPressure/dt）。
	wRise := tokenWorld()
	for _, p := range []int64{500, 575, 650} {
		rep := baseReport()
		rep.Pressure = p
		rep.LoadRatio = fpSqrt(p) // Pressure = LoadRatio² 的反推口径
		rep.PostIntakeRatio = rep.LoadRatio
		tok := step(wRise, rep)
		if p == 650 && tok.Distension.trend != trendRising {
			t.Errorf("上涨序列末 Tick 胀感趋势应上升: %d", tok.Distension.trend)
		}
	}
	riseText := rendered(wRise, 2)
	if !strings.Contains(riseText, "胀意仍在增强") {
		t.Errorf("上涨序列应报增强趋势: %s", riseText)
	}
}

// TestHysteresis_ThermalNoFlapping（b / §22.2-2/§15）：ThermalLoad 在温热
// 档进入阈值（170）附近小幅波动（168/172），只允许一次进档，此后档位
// 纹丝不动。
func TestHysteresis_ThermalNoFlapping(t *testing.T) {
	w := tokenWorld()
	levels := []int{}
	for i := 0; i < 10; i++ {
		raw := int64(168)
		if i%2 == 1 {
			raw = 172
		}
		a := &w.Character.BodyGraph.Acupoints[0]
		a.ThermalLoad = raw
		w.Tick++ // 直接操纵世界后前移 Tick，令 token 重算
		levels = append(levels, sensoryTokens(w)[0].Thermal.level)
	}
	// 168 → 档1（微温 enter 50）；172 → 进入档2（温热 enter 170）；
	// 此后 168 > exit(120) → 恒为档2。绝不允许在 1/2 之间反复跳。
	if levels[0] != 1 || levels[1] != 2 {
		t.Fatalf("进档序列异常: %v", levels)
	}
	for i, lv := range levels[2:] {
		if lv != 2 {
			t.Errorf("tick %d: 滞回失效，温热档应保持: levels=%v", i+2, levels)
			break
		}
	}
	changes := 0
	for i := 1; i < len(levels); i++ {
		if levels[i] != levels[i-1] {
			changes++
		}
	}
	if changes != 1 {
		t.Errorf("10 Tick 阈值抖动应只产生 1 次档位变化: %v", levels)
	}
}

// TestPain_SharpVsDull（c / §22.2-3/§11.2）：高频 FlowShock + 低 Damage →
// 刺痛主导；持续 Damage（低冲击）→ 隐痛主导。
func TestPain_SharpVsDull(t *testing.T) {
	// 刺痛：FlowShock 0.9 + Damage 0.1，连续 3 Tick。
	w := tokenWorld()
	for i := 0; i < 3; i++ {
		rep := baseReport()
		rep.FlowShock = 900
		rep.Damage = 100
		tok := step(w, rep)
		if i == 2 {
			if tok.Kind != painSharp {
				t.Errorf("高频冲击 + 低损伤应判刺痛: kind=%d", tok.Kind)
			}
			if tok.Pain.level < 1 {
				t.Errorf("冲击痛应可感: level=%d", tok.Pain.level)
			}
		}
	}
	sharpText := rendered(w, 2)
	if !strings.Contains(sharpText, "刺痛") {
		t.Errorf("刺痛措辞缺失: %s", sharpText)
	}
	if strings.Contains(sharpText, "隐痛") || strings.Contains(sharpText, "隐隐作痛") {
		t.Errorf("高频冲击不应判隐痛: %s", sharpText)
	}

	// 隐痛：Damage 0.6 持续 + 微弱冲击，连续 3 Tick（满足持续性门槛）。
	w = tokenWorld()
	for i := 0; i < 3; i++ {
		rep := baseReport()
		rep.Damage = 600
		rep.FlowShock = 50
		rep.Strain = 100
		tok := step(w, rep)
		if i == 2 && tok.Kind != painDull {
			t.Errorf("持续损伤应判隐痛: kind=%d", tok.Kind)
		}
	}
	dullText := rendered(w, 2)
	if !strings.Contains(dullText, "隐隐作痛") && !strings.Contains(dullText, "隐痛") {
		t.Errorf("隐痛措辞缺失: %s", dullText)
	}
}

// TestPain_FullnessBeforePain（d / §22.2-4）：高负载比 + 零应变/损伤 →
// 充盈/胀在前，不直接报疼痛。
func TestPain_FullnessBeforePain(t *testing.T) {
	w := tokenWorld()
	rep := baseReport()
	rep.LoadRatio = 900       // 高负载比
	rep.Pressure = 810        // 0.9²
	rep.PostIntakeRatio = 900 // §47 口径：吸收后负载比（渲染用）
	tok := step(w, rep)
	if tok.Pain.level != 0 {
		t.Errorf("无应变无损伤不应产生痛感: level=%d", tok.Pain.level)
	}
	if tok.Fullness.level < 3 || tok.Distension.level < 3 {
		t.Errorf("高负载应先表现为充盈/胀: full=%d dist=%d", tok.Fullness.level, tok.Distension.level)
	}
	text := rendered(w, 2)
	if strings.Contains(text, "痛") {
		t.Errorf("高 LoadRatio + 低 Pain 不应报疼痛: %s", text)
	}
	if !strings.Contains(text, "胀") {
		t.Errorf("应先报胀感: %s", text)
	}
}

// TestThermal_FromLoadNotElementName（e / §22.2-5/§7.2）：完全相同的
// 气构成，只有 ThermalLoad 不同 → 热词必然不同（热来自热负荷）。
func TestThermal_FromLoadNotElementName(t *testing.T) {
	qi := [ElementCount]int64{3000, 200, 200, 100, 800} // 水金偏重的构成
	build := func(thermal int64) *World {
		w := tokenWorld()
		w.Character.BodyGraph.Acupoints[0].Qi = qi
		w.Character.BodyGraph.Acupoints[0].ThermalLoad = thermal
		w.Tick++
		return w
	}
	cool := rendered(build(60), 3) // 微温档
	hot := rendered(build(600), 3) // 灼热档
	if !strings.Contains(cool, "微温") {
		t.Errorf("低热负荷应报微温: %s", cool)
	}
	if !strings.Contains(hot, "灼热") {
		t.Errorf("高热负荷应报灼热: %s", hot)
	}
	if cool == hot {
		t.Fatal("同构成不同 ThermalLoad 产出相同热词——热感未跟随热负荷")
	}
}

// TestRefinement_ThreeBands（f / 修订包 Part F / T6）：炼化三档 → 对应
// 定性词；构成/身体变化 → 词随重算变化。零数字泄漏由
// TestPerceptionIsolation_NoTruthLeak 统一锁死。
func TestRefinement_ThreeBands(t *testing.T) {
	// 金手指（golden 亲和有梯度）：affEff = {1500, 600, 810, 638, 1100}，
	// MatchMin = 930，MatchMax = 1500。
	//   均匀构成 → Refine 0（低）；木 0.4/水 0.6 → Refine ≈ 0.58（中）；
	//   全木 → Refine 1000（高）。
	build := func(qi [ElementCount]int64) *World {
		w := NewWorld(goldenCharacter(), goldenEnv())
		w.Character.BodyGraph.Acupoints[0].Qi = qi
		w.Tick++
		return w
	}

	low := rendered(build([ElementCount]int64{1000, 1000, 1000, 1000, 1000}), 2)
	if !strings.Contains(low, "驳杂") || !strings.Contains(low, "逸散") {
		t.Errorf("炼化低档应报驳杂/逸散感: %s", low)
	}

	mid := rendered(build([ElementCount]int64{2000, 0, 0, 0, 3000}), 2)
	if !strings.Contains(mid, "驯") || !strings.Contains(mid, "阻力减小") {
		t.Errorf("炼化中档应报驯顺/阻力减小: %s", mid)
	}

	high := rendered(build([ElementCount]int64{5000, 0, 0, 0, 0}), 2)
	if !strings.Contains(high, "凝练") || !strings.Contains(high, "不再自行散去") {
		t.Errorf("炼化高档应报凝练/几乎不散: %s", high)
	}

	// 构成变化 → 同一世界重算后词随之变化（D2 导出量）。
	w := build([ElementCount]int64{5000, 0, 0, 0, 0})
	before := rendered(w, 2)
	w.Character.BodyGraph.Acupoints[0].Qi = [ElementCount]int64{1000, 1000, 1000, 1000, 1000}
	w.Tick++
	after := rendered(w, 2)
	if before == after || !strings.Contains(after, "驳杂") {
		t.Errorf("构成变化后炼化词应重算变化: before=%q after=%q", before, after)
	}

	// 身体变化 → 重算（D2）：主行亲和跌变使全木构成失去收敛方向。
	w = build([ElementCount]int64{5000, 0, 0, 0, 0})
	before = rendered(w, 2)
	w.Character.BodyGraph.Acupoints[0].Affinity[Wood] = 600
	w.Tick++
	after = rendered(w, 2)
	if before == after || !strings.Contains(after, "驳杂") {
		t.Errorf("身体跌变后炼化词应重算下降: before=%q after=%q", before, after)
	}
}

// TestSensoryState_ReplayEquivalence（g）：同 seed 同 Ops + 相同观察序列
// → 滞回状态存档逐位一致（DeepEqual）；Observation 也深度相等。
func TestSensoryState_ReplayEquivalence(t *testing.T) {
	run := func() *World {
		w := NewWorld(goldenCharacter(), goldenEnv())
		w.Ops = Ops{Intensity: []int64{600}}
		for i := 0; i < 40; i++ {
			if i%3 == 0 {
				w.Ops = Ops{Intensity: []int64{int64(200 + (i%5)*100)}}
			}
			Advance(w)
			Observe(w, 2) // 观察序列本身是重放的一部分
		}
		return w
	}
	w1, w2 := run(), run()
	if !reflect.DeepEqual(w1.sensory, w2.sensory) {
		t.Fatal("滞回状态存档在重放下不一致——确定性被破坏")
	}
	if !reflect.DeepEqual(Observe(w1, 2), Observe(w2, 2)) {
		t.Fatal("重放观察不一致")
	}
}

// TestObserve_EnginePurityAndMemo（h）：Observe 不影响引擎结算；同一
// Tick 反复观察复用 token（状态不二次推进）。
func TestObserve_EnginePurityAndMemo(t *testing.T) {
	// 引擎纯度：观察插桩与不插桩的 Advance 轨迹逐位一致。
	plain := NewWorld(goldenCharacter(), goldenEnv())
	probed := NewWorld(goldenCharacter(), goldenEnv())
	for i := 0; i < 5; i++ {
		ops := Ops{Intensity: []int64{int64(300 + i*50)}}
		plain.Ops, probed.Ops = ops, ops
		want := Advance(plain)
		Advance(probed)
		Observe(probed, 2)
		if !reflect.DeepEqual(want, probed.Last) {
			t.Fatalf("tick %d: Observe 影响了引擎结算", i)
		}
	}
	if !reflect.DeepEqual(plain.Character, probed.Character) {
		t.Fatal("Observe 影响了角色状态")
	}

	// token 备忘幂等：同一 Tick 第二次观察不推进任何滞回状态。
	w := NewWorld(goldenCharacter(), goldenEnv())
	w.Ops = Ops{Intensity: []int64{600}}
	Advance(w)
	Observe(w, 2)
	snapshot := *w.sensory
	snapshot.points = append([]pointMemory(nil), w.sensory.points...)
	Observe(w, 2)
	if !reflect.DeepEqual(snapshot, *w.sensory) {
		t.Fatal("同一 Tick 的重复观察推进了感知状态——备忘失效")
	}
}
