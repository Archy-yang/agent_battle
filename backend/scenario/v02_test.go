package scenario

import (
	"testing"

	"agentbattle/backend/engine"
)

// bfsReachable 在静态可通行网格上验证 from → to 连通（测试自用）。
func bfsReachable(p *Pack, from, to engine.Vec2) bool {
	if from == to {
		return true
	}
	seen := map[engine.Vec2]bool{from: true}
	queue := []engine.Vec2{from}
	dirs := [][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, d := range dirs {
			nxt := engine.Vec2{X: cur.X + d[0], Y: cur.Y + d[1]}
			if seen[nxt] || !p.Passable(nxt) {
				continue
			}
			if nxt == to {
				return true
			}
			seen[nxt] = true
			queue = append(queue, nxt)
		}
	}
	return false
}

// v0.2 布局：24×24、TurnLimit 60、出生点西南角、Fragment 在危险区
// 深处、三只五行各异的怪物落在中带。
func TestV02Layout(t *testing.T) {
	p := V02(42)
	if p.W != 24 || p.H != 24 {
		t.Fatalf("map size = %dx%d", p.W, p.H)
	}
	if p.TurnLimit != 60 {
		t.Fatalf("turn limit = %d", p.TurnLimit)
	}
	if p.PlayerSpawn != (engine.Vec2{X: 1, Y: 22}) {
		t.Fatalf("player spawn = %v", p.PlayerSpawn)
	}
	cell := p.Terrain(p.FragmentPos)
	if !cell.Danger || cell.Essence != engine.Fire {
		t.Fatalf("fragment at %v not inside the 火阳过盛 zone: %+v", p.FragmentPos, cell)
	}
	if len(p.MonsterSpawns) != 3 {
		t.Fatalf("monster count = %d", len(p.MonsterSpawns))
	}
	essences := map[engine.Essence]bool{}
	for _, ms := range p.MonsterSpawns {
		if ms.Pos.X < 6 || ms.Pos.X > 18 || ms.Pos.Y < 6 || ms.Pos.Y > 18 {
			t.Errorf("monster %s spawn %v off the mid band", ms.Essence, ms.Pos)
		}
		if ms.Form < 60 || ms.Form > 80 {
			t.Errorf("monster %s form = %d", ms.Essence, ms.Form)
		}
		if !p.Passable(ms.Pos) {
			t.Errorf("monster %s spawn %v impassable", ms.Essence, ms.Pos)
		}
		essences[ms.Essence] = true
	}
	for _, want := range []engine.Essence{engine.Fire, engine.Metal, engine.Earth} {
		if !essences[want] {
			t.Errorf("no monster of essence %s", want)
		}
	}
	// 五行原则：北区金阴、西缘水泽、中央火阳过盛
	if c := p.Terrain(engine.Vec2{X: 12, Y: 1}); c.Essence != engine.Metal || !c.Passable {
		t.Errorf("north band wrong: %+v", c)
	}
	if c := p.Terrain(engine.Vec2{X: 0, Y: 12}); c.Essence != engine.Water || !c.Passable {
		t.Errorf("west band wrong: %+v", c)
	}
	if c := p.Terrain(engine.Vec2{X: 10, Y: 10}); !c.Danger || c.Essence != engine.Fire {
		t.Errorf("danger zone wrong: %+v", c)
	}
}

// 出生点到 Fragment（以及各怪物、Unknown 出生点）BFS 可达——多 seed
// 确定性验证。
func TestV02SpawnToFragmentReachable(t *testing.T) {
	for _, seed := range []uint64{1, 2, 3, 7, 42, 99, 20260917} {
		p := V02(seed)
		if !bfsReachable(p, p.PlayerSpawn, p.FragmentPos) {
			t.Fatalf("seed %d: Fragment unreachable from spawn", seed)
		}
		if !bfsReachable(p, p.PlayerSpawn, p.UnknownSpawn) {
			t.Fatalf("seed %d: Unknown spawn unreachable", seed)
		}
		for _, ms := range p.MonsterSpawns {
			if !bfsReachable(p, p.PlayerSpawn, ms.Pos) {
				t.Fatalf("seed %d: monster spawn %v unreachable", seed, ms.Pos)
			}
		}
	}
}

// 不可通行障碍群确实存在（4~6 群 × 每群 2~4 格），且不吞掉关键格。
func TestV02Obstacles(t *testing.T) {
	p := V02(42)
	blocked := 0
	for y := 0; y < p.H; y++ {
		for x := 0; x < p.W; x++ {
			v := engine.Vec2{X: x, Y: y}
			if p.Passable(v) {
				continue
			}
			blocked++
			if v == p.PlayerSpawn || v == p.FragmentPos || v == p.UnknownSpawn {
				t.Fatalf("obstacle swallowed key cell %v", v)
			}
		}
	}
	// 障碍群可能少量重叠，放宽下界；上界 = 6 群 × 4 格。
	if blocked < 4 || blocked > 6*4 {
		t.Fatalf("impassable cells = %d", blocked)
	}
}

// 同 seed 两次生成：地形逐格一致、怪物出生点一致（world_genesis §7）。
func TestV02DeterministicGeneration(t *testing.T) {
	a, b := V02(20260917), V02(20260917)
	for y := 0; y < a.H; y++ {
		for x := 0; x < a.W; x++ {
			v := engine.Vec2{X: x, Y: y}
			if a.Terrain(v) != b.Terrain(v) {
				t.Fatalf("terrain mismatch at %v: %+v vs %+v", v, a.Terrain(v), b.Terrain(v))
			}
		}
	}
	if len(a.MonsterSpawns) != len(b.MonsterSpawns) {
		t.Fatalf("monster count mismatch")
	}
	for i := range a.MonsterSpawns {
		if a.MonsterSpawns[i] != b.MonsterSpawns[i] {
			t.Fatalf("monster spawn %d mismatch: %+v vs %+v", i, a.MonsterSpawns[i], b.MonsterSpawns[i])
		}
	}
}

// GenesisV02：怪物实体五行走铁律一（与玩家同一套组件与物理）。
func TestV02GenesisMonsters(t *testing.T) {
	p := V02(42)
	w, _, _, _, monsters := p.GenesisV02()
	if len(monsters) != 3 {
		t.Fatalf("monster ids = %v", monsters)
	}
	forms := map[engine.Essence]int{}
	for _, id := range monsters {
		m := w.Entities[id]
		if m == nil {
			t.Fatalf("monster %d missing", id)
		}
		if m.Kind != "monster" || m.Owner != "monster" {
			t.Errorf("monster %d kind/owner = %s/%s", id, m.Kind, m.Owner)
		}
		if m.Integrity < 60 || m.Integrity > 80 || m.MaxIntegrity != m.Integrity {
			t.Errorf("monster %d form = %d/%d", id, m.Integrity, m.MaxIntegrity)
		}
		if m.Battery != 60 || m.MaxBattery != 60 {
			t.Errorf("monster %d battery = %d", id, m.Battery)
		}
		if m.FindComponent(engine.CFrame) == nil || m.FindComponent(engine.CBattery) == nil ||
			m.FindComponent(engine.CMotor) == nil || m.FindComponent(engine.CEmitter) == nil {
			t.Errorf("monster %d missing required components", id)
		}
		forms[m.Essence] = m.Integrity
	}
	for ess, form := range forms {
		if form < 60 || form > 80 {
			t.Errorf("monster %s form = %d", ess, form)
		}
	}
	for _, want := range []engine.Essence{engine.Fire, engine.Metal, engine.Earth} {
		if _, ok := forms[want]; !ok {
			t.Errorf("no monster of essence %s", want)
		}
	}
}
