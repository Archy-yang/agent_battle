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
		PlayerSpawn: engine.Vec2{X: 1, Y: 10},
		FragmentPos: engine.Vec2{X: 5, Y: 5},
		Materials: map[engine.Essence]int{
			engine.Earth: 2, engine.Wood: 2, engine.Metal: 1, engine.Water: 1,
		},
	}
}

// Genesis instantiates the world from the pack (world_genesis §3):
// it returns the world plus the ids of player, unknown agent and fragment.
func (p *Pack) Genesis() (*engine.World, engine.EntityID, engine.EntityID, engine.EntityID) {
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

	// 玩家 Agent：木架（韧）——选材质即选天敌（world_laws §8）
	player := &engine.Entity{
		Pos: p.PlayerSpawn, Kind: "agent", Owner: "player", Essence: engine.Wood,
		Integrity: 100, MaxIntegrity: 100, Battery: 100, MaxBattery: 100,
	}
	player.AddComponent(engine.CFrame, engine.Wood)
	player.AddComponent(engine.CBattery, engine.Earth)
	player.AddComponent(engine.CMotor, engine.Wood)
	player.AddComponent(engine.CSensor, engine.Water)
	player.AddComponent(engine.CProcessor, engine.Earth)
	w.Spawn(player)

	// Unknown Agent：虚张声势的采集者（world_genesis §8）
	// 金甲 + 火行离火 → 火迹明显，但实为低威胁；常驻外环
	unknown := &engine.Entity{
		Pos: engine.Vec2{X: 11, Y: 2}, Kind: "agent", Owner: "unknown", Essence: engine.Metal,
		Integrity: 60, MaxIntegrity: 60, Battery: 60, MaxBattery: 60, Heat: 8,
	}
	unknown.AddComponent(engine.CFrame, engine.Metal)
	unknown.AddComponent(engine.CBattery, engine.Earth)
	unknown.AddComponent(engine.CMotor, engine.Wood)
	unknown.AddComponent(engine.CEmitter, engine.Fire)
	unknown.AddComponent(engine.CSensor, engine.Water)
	w.Spawn(unknown)

	// Fragment：先天一炁之凝（world_laws §8：冲气以为和，故属土）
	frag := &engine.Entity{
		Pos: p.FragmentPos, Kind: "fragment", Owner: "world", Essence: engine.Earth,
		Integrity: 1, MaxIntegrity: 1,
	}
	w.Spawn(frag)

	return w, player.ID, unknown.ID, frag.ID
}
