package engine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
)

// dumpEssences renders the essence/顽石 matrix of the rectangle
// [q0,q1]×[r0,r1] as deterministic text (one rune per cell: 木火土金水,
// '*' = 顽石).
func dumpEssences(seed uint64, q0, r0, q1, r1 int) string {
	out := ""
	for r := r0; r <= r1; r++ {
		for q := q0; q <= q1; q++ {
			c := Terrain(seed, q, r)
			if !c.Passable {
				out += "*"
				continue
			}
			out += c.Essence.String()
		}
		out += "\n"
	}
	return out
}

func dumpHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// 跨进程确定性（world_genesis §7）：同 seed 同坐标两次独立生成，dump
// hash 逐位一致。地形是 (seed, coord) 纯函数、不消耗 Rng 状态.
func TestTerrainDeterministic(t *testing.T) {
	const seed = 20260918
	h1 := dumpHash(dumpEssences(seed, -40, -40, 40, 40))
	h2 := dumpHash(dumpEssences(seed, -40, -40, 40, 40))
	if h1 != h2 {
		t.Fatalf("same seed twice: %s vs %s", h1, h2)
	}
	// 独立世界（v0.4 生成 + LRU）重算同一区域，结果一致
	w := NewWorld(seed)
	for r := -30; r <= 30; r += 7 {
		for q := -30; q <= 30; q += 7 {
			if got, want := *w.At(Hex{q, r}), Terrain(seed, q, r); got != want {
				t.Fatalf("world At(%d,%d) = %+v, Terrain = %+v", q, r, got, want)
			}
		}
	}
	// 不同 seed 必须不同（防"恒等函数"假阳性）
	if dumpHash(dumpEssences(1, 0, 0, 20, 20)) == dumpHash(dumpEssences(2, 0, 0, 20, 20)) {
		t.Fatalf("different seeds produced identical dumps")
	}
}

// apron 等价性：14×14 围裙快路径与逐格 Terrain 完全一致——chunk 边界
// 两侧的第三/四步判定用的是"真邻居" (design §2.2).
func TestChunkApronMatchesTerrain(t *testing.T) {
	const seed = 7
	for _, ck := range []chunkKey{{0, 0}, {1, 0}, {-1, -1}, {2, -3}, {5, 5}} {
		ch := genChunk(seed, ck.qc, ck.rc)
		for j := 0; j < ChunkSize; j++ {
			for i := 0; i < ChunkSize; i++ {
				q, r := ck.qc*ChunkSize+i, ck.rc*ChunkSize+j
				if got, want := ch.Cells[j*ChunkSize+i], Terrain(seed, q, r); got != want {
					t.Fatalf("chunk (%d,%d) cell (%d,%d) = %+v, Terrain = %+v",
						ck.qc, ck.rc, q, r, got, want)
				}
			}
		}
	}
}

// 地貌占比统计：阴/阳极与五行档位来自噪声阈值，顽石极稀疏 (design
// §2.2)。断言取结构性的宽界，具体实测值打进日志供调参/评审.
func TestTerrainStats(t *testing.T) {
	const seed = 20260918
	var counts [5]int
	danger, stones, total := 0, 0, 0
	for r := -100; r < 100; r++ {
		for q := -100; q < 100; q++ {
			c := Terrain(seed, q, r)
			total++
			counts[c.Essence]++
			if c.Danger {
				danger++
			}
			if !c.Passable {
				stones++
			}
		}
	}
	for _, e := range All() {
		t.Logf("%s 占比 %.2f%%", e, 100*float64(counts[e])/float64(total))
	}
	t.Logf("Danger 占比 %.2f%%，顽石占比 %.4f%%", 100*float64(danger)/float64(total), 100*float64(stones)/float64(total))
	for _, e := range All() {
		if counts[e] == 0 {
			t.Errorf("essence %s never occurs", e)
		}
	}
	if stoneRate := float64(stones) / float64(total); stoneRate > 0.02 || stoneRate <= 0 {
		t.Errorf("stone rate = %.4f, want sparse and nonzero (0, 2%%]", stoneRate)
	}
	if dangerRate := float64(danger) / float64(total); dangerRate > 0.35 {
		t.Errorf("danger rate = %.2f%%, want <= 35%%", 100*dangerRate)
	}
}

// 出生区豁免：chunk (0,0) 及八邻块（q,r ∈ [-12,24)）无顽石——出生
// 区域保证开阔 (design §2.2 通行性).
func TestBirthRegionOpen(t *testing.T) {
	const seed = 99
	for r := -ChunkSize; r < 2*ChunkSize; r++ {
		for q := -ChunkSize; q < 2*ChunkSize; q++ {
			if c := Terrain(seed, q, r); !c.Passable {
				t.Fatalf("birth region cell (%d,%d) is stone: %+v", q, r, c)
			}
		}
	}
}

// 跨块连续性：相邻格 essence 相同或相生的占比（防碎屑化回归）。
// 显式覆盖 chunk 边界两侧的相邻对——生成是逐块可计算纯函数，边界
// 两侧与块内不应有统计差异 (design §2.2).
func TestCrossChunkContinuity(t *testing.T) {
	const seed = 20260918
	const lo, hi = -36, 35 // 覆盖 6×6 个 chunk 及其全部边界
	sameSheng, ke, total := 0, 0, 0
	pairAt := func(a, b Hex) {
		ea := Terrain(seed, a.Q, a.R).Essence
		eb := Terrain(seed, b.Q, b.R).Essence
		total++
		switch {
		case ea == eb || Generates(ea, eb) || Generates(eb, ea):
			sameSheng++
		default:
			ke++
		}
	}
	for r := lo; r <= hi; r++ {
		for q := lo; q <= hi; q++ {
			h := Hex{q, r}
			pairAt(h, h.Add(HexDirs[0])) // E
			pairAt(h, h.Add(HexDirs[5])) // SE
		}
	}
	ratio := float64(sameSheng) / float64(total)
	t.Logf("相邻格 同/相生 占比 %.3f（克对 %.3f）", ratio, 1-ratio)
	if ratio < 0.80 {
		t.Errorf("same-or-generating adjacency ratio = %.3f, want >= 0.80 (碎屑化回归)", ratio)
	}
	// chunk 边界专门抽查：q = ±12k、r = ±12k 两侧的横跨对
	boundarySheng, boundaryTotal := 0, 0
	for k := -3; k <= 3; k++ {
		for m := -12; m <= 12; m++ {
			edges := [][2]Hex{
				{{Q: k * ChunkSize, R: m}, {Q: k*ChunkSize - 1, R: m}},     // 竖直 chunk 缝
				{{Q: m, R: k * ChunkSize}, {Q: m, R: k*ChunkSize - 1}},     // 水平 chunk 缝
				{{Q: m, R: k * ChunkSize}, {Q: m - 1, R: k*ChunkSize - 1}}, // 斜向缝
			}
			for _, e := range edges {
				ea := Terrain(seed, e[0].Q, e[0].R).Essence
				eb := Terrain(seed, e[1].Q, e[1].R).Essence
				boundaryTotal++
				if ea == eb || Generates(ea, eb) || Generates(eb, ea) {
					boundarySheng++
				}
			}
		}
	}
	bratio := float64(boundarySheng) / float64(boundaryTotal)
	t.Logf("chunk 缝上 同/相生 占比 %.3f（%d 对）", bratio, boundaryTotal)
	if bratio < 0.80 {
		t.Errorf("chunk-seam ratio = %.3f, want >= 0.80", bratio)
	}
}

// LRU（评审决定 #4：容量 256）：容量淘汰后按纯函数重算，结果与从未
// 淘汰的逐格 Terrain 一致——"淘汰只丢地形，重新进入零成本回放"
// (design §2.3).
func TestChunkCacheLRU(t *testing.T) {
	const seed = 5
	cc := NewWorld(seed).Chunks
	// 触碰 300 个 chunk（> 256），迫使 LRU 淘汰
	for k := 0; k < 300; k++ {
		if c := cc.At(k*ChunkSize, 0); c.Essence < Wood || c.Essence > Water {
			t.Fatalf("At(%d,0) invalid cell %+v", k*ChunkSize, c)
		}
	}
	if len(cc.chunks) != ChunkLRUCapacity {
		t.Errorf("cache size = %d, want %d", len(cc.chunks), ChunkLRUCapacity)
	}
	// 被淘汰的老块重算，结果必须与纯函数一致
	for k := 0; k < 300; k += 37 {
		q := k * ChunkSize
		for _, off := range []Hex{{0, 0}, {3, 5}, {11, 11}} {
			if got, want := cc.At(q+off.Q, off.R), Terrain(seed, q+off.Q, off.R); got != want {
				t.Fatalf("after eviction At(%d,%d) = %+v, Terrain = %+v", q+off.Q, off.R, got, want)
			}
		}
	}
}

// 注入地形缝：NewWorldTerrain 的 gen 按 (seed, q, r) 逐格调用.
func TestTerrainFuncInjection(t *testing.T) {
	gen := func(seed uint64, q, r int) Cell {
		c := Cell{Essence: Water, Passable: true}
		if q < 0 {
			c = Cell{Essence: Fire, Danger: true, Passable: true}
		}
		return c
	}
	w := NewWorldTerrain(1, gen)
	if got := w.At(Hex{3, 3}); got.Essence != Water {
		t.Errorf("injected terrain (3,3) = %+v", got)
	}
	if got := w.At(Hex{-3, 3}); got.Essence != Fire || !got.Danger {
		t.Errorf("injected terrain (-3,3) = %+v", got)
	}
}

// 值拷贝语义（WP1 挂账）：World.At 返回的指针指向私有拷贝，写入它
// 不影响缓存（LRU 淘汰/重算不受调用方污染）.
func TestAtReturnsCopy(t *testing.T) {
	w := NewWorld(3)
	p := Hex{40, 40} // 出生豁免区之外
	before := *w.At(p)
	c := w.At(p)
	c.Danger = true
	c.Passable = false
	if after := *w.At(p); after != before {
		t.Errorf("mutating At's return leaked into the cache: %+v vs %+v", after, before)
	}
	_ = fmt.Sprint(p) // 保持 fmt 引用（dumpEssences 也在用）
}
