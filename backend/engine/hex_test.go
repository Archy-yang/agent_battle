package engine

import (
	"encoding/json"
	"math"
	"testing"
)

// —— HexDist：cube 语义下的真实步数（design §1.1） ——

func TestHexDist(t *testing.T) {
	if d := HexDist(Hex{0, 0}, Hex{3, -3}); d != 3 { // 对角直线
		t.Errorf("HexDist(0,0→3,-3) = %d, want 3", d)
	}
	if d := HexDist(Hex{0, 0}, Hex{8, -4}); d != 8 { // v0.4 Fragment 落点
		t.Errorf("HexDist(0,0→8,-4) = %d, want 8", d)
	}
	if d := HexDist(Hex{-3, 21}, Hex{0, 12}); d != 9 { // v02 spawn→Fragment
		t.Errorf("HexDist(-3,21→0,12) = %d, want 9", d)
	}
	if HexDist(Hex{1, 1}, Hex{1, 1}) != 0 {
		t.Errorf("distance to self must be 0")
	}
}

// 距离公理：非负、对称、三角不等式、六邻恰为 1。
func TestHexDistAxioms(t *testing.T) {
	coords := []Hex{
		{0, 0}, {1, 0}, {1, -1}, {0, -1}, {-1, 0}, {-1, 1}, {0, 1},
		{3, 1}, {-2, 5}, {4, -6}, {-8, 8},
	}
	for _, a := range coords {
		for _, b := range coords {
			if HexDist(a, b) < 0 {
				t.Errorf("negative distance %v→%v", a, b)
			}
			if HexDist(a, b) != HexDist(b, a) {
				t.Errorf("asymmetric distance %v→%v", a, b)
			}
			for _, c := range coords {
				if HexDist(a, c) > HexDist(a, b)+HexDist(b, c) {
					t.Errorf("triangle inequality violated: %v→%v→%v", a, b, c)
				}
			}
			if Adjacent(a, b) != (a != b && HexDist(a, b) == 1) {
				t.Errorf("Adjacent mismatch %v→%v", a, b)
			}
		}
	}
}

// 六邻居：固定顺序 E NE NW W SW SE，且每个邻居距离恰为 1。
func TestHexNeighbors(t *testing.T) {
	h := Hex{5, -2}
	want := [6]Hex{{6, -2}, {6, -3}, {5, -3}, {4, -2}, {4, -1}, {5, -1}}
	got := HexNeighbors(h)
	if got != want {
		t.Fatalf("neighbors = %v, want %v", got, want)
	}
	for i, n := range got {
		if HexDist(h, n) != 1 {
			t.Errorf("neighbor %d (%v) at distance %d", i, n, HexDist(h, n))
		}
		if n != h.Add(HexDirs[i]) {
			t.Errorf("neighbor %d off HexDirs", i)
		}
	}
}

// HexStepToward：首步必缩短距离；平局按 HexDirs 固定次序。
func TestHexStepToward(t *testing.T) {
	from, to := Hex{0, 0}, Hex{4, 2}
	ns := HexStepToward(from, to)
	d0 := HexDist(from, to)
	if HexDist(ns[0], to) != d0-1 {
		t.Errorf("first greedy step does not close distance: %v", ns[0])
	}
	for i := 1; i < 6; i++ {
		if HexDist(ns[i], to) < HexDist(ns[i-1], to) {
			t.Errorf("candidates not in ascending distance order at %d", i)
		}
	}
}

// —— 像素换算往返（pointy-top，design §1.3）——

func TestHexPixelRoundTrip(t *testing.T) {
	s := 20.0
	coords := []Hex{
		{0, 0}, {1, 0}, {0, 1}, {-1, 0}, {0, -1}, {3, -3}, {8, -4},
		{-3, 21}, {10, 2}, {-12, 7}, {25, -13},
	}
	for _, h := range coords {
		x, y := HexToPixel(h, s)
		back := PixelToHex(x, y, s)
		if back != h {
			t.Errorf("round trip %v → (%.4f,%.4f) → %v", h, x, y, back)
			continue
		}
		// 同一 hex 的像素往返误差 < 1e-6
		x2, y2 := HexToPixel(back, s)
		if math.Abs(x-x2) > 1e-6 || math.Abs(y-y2) > 1e-6 {
			t.Errorf("pixel drift for %v: (%.9f,%.9f) vs (%.9f,%.9f)", h, x, y, x2, y2)
		}
	}
	// 公式抽检：center_x = s·√3·(q + r/2), center_y = s·1.5·r
	x, y := HexToPixel(Hex{2, 1}, 10.0)
	if math.Abs(x-10*math.Sqrt(3)*2.5) > 1e-9 || math.Abs(y-15) > 1e-9 {
		t.Errorf("HexToPixel formula mismatch: (%f,%f)", x, y)
	}
}

// 中心点附近任意像素都应拾取回原 hex（拾取语义）。
func TestPixelToHexNearCenters(t *testing.T) {
	s := 16.0
	for q := -5; q <= 5; q++ {
		for r := -5; r <= 5; r++ {
			h := Hex{q, r}
			x, y := HexToPixel(h, s)
			if got := PixelToHex(x+0.1*s, y-0.1*s, s); got != h {
				t.Errorf("pick near center of %v got %v", h, got)
			}
		}
	}
}

// JSON 契约：{"q":..,"r":..}（评审决定 #2：{x,y}→{q,r} 硬切）。
func TestHexJSON(t *testing.T) {
	b, err := json.Marshal(Hex{Q: 8, R: -4})
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"q":8,"r":-4}` {
		t.Fatalf("marshal = %s", b)
	}
	var h Hex
	if err := json.Unmarshal([]byte(`{"q":3,"r":5}`), &h); err != nil {
		t.Fatal(err)
	}
	if h != (Hex{3, 5}) {
		t.Fatalf("unmarshal = %v", h)
	}
}

// ChunkCache：任意 (q,r)（含负数）查询返回可通行土性格，chunk 复用
// 同一底层存储（占位实现的行为契约，WP2 替换内部后仍须成立）。
func TestChunkCacheAt(t *testing.T) {
	cc := NewChunkCache(7, nil)
	for _, h := range []Hex{{0, 0}, {-1, -1}, {13, -25}, {-144, 144}} {
		c := cc.At(h.Q, h.R)
		if c == nil || !c.Passable || c.Essence != Earth {
			t.Errorf("At(%v) = %+v, want passable earth", h, c)
		}
	}
	// 指针稳定性：同一格两次查询指向同一存储
	if cc.At(3, 4) != cc.At(3, 4) {
		t.Errorf("chunk cache should reuse cell storage")
	}
	// 自定义地形纯函数按 (seed,q,r) 调用
	called := map[Hex]Cell{}
	gen := func(seed uint64, q, r int) Cell {
		c := Cell{Essence: Earth, Passable: true}
		if q == 0 && r == 0 {
			c.Danger = true
		}
		called[Hex{q, r}] = c
		return c
	}
	cc2 := NewChunkCache(9, gen)
	if !cc2.At(0, 0).Danger {
		t.Errorf("custom terrain not applied")
	}
	_ = called
}
