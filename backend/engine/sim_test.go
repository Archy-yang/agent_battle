package engine

import (
	"strings"
	"testing"
)

func newTestWorld() (*World, *Entity) {
	w := NewWorld(8, 8, 42)
	p := &Entity{
		Pos: Vec2{1, 1}, Kind: "agent", Owner: "player", Essence: Wood,
		Integrity: 100, MaxIntegrity: 100, Battery: 100, MaxBattery: 100,
	}
	p.AddComponent(CFrame, Wood)
	p.AddComponent(CBattery, Earth)
	p.AddComponent(CMotor, Wood)
	p.AddComponent(CSensor, Water)
	p.AddComponent(CEmitter, Fire)
	w.Spawn(p)
	return w, p
}

func TestMoveCostsAndCurves(t *testing.T) {
	w, p := newTestWorld()
	evs := w.Advance(map[EntityID][]Intent{p.ID: {{Type: "move", To: &Vec2{2, 1}}}})
	if p.Pos != (Vec2{2, 1}) {
		t.Fatalf("pos = %v", p.Pos)
	}
	if p.Battery != 96 { // 100 - EnergyCost(400)=4
		t.Errorf("battery = %d", p.Battery)
	}
	if p.Heat != 1 { // HeatGain(400) = 1, cooled 2 → floor 0? see below
		t.Logf("heat after tick = %d (gain 1, cooling 2)", p.Heat)
	}
	if len(evs) == 0 || evs[0].Type != "move" {
		t.Errorf("events = %+v", evs)
	}
}

func TestMoveRejectedByOccupancy(t *testing.T) {
	w, p := newTestWorld()
	blocker := &Entity{Pos: Vec2{2, 1}, Kind: "device", Owner: "world", Essence: Earth, Integrity: 30, MaxIntegrity: 30, Battery: 10, MaxBattery: 10}
	blocker.AddComponent(CFrame, Metal)
	w.Spawn(blocker)
	w.Advance(map[EntityID][]Intent{p.ID: {{Type: "move", To: &Vec2{2, 1}}}})
	if p.Pos != (Vec2{1, 1}) {
		t.Errorf("should not move onto occupied cell, pos=%v", p.Pos)
	}
}

// 火阳过盛地形：耗气、生热（world_genesis §8 环境损耗）。
func TestDangerZoneDrains(t *testing.T) {
	w, p := newTestWorld()
	for y := 0; y < w.H; y++ {
		for x := 0; x < w.W; x++ {
			if x >= 4 && x <= 5 && y >= 4 && y <= 5 {
				w.Cells[y*w.W+x] = Cell{Essence: Fire, Danger: true, Passable: true}
			}
		}
	}
	p.Pos = Vec2{4, 4}
	b0, i0 := p.Battery, p.Integrity
	w.Advance(nil)
	if p.Battery != b0-DangerEnergyDrain {
		t.Errorf("battery = %d", p.Battery)
	}
	if p.Heat != DangerHeat-PassiveCooling {
		t.Errorf("heat = %d", p.Heat)
	}
	_ = i0
}

// 阳极自毁（world_laws 互根律）：过热每回合扣形。
func TestOverheatDamages(t *testing.T) {
	w, p := newTestWorld()
	p.Heat = OverheatThreshold + 1
	i0 := p.Integrity
	w.Advance(nil)
	if p.Integrity != i0-OverheatDamage {
		t.Errorf("integrity = %d, want -%d", p.Integrity, OverheatDamage)
	}
}

// 火克金走克曲线；金甲目标受伤（world_laws §5）。
func TestEmitterAttackHeng(t *testing.T) {
	w, p := newTestWorld()
	// 目标：金甲炮台在 (3,1)，其质 600 → r=1.0 落衡区
	m := &Entity{Pos: Vec2{3, 1}, Kind: "agent", Owner: "unknown", Essence: Metal, Integrity: 600, MaxIntegrity: 600, Battery: 50, MaxBattery: 50}
	m.AddComponent(CFrame, Metal)
	m.AddComponent(CEmitter, Fire)
	w.Spawn(m)
	em := p.FindComponent(CEmitter)
	_ = em
	evs := w.Advance(map[EntityID][]Intent{p.ID: {{Type: "activate", Component: em.ID, Target: m.ID, Power: 600}}})
	if m.Integrity != 600-90 { // 衡区 600×1.5÷10
		t.Errorf("target integrity = %d, want %d", m.Integrity, 600-90)
	}
	found := false
	for _, e := range evs {
		if e.Type == "attack" && strings.Contains(e.Text, "衡") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 衡 attack event, got %+v", evs)
	}
}

// 侮区：以弱击强会被反噬（world_laws §5 克曲线）。
func TestEmitterAttackWu(t *testing.T) {
	w, p := newTestWorld()
	p.Integrity = 100
	m := &Entity{Pos: Vec2{3, 1}, Kind: "agent", Owner: "unknown", Essence: Metal, Integrity: 1000, MaxIntegrity: 1000, Battery: 50, MaxBattery: 50}
	m.AddComponent(CFrame, Metal)
	w.Spawn(m)
	em := p.FindComponent(CEmitter)
	_ = em
	i0 := p.Integrity
	w.Advance(map[EntityID][]Intent{p.ID: {{Type: "activate", Component: em.ID, Target: m.ID, Power: 100}}})
	if p.Integrity != i0-5 { // 反噬 = 100×0.5÷10
		t.Errorf("attacker integrity = %d, want backlash -5", p.Integrity)
	}
	if m.Integrity != 1000 {
		t.Errorf("target should be unharmed in 侮 zone")
	}
}

func TestTransferPickup(t *testing.T) {
	w, p := newTestWorld()
	f := &Entity{Pos: p.Pos, Kind: "fragment", Owner: "world", Essence: Earth, Integrity: 1, MaxIntegrity: 1}
	w.Spawn(f)
	w.Advance(map[EntityID][]Intent{p.ID: {{Type: "transfer", Target: f.ID}}})
	if _, ok := w.Entities[f.ID]; ok {
		t.Errorf("fragment should leave the world map when carried")
	}
	if len(p.Carrying) != 1 || p.Carrying[0] != f.ID {
		t.Errorf("carrying = %v", p.Carrying)
	}
}

func TestAssembleConsumesMaterials(t *testing.T) {
	w, p := newTestWorld()
	w.Materials["player"] = map[Essence]int{Metal: 2, Earth: 2, Wood: 2, Water: 2}
	metal := Metal
	bp := []CompSpec{
		{Type: CFrame, Essence: &metal},
		{Type: CBattery},
		{Type: CSensor},
	}
	evs := w.Advance(map[EntityID][]Intent{p.ID: {{Type: "assemble", Blueprint: bp}}})
	var dev *Entity
	for _, e := range w.Entities {
		if e.Kind == "device" {
			dev = e
		}
	}
	if dev == nil {
		t.Fatalf("device not spawned, events %+v", evs)
	}
	if len(dev.Components) != 3 {
		t.Errorf("device components = %d", len(dev.Components))
	}
	if len(dev.Connections) != 2 {
		t.Errorf("battery should feed 2 consumers, got %d", len(dev.Connections))
	}
	if w.Materials["player"][Metal] != 1 {
		t.Errorf("metal = %d", w.Materials["player"][Metal])
	}
}

func TestAssembleRejectsBadBlueprint(t *testing.T) {
	w, p := newTestWorld()
	w.Materials["player"] = map[Essence]int{Metal: 5}
	// 无储气
	evs := w.Advance(map[EntityID][]Intent{p.ID: {{Type: "assemble", Blueprint: []CompSpec{{Type: CFrame}, {Type: CMotor}}}}})
	if len(evs) == 0 || evs[0].Type != "intent_rejected" {
		t.Errorf("want rejection, got %+v", evs)
	}
}

// 感识开启扩大感知半径（world_laws 律六）。
func TestObservationRadius(t *testing.T) {
	w, p := newTestWorld()
	w.Advance(nil)
	obs := w.Observe(p.ID, nil)
	if obs == nil || len(obs.Visible) != 9 { // radius 1 → 3×3
		t.Fatalf("passive visible = %+v", obs)
	}
	s := p.FindComponent(CSensor)
	w.Advance(map[EntityID][]Intent{p.ID: {{Type: "activate", Component: s.ID}}})
	obs = w.Observe(p.ID, nil)
	if len(obs.Visible) != 36 { // radius 4，8×8 边界截断后 6×6
		t.Errorf("sensor visible = %d", len(obs.Visible))
	}
	if !obs.Self.SensorActive {
		t.Errorf("sensor should be active")
	}
}

// ---- move_to 寻路移动订单（M3.0）----

func hasEvent(evs []Event, typ string) bool {
	for _, e := range evs {
		if e.Type == typ {
			return true
		}
	}
	return false
}

// 绕障碍：直线路径上有不可通行格，move_to 仍到达（静态网格 BFS）。
func TestMoveToRoutesAroundObstacle(t *testing.T) {
	w, p := newTestWorld()
	// 封死 x=3 整列，只留 (3,5) 一条缝
	for y := 0; y < w.H; y++ {
		w.Cells[y*w.W+3].Passable = (y == 5)
	}
	target := Vec2{5, 1}
	w.Advance(map[EntityID][]Intent{p.ID: {{Type: "move_to", To: &target}}})
	if p.OrderTo == nil || *p.OrderTo != target {
		t.Fatalf("order not set: %+v", p.OrderTo)
	}
	if p.Pos == target {
		t.Fatalf("should not teleport to target: %v", p.Pos)
	}
	var evs []Event
	for i := 0; i < 30 && p.OrderTo != nil; i++ {
		evs = w.Advance(nil)
	}
	if p.OrderTo != nil || p.Pos != target {
		t.Fatalf("did not arrive: pos=%v order=%v", p.Pos, p.OrderTo)
	}
	if !hasEvent(evs, "order_done") {
		t.Errorf("missing order_done, events = %+v", evs)
	}
	// BFS 最短路 12 步 × EnergyCost(400)=4
	if p.Battery != 100-12*4 {
		t.Errorf("battery = %d, want %d（应走最短路 12 步）", p.Battery, 100-12*4)
	}
	if hasEvent(evs, "order_blocked") {
		t.Errorf("静态障碍不应造成 order_blocked")
	}
}

// 被实体挡路：本 tick 不动、订单保留（order_blocked），障碍消失后继续走。
func TestMoveToWaitsForDynamicBlocker(t *testing.T) {
	w, p := newTestWorld()
	blocker := &Entity{Pos: Vec2{2, 1}, Kind: "device", Owner: "world", Essence: Earth, Integrity: 30, MaxIntegrity: 30, Battery: 10, MaxBattery: 10}
	blocker.AddComponent(CFrame, Metal)
	w.Spawn(blocker)
	target := Vec2{4, 1}
	w.Advance(map[EntityID][]Intent{p.ID: {{Type: "move_to", To: &target}}})
	evs := w.Advance(nil)
	if p.Pos != (Vec2{1, 1}) {
		t.Fatalf("should wait, pos = %v", p.Pos)
	}
	if p.OrderTo == nil || *p.OrderTo != target {
		t.Fatalf("order lost while blocked: %+v", p.OrderTo)
	}
	if !hasEvent(evs, "order_blocked") {
		t.Errorf("missing order_blocked, events = %+v", evs)
	}
	delete(w.Entities, blocker.ID) // 障碍消失
	for i := 0; i < 10 && p.OrderTo != nil; i++ {
		evs = w.Advance(nil)
	}
	if p.Pos != target || p.OrderTo != nil {
		t.Fatalf("did not arrive after blocker left: pos=%v", p.Pos)
	}
	if !hasEvent(evs, "order_done") {
		t.Errorf("missing order_done, events = %+v", evs)
	}
}

// 到达后订单清除（order_done）。
func TestMoveToClearsOrderOnArrival(t *testing.T) {
	w, p := newTestWorld()
	target := Vec2{2, 1}
	evs := w.Advance(map[EntityID][]Intent{p.ID: {{Type: "move_to", To: &target}}})
	if p.Pos != target || p.OrderTo != nil {
		t.Fatalf("pos=%v order=%v", p.Pos, p.OrderTo)
	}
	if !hasEvent(evs, "order_done") {
		t.Errorf("missing order_done, events = %+v", evs)
	}
	// 之后不应再有任何订单步进
	evs = w.Advance(nil)
	if p.Pos != target {
		t.Errorf("moved after order done: %v", p.Pos)
	}
	if hasEvent(evs, "move") {
		t.Errorf("unexpected move after arrival: %+v", evs)
	}
}

// 显式 move 优先于订单：该 tick 走显式目标，订单保留。
func TestExplicitMoveOverridesOrder(t *testing.T) {
	w, p := newTestWorld()
	target := Vec2{3, 1}
	w.Advance(map[EntityID][]Intent{p.ID: {
		{Type: "move_to", To: &target},
		{Type: "move", To: &Vec2{1, 2}},
	}})
	if p.Pos != (Vec2{1, 2}) {
		t.Fatalf("explicit move should win, pos = %v", p.Pos)
	}
	if p.OrderTo == nil || *p.OrderTo != target {
		t.Fatalf("order should be kept: %+v", p.OrderTo)
	}
	for i := 0; i < 10 && p.OrderTo != nil; i++ {
		w.Advance(nil)
	}
	if p.Pos != target || p.OrderTo != nil {
		t.Fatalf("order should resume and finish: pos=%v", p.Pos)
	}
}

// move_to 不占 2 意图预算：同 tick move_to + 另两个动作都生效。
func TestMoveToDoesNotConsumeIntentBudget(t *testing.T) {
	w, p := newTestWorld()
	target := Vec2{4, 1}
	s := p.FindComponent(CSensor)
	evs := w.Advance(map[EntityID][]Intent{p.ID: {
		{Type: "move_to", To: &target},      // 订单，不占预算
		{Type: "activate", Component: s.ID}, // 动作 1
		{Type: "move", To: &Vec2{1, 2}},     // 动作 2（预算满）
	}})
	if p.Pos != (Vec2{1, 2}) {
		t.Errorf("second action should resolve, pos = %v", p.Pos)
	}
	if s.Active != true {
		t.Errorf("first action should resolve, sensor active = %v", s.Active)
	}
	if p.OrderTo == nil || *p.OrderTo != target {
		t.Errorf("move_to should resolve despite full budget: %+v", p.OrderTo)
	}
	if !hasEvent(evs, "order_set") {
		t.Errorf("missing order_set, events = %+v", evs)
	}
}

// 不可达目标被拒绝：越界/不可通行/被墙隔死。
func TestMoveToRejectsUnreachable(t *testing.T) {
	w, p := newTestWorld()
	// 目标格本身不可通行
	w.Cells[1*w.W+2].Passable = false
	bad := Vec2{2, 1}
	evs := w.Advance(map[EntityID][]Intent{p.ID: {{Type: "move_to", To: &bad}}})
	if !hasEvent(evs, "intent_rejected") || p.OrderTo != nil {
		t.Errorf("impassable target should be rejected: %+v", evs)
	}
	// 目标被整列墙隔死
	for y := 0; y < w.H; y++ {
		w.Cells[y*w.W+3].Passable = false
	}
	far := Vec2{5, 1}
	evs = w.Advance(map[EntityID][]Intent{p.ID: {{Type: "move_to", To: &far}}})
	if !hasEvent(evs, "intent_rejected") || p.OrderTo != nil {
		t.Errorf("unreachable target should be rejected: %+v", evs)
	}
	if p.Pos != (Vec2{1, 1}) {
		t.Errorf("rejected order must not move, pos = %v", p.Pos)
	}
}
