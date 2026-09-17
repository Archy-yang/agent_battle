package engine

// Observation is what an agent may know at one tick — never the World
// State itself (world_rule §19: Reality ≠ Agent Knowledge).
//
// v0.1 honesty model (world_genesis §5.2): perceived traces are exact
// (the world does not lie), but only what perception reaches exists
// for the agent — hiding is legal, lying is not.
type Observation struct {
	Tick     int64        `json:"tick"`
	Self     SelfView     `json:"self"`
	Visible  []Vec2       `json:"visible"`
	Entities []SeenEntity `json:"entities"`
	Events   []Event      `json:"events"`
}

// ComponentView is one of the agent's own components as seen in the
// Capability Manifest sense (world_rule §22): what it is, its essence,
// whether it is active.
type ComponentView struct {
	ID      ComponentID `json:"id"`
	Type    string      `json:"type"`
	Essence string      `json:"essence"`
	Active  bool        `json:"active"`
}

// SelfView is the agent's own state — the one place full detail is legal.
type SelfView struct {
	Pos          Vec2            `json:"pos"`
	Essence      Essence         `json:"essence"`
	Integrity    int             `json:"integrity"`
	MaxIntegrity int             `json:"max_integrity"`
	Heat         int             `json:"heat"`
	Battery      int             `json:"battery"`
	MaxBattery   int             `json:"max_battery"`
	Carrying     []EntityID      `json:"carrying"`
	Components   []ComponentView `json:"components"`
	HasMotor     bool            `json:"has_motor"`
	SensorActive bool            `json:"sensor_active"`
	HasEmitter   bool            `json:"has_emitter"`
}

// SeenEntity is another entity as perceived — with its 五行 traces
// (world_laws 律六：观察即读迹).
type SeenEntity struct {
	ID       EntityID `json:"id"`
	Pos      Vec2     `json:"pos"`
	Distance int      `json:"distance"`
	Kind     string   `json:"kind"`
	Owner    string   `json:"owner"`
	Essence  Essence  `json:"essence"`
	Integrity int    `json:"integrity"`
	Heat     int      `json:"heat"`
}

// Observe generates the perception for entity id. lastEvents are the
// events of the most recent tick (agents see what happened, world_rule
// §12 Events).
func (w *World) Observe(id EntityID, lastEvents []Event) *Observation {
	e := w.Entities[id]
	if e == nil {
		return nil
	}
	radius := 1 // 被动：自身与邻格
	if s := e.FindComponent(CSensor); s != nil && s.Active {
		radius = 4 // 感识开启（world_laws 律六：读迹有代价）
	}
	carrying := e.Carrying
	if carrying == nil {
		carrying = []EntityID{} // JSON 空数组，而不是 null
	}
	comps := make([]ComponentView, 0, len(e.Components))
	for _, c := range e.Components {
		comps = append(comps, ComponentView{
			ID: c.ID, Type: c.Type.String(), Essence: c.Essence.String(), Active: c.Active,
		})
	}
	obs := &Observation{
		Tick: w.Tick,
		Self: SelfView{
			Pos: e.Pos, Essence: e.Essence,
			Integrity: e.Integrity, MaxIntegrity: e.MaxIntegrity,
			Heat: e.Heat, Battery: e.Battery, MaxBattery: e.MaxBattery,
			Carrying: carrying,
			Components: comps,
			HasMotor: e.FindComponent(CMotor) != nil,
			HasEmitter: e.FindComponent(CEmitter) != nil,
			SensorActive: func() bool {
				if s := e.FindComponent(CSensor); s != nil {
					return s.Active
				}
				return false
			}(),
		},
		Visible:  []Vec2{},
		Entities: []SeenEntity{},
		Events:   lastEvents,
	}
	for dx := -radius; dx <= radius; dx++ {
		for dy := -radius; dy <= radius; dy++ {
			p := Vec2{X: e.Pos.X + dx, Y: e.Pos.Y + dy}
			if w.InBounds(p) {
				obs.Visible = append(obs.Visible, p)
			}
		}
	}
	for _, o := range w.SortedEntities() {
		d := Chebyshev(e.Pos, o.Pos)
		if o.ID == e.ID || d > radius {
			continue
		}
		obs.Entities = append(obs.Entities, SeenEntity{
			ID: o.ID, Pos: o.Pos, Distance: d,
			Kind: o.Kind, Owner: o.Owner, Essence: o.Essence,
			Integrity: o.Integrity, Heat: o.Heat,
		})
	}
	return obs
}
