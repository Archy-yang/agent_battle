package scenario_test

// balance_test.go —— V04（scenario/infinite-0.4）平衡性批量模拟。
//
// 驾驶：agent.ScriptedAgent 本体（agent/agent.go，走向 Fragment、拾取、
// 返回）逐 tick 出手，一字未改；本文件只加一层确定性"顺路采集"路点
// 调度（gatherHarness）：行走、拾取、返回全部仍由 ScriptedAgent 完成，
// harness 只补两处 ScriptedAgent 语义覆盖不到的地方——
//   1. 晶簇 transfer（引擎 resolveTransfer 只认 fragment，晶簇采集是
//      match 层扩展，ScriptedAgent 只会对 fragment 出 transfer）；
//   2. 踏上晶簇格的最后一步（ScriptedAgent.blocked 把 Kind "resource"
//      视为阻挡物，支线最后一格它会绕行——harness 代出这一步 move）。
//
// 不修改引擎、场景、Agent 的任何参数——产出的是统计数据与诊断建议
// （见 balance_report.md），调参决定留给总管。
//
// 运行口径：
//   go test ./...                    → 快速版 5 seed（常规回归不拖时间）
//   go test ./scenario -run Balance  → 全量 30 seed
//   go test -short ./scenario        → 强制快速版（即使 -run Balance）

import (
	"flag"
	"strings"
	"testing"

	"agentbattle/backend/agent"
	"agentbattle/backend/engine"
	"agentbattle/backend/match"
	"agentbattle/backend/scenario"
)

// tickEvents 返回本 tick 并入事件流的段落（m.Events 按 tick 追加，
// 外部测试包取不到未导出的 lastEvs，按 Tick 过滤等价）.
func tickEvents(m *match.Match) []engine.Event {
	var out []engine.Event
	for _, ev := range m.Events {
		if ev.Tick == m.W.Tick {
			out = append(out, ev)
		}
	}
	return out
}

// balanceSeed 返回第 i 局的 seed：固定步长数列，不挑 seed——前 30 项
// 均匀铺开（Fragment 12 向 × 距离 14~18 由 seed 哈希决定，交叉覆盖；
// 2026-09-19 平衡修订后口径）。
func balanceSeed(i int) uint64 { return 20260001 + uint64(i)*97 }

// balanceSeedCount decides the matrix size: -run Balance 点名本测试 →
// 全量 30；-short 或常规 `go test ./...` → 快速版 5.
func balanceSeedCount() int {
	if testing.Short() {
		return 5
	}
	if f := flag.Lookup("test.run"); f != nil && strings.Contains(f.Value.String(), "Balance") {
		return 30
	}
	return 5
}

// runStat 是一局（一个 seed）的统计快照.
type runStat struct {
	seed           uint64
	fragDist       int    // Fragment 与出生点的 HexDist
	status         string // won | lost | timeout
	ticks          int64
	form           int   // 终局形（形散记 0）
	battery        int   // 终局气
	heat           int   // 终局热
	fragmentTaken  bool  // 拾取（transfer fragment）成功
	gathers        int   // 晶簇采集次数（match 层 gather 事件）
	combatTicks    int   // InCombat 为真的 tick 数
	combatEpisodes int   // 连续 in_combat 段数（遭遇次数口径）
	combatDamage   int   // 战斗 tick 上的形损失总量
	envDamage      int   // 非战斗 tick 的形损失总量（过热等）
	lazyMobs       int   // 懒激活怪物数（spawn 事件"妖物"）
	crystalSpawns  int   // 懒激活晶簇数（spawn 事件"晶簇"）
	qiStarvedTick  int64 // 首个"储气不足"事件的 tick（0 = 从未）
	deadTick       int64 // 形散 tick（0 = 存活到终局）
}

// gatherHarness 是 ScriptedAgent 外面的确定性薄壳：路点调度 + 晶簇
// transfer/末步补位。ScriptedAgent 的任何字段与行为都不被改动。
type gatherHarness struct {
	spawn    engine.Hex
	fragment engine.Hex
	passable func(engine.Hex) bool
	m        *match.Match

	home    *agent.ScriptedAgent // 主线 pilot（Target=Fragment；携晶自动改道归途）
	legGoal engine.Hex           // 当前支线目标（晶簇位）；零值 = 主线
	leg     *agent.ScriptedAgent // 当前支线 pilot（Target=晶簇位）
	wpID    engine.EntityID      // 支线晶簇实体 id
	tried   map[engine.EntityID]bool
	lastPos engine.Hex
	stuck   int
}

func newGatherHarness(m *match.Match, pack *scenario.Pack) *gatherHarness {
	return &gatherHarness{
		spawn:    pack.PlayerSpawn,
		fragment: pack.FragmentPos,
		passable: pack.Passable,
		m:        m,
		home:     agent.NewScripted(pack.PlayerSpawn, pack.FragmentPos, pack.Passable),
		tried:    map[engine.EntityID]bool{},
	}
}

// Decide 每回合被咨询（m.Tick(h.Decide(obs)) 的逐 tick 口径）：
//  1. 携晶归途 → 原样交给 ScriptedAgent（Decide 自带"回出生点"分支）；
//  2. 站上晶簇 → 补一条 transfer（match 层结算采集）；
//  3. 支线末步（距晶簇 1）→ harness 代出踏上晶簇格的 move；
//  4. 支线其余路程 → 支线 pilot（仍是一个 ScriptedAgent，Target=晶簇位）；
//  5. 其余 → 主线 ScriptedAgent（走向 Fragment、transfer 拾取）。
//
// 卡死护栏：位置连续 5 tick 不动且在支线上 → 拉黑该晶簇重选（确定性）。
func (h *gatherHarness) Decide(obs *engine.Observation) []engine.Intent {
	if obs == nil {
		return nil
	}
	self := obs.Self
	if len(self.Carrying) > 0 {
		return h.home.Decide(obs) // 携晶归途：ScriptedAgent 原生分支
	}
	if self.Pos != h.lastPos {
		h.lastPos = self.Pos
		h.stuck = 0
	} else {
		h.stuck++
	}

	// 站上晶簇：补 transfer（match 层结算采集）
	if h.legGoal != (engine.Hex{}) && self.Pos == h.legGoal {
		for _, seen := range obs.Entities {
			if seen.Kind == "resource" && seen.Pos == self.Pos {
				h.tried[seen.ID] = true
				h.clearLeg()
				return []engine.Intent{{Type: "transfer", Target: seen.ID}}
			}
		}
		h.clearLeg() // 目标晶簇不在感知内（已消散等）：放弃支线
	}
	if h.stuck >= 5 && h.legGoal != (engine.Hex{}) {
		h.tried[h.wpID] = true // 支线卡死：拉黑，回到主线
		h.clearLeg()
		h.stuck = 0
	}

	if h.legGoal == (engine.Hex{}) { // 无支线：找最近的未采集晶簇
		if pos, id, ok := h.nearestCrystal(self.Pos); ok {
			h.legGoal, h.wpID = pos, id
			h.leg = agent.NewScripted(h.spawn, pos, h.passable)
		}
	}

	if h.legGoal != (engine.Hex{}) {
		if engine.HexDist(self.Pos, h.legGoal) == 1 {
			// 支线末步：ScriptedAgent.blocked 把晶簇当阻挡物会绕行，
			// harness 代出踏上晶簇格的这一步（被怪物占位则被拒，走卡死护栏）。
			to := h.legGoal
			return []engine.Intent{{Type: "move", To: &to, Power: 400}}
		}
		if h.leg != nil {
			if step := h.leg.Decide(obs); len(step) > 0 {
				return step // 支线行走：ScriptedAgent 出手（move，Power 400）
			}
			return nil // 被完全卡死（等卡死护栏拉黑）
		}
	}
	return h.home.Decide(obs) // 主线：走向 Fragment、拾取、（携晶后）返回
}

func (h *gatherHarness) clearLeg() {
	h.legGoal, h.wpID, h.leg = engine.Hex{}, 0, nil
}

// nearestCrystal 在已懒激活的晶簇实体里挑最近的未采集者（支线半径
// 5，v0.6 三轮由 10 下调：二轮数据里支线绕路是"携晶超时"的另一主
// 因，收窄后采集仍均值 ≥1 而时钟消耗大减）。
// 平局取实体 id 小者；map 遍历序无关（判定是全序的）。
func (h *gatherHarness) nearestCrystal(self engine.Hex) (engine.Hex, engine.EntityID, bool) {
	bestD, bestID, bestPos := 1<<30, engine.EntityID(0), engine.Hex{}
	found := false
	for _, e := range h.m.W.Entities {
		if e.Kind != "resource" || h.tried[e.ID] {
			continue
		}
		d := engine.HexDist(self, e.Pos)
		if d > 5 {
			continue
		}
		if d < bestD || (d == bestD && (!found || e.ID < bestID)) {
			found, bestD, bestID, bestPos = true, d, e.ID, e.Pos
		}
	}
	return bestPos, bestID, found
}

// runBalance plays one V04 match to completion and returns its stats.
func runBalance(t *testing.T, seed uint64) (st runStat) {
	st.seed = seed
	defer func() { // 无 panic 健康线：panic 转为明确的测试失败
		if r := recover(); r != nil {
			t.Fatalf("seed %d panic: %v", seed, r)
		}
	}()
	pack := scenario.V04(seed)
	m := match.NewV04("balance", seed, nil)
	// 强制 ScriptedAgent 驾驶（不随 AGENTBATTLE_LLM_* 环境漂移）
	h := newGatherHarness(m, pack)
	m.Pilot = h
	m.PilotName = "scripted"
	st.fragDist = engine.HexDist(pack.PlayerSpawn, pack.FragmentPos)

	inCombatEpi := false
	for m.Status == match.StatusRunning && m.W.Tick < m.TurnLimit {
		prev := 0
		if p := m.W.Entities[m.PlayerID]; p != nil {
			prev = p.Integrity
		}
		intents := h.Decide(m.Observe())
		m.Tick(intents)

		// —— 本 tick 统计 ——
		cur := 0
		if p := m.W.Entities[m.PlayerID]; p != nil {
			cur = p.Integrity
		}
		if loss := prev - cur; loss > 0 {
			if m.InCombat() {
				st.combatDamage += loss
			} else {
				st.envDamage += loss
			}
		}
		if m.InCombat() {
			st.combatTicks++
			if !inCombatEpi { // 新遭遇段
				st.combatEpisodes++
				inCombatEpi = true
			}
		} else {
			inCombatEpi = false
		}
		for _, ev := range tickEvents(m) {
			switch {
			case ev.Type == "gather":
				st.gathers++
			case ev.Type == "pickup":
				st.fragmentTaken = true
			case ev.Type == "spawn" && strings.Contains(ev.Text, "妖物"):
				st.lazyMobs++
			case ev.Type == "spawn" && strings.Contains(ev.Text, "晶簇"):
				st.crystalSpawns++
			case ev.Type == "intent_rejected" && ev.Entity == m.PlayerID && strings.Contains(ev.Text, "储气不足") && st.qiStarvedTick == 0:
				st.qiStarvedTick = m.W.Tick
			case ev.Type == "destroyed" && ev.Entity == m.PlayerID:
				st.deadTick = m.W.Tick
			}
		}
	}
	st.status = m.Status
	st.ticks = m.W.Tick
	if p := m.W.Entities[m.PlayerID]; p != nil {
		st.form, st.battery, st.heat = p.Integrity, p.Battery, p.Heat
	}
	return st
}

func TestBalanceMatrixV04(t *testing.T) {
	n := balanceSeedCount()
	stats := make([]runStat, 0, n)
	for i := 0; i < n; i++ {
		seed := balanceSeed(i)
		st := runBalance(t, seed)
		stats = append(stats, st)
		t.Logf("seed=%-9d dist=%2d %-7s t=%2d 形=%3d 气=%3d 热=%2d 采=%d 拾=%v 遭遇=%d段/%2dtick 形损(战/环)=%3d/%d 懒怪=%2d 晶=%d 断气@t=%d 卒@t=%d",
			st.seed, st.fragDist, st.status, st.ticks, st.form, st.battery, st.heat,
			st.gathers, st.fragmentTaken, st.combatEpisodes, st.combatTicks,
			st.combatDamage, st.envDamage, st.lazyMobs, st.crystalSpawns,
			st.qiStarvedTick, st.deadTick)
	}

	// —— 汇总 ——
	var wins, losses, timeouts, taken float64
	var sumTicks, sumCombat, sumEpi, sumGathers, sumDmg, sumCombatDmg, sumLazy, sumEnvDmg float64
	for _, s := range stats {
		switch s.status {
		case match.StatusWon:
			wins++
		case match.StatusLost:
			losses++
		default:
			timeouts++
		}
		if s.fragmentTaken {
			taken++
		}
		sumTicks += float64(s.ticks)
		sumCombat += float64(s.combatTicks)
		sumEpi += float64(s.combatEpisodes)
		sumGathers += float64(s.gathers)
		sumDmg += float64(s.combatDamage + s.envDamage)
		sumCombatDmg += float64(s.combatDamage)
		sumEnvDmg += float64(s.envDamage)
		sumLazy += float64(s.lazyMobs)
	}
	f := float64(len(stats))
	t.Logf("汇总（%d 局）：胜 %.0f%% 负 %.0f%% 超时 %.0f%% | 拾取Fragment %.0f%% | 平均存活 %.1f 回合 | 平均采集 %.2f | 遭遇 %.2f 段/局、%.2f tick/局 | 平均形损 %.1f（战斗 %.1f 环境 %.1f） | 平均懒激活怪 %.1f",
		len(stats), 100*wins/f, 100*losses/f, 100*timeouts/f, 100*taken/f,
		sumTicks/f, sumGathers/f, sumEpi/f, sumCombat/f,
		sumDmg/f, sumCombatDmg/f, sumEnvDmg/f, sumLazy/f)

	// —— 基本健康线（world_genesis §7 决定性 + 可玩性下限）——
	for _, s := range stats {
		if s.status != match.StatusWon && s.status != match.StatusLost && s.status != match.StatusTimeout {
			t.Fatalf("seed %d: 非法终局状态 %q", s.seed, s.status)
		}
		if s.ticks > 60 {
			t.Fatalf("seed %d: 超出 60 回合上限（t=%d）", s.seed, s.ticks)
		}
		if s.status == match.StatusLost && s.ticks <= 5 {
			t.Fatalf("seed %d: 第 %d 回合即死——出生 3×3 懒激活/出生环怪物合围过快", s.seed, s.ticks)
		}
	}
	// 采集均值健康线：v0.6 三轮把支线半径 10 → 5（时钟消耗大减，拾取
	// 率 20% → 53%），可达晶簇变少，阈值随之 1 → 0.5——晶簇经济仍须
	// 与 ScriptedAgent 路径连通，不得归零。
	if mean := sumGathers / f; mean < 0.5 {
		t.Errorf("采集均值 %.2f < 0.5——晶簇经济与 ScriptedAgent 路径脱节", mean)
	}
}
