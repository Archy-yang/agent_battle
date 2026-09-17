package scenario

import "agentbattle/backend/engine"

// Pack is a Scenario Pack — the complete definition of one match
// (docs/world_genesis.md §2). Packs are constructed in code; JSON
// loading and the 三道关 validation pipeline come later.
//
// v0.4 起世界为无限六边形网格：Pack 不再持有 W/H/方形地形表，改为持有
// 地形纯函数 TerrainFn（F(seed, q, r) → Cell，design §2.2）。
type Pack struct {
	Name        string
	Seed        uint64
	TurnLimit   int64
	TerrainFn   engine.TerrainFunc
	PlayerSpawn engine.Hex
	FragmentPos engine.Hex
	Materials   map[engine.Essence]int
	// UnknownSpawn is where the Unknown Agent materializes. The zero
	// value falls back to the mvp-0.1 position.
	UnknownSpawn engine.Hex
	// MonsterSpawns lists the hand-placed monsters (world_genesis §5.1).
	// Empty for mvp-0.1.
	MonsterSpawns []MonsterSpec
}

// MonsterSpec is one hand-placed monster of a pack.
type MonsterSpec struct {
	Essence engine.Essence
	Form    int // 形 60..80
	Pos     engine.Hex
}

// Terrain returns the cell at h via the pack's terrain pure function.
func (p *Pack) Terrain(h engine.Hex) engine.Cell {
	return p.terrainFn()(p.Seed, h.Q, h.R)
}

// Passable exposes terrain knowledge for planners (旧地图先验,
// world_genesis §3.3).
func (p *Pack) Passable(h engine.Hex) bool { return p.Terrain(h).Passable }

func (p *Pack) terrainFn() engine.TerrainFunc {
	if p.TerrainFn == nil {
		return engine.PlainTerrain
	}
	return p.TerrainFn
}

// hexXY maps axial (q, r) onto the integer grid (X, Y) = (2q+r, r)
// (design §2.2 的线性变换): 邻格的 (X,Y) 距离近似恒定，v0.1/v0.2 的方形
// 地带谓词在 (X,Y) 上原样可用——这是废弃方形包改动最小的直译路径.
func hexXY(h engine.Hex) (int, int) { return 2*h.Q + h.R, h.R }

// MVP is 场景包 #1 (world_genesis §8), hex 直译版（Deprecated）：原
// 12×12 方形地形经 (X,Y)=(2q+r,r) 线性映射铺到无限 hex 网格上。v0.4 起
// 仅作行为回归基准；新内容请用 V04。
func MVP(seed uint64) *Pack {
	return &Pack{
		Name:      "scenario/mvp-0.1",
		Seed:      seed,
		TurnLimit: 30,
		TerrainFn: func(_ uint64, q, r int) engine.Cell {
			x, y := 2*q+r, r
			// 中央危险区：火阳过盛（环境损耗的来源）
			if x >= 3 && x <= 7 && y >= 3 && y <= 7 {
				return engine.Cell{Essence: engine.Fire, Danger: true, Passable: true}
			}
			// 北区遗迹：金阴（寒冷蚀能、硬物回声强）
			if y <= 1 {
				return engine.Cell{Essence: engine.Metal, Passable: true}
			}
			// 西缘水泽：水阴（利水行隐匿）
			if x == 0 {
				return engine.Cell{Essence: engine.Water, Passable: true}
			}
			return engine.Cell{Essence: engine.Earth, Passable: true} // 外环土性
		},
		PlayerSpawn:  engine.Hex{Q: 0, R: 10},  // (X,Y)=(10,10)：外环土性
		FragmentPos:  engine.Hex{Q: 0, R: 5},   // (X,Y)=(5,5)：危险区深处，出生距 5
		UnknownSpawn: engine.Hex{Q: 4, R: 3},   // (X,Y)=(11,3)：东部
		Materials: map[engine.Essence]int{
			engine.Earth: 2, engine.Wood: 2, engine.Metal: 1, engine.Water: 1,
		},
	}
}

// v0.2 地形生成参数（经 hexXY 映射沿用 world_genesis §3.1 五行原则）:
const (
	v02TurnLimit = 60
	// 中央火阳过盛区：X,Y ∈ 9..14
	v02DangerLo, v02DangerHi = 9, 14
	// 北区金阴带：Y ≤ 3
	v02NorthBand = 3
	// 西缘水泽带：X ≤ 2
	v02WestBand       = 2
	v02MonsterBattery = 60
	v02MaxRetries     = 10 // 障碍群导致不连通时的重生成上限
	// 关键格（出生 / Fragment / Unknown）周围不放障碍的半径
	v02KeyClearance = 3
)

// V02 is 场景包 #2, hex 直译版（Deprecated）：无限网格上的中部战场。
// 原 24×24 方形边界取消；障碍群（顽石）仍由 pack Rng 散布（hex 六方向
// 蔓延，关键格 HexDist ≤ 3 保护区豁免），spawn→Fragment 走廊 hex-BFS
// 验证，布局被隔断时重生成（≤10 重试）。Same seed → same map, byte for
// byte (world_genesis §7).
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
	spawn := engine.Hex{Q: -3, R: 21}  // (X,Y)=(15,21)：南带土性
	frag := engine.Hex{Q: 0, R: 12}    // (X,Y)=(12,12)：危险区深处，出生距 12
	unknown := engine.Hex{Q: 10, R: 2} // (X,Y)=(22,2)：北区金阴
	key := []engine.Hex{spawn, frag, unknown}
	protected := func(h engine.Hex) bool {
		for _, k := range key {
			if engine.HexDist(h, k) <= v02KeyClearance {
				return true
			}
		}
		return false
	}

	obstacles := map[engine.Hex]bool{}
	if withObstacles {
		// 4~6 个不可通行障碍群：每群 2~4 格，从 Rng 选取的中带锚点
		// 开始沿 hex 六方向蔓延（关键格保护区豁免）。
		clusters := 4 + rng.Intn(3)
		for i := 0; i < clusters; i++ {
			cur := engine.Hex{Q: -3 + rng.Intn(9), R: 4 + rng.Intn(16)} // q∈-3..5, r∈4..19
			size := 2 + rng.Intn(3)
			for j := 0; j < size; j++ {
				if !protected(cur) {
					obstacles[cur] = true
				}
				cur = cur.Add(engine.HexDirs[rng.Intn(6)])
			}
		}
	}

	terr := func(_ uint64, q, r int) engine.Cell {
		h := engine.Hex{Q: q, R: r}
		if obstacles[h] {
			return engine.Cell{Essence: engine.Earth, Passable: false} // 顽石
		}
		x, y := 2*q+r, r
		if x >= v02DangerLo && x <= v02DangerHi && y >= v02DangerLo && y <= v02DangerHi {
			return engine.Cell{Essence: engine.Fire, Danger: true, Passable: true}
		}
		if y <= v02NorthBand {
			return engine.Cell{Essence: engine.Metal, Passable: true}
		}
		if x <= v02WestBand {
			return engine.Cell{Essence: engine.Water, Passable: true}
		}
		return engine.Cell{Essence: engine.Earth, Passable: true} // 外环土性
	}

	p := &Pack{
		Name: "scenario/v02-0.2", Seed: seed,
		TurnLimit: v02TurnLimit, TerrainFn: terr,
		PlayerSpawn: spawn, FragmentPos: frag, UnknownSpawn: unknown,
		Materials: map[engine.Essence]int{
			engine.Earth: 3, engine.Wood: 3, engine.Metal: 2, engine.Water: 2,
		},
	}

	if !withObstacles {
		// 固定出生点（中带，且不在危险区内）——无障碍必然连通。
		p.MonsterSpawns = []MonsterSpec{
			{Essence: engine.Fire, Form: 80, Pos: engine.Hex{Q: 0, R: 8}},
			{Essence: engine.Metal, Form: 60, Pos: engine.Hex{Q: 3, R: 9}},
			{Essence: engine.Earth, Form: 70, Pos: engine.Hex{Q: -2, R: 16}},
		}
		return p
	}

	// 怪物出生：由 Rng 在中带确定性选取（互不重叠、不占关键格保护区、
	// 必须可通行）。
	kinds := []struct {
		ess  engine.Essence
		form int
	}{
		{engine.Fire, 80}, {engine.Metal, 60}, {engine.Earth, 70},
	}
	taken := map[engine.Hex]bool{}
	for _, k := range kinds {
		found := false
		for t := 0; t < 200 && !found; t++ {
			pos := engine.Hex{Q: rng.Intn(9) - 3, R: 6 + rng.Intn(13)}
			if !p.Passable(pos) || taken[pos] || protected(pos) {
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
// and every monster/Unknown spawn on the static passable terrain.
// 无限图：early-exit + 访问上限兜底（与 engine.PathBudget 同思路）。
func v02Connected(p *Pack) bool {
	reach := func(to engine.Hex) bool {
		if p.PlayerSpawn == to {
			return true
		}
		seen := map[engine.Hex]bool{p.PlayerSpawn: true}
		queue := []engine.Hex{p.PlayerSpawn}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			for _, d := range engine.HexDirs {
				nxt := cur.Add(d)
				if seen[nxt] || !p.Passable(nxt) {
					continue
				}
				seen[nxt] = true
				if nxt == to {
					return true
				}
				if len(seen) >= engine.PathBudget {
					return false
				}
				queue = append(queue, nxt)
			}
		}
		return false
	}
	if !reach(p.FragmentPos) || !reach(p.UnknownSpawn) {
		return false
	}
	for _, ms := range p.MonsterSpawns {
		if !reach(ms.Pos) {
			return false
		}
	}
	return true
}

// V04 is 场景包 #3：scenario/infinite-0.4，v0.4 无限世界最小包
// (design §2.4)。出生 (0,0)；Fragment (8,-4)（出生 HexDist = 8）；Unknown
// 与三只 Tier-2 怪物沿出生环（HexDist 8）六向分布；物料库沿用 v0.2 配比。
// WP1 地形为无限平原；WP2 接入逐块五行生成后本包结构不变。
func V04(seed uint64) *Pack {
	return &Pack{
		Name:      "scenario/infinite-0.4",
		Seed:      seed,
		TurnLimit: 60,
		// TerrainFn nil → engine.PlainTerrain：无限平原，皆土可通行.
		PlayerSpawn:  engine.Hex{Q: 0, R: 0},
		FragmentPos:  engine.Hex{Q: 8, R: -4},
		UnknownSpawn: engine.Hex{Q: -8, R: 0}, // 西位：与 Fragment 相背
		MonsterSpawns: []MonsterSpec{
			{Essence: engine.Fire, Form: 80, Pos: engine.Hex{Q: 0, R: 8}},   // 东南位
			{Essence: engine.Metal, Form: 60, Pos: engine.Hex{Q: 0, R: -8}}, // 西北位
			{Essence: engine.Earth, Form: 70, Pos: engine.Hex{Q: -8, R: 8}}, // 西南位
		},
		Materials: map[engine.Essence]int{
			engine.Earth: 3, engine.Wood: 3, engine.Metal: 2, engine.Water: 2,
		},
	}
}

// Genesis instantiates the world from the pack (world_genesis §3):
// it returns the world plus the ids of player, unknown agent, fragment
// and any hand-placed monsters (empty when the pack has none).
func (p *Pack) Genesis() (*engine.World, engine.EntityID, engine.EntityID, engine.EntityID, []engine.EntityID) {
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

// newWorld fills the infinite terrain and the player 物料库 from the pack.
func (p *Pack) newWorld() *engine.World {
	w := engine.NewWorldTerrain(p.Seed, p.terrainFn())
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
// (world_genesis §8) — 金甲 + 火行离火.
func (p *Pack) spawnUnknown(w *engine.World) *engine.Entity {
	pos := p.UnknownSpawn
	if pos == (engine.Hex{}) {
		pos = engine.Hex{Q: 4, R: 3} // mvp-0.1 缺省位
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
