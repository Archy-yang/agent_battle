package engine

import "strconv"

// v0.4 确定性无限生成 (design §2.2)：把 world_genesis §3.1 的四步生成
// 从"全局"改写为以世界坐标为输入的纯函数。chunk 只负责枚举自己名下的
// 144 个 (q, r)；Terrain 对任意单格良定义，与 chunk 边界无关。
//
// 世界整数格坐标取线性变换 X = 2q + r, Y = r（design §1.2/§2.2）：邻格
// 的 (X,Y) 距离近似恒定，值噪声在 (X,Y) 上各向同性。四步：
//
//	一步 阴阳场  n1 = Noise(seed,"yin",X,Y, L1=32)
//	二步 五行    n1<0.30 → 水泽；n1>0.70 → 火地；
//	             中带 n2 = Noise(seed,"wuxing",X,Y, L2=10) 分三档：
//	             金（偏阴） / 土（中和） / 木（偏阳）
//	三步 相生梯度 过渡带（|n1−阈值|<ε）内 essence 取该阈值两侧的
//	             相生对（金↔水的凝露带、木↔火的煅木带），阈值沿
//	             第三层平滑噪声摆动 → 地貌沿相生方向渐变而非突跳，
//	             且不会出现椒盐碎屑
//	四步 相克边界 六邻内出现克对（水|火、土|水、木|土、火|金、金|木）
//	             → 该格标记 Danger（迷雾泽/荒铜林……），危险位置由
//	             法则自洽产生
//	通行性 顽石 n3 = Noise(seed,"stone",X,Y, L3=6)，阈值极稀疏；
//	             出生 chunk (0,0) 及八邻块豁免——出生区域保证开阔
const (
	// YinPeriod L1：阴阳场格距（低频）.
	YinPeriod = 32
	// WuxingPeriod L2：中带五行分档格距（中频）.
	WuxingPeriod = 10
	// StonePeriod L3：顽石层格距（高频）.
	StonePeriod = 6
	// TransPeriod 过渡带阈值摆动的格距.
	TransPeriod = 7
	// BandLo / BandHi 是阴阳场的阴极/阳极阈值 (design §2.2 第二步).
	BandLo = 0.30
	BandHi = 0.70
	// TransEps 是过渡带半宽 ε.
	TransEps = 0.05
	// StoneThreshold 顽石阈值——极稀疏（实测 ≈0.76%，chunk_test 统计）.
	StoneThreshold = 0.94
)

// terrainBase carries a cell through 第一步+第二步：base essence 与
// 阴阳值 n1（第三步需要 n1 判断过渡带）.
type terrainBase struct {
	ess Essence
	n1  float64
}

func terrainSeedStr(seed uint64) string { return strconv.FormatUint(seed, 10) }

func terrainXY(q, r int) (float64, float64) { return float64(2*q + r), float64(r) }

// terrainBaseOf computes 第一步+第二步 for one cell — 纯函数.
func terrainBaseOf(ss string, q, r int) terrainBase {
	x, y := terrainXY(q, r)
	n1 := Noise(ss, "yin", x, y, YinPeriod)
	switch {
	case n1 < BandLo: // 阴极：水泽（老阴）
		return terrainBase{Water, n1}
	case n1 > BandHi: // 阳极：火地（老阳）
		return terrainBase{Fire, n1}
	}
	n2 := Noise(ss, "wuxing", x, y, WuxingPeriod)
	switch {
	case n2 < 1.0/3.0: // 偏阴档：金（少阴）
		return terrainBase{Metal, n1}
	case n2 < 2.0/3.0: // 中和：土（冲气以为和）
		return terrainBase{Earth, n1}
	default: // 偏阳档：木（少阳）
		return terrainBase{Wood, n1}
	}
}

// terrainEssenceOf applies 第三步（相生梯度过渡带）to a base cell.
//
// 过渡带内 essence 取阈值两侧的相生对：低阈值带为 金↔水（金生水，
// 凝露带），高阈值带为 木↔火（木生火，煅木带）。带的分界阈值不取
// 死值，而是沿 TransPeriod 平滑噪声在 ±TransEps 内摆动——等价于沿
// ∇n1 梯度方向对边界做单调重参数化：跨带相邻格恒为相生对，地貌沿
// 梯度渐变，且边界是平滑曲线而非椒盐碎屑.
func terrainEssenceOf(ss string, q, r int, b terrainBase) Essence {
	x, y := terrainXY(q, r)
	wiggle := (Noise(ss, "trans", x, y, TransPeriod) - 0.5) * 2 * TransEps
	if d := b.n1 - BandLo; d > -TransEps && d < TransEps {
		if b.n1 > BandLo+wiggle {
			return Metal
		}
		return Water
	}
	if d := b.n1 - BandHi; d > -TransEps && d < TransEps {
		if b.n1 > BandHi+wiggle {
			return Fire
		}
		return Wood
	}
	return b.ess
}

// restrainPair reports whether a and b form a 克对 in either direction
// (design §2.2 第四步：水|火、金|木……无向).
func restrainPair(a, b Essence) bool { return Restrains(a, b) || Restrains(b, a) }

// terrainStone reports 第"通行性"步：该格是否顽石。出生 chunk (0,0)
// 及其八邻块（qc,rc ∈ {-1,0,1}²）豁免——出生区域保证开阔.
func terrainStone(ss string, q, r int) bool {
	if qc, rc := floorDiv(q, ChunkSize), floorDiv(r, ChunkSize); qc >= -1 && qc <= 1 && rc >= -1 && rc <= 1 {
		return false
	}
	x, y := terrainXY(q, r)
	return Noise(ss, "stone", x, y, StonePeriod) > StoneThreshold
}

// Terrain is the pure per-cell generator F(seed, q, r) → Cell (design
// §2.2). 对任意 (q, r) 良定义、跨块连续、与 chunk 边界无关：第三步的
// 邻居 essence 与第四步的六邻克对都由噪声按需重算，天然跨块一致.
// GenerateChunk 的 apron 快路径与本函数逐位一致（chunk_test 断言）.
func Terrain(seed uint64, q, r int) Cell {
	ss := terrainSeedStr(seed)
	self := Hex{Q: q, R: r}
	b := terrainBaseOf(ss, q, r)
	cell := Cell{Essence: terrainEssenceOf(ss, q, r, b), Passable: true}
	for _, d := range HexDirs { // 第四步：六邻克对 → Danger
		nb := self.Add(d)
		neigh := terrainBaseOf(ss, nb.Q, nb.R)
		if restrainPair(cell.Essence, terrainEssenceOf(ss, nb.Q, nb.R, neigh)) {
			cell.Danger = true
			break
		}
	}
	if terrainStone(ss, q, r) {
		cell.Passable = false // 顽石
	}
	return cell
}

// chunkApron is the one-cell apron (围裙, design §2.2)：chunk 按
// 14×14 计算邻居 essence，只保留内部 12×12——块内第四步判定用的是
// "真邻居"而非猜测，且整个过程仍是 (seed, coord) 纯函数.
const chunkApron = 1

// genChunk builds one chunk via the 14×14 apron path. 逐格结果与
// Terrain(seed, q, r) 完全一致：两者共享 terrainBaseOf/terrainEssenceOf/
// terrainStone，apron 只是避免逐格重算邻居的等价快路径.
func genChunk(seed uint64, qc, rc int) *Chunk {
	ss := terrainSeedStr(seed)
	n := ChunkSize + 2*chunkApron // 14
	bases := make([]terrainBase, n*n)
	finals := make([]Essence, n*n)
	for j := 0; j < n; j++ {
		for i := 0; i < n; i++ {
			q, r := qc*ChunkSize+i-chunkApron, rc*ChunkSize+j-chunkApron
			b := terrainBaseOf(ss, q, r)
			bases[j*n+i] = b
			finals[j*n+i] = terrainEssenceOf(ss, q, r, b)
		}
	}
	ch := &Chunk{QC: qc, RC: rc}
	for j := 0; j < ChunkSize; j++ {
		for i := 0; i < ChunkSize; i++ {
			ai, aj := i+chunkApron, j+chunkApron
			q, r := qc*ChunkSize+i, rc*ChunkSize+j
			cell := Cell{Essence: finals[aj*n+ai], Passable: true}
			for _, d := range HexDirs { // apron 保证 (ai+d.Q, aj+d.R) 在界内
				if restrainPair(cell.Essence, finals[(aj+d.R)*n+(ai+d.Q)]) {
					cell.Danger = true
					break
				}
			}
			if terrainStone(ss, q, r) {
				cell.Passable = false
			}
			ch.Cells[j*ChunkSize+i] = cell
		}
	}
	return ch
}
