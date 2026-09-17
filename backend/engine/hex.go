package engine

import "math"

// Hex is one axial coordinate (q, r) on the pointy-top hex grid
// (v0.4 design §1.1). It replaces Vec2 wholesale — the JSON contract
// changes from {"x":..,"y":..} to {"q":..,"r":..} (评审决定 #2：硬切).
type Hex struct {
	Q int `json:"q"`
	R int `json:"r"`
}

// HexDirs is the fixed six-neighbor order that replaces bfsDirs
// (design §1.1): E NE NW W SW SE. Every expansion over neighbors —
// pathfinding, freeAdjacent, greedy steps, wander — uses this exact
// order so that same-seed replays stay byte-identical.
var HexDirs = [6]Hex{{1, 0}, {1, -1}, {0, -1}, {-1, 0}, {-1, 1}, {0, 1}}

// HexNeighbors returns h's six neighbors in the fixed HexDirs order.
func HexNeighbors(h Hex) [6]Hex {
	var out [6]Hex
	for i, d := range HexDirs {
		out[i] = h.Add(d)
	}
	return out
}

// Add returns the hex sum a+b.
func (h Hex) Add(o Hex) Hex { return Hex{Q: h.Q + o.Q, R: h.R + o.R} }

// HexDist returns the hex distance between a and b — the true step
// count on the six-neighborhood (design §1.1):
//
//	HexDist(a, b) = (|dq| + |dr| + |dq+dr|) / 2
//
// It replaces Chebyshev everywhere (design §1.4).
func HexDist(a, b Hex) int {
	dq := a.Q - b.Q
	dr := a.R - b.R
	ds := dq + dr // 第三 cube 轴差：必须取自有符号差值
	if dq < 0 {
		dq = -dq
	}
	if dr < 0 {
		dr = -dr
	}
	if ds < 0 {
		ds = -ds
	}
	return (dq + dr + ds) / 2
}

// Adjacent reports whether a and b are hex-neighbors. 六边形天然消灭了
// 方形网格的角接触歧义：六个方向等价（design §1.4）.
func Adjacent(a, b Hex) bool { return HexDist(a, b) == 1 }

// HexStepToward returns from's six neighbors ordered by ascending
// HexDist to to, ties broken by the fixed HexDirs order — a
// deterministic greedy step sequence. Step 0 always closes distance by
// 1 on open terrain; blocked callers fall through to later steps.
func HexStepToward(from, to Hex) [6]Hex {
	ns := HexNeighbors(from)
	dists := make([]int, 6)
	for i := range ns {
		dists[i] = HexDist(ns[i], to)
	}
	for i := 1; i < 6; i++ { // insertion sort, stable in HexDirs order
		for j := i; j > 0 && dists[j] < dists[j-1]; j-- {
			dists[j], dists[j-1] = dists[j-1], dists[j]
			ns[j], ns[j-1] = ns[j-1], ns[j]
		}
	}
	return ns
}

// —— 像素换算（pointy-top，design §1.3；Go 侧纯函数供测试与未来服务
// 端拾取用，前端 TS 侧同公式各一份） ——

// HexToPixel returns the pixel center of h for hexsize s (外接圆半径):
//
//	center_x = s * √3 * (q + r/2)
//	center_y = s * 3/2 * r
func HexToPixel(h Hex, s float64) (x, y float64) {
	x = s * math.Sqrt(3) * (float64(h.Q) + float64(h.R)/2)
	y = s * 1.5 * float64(h.R)
	return x, y
}

// PixelToHex maps pixel coordinates back to the hex containing them:
// axial float 解 + cube rounding（design §1.3）.
func PixelToHex(x, y, s float64) Hex {
	qf := (math.Sqrt(3)/3*x - y/3) / s
	rf := (2.0 / 3.0 * y) / s
	return cubeRound(qf, rf)
}

// cubeRound rounds fractional axial (qf, rf) to the nearest hex by
// fixing the largest cube-axis rounding error (design §1.3).
func cubeRound(qf, rf float64) Hex {
	xf, zf := qf, rf
	yf := -xf - zf
	x, y, z := math.Round(xf), math.Round(yf), math.Round(zf)
	dx, dy, dz := math.Abs(x-xf), math.Abs(y-yf), math.Abs(z-zf)
	switch {
	case dx > dy && dx > dz:
		x = -y - z
	case dy > dz:
		y = -x - z
	default:
		z = -x - y
	}
	return Hex{Q: int(x), R: int(z)}
}
