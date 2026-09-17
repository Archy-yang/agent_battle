package scenario

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"agentbattle/backend/engine"
)

// v04GoldenSeed is the golden-file seed (评审 §7：固定 seed 的 chunk 布
// 局快照进代码库作视觉回归).
const v04GoldenSeed = uint64(20260918)

// v04GoldenDump renders the 5×5 chunk essence matrix around the birth
// chunk (qc,rc ∈ [-2,2])：每格一字符（木火土金水，'*' = 顽石），行按
// r 升序、列按 q 升序，chunk 缝不换行——整体就是一张 60×60 矩阵.
func v04GoldenDump(seed uint64) string {
	p := V04(seed)
	var b strings.Builder
	fmt.Fprintf(&b, "# v0.4 golden: seed=%d, chunks qc,rc in [-2,2] (5×5)\n", seed)
	b.WriteString("# rows: r = -24..23, cols: q = -24..23; 木火土金水 = essence, * = 顽石\n")
	for rc := -2; rc <= 2; rc++ {
		for j := 0; j < engine.ChunkSize; j++ {
			r := rc*engine.ChunkSize + j
			for qc := -2; qc <= 2; qc++ {
				for i := 0; i < engine.ChunkSize; i++ {
					q := qc*engine.ChunkSize + i
					c := p.Terrain(engine.Hex{Q: q, R: r})
					if !c.Passable {
						b.WriteByte('*')
						continue
					}
					b.WriteString(c.Essence.String())
				}
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

var flagUpdateGolden = flag.Bool("update", false, "rewrite scenario/testdata/golden_v04.txt")

// golden file：固定 seed 的 5×5 chunk essence 矩阵与代码库内快照逐字
// 节一致，不一致即 FAIL（-update 重新生成）.
func TestV04GoldenFile(t *testing.T) {
	got := v04GoldenDump(v04GoldenSeed)
	path := "testdata/golden_v04.txt"
	if *flagUpdateGolden {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("golden updated (%d bytes)", len(got))
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v (run `go test ./scenario -run TestV04GoldenFile -update` to create)", err)
	}
	if got != string(want) {
		t.Fatalf("golden mismatch:\n got hash %s\nwant hash %s\n(run with -update after an intentional world-gen change)",
			v04Hash(got), v04Hash(string(want)))
	}
}

func v04Hash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// 跨进程确定性：子进程独立重算同一 dump，sha256 与本进程一致
// （world_genesis §7：同一 seed 跨进程逐位可重放）。
func TestV04CrossProcessDeterminism(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^$") // TestMain 分支处理
	cmd.Env = append(os.Environ(), "V04_GOLDEN_HASH=1")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("subprocess: %v (%s)", err, out)
	}
	want := fmt.Sprintf("%x\n", sha256.Sum256([]byte(v04GoldenDump(v04GoldenSeed))))
	if string(out) != want {
		t.Fatalf("cross-process hash mismatch:\n got %s\nwant %s", out, want)
	}
}

// Fragment：由 seed 哈希确定，12 向 × HexDist 14~18（2026-09-19 平衡
// 修订，与蓄息经济匹配），跨 seed 分布且落点强制可通行 (design §2.4).
func TestV04Fragment(t *testing.T) {
	seen := map[engine.Hex]bool{}
	for _, seed := range []uint64{1, 7, 42, 20260917, 20260918, 99, 12345} {
		p := V04(seed)
		frag := p.FragmentPos
		if frag != FragmentFor(seed) {
			t.Fatalf("seed %d: FragmentPos != FragmentFor", seed)
		}
		if d := engine.HexDist(engine.Hex{}, frag); d < 14 || d > 18 {
			t.Fatalf("seed %d: fragment dist = %d, want [14,18]", seed, d)
		}
		if !p.Passable(frag) {
			t.Fatalf("seed %d: fragment cell %v impassable", seed, frag)
		}
		if seen[frag] {
			t.Errorf("seed %d: fragment %v repeats an earlier seed's position", seed, frag)
		}
		seen[frag] = true
	}
	// 跨进程一致性由 FragmentFor 纯函数性保证：同 seed 两次一致
	if FragmentFor(20260918) != FragmentFor(20260918) {
		t.Fatal("FragmentFor not deterministic")
	}
}

// 出生区连通：v0.4 无限平面上（顽石极稀疏 + 出生 3×3 chunk 豁免），
// BFS 预算 4096 内从 (0,0) 可达 Fragment.
func TestV04InfiniteReachability(t *testing.T) {
	for _, seed := range []uint64{1, 42, 20260917, 20260918} {
		p := V04(seed)
		if !bfsReachable(p, p.PlayerSpawn, p.FragmentPos) {
			t.Fatalf("seed %d: fragment %v unreachable within budget", seed, p.FragmentPos)
		}
		if !bfsReachable(p, p.PlayerSpawn, p.UnknownSpawn) {
			t.Fatalf("seed %d: unknown %v unreachable within budget", seed, p.UnknownSpawn)
		}
	}
}

// 怪物懒激活：同 seed 同路径激活序列一致；激活事件 Type "spawn" 且进
// 事件流；同一 chunk 只激活一次；essence = chunk 主导行；Form ∈
// [60,80]；落点可通行且避开玩家位 (design §2.4).
func TestV04LazyActivation(t *testing.T) {
	path := []engine.Hex{ // 一条向东南推进的玩家路径（跨多个 chunk）
		{Q: 0, R: 0}, {Q: 6, R: 6}, {Q: 13, R: 13}, {Q: 25, R: 25},
		{Q: 26, R: 30}, {Q: -14, R: 2}, {Q: 40, R: 40},
	}
	run := func() []string {
		p := V04(20260918)
		w, _, _, _, _ := p.Genesis()
		var seq []string
		activated := map[[2]int]bool{} // 调用方持状态（match.activatedChunks 的对应物）
		for _, pos := range path {
			w.Tick++
			for _, cs := range PendingSpawns(w, pos, activated) {
				key := [2]int{cs.QC, cs.RC}
				if activated[key] {
					t.Fatalf("chunk (%d,%d) covered twice", cs.QC, cs.RC)
				}
				activated[key] = true // 无论是否出怪，该 chunk 只激活一次
				if cs.Spawn == nil {
					continue
				}
				ps := cs.Spawn
				qc, rc := cs.QC, cs.RC
				if ps.Event.Type != "spawn" {
					t.Errorf("event type = %s, want spawn", ps.Event.Type)
				}
				if ps.Event.Tick != w.Tick {
					t.Errorf("event tick = %d, want %d", ps.Event.Tick, w.Tick)
				}
				e := ps.Entity
				if e.Kind != "monster" || e.Essence != v04DominantEssence(p.Seed, qc, rc) {
					t.Errorf("mob %+v essence mismatch with dominant row", e.Pos)
				}
				if e.Integrity < 60 || e.Integrity > 80 || e.Integrity != e.MaxIntegrity {
					t.Errorf("mob form = %d, want [60,80]", e.Integrity)
				}
				if !w.At(e.Pos).Passable || e.Pos == pos {
					t.Errorf("mob pos %v impassable or on player", e.Pos)
				}
				seq = append(seq, fmt.Sprintf("tick%d:(%d,%d):%s:form%d", ps.Event.Tick, qc, rc, e.Essence, e.Integrity))
			}
		}
		// 回放已完成激活的路径：不得再产出任何怪物
		if extra := PendingSpawns(w, path[len(path)-1], activated); len(extra) != 0 {
			t.Errorf("re-activation produced %d extra spawns", len(extra))
		}
		return seq
	}
	a, b := run(), run()
	if strings.Join(a, "|") != strings.Join(b, "|") {
		t.Fatalf("activation sequence differs between runs:\n%s\n%s",
			strings.Join(a, "\n"), strings.Join(b, "\n"))
	}
	if len(a) == 0 {
		t.Fatalf("no mobs activated along the path (probabilities too low?)")
	}
	t.Logf("激活序列：%s", strings.Join(a, " → "))
}

// chunk 深度越高出怪概率越高（粗粒度统计断言，防止参数被调反）.
func TestV04MobChanceMonotonic(t *testing.T) {
	deep, shallow := 0, 0
	for qc := -40; qc <= 40; qc += 2 {
		for rc := -40; rc <= 40; rc += 2 {
			depth := engine.HexDist(chunkCenter(qc, rc), chunkCenter(0, 0))
			if engine.Hash01("20260918", "mob", qc, rc) < v04MobChance(depth) {
				if depth > 60 {
					deep++
				} else if depth < 24 {
					shallow++
				}
			}
		}
	}
	if deep <= shallow {
		t.Errorf("deep chunks spawned %d vs shallow %d — depth scaling broken", deep, shallow)
	}
}

// TestMain 支撑跨进程确定性测试：V04_GOLDEN_HASH=1 时打印 dump 的
// sha256 并退出（不做任何测试）。
func TestMain(m *testing.M) {
	if os.Getenv("V04_GOLDEN_HASH") == "1" {
		fmt.Printf("%x\n", sha256.Sum256([]byte(v04GoldenDump(v04GoldenSeed))))
		os.Exit(0)
	}
	os.Exit(m.Run())
}
