package match

// monsters_test.go —— v0.8 WPC 三种怪物行为模板 (world_genesis §5.1
// Tier 1：行为模板 + 参数变体；§5.2 铁律：同一条物理链、无特权 AI)。
//
// 覆盖：
//   - 哨卫 sentinel：警戒圈（HexDist ≤3）内才追击，圈外驻守锚点 ±2；
//     玩家离开追击圈 3 tick 后返回锚点
//   - 掠食者 predator：远距（dist 4~5）即开始接近（最短路径）
//   - 温和开局：出生环 3 只固定 wanderer
//   - 模板分布：hash 分配 50/30/20，纯函数可重复
//   - 行为确定性：同 seed 同激活序列 → 同事件流、同怪物轨迹、同 mind
//
// 驾驶口径：受控包（PlainTerrain + 手摆单只怪物）逐 tick m.Tick(nil)，
// 玩家位白盒传送——只测怪物侧行为，不掺 pilot 随机性。

import (
	"fmt"
	"strings"
	"testing"

	"agentbattle/backend/agent"
	"agentbattle/backend/engine"
	"agentbattle/backend/scenario"
)

// temperPack builds a controlled one-monster pack：全平地、玩家 (0,0)、
// Fragment/Unknown 远置（不掺合成变数），怪物按规格手摆。包名不取
// v04PackName → 懒激活不跑，场上有且只有这只怪。
func temperPack(t *testing.T, seed uint64, spec scenario.MonsterSpec) *Match {
	t.Helper()
	pack := &scenario.Pack{
		Name: "test/monsters-0.8", Seed: seed, TurnLimit: 60,
		TerrainFn:    engine.PlainTerrain,
		PlayerSpawn:  engine.Hex{},
		FragmentPos:  engine.Hex{Q: 0, R: 30},
		UnknownSpawn: engine.Hex{Q: 0, R: -30},
		MonsterSpawns: []scenario.MonsterSpec{
			{Essence: engine.Fire, Form: 60, Pos: spec.Pos, Temper: spec.Temper},
		},
	}
	return newMatch("temper", seed, nil, pack)
}

// tickEventsOf returns this tick's events of the given type.
func tickEventsOf(m *Match, typ string) []engine.Event {
	var out []engine.Event
	for _, ev := range m.lastEvs {
		if ev.Type == typ {
			out = append(out, ev)
		}
	}
	return out
}

// mindOf returns the monster's mind (白盒).
func mindOf(m *Match, id engine.EntityID) *monsterMind {
	return m.minds[id]
}

// 哨卫：玩家在警戒圈（HexDist >3）外时几乎不游荡——始终驻守锚点 ±2、
// 永不进抵玩家 4 格之内、从不攻击。
func TestSentinelHoldsAnchorOutsideAlertRing(t *testing.T) {
	m := temperPack(t, 42, scenario.MonsterSpec{Pos: engine.Hex{Q: 6, R: 0}, Temper: scenario.TemperSentinel})
	mon := m.W.Entities[m.MonsterIDs[0]]
	anchor := mon.Pos
	p := m.W.Entities[m.PlayerID]
	if d := engine.HexDist(p.Pos, mon.Pos); d != 6 {
		t.Fatalf("precondition: dist = %d, want 6 (> 警戒圈 3)", d)
	}
	for i := 0; i < 24; i++ {
		m.Tick(nil)
		if len(tickEventsOf(m, "attack")) > 0 {
			t.Fatalf("tick %d: sentinel attacked from outside the alert ring", m.W.Tick)
		}
		if d := engine.HexDist(mon.Pos, p.Pos); d < 4 {
			t.Fatalf("tick %d: sentinel closed to dist %d while player outside the ring", m.W.Tick, d)
		}
		if d := engine.HexDist(mon.Pos, anchor); d > sentinelPatrolRadius {
			t.Fatalf("tick %d: sentinel strayed %d from anchor (patrol ±%d)", m.W.Tick, d, sentinelPatrolRadius)
		}
		if mind := mindOf(m, mon.ID); mind.hot {
			t.Fatalf("tick %d: sentinel hot with player outside the ring", m.W.Tick)
		}
	}
}

// 哨卫：玩家进入警戒圈（HexDist ≤3）即追击；相邻攻击 power 550。
func TestSentinelChasesInsideAlertRing(t *testing.T) {
	m := temperPack(t, 42, scenario.MonsterSpec{Pos: engine.Hex{Q: 6, R: 0}, Temper: scenario.TemperSentinel})
	mon := m.W.Entities[m.MonsterIDs[0]]
	p := m.W.Entities[m.PlayerID]
	p.Pos = mon.Pos.Add(engine.Hex{Q: 3, R: 0}) // HexDist 3：警戒圈边缘
	m.Tick(nil)
	if d := engine.HexDist(p.Pos, mon.Pos); d >= 3 {
		t.Fatalf("sentinel inside alert ring did not close in: dist %d", d)
	}
	// 追抵相邻 → 攻击，且功率为哨卫的 550（attack 事件只带曲线后的作用
	// 量，意图功率白盒取自 monsterAct 的出 Intent）
	attacked := false
	for i := 0; i < 6 && !attacked; i++ {
		m.Tick(nil)
		for _, ev := range tickEventsOf(m, "attack") {
			if ev.Entity == mon.ID {
				attacked = true
			}
		}
	}
	if !attacked {
		t.Fatalf("sentinel never attacked: player=%v monster=%v", p.Pos, mon.Pos)
	}
	if intent := m.monsterAct(mon.ID); len(intent) == 0 || intent[0].Type != "activate" || intent[0].Power != temperAttackPower[scenario.TemperSentinel] {
		t.Fatalf("adjacent sentinel intent = %+v, want activate power %d", intent, temperAttackPower[scenario.TemperSentinel])
	}
	if mind := mindOf(m, mon.ID); !mind.hot {
		t.Fatalf("sentinel should be hot while the player is in the ring")
	}
}

// 哨卫：玩家离开追击圈连续 3 tick 后解除警戒、返回锚点。
func TestSentinelReturnsToAnchorAfterLostGrace(t *testing.T) {
	m := temperPack(t, 42, scenario.MonsterSpec{Pos: engine.Hex{Q: 6, R: 0}, Temper: scenario.TemperSentinel})
	mon := m.W.Entities[m.MonsterIDs[0]]
	p := m.W.Entities[m.PlayerID]
	p.Pos = mon.Pos.Add(engine.Hex{Q: 2, R: 0}) // 入圈：警戒
	m.Tick(nil)
	if mind := mindOf(m, mon.ID); !mind.hot {
		t.Fatalf("precondition: sentinel not hot after the player entered the ring")
	}
	p.Pos = engine.Hex{Q: 30, R: 0} // 玩家远遁
	returned := false
	for i := 0; i < 24; i++ {
		m.Tick(nil)
		mind := mindOf(m, mon.ID)
		if i < sentinelLostGrace && !mind.hot {
			t.Fatalf("tick %d: sentinel gave up before the %d-tick grace elapsed", m.W.Tick, sentinelLostGrace)
		}
		if !mind.hot && engine.HexDist(mon.Pos, mind.anchor) <= sentinelPatrolRadius {
			returned = true
			break
		}
	}
	if !returned {
		t.Fatalf("sentinel never returned to its anchor: pos=%v anchor=%v", mon.Pos, mindOf(m, mon.ID).anchor)
	}
}

// 掠食者：远距（dist 4~5，超出游荡者的 3 格圈）即开始沿最短路径接近，
// 逐 tick 逼近直至相邻攻击 power 450。
func TestPredatorApproachesFromFarDistance(t *testing.T) {
	m := temperPack(t, 42, scenario.MonsterSpec{Pos: engine.Hex{Q: 5, R: 0}, Temper: scenario.TemperPredator})
	mon := m.W.Entities[m.MonsterIDs[0]]
	p := m.W.Entities[m.PlayerID]
	if d := engine.HexDist(p.Pos, mon.Pos); d != 5 {
		t.Fatalf("precondition: dist = %d, want 5", d)
	}
	last := 5
	attacked := false
	for i := 0; i < 8 && !attacked; i++ {
		m.Tick(nil)
		d := engine.HexDist(p.Pos, mon.Pos)
		if d > 1 && d >= last { // 相邻即转攻击，距离不再变化
			t.Fatalf("tick %d: predator did not close the shortest path: dist %d → %d", m.W.Tick, last, d)
		}
		last = d
		for _, ev := range tickEventsOf(m, "attack") {
			if ev.Entity == mon.ID {
				attacked = true
			}
		}
	}
	if !attacked {
		t.Fatalf("predator never attacked: dist now %d", last)
	}
	// 相邻出招：activate power 450（意图功率白盒取自 monsterAct）
	if intent := m.monsterAct(mon.ID); len(intent) == 0 || intent[0].Type != "activate" || intent[0].Power != temperAttackPower[scenario.TemperPredator] {
		t.Fatalf("adjacent predator intent = %+v, want activate power %d", intent, temperAttackPower[scenario.TemperPredator])
	}
}

// 温和开局：出生环 3 只固定 wanderer（保底温和开局，与 v0.6 平衡结论
// 一致），锚点即出生位。
func TestBirthRingMonstersAreWanderers(t *testing.T) {
	for _, seed := range []uint64{7, 42, 20260917} {
		m := NewV04("birth", seed, nil)
		if len(m.MonsterIDs) != 3 {
			t.Fatalf("seed %d: birth ring monsters = %d, want 3", seed, len(m.MonsterIDs))
		}
		pack := scenario.V04(seed)
		for i, id := range m.MonsterIDs {
			mind := mindOf(m, id)
			if mind == nil {
				t.Fatalf("seed %d: monster %d has no mind", seed, id)
			}
			if mind.temper != scenario.TemperWanderer {
				t.Fatalf("seed %d: birth-ring monster %d temper = %q, want wanderer", seed, id, mind.temper)
			}
			if mind.anchor != pack.MonsterSpawns[i].Pos {
				t.Fatalf("seed %d: monster %d anchor = %v, want %v", seed, id, mind.anchor, pack.MonsterSpawns[i].Pos)
			}
		}
	}
}

// 模板分布：懒激活 hash 分配 50/30/20，且为 seed 的纯函数（逐位可重复）。
func TestTemperDistribution(t *testing.T) {
	const seed = 20260917
	counts := map[string]int{}
	total := 0
	for qc := -25; qc <= 25; qc++ {
		for rc := -25; rc <= 25; rc++ {
			tp := scenario.MonsterTemper(seed, qc, rc)
			counts[tp]++
			total++
			if tp != scenario.MonsterTemper(seed, qc, rc) {
				t.Fatalf("MonsterTemper(%d,%d,%d) not reproducible", seed, qc, rc)
			}
		}
	}
	want := map[string]float64{
		scenario.TemperWanderer: 0.50,
		scenario.TemperSentinel: 0.30,
		scenario.TemperPredator: 0.20,
	}
	for tp, target := range want {
		got := float64(counts[tp]) / float64(total)
		if got < target-0.04 || got > target+0.04 {
			t.Errorf("temper %q share = %.3f, want %.2f±0.04 (%d/%d)", tp, got, target, counts[tp], total)
		}
		t.Logf("temper %-8s %5d/%d = %.1f%%（目标 %.0f%%）", tp, counts[tp], total, 100*got, 100*target)
	}
	if counts[scenario.TemperWanderer] <= counts[scenario.TemperSentinel] ||
		counts[scenario.TemperSentinel] <= counts[scenario.TemperPredator] {
		t.Errorf("distribution ordering broken: %v", counts)
	}
}

// 行为确定性：同 seed 两次对局（同激活序列）→ 事件流逐字节一致、
// 怪物轨迹与 mind 状态逐 tick 一致 (world_genesis §5.2/§7)。
func TestMonsterBehaviourDeterminism(t *testing.T) {
	run := func() (string, string) {
		seed := uint64(20260917)
		pack := scenario.V04(seed)
		m := NewV04("det", seed, nil)
		m.Pilot = agent.NewScripted(pack.PlayerSpawn, pack.FragmentPos, pack.Passable) // 隔离环境 LLM
		var log, trace strings.Builder
		for m.Status == StatusRunning && m.W.Tick < 15 {
			m.TickAuto()
			fmt.Fprintf(&log, "[%d]", m.W.Tick)
			for _, ev := range m.lastEvs {
				fmt.Fprintf(&log, "%s#%d:%s|", ev.Type, ev.Entity, ev.Text)
			}
			for _, id := range m.MonsterIDs {
				e := m.W.Entities[id]
				mind := m.minds[id]
				fmt.Fprintf(&trace, "t%d #%d %v hot=%v lost=%d temper=%s;",
					m.W.Tick, id, e.Pos, mind.hot, mind.lostTicks, mind.temper)
			}
			fmt.Fprintln(&trace)
		}
		return log.String(), trace.String()
	}
	logA, traceA := run()
	logB, traceB := run()
	if logA != logB {
		t.Fatalf("event log replay mismatch:\n--- A ---\n%s\n--- B ---\n%s", logA, logB)
	}
	if traceA != traceB {
		t.Fatalf("monster trace replay mismatch:\n--- A ---\n%s\n--- B ---\n%s", traceA, traceB)
	}
}

// 懒激活怪物登记行为模板：本 tick 现身的妖物在 minds 有条目，模板为
// 合法 hash 分配值、锚点即现身格（现身 tick 怪物尚未行动）。
func TestLazyMonstersGetTemper(t *testing.T) {
	m := NewV04("lazy", 20260917, nil)
	m.Tick(nil) // 首 tick：出生 3×3 chunk 懒激活
	valid := map[string]bool{
		scenario.TemperWanderer: true, scenario.TemperSentinel: true, scenario.TemperPredator: true,
	}
	spawned := 0
	for _, ev := range m.lastEvs {
		if ev.Type != "spawn" || !strings.Contains(ev.Text, "妖物") {
			continue
		}
		e := m.W.Entities[ev.Entity]
		if e == nil {
			t.Fatalf("spawn event for missing entity %d", ev.Entity)
		}
		mind := mindOf(m, ev.Entity)
		if mind == nil {
			t.Fatalf("lazy monster %d spawned but has no mind", ev.Entity)
		}
		if !valid[mind.temper] {
			t.Fatalf("lazy monster %d temper %q invalid", ev.Entity, mind.temper)
		}
		if mind.anchor != e.Pos {
			t.Fatalf("lazy monster %d anchor %v != spawn pos %v", ev.Entity, mind.anchor, e.Pos)
		}
		spawned++
	}
	if spawned == 0 {
		t.Fatalf("no lazy monster spawned this tick")
	}
	t.Logf("tick %d：本 tick 懒激活妖物 %d 只，均按 hash 登记模板", m.W.Tick, spawned)
}
