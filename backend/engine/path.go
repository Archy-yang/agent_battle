package engine

// Deterministic pathfinding for the move_to order (M3.0).
//
// BFS runs on the static passable terrain only — 动态占位实体
// （agent/device）不参与建图。Blocking by dynamic entities is handled
// at stepping time (order_blocked + wait/reroute), not here; this keeps
// the graph a pure function of terrain, so the same input always yields
// the same path.
//
// v0.4: expansion is over the six hex neighbors in the fixed HexDirs
// order (design §1.2). The world is infinite, so unbounded BFS could
// diverge on an unreachable target (e.g. beyond an endless wall line);
// PathBudget caps the explored set — beyond it the target counts as
// unreachable and the order is rejected/blocked. WP5 upgrades this to
// A* with the hex-distance heuristic (design §2.5); the budget stays
// as the hard stop.

// PathBudget caps BFS expansions on the infinite grid (design §2.5
// previews 512 nodes for A*; BFS keeps a larger allowance until then).
const PathBudget = 4096

// bfsPath returns the shortest path from to target on the static
// passable terrain, excluding the start cell and including the target.
// For from == to the path is empty (ok=true). ok=false means the target
// is unreachable (impassable, walled off, or beyond the budget).
func (w *World) bfsPath(from, to Hex) ([]Hex, bool) {
	if !w.At(to).Passable {
		return nil, false
	}
	if from == to {
		return []Hex{}, true
	}
	// prev doubles as the visited set; map iteration order is never
	// read, only point lookups — determinism holds.
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
			if len(prev) >= PathBudget { // 无限图兜底：预算耗尽视为不可达
				return nil, false
			}
			queue = append(queue, nxt)
		}
	}
	return nil, false
}
