package match

import (
	"fmt"
	"strings"
	"testing"

	"agentbattle/backend/engine"
)

// 整局集成测试：脚本 Agent 在场景包 mvp-0.1 内拿到 Fragment 并回到
// 出生点（world_rule §31 的最小可玩性证明）。
func TestScriptedAgentWinsMVP(t *testing.T) {
	m := New("test", 42, nil)
	for m.Status == StatusRunning && m.W.Tick < m.TurnLimit {
		m.TickAuto()
	}
	if m.Status != StatusWon {
		t.Fatalf("expected win, got %s (%s)", m.Status, m.Summary())
	}
	if m.W.Tick > m.TurnLimit {
		t.Errorf("exceeded turn limit")
	}
	// 胜利时必须携带 Fragment 并站在出生点
	p := m.W.Entities[m.PlayerID]
	if len(p.Carrying) == 0 || p.Pos != m.Spawn {
		t.Errorf("win state wrong: carrying=%v pos=%v", p.Carrying, p.Pos)
	}
	t.Logf("won at turn %d: %s", m.W.Tick, m.Summary())
}

// 同 seed 两次对局，事件流逐字节一致（world_genesis §7 决定性）。
func TestDeterminism(t *testing.T) {
	run := func() (*Match, string) {
		m := New("d", 20260917, nil)
		for m.Status == StatusRunning && m.W.Tick < m.TurnLimit {
			m.TickAuto()
		}
		var b strings.Builder
		for _, e := range m.Events {
			fmt.Fprintf(&b, "[%d]%s#%d:%s\n", e.Tick, e.Type, e.Entity, e.Text)
		}
		return m, b.String()
	}
	_, logA := run()
	_, logB := run()
	if logA != logB {
		t.Fatalf("replay mismatch:\n--- A ---\n%s\n--- B ---\n%s", logA, logB)
	}
}

// 玩家形散即负（world_laws 律二）。
func TestLossOnDeath(t *testing.T) {
	m := New("loss", 7, nil)
	p := m.W.Entities[m.PlayerID]
	p.Integrity = 1
	p.Heat = 100 // 强制过热
	m.Tick(nil)
	if m.Status != StatusLost {
		t.Fatalf("expected loss, got %s", m.Summary())
	}
}

// Guidance 消耗 Influence；余额不足被拒（strage_1 影响表）。
func TestGuidanceEconomy(t *testing.T) {
	m := New("g", 1, nil)
	if err := m.AddGuidance("concern", "类型非法"); err == nil {
		t.Fatal("unknown kind should be rejected")
	}
	if err := m.AddGuidance("guide", "优先生存。"); err != nil {
		t.Fatalf("guide rejected: %v", err)
	}
	if m.Influence != InfluenceStart-CostGuide {
		t.Errorf("influence = %.1f", m.Influence)
	}
	if err := m.AddGuidance("deeplink", "重新评估战略。"); err != nil {
		t.Fatalf("deeplink rejected: %v", err)
	}
	if err := m.AddGuidance("whisper", "注意能源。"); err != nil {
		t.Fatalf("whisper rejected: %v", err)
	}
	if err := m.AddGuidance("deeplink", "再来一次深谈。"); err == nil {
		t.Fatal("insufficient influence should be rejected") // 余 1.5 < 2.0
	}
	// 预测命中返还 +0.5
	if err := m.Predict("move"); err != nil {
		t.Fatalf("predict rejected: %v", err)
	}
	m.lastIntents = []engine.Intent{{Type: "move", To: &engine.Vec2{X: 1, Y: 9}}}
	m.W.Tick++ // 模拟推进一步
	m.judgePending()
	if m.Influence != InfluenceStart-CostGuide-CostDeepLink-CostWhisper+PredictReward {
		t.Errorf("influence after hit = %.1f", m.Influence)
	}
}

// v0.2：三只怪物入场；玩家靠近后怪物逐回合接近直至相邻并发起攻击
// （离火 activate，事件流里出现 attack）。
func TestV02MonsterChaseAndAttack(t *testing.T) {
	m := NewV02("v02c", 42, nil)
	if len(m.MonsterIDs) != 3 {
		t.Fatalf("expected 3 monsters, got %d", len(m.MonsterIDs))
	}
	p := m.W.Entities[m.PlayerID]
	mon := m.W.Entities[m.MonsterIDs[0]] // 火行怪
	p.Pos = engine.Vec2{X: 5, Y: 5}
	mon.Pos = engine.Vec2{X: 5, Y: 8} // 距离 3：进入追击圈

	attacked := false
	lastDist := engine.Chebyshev(p.Pos, mon.Pos)
	for i := 0; i < 5 && !attacked; i++ {
		m.Tick(nil)
		d := engine.Chebyshev(p.Pos, mon.Pos)
		if d > lastDist {
			t.Fatalf("monster did not close in: dist %d → %d", lastDist, d)
		}
		lastDist = d
		for _, ev := range m.lastEvs {
			if ev.Type == "attack" && ev.Entity == mon.ID {
				attacked = true
			}
		}
	}
	if !attacked {
		t.Fatalf("monster never attacked: player=%v monster=%v", p.Pos, mon.Pos)
	}
	if p.Integrity >= 100 {
		t.Errorf("player took no damage from the attack")
	}
	if v := m.View(); !v.InCombat {
		t.Errorf("in_combat should be true in melee")
	}
	// 敌人视图：相邻敌对实体在被动半径 1 内可见
	if !enemyVisible(m.View(), mon.ID) {
		t.Errorf("adjacent monster should be visible in enemies view")
	}
	// 远离后：无相邻敌对实体、本 tick 无战斗事件 → in_combat 为 false
	p.Pos = engine.Vec2{X: 0, Y: 23}
	m.Tick(nil)
	if v := m.View(); v.InCombat {
		t.Errorf("in_combat should be false when far from every hostile")
	}
	// enemies 视图：感知半径 1（感识未开启）下，远处的怪物不可见
	if v := m.View(); len(v.Enemies) != 0 {
		t.Errorf("enemies should be empty out of perception, got %+v", v.Enemies)
	}
	// 感识开启：感知半径扩到 4——距离 3 的怪物可见（被动半径看不见）
	sens := p.FindComponent(engine.CSensor)
	p.Pos = engine.Vec2{X: 10, Y: 10}
	mon.Pos = engine.Vec2{X: 10, Y: 6} // 距离 4：本 tick 怪物追近一格
	m.Tick([]engine.Intent{{Type: "activate", Component: sens.ID}})
	if d := engine.Chebyshev(p.Pos, mon.Pos); d <= 1 {
		t.Fatalf("precondition broken: monster distance %d", d)
	} else if !enemyVisible(m.View(), mon.ID) {
		t.Errorf("monster at distance %d should be visible with the sensor on", d)
	}
}

func enemyVisible(v *StateView, id engine.EntityID) bool {
	for _, en := range v.Enemies {
		if en.ID == id {
			return true
		}
	}
	return false
}

// v0.2 全局：Registry 按 scenario 名选择场景；默认与旧调用点不变；
// 未知场景名返回 nil（由调用方转成 4xx）。
func TestRegistryScenarioSelect(t *testing.T) {
	r := NewRegistry(nil)
	if m := r.Create(7); m.PackName != "scenario/mvp-0.1" {
		t.Errorf("default pack = %s", m.PackName)
	}
	if m := r.Create(7, "mvp"); m.PackName != "scenario/mvp-0.1" {
		t.Errorf("mvp pack = %s", m.PackName)
	}
	m := r.Create(7, "v02")
	if m.PackName != "scenario/v02-0.2" {
		t.Errorf("v02 pack = %s", m.PackName)
	}
	if m.W.W != 24 || m.W.H != 24 {
		t.Errorf("v02 map = %dx%d", m.W.W, m.W.H)
	}
	if m.TurnLimit != 60 {
		t.Errorf("v02 turn limit = %d", m.TurnLimit)
	}
	if len(m.MonsterIDs) != 3 {
		t.Errorf("v02 monsters = %d", len(m.MonsterIDs))
	}
	if m := r.Create(7, "v99"); m != nil {
		t.Errorf("unknown scenario should be nil, got %s", m.PackName)
	}
	// 落库校验：非法场景不注册，合法场景按 id 可取回
	if got := r.Get(m.ID); got == nil || got != m {
		t.Errorf("registry lookup after selection failed: %+v", got)
	}
}

// v0.2 决定性：同一 seed 跑 30 回合两次，事件流逐字节一致
// （world_genesis §7；怪物游荡的 Rng 消耗也必须可重放）。
func TestV02Determinism30Turns(t *testing.T) {
	run := func() string {
		m := NewV02("d2", 20260917, nil)
		for m.Status == StatusRunning && m.W.Tick < 30 {
			m.TickAuto()
		}
		var bld strings.Builder
		for _, e := range m.Events {
			fmt.Fprintf(&bld, "[%d]%s#%d:%s\n", e.Tick, e.Type, e.Entity, e.Text)
		}
		return bld.String()
	}
	logA, logB := run(), run()
	if logA != logB {
		t.Fatalf("v02 replay mismatch:\n--- A ---\n%s\n--- B ---\n%s", logA, logB)
	}
}

// 战斗状态另一路径：即便距离拉开，本 tick 玩家/敌对实体被
// attack/spill 作用（事件侧）也置 in_combat。
func TestInCombatViaEvents(t *testing.T) {
	m := NewV02("v02e", 42, nil)
	p := m.W.Entities[m.PlayerID]
	mon := m.W.Entities[m.MonsterIDs[0]]
	p.Pos = engine.Vec2{X: 0, Y: 23}
	mon.Pos = engine.Vec2{X: 12, Y: 12} // 与玩家和所有敌对实体拉开

	m.lastEvs = []engine.Event{{Tick: m.W.Tick, Type: "attack", Entity: m.PlayerID}}
	if !m.InCombat() {
		t.Errorf("in_combat should be true when the player is attacked this tick")
	}
	m.lastEvs = []engine.Event{{Tick: m.W.Tick, Type: "spill", Entity: mon.ID}}
	if !m.InCombat() {
		t.Errorf("in_combat should be true when a hostile is hit by spill this tick")
	}
	m.lastEvs = nil
	if m.InCombat() {
		t.Errorf("in_combat should be false with no hostiles near and no combat events")
	}
}
