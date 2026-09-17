package scenario

import "agentbattle/backend/engine"

// Pack is a Scenario Pack — the complete definition of one match
// (docs/world_genesis.md §2). v0.1 packs are constructed in code;
// JSON loading and the 三道关 validation pipeline come later.
type Pack struct {
	Name        string
	Seed        uint64
	W, H        int
	TurnLimit   int64
	Terrain     func(p engine.Vec2) engine.Cell
	PlayerSpawn engine.Vec2
	FragmentPos engine.Vec2
	Materials   map[engine.Essence]int
	// UnknownSpawn is where the Unknown Agent materializes. The zero
	// value falls back to the mvp-0.1 position.
	UnknownSpawn engine.Vec2
	// MonsterSpawns lists the Tier-0 hand-placed monsters
	// (world_genesis §5.1). Empty for mvp-0.1.
	MonsterSpawns []MonsterSpec
}

// MonsterSpec is one hand-placed monster of a v0.2 pack.
type MonsterSpec struct {
	Essence engine.Essence
	Form    int // 形 60..80
	Pos     engine.Vec2
}

// Passable exposes terrain knowledge for planners (旧地图先验,
// world_genesis §3.3).
func (p *Pack) Passable(v engine.Vec2) bool {
	return v.X >= 0 && v.Y >= 0 && v.X < p.W && v.Y < p.H && p.Terrain(v).Passable
}

// MVP is 场景包 #1 (world_genesis §8): 12×12, Fragment quest,
// one Unknown Agent, 火阳过盛 central zone, 金阴 north ruins.
func MVP(seed uint64) *Pack {
	return &Pack{
		Name:      "scenario/mvp-0.1",
		Seed:      seed,
		W:         12,
		H:         12,
		TurnLimit: 30,
		Terrain: func(p engine.Vec2) engine.Cell {
			// 中央危险区：火阳过盛（环境损耗的来源）
			if p.X >= 3 && p.X <= 7 && p.Y >= 3 && p.Y <= 7 {
				return engine.Cell{Essence: engine.Fire, Danger: true, Passable: true}
			}
			// 北区遗迹：金阴（寒冷蚀能、硬物回声强）
			if p.Y <= 1 {
				return engine.Cell{Essence: engine.Metal, Passable: true}
			}
			// 西缘水泽：水阴（利水行隐匿）
			if p.X == 0 {
				return engine.Cell{Essence: engine.Water, Passable: true}
			}
			return engine.Cell{Essence: engine.Earth, Passable: true} // 外环土性
		},
		PlayerSpawn:  engine.Vec2{X: 1, Y: 10},
		FragmentPos:  engine.Vec2{X: 5, Y: 5},
		UnknownSpawn: engine.Vec2{X: 11, Y: 2},
		Materials: map[engine.Essence]int{
			engine.Earth: 2, engine.Wood: 2, engine.Metal: 1, engine.Water: 1,
		},
	}
}

// v0.2 地形生成参数 (world_genesis §3.1 五行原则, 放大版):
const (
	v02W, v02H   = 24, 24
	v02TurnLimit = 60
	// 中央火阳过盛区：x,y ∈ 9..14
	v02DangerLo, v02DangerHi = 9, 14
	// 北区金阴带：y ≤ 3
	v02NorthBand = 3
	// 西缘水泽带：x ≤ 2
	v02WestBand       = 2
	v02MonsterBattery = 60
	v02MaxRetries     = 10 // 障碍群导致不连通时的重生成上限
)

// V02 is 场景包 #2: 24×24, TurnLimit 60, one Unknown Agent on the
// outer ring plus three hand-placed monsters (火/金/土), and the
// Fragment deep inside the central 火阳过盛 zone. Obstacle clusters
// (顽石) are scattered by the pack Rng; the spawn→Fragment corridor is
// BFS-verified and the layout regenerated (≤10 retries) when a cluster
// walls it off. Same seed → same map, byte for byte (world_genesis §7).
func V02(seed uint64) *Pack {
	rng := engine.NewRng(seed)
	for attempt := 0; attempt <= v02MaxRetries; attempt++ { // 初次生成 + 重试
		if p := buildV02(seed, rng, true); p != nil {
			return p
		}
	}
	return buildV02(seed, rng, false) // 兜底：无顽石，必然连通
}

// buildV02 assembles one candidate layout. withObstacles=false skips
// the 顽石 pass entirely (the guaranteed-connected fallback); obstacles
// are drawn from rng so retries consume successive Rng states — the
// whole chain stays a pure function of the seed.
func buildV02(seed uint64, rng *engine.Rng, withObstacles bool) *Pack {
	spawn := engine.Vec2{X: 1, Y: v02H - 2}        // 西南角
	frag := engine.Vec2{X: 12, Y: 12}              // 危险区深处
	unknownSpawn := engine.Vec2{X: v02W - 2, Y: 2} // 外环东北角
	protected := map[engine.Vec2]bool{spawn: true, frag: true, unknownSpawn: true}

	obstacles := map[engine.Vec2]bool{}
	if withObstacles {
		// 4~6 个不可通行障碍群：每群 2~4 格，从 Rng 选取的中带锚点
		// 开始沿 Rng 方向蔓延（只落在中带，出生角与外环保持开阔）。
		dirs := [][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}
		clusters := 4 + rng.Intn(3)
		for i := 0; i < clusters; i++ {
			cur := engine.Vec2{X: 4 + rng.Intn(16), Y: 4 + rng.Intn(16)} // 4..19
			size := 2 + rng.Intn(3)
			for j := 0; j < size; j++ {
				if cur.X >= 1 && cur.X <= v02W-2 && cur.Y >= 1 && cur.Y <= v02H-2 && !protected[cur] {
					obstacles[cur] = true
				}
				d := dirs[rng.Intn(4)]
				cur = engine.Vec2{X: cur.X + d[0], Y: cur.Y + d[1]}
			}
		}
	}

	terr := func(pos engine.Vec2) engine.Cell {
		if obstacles[pos] {
			return engine.Cell{Essence: engine.Earth, Passable: false} // 顽石
		}
		if pos.X >= v02DangerLo && pos.X <= v02DangerHi && pos.Y >= v02DangerLo && pos.Y <= v02DangerHi {
			return engine.Cell{Essence: engine.Fire, Danger: true, Passable: true}
		}
		if pos.Y <= v02NorthBand {
			return engine.Cell{Essence: engine.Metal, Passable: true}
		}
		if pos.X <= v02WestBand {
			return engine.Cell{Essence: engine.Water, Passable: true}
		}
		return engine.Cell{Essence: engine.Earth, Passable: true} // 外环土性
	}

	p := &Pack{
		Name: "scenario/v02-0.2", Seed: seed, W: v02W, H: v02H,
		TurnLimit: v02TurnLimit, Terrain: terr,
		PlayerSpawn: spawn, FragmentPos: frag, UnknownSpawn: unknownSpawn,
		Materials: map[engine.Essence]int{
			engine.Earth: 3, engine.Wood: 3, engine.Metal: 2, engine.Water: 2,
		},
	}

	if !withObstacles {
		// 固定出生点（中带，且不在危险区内）——无障碍必然连通。
		p.MonsterSpawns = []MonsterSpec{
			{Essence: engine.Fire, Form: 80, Pos: engine.Vec2{X: 8, Y: 8}},
			{Essence: engine.Metal, Form: 60, Pos: engine.Vec2{X: 15, Y: 8}},
			{Essence: engine.Earth, Form: 70, Pos: engine.Vec2{X: 12, Y: 16}},
		}
		return p
	}

	// 怪物出生：由 Rng 在中带 6..18 确定性选取（互不重叠、不占
	// 关键格、必须可通行）。
	kinds := []struct {
		ess  engine.Essence
		form int
	}{
		{engine.Fire, 80}, {engine.Metal, 60}, {engine.Earth, 70},
	}
	taken := map[engine.Vec2]bool{}
	for _, k := range kinds {
		found := false
		for t := 0; t < 200 && !found; t++ {
			pos := engine.Vec2{X: 6 + rng.Intn(13), Y: 6 + rng.Intn(13)}
			if !p.Passable(pos) || taken[pos] || protected[pos] {
				continue
			}
			p.MonsterSpawns = append(p.MonsterSpawns, MonsterSpec{Essence: k.ess, Form: k.form, Pos: pos})
			taken[pos] = true
			found = true
		}
		if !found {
			return nil
		}
	}

	if !v02Connected(p) { // 出生点 → Fragment 无通路：重生成
		return nil
	}
	return p
}

// v02Connected BFS-verifies that the player spawn reaches the Fragment
// and every monster/Unknown spawn on the static passable grid.
func v02Connected(p *Pack) bool {
	seen := map[engine.Vec2]bool{p.PlayerSpawn: true}
	queue := []engine.Vec2{p.PlayerSpawn}
	dirs := [][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, d := range dirs {
			nxt := engine.Vec2{X: cur.X + d[0], Y: cur.Y + d[1]}
			if seen[nxt] || !p.Passable(nxt) {
				continue
			}
			seen[nxt] = true
			queue = append(queue, nxt)
		}
	}
	if !seen[p.FragmentPos] || !seen[p.UnknownSpawn] {
		return false
	}
	for _, ms := range p.MonsterSpawns {
		if !seen[ms.Pos] {
			return false
		}
	}
	return true
}

// Genesis instantiates the world from the pack (world_genesis §3):
// it returns the world plus the ids of player, unknown agent and fragment.
func (p *Pack) Genesis() (*engine.World, engine.EntityID, engine.EntityID, engine.EntityID) {
	w := p.newWorld()
	player := p.spawnPlayer(w)
	unknown := p.spawnUnknown(w)
	frag := p.spawnFragment(w)
	return w, player.ID, unknown.ID, frag.ID
}

// GenesisV02 instantiates the v0.2 world: player, Unknown Agent, the
// three hand-placed monsters and the Fragment. It returns the monster
// ids additionally (world_genesis §5.1 Tier 0 手摆实体).
func (p *Pack) GenesisV02() (*engine.World, engine.EntityID, engine.EntityID, engine.EntityID, []engine.EntityID) {
	w := p.newWorld()
	player := p.spawnPlayer(w)
	unknown := p.spawnUnknown(w)
	frag := p.spawnFragment(w)
	var monsters []engine.EntityID
	for _, ms := range p.MonsterSpawns {
		mon := &engine.Entity{
			Pos: ms.Pos, Kind: "monster", Owner: "monster", Essence: ms.Essence,
			Integrity: ms.Form, MaxIntegrity: ms.Form,
			Battery: v02MonsterBattery, MaxBattery: v02MonsterBattery,
		}
		mon.AddComponent(engine.CFrame, ms.Essence)     // 躯壳
		mon.AddComponent(engine.CBattery, engine.Earth) // 储气
		mon.AddComponent(engine.CMotor, engine.Wood)    // 灵机
		mon.AddComponent(engine.CEmitter, engine.Fire)  // 离火
		w.Spawn(mon)
		monsters = append(monsters, mon.ID)
	}
	return w, player.ID, unknown.ID, frag.ID, monsters
}

// newWorld fills the grid and the player 物料库 from the pack.
func (p *Pack) newWorld() *engine.World {
	w := engine.NewWorld(p.W, p.H, p.Seed)
	for y := 0; y < p.H; y++ {
		for x := 0; x < p.W; x++ {
			w.Cells[y*p.W+x] = p.Terrain(engine.Vec2{X: x, Y: y})
		}
	}
	inv := map[engine.Essence]int{}
	for k, v := range p.Materials {
		inv[k] = v
	}
	w.Materials["player"] = inv
	return w
}

// spawnPlayer builds the player Agent: 木架（韧）——选材质即选天敌
// (world_laws §8).
func (p *Pack) spawnPlayer(w *engine.World) *engine.Entity {
	player := &engine.Entity{
		Pos: p.PlayerSpawn, Kind: "agent", Owner: "player", Essence: engine.Wood,
		Integrity: 100, MaxIntegrity: 100, Battery: 100, MaxBattery: 100,
	}
	player.AddComponent(engine.CFrame, engine.Wood)
	player.AddComponent(engine.CBattery, engine.Earth)
	player.AddComponent(engine.CMotor, engine.Wood)
	player.AddComponent(engine.CSensor, engine.Water)
	player.AddComponent(engine.CProcessor, engine.Earth)
	return w.Spawn(player)
}

// spawnUnknown builds the Unknown Agent: 虚张声势的采集者
// (world_genesis §8) — 金甲 + 火行离火，常驻外环.
func (p *Pack) spawnUnknown(w *engine.World) *engine.Entity {
	pos := p.UnknownSpawn
	if pos == (engine.Vec2{}) {
		pos = engine.Vec2{X: 11, Y: 2}
	}
	unknown := &engine.Entity{
		Pos: pos, Kind: "agent", Owner: "unknown", Essence: engine.Metal,
		Integrity: 60, MaxIntegrity: 60, Battery: 60, MaxBattery: 60, Heat: 8,
	}
	unknown.AddComponent(engine.CFrame, engine.Metal)
	unknown.AddComponent(engine.CBattery, engine.Earth)
	unknown.AddComponent(engine.CMotor, engine.Wood)
	unknown.AddComponent(engine.CEmitter, engine.Fire)
	unknown.AddComponent(engine.CSensor, engine.Water)
	return w.Spawn(unknown)
}

// spawnFragment builds the 先天一炁之凝 (world_laws §8：冲气以为和，故属土).
func (p *Pack) spawnFragment(w *engine.World) *engine.Entity {
	frag := &engine.Entity{
		Pos: p.FragmentPos, Kind: "fragment", Owner: "world", Essence: engine.Earth,
		Integrity: 1, MaxIntegrity: 1,
	}
	return w.Spawn(frag)
}
