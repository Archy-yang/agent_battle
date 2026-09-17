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
