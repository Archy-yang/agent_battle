package engine

import "testing"

// —— 相生相克矩阵（world_laws §5） ——

func TestGenerates(t *testing.T) {
	want := map[[2]Essence]bool{
		{Wood, Fire}: true, {Fire, Earth}: true, {Earth, Metal}: true,
		{Metal, Water}: true, {Water, Wood}: true,
	}
	for _, a := range All() {
		for _, b := range All() {
			got := Generates(a, b)
			if got != want[[2]Essence{a, b}] {
				t.Errorf("Generates(%v,%v)=%v", a, b, got)
			}
		}
	}
}

func TestRestrains(t *testing.T) {
	want := map[[2]Essence]bool{
		{Wood, Earth}: true, {Earth, Water}: true, {Water, Fire}: true,
		{Fire, Metal}: true, {Metal, Wood}: true,
	}
	for _, a := range All() {
		for _, b := range All() {
			got := Restrains(a, b)
			if got != want[[2]Essence{a, b}] {
				t.Errorf("Restrains(%v,%v)=%v", a, b, got)
			}
		}
	}
}

// —— 克曲线：侮—微—衡—乘（world_laws §5） ——

func TestResolveRestraintZones(t *testing.T) {
	cases := []struct {
		power, strength int
		zone            CurveZone
		damage          int
		backlash        int
		spill           int
	}{
		{100, 600, ZoneWu, 0, 5, 0},        // r≈0.17 侮：反噬 = 100×0.5÷10
		{100, 250, ZoneWei, 0, 0, 0},       // r=0.40 微：杯水车薪
		{600, 600, ZoneHeng, 90, 0, 0},     // r=1.00 衡：600×1.5÷10
		{1999, 1000, ZoneHeng, 299, 0, 0},  // r=1.999 仍在衡区
		{1200, 500, ZoneCheng, 300, 0, 250}, // r=2.4 乘：×2.5÷10 溢出 25%
		{2400, 500, ZoneCheng, 600, 0, 500}, // r=4.8 溢出 50%
	}
	for _, c := range cases {
		got := ResolveRestraint(c.power, c.strength)
		if got.Zone != c.zone || got.Damage != c.damage ||
			got.Backlash != c.backlash || got.Spill != c.spill {
			t.Errorf("ResolveRestraint(%d,%d) = %+v, want zone=%v dmg=%d back=%d spill=%d",
				c.power, c.strength, got, c.zone, c.damage, c.backlash, c.spill)
		}
	}
}

// 非克关系一律 ×0.5（world_laws 律五：只有生克二门）。
func TestApplyPowerNeutral(t *testing.T) {
	// 火→土 是相生不是相克 → 中性 ×0.5
	got := ApplyPower(Fire, Earth, 600, 600)
	if got.Zone != ZoneHeng || got.Damage != 30 {
		t.Errorf("ApplyPower(Fire,Earth,600,600) = %+v", got)
	}
	// 水克火 → 走克曲线
	got = ApplyPower(Water, Fire, 600, 600)
	if got.Zone != ZoneHeng || got.Damage != 90 {
		t.Errorf("ApplyPower(Water,Fire,600,600) = %+v", got)
	}
}

// —— 阴阳曲线（world_laws §3）：热与迹 ∝ x² ——

func TestCurves(t *testing.T) {
	if got := EnergyCost(1000); got != 10 {
		t.Errorf("EnergyCost(1000)=%d", got)
	}
	if got := EnergyCost(400); got != 4 {
		t.Errorf("EnergyCost(400)=%d", got)
	}
	if got := HeatGain(400); got != 1 {
		t.Errorf("HeatGain(400)=%d", got)
	}
	if got := HeatGain(1000); got != 10 {
		t.Errorf("HeatGain(1000)=%d", got)
	}
	// 平方曲线：功率翻倍，热量翻四倍以上
	if HeatGain(800) < 4*HeatGain(400) {
		t.Errorf("heat curve is not superlinear: %d vs %d", HeatGain(800), HeatGain(400))
	}
	if SignalEmission(1000) != HeatGain(1000) {
		t.Errorf("signal should mirror heat (∝x²)")
	}
}
