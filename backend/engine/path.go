package engine

import "container/heap"

// Deterministic pathfinding for the move_to order (M3.0, v0.4 WP5).
//
// A* runs on the static passable terrain only — 动态占位实体
// （agent/device）不参与建图。Blocking by dynamic entities is handled
// at stepping time (order_blocked + wait/reroute), not here; this keeps
// the graph a pure function of terrain, so the same input always yields
// the same path.
//
// v0.4：无限图上 BFS 无界，改 A*（design §2.5）——启发函数取 HexDist。
// 六邻域上每步至多令 HexDist ±1，故 h 既 admissible（不高估）也一致
// （相邻节点 h 差 ≤1 = 边权），节点首次出队即最优，可安全闭合.
//
// 确定性三件套（design §2.5 / §1.1）：
//   - 邻居展开固定沿 HexDirs 序；
//   - 优先队列按 (f, seq) 出队——f 相同按入队序（先进先出）；
//   - 仅当 g 严格更优才重入队（无同分抖动，seq 单调不回退）.
//
// 终止性：无限平面上任何输入都确定终止——每次有效出队计入扩展预算，
// 预算耗尽即判不可达返回；可达分量有限时堆自然排空。不可达目标
// （不可通行、无限墙线、超远目标）都在 ≤PathBudget 次扩展内返回.

// PathBudget caps A* expansions on the infinite grid (design §2.5)：
// 超预算视为不可达，调用方按现有 order_blocked 语义处理（订单保留）.
const PathBudget = 4096

// pathNode is one frontier entry of the A* search.
type pathNode struct {
	pos Hex
	g   int // 起点到 pos 的已走步数
	f   int // g + HexDist(pos, to)
	seq int // 入队序：f 相同时的确定性 tie-break
}

// pathFrontier is a min-heap ordered by (f, seq) — f 相同时先入队者先
// 出队，展开顺序与 map 遍历的随机性完全解耦.
type pathFrontier []pathNode

func (h pathFrontier) Len() int { return len(h) }
func (h pathFrontier) Less(i, j int) bool {
	if h[i].f != h[j].f {
		return h[i].f < h[j].f
	}
	return h[i].seq < h[j].seq
}
func (h pathFrontier) Swap(i, j int) { h[i], h[j] = h[j], h[i] }
func (h *pathFrontier) Push(x any)   { *h = append(*h, x.(pathNode)) }
func (h *pathFrontier) Pop() any {
	old := *h
	n := len(old)
	it := old[n-1]
	*h = old[:n-1]
	return it
}

// astarPath returns the shortest path from to target on the static
// passable terrain, excluding the start cell and including the target.
// For from == to the path is empty (ok=true). ok=false means the target
// is unreachable (impassable, walled off, or beyond the budget).
func (w *World) astarPath(from, to Hex) ([]Hex, bool) {
	if !w.At(to).Passable { // 目标格本身不可通行
		return nil, false
	}
	if from == to {
		return []Hex{}, true
	}
	bestG := map[Hex]int{from: 0}
	prev := map[Hex]Hex{from: from}
	closed := map[Hex]bool{}
	frontier := &pathFrontier{{pos: from, f: HexDist(from, to)}}
	heap.Init(frontier)
	seq := 1
	expanded := 0
	for frontier.Len() > 0 {
		cur := heap.Pop(frontier).(pathNode)
		if closed[cur.pos] || cur.g > bestG[cur.pos] { // 惰性删除：陈旧条目跳过
			continue
		}
		if cur.pos == to { // 一致启发：首次出队即最短
			path := []Hex{to}
			for p := prev[to]; p != from; p = prev[p] {
				path = append(path, p)
			}
			for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
				path[i], path[j] = path[j], path[i]
			}
			return path, true
		}
		closed[cur.pos] = true
		expanded++
		if expanded >= PathBudget { // 无限图兜底：预算耗尽视为不可达
			return nil, false
		}
		g2 := cur.g + 1
		for _, d := range HexDirs { // 邻居展开固定沿 HexDirs 序
			nxt := cur.pos.Add(d)
			if closed[nxt] || !w.At(nxt).Passable {
				continue
			}
			if old, seen := bestG[nxt]; seen && old <= g2 {
				continue // 只有严格更优才重入队
			}
			bestG[nxt] = g2
			prev[nxt] = cur.pos
			heap.Push(frontier, pathNode{pos: nxt, g: g2, f: g2 + HexDist(nxt, to), seq: seq})
			seq++
		}
	}
	return nil, false // 可达分量已穷尽（有限封闭区域）
}

// bfsPath is the compatibility shim kept for the existing sim.go call
// sites（调用点零改动）：WP5 起实现为 A*（design §2.5），签名与返回
// 契约与原 BFS 版逐字一致.
func (w *World) bfsPath(from, to Hex) ([]Hex, bool) { return w.astarPath(from, to) }
