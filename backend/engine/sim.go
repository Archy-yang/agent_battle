package engine

import "fmt"

// MaxIntentsPerTick caps what one entity may attempt per tick
// (world_laws 律四：一时之所为有限).
const MaxIntentsPerTick = 2

// Intent is one proposed action. Agents return intents; the engine
// decides reality (world_rule §12). Only primitive intents exist —
// "attack"/"build" are NOT intents (world_rule §14): attacking is
// activating an Emitter, building is assembling a Blueprint.
type Intent struct {
	Type      string      `json:"type"` // move|move_to|activate|transfer|inspect|connect|disconnect|assemble
	Target    EntityID    `json:"target,omitempty"`
	To        *Hex        `json:"to,omitempty"`
	Component ComponentID `json:"component,omitempty"`
	Power     int         `json:"power,omitempty"` // 阳性输出强度 0..1000, 0 = default
	Blueprint []CompSpec  `json:"blueprint,omitempty"`
}

// CompSpec requests one component inside an assemble blueprint.
type CompSpec struct {
	Type    ComponentType `json:"type"`
	Essence *Essence      `json:"essence,omitempty"` // nil = default essence
}

// Event is a resolved fact of one tick — the world's own record.
type Event struct {
	Tick   int64    `json:"tick"`
	Type   string   `json:"type"`
	Text   string   `json:"text"`
	Entity EntityID `json:"entity,omitempty"`
}

func (w *World) emit(evs *[]Event, e *Entity, typ, format string, args ...any) {
	*evs = append(*evs, Event{Tick: w.Tick, Type: typ, Entity: e.ID, Text: fmt.Sprintf(format, args...)})
}

// Tick advances the world one step (world_rule §3.6):
// collect → validate → resolve → physics. Entities act in id order.
//
// move_to orders are not actions (订单不是动作)：they bypass the
// MaxIntentsPerTick budget, and each tick — when the entity spent no
// explicit move intent — the engine auto-steps one cell along the path.
func (w *World) Advance(intents map[EntityID][]Intent) []Event {
	w.Tick++
	var evs []Event
	for _, e := range w.SortedEntities() {
		list := intents[e.ID]
		used := 0
		explicitMove := false
		for _, in := range list {
			if in.Type == "move_to" { // 订单不占意图预算
				w.resolve(e, in, &evs)
				continue
			}
			if used >= MaxIntentsPerTick {
				break
			}
			used++
			w.resolve(e, in, &evs)
			if in.Type == "move" {
				explicitMove = true // 显式 move 优先于订单
			}
			if e.Integrity <= 0 {
				break
			}
		}
		if explicitMove || e.Integrity <= 0 {
			continue // 本 tick 已有显式移动，订单让位
		}
		w.stepOrder(e, &evs)
	}
	w.environment(&evs)
	return evs
}

// stepOrder executes one auto-step of a standing move_to order.
// The path is recomputed each tick on the static grid, so a reroute is
// picked up automatically once the terrain picture changes (it never
// does in v0.1); dynamic blockers are handled below by waiting.
func (w *World) stepOrder(e *Entity, evs *[]Event) {
	if e.OrderTo == nil {
		return
	}
	to := *e.OrderTo
	if e.Pos == to { // 到达：清除订单
		e.OrderTo = nil
		w.emit(evs, e, "order_done", "已到达目标格 %v", to)
		return
	}
	path, ok := w.bfsPath(e.Pos, to)
	if !ok || len(path) == 0 {
		w.emit(evs, e, "order_blocked", "无法寻路至 %v，订单保留", to)
		return
	}
	step := path[0]
	if w.EntityAt(step) != nil { // 被动态实体挡路：等待，订单保留
		w.emit(evs, e, "order_blocked", "前方 %v 被占据，本 tick 等待", step)
		return
	}
	cost := EnergyCost(clampPower(400))
	if e.Battery < cost { // 储气不足：保留订单，气够了继续走
		w.emit(evs, e, "order_blocked", "储气不足（需 %d，余 %d），订单保留", cost, e.Battery)
		return
	}
	w.resolveMove(e, Intent{Type: "move", To: &step, Power: 400}, evs)
	if e.Pos == step && e.Pos == to { // 这一步踏上目标格
		e.OrderTo = nil
		w.emit(evs, e, "order_done", "订单完成，已到达 %v", to)
	}
}

func (w *World) resolve(e *Entity, in Intent, evs *[]Event) {
	switch in.Type {
	case "move":
		w.resolveMove(e, in, evs)
	case "move_to":
		w.resolveMoveTo(e, in, evs)
	case "activate":
		w.resolveActivate(e, in, evs)
	case "transfer":
		w.resolveTransfer(e, in, evs)
	case "inspect":
		w.resolveInspect(e, in, evs)
	case "connect":
		w.resolveConnect(e, in, evs)
	case "disconnect":
		w.resolveDisconnect(e, in, evs)
	case "assemble":
		w.resolveAssemble(e, in, evs)
	default:
		w.emit(evs, e, "intent_rejected", "未知意图 %q", in.Type)
	}
}

func clampPower(x int) int {
	if x <= 0 {
		return 400
	}
	if x > 1000 {
		return 1000
	}
	return x
}

func (w *World) resolveMove(e *Entity, in Intent, evs *[]Event) {
	if in.To == nil {
		w.emit(evs, e, "intent_rejected", "move 缺少目标格")
		return
	}
	if e.FindComponent(CMotor) == nil {
		w.emit(evs, e, "intent_rejected", "没有灵机，无法移动")
		return
	}
	to := *in.To
	// v0.4 无限世界：InBounds 校验取消（一切格皆在界内），只保留地形
	// 可通行 / 邻接 / 占位 / 储气检查.
	if !w.At(to).Passable {
		w.emit(evs, e, "intent_rejected", "目标格不可通行 %v", to)
		return
	}
	if !Adjacent(e.Pos, to) {
		w.emit(evs, e, "intent_rejected", "一时只能移动一格")
		return
	}
	if w.EntityAt(to) != nil {
		w.emit(evs, e, "intent_rejected", "目标格被占据 %v", to)
		return
	}
	x := clampPower(in.Power)
	cost := EnergyCost(x)
	if e.Battery < cost {
		w.emit(evs, e, "intent_rejected", "储气不足（需 %d，余 %d）", cost, e.Battery)
		return
	}
	e.Battery -= cost
	e.Heat += HeatGain(x)
	e.Pos = to
	w.emit(evs, e, "move", "移动至 %v（耗气 %d，生热 %d）", to, cost, HeatGain(x))
}

// resolveMoveTo registers a standing pathing order ("move_to", M3.0).
// 订单不是动作：this tick it consumes no action slot and no qi — the
// engine walks the path one cell per tick afterwards (stepOrder).
// Reachability is judged on the static passable grid via BFS; dynamic
// occupants are ignored there and handled at stepping time.
func (w *World) resolveMoveTo(e *Entity, in Intent, evs *[]Event) {
	if in.To == nil {
		w.emit(evs, e, "intent_rejected", "move_to 缺少目标格")
		return
	}
	if e.FindComponent(CMotor) == nil {
		w.emit(evs, e, "intent_rejected", "没有灵机，无法移动")
		return
	}
	to := *in.To
	if !w.At(to).Passable {
		w.emit(evs, e, "intent_rejected", "目标格不可通行 %v", to)
		return
	}
	path, ok := w.bfsPath(e.Pos, to)
	if !ok {
		w.emit(evs, e, "intent_rejected", "目标格不可达 %v", to)
		return
	}
	e.OrderTo = &to
	w.emit(evs, e, "order_set", "移动订单：前往 %v（%d 步）", to, len(path))
}

// resolveActivate: sensor → toggle perception on; emitter + Target →
// project its essence onto the target (this is the only way anything
// "attacks" — there is no attack intent, world_laws §4 律五).
func (w *World) resolveActivate(e *Entity, in Intent, evs *[]Event) {
	c := e.Component(in.Component)
	if c == nil {
		w.emit(evs, e, "intent_rejected", "组件不存在 %d", in.Component)
		return
	}
	x := clampPower(in.Power)
	cost := EnergyCost(x)
	if e.Battery < cost {
		w.emit(evs, e, "intent_rejected", "储气不足（需 %d，余 %d）", cost, e.Battery)
		return
	}
	switch c.Type {
	case CSensor:
		e.Battery -= cost
		e.Heat += HeatGain(x)
		c.Active = !c.Active
		w.emit(evs, e, "sense", "感识%s（耗气 %d）", onOff(c.Active), cost)
	case CEmitter:
		t := w.Entities[in.Target]
		if t == nil || t.ID == e.ID {
			w.emit(evs, e, "intent_rejected", "作用目标无效")
			return
		}
		if HexDist(e.Pos, t.Pos) > 3 {
			w.emit(evs, e, "intent_rejected", "超出作用距离")
			return
		}
		e.Battery -= cost
		e.Heat += HeatGain(x)
		w.project(e, c, t, x, evs)
	default:
		w.emit(evs, e, "intent_rejected", "该组件不可激活 %s", c.Type)
	}
}

func onOff(b bool) string {
	if b {
		return "开启"
	}
	return "关闭"
}

// project applies essence power onto target through the 克曲线.
// 本源强度取目标的 MaxIntegrity（其质，稳定量）。
func (w *World) project(e *Entity, c *Component, t *Entity, x int, evs *[]Event) {
	res := ApplyPower(c.Essence, t.Essence, x, t.MaxIntegrity)
	if res.Backlash > 0 {
		e.Integrity -= res.Backlash
		w.emit(evs, e, "backlash", "%s→%s【%s】反噬！自损 %d",
			c.Essence, t.Essence, res.Zone, res.Backlash)
		return
	}
	if res.Damage <= 0 {
		w.emit(evs, e, "attack", "%s→%s【%s】杯水车薪，无功而返", c.Essence, t.Essence, res.Zone)
		return
	}
	t.Integrity -= res.Damage
	w.emit(evs, e, "attack", "%s→%s【%s】作用 %d（目标形余 %d）",
		c.Essence, t.Essence, res.Zone, res.Damage, maxInt(0, t.Integrity))
	if res.Spill > 0 {
		spillDmg := res.Damage * res.Spill / 1000
		for _, o := range w.SortedEntities() {
			if o.ID == e.ID || o.ID == t.ID || o.Kind == "fragment" {
				continue
			}
			if HexDist(o.Pos, t.Pos) <= 1 {
				o.Integrity -= spillDmg
				w.emit(evs, o, "spill", "溢出波及，损 %d", spillDmg)
			}
		}
	}
}

func (w *World) resolveTransfer(e *Entity, in Intent, evs *[]Event) {
	t := w.Entities[in.Target]
	if t == nil || t.Kind != "fragment" {
		w.emit(evs, e, "intent_rejected", "只能搬取元气结晶")
		return
	}
	if t.Pos != e.Pos {
		w.emit(evs, e, "intent_rejected", "必须站在结晶所在格")
		return
	}
	delete(w.Entities, t.ID)
	e.Carrying = append(e.Carrying, t.ID)
	w.emit(evs, e, "pickup", "取得元气结晶")
}

func (w *World) resolveInspect(e *Entity, in Intent, evs *[]Event) {
	t := w.Entities[in.Target]
	if t == nil {
		w.emit(evs, e, "intent_rejected", "检视目标不存在")
		return
	}
	if HexDist(e.Pos, t.Pos) > 4 {
		w.emit(evs, e, "intent_rejected", "超出感知范围")
		return
	}
	names := ""
	for _, c := range t.Components {
		names += c.Type.String() + "(" + c.Essence.String() + ") "
	}
	w.emit(evs, e, "inspect", "%s#%d %s：形 %d/%d 热 %d 组件[%s]",
		t.Kind, t.ID, t.Essence, maxInt(0, t.Integrity), t.MaxIntegrity, t.Heat, names)
}

func (w *World) resolveConnect(e *Entity, in Intent, evs *[]Event) {
	if !e.Connect(in.Component, ComponentID(in.Target)) {
		w.emit(evs, e, "intent_rejected", "连接失败（组件不存在）")
		return
	}
	w.emit(evs, e, "connect", "连接 %d → %d", in.Component, in.Target)
}

func (w *World) resolveDisconnect(e *Entity, in Intent, evs *[]Event) {
	kept := e.Connections[:0]
	ok := false
	for _, c := range e.Connections {
		if c.From == in.Component && c.To == ComponentID(in.Target) {
			ok = true
			continue
		}
		kept = append(kept, c)
	}
	e.Connections = kept
	if !ok {
		w.emit(evs, e, "intent_rejected", "连接不存在")
		return
	}
	w.emit(evs, e, "disconnect", "断开 %d → %d", in.Component, in.Target)
}

// resolveAssemble instantiates a Blueprint: engine validates ports and
// complexity, consumes materials, spawns a device entity
// (world_rule §15：LLM 负责设计，Engine 负责实现).
func (w *World) resolveAssemble(e *Entity, in Intent, evs *[]Event) {
	bp := in.Blueprint
	if len(bp) < 2 {
		w.emit(evs, e, "intent_rejected", "蓝图至少需要两个组件")
		return
	}
	if len(bp) > 8 {
		w.emit(evs, e, "intent_rejected", "超出复杂度预算（≤8 组件）")
		return
	}
	frames, batteries := 0, 0
	for _, s := range bp {
		if s.Type == CFrame {
			frames++
		}
		if s.Type == CBattery {
			batteries++
		}
	}
	if frames != 1 || batteries != 1 {
		w.emit(evs, e, "intent_rejected", "蓝图需要恰好一个躯壳和一个储气")
		return
	}
	inv := w.Materials[e.Owner]
	for _, s := range bp {
		ess := DefaultEssence(s.Type)
		if s.Essence != nil {
			ess = *s.Essence
		}
		if inv[ess] <= 0 {
			w.emit(evs, e, "intent_rejected", "物料不足：%s", ess)
			return
		}
	}
	// all-or-nothing consumption
	for _, s := range bp {
		ess := DefaultEssence(s.Type)
		if s.Essence != nil {
			ess = *s.Essence
		}
		inv[ess]--
	}
	pos, ok := w.freeAdjacent(e.Pos)
	if !ok {
		w.emit(evs, e, "intent_rejected", "周围没有可放置的空间")
		return
	}
	d := &Entity{
		Pos: pos, Kind: "device", Owner: e.Owner,
		Essence: Earth, Integrity: 30, MaxIntegrity: 30,
		Battery: 30, MaxBattery: 30,
	}
	var battery ComponentID
	for _, s := range bp {
		ess := DefaultEssence(s.Type)
		if s.Essence != nil {
			ess = *s.Essence
		}
		c := d.AddComponent(s.Type, ess)
		if s.Type == CBattery {
			battery = c.ID
		}
	}
	for _, c := range d.Components { // 源→储→用 (world_laws 律七)
		if c.ID != battery {
			d.Connect(battery, c.ID)
		}
	}
	w.Spawn(d)
	w.emit(evs, e, "assemble", "组装完成：%s @ %v", describeDevice(d), pos)
}

func describeDevice(d *Entity) string {
	s := ""
	for i, c := range d.Components {
		if i > 0 {
			s += "+"
		}
		s += c.Type.String() + c.Essence.String()
	}
	return s
}

func (w *World) freeAdjacent(p Hex) (Hex, bool) {
	for _, q := range HexNeighbors(p) { // 六方向（design §1.4）
		if w.At(q).Passable && w.EntityAt(q) == nil {
			return q, true
		}
	}
	return Hex{}, false
}

// environment is the physics phase: 火阳过盛地形、被动散热、过热损伤、形散.
func (w *World) environment(evs *[]Event) {
	for _, e := range w.SortedEntities() {
		if e.Kind == "fragment" {
			continue
		}
		if c := w.At(e.Pos); c != nil && c.Danger {
			e.Battery = maxInt(0, e.Battery-DangerEnergyDrain)
			e.Heat += DangerHeat
		}
		if e.Heat > OverheatThreshold { // 阳极自毁，先于被动散热判定
			e.Integrity -= OverheatDamage
			e.Heat -= 2
			w.emit(evs, e, "overheat", "过热！形 -%d（余 %d）", OverheatDamage, maxInt(0, e.Integrity))
		}
		e.Heat = maxInt(0, e.Heat-PassiveCooling)
		if e.Integrity <= 0 { // 形散，不可复聚（world_laws 禁区四）
			w.emit(evs, e, "destroyed", "%s#%d 形散", e.Kind, e.ID)
			delete(w.Entities, e.ID)
		}
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
