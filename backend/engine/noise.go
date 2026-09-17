package engine

import "math"

// 确定性值噪声 (v0.4 design §2.2)：无限世界逐块生成的地基。
//
// 硬约束（world_genesis §7 + design §2.5）：
//   - 纯函数、无全局状态、不用 math/rand——同一 (seed, salt, x, y)
//     永远得到同一 [0,1) 值，跨进程跨平台逐位一致；
//   - 不消耗 match Rng 的任何状态（hash 自带 seed），chunk 生成与
//     重放正交。
//
// 实现分两层：
//
//	Hash01  格点值函数：整数 hash → [0,1)。FNV-1a 混合 seed/salt，
//	        再以 splitmix64 finalizer 混入坐标（全位雪崩）。
//	Noise   值噪声：格距 period 的整数格点上取 Hash01，格内
//	        smoothstep + 双线性插值。只依赖绝对坐标，因此任何 chunk
//	        算边界格都得到同一个值——"全局低频噪声 → 逐块可计算"
//	        的关键一步 (design §2.2 第一步)。

const (
	fnvOffset uint64 = 14695981039346656037
	fnvPrime  uint64 = 1099511628211
)

// fnv1a is the FNV-1a digest of s — used to fold the string-typed seed
// and salt into one starting hash state.
func fnv1a(s string) uint64 {
	h := fnvOffset
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= fnvPrime
	}
	return h
}

// mix64 is the splitmix64 finalizer: 三轮移位-乘法把输入雪崩到全 64 位.
func mix64(z uint64) uint64 {
	z ^= z >> 33
	z *= 0xff51afd7ed558ccd
	z ^= z >> 33
	z *= 0xc4ceb9fe1a85ec53
	z ^= z >> 33
	return z
}

// Hash01 hashes (seed, salt, x, y) into [0,1). 坐标按 int64 二进制补码
// 折进 hash——负坐标与正坐标同样良定义.
func Hash01(seed, salt string, x, y int) float64 {
	h := mix64(fnv1a(seed) ^ fnv1a(salt))
	h = mix64(h ^ uint64(int64(x)))
	h = mix64(h ^ uint64(int64(y)))
	// 取高 53 位 / 2^53：double 精度下精确可表示，值域 [0,1).
	return float64(h>>11) / float64(1<<53)
}

// Noise evaluates the deterministic value noise at (x, y) on an integer
// lattice of spacing period. 格点值为 Hash01(seed, salt, 格点)，格内
// smoothstep(3t²-2t³) + 双线性插值；period < 1 视为 1.
func Noise(seed, salt string, x, y float64, period int) float64 {
	p := float64(period)
	if p < 1 {
		p = 1
	}
	gx := math.Floor(x / p)
	gy := math.Floor(y / p)
	fx := x/p - gx
	fy := y/p - gy
	ix, iy := int(gx), int(gy)

	v00 := Hash01(seed, salt, ix, iy)
	v10 := Hash01(seed, salt, ix+1, iy)
	v01 := Hash01(seed, salt, ix, iy+1)
	v11 := Hash01(seed, salt, ix+1, iy+1)

	sx, sy := smoothstep(fx), smoothstep(fy)
	top := v00 + (v10-v00)*sx
	bot := v01 + (v11-v01)*sx
	return top + (bot-top)*sy
}

// smoothstep is the 3t²-2t³ easing — 一阶导数在格点处为零，地貌沿格点
// 平滑过渡而非折线突跳.
func smoothstep(t float64) float64 { return t * t * (3 - 2*t) }
