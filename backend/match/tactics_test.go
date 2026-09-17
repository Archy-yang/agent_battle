package match

// v0.6 克关系战术预览测试：StateView.Tactics 把克曲线的公开推论算给
// 玩家看（docs/world_laws.md §5）。边界口径与 engine.ResolveRestraint
// 一致：r = power / 敌本源强度，r<0.25 侮，r<0.5 微，r<2 衡，否则乘。

import (
	"testing"

	"agentbattle/backend/engine"
)

// giveEmitter mounts (or retunes) the player's 离火 with the given
// essence — the MVP player ships without one, the v0.2+ player has one.
func giveEmitter(p *engine.Entity, ess engine.Essence) {
	if em := p.FindComponent(engine.CEmitter); em != nil {
		em.Essence = ess
		return
	}
	p.AddComponent(engine.CEmitter, ess)
}

// stageTactics builds a fresh MVP match, gives the player an emitter of
// the given essence, parks the Unknown Agent adjacent (perception radius
// 1, sensor off) and retunes its essence/strength.
func stageTactics(t *testing.T, emitterEss, enemyEss engine.Essence, enemyStrength int) *Match {
	t.Helper()
	m := New("tactics", 3, nil)
	p := m.W.Entities[m.PlayerID]
	if p == nil {
		t.Fatal("no player entity")
	}
	giveEmitter(p, emitterEss)
	if s := p.FindComponent(engine.CSensor); s != nil {
		s.Active = false // 被动感知：半径 1
	}
	u := m.W.Entities[m.UnknownID]
	if u == nil {
		t.Fatal("no unknown entity")
	}
	u.Essence = enemyEss
	u.MaxIntegrity = enemyStrength
	u.Integrity = enemyStrength
	u.Pos = p.Pos.Add(engine.HexDirs[0]) // HexDist 1：感知内
	return m
}

// cardOf picks the tactics card of the Unknown Agent.
func cardOf(t *testing.T, m *Match) *TacticsView {
	t.Helper()
	for i := range m.Tactics() {
		if m.Tactics()[i].EnemyID == m.UnknownID {
			return &m.Tactics()[i]
		}
	}
	t.Fatalf("no tactics card for unknown %d in %v", m.UnknownID, m.Tactics())
	return nil
}

// 金甲 + 我方火 emitter：火克金，600/500 → r=1.2 → 衡（正常克制）。
func TestTacticsMetalTargetHeng(t *testing.T) {
	m := stageTactics(t, engine.Fire, engine.Metal, 500)
	c := cardOf(t, m)
	if c.MyEssence != "火" || c.EnemyEssence != "金" {
		t.Fatalf("essences = %q→%q, want 火→金", c.MyEssence, c.EnemyEssence)
	}
	if c.PredictedZone != "衡" {
		t.Fatalf("zone = %q, want 衡", c.PredictedZone)
	}
	if c.Note != "正常克制" {
		t.Fatalf("note = %q", c.Note)
	}
	if c.MyPowerGuess != tacticPowerGuess {
		t.Fatalf("power guess = %d, want %d", c.MyPowerGuess, tacticPowerGuess)
	}
}

// 水身 + 火敌：水克火，但强度比决定落区——
//   r = 600/300  = 2.0   → 乘（失控放大，恰好压线：衡上界开区间）
//   r = 600/3000 = 0.2   → 侮（以弱击强，反噬）
//   r = 600/2400 = 0.25  → 微（恰好压线：侮上界开区间）
func TestTacticsWaterVsFireZoneBoundaries(t *testing.T) {
	cases := []struct {
		strength int
		zone     string
		note     string
	}{
		{300, "乘", "失控放大，会波及周围"},
		{3000, "侮", "以弱击强，会反噬"},
		{2400, "微", "杯水车薪"},
	}
	for _, tc := range cases {
		m := stageTactics(t, engine.Water, engine.Fire, tc.strength)
		c := cardOf(t, m)
		if c.PredictedZone != tc.zone || c.Note != tc.note {
			t.Errorf("strength %d: got %s/%q, want %s/%q",
				tc.strength, c.PredictedZone, c.Note, tc.zone, tc.note)
		}
	}
}

// 非克关系（木 vs 金）不落克曲线：中性，作用减半（律五：只有生克二门）。
func TestTacticsNeutralNonRestraint(t *testing.T) {
	m := stageTactics(t, engine.Wood, engine.Metal, 500)
	c := cardOf(t, m)
	if c.PredictedZone != "中性" || c.Note != "非克关系，作用减半" {
		t.Fatalf("got %s/%q, want 中性/非克关系，作用减半", c.PredictedZone, c.Note)
	}
}

// 无离火组件：zone 与 my_essence 留空，note 提示无法反击。
func TestTacticsNoEmitter(t *testing.T) {
	m := New("tactics", 3, nil)
	p := m.W.Entities[m.PlayerID]
	for i, c := range p.Components { // 摘掉可能存在的离火
		if c.Type == engine.CEmitter {
			p.Components = append(p.Components[:i], p.Components[i+1:]...)
			break
		}
	}
	u := m.W.Entities[m.UnknownID]
	u.Pos = p.Pos.Add(engine.HexDirs[0])

	c := cardOf(t, m)
	if c.PredictedZone != "" || c.MyEssence != "" {
		t.Fatalf("zone/essence = %q/%q, want empty", c.PredictedZone, c.MyEssence)
	}
	if c.Note != "无离火，无法反击" {
		t.Fatalf("note = %q", c.Note)
	}
	if c.EnemyEssence != u.Essence.String() {
		t.Fatalf("enemy essence = %q", c.EnemyEssence)
	}
}

// 信息边界：感知外（距离 2、感识关闭）不出现战术卡；感识开启（半径 4）
// 后同一敌人出现——战术卡与 Enemies 严格同边界。
func TestTacticsPerceptionBoundary(t *testing.T) {
	m := stageTactics(t, engine.Fire, engine.Metal, 500)
	p := m.W.Entities[m.PlayerID]
	u := m.W.Entities[m.UnknownID]
	u.Pos = p.Pos.Add(engine.HexDirs[0]).Add(engine.HexDirs[0]) // HexDist 2

	if cards := m.Tactics(); len(cards) != 0 {
		t.Fatalf("sensor off, dist 2: got %d cards, want 0 (%v)", len(cards), cards)
	}
	if sv := m.View(); len(sv.Enemies) != 0 || len(sv.Tactics) != 0 {
		t.Fatalf("view: enemies=%d tactics=%d, want 0/0", len(sv.Enemies), len(sv.Tactics))
	}

	if s := p.FindComponent(engine.CSensor); s != nil {
		s.Active = true // 半径 4
	}
	c := cardOf(t, m)
	if c.PredictedZone != "衡" {
		t.Fatalf("sensor on: zone = %q, want 衡", c.PredictedZone)
	}
	if sv := m.View(); len(sv.Enemies) != 1 || len(sv.Tactics) != 1 {
		t.Fatalf("view: enemies=%d tactics=%d, want 1/1", len(sv.Enemies), len(sv.Tactics))
	}
}
