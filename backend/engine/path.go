package engine

// Deterministic pathfinding for the move_to order (M3.0).
//
// BFS runs on the static passable grid only — 动态占位实体（agent/device）
// 不参与建图。Blocking by dynamic entities is handled at stepping time
// (order_blocked + wait/reroute), not here; this keeps the graph a pure
// function of terrain, so the same input always yields the same path.

// bfsDirs is the fixed neighbor expansion order: 上、右、下、左.
var bfsDirs = [][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}

// bfsPath returns the shortest path from to target on the static
// passable grid, excluding the start cell and including the target.
// For from == to the path is empty (ok=true). ok=false means the target
// is unreachable (out of bounds, impassable, or walled off).
func (w *World) bfsPath(from, to Vec2) ([]Vec2, bool) {
	if !w.InBounds(to) || !w.At(to).Passable {
		return nil, false
	}
	if from == to {
		return []Vec2{}, true
	}
	// prev doubles as the visited set; map iteration order is never
	// read, only point lookups — determinism holds.
	prev := map[Vec2]Vec2{from: from}
	queue := []Vec2{from}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, d := range bfsDirs {
			nxt := Vec2{X: cur.X + d[0], Y: cur.Y + d[1]}
			if !w.InBounds(nxt) || !w.At(nxt).Passable {
				continue
			}
			if _, seen := prev[nxt]; seen {
				continue
			}
			prev[nxt] = cur
			if nxt == to {
				path := []Vec2{nxt}
				for p := cur; p != from; p = prev[p] {
					path = append(path, p)
				}
				for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
					path[i], path[j] = path[j], path[i]
				}
				return path, true
			}
			queue = append(queue, nxt)
		}
	}
	return nil, false
}
