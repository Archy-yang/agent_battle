package scenario

import (
	"fmt"
	"strconv"

	"agentbattle/backend/engine"
)

// v0.4 无限世界场景包 scenario/infinite-0.4 (design §2.4)。
//
// 地形：engine.Terrain 四步五行生成（design §2.2），经 TerrainFn 注入
// 世界；Fragment 落点格在纯函数内强制可通行（不改噪声）。
// Fragment：由 seed 哈希在 12 向 × 距离 20~28 内确定（FragmentFor）。
// 怪物：Tier-2 生成表——每 chunk 用 hash(seed,"mob",qc,rc) 决定是否出
// 怪，概率随 HexDist(出生 chunk, 该 chunk) 上升；essence 取该 chunk 主
// 导行，Form 由 hash 定。怪物在 chunk 首次参与激活圈时懒激活，激活事
// 件（"spawn"）进事件流（PendingSpawns）。
//
// 资源晶簇 (v0.5 WPA 经济循环)：每 chunk 用 hash(seed,"res",qc,rc) 决定
// 是否出晶簇——概率曲线与怪物同斜率、高 0.35 截距（v04ResChance），任
// 何深度采集都比遭遇更常见；essence 取该 chunk 主导行（"看地貌知资
// 源"）；出生 3×3 chunk 全部落空时 (0,0) 强制出一簇（开局保底，判定仍
// 是 seed 的纯函数）。晶簇与怪物走同一条懒激活通道（ChunkSpawn.Resource，
// 激活事件同为 "spawn"、文本区分）；玩家对同格晶簇发 transfer → match
// 层结算采集（"gather" 事件、物料入库）——晶簇不阻挡移动（engine
// EntityAt 对 Kind "resource" 透明）。
//
// 【match 接入点】match 持有"已激活 chunk 集合"（确定性状态，进重放
// 语义），每回合推进时调用
//
//	for _, cs := range scenario.PendingSpawns(m.W, player.Pos, m.activatedChunks) {
//	    m.activatedChunks[[2]int{cs.QC, cs.RC}] = true
//	    for _, ps := range []*scenario.PendingSpawn{cs.Spawn, cs.Resource} {
//	        if ps == nil {
//	            continue // 该 chunk 已覆盖但无此物
//	        }
//	        e := m.W.Spawn(ps.Entity)
//	        ev := ps.Event
//	        ev.Entity = e.ID // 实体 id 在 Spawn 后才存在
//	        evs = append(evs, ev)
//	    }
//	}
//
// 并把返回的事件并入该回合事件流。PendingSpawns 是纯函数：不修改
// activated，同一 (world 状态, 玩家位, activated) 重复调用结果逐位一致
// ——不持任何跨对局状态，World 被 GC 后地址复用也绝无激活状态串局
// （design §2.5 "纯函数 + 调用方持状态"）。

// V04TurnLimit is the v0.4 turn limit (design §2.4).
const V04TurnLimit = 60

// fragmentDirs12 返回 12 个"方向单位向量"：HexDist=2 的环上 12 格
// （六邻向的直进向与六角向），按 (dq, dr) 字典序固定排序。环上格的
// k 倍恒有 HexDist = 2k——这是"沿该方向走 2k 步"的构造性保证.
func fragmentDirs12() [12]engine.Hex {
	var out [12]engine.Hex
	i := 0
	for dq := -2; dq <= 2; dq++ {
		for dr := -2; dr <= 2; dr++ {
			h := engine.Hex{Q: dq, R: dr}
			if engine.HexDist(h, engine.Hex{}) == 2 {
				out[i] = h
				i++
			}
		}
	}
	return out
}

// FragmentFor deterministically places the Fragment from the seed:
// 12 向之一 × HexDist 20~28 (design §2.4). 奇数距离以"再向外一步"补足
// （取第一个使距离 +1 的 hex 邻向，HexDirs 序保证确定性）.
func FragmentFor(seed uint64) engine.Hex {
	ss := strconv.FormatUint(seed, 10)
	k := int(engine.Hash01(ss, "frag-dir", 0, 0)*12) % 12
	d := 20 + int(engine.Hash01(ss, "frag-dist", 0, 0)*9) // 20..28
	u := fragmentDirs12()[k]
	pos := engine.Hex{Q: u.Q * (d / 2), R: u.R * (d / 2)} // HexDist = 2*(d/2)
	if d%2 == 1 {
		origin := engine.Hex{}
		for _, dir := range engine.HexDirs {
			if engine.HexDist(pos.Add(dir), origin) == d {
				pos = pos.Add(dir)
				break
			}
		}
	}
	return pos
}

// v04Terrain returns the V04 terrain pure function: engine.Terrain 四步
// 生成 + Fragment 落点格强制可通行（design §2.4："落点格强制改写为可
// 通行（纯函数内完成，不改噪声）"）.
func v04Terrain(seed uint64, frag engine.Hex) engine.TerrainFunc {
	return func(_ uint64, q, r int) engine.Cell {
		c := engine.Terrain(seed, q, r)
		if q == frag.Q && r == frag.R {
			c.Passable = true
		}
		return c
	}
}

// V04 is 场景包 #3：scenario/infinite-0.4，v0.4 无限世界 (design §2.4)。
// 出生 (0,0)（3×3 出生 chunk 豁免顽石，engine 侧保证）；Fragment 由
// seed 哈希落在 HexDist 20~28；Unknown 在西位出生环上；三只 Tier-2 怪
// 物沿出生环（HexDist 8）分布——它们是"开局即在场上"的保底遭遇（继承
// WP1 结构，match_test 契约）；其余怪物由 chunk 懒激活按深度出怪
// （PendingSpawns）。
func V04(seed uint64) *Pack {
	frag := FragmentFor(seed)
	return &Pack{
		Name:         "scenario/infinite-0.4",
		Seed:         seed,
		TurnLimit:    V04TurnLimit,
		TerrainFn:    v04Terrain(seed, frag),
		PlayerSpawn:  engine.Hex{Q: 0, R: 0},
		FragmentPos:  frag,
		UnknownSpawn: engine.Hex{Q: -8, R: 0}, // 西位：与 Fragment 相背（出生 chunk 豁免区内，必可通行）
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

// —— 怪物懒激活 (design §2.4/§2.5) ——
//
// 原则：纯函数 + 调用方持状态。某 chunk 是否已激活由调用方（match 的
// activatedChunks，确定性重放状态）持有；本包只提供两个纯函数——
// MonsterSpawnsFor（(seed, chunk) → 怪物规格）与 PendingSpawns
// （激活圈覆盖计算）。不持任何跨对局状态：全局注册表以 *World 指针为
// 键，World 被 GC 后地址复用会让新对局继承旧激活状态（怪物不激活、
// 重放被破坏），该设计已于 WP5 评审后废除。

// MobSpec is one chunk's lazily-activated mob specification (不含落点——
// 落点依赖激活时刻的占位状态，由 PendingSpawns 在激活 tick 决定).
type MobSpec struct {
	Essence engine.Essence
	Form    int
}

// PendingSpawn is one lazily-activated chunk mob. match 负责把 Entity
// 交给 World.Spawn，并把 Event（回填 Entity 为新实体 id 后）并入该回合
// 的事件流——见文件头【match 接入点】.
type PendingSpawn struct {
	Entity *engine.Entity
	Event  engine.Event
}

// chunkCenter is the center cell of chunk (qc, rc)——深度以 chunk 中心
// 到出生 chunk 中心的 HexDist 计.
func chunkCenter(qc, rc int) engine.Hex {
	return engine.Hex{Q: qc*engine.ChunkSize + engine.ChunkSize/2, R: rc*engine.ChunkSize + engine.ChunkSize/2}
}

// chunkOf maps a cell to its chunk coordinates.
func chunkOf(h engine.Hex) (int, int) {
	return floorDiv12(h.Q), floorDiv12(h.R)
}

func floorDiv12(a int) int {
	q := a / engine.ChunkSize
	if (a%engine.ChunkSize != 0) && ((a < 0) != (engine.ChunkSize < 0)) {
		q--
	}
	return q
}

// v04MobChance is the per-chunk mob probability as a function of depth
// (HexDist from the birth chunk center)：p = min(0.60, 0.10 + 0.012·d)
// ——出生附近偶有遭遇，越深越凶.
func v04MobChance(depth int) float64 {
	p := 0.10 + 0.012*float64(depth)
	if p > 0.60 {
		p = 0.60
	}
	return p
}

// MonsterSpawnsFor returns the chunk's lazily-activated mob spec, or nil
// when the chunk rolls no mob：纯函数 (seed, qc, rc)——hash 自带 seed，
// 不消耗 match Rng，可重复调用、结果逐位一致 (design §2.5).
func MonsterSpawnsFor(seed uint64, qc, rc int) *MobSpec {
	ss := strconv.FormatUint(seed, 10)
	depth := engine.HexDist(chunkCenter(qc, rc), chunkCenter(0, 0))
	if engine.Hash01(ss, "mob", qc, rc) >= v04MobChance(depth) {
		return nil // 本 chunk 无怪
	}
	return &MobSpec{
		Essence: v04DominantEssence(seed, qc, rc), // essence 取该 chunk 主导行
		Form:    60 + int(engine.Hash01(ss, "mob-form", qc, rc)*21), // Form 由 hash 定：60..80
	}
}

// ChunkSpawn is one newly-covered chunk's lazy-activation result of a
// tick. Spawn == nil 表示该 chunk 覆盖到位但无怪——它同样必须记入调用
// 方的 activated 集合（"只激活一次"是对 chunk 而非对怪成立的）.
// Resource 同理装晶簇（v0.5 WPA）：Spawn 只装怪物、Resource 只装晶簇，
// 同 chunk 同 tick 可以两者皆有（晶簇落点避开同 tick 的怪物落点）.
type ChunkSpawn struct {
	QC, RC   int
	Spawn    *PendingSpawn
	Resource *PendingSpawn
}

// PendingSpawns walks the 3×3 activation circle around playerPos（与出
// 生豁免区同构）and returns every chunk not yet in activated，连同其怪
// 物与晶簇（若有，v0.5 WPA）。
//
// 纯函数：不修改 activated；同一 (world 状态, playerPos, activated)
// 重复调用结果逐位一致。跨对局零状态——world 指针不参与任何判定，
// 同 seed 同玩家路径 + 同 activated 演化 → 同一激活序列（重放安全，
// design §2.5）。幂等性由调用方把返回项的 chunk 键并入 activated 保证.
func PendingSpawns(w *engine.World, playerPos engine.Hex, activated map[[2]int]bool) []ChunkSpawn {
	frag := FragmentFor(w.Seed)
	pcq, pcr := chunkOf(playerPos)

	var out []ChunkSpawn
	for drc := -1; drc <= 1; drc++ { // 固定顺序 → 确定激活序列
		for dqc := -1; dqc <= 1; dqc++ {
			qc, rc := pcq+dqc, pcr+drc
			key := [2]int{qc, rc}
			if activated[key] {
				continue // 调用方已激活过该 chunk
			}
			cs := ChunkSpawn{QC: qc, RC: rc}
			mobPos := engine.Hex{}
			if ms := MonsterSpawnsFor(w.Seed, qc, rc); ms != nil {
				if pos, ok := v04MobCell(w, w.Seed, qc, rc, playerPos, frag); ok {
					mobPos = pos
					cs.Spawn = &PendingSpawn{
						Entity: v04BuildMob(pos, ms.Essence, ms.Form),
						Event: engine.Event{
							Tick: w.Tick,
							Type: "spawn",
							Text: fmt.Sprintf("懒激活：%s 行妖物现形于 (%d,%d)", ms.Essence, pos.Q, pos.R),
						},
					}
				}
			}
			if rs := ResourceSpawnsFor(w.Seed, qc, rc); rs != nil {
				if pos, ok := v04ResCell(w, w.Seed, qc, rc, playerPos, frag, mobPos); ok {
					cs.Resource = &PendingSpawn{
						Entity: v04BuildResource(pos, rs.Essence),
						Event: engine.Event{
							Tick: w.Tick,
							Type: "spawn",
							Text: fmt.Sprintf("懒激活：%s 行晶簇显化于 (%d,%d)", rs.Essence, pos.Q, pos.R),
						},
					}
				}
			}
			out = append(out, cs)
		}
	}
	return out
}

// v04DominantEssence returns the mode of the chunk's 144 cell essences
// （顽石格仍计入其 underlying essence）；并列取五行序最前者。纯函数：
// 直接由 engine.Terrain 计算（与 pack 地形缝的 essence 逐位一致），
// 不依赖任何 *World 实例.
func v04DominantEssence(seed uint64, qc, rc int) engine.Essence {
	var counts [5]int
	for j := 0; j < engine.ChunkSize; j++ {
		for i := 0; i < engine.ChunkSize; i++ {
			counts[engine.Terrain(seed, qc*engine.ChunkSize+i, rc*engine.ChunkSize+j).Essence]++
		}
	}
	best := engine.Essence(0)
	for e := engine.Essence(1); e <= engine.Water; e++ {
		if counts[e] > counts[best] {
			best = e
		}
	}
	return best
}

// v04MobCell picks the mob's cell: chunk 内可通行、非玩家位、非
// Fragment 位、当前无实体阻挡的格子中按 hash 选取（固定行枚举序 →
// 确定性）.
func v04MobCell(w *engine.World, seed uint64, qc, rc int, playerPos, frag engine.Hex) (engine.Hex, bool) {
	ss := strconv.FormatUint(seed, 10)
	var cand []engine.Hex
	for j := 0; j < engine.ChunkSize; j++ {
		for i := 0; i < engine.ChunkSize; i++ {
			p := engine.Hex{Q: qc*engine.ChunkSize + i, R: rc*engine.ChunkSize + j}
			if p == playerPos || p == frag || !w.At(p).Passable || w.EntityAt(p) != nil {
				continue
			}
			cand = append(cand, p)
		}
	}
	if len(cand) == 0 {
		return engine.Hex{}, false
	}
	idx := int(engine.Hash01(ss, "mob-pos", qc, rc) * float64(len(cand)))
	if idx >= len(cand) {
		idx = len(cand) - 1
	}
	return cand[idx], true
}

// v04BuildMob assembles one Tier-2 monster (world_genesis §5.1 装配,
// 与 Genesis 手置怪物同构：躯壳/储气/灵机/离火).
func v04BuildMob(pos engine.Hex, ess engine.Essence, form int) *engine.Entity {
	m := &engine.Entity{
		Pos: pos, Kind: "monster", Owner: "monster", Essence: ess,
		Integrity: form, MaxIntegrity: form,
		Battery: v02MonsterBattery, MaxBattery: v02MonsterBattery,
	}
	m.AddComponent(engine.CFrame, ess)            // 躯壳
	m.AddComponent(engine.CBattery, engine.Earth) // 储气
	m.AddComponent(engine.CMotor, engine.Wood)    // 灵机
	m.AddComponent(engine.CEmitter, engine.Fire)  // 离火
	return m
}

// —— 资源晶簇懒激活 (v0.5 WPA 经济循环：采集到物料的供给线) ——
//
// 与怪物同一套"纯函数 + 调用方持状态"模式：ResourceSpawnsFor 对应
// MonsterSpawnsFor（(seed, chunk) → 规格，hash 自带 seed、不消耗 match
// Rng、可重复调用），落点选取与 PendingSpawns 的激活圈行走完全复用。
// 晶簇规格只有 essence——Integrity 恒 1，采一次即空。

// ResourceSpec is one chunk's lazily-activated resource crystal spec
// （不含落点——与 MobSpec 同理，落点由 v04ResCell 在激活 tick 决定）.
type ResourceSpec struct {
	Essence engine.Essence
}

// chunkDepth is a chunk's depth: chunk 中心到出生 chunk 中心的 HexDist.
func chunkDepth(qc, rc int) int {
	return engine.HexDist(chunkCenter(qc, rc), chunkCenter(0, 0))
}

// v04ResChance is the per-chunk crystal probability as a function of
// depth：p = min(0.85, 0.45 + 0.012·d)——与怪物曲线（0.10 + 0.012·d，
// 上限 0.60）同斜率、高 0.35 截距：任何深度晶簇概率都高于出怪，采集
// 是常规事件而非遭遇.
func v04ResChance(depth int) float64 {
	p := 0.45 + 0.012*float64(depth)
	if p > 0.85 {
		p = 0.85
	}
	return p
}

// v04BirthResRolled reports whether any of the 9 birth chunks rolled a
// crystal by hash（只看原始掷点、不含保底强制项——递归自由）.
func v04BirthResRolled(seed uint64) bool {
	ss := strconv.FormatUint(seed, 10)
	for qc := -1; qc <= 1; qc++ {
		for rc := -1; rc <= 1; rc++ {
			if engine.Hash01(ss, "res", qc, rc) < v04ResChance(chunkDepth(qc, rc)) {
				return true
			}
		}
	}
	return false
}

// ResourceSpawnsFor returns the chunk's lazily-activated crystal spec,
// or nil when the chunk rolls none：纯函数 (seed, qc, rc)——hash 自带
// seed，不消耗 match Rng，可重复调用、结果逐位一致 (design §2.5).
// essence 取该 chunk 主导行（"看地貌知资源"，与怪物同规）。开局保底：
// 出生 3×3 chunk 原始掷点全落空时 (0,0) 强制出一簇——判定仍是 seed 的
// 纯函数（v04BirthResRolled 只重算 9 个 hash，无状态、跨调用一致）.
func ResourceSpawnsFor(seed uint64, qc, rc int) *ResourceSpec {
	ss := strconv.FormatUint(seed, 10)
	hit := engine.Hash01(ss, "res", qc, rc) < v04ResChance(chunkDepth(qc, rc))
	if !hit && qc == 0 && rc == 0 && !v04BirthResRolled(seed) {
		hit = true
	}
	if !hit {
		return nil
	}
	return &ResourceSpec{Essence: v04DominantEssence(seed, qc, rc)}
}

// v04CellTaken reports whether any world entity stands at p. 晶簇不阻挡
// 移动、World.EntityAt 对 Kind "resource" 透明，故占位判定直接扫实体表
// （map 遍历只做存在性判定，与序无关，确定性不受影响）.
func v04CellTaken(w *engine.World, p engine.Hex) bool {
	for _, e := range w.Entities {
		if e.Pos == p {
			return true
		}
	}
	return false
}

// v04ResCell picks the crystal's cell: chunk 内可通行、非玩家位、非
// Fragment 位、无任何实体占据、不与 avoid（同 chunk 同 tick 的怪物落
// 点）重叠的格子中按 hash 选取（固定行枚举序 → 确定性）.
func v04ResCell(w *engine.World, seed uint64, qc, rc int, playerPos, frag, avoid engine.Hex) (engine.Hex, bool) {
	ss := strconv.FormatUint(seed, 10)
	var cand []engine.Hex
	for j := 0; j < engine.ChunkSize; j++ {
		for i := 0; i < engine.ChunkSize; i++ {
			p := engine.Hex{Q: qc*engine.ChunkSize + i, R: rc*engine.ChunkSize + j}
			if p == playerPos || p == frag || p == avoid || !w.At(p).Passable || v04CellTaken(w, p) {
				continue
			}
			cand = append(cand, p)
		}
	}
	if len(cand) == 0 {
		return engine.Hex{}, false
	}
	idx := int(engine.Hash01(ss, "res-pos", qc, rc) * float64(len(cand)))
	if idx >= len(cand) {
		idx = len(cand) - 1
	}
	return cand[idx], true
}

// v04BuildResource assembles one resource crystal: 世界所有物（Owner
// "world"），Integrity 1、无组件——不可动、不可感、采一次即空；不阻
// 挡移动（engine EntityAt 对 Kind "resource" 透明，见 world.go）.
func v04BuildResource(pos engine.Hex, ess engine.Essence) *engine.Entity {
	return &engine.Entity{
		Pos: pos, Kind: "resource", Owner: "world", Essence: ess,
		Integrity: 1, MaxIntegrity: 1,
	}
}
