package engine

import "container/list"

// ComponentType enumerates the v0.1 component set (world_rule §5, §31).
type ComponentType int

const (
	CFrame ComponentType = iota // 躯壳
	CBattery                    // 储气
	CMotor                      // 灵机
	CSensor                     // 感识
	CProcessor                  // 识海
	CEmitter                    // 离火
	CStorage                    // 仓廪
	CCooling                    // 清源
)

var componentNames = [...]string{"躯壳", "储气", "灵机", "感识", "识海", "离火", "仓廪", "清源"}

// String returns the Chinese name of the component type.
func (c ComponentType) String() string {
	if c < CFrame || c > CCooling {
		return "?"
	}
	return componentNames[c]
}

// DefaultEssence returns the 默认本源 of a component type
// (docs/world_laws.md §8 组件-本源表).
func DefaultEssence(t ComponentType) Essence {
	switch t {
	case CEmitter:
		return Fire
	case CSensor, CCooling:
		return Water
	case CMotor:
		return Wood
	case CFrame:
		return Metal // 刚；韧木由建造者显式选择（选材质即选天敌）
	default: // Battery, Processor, Storage
		return Earth
	}
}

// Component is one mounted capability (world_rule §5).
type Component struct {
	ID      ComponentID
	Type    ComponentType
	Essence Essence
	Active  bool // sensors/emitters are toggled by the activate intent
}

// ComponentID identifies a component within one entity.
type ComponentID int64

// Connection is a Port link between two components of one entity
// (world_rule §5: Component Graph; v0.1 records the graph, energy
// flows through the single battery pool below).
type Connection struct {
	From, To ComponentID
}

// Entity is the only kind of object in the world (world_rule §4).
type Entity struct {
	ID           EntityID
	Pos          Hex
	Kind         string // "agent" | "fragment" | ...
	Owner        string // "player" | "unknown" | "world"
	Essence      Essence
	Integrity    int // 形 (world_laws 律二)；Integrity ≤ 0 即形散
	MaxIntegrity int
	Heat         int
	Battery      int // 储气
	MaxBattery   int

	Components  []Component
	Connections []Connection
	NextCompID  ComponentID

	Carrying []EntityID // carried entities (e.g. the Fragment)

	// OrderTo is a standing pathing order ("move_to"). It is not an
	// action: issuing it costs no action slot and no qi; the engine
	// auto-steps along the path each tick (see sim.go stepOrder).
	OrderTo *Hex
}

// EntityID identifies an entity within one world.
type EntityID int64

// Component returns the component with the given id, or nil.
func (e *Entity) Component(id ComponentID) *Component {
	for i := range e.Components {
		if e.Components[i].ID == id {
			return &e.Components[i]
		}
	}
	return nil
}

// FindComponent returns the first active-or-present component of a type.
func (e *Entity) FindComponent(t ComponentType) *Component {
	for i := range e.Components {
		if e.Components[i].Type == t {
			return &e.Components[i]
		}
	}
	return nil
}

// AddComponent mounts a component with the next component id.
func (e *Entity) AddComponent(t ComponentType, ess Essence) *Component {
	e.NextCompID++
	c := Component{ID: e.NextCompID, Type: t, Essence: ess}
	e.Components = append(e.Components, c)
	return &e.Components[len(e.Components)-1]
}

// Connect links two ports (world_rule §5); both must belong to e.
func (e *Entity) Connect(from, to ComponentID) bool {
	if e.Component(from) == nil || e.Component(to) == nil {
		return false
	}
	e.Connections = append(e.Connections, Connection{From: from, To: to})
	return true
}

// Cell is one terrain tile (world_genesis §3.1: terrain speaks the
// laws' language — a five-element bias plus danger).
type Cell struct {
	Essence  Essence
	Danger   bool // 火阳过盛
	Passable bool
}

// ChunkSize is the axial parallelogram side of one chunk — 12×12 = 144
// hex per chunk (评审决定 #4：CS=12 照案执行). WP2 fills chunks with the
// deterministic 五行 generation; WP1 ships the infinite plain.
const ChunkSize = 12

// TerrainFunc is the pure terrain generator F(seed, q, r) → Cell
// (design §2.2): 逐块可计算的纯函数，跨块连续，与 chunk 边界无关.
type TerrainFunc func(seed uint64, q, r int) Cell

// PlainTerrain is the engine-level default terrain: 无限平原，皆土、皆
// 可通行（nil gen 兼容缝）。v0.4 的五行生成见 chunk.go Terrain——由
// 场景包（scenario.V04）显式接入.
func PlainTerrain(uint64, int, int) Cell { return Cell{Essence: Earth, Passable: true} }

// Chunk holds ChunkSize×ChunkSize cells laid out flat as an axial
// parallelogram: idx = (r%CS)*CS + (q%CS), with negative coordinates
// normalized to floor-division remainders (design §1.1).
type Chunk struct {
	QC, RC int
	Cells  [ChunkSize * ChunkSize]Cell
}

// ChunkLRUCapacity is the chunk cache capacity (评审决定 #4：LRU=256 照案
// 执行)：约 256 × 0.6KB ≈ 150KB，内存无压力 (design §2.1/§2.3).
const ChunkLRUCapacity = 256

// chunkKey identifies a chunk in the cache.
type chunkKey struct{ qc, rc int }

// ChunkCache generates and caches terrain chunks on demand (design
// §2.3)：命中即晋升的 LRU；淘汰只丢地形——地形是纯函数，重新进入时
// 按需重算，零成本回放。v0.4 不落盘（持久世界属未来扩展）。
//
// At 返回 Cell 值拷贝（WP1 注释挂的账在此结清）：LRU 淘汰随时可能
// 回收底层 chunk 存储，指针穿越淘汰边界是不安全的，故对外一律传值.
type ChunkCache struct {
	seed   uint64
	gen    TerrainFunc
	v04    bool                       // v0.4 五行生成（apron 快路径）
	chunks map[chunkKey]*list.Element // el.Value = *Chunk
	order  *list.List                 // front = 最近使用，back = 下一个淘汰
}

// NewChunkCache creates a cache bound to a seed and a terrain pure
// function; a nil gen falls back to PlainTerrain (WP1 兼容缝). v0.4 的
// 五行生成走 NewWorld / 显式注入 engine.Terrain.
func NewChunkCache(seed uint64, gen TerrainFunc) *ChunkCache {
	if gen == nil {
		gen = PlainTerrain
	}
	return &ChunkCache{
		seed:   seed,
		gen:    gen,
		chunks: map[chunkKey]*list.Element{},
		order:  list.New(),
	}
}

// At returns a copy of the cell at (q, r), generating its chunk on
// first touch and evicting the least-recently-used chunk past capacity.
func (c *ChunkCache) At(q, r int) Cell {
	key := chunkKey{floorDiv(q, ChunkSize), floorDiv(r, ChunkSize)}
	if el, ok := c.chunks[key]; ok {
		c.order.MoveToFront(el)
		return chunkCellAt(el.Value.(*Chunk), q, r)
	}
	ch := c.build(key.qc, key.rc)
	c.chunks[key] = c.order.PushFront(ch)
	if len(c.chunks) > ChunkLRUCapacity {
		if back := c.order.Back(); back != nil {
			victim := back.Value.(*Chunk)
			c.order.Remove(back)
			delete(c.chunks, chunkKey{victim.QC, victim.RC})
		}
	}
	return chunkCellAt(ch, q, r)
}

// build generates one chunk: v04 → 五行生成（14×14 apron 快路径）；
// nil gen → 无限平原（WP1 兼容）；注入的 gen 按 (seed, q, r) 逐格
// 调用（测试地形缝）.
func (c *ChunkCache) build(qc, rc int) *Chunk {
	switch {
	case c.v04:
		return genChunk(c.seed, qc, rc)
	case c.gen == nil:
		ch := &Chunk{QC: qc, RC: rc}
		for j := 0; j < ChunkSize; j++ {
			for i := 0; i < ChunkSize; i++ {
				ch.Cells[j*ChunkSize+i] = PlainTerrain(c.seed, qc*ChunkSize+i, rc*ChunkSize+j)
			}
		}
		return ch
	default:
		ch := &Chunk{QC: qc, RC: rc}
		for j := 0; j < ChunkSize; j++ {
			for i := 0; i < ChunkSize; i++ {
				ch.Cells[j*ChunkSize+i] = c.gen(c.seed, qc*ChunkSize+i, rc*ChunkSize+j)
			}
		}
		return ch
	}
}

func chunkCellAt(ch *Chunk, q, r int) Cell {
	return ch.Cells[floorMod(r, ChunkSize)*ChunkSize+floorMod(q, ChunkSize)]
}

func floorDiv(a, b int) int {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func floorMod(a, b int) int { return a - floorDiv(a, b)*b }

// World is the唯一真实状态 (world_rule §32). It never leaves the engine.
type World struct {
	Tick int64
	Seed uint64
	Rng  *Rng
	// Chunks is the terrain: on-demand generated, per-chunk cached
	// (design §1.1). v0.4 无限世界——任何 (q, r) 皆可查询.
	Chunks *ChunkCache
	// Entities is world-level storage, independent of chunks
	// (design §2.3): 实体可跨 chunk 移动，chunk 只是地形缓存.
	Entities map[EntityID]*Entity
	// Materials is the 物料库 per owner (world_genesis §3.2): essence → units.
	// Assembling consumes one unit per component.
	Materials map[string]map[Essence]int
	nextID    EntityID
}

// NewWorld creates the v0.4 deterministic infinite world: 逐块五行生成
// (chunk.go Terrain, design §2.2, apron 快路径) + LRU chunk cache (§2.3).
func NewWorld(seed uint64) *World {
	w := NewWorldTerrain(seed, nil)
	w.Chunks.v04 = true
	return w
}

// NewWorldTerrain creates an infinite world generated by gen — nil →
// PlainTerrain（WP1 兼容缝）；非 nil 时按 (seed, q, r) 逐格调用，是
// 地形相关的注入缝（场景包在此接入 v0.4 五行生成）.
func NewWorldTerrain(seed uint64, gen TerrainFunc) *World {
	return &World{
		Seed:      seed,
		Rng:       NewRng(seed),
		Chunks:    NewChunkCache(seed, gen),
		Entities:  map[EntityID]*Entity{},
		Materials: map[string]map[Essence]int{},
	}
}

// Spawn registers a new entity and returns it.
func (w *World) Spawn(e *Entity) *Entity {
	w.nextID++
	e.ID = w.nextID
	w.Entities[e.ID] = e
	return e
}

// At returns a pointer to a private copy of the cell at p — the copy
// shields callers from LRU eviction (WP1 挂账：缓存淘汰下裸指针不安
// 全，值拷贝语义在 ChunkCache.At 结清). 无限世界一切格皆可查询
// (v0.4 design §1.2)：the query may generate a chunk on demand,
// never fails.
func (w *World) At(p Hex) *Cell {
	cell := w.Chunks.At(p.Q, p.R)
	return &cell
}

// InBounds reports whether p is inside the world. v0.4 无限世界总在界内
// —— resolveMove 的越界校验语义随之取消；函数保留是给"显式界外"概念
// （未来天空/虚空层）留的桩.
func (w *World) InBounds(Hex) bool { return true }

// EntityAt returns an entity at p that blocks movement (fragments and
// v0.5 WPA 资源晶簇 do not block——晶簇与 fragment 同类：走过、站上皆可，
// 采集由 match 层对 transfer 的扩展结算), or nil. Iteration order is
// deterministic for v0.1: the lowest entity id wins.
func (w *World) EntityAt(p Hex) *Entity {
	var found *Entity
	for id, e := range w.Entities {
		if e.Pos != p || e.Kind == "fragment" || e.Kind == "resource" {
			continue
		}
		if found == nil || id < found.ID {
			found = e
		}
	}
	return found
}

// SortedEntities returns all entities ordered by id (determinism).
func (w *World) SortedEntities() []*Entity {
	ids := make([]EntityID, 0, len(w.Entities))
	for id := range w.Entities {
		ids = append(ids, id)
	}
	sortIDs(ids)
	out := make([]*Entity, 0, len(ids))
	for _, id := range ids {
		out = append(out, w.Entities[id])
	}
	return out
}

func sortIDs(ids []EntityID) {
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && ids[j] < ids[j-1]; j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
}
