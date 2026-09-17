package engine

import (
	"reflect"
	"testing"
)

// WP5 路径质量测试（design §2.5）：
//   - 与测试内参考 BFS 的路径长度一致性（最短性互证）；
//   - 绕障：直线墙留缝，A* 必须过缝；
//   - 预算：超预算目标确定返回不可达而非挂死；
//   - 确定性：同输入多次运行路径逐字节一致.

// refBFSPath is the test-local reference implementation: plain hex BFS
// on the static passable terrain，与被测 A* 相互独立。无限图上无界 BFS
// 会发散，与引擎同语义以访问上限兜底（超限判不可达）.
func refBFSPath(w *World, from, to Hex) ([]Hex, bool) {
	if !w.At(to).Passable {
		return nil, false
	}
	if from == to {
		return []Hex{}, true
	}
	prev := map[Hex]Hex{from: from}
	queue := []Hex{from}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, d := range HexDirs {
			nxt := cur.Add(d)
			if _, seen := prev[nxt]; seen || !w.At(nxt).Passable {
				continue
			}
			prev[nxt] = cur
			if nxt == to {
				path := []Hex{nxt}
				for p := cur; p != from; p = prev[p] {
					path = append(path, p)
				}
				for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
					path[i], path[j] = path[j], path[i]
				}
				return path, true
			}
			if len(prev) >= PathBudget { // 与引擎旧 BFS 相同的兜底语义
				return nil, false
			}
			queue = append(queue, nxt)
		}
	}
	return nil, false
}

// checkPath validates path contract: 逐格邻接、全可通行、不含起点、
// 终点正确、无重复格.
func checkPath(t *testing.T, w *World, from, to Hex, path []Hex) {
	t.Helper()
	if len(path) == 0 {
		t.Fatalf("empty path for %v→%v", from, to)
	}
	seen := map[Hex]bool{}
	cur := from
	for i, p := range path {
		if !Adjacent(cur, p) {
			t.Fatalf("path[%d]=%v not adjacent to %v", i, p, cur)
		}
		if !w.At(p).Passable {
			t.Fatalf("path[%d]=%v impassable", i, p)
		}
		if seen[p] {
			t.Fatalf("path revisits %v", p)
		}
		seen[p] = true
		cur = p
	}
	if cur != to {
		t.Fatalf("path ends at %v, want %v", cur, to)
	}
}

// gapWallMap：q=5 一堵直线墙，仅 (5,3) 留缝.
func gapWallMap(_ uint64, q, r int) Cell {
	c := Cell{Essence: Earth, Passable: true}
	if q == 5 && r != 3 {
		c.Passable = false
	}
	return c
}

// mazeMap：两段错位墙线 + 散石 + 一只 dist-2 石环围死的口袋.
func mazeMap(_ uint64, q, r int) Cell {
	c := Cell{Essence: Earth, Passable: true}
	if q == 3 && r >= -2 && r <= 4 {
		c.Passable = false
	}
	if q == 7 && r >= 0 && r <= 6 {
		c.Passable = false
	}
	h := Hex{Q: q, R: r}
	switch h {
	case Hex{5, 5}, Hex{6, 1}, Hex{-1, -2}, Hex{2, 7}:
		c.Passable = false // 散石
	}
	if HexDist(h, Hex{20, 20}) == 2 {
		c.Passable = false // 石环口袋：内部与外界完全隔绝
	}
	return c
}

// A* 与参考 BFS 在同一批 (地图, 起点, 终点) 上结论一致：可达性一致、
// 可达时路径长度一致（均为最短），且 A* 路径满足契约.
//
// 注：A* 预算计"扩展节点数"、BFS 兜底计"已发现节点数"，同为 4096 时
// A* 的有向探索能到达更远的目标（这正是升级的意义）——故参考对比的
// 目标集限制在参考 BFS 预算内，预算语义本身由 TestAStarBudgetReturns
// Unreachable 单独验证.
func TestAStarPathLengthMatchesBFSReference(t *testing.T) {
	base := []Hex{{9, 3}, {5, 3}, {5, -3}, {3, 3}, {9, -2}, {4, 6}, {2, 1}}
	targets := map[string][]Hex{
		"gapwall": base,
		"maze":    append(append([]Hex{}, base...), Hex{20, 20}), // 石环口袋：真不可达
	}
	starts := []Hex{{0, 0}, {1, 3}, {-2, 4}, {6, 2}}
	for name, gen := range map[string]TerrainFunc{
		"gapwall": gapWallMap,
		"maze":    mazeMap,
	} {
		w := NewWorldTerrain(42, gen)
		for _, s := range starts {
			for _, tt := range targets[name] {
				got, aok := w.bfsPath(s, tt) // A*（经兼容签名）
				want, wok := refBFSPath(w, s, tt)
				if aok != wok {
					t.Fatalf("%s %v→%v: reachable A*=%v refBFS=%v", name, s, tt, aok, wok)
				}
				if !aok {
					continue
				}
				if len(got) != len(want) {
					t.Fatalf("%s %v→%v: A* len %d != BFS len %d\nA*: %v\nBFS: %v",
						name, s, tt, len(got), len(want), got, want)
				}
				checkPath(t, w, s, tt, got)
				if len(got) < HexDist(s, tt) {
					t.Fatalf("%s %v→%v: path len %d shorter than hex distance %d — impossible",
						name, s, tt, len(got), HexDist(s, tt))
				}
			}
		}
		// 退化情形：起点==终点 → 空路径；目标不可通行 → 不可达
		if p, ok := w.bfsPath(Hex{2, 2}, Hex{2, 2}); !ok || len(p) != 0 {
			t.Fatalf("%s: self path = %v, %v", name, p, ok)
		}
		stone := map[string]Hex{"gapwall": {5, 0}, "maze": {5, 5}}[name] // 各图的一块顽石
		if _, ok := w.bfsPath(Hex{0, 0}, stone); ok {
			t.Fatalf("%s: impassable target %v reported reachable", name, stone)
		}
	}
}

// 绕障：直线墙仅留一缝，A* 的最短路必须穿缝而过.
func TestAStarDetoursThroughGap(t *testing.T) {
	w := NewWorldTerrain(42, gapWallMap)
	from, to := Hex{1, 3}, Hex{9, 3}
	path, ok := w.bfsPath(from, to)
	if !ok {
		t.Fatal("gap should be reachable")
	}
	checkPath(t, w, from, to, path)
	gap := Hex{5, 3}
	found := false
	for _, p := range path {
		if p == gap {
			found = true
		}
	}
	if !found {
		t.Fatalf("path does not pass through the gap %v: %v", gap, path)
	}
	// 任何路径都必须过缝：len == dist(from,gap) + dist(gap,to) = 4+4
	ref, _ := refBFSPath(w, from, to)
	if len(path) != 8 || len(path) != len(ref) {
		t.Fatalf("path len = %d, want 8 (ref BFS %d)", len(path), len(ref))
	}
}

// 预算：无限平原上超远目标（远超 PathBudget 扩展能力）确定返回不可
// 达而非挂死（设计 §2.5 的兜底语义，与旧 BFS 一致）.
func TestAStarBudgetReturnsUnreachable(t *testing.T) {
	w := NewWorldTerrain(1, nil) // 无限平原，全可通行
	far := Hex{Q: 5000, R: -5000}
	if path, ok := w.bfsPath(Hex{}, far); ok || path != nil {
		t.Fatalf("beyond-budget target should be unreachable, got %v (%v)", path, ok)
	}
	// 墙线后的不可达目标同样在预算内返回（order_blocked 语义的来源）
	wall := func(_ uint64, q, r int) Cell {
		c := Cell{Essence: Earth, Passable: true}
		if q == 3 {
			c.Passable = false
		}
		return c
	}
	w2 := NewWorldTerrain(1, wall)
	if _, ok := w2.bfsPath(Hex{1, 1}, Hex{5, 1}); ok {
		t.Fatal("target behind an endless wall should be unreachable")
	}
	// 正常距离的目标不受预算影响
	if path, ok := w.bfsPath(Hex{}, Hex{30, 0}); !ok || len(path) != 30 {
		t.Fatalf("ordinary target should be reachable in 30 steps, got %v (%v)", path, ok)
	}
}

// 确定性：同输入跑 3 次路径逐字节一致（design §2.5 重放要求）.
func TestAStarDeterministicBytes(t *testing.T) {
	w := NewWorldTerrain(42, mazeMap)
	for _, pair := range [][2]Hex{
		{{0, 0}, {9, 3}},
		{{1, 3}, {5, -3}},
		{{-2, 4}, {4, 6}},
		{{6, 2}, {9, -2}},
	} {
		first, ok := w.bfsPath(pair[0], pair[1])
		if !ok {
			t.Fatalf("%v→%v unexpectedly unreachable", pair[0], pair[1])
		}
		for run := 2; run <= 3; run++ {
			again, ok := w.bfsPath(pair[0], pair[1])
			if !ok || !reflect.DeepEqual(first, again) {
				t.Fatalf("run %d differs for %v→%v:\n%v\n%v", run, pair[0], pair[1], first, again)
			}
		}
	}
}
