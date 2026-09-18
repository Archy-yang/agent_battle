package match

// integration_test.go —— v0.9 契约缝隙修复验收（backend/match/api.go 的
// View ↔ 前端 BattleView/MapView 的预计契约）。
//
// 并行工作包之间缺的三个字段在此对账：
//   - EnemyView.owner：rival 条目归属 "rival"，普通怪物 "monster"、
//     Unknown Agent "unknown"（前端 isRivalEnemy 的判据）；
//   - StateView.rival_trigger：rival 侧分类器最近一次触发原因
//     （前端 rivalTrigger() 的消费源，对称于 last_trigger）；
//   - StateView.rival：感知内实时摘要，感知外/无 rival 包为 null
//     （信息边界：不剧透雾后位置）。
//
// 驾驶口径：玩家全程 idle（nil 意图 / idlePilot），rival 由 ScriptedRival
// 经 scheduleRival 驱动——零 LLM 依赖，隔离 AGENTBATTLE_LLM_* 环境漂移。

import (
	"testing"

	"agentbattle/backend/engine"
)

// activateSensor issues one activate intent for the player's sensor
// （感识开：感知半径 1 → 4，rival 贴脸前即可入圈）.
func activateSensor(t *testing.T, m *Match) {
	t.Helper()
	p := m.W.Entities[m.PlayerID]
	if p == nil {
		t.Fatal("player missing")
	}
	s := p.FindComponent(engine.CSensor)
	if s == nil {
		t.Fatal("player has no sensor — perception contract unverifiable")
	}
	m.Tick([]engine.Intent{{Type: "activate", Component: s.ID}})
	if s = p.FindComponent(engine.CSensor); s == nil || !s.Active {
		t.Fatal("player sensor did not activate")
	}
}

// TestViewContractRivalInView: V05 局 rival 进入玩家感知后——enemies 含
// owner="rival"、rival 非 null 且与实体逐位一致、rival_trigger 有值；
// rival 退出感知后 rival 归 null 且不再出现在 enemies.
func TestViewContractRivalInView(t *testing.T) {
	m := NewV05("contract", 42, nil)
	if m.RivalID == 0 {
		t.Fatal("V05 has no rival")
	}
	banishOthers(t, m) // 受控：初置敌对只留 rival（断言只锚定 rival，懒激活怪不碍事）
	activateSensor(t, m)

	spotted := false
	for i := 0; i < 30 && !spotted; i++ {
		m.Tick(nil) // 玩家 idle：接近与触发全由 rival 驱动
		if m.W.Entities[m.PlayerID] == nil {
			t.Fatalf("tick %d: player destroyed before the rival contract could be checked", m.W.Tick)
		}
		if m.W.Entities[m.RivalID] == nil {
			t.Fatalf("tick %d: rival vanished", m.W.Tick)
		}
		for _, id := range m.visibleHostiles() {
			if id == m.RivalID {
				spotted = true
			}
		}
	}
	if !spotted {
		t.Fatalf("rival never entered perception in 30 ticks: player=%v rival=%v",
			m.W.Entities[m.PlayerID].Pos, m.W.Entities[m.RivalID].Pos)
	}

	view := m.View()
	// 1. enemies 含 owner="rival" 的条目；其余条目归属不得是 rival。
	found := false
	for _, en := range view.Enemies {
		if en.ID == m.RivalID {
			found = true
			if en.Owner != "rival" {
				t.Fatalf("rival enemy owner = %q, want \"rival\"", en.Owner)
			}
		} else if en.Owner == "rival" {
			t.Fatalf("non-rival enemy %d tagged owner=\"rival\"", en.ID)
		}
	}
	if !found {
		t.Fatalf("rival %d missing from view.Enemies: %+v", m.RivalID, view.Enemies)
	}
	// 2. rival 非 null，字段与实体实时状态逐位一致（essence 为中文名）。
	rv := m.W.Entities[m.RivalID]
	if got := view.Rival; got == nil {
		t.Fatal("view.Rival = nil while the rival is inside perception")
	} else if got.ID != m.RivalID || got.Pos != rv.Pos || got.Essence != rv.Essence.String() ||
		got.Integrity != rv.Integrity || got.MaxIntegrity != rv.MaxIntegrity || got.Heat != rv.Heat {
		t.Fatalf("view.Rival = %+v, want id=%d pos=%v essence=%s form=%d/%d heat=%d",
			got, m.RivalID, rv.Pos, rv.Essence.String(), rv.Integrity, rv.MaxIntegrity, rv.Heat)
	}
	// 3. rival_trigger 有值，与 rival 侧日志末项一致。
	if len(m.rivalTriggerLog) == 0 {
		t.Fatal("rivalTriggerLog empty — the rival never triggered")
	}
	if view.RivalTrigger == "" {
		t.Fatal("view.RivalTrigger empty while the rival has triggered")
	}
	if view.RivalTrigger != m.LastRivalTrigger() {
		t.Fatalf("view.RivalTrigger = %q, want %q", view.RivalTrigger, m.LastRivalTrigger())
	}

	// 4. rival 退出感知 → rival 为 null（不剧透雾后位置），enemies 同步收敛。
	m.W.Entities[m.RivalID].Pos = engine.Hex{Q: 400, R: -400}
	view = m.View()
	if view.Rival != nil {
		t.Fatalf("view.Rival = %+v after the rival left perception, want nil", view.Rival)
	}
	for _, en := range view.Enemies {
		if en.ID == m.RivalID {
			t.Fatal("rival still listed in view.Enemies after leaving perception")
		}
	}
}

// TestViewContractV04NoRival: 无 rival 包（V04）——rival 恒 null、
// rival_trigger 恒空、enemies 无 owner="rival".
func TestViewContractV04NoRival(t *testing.T) {
	m := NewV04("contract-v04", 42, nil)
	if m.RivalID != 0 {
		t.Fatalf("V04 unexpectedly has rival id %d", m.RivalID)
	}
	m.Pilot = idlePilot{} // 隔离 AGENTBATTLE_LLM_* 环境漂移
	for i := 0; i < 5; i++ {
		m.TickAuto()
	}
	view := m.View()
	if view.Rival != nil {
		t.Fatalf("V04 view.Rival = %+v, want nil (no-rival pack)", view.Rival)
	}
	if view.RivalTrigger != "" {
		t.Fatalf("V04 view.RivalTrigger = %q, want \"\"", view.RivalTrigger)
	}
	for _, en := range view.Enemies {
		if en.Owner == "rival" {
			t.Fatalf("V04 enemy %d tagged owner=\"rival\"", en.ID)
		}
		if en.Owner != "monster" && en.Owner != "unknown" {
			t.Fatalf("V04 enemy %d owner = %q, want \"monster\"|\"unknown\"", en.ID, en.Owner)
		}
	}
}
