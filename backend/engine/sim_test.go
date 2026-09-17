package engine

import (
	"strings"
	"testing"
)

func newTestWorldT(gen TerrainFunc) (*World, *Entity) {
	w := NewWorldTerrain(42, gen) // 无限平原（gen=nil）或自定义地形
	p := &Entity{
		Pos: Hex{1, 1}, Kind: "agent", Owner: "player", Essence: Wood,
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

func newTestWorld() (*World, *Entity) { return newTestWorldT(nil) }

func TestMoveCostsAndCurves(t *testing.T) {
	w, p := newTestWorld()
	evs := w.Advance(map[EntityID][]Intent{p.ID: {{Type: "move", To: &Hex{2, 1}}}})
	if p.Pos != (Hex{2, 1}) {
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
	blocker := &Entity{Pos: Hex{2, 1}, Kind: "device", Owner: "world", Essence: Earth, Integrity: 30, MaxIntegrity: 30, Battery: 10, MaxBattery: 10}
	blocker.AddComponent(CFrame, Metal)
	w.Spawn(blocker)
	w.Advance(map[EntityID][]Intent{p.ID: {{Type: "move", To: &Hex{2, 1}}}})
	if p.Pos != (Hex{1, 1}) {
		t.Errorf("should not move onto occupied cell, pos=%v", p.Pos)
	}
}

// 火阳过盛地形：耗气、生热（world_genesis §8 环境损耗）。
// hex 危险区：(X,Y)=(2q+r,r) ∈ {4,5}×{4,5} ⇒ 格 (0,4) 与 (0,5)。
// world-laws/0.2：驻留者无输出 → 蓄息 +RestRegen，危险格耗气 −1 →
// 净 +1 气（可自愈脱困仍劣于平地）；DangerHeat 3→2 后行进净热归零.
func TestDangerZoneDrains(t *testing.T) {
	gen := func(_ uint64, q, r int) Cell {
		x, y := 2*q+r, r
		if x >= 4 && x <= 5 && y >= 4 && y <= 5 {
			return Cell{Essence: Fire, Danger: true, Passable: true}
		}
		return Cell{Essence: Earth, Passable: true}
	}
	w, p := newTestWorldT(gen)
	p.Pos = Hex{0, 4}
	p.Battery = 60 // 低于上限：蓄息才会触发（满气者不蓄）
	b0, i0 := p.Battery, p.Integrity
	w.Advance(nil)
	if p.Battery != b0+RestRegen-DangerEnergyDrain {
		t.Errorf("battery = %d, want %d（蓄息 +2 与危险税 −2 相抵）", p.Battery, b0+RestRegen-DangerEnergyDrain)
	}
	if p.Heat != DangerHeat-PassiveCooling {
		t.Errorf("heat = %d", p.Heat)
	}
	_ = i0
}

// 蓄息律 (world-laws/0.2)：无输出回合回气且封顶；有行动不回气；
// 被拒的意图不算输出——断气者原地自愈.
func TestRestRegen(t *testing.T) {
	w, p := newTestWorld()
	// 触发：无任何意图 → +2
	p.Battery = 50
	w.Advance(nil)
	if p.Battery != 50+RestRegen {
		t.Errorf("resting battery = %d, want %d", p.Battery, 50+RestRegen)
	}
	// 封顶：不超过 MaxBattery
	p.Battery = p.MaxBattery - 1
	w.Advance(nil)
	if p.Battery != p.MaxBattery {
		t.Errorf("capped battery = %d, want %d", p.Battery, p.MaxBattery)
	}
	// 不触发：move 生效则不回气（50 − EnergyCost(400)=4）
	p.Battery = 50
	w.Advance(map[EntityID][]Intent{p.ID: {{Type: "move", To: &Hex{2, 1}}}})
	if p.Battery != 46 {
		t.Errorf("acting battery = %d, want 46（行动回合不蓄息）", p.Battery)
	}
	// 不触发：activate 生效则不回气（默认功率 400 → 耗气 4）
	p.Battery = 50
	p.Pos = Hex{1, 1}
	s := p.FindComponent(CSensor)
	w.Advance(map[EntityID][]Intent{p.ID: {{Type: "activate", Component: s.ID}}})
	if p.Battery != 46 {
		t.Errorf("sensing battery = %d, want 46", p.Battery)
	}
	// 被拒意图不算输出：气不足被拒 → 原地蓄息（断气自愈，罚站消失）
	p.Battery = 3
	p.Pos = Hex{1, 1}
	w.Advance(map[EntityID][]Intent{p.ID: {{Type: "move", To: &Hex{2, 1}}}})
	if p.Battery != 3+RestRegen {
		t.Errorf("starved battery = %d, want %d", p.Battery, 3+RestRegen)
	}
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
	// HexDist((1,1),(3,1)) = 2 ≤ 作用距离 3
	m := &Entity{Pos: Hex{3, 1}, Kind: "agent", Owner: "unknown", Essence: Metal, Integrity: 600, MaxIntegrity: 600, Battery: 50, MaxBattery: 50}
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
	m := &Entity{Pos: Hex{3, 1}, Kind: "agent", Owner: "unknown", Essence: Metal, Integrity: 1000, MaxIntegrity: 1000, Battery: 50, MaxBattery: 50}
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
// 无限 hex 世界无边界截断：HexDist ≤ r 的格数 = 3r²+3r+1（r=1 → 7，
// r=4 → 61）。
func TestObservationRadius(t *testing.T) {
	w, p := newTestWorld()
	w.Advance(nil)
	obs := w.Observe(p.ID, nil)
	if obs == nil || len(obs.Visible) != 7 { // radius 1 → 3+3+1
		t.Fatalf("passive visible = %d (want 7), obs %+v", len(obs.Visible), obs)
	}
	s := p.FindComponent(CSensor)
	w.Advance(map[EntityID][]Intent{p.ID: {{Type: "activate", Component: s.ID}}})
	obs = w.Observe(p.ID, nil)
	if len(obs.Visible) != 61 { // radius 4 → 3·16+12+1
		t.Errorf("sensor visible = %d, want 61", len(obs.Visible))
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

// 绕障碍：直线路径上有不可通行格，move_to 仍到达（静态地形 BFS）。
// hex 墙：q=3 轴线（r∈-20..20）仅留缝 (3,5)；最短路 = HexDist(1,1→3,5)
// + HexDist(3,5→5,1) = 6+4 = 10 步（任何路径必须过缝，三角不等式取等）。
func TestMoveToRoutesAroundObstacle(t *testing.T) {
	gen := func(_ uint64, q, r int) Cell {
		c := Cell{Essence: Earth, Passable: true}
		if q == 3 && r >= -20 && r <= 20 && r != 5 {
			c.Passable = false
		}
		return c
	}
	w, p := newTestWorldT(gen)
	target := Hex{5, 1}
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
	// BFS 最短路 10 步 × EnergyCost(400)=4
	if p.Battery != 100-10*4 {
		t.Errorf("battery = %d, want %d（应走最短路 10 步）", p.Battery, 100-10*4)
	}
	if hasEvent(evs, "order_blocked") {
		t.Errorf("静态障碍不应造成 order_blocked")
	}
}

// 被实体挡路：本 tick 不动、订单保留（order_blocked），障碍消失后继续走。
func TestMoveToWaitsForDynamicBlocker(t *testing.T) {
	w, p := newTestWorld()
	blocker := &Entity{Pos: Hex{2, 1}, Kind: "device", Owner: "world", Essence: Earth, Integrity: 30, MaxIntegrity: 30, Battery: 10, MaxBattery: 10}
	blocker.AddComponent(CFrame, Metal)
	w.Spawn(blocker)
	target := Hex{4, 1}
	w.Advance(map[EntityID][]Intent{p.ID: {{Type: "move_to", To: &target}}})
	evs := w.Advance(nil)
	if p.Pos != (Hex{1, 1}) {
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
	target := Hex{2, 1}
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
	target := Hex{3, 1}
	w.Advance(map[EntityID][]Intent{p.ID: {
		{Type: "move_to", To: &target},
		{Type: "move", To: &Hex{1, 2}}, // SE 邻格
	}})
	if p.Pos != (Hex{1, 2}) {
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
	target := Hex{4, 1}
	s := p.FindComponent(CSensor)
	evs := w.Advance(map[EntityID][]Intent{p.ID: {
		{Type: "move_to", To: &target},      // 订单，不占预算
		{Type: "activate", Component: s.ID}, // 动作 1
		{Type: "move", To: &Hex{1, 2}},      // 动作 2（预算满）
	}})
	if p.Pos != (Hex{1, 2}) {
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

// 不可达目标被拒绝：不可通行 / 被无限墙线隔死（hex 下无限直线确实
// 分割平面；BFS 以 PathBudget 兜底，预算耗尽视为不可达 → 拒单）。
func TestMoveToRejectsUnreachable(t *testing.T) {
	gen := func(_ uint64, q, r int) Cell {
		c := Cell{Essence: Earth, Passable: true}
		if q == 3 || (q == 2 && r == 1) {
			c.Passable = false
		}
		return c
	}
	w, p := newTestWorldT(gen)
	// 目标格本身不可通行
	bad := Hex{2, 1}
	evs := w.Advance(map[EntityID][]Intent{p.ID: {{Type: "move_to", To: &bad}}})
	if !hasEvent(evs, "intent_rejected") || p.OrderTo != nil {
		t.Errorf("impassable target should be rejected: %+v", evs)
	}
	// 目标被 q=3 无限墙线隔死
	far := Hex{5, 1}
	evs = w.Advance(map[EntityID][]Intent{p.ID: {{Type: "move_to", To: &far}}})
	if !hasEvent(evs, "intent_rejected") || p.OrderTo != nil {
		t.Errorf("unreachable target should be rejected: %+v", evs)
	}
	if p.Pos != (Hex{1, 1}) {
		t.Errorf("rejected order must not move, pos = %v", p.Pos)
	}
}

// ---- v0.5 经济循环：equip 意图（装备自身·武装反击）----

// newBareAgentWorld builds a world whose player has no Emitter: 躯壳 +
// 储气 + 灵机 three roots, ready to equip 离火 for the counterattack.
func newBareAgentWorld() (*World, *Entity) {
	w := NewWorldTerrain(42, nil)
	p := &Entity{
		Pos: Hex{1, 1}, Kind: "agent", Owner: "player", Essence: Wood,
		Integrity: 100, MaxIntegrity: 100, Battery: 100, MaxBattery: 100,
	}
	p.AddComponent(CFrame, Wood)
	p.AddComponent(CBattery, Earth)
	p.AddComponent(CMotor, Wood)
	w.Spawn(p)
	return w, p
}

// equip 消耗物料并生效：装离火 → HasEmitter，可 activate 反击假想敌。
func TestEquipInstallsEmitterAndEnablesCounterattack(t *testing.T) {
	w, p := newBareAgentWorld()
	w.Materials["player"] = map[Essence]int{Fire: 1}
	evs := w.Advance(map[EntityID][]Intent{p.ID: {{Type: "equip", Blueprint: []CompSpec{{Type: CEmitter}}}}})
	if p.FindComponent(CEmitter) == nil {
		t.Fatalf("emitter not installed, events %+v", evs)
	}
	if p.Battery != 97 { // 100 - EnergyCost(300) = 3
		t.Errorf("battery = %d, want 97", p.Battery)
	}
	if w.Materials["player"][Fire] != 0 {
		t.Errorf("fire material = %d, want 0", w.Materials["player"][Fire])
	}
	if len(evs) == 0 || evs[0].Type != "equip" || evs[0].Text != "装备 离火(火)，仓库消耗 火×1" {
		t.Errorf("equip event = %+v", evs)
	}
	// 装上即生效：下一 tick activate 反击相邻假想敌（木躯壳，火不克木，
	// 中性 ×0.5：400×0.5/10 = 20 伤）。
	foe := &Entity{Pos: Hex{2, 1}, Kind: "agent", Owner: "unknown", Essence: Wood, Integrity: 60, MaxIntegrity: 60, Battery: 60, MaxBattery: 60}
	w.Spawn(foe)
	em := p.FindComponent(CEmitter)
	evs = w.Advance(map[EntityID][]Intent{p.ID: {{Type: "activate", Component: em.ID, Target: foe.ID, Power: 400}}})
	if !hasEvent(evs, "attack") {
		t.Fatalf("counterattack did not land, events %+v", evs)
	}
	if foe.Integrity != 40 {
		t.Errorf("foe integrity = %d, want 40", foe.Integrity)
	}
}

// Frame/Battery 是身体之根，不可 equip。
func TestEquipRejectsFrameAndBattery(t *testing.T) {
	w, p := newBareAgentWorld()
	w.Materials["player"] = map[Essence]int{Metal: 5, Earth: 5}
	before := len(p.Components)
	for _, ct := range []ComponentType{CFrame, CBattery} {
		evs := w.Advance(map[EntityID][]Intent{p.ID: {{Type: "equip", Blueprint: []CompSpec{{Type: ct}}}}})
		if len(evs) == 0 || evs[0].Type != "intent_rejected" {
			t.Errorf("%s should be rejected, got %+v", ct, evs)
		}
	}
	if len(p.Components) != before {
		t.Errorf("components changed: %d → %d", before, len(p.Components))
	}
	if w.Materials["player"][Metal] != 5 || w.Materials["player"][Earth] != 5 {
		t.Errorf("materials consumed on rejection: %+v", w.Materials["player"])
	}
}

// 物料不足拒绝（all-or-nothing：两个组件同需 火×1 而库存只有 1 → 全拒）。
func TestEquipRejectsInsufficientMaterials(t *testing.T) {
	w, p := newBareAgentWorld()
	w.Materials["player"] = map[Essence]int{Fire: 1}
	evs := w.Advance(map[EntityID][]Intent{p.ID: {{Type: "equip", Blueprint: []CompSpec{{Type: CEmitter}, {Type: CEmitter}}}}})
	if len(evs) == 0 || evs[0].Type != "intent_rejected" {
		t.Fatalf("should be rejected, got %+v", evs)
	}
	if p.FindComponent(CEmitter) != nil {
		t.Errorf("component installed despite rejection")
	}
	if w.Materials["player"][Fire] != 1 {
		t.Errorf("fire = %d, want 1 (all-or-nothing)", w.Materials["player"][Fire])
	}
}

// 身体复杂度预算：组件总数 ≤8（含躯壳与储气）。
func TestEquipRejectsOverComplexityBudget(t *testing.T) {
	w, p := newBareAgentWorld()
	w.Materials["player"] = map[Essence]int{Water: 5, Earth: 5}
	for _, ct := range []ComponentType{CStorage, CCooling, CProcessor, CSensor, CStorage} { // 3 + 5 = 8
		p.AddComponent(ct, Earth)
	}
	evs := w.Advance(map[EntityID][]Intent{p.ID: {{Type: "equip", Blueprint: []CompSpec{{Type: CCooling}}}}})
	if len(evs) == 0 || evs[0].Type != "intent_rejected" {
		t.Errorf("9th component should be rejected, got %+v", evs)
	}
	if len(p.Components) != 8 {
		t.Errorf("components = %d, want 8", len(p.Components))
	}
	// 蓝图合计也计入预算：7 组件身体 + 双件蓝图 = 9 → 拒
	w2, q := newBareAgentWorld()
	w2.Materials["player"] = map[Essence]int{Water: 5, Earth: 5}
	for _, ct := range []ComponentType{CStorage, CCooling, CProcessor, CSensor} { // 3 + 4 = 7
		q.AddComponent(ct, Earth)
	}
	evs = w2.Advance(map[EntityID][]Intent{q.ID: {{Type: "equip", Blueprint: []CompSpec{{Type: CCooling}, {Type: CStorage}}}}})
	if len(evs) == 0 || evs[0].Type != "intent_rejected" {
		t.Errorf("7+2=9 should be rejected, got %+v", evs)
	}
}

// 装第二个感识不叠加感知半径（v0.1 单组件语义）：半径仍 4 → 61 格。
func TestEquipSecondSensorDoesNotStackPerception(t *testing.T) {
	w, p := newTestWorld() // 自带一个感识
	w.Materials["player"] = map[Essence]int{Water: 1}
	evs := w.Advance(map[EntityID][]Intent{p.ID: {{Type: "equip", Blueprint: []CompSpec{{Type: CSensor}}}}})
	if len(evs) == 0 || evs[0].Type != "equip" {
		t.Fatalf("second sensor should install, got %+v", evs)
	}
	sensors := 0
	for _, c := range p.Components {
		if c.Type == CSensor {
			sensors++
		}
	}
	if sensors != 2 {
		t.Fatalf("sensors = %d, want 2", sensors)
	}
	s := p.FindComponent(CSensor)
	w.Advance(map[EntityID][]Intent{p.ID: {{Type: "activate", Component: s.ID}}})
	obs := w.Observe(p.ID, nil)
	if len(obs.Visible) != 61 { // radius 4 → 3·16+12+1
		t.Errorf("visible = %d, want 61 (半径不叠加)", len(obs.Visible))
	}
}
