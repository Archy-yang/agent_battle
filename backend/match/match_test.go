package match

import (
	"fmt"
	"strings"
	"testing"
)

// 整局集成测试：脚本 Agent 在场景包 mvp-0.1 内拿到 Fragment 并回到
// 出生点（world_rule §31 的最小可玩性证明）。
func TestScriptedAgentWinsMVP(t *testing.T) {
	m := New("test", 42)
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
		m := New("d", 20260917)
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
	m := New("loss", 7)
	p := m.W.Entities[m.PlayerID]
	p.Integrity = 1
	p.Heat = 100 // 强制过热
	m.Tick(nil)
	if m.Status != StatusLost {
		t.Fatalf("expected loss, got %s", m.Summary())
	}
}

// Guidance 只记录、不代行（v0.1；M1 起进入 Agent 决策上下文）。
func TestGuidanceIsLogged(t *testing.T) {
	m := New("g", 1)
	m.AddGuidance("concern", "我们的 Energy 已经很低了。")
	if len(m.Guidance) != 1 || m.Guidance[0].Kind != "concern" {
		t.Fatalf("guidance = %+v", m.Guidance)
	}
}
