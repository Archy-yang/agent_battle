package engine

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

// Vec2 is a grid coordinate.
type Vec2 struct{ X, Y int }

// Entity is the only kind of object in the world (world_rule §4).
type Entity struct {
	ID           EntityID
	Pos          Vec2
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

// World is the唯一真实状态 (world_rule §32). It never leaves the engine.
type World struct {
	Tick     int64
	Seed     uint64
	Rng      *Rng
	W, H     int
	Cells    []Cell // row-major, idx = y*W+x
	Entities map[EntityID]*Entity
	// Materials is the 物料库 per owner (world_genesis §3.2): essence → units.
	// Assembling consumes one unit per component.
	Materials map[string]map[Essence]int
	nextID    EntityID
}

// NewWorld creates a W×H world of passable earth.
func NewWorld(w, h int, seed uint64) *World {
	cells := make([]Cell, w*h)
	for i := range cells {
		cells[i] = Cell{Essence: Earth, Passable: true}
	}
	return &World{
		Seed:      seed,
		Rng:       NewRng(seed),
		W:         w,
		H:         h,
		Cells:     cells,
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

// At returns the cell at p, or nil if out of bounds.
func (w *World) At(p Vec2) *Cell {
	if p.X < 0 || p.Y < 0 || p.X >= w.W || p.Y >= w.H {
		return nil
	}
	return &w.Cells[p.Y*w.W+p.X]
}

// InBounds reports whether p is inside the grid.
func (w *World) InBounds(p Vec2) bool { return w.At(p) != nil }

// Chebyshev returns the Chebyshev distance between a and b.
func Chebyshev(a, b Vec2) int {
	dx := a.X - b.X
	if dx < 0 {
		dx = -dx
	}
	dy := a.Y - b.Y
	if dy < 0 {
		dy = -dy
	}
	if dx > dy {
		return dx
	}
	return dy
}

// Adjacent reports whether a and b are orthogonally adjacent.
func Adjacent(a, b Vec2) bool {
	return Chebyshev(a, b) == 1 && (a.X == b.X || a.Y == b.Y)
}

// EntityAt returns an entity at p that blocks movement (fragments do
// not block), or nil. Iteration order is deterministic for v0.1: the
// lowest entity id wins.
func (w *World) EntityAt(p Vec2) *Entity {
	var found *Entity
	for id, e := range w.Entities {
		if e.Pos != p || e.Kind == "fragment" {
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
