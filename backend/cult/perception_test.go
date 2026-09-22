package cult

// perception_test.go —— WP-2 感知解析器测试。
//
// 覆盖（M1 计划 §3 WP-2 验收）：
//   - stage_3 §43 每档边界（0.25/0.45/0.65/0.85/1.0）两侧措辞正确；
//   - acuity 1 vs 3 同一真值产出不同粒度措辞（§43 低感知叙事）；
//   - 感知隔离律：Observation 序列化后无真值泄漏（除 Tick / band 序号
//     外全文无数字）；
//   - 黄金用例（§47）：Tick1 后青关真值压力入剧痛档，低感知措辞
//     "强烈胀感"级，PressureBand 报同档 felt 档（所见即所得）；
//   - 确定性：同 World 同 acuity 两次 Observe 结果深度相等。

import (
	"encoding/json"
	"reflect"
	"regexp"
	"testing"
)

// senseWorld 构造单穴位世界：Qi[Wood]=lr（容量 1000 → 负载比 = lr/1000），
// 便于把压力精确摆到 §43 各档边界上。
func senseWorld(lr, perception int64) *World {
	a := Acupoint{
		Name:       "青关",
		Affinity:   [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Capacity:   1000,
		Throughput: 3000,
		Retention:  500,
		Openness:   800000,
		Stability:  950,
	}
	a.Qi[Wood] = lr
	return NewWorld(Character{
		Affinity:   [ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Control:    1000,
		Perception: perception,
		BodyGraph:  BodyGraph{Acupoints: []Acupoint{a}},
	}, Environment{})
}

// TestPressureWords_Stage43Boundaries 逐边界两侧验证措辞与 band。
// Perception=1.0、acuity=3 → Familiarity=1.0 → PerceivedSignalStrength
// 等于真值（§42），措辞落在真值档（tier 2）。
func TestPressureWords_Stage43Boundaries(t *testing.T) {
	cases := []struct {
		lr         int64 // 负载比（千分位）；压力 = lr²（§14）
		wantBand   int
		wantPhrase string // tier 2（acuity 3）措辞
	}{
		// 0.25 边界两侧（0.499² ≈ 0.249 / 0.5² = 0.25）
		{499, 0, "空明，可容新纳（约莫一成）"},
		{500, 1, "渐盈（约三成）"},
		// 0.45 边界两侧
		{670, 1, "渐盈（约三成）"},
		{671, 2, "胀，然尚可续引（约五成）"},
		// 0.65 边界两侧
		{805, 2, "胀，然尚可续引（约五成）"},
		{806, 3, "满胀，阻力大增（约七成）"},
		// 0.85 边界两侧
		{921, 3, "满胀，阻力大增（约七成）"},
		{922, 4, "灼痛，脉将不堪（约九成）"},
		// 1.0 边界两侧（0.85~1.0 疼痛档含 1.0，>1.0 剧痛档）
		{1000, 4, "灼痛，脉将不堪（约九成）"},
		{1001, 5, "过载！有撕裂之危（已超限）"},
	}
	for _, c := range cases {
		obs := Observe(senseWorld(c.lr, 1000), 3)
		p := obs.Points[0]
		if p.PressureBand != c.wantBand {
			t.Errorf("lr=%d: PressureBand = %d, want %d", c.lr, p.PressureBand, c.wantBand)
		}
		if got := p.Feelings[0]; got != c.wantPhrase {
			t.Errorf("lr=%d: Feelings[0] = %q, want %q", c.lr, got, c.wantPhrase)
		}
	}
}

// TestPressureWords_Tier1IsStage43 锁死 tier 1（acuity 2）措辞 =
// stage_3 §43 原文六级。
func TestPressureWords_Tier1IsStage43(t *testing.T) {
	canon := []string{
		"无明显感觉", "轻微充盈", "明显胀感",
		"强烈胀感·不适", "疼痛", "剧痛·恐有失稳之患",
	}
	for band, want := range canon {
		if got := pressureWords[band][1]; got != want {
			t.Errorf("pressureWords[%d][1] = %q, want §43 %q", band, got, want)
		}
	}
}

// TestAcuityGranularity 同一真值（压力 ≈ 0.70，band 3），神识三档措辞
// 粒度逐级展开（stage_2 §24：Detection → Recognition → Measurement）。
func TestAcuityGranularity(t *testing.T) {
	// lr=837 → 压力 = 0.837² ≈ 0.701（band 3）。
	w := senseWorld(837, 1000) // Perception=1.0：隔离神识以外的变量

	low := Observe(w, 1)
	if got := low.Points[0].Feelings[0]; got != "似有微涨" {
		t.Errorf("acuity1 措辞 = %q, want 初见钝化措辞 %q", got, "似有微涨")
	}
	if len(low.Points[0].Nature) != 0 {
		t.Errorf("acuity1 不应辨出五行倾向（§24 Recognition 未解锁），got %v", low.Points[0].Nature)
	}

	mid := Observe(w, 2)
	if got := mid.Points[0].Feelings[0]; got != "明显胀感" {
		t.Errorf("acuity2 措辞 = %q, want §43 原文 %q", got, "明显胀感")
	}
	if len(mid.Points[0].Nature) != 1 {
		t.Errorf("acuity2 应给出 nature 推断词，got %v", mid.Points[0].Nature)
	}

	high := Observe(w, 3)
	if got := high.Points[0].Feelings[0]; got != "满胀，阻力大增（约七成）" {
		t.Errorf("acuity3 措辞 = %q, want 熟练措辞（约几成提示）", got)
	}
	if len(high.Points[0].Nature) != 1 || high.Points[0].Nature[0] != "似偏木性（未证实）" {
		t.Errorf("acuity3 nature = %v, want [似偏木性（未证实）]", high.Points[0].Nature)
	}

	if low.Points[0].Feelings[0] == high.Points[0].Feelings[0] {
		t.Fatal("acuity 1 与 3 同真值产出相同措辞——粒度分级失效")
	}
}

// TestLowPerception_Downgrade 复现 stage_3 §43 低感知叙事：
// 低感知角色（Perception=0.5）在真实过载（压力 ≈ 1.028，真值档 5）下
// 只报"有些不适"级措辞；PerceivedSignalStrength 静默不超过两档。
// PressureBand 取感知后档（felt）：真值档 5 静默两档 → felt 档 3，
// 与措辞同档（所见即所得，band 不得旁路泄真值档）。
func TestLowPerception_Downgrade(t *testing.T) {
	// lr=1014 → 压力 = 1.014² ≈ 1.028（真值档 5，黄金用例 §47 同款）。
	obs := Observe(senseWorld(1014, 500), 1) // Perception=0.5 + acuity 1（F=0.5）
	p := obs.Points[0]
	if p.PressureBand != 3 {
		t.Errorf("PressureBand = %d, want 3（感知后 felt 档 = 真值档 5 静默两档）", p.PressureBand)
	}
	if got := p.Feelings[0]; got != "有些不适" {
		t.Errorf("低感知措辞 = %q, want %q（§43：低感知角色只感受到有些不适）", got, "有些不适")
	}
	// 所见即所得：PressureBand 与措辞档一致（pressureWords[band][tier]
	// 即 Feelings[0]）。
	if got := pressureWords[p.PressureBand][0]; got != p.Feelings[0] {
		t.Errorf("PressureBand=%d 与措辞 %q 不同档（want %q）——band 泄真值", p.PressureBand, p.Feelings[0], got)
	}
}

// TestGoldenCase_PressureTick1 黄金用例（§46-47）：Tick1 后青关
// Pressure 1.029 > 1.0 → 剧痛档（band 5），acuity 2 措辞报
// "强烈胀感"级（PerceivedSignalStrength 静默两档的下限）。
func TestGoldenCase_PressureTick1(t *testing.T) {
	w := NewWorld(goldenCharacter(), goldenEnv())
	w.Ops = Ops{Intensity: []int64{600}} // §46：intensity 0.60
	Advance(w)

	obs := Observe(w, 2)
	p := obs.Points[0]

	// §47：LoadRatio = 1014 → Pressure ≈ 1.028 > 1.0 → 真值档 5（剧痛档）。
	// PressureBand 报感知后档（Perception=0.5 静默两档）→ felt 档 3，
	// 与措辞"强烈胀感·不适"同档（所见即所得）。
	if p.PressureBand != 3 {
		t.Errorf("PressureBand = %d, want 3（§47 真值档 5 经低感知静默两档）", p.PressureBand)
	}
	if got := p.Feelings[0]; got != "强烈胀感·不适" {
		t.Errorf("黄金用例措辞 = %q, want 强烈胀感级", got)
	}
	// §48 叙事：第一轮灵气涌入十分明显。
	if p.Flow != "进入" {
		t.Errorf("Flow = %q, want 进入（首 Tick 吸收）", p.Flow)
	}
	// 环境灵气：木=10 → 沛然；水=4 → 浓郁；金=0.5 → 流转。
	if obs.Env.Qi[Wood] != "木气沛然" {
		t.Errorf("Env.Qi[木] = %q, want 木气沛然", obs.Env.Qi[Wood])
	}
	if obs.Env.Qi[Water] != "水气浓郁" {
		t.Errorf("Env.Qi[水] = %q, want 水气浓郁", obs.Env.Qi[Water])
	}
	if obs.Env.Qi[Metal] != "金气流转" {
		t.Errorf("Env.Qi[金] = %q, want 金气流转", obs.Env.Qi[Metal])
	}
}

// TestPerceptionIsolation_NoTruthLeak 感知隔离律：序列化 Observation
// 后全文搜索数字模式——除 Tick 与 PressureBand（band 序号，契约允许）
// 外，任何真值数字（qi/load/pressure/stability…）都不得出现。
func TestPerceptionIsolation_NoTruthLeak(t *testing.T) {
	w := NewWorld(goldenCharacter(), goldenEnv())
	for i := 0; i < 3; i++ {
		w.Ops = Ops{Intensity: []int64{600}}
		w.Ops.Requests = nil
		Advance(w)
	}
	obs := Observe(w, 3)

	raw, err := json.Marshal(obs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	js := string(raw)
	if !regexp.MustCompile(`"PressureBand":\d`).MatchString(js) {
		t.Fatalf("序列化结果缺 PressureBand 字段（测试前提失效）: %s", js)
	}

	// 剔除契约允许的两类序号后，全文不得再有任何数字。
	stripped := regexp.MustCompile(`"Tick":\d+`).ReplaceAllString(js, "")
	stripped = regexp.MustCompile(`"PressureBand":\d+`).ReplaceAllString(stripped, "")
	if loc := regexp.MustCompile(`[0-9]`).FindStringIndex(stripped); loc != nil {
		start := loc[0] - 40
		if start < 0 {
			start = 0
		}
		end := loc[1] + 40
		if end > len(stripped) {
			end = len(stripped)
		}
		t.Errorf("感知泄漏：Observation 序列化含真值数字 …%s…", stripped[start:end])
	}
}

// TestObserve_DeterministicAndPure 同 World 同 acuity 两次 Observe 结果
// 深度相等；两条同构世界独立推进后观察也一致（同 seed 重演一致）。
func TestObserve_DeterministicAndPure(t *testing.T) {
	w := NewWorld(goldenCharacter(), goldenEnv())
	w.Ops = Ops{Intensity: []int64{600}}
	Advance(w)
	Advance(w)

	o1 := Observe(w, 2)
	o2 := Observe(w, 2)
	if !reflect.DeepEqual(o1, o2) {
		t.Error("同 World 同 acuity 两次 Observe 结果不等——感知不确定")
	}

	w2 := NewWorld(goldenCharacter(), goldenEnv())
	w2.Ops = Ops{Intensity: []int64{600}}
	Advance(w2)
	Advance(w2)
	if !reflect.DeepEqual(o1, Observe(w2, 2)) {
		t.Error("同构世界观察不等——重演一致律被破坏")
	}
}

// TestStateWords 状态词表五档（契约词表）边界。
func TestStateWords(t *testing.T) {
	stabilityCases := []struct {
		s    int64
		want string
	}{
		{1000, "稳如磐石"}, {900, "稳如磐石"}, {899, "稳固"},
		{750, "稳固"}, {749, "微澜"}, {600, "微澜"},
		{599, "不稳"}, {400, "不稳"}, {399, "将溃"}, {0, "将溃"},
	}
	for _, c := range stabilityCases {
		if got := stabilityWord(c.s); got != c.want {
			t.Errorf("stabilityWord(%d) = %q, want %q", c.s, got, c.want)
		}
	}

	opennessCases := []struct {
		o    int64
		want string
	}{
		{0, "闭"}, {249, "闭"}, {250, "微开"}, {499, "微开"},
		{500, "半开"}, {749, "半开"}, {750, "大开"}, {899, "大开"},
		{900, "洞开"}, {1000, "洞开"},
	}
	for _, c := range opennessCases {
		if got := opennessWord(c.o); got != c.want {
			t.Errorf("opennessWord(%d) = %q, want %q", c.o, got, c.want)
		}
	}

	damageCases := []struct {
		d    int64
		want string
	}{
		{0, "无损"}, {249, "隐伤"}, {250, "作痛"},
		{499, "作痛"}, {500, "重创"}, {749, "重创"}, {750, "脉裂"}, {1000, "脉裂"},
	}
	for _, c := range damageCases {
		if got := damageWord(c.d); got != c.want {
			t.Errorf("damageWord(%d) = %q, want %q", c.d, got, c.want)
		}
	}
}

// TestFlowWords flow 词：未推进 → 停滞；黄金用例首 Tick → 进入；
// 经脉流量骤变 → 加速（§15 冲击的体验面）。
func TestFlowWords(t *testing.T) {
	w := NewWorld(goldenCharacter(), goldenEnv())
	if got := Observe(w, 2).Points[0].Flow; got != "停滞" {
		t.Errorf("未推进 Flow = %q, want 停滞", got)
	}

	w.Ops = Ops{Intensity: []int64{600}}
	Advance(w)
	if got := Observe(w, 2).Points[0].Flow; got != "进入" {
		t.Errorf("首 Tick Flow = %q, want 进入", got)
	}

	// 双穴位 + 经脉猛推：流量冲击 → 加速。
	dst := goldenAcupoint("丹田")
	dst.Capacity = 50000
	w2 := NewWorld(Character{
		Affinity:   [ElementCount]int64{1250, 800, 900, 750, 1100},
		Control:    1000,
		Perception: 500,
		BodyGraph: BodyGraph{
			Acupoints: []Acupoint{goldenAcupoint("青关"), dst},
			Channels:  []Channel{{Name: "青关→丹田", From: 0, To: 1, BaseConductance: 1000, Throughput: 2000}},
		},
	}, goldenEnv())
	// 预置青关存木，供传输。
	w2.Character.BodyGraph.Acupoints[0].Qi[Wood] = 20000
	req := FlowRequest{Channel: 0}
	req.Amount[Wood] = 1500
	w2.Ops = Ops{Requests: []FlowRequest{req}}
	rep := Advance(w2)
	if rep.Points[0].CurrentFlow == 0 {
		t.Fatalf("测试前提失效：本 Tick 无经脉流量")
	}
	if got := Observe(w2, 2).Points[0].Flow; got != "加速" {
		t.Errorf("流量冲击 Flow = %q, want 加速（CurrentFlow=%d FlowShock=%d）",
			got, rep.Points[0].CurrentFlow, rep.Points[0].FlowShock)
	}
}
