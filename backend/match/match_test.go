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
