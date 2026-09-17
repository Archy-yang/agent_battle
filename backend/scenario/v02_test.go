package scenario

import (
	"testing"

	"agentbattle/backend/engine"
)

// bfsReachable 在静态可通行地形上验证 from → to 连通（测试自用）。
// 无限图：early-exit + 访问上限兜底。
func bfsReachable(p *Pack, from, to engine.Hex) bool {
	if from == to {
		return true
	}
	seen := map[engine.Hex]bool{from: true}
	queue := []engine.Hex{from}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, d := range engine.HexDirs {
			nxt := cur.Add(d)
			if seen[nxt] || !p.Passable(nxt) {
				continue
			}
			if nxt == to {
				return true
			}
			seen[nxt] = true
			if len(seen) >= engine.PathBudget {
				return false
			}
			queue = append(queue, nxt)
		}
	}
	return false
}

// v0.2 布局（hex 直译）：TurnLimit 60、出生点南带土性、Fragment 在危险
// 区深处（出生 HexDist 12）、三只五行各异的怪物落在中带。
func TestV02LayoutHex(t *testing.T) {
	p := V02(42)
	if p.TurnLimit != 60 {
		t.Fatalf("turn limit = %d", p.TurnLimit)
	}
	if p.PlayerSpawn != (engine.Hex{Q: -3, R: 21}) {
		t.Fatalf("player spawn = %v", p.PlayerSpawn)
	}
	if d := engine.HexDist(p.PlayerSpawn, p.FragmentPos); d != 9 {
		t.Fatalf("spawn→fragment hex distance = %d, want 9", d)
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
		x, y := hexXY(ms.Pos)
		// 锚点 q∈-3..5, r∈6..18 ⇒ (X,Y) ∈ [0,28]×[6,18]
		if x < 0 || x > 28 || y < 6 || y > 18 {
			t.Errorf("monster %s spawn %v (X,Y)=(%d,%d) off the mid band", ms.Essence, ms.Pos, x, y)
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
	// 五行原则（经 (X,Y)=(2q+r,r) 映射）：北区金阴、西缘水泽、中央火阳过盛
	if c := p.Terrain(engine.Hex{Q: 5, R: 2}); c.Essence != engine.Metal || !c.Passable { // (X,Y)=(12,2)
		t.Errorf("north band wrong: %+v", c)
	}
	if c := p.Terrain(engine.Hex{Q: -2, R: 6}); c.Essence != engine.Water || !c.Passable { // (X,Y)=(2,6)
		t.Errorf("west band wrong: %+v", c)
	}
	if c := p.Terrain(engine.Hex{Q: 0, R: 10}); !c.Danger || c.Essence != engine.Fire { // (X,Y)=(10,10)
		t.Errorf("danger zone wrong: %+v", c)
	}
}

// 出生点到 Fragment（以及各怪物、Unknown 出生点）hex-BFS 可达——多 seed
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

// 不可通行障碍群确实存在（4~6 群 × 每群 2~4 格），且不吞掉关键格——
// v0.4 起"关键格"扩大为出生/Fragment/Unknown 周围 HexDist ≤ 3 保护区。
func TestV02Obstacles(t *testing.T) {
	p := V02(42)
	key := []engine.Hex{p.PlayerSpawn, p.FragmentPos, p.UnknownSpawn}
	blocked := 0
	// 采样窗完全覆盖锚点蔓延范围（q∈-7..9, r∈0..23）
	for qc := -8; qc <= 10; qc++ {
		for rc := -1; rc <= 24; rc++ {
			h := engine.Hex{Q: qc, R: rc}
			if p.Passable(h) {
				continue
			}
			blocked++
			for _, k := range key {
				if engine.HexDist(h, k) <= 3 {
					t.Fatalf("obstacle inside the key-cell clearance at %v (dist %d)", h, engine.HexDist(h, k))
				}
			}
		}
	}
	// 障碍群可能少量重叠或落出采样窗，放宽下界；上界 = 6 群 × 4 格。
	if blocked < 4 || blocked > 6*4 {
		t.Fatalf("impassable cells in sampled window = %d", blocked)
	}
}

// 同 seed 两次生成：地形逐格一致、怪物出生点一致（world_genesis §7）。
func TestV02DeterministicGeneration(t *testing.T) {
	a, b := V02(20260917), V02(20260917)
	// 无限世界：方形全图遍历不存在，改为中带 hex 窗口逐一比对。
	for q := -6; q <= 8; q++ {
		for r := -2; r <= 24; r++ {
			h := engine.Hex{Q: q, R: r}
			if a.Terrain(h) != b.Terrain(h) {
				t.Fatalf("terrain mismatch at %v: %+v vs %+v", h, a.Terrain(h), b.Terrain(h))
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

// v0.4 无限世界包：出生 (0,0)、Fragment 由 seed 哈希落在 12 向 ×
// HexDist 14~18（WP2，design §2.4；2026-09-19 平衡修订，与蓄息经济
// 匹配）、Unknown 与三怪物沿出生环分布、TurnLimit 60、v0.4 五行地形。
func TestV04Layout(t *testing.T) {
	p := V04(42)
	if p.Name != "scenario/infinite-0.4" || p.TurnLimit != 60 {
		t.Fatalf("pack = %s limit = %d", p.Name, p.TurnLimit)
	}
	if p.PlayerSpawn != (engine.Hex{Q: 0, R: 0}) {
		t.Fatalf("player spawn = %v", p.PlayerSpawn)
	}
	if p.FragmentPos != FragmentFor(42) {
		t.Fatalf("fragment = %v, want FragmentFor(42)", p.FragmentPos)
	}
	if d := engine.HexDist(p.PlayerSpawn, p.FragmentPos); d < 14 || d > 18 {
		t.Fatalf("spawn→fragment hex distance = %d, want in [14,18]", d)
	}
	spawns := append([]MonsterSpec{{
		Essence: engine.Metal, Form: 0, Pos: p.UnknownSpawn,
	}}, p.MonsterSpawns...)
	if len(spawns) != 4 {
		t.Fatalf("unknown + monsters = %d, want 4", len(spawns))
	}
	essences := map[engine.Essence]bool{}
	for _, ms := range spawns {
		if d := engine.HexDist(p.PlayerSpawn, ms.Pos); d != 8 {
			t.Errorf("spawn %v not on the radius-8 ring (dist %d)", ms.Pos, d)
		}
		if !p.Passable(ms.Pos) {
			t.Errorf("spawn %v impassable", ms.Pos)
		}
		essences[ms.Essence] = true // Unknown 与金行怪同为金，去重后仍须三行齐备
	}
	for _, want := range []engine.Essence{engine.Fire, engine.Metal, engine.Earth} {
		if !essences[want] {
			t.Errorf("no monster of essence %s", want)
		}
	}
	for _, e := range []engine.Essence{engine.Earth, engine.Wood, engine.Metal, engine.Water} {
		if p.Materials[e] == 0 {
			t.Errorf("materials missing %s (v0.2 配比应沿用)", e)
		}
	}
}

// v0.4 全局可达：无限平原上出生点与 Fragment、怪物、Unknown 必然连通。
func TestV04SpawnToFragmentReachable(t *testing.T) {
	for _, seed := range []uint64{1, 42, 20260917} {
		p := V04(seed)
		for _, target := range append([]engine.Hex{p.FragmentPos, p.UnknownSpawn},
			func() (out []engine.Hex) {
				for _, ms := range p.MonsterSpawns {
					out = append(out, ms.Pos)
				}
				return out
			}()...) {
			if !bfsReachable(p, p.PlayerSpawn, target) {
				t.Fatalf("seed %d: %v unreachable from spawn", seed, target)
			}
		}
	}
}

// Genesis：怪物实体五行走铁律一（与玩家同一套组件与物理）。
func TestV02GenesisMonsters(t *testing.T) {
	p := V02(42)
	w, _, _, _, monsters := p.Genesis()
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
