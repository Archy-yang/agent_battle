package match

// rival_test.go —— v0.9 敌对 Agent（rival）验收测试 (docs/world_genesis.md
// §5.2 敌人三铁律 + Agent Protocol 对称性)。
//
// 覆盖：
//   - V05 场景包：rival 存在、出生对角（HexDist 10~14、与 Fragment 相背）、
//     五行克制（金 rival vs 木玩家）、形 100、五组件齐全（Emitter 火 +
//     Sensor + Motor + Frame + Battery）；
//   - Scripted rival 侵略性：逐回合接近玩家、相邻即攻击（事件流 attack，
//     power 500）、玩家掉形；rival 意图逐帧进录像；
//   - 触发独立：rival 专属 EventClassifier——玩家分类器静音后 rival 仍被
//     自己的事件唤醒并调度（互不污染）；
//   - 撤退：形 <40% 回出生锚点，到位驻足；
//   - 重放一致性：含 rival 的 60 回合两次运行逐字节一致；录像（含
//     rival_intents）经 JSON 往返后精确重演；篡改 rival 意图 →
//     ErrReplayMismatch（重放不重跑 rival pilot 的在线自证）。
//
// 驾驶口径：V05 真实包（侵略性/重放）+ 受控包（PlainTerrain + 单只
// rival，触发独立/撤退白盒）。玩家 pilot 一律显式 scripted/idle，隔离
// AGENTBATTLE_LLM_* 环境漂移（rival 恒为 ScriptedRival，与环境无关）。

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"agentbattle/backend/agent"
	"agentbattle/backend/engine"
	"agentbattle/backend/scenario"
)

// banishOthers teleports every hostile except the rival far away（受控
// 变数：只留 rival 在场上）.
func banishOthers(t *testing.T, m *Match) {
	t.Helper()
	for _, id := range m.MonsterIDs {
		if id == m.RivalID {
			continue
		}
		if e := m.W.Entities[id]; e != nil {
			e.Pos = engine.Hex{Q: 400 + int(id), R: 400}
		}
	}
}

// rivalPack builds a controlled one-rival pack: 全平地、玩家 (0,0)、
// Fragment/Unknown 远置，rival 手摆于 rivalPos。包名不取 infinite → 懒
// 激活不跑，场上敌对实体有且只有 rival（触发独立性测试的前提）。
func rivalPack(t *testing.T, seed uint64, rivalPos engine.Hex) *Match {
	t.Helper()
	pack := &scenario.Pack{
		Name: "test/rival-0.9", Seed: seed, TurnLimit: 60,
		TerrainFn:    engine.PlainTerrain,
		PlayerSpawn:  engine.Hex{},
		FragmentPos:  engine.Hex{Q: 0, R: 30},
		UnknownSpawn: engine.Hex{Q: 0, R: -30},
		MonsterSpawns: []scenario.MonsterSpec{
			{Essence: engine.Metal, Form: 100, Pos: rivalPos, Temper: scenario.TemperRival},
		},
	}
	return newMatch("rival", seed, nil, pack)
}

// —— V05 场景包 ——

// TestV05RivalGenesis: rival 存在、位置对角（HexDist 10~14、与 Fragment
// 相背）、五行克制（金 rival vs 木玩家）、形 100、五组件齐全。
func TestV05RivalGenesis(t *testing.T) {
	const seed = uint64(20260917)
	m := NewV05("v05", seed, nil)
	if m.PackName != "scenario/infinite-0.5" || m.TurnLimit != 60 {
		t.Fatalf("pack = %s limit = %d, want scenario/infinite-0.5 / 60", m.PackName, m.TurnLimit)
	}
	if m.RivalID == 0 {
		t.Fatal("V05 has no rival")
	}
	found := false
	for _, id := range m.MonsterIDs {
		if id == m.RivalID {
			found = true
		}
	}
	if len(m.MonsterIDs) != 4 || !found {
		t.Fatalf("MonsterIDs = %v, want 4 entries containing the rival", m.MonsterIDs)
	}
	rival := m.W.Entities[m.RivalID]
	if rival == nil {
		t.Fatal("rival entity missing")
	}
	// 五行错开：玩家木、rival 金——金克木（world_laws §5 相克表）。
	p := m.W.Entities[m.PlayerID]
	if p.Essence != engine.Wood || rival.Essence != engine.Metal {
		t.Fatalf("essences player=%s rival=%s, want 木/金", p.Essence, rival.Essence)
	}
	if !engine.Restrains(rival.Essence, p.Essence) {
		t.Fatal("金 does not restrain 木 — restraint tension broken")
	}
	// 形 100、五组件：Frame+Battery+Motor+Emitter（火）+Sensor。
	if rival.Integrity != 100 || rival.MaxIntegrity != 100 {
		t.Fatalf("rival form = %d/%d, want 100/100", rival.Integrity, rival.MaxIntegrity)
	}
	for _, ct := range []engine.ComponentType{engine.CFrame, engine.CBattery, engine.CMotor, engine.CEmitter, engine.CSensor} {
		if rival.FindComponent(ct) == nil {
			t.Fatalf("rival missing component %s", ct)
		}
	}
	if em := rival.FindComponent(engine.CEmitter); em.Essence != engine.Fire {
		t.Fatalf("rival emitter essence = %s, want 火", em.Essence)
	}
	// 出生：玩家对角 HexDist 10~14，与 Fragment（14~18）相背。
	d := engine.HexDist(rival.Pos, p.Pos)
	if d < 10 || d > 14 {
		t.Fatalf("rival spawn dist = %d, want [10,14]", d)
	}
	if fd := engine.HexDist(rival.Pos, m.W.Entities[m.FragmentID].Pos); fd < 20 {
		t.Fatalf("rival not on the antipodal diagonal: dist to fragment = %d, want ≥ 20", fd)
	}
	if !m.W.At(rival.Pos).Passable || rival.Pos == p.Pos {
		t.Fatalf("rival spawn %v impassable or on the player", rival.Pos)
	}
	// Registry 按名可选（api 契约）。
	if got := NewRegistry(nil).Create(seed, "v05"); got == nil || got.PackName != "scenario/infinite-0.5" || got.RivalID == 0 {
		t.Fatalf("registry v05 = %+v", got)
	}
	t.Logf("V05 seed %d：rival 金甲 @%v（dist %d），Fragment @%v——对角相背，金克木",
		seed, rival.Pos, d, m.W.Entities[m.FragmentID].Pos)
}

// —— Scripted rival 侵略性 ——

// TestV05ScriptedRivalAggression: 玩家驻足，scripted rival 逐回合接近
// （HexDist 单调不增、直达相邻）、相邻即攻击（事件流 attack，power
// 500）、玩家掉形；rival 意图逐帧进录像。
func TestV05ScriptedRivalAggression(t *testing.T) {
	m := NewV05("aggr", 7, nil)
	banishOthers(t, m) // 受控：场上只有 rival（测试内夹具，与重放无关）
	rival := m.W.Entities[m.RivalID]
	p := m.W.Entities[m.PlayerID]
	last := engine.HexDist(p.Pos, rival.Pos)
	attacked := false
	for i := 0; i < 30 && !attacked; i++ {
		m.Tick(nil) // 玩家 idle：接近与攻击全由 rival 驱动
		if m.W.Entities[m.RivalID] == nil {
			t.Fatalf("tick %d: rival vanished", m.W.Tick)
		}
		d := engine.HexDist(p.Pos, rival.Pos)
		if d > 1 && d > last {
			t.Fatalf("tick %d: rival did not close in: %d → %d", m.W.Tick, last, d)
		}
		last = d
		for _, ev := range m.lastEvs {
			if ev.Type == "attack" && ev.Entity == m.RivalID {
				attacked = true
			}
		}
	}
	if !attacked {
		t.Fatalf("rival never attacked: player=%v rival=%v", p.Pos, rival.Pos)
	}
	if p.Integrity >= 100 {
		t.Fatal("player took no damage from the rival's attack")
	}
	// 相邻出招功率 500（意图白盒取自录像帧——scheduleRival 的产出）。
	power := 0
	for i := len(m.rec.Frames) - 1; i >= 0 && power == 0; i-- {
		for _, in := range m.rec.Frames[i].RivalIntents {
			if in.Type == "activate" && in.Target == m.PlayerID {
				power = in.Power
			}
		}
	}
	if power != rivalAttackPower {
		t.Fatalf("rival attack power = %d, want %d", power, rivalAttackPower)
	}
	// rival 意图逐帧进录像（v0.9 Frame.RivalIntents）。
	rivalFrames := 0
	for _, f := range m.rec.Frames {
		if len(f.RivalIntents) > 0 {
			rivalFrames++
		}
	}
	if rivalFrames == 0 {
		t.Fatal("no frame records rival intents — replay of the rival would be vacuous")
	}
	t.Logf("seed 7：rival 于第 %d tick 贴脸开火（power %d），玩家形 %d；%d 帧记录 rival 意图",
		m.W.Tick, power, p.Integrity, rivalFrames)
}

// —— 触发独立 ——

// TestRivalTriggerIndependence: 玩家分类器静音（心跳隔离 + idle pilot）
// 后，rival 仍被自己的事件唤醒并推进——两套 EventClassifier 状态互不
// 污染，rival 调度不依赖玩家触发。
func TestRivalTriggerIndependence(t *testing.T) {
	m := rivalPack(t, 42, engine.Hex{Q: 6, R: 0}) // dist 6：感知圈外开局
	m.Pilot = idlePilot{}
	m.cls.HeartbeatEvery = 1000 // 静音玩家心跳：无事件则玩家永不触发
	if m.RivalID == 0 || m.W.Entities[m.RivalID] == nil {
		t.Fatal("controlled pack has no rival")
	}
	rival := m.W.Entities[m.RivalID]
	p := m.W.Entities[m.PlayerID]
	start := engine.HexDist(p.Pos, rival.Pos)
	for engine.HexDist(p.Pos, rival.Pos) > 2 && m.W.Tick < 20 {
		m.TickAuto() // 玩家 idle（idlePilot 恒 nil）
	}
	if len(m.TriggerLog) != 0 {
		t.Fatalf("player trigger log = %v, want empty — rival-side activity polluted the player classifier", m.TriggerLog)
	}
	if len(m.rivalTriggerLog) == 0 {
		t.Fatal("rival was never woken by its own triggers")
	}
	if !containsTrigger(m.rivalTriggerLog, TriggerHeartbeat) {
		t.Fatalf("rival log %v missing the opening heartbeat", m.rivalTriggerLog)
	}
	if !containsTrigger(m.rivalTriggerLog, TriggerEncounter) {
		t.Fatalf("rival log %v missing encounter (sensor-on perception of the player)", m.rivalTriggerLog)
	}
	if d := engine.HexDist(p.Pos, rival.Pos); d >= start {
		t.Fatalf("rival did not close in on its own schedule: %d → %d", start, d)
	}
	t.Logf("触发独立：玩家 TriggerLog 空；rival 自触发 %v，从 dist %d 逼近到 %d",
		m.rivalTriggerLog, start, engine.HexDist(p.Pos, rival.Pos))
}

func containsTrigger(log []string, tr string) bool {
	for _, v := range log {
		if v == tr {
			return true
		}
	}
	return false
}

// —— 撤退 ——

// TestV05RivalRetreat: 形 <40% → 放弃进攻，回出生锚点；到位驻足（意图
// 为空，蓄息）。
func TestV05RivalRetreat(t *testing.T) {
	m := rivalPack(t, 42, engine.Hex{Q: 8, R: 0})
	m.Pilot = idlePilot{}
	rival := m.W.Entities[m.RivalID]
	home := rival.Pos                  // 出生锚点 = 手摆位（Spec.Pos）
	rival.Integrity = 39               // 39 < 40% × 100：触发撤退线
	rival.Pos = engine.Hex{Q: 4, R: 0} // 已离家 4 格：撤退有意义
	returned := false
	for i := 0; i < 12 && !returned; i++ {
		m.Tick(nil)
		if rival.Pos == home && len(m.rec.Frames[len(m.rec.Frames)-1].RivalIntents) == 0 {
			returned = true // 到位且无输出（驻足蓄息）
		}
	}
	if !returned {
		t.Fatalf("rival never stood down at home: pos=%v home=%v", rival.Pos, home)
	}
	// 撤退首帧白盒：订单朝锚点。
	if f := m.rec.Frames[0]; len(f.RivalIntents) == 0 || f.RivalIntents[0].Type != "move_to" || *f.RivalIntents[0].To != home {
		t.Fatalf("first retreat frame intents = %+v, want move_to %v", f.RivalIntents, home)
	}
	t.Logf("撤退：形 39/100 → 回锚点 %v 驻足", home)
}

// —— 重放一致性 ——

// intentsEqual compares intent slices（engine.Intent 含指针/切片字段，
// 不可 == 比较）.
func intentsEqual(a, b []engine.Intent) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !reflect.DeepEqual(a[i], b[i]) {
			return false
		}
	}
	return true
}

// TestV05ReplayWithRival: 含 rival 的 60 回合——两次运行（事件流 +
// rival 意图）逐字节一致；录像（含 rival_intents）经 JSON 往返后逐字段
// 重演一致；篡改一帧 rival 意图 → ErrReplayMismatch。
//
// seed 42 依据：1..200 升序扫描（scripted TickAuto、无任何夹具、玩家
// 原生形 100）中跑满 60 回合（timeout）且 rival 全程无伤的 seed——
// 重放窗口完整、世界形态零人工修饰（重演局由构造函数原样重建，任何
// 夹具都会破坏逐字段一致性，故不用）。
func TestV05ReplayWithRival(t *testing.T) {
	run := func() *Recording {
		pack := scenario.V05(42)
		m := NewV05("replay", 42, nil)
		m.Pilot = agent.NewScripted(pack.PlayerSpawn, pack.FragmentPos, pack.Passable) // 隔离环境 LLM
		rec, wantState, wantEvents := runAndSnapshot(t, m, 60, map[int64]bool{30: true, 60: true})
		if len(rec.Frames) != 60 {
			t.Fatalf("recorded %d frames, want 60 (status %s)", len(rec.Frames), m.Status)
		}
		rivalFrames := 0
		for _, f := range rec.Frames {
			if len(f.RivalIntents) > 0 {
				rivalFrames++
			}
		}
		if rivalFrames == 0 {
			t.Fatal("no recorded rival intents — determinism gate would be vacuous")
		}
		// 重演：30/60 两个采样点逐字段一致。
		for _, until := range []int64{30, 60} {
			assertReplayMatches(t, rec, until, wantState, wantEvents)
		}
		return rec
	}

	rec := run()

	// JSON 往返（GET recording 的传输形态）后仍精确重演.
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal recording: %v", err)
	}
	var back Recording
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal recording: %v", err)
	}

	// 两次独立运行逐字节一致（事件流 + rival 意图帧）。
	rec2 := run()
	for i := range rec.Frames {
		a, b := rec.Frames[i], rec2.Frames[i]
		if !slicesEqualEvents(a.ResultingEvents, b.ResultingEvents) || !intentsEqual(a.RivalIntents, b.RivalIntents) {
			t.Fatalf("tick %d diverges between runs:\nA ev=%v rival=%v\nB ev=%v rival=%v",
				a.Tick, a.ResultingEvents, a.RivalIntents, b.ResultingEvents, b.RivalIntents)
		}
	}

	// 篡改一帧 rival 意图 → 重演分歧在线捕获（rival 用记录意图、不被重跑）。
	tampered := 0
	for i := range back.Frames {
		if len(back.Frames[i].RivalIntents) > 0 {
			to := engine.Hex{Q: 60 + i, R: -60}
			back.Frames[i].RivalIntents = []engine.Intent{{Type: "move_to", To: &to}}
			tampered = i + 1
			break
		}
	}
	if tampered == 0 {
		t.Fatal("no rival intent frame to tamper")
	}
	if _, err := Replay(&back, 60); !errors.Is(err, ErrReplayMismatch) {
		t.Fatalf("tampered rival intent in frame %d: got %v, want ErrReplayMismatch", tampered, err)
	}
	t.Logf("60 帧含 rival 录像：两次运行逐字节一致；JSON 往返（%d 字节）后 30/60 逐字段重演一致；篡改第 %d 帧 rival 意图被在线捕获",
		len(data), tampered)
}

func slicesEqualEvents(a, b []engine.Event) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// —— 全量不回退抽查 ——

// TestV04HasNoRival: V04 无 rival——rival 通道整局闲置（balance/recording
// 健康线不受 v0.9 rival 影响；懒怪战斗化的 V04 回归由 recording_test 门禁）。
func TestV04HasNoRival(t *testing.T) {
	m := NewV04("no-rival", 7, nil)
	if m.RivalID != 0 || m.RivalPilot != nil {
		t.Fatalf("V04 unexpectedly has a rival: id=%d pilot=%v", m.RivalID, m.RivalPilot)
	}
	m.Tick(nil)
	if len(m.rivalTriggerLog) != 0 {
		t.Fatalf("rival scheduler fired on a rival-less match: %v", m.rivalTriggerLog)
	}
}
