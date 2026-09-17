package engine_test

// WP5 重放门禁（design §2.5 + 评审 §7 补充要求：无限世界下同 seed+Intent
// 流跨 chunk 行走的重放 hash 一致性用例为发布门禁）。
//
// 本文件用外部测试包（engine_test）驱动真实 scenario.V04 场景：
// 写死的 Intent 序列（move_to 路标订单）驾驶玩家一局 60 回合——向东北
// 走 10 步、再向西南折返、再向 Fragment 方向 8 步；怪物懒激活按 match
// 接入点约定（scenario.PendingSpawns 纯函数 + 调用方持 activated 集合 →
// World.Spawn + 事件并入事件流）在测试内接线——测试自身就是"调用方"，
// 与 match.activatedChunks 同构（WP5 评审修复：激活状态不再挂 *World
// 指针全局注册表，杜绝 GC 地址复用导致的跨对局串局）。

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"runtime"
	"sort"
	"strings"
	"testing"

	"agentbattle/backend/engine"
	"agentbattle/backend/scenario"
)

const replaySeed = uint64(31337)

func sha256hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

func chunkKeyOf(h engine.Hex) [2]int {
	return [2]int{floorDiv(h.Q, engine.ChunkSize), floorDiv(h.R, engine.ChunkSize)}
}

// eightStepsTowardFragment computes the third waypoint: 从出生点向
// Fragment 贪心 8 步（HexStepToward 每步必缩距，纯函数、跨运行一致）.
func eightStepsTowardFragment(seed uint64) engine.Hex {
	cur := engine.Hex{}
	for i := 0; i < 8; i++ {
		cur = engine.HexStepToward(cur, scenario.V04(seed).FragmentPos)[0]
	}
	return cur
}

// replayRun is one full run's observable record.
type replayRun struct {
	eventLog     string // 全部事件流（含懒激活 spawn 事件），逐字节
	stateHash    string // 终态：所有实体 id/kind/owner/pos/形/储/热/携带
	chunkHash    string // 所有已生成 chunk 的 144 格内容 hash
	activations  string // 懒激活序列（chunk/essence/form/落点）
	chunkGens    int    // 本局 chunk 生成次数（去重后的 chunk 数）
	playerChunks int    // 玩家到访过的 chunk 数（跨 chunk 证据）
	endPos       engine.Hex
}

// replayGame is one driven V04 run: 每局一个实例，activation 判定状态
// activated 由本结构持有（"调用方持状态"，design §2.5）。
//
// 路标路线（评审 §7：跨 chunk 行走）：
//   - 东北（{1,-1}）× 10 步 → (10,-10)
//   - 西南折返回出生点 (0,0)
//   - 向 Fragment 方向贪心 8 步
type replayGame struct {
	pack         *scenario.Pack
	w            *engine.World
	p            *engine.Entity
	waypoints    []engine.Hex
	wp           int
	activated    map[[2]int]bool
	genChunks    map[[2]int]bool
	playerChunks map[[2]int]bool
	evLog        strings.Builder
	acts         []string
}

// startReplayGame builds one game. chunk 生成计数缝：包装 TerrainFn——
// 每个 chunk 首次生成时其 144 格逐格穿过这里，map 去重后即本局的
// chunk 生成集合（LRU=256 > 本局生成数，无淘汰重算，计数即生成次数）.
func startReplayGame(seed uint64) *replayGame {
	pack := scenario.V04(seed)
	genChunks := map[[2]int]bool{}
	orig := pack.TerrainFn
	pack.TerrainFn = func(s uint64, q, r int) engine.Cell {
		genChunks[chunkKeyOf(engine.Hex{Q: q, R: r})] = true
		return orig(s, q, r)
	}
	w, pid, _, _, _ := pack.Genesis()
	p := w.Entities[pid]
	// 一局全程 28 步 × EnergyCost(400)=4 = 112 > 出厂储气 100——测试夹具
	// 补足储气（各局一致，不影响决定性），保证三段路标都能走完.
	p.Battery, p.MaxBattery = 300, 300
	return &replayGame{
		pack:         pack,
		w:            w,
		p:            p,
		waypoints:    []engine.Hex{{10, -10}, {0, 0}, eightStepsTowardFragment(seed)},
		activated:    map[[2]int]bool{}, // 调用方持状态
		genChunks:    genChunks,
		playerChunks: map[[2]int]bool{},
	}
}

// step advances one turn: 写死路标 Intent（订单在途时引擎自动步进），
// 推进引擎，再按 match 接入点协议做怪物懒激活（spawn 事件并入本回合
// 事件流，覆盖到的 chunk 键记入 activated——无论是否出怪）.
func (g *replayGame) step() {
	var intents map[engine.EntityID][]engine.Intent
	if g.p.OrderTo == nil {
		if g.wp < len(g.waypoints) && g.p.Pos == g.waypoints[g.wp] {
			g.wp++
		}
		if g.wp < len(g.waypoints) {
			to := g.waypoints[g.wp]
			intents = map[engine.EntityID][]engine.Intent{g.p.ID: {{Type: "move_to", To: &to}}}
		}
	}
	for _, e := range g.w.Advance(intents) {
		fmt.Fprintf(&g.evLog, "[%d]%s#%d:%s\n", e.Tick, e.Type, e.Entity, e.Text)
	}
	g.playerChunks[chunkKeyOf(g.p.Pos)] = true
	for _, cs := range scenario.PendingSpawns(g.w, g.p.Pos, g.activated) {
		g.activated[[2]int{cs.QC, cs.RC}] = true
		if cs.Spawn == nil {
			continue // 覆盖到位但该 chunk 无怪
		}
		m := g.w.Spawn(cs.Spawn.Entity)
		ev := cs.Spawn.Event
		ev.Entity = m.ID // 实体 id 在 Spawn 后才存在
		fmt.Fprintf(&g.evLog, "[%d]%s#%d:%s\n", ev.Tick, ev.Type, ev.Entity, ev.Text)
		g.acts = append(g.acts, fmt.Sprintf("tick%d:(%d,%d):%s:form%d@(%d,%d)",
			ev.Tick, floorDiv(m.Pos.Q, 12), floorDiv(m.Pos.R, 12), m.Essence, m.Integrity, m.Pos.Q, m.Pos.R))
	}
}

// finish snapshots the run's observable record. 所有已生成 chunk 的缓存
// 内容 hash：chunk 集合即 genChunks（都在缓存中，无淘汰），逐 chunk
// 144 格 dump essence/danger/passable.
func (g *replayGame) finish() replayRun {
	var sb strings.Builder
	for _, e := range g.w.SortedEntities() {
		fmt.Fprintf(&sb, "#%d %s %s %v 形%d 储%d 热%d 携%d\n",
			e.ID, e.Kind, e.Owner, e.Pos, e.Integrity, e.Battery, e.Heat, len(e.Carrying))
	}
	chunkGens := len(g.genChunks)
	keys := make([][2]int, 0, len(g.genChunks))
	for k := range g.genChunks {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i][0] != keys[j][0] {
			return keys[i][0] < keys[j][0]
		}
		return keys[i][1] < keys[j][1]
	})
	var cb strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&cb, "chunk(%d,%d):", k[0], k[1])
		for j := 0; j < engine.ChunkSize; j++ {
			for i := 0; i < engine.ChunkSize; i++ {
				c := g.w.At(engine.Hex{Q: k[0]*engine.ChunkSize + i, R: k[1]*engine.ChunkSize + j})
				d, pa := 0, 0
				if c.Danger {
					d = 1
				}
				if c.Passable {
					pa = 1
				}
				fmt.Fprintf(&cb, "%s%d%d", c.Essence, d, pa)
			}
		}
	}
	return replayRun{
		eventLog:     g.evLog.String(),
		stateHash:    sha256hex(sb.String()),
		chunkHash:    sha256hex(cb.String()),
		activations:  strings.Join(g.acts, " → "),
		chunkGens:    chunkGens,
		playerChunks: len(g.playerChunks),
		endPos:       g.p.Pos,
	}
}

func runReplayN(seed uint64, turns int) replayRun {
	g := startReplayGame(seed)
	for turn := 0; turn < turns; turn++ {
		g.step()
	}
	return g.finish()
}

// TestV04ReplayGate is the发布门禁：同 seed + 同 Intent 流跑两次，全部
// 事件流 + 终态 + chunk 缓存内容 + 懒激活序列逐字节一致（评审 §7）.
func TestV04ReplayGate(t *testing.T) {
	a := runReplayN(replaySeed, 60)
	b := runReplayN(replaySeed, 60)

	if a.eventLog != b.eventLog {
		t.Fatalf("event stream mismatch:\n--- A ---\n%s\n--- B ---\n%s", a.eventLog, b.eventLog)
	}
	if a.stateHash != b.stateHash {
		t.Fatalf("final state hash mismatch: %s vs %s", a.stateHash, b.stateHash)
	}
	if a.chunkHash != b.chunkHash {
		t.Fatalf("chunk cache content hash mismatch: %s vs %s", a.chunkHash, b.chunkHash)
	}
	if a.activations != b.activations {
		t.Fatalf("lazy activation sequence mismatch:\n%s\n%s", a.activations, b.activations)
	}

	// 门禁有效性自检：行走确实跨 chunk 走完，且确实发生了懒激活
	wantEnd := eightStepsTowardFragment(replaySeed)
	if a.endPos != wantEnd {
		t.Fatalf("scripted walk did not complete: end %v, want %v (mob blocked the corridor?)", a.endPos, wantEnd)
	}
	if a.playerChunks < 2 {
		t.Fatalf("walk crossed %d chunks, want ≥2", a.playerChunks)
	}
	if !strings.Contains(a.eventLog, "order_done") || !strings.Contains(a.eventLog, "spawn") {
		t.Fatalf("event log missing order_done/spawn events")
	}
	if len(a.activations) == 0 {
		t.Fatal("no mob lazily activated along the walk — the gate would be vacuous")
	}
	if a.chunkGens >= 100 {
		t.Fatalf("chunk generations = %d, want < 100 (LRU pressure sanity)", a.chunkGens)
	}

	t.Logf("60 回合门禁通过：chunk 生成 %d 块（<100），跨 %d 个 chunk，终态 hash %s，chunk 缓存 hash %s",
		a.chunkGens, a.playerChunks, a.stateHash[:16], a.chunkHash[:16])
	t.Logf("懒激活序列：%s", a.activations)
}

// TestReplayLazyActivationNoCrossMatchPollution simulates the server
// opening many matches back-to-back: 5 个不同 seed 的 world 同时存活、
// 轮流推进 20 回合，再串行重跑并夹以 GC 压力（鼓励 World 指针地址复用
// ——旧的全局注册表实现在此形态下会把上一局的激活状态串进下一局）。
// 各局激活序列必须与独立串行参考逐位一致——互不污染.
func TestReplayLazyActivationNoCrossMatchPollution(t *testing.T) {
	seeds := []uint64{1, 42, 99, 12345, 31337}

	// 独立串行参考：每 seed 一局 20 回合
	ref := make([]string, len(seeds))
	mobs := 0
	for i, s := range seeds {
		ref[i] = runReplayN(s, 20).activations
		if ref[i] != "" {
			mobs++
		}
	}
	if mobs == 0 {
		t.Fatal("reference runs activated no mobs — pollution test would be vacuous")
	}

	// 形态一：5 个 world 同时存活、轮流推进（真实多局并发形态）
	games := make([]*replayGame, len(seeds))
	for i, s := range seeds {
		games[i] = startReplayGame(s)
	}
	interleaved := make([]strings.Builder, len(seeds))
	for turn := 0; turn < 20; turn++ {
		for i, g := range games {
			n := len(g.acts)
			g.step()
			for _, a := range g.acts[n:] { // 本 tick 的新激活全部记入该局序列
				if interleaved[i].Len() > 0 {
					interleaved[i].WriteString(" → ")
				}
				interleaved[i].WriteString(a)
			}
		}
	}
	for i := range games {
		if got := interleaved[i].String(); got != ref[i] {
			t.Fatalf("seed %d (interleaved): activation sequence polluted:\n got %q\nwant %q", seeds[i], got, ref[i])
		}
	}

	// 形态二：串行连续开多局 + 每局之间 GC（World 指针地址复用场景）
	for round := 0; round < 3; round++ {
		for i, s := range seeds {
			runtime.GC() // 鼓励新 world 落在旧 world 回收的地址上
			if got := runReplayN(s, 20).activations; got != ref[i] {
				t.Fatalf("seed %d (round %d, serial+GC): activation sequence polluted:\n got %q\nwant %q", s, round, got, ref[i])
			}
		}
	}
	t.Logf("5 seed × 20 回合：并发交错与串行+GC 复跑共 %d 局，激活序列与独立参考逐位一致，零串局",
		len(seeds)*(1+3)+len(seeds))
}

// chunk 生成不消耗对局 Rng 的直接证明（design §2.5）：在两次 Rng 消费
// 之间强行生成一大批 chunk（441 块 > LRU 容量，连淘汰一起压），Rng 序
// 列与"零 chunk 生成"的对照世界逐位一致.
func TestChunkGenDoesNotConsumeMatchRng(t *testing.T) {
	newWorld := func() *engine.World {
		w, _, _, _, _ := scenario.V04(replaySeed).Genesis()
		return w
	}
	wa := newWorld()
	a1 := wa.Rng.Next()
	genned := 0
	for q := -30; q <= 30; q += 3 { // 21×21 = 441 个不同 chunk
		for r := -30; r <= 30; r += 3 {
			wa.At(engine.Hex{Q: q * engine.ChunkSize, R: r * engine.ChunkSize})
			genned++
		}
	}
	a2 := wa.Rng.Next()

	wb := newWorld() // 对照：两次 Rng 消费之间不生成任何 chunk
	b1 := wb.Rng.Next()
	b2 := wb.Rng.Next()

	if a1 != b1 || a2 != b2 {
		t.Fatalf("chunk generation shifted the match Rng: with chunks (%d,%d) vs without (%d,%d) after %d chunks",
			a1, a2, b1, b2, genned)
	}
	t.Logf("chunk 生成 %d 块（含 LRU 淘汰）前后 Rng 序列与对照世界逐位一致", genned)
}
