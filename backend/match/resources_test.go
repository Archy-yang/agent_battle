package match

import (
	"fmt"
	"strings"
	"testing"

	"agentbattle/backend/engine"
	"agentbattle/backend/scenario"
)

// —— v0.5 WPA 经济循环：资源晶簇（采集到物料的供给线）——
//
// 覆盖四条验收线：生成确定性（同 seed 同序列 + 开局保底）、采集流程
// （走到晶簇格 → transfer → 物料 +1、实体消失、事件 gather）、拾取进
// 触发分类（gather 经 pickup 镜像进 resource 通道，与 pickupSeen 协同）。

// resSeed is the fixed seed of the resource work-package tests.
const resSeed = uint64(20260919)

// idlePilot is the stand-by pilot stub for gather tests: TickAuto 的分类
// 器路径照走（触发、镜像消费、llmCalls 计数），但 pilot 零决策、零订
// 单——玩家状态完全由测试白盒控制。
type idlePilot struct{}

func (idlePilot) Decide(*engine.Observation) []engine.Intent {
	return nil
}

func (idlePilot) DecideOrder(*engine.Observation) []engine.Intent {
	return nil
}

// banishHostiles teleports every hostile far off-field (白盒：隔离遭遇/
// 战斗触发，让采集语义独占断言).
func banishHostiles(t *testing.T, m *Match) {
	t.Helper()
	for _, id := range m.hostiles() {
		if e := m.W.Entities[id]; e != nil {
			e.Pos = engine.Hex{Q: 400 + int(id), R: 400}
		}
	}
}

// findResource returns the first crystal entity in the world.
func findResource(m *Match) *engine.Entity {
	for _, e := range m.W.Entities {
		if e.Kind == "resource" {
			return e
		}
	}
	return nil
}

// testDominantEssence mirrors scenario 包内的 chunk 主导行计算（包外测
// 试无法直接调用未导出函数）：mode of the chunk's 144 cell essences.
func testDominantEssence(seed uint64, qc, rc int) engine.Essence {
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

// 晶簇生成确定性：同 seed 同序列、逐位可重放（design §2.5 纯函数）；
// essence 恒等于该 chunk 主导行（"看地貌知资源"）。
func TestResourceSpawnsDeterministic(t *testing.T) {
	run := func() string {
		var b strings.Builder
		for qc := -6; qc <= 6; qc++ {
			for rc := -6; rc <= 6; rc++ {
				if rs := scenario.ResourceSpawnsFor(resSeed, qc, rc); rs != nil {
					fmt.Fprintf(&b, "(%d,%d):%s\n", qc, rc, rs.Essence)
				}
			}
		}
		return b.String()
	}
	a, b := run(), run()
	if a != b {
		t.Fatalf("resource spawn sequence differs between runs:\n%s\n%s", a, b)
	}
	if a == "" {
		t.Fatal("no crystals in a 13×13 chunk sweep (probabilities too low?)")
	}
	for qc := -6; qc <= 6; qc++ {
		for rc := -6; rc <= 6; rc++ {
			if rs := scenario.ResourceSpawnsFor(resSeed, qc, rc); rs != nil {
				if want := testDominantEssence(resSeed, qc, rc); rs.Essence != want {
					t.Errorf("chunk (%d,%d): crystal essence = %s, dominant row = %s",
						qc, rc, rs.Essence, want)
				}
			}
		}
	}
}

// 开局保底：每个 seed 的出生 3×3 chunk 至少 1 簇；概率曲线抽检——晶
// 簇出簇率必须高于怪物出怪率（采集是常规事件）。
func TestResourceBirthGuaranteeAndRate(t *testing.T) {
	seeds := []uint64{1, 2, 3, 7, 42, 99, 12345, 20260917, 20260918, resSeed}
	for _, seed := range seeds {
		n := 0
		for qc := -1; qc <= 1; qc++ {
			for rc := -1; rc <= 1; rc++ {
				if scenario.ResourceSpawnsFor(seed, qc, rc) != nil {
					n++
				}
			}
		}
		if n < 1 {
			t.Fatalf("seed %d: birth 3×3 has %d crystals, want ≥ 1", seed, n)
		}
	}
	cristals, mobs := 0, 0
	for qc := -30; qc <= 30; qc++ {
		for rc := -30; rc <= 30; rc++ {
			if scenario.ResourceSpawnsFor(resSeed, qc, rc) != nil {
				cristals++
			}
			if scenario.MonsterSpawnsFor(resSeed, qc, rc) != nil {
				mobs++
			}
		}
	}
	if cristals <= mobs {
		t.Fatalf("crystal rate (%d) must exceed mob rate (%d) over 61×61 chunks", cristals, mobs)
	}
	t.Logf("出生 3×3 保底：10 个 seed 全部 ≥1 簇；61×61 chunk 实测：出簇 %d vs 出怪 %d（%.1f%% vs %.1f%%）",
		cristals, mobs, 100*float64(cristals)/61/61, 100*float64(mobs)/61/61)
}

// 采集流程：晶簇不阻挡移动 → 走上晶簇格 → transfer → 物料 +1、实体消
// 失、事件 gather（文本"采集 X×1，仓库存料 N"）；晶簇 transfer 不得泄
// 漏进引擎（否则会被"只能搬取元气结晶"拒绝）。
func TestGatherCrystalFlow(t *testing.T) {
	m := NewV04("gather", resSeed, nil)
	banishHostiles(t, m)
	m.Tick(nil) // 首 tick：出生 3×3 chunk 懒激活，晶簇随怪物入场
	if findResource(m) == nil {
		t.Fatal("no crystal activated in the birth circle")
	}
	// 激活事件："spawn" 文本区分晶簇与怪物
	crystalSpawn := false
	for _, ev := range m.Events {
		if ev.Type == "spawn" && strings.Contains(ev.Text, "晶簇") {
			crystalSpawn = true
		}
	}
	if !crystalSpawn {
		t.Fatalf("no crystal spawn event in %v", m.Events)
	}

	res := findResource(m)
	p := m.W.Entities[m.PlayerID]
	before := m.W.Materials["player"][res.Essence]
	// 传送到邻格，再真实 move 一步踏上晶簇格——move 被接受即证明晶簇
	// 不阻挡移动（EntityAt 对 Kind "resource" 透明）
	adj := engine.HexNeighbors(res.Pos)[0]
	p.Pos = adj
	m.Tick([]engine.Intent{{Type: "move", To: &res.Pos, Power: 400}})
	if p.Pos != res.Pos {
		t.Fatalf("move onto the crystal cell was blocked: player=%v crystal=%v lastEvs=%v",
			p.Pos, res.Pos, m.lastEvs)
	}
	m.Tick([]engine.Intent{{Type: "transfer", Target: res.ID}})
	if m.W.Entities[res.ID] != nil {
		t.Fatal("crystal still in the world after gathering")
	}
	if got := m.W.Materials["player"][res.Essence]; got != before+1 {
		t.Fatalf("materials[%s] = %d, want %d", res.Essence, got, before+1)
	}
	var gather *engine.Event
	for i := range m.lastEvs {
		if m.lastEvs[i].Type == "gather" && m.lastEvs[i].Entity == p.ID {
			gather = &m.lastEvs[i]
		}
	}
	if gather == nil {
		t.Fatalf("no gather event, got %v", m.lastEvs)
	}
	if want := fmt.Sprintf("采集 %s×1，仓库存料 %d", res.Essence, before+1); gather.Text != want {
		t.Fatalf("gather text = %q, want %q", gather.Text, want)
	}
	for _, ev := range m.lastEvs {
		if ev.Type == "intent_rejected" {
			t.Fatalf("gather leaked into the engine and was rejected: %+v", ev)
		}
	}
}

// 不站在晶簇格上的 transfer：被拒且不入库。
func TestGatherRequiresSameCell(t *testing.T) {
	m := NewV04("gather-far", resSeed, nil)
	banishHostiles(t, m)
	m.Tick(nil)
	res := findResource(m)
	if res == nil {
		t.Fatal("no crystal activated")
	}
	p := m.W.Entities[m.PlayerID]
	p.Pos = engine.HexNeighbors(res.Pos)[0] // 邻格而非同格
	before := m.W.Materials["player"][res.Essence]
	m.Tick([]engine.Intent{{Type: "transfer", Target: res.ID}})
	if m.W.Entities[res.ID] == nil {
		t.Fatal("crystal should survive a same-cell-missed gather")
	}
	if got := m.W.Materials["player"][res.Essence]; got != before {
		t.Fatalf("materials changed on rejected gather: %d → %d", before, got)
	}
	rejected := false
	for _, ev := range m.lastEvs {
		if ev.Type == "intent_rejected" && strings.Contains(ev.Text, "晶簇") {
			rejected = true
		}
	}
	if !rejected {
		t.Fatalf("expected a rejection event, got %v", m.lastEvs)
	}
}

// 首次采集进触发分类：gather 经 pickup 镜像进 resource 通道触发一次
// （与 pickupSeen 协同）；晶簇懒激活的 spawn 事件同走 resource 通道。
func TestGatherTriggersResourceChannel(t *testing.T) {
	m := NewV04("gather-trig", resSeed, nil)
	m.Pilot = idlePilot{}
	banishHostiles(t, m)
	m.TickAuto() // tick0 心跳唤醒 → tick1 懒激活（晶簇 spawn 事件）
	m.TickAuto() // 分类 tick1：spawn 事件 → resource 触发
	if len(m.TriggerLog) == 0 || m.TriggerLog[len(m.TriggerLog)-1] != TriggerResource {
		t.Fatalf("crystal lazy-activation should trigger resource, log=%v", m.TriggerLog)
	}
	res := findResource(m)
	if res == nil {
		t.Fatal("no crystal activated")
	}
	p := m.W.Entities[m.PlayerID]
	p.Pos = engine.HexNeighbors(res.Pos)[0]
	m.Tick([]engine.Intent{{Type: "move", To: &res.Pos, Power: 400}})
	m.Tick([]engine.Intent{{Type: "transfer", Target: res.ID}})
	m.TickAuto() // 分类消费 gather（镜像 pickup）→ resource 再次触发
	if m.TriggerLog[len(m.TriggerLog)-1] != TriggerResource {
		t.Fatalf("first gather should trigger the resource channel, log=%v", m.TriggerLog)
	}
	n := len(m.TriggerLog)
	m.TickAuto() // pickupSeen 已消费：采集不得再触发
	if len(m.TriggerLog) != n {
		t.Fatalf("gather must trigger once (pickupSeen), log=%v", m.TriggerLog)
	}
}
