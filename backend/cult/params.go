// Package cult 实现修炼世界的确定性引擎核（stage_3 数值模型，M1 WP-1）。
//
// 包不知道 LLM、感知解析或功法 Protocol 的存在——它只依据
// 角色（stage_3 §3）、穴位（§4）、经脉（§5）、环境（§6）与
// 每 Tick 的操作参数，依 §7-24 的顺序生产因果。
package cult

// ---------------------------------------------------------------------------
// 定点标度约定（M1 计划 §1，全包统一）
//
//   - 量（qi / capacity / flow / amount）：×1000 整数，1.0 = 1000，
//     典型域 0 ~ 30000。
//   - 比率（0~1：openness / stability / activity / damage / retention …）：
//     ×1000。
//   - 亲和（0.5~1.5）：×1000，即 500 ~ 1500。
//   - 乘法 fpMul(a,b) = a*b/1000，int64 中间量；长乘法链逐对折叠
//     （≤6 因子安全，避免中间量溢出与精度跳变）。
//   - 开方 fpSqrt(x) = isqrt(x*1000)（千分位精度，整数运算）。
//   - 除法 fpDiv(a,b) = a*1000/b，调用方保证 b≠0。
//   - 控制误差 ε：noise(seed, tick, acupoint) ∈ [-1000,1000]，
//     线性映射到 ±ControlErrorMax——不用全局 RNG（stage_3 §8.2）。
//
// 所有数值均为实验初值（stage_3 §52）；公式关系与因果方向才是
// 不可变规则。改动参数只改本文件。
// ---------------------------------------------------------------------------

// One 是定点 1.0（千分位标度）。
const One = 1000

// 五行下标（stage_3 §2.1，顺序固定）。
const (
	Wood = iota
	Fire
	Earth
	Metal
	Water
	ElementCount
)

// ShengSrc[e] 是相生关系中生成 e 的源元素（木→火→土→金→水→木，§10）。
var ShengSrc = [ElementCount]int{Water, Wood, Fire, Earth, Metal}

// ShengDst[e] 是相生关系中 e 生成目标元素。
var ShengDst = [ElementCount]int{Fire, Earth, Metal, Water, Wood}

// KeTarget[e] 是 e 所克的目标元素（木克土、土克水、水克火、火克金、金克木，§11）。
var KeTarget = [ElementCount]int{Earth, Metal, Water, Wood, Fire}

// KeSource[e] 是克 e 的源元素。
var KeSource = [ElementCount]int{Metal, Water, Fire, Wood, Earth}

// 相生转化的处理顺序：按元素下标依次作为源（顺序固定保证确定性）。
var shengOrder = [ElementCount]int{Wood, Fire, Earth, Metal, Water}

// ---------------------------------------------------------------------------
// stage_3 §7-24 默认参数
// ---------------------------------------------------------------------------

const (
	// ControlErrorBase 是 ControlErrorMax = 0.20×(1-Control) 的基数（§3 control）。
	ControlErrorBase = 200 // 0.20

	// BaseShengRate 相生转化速率 0.04/Tick（§10）。
	BaseShengRate = 40 // 0.04

	// ShengEfficiency 相生转化效率 0.90（§10.1）。
	ShengEfficiency = 900 // 0.90

	// BaseKeRate 相克压制速率 0.15（§11）。
	BaseKeRate = 150 // 0.15

	// KeEpsilon 是 CounterRatio 分母中的 epsilon，防除零（§11）。
	KeEpsilon = 1 // 0.001

	// ActivityRecoveryRate 活性自然恢复 0.10/Tick（§12）。
	ActivityRecoveryRate = 100 // 0.10

	// BaseDiffuseRate 灵气逸散 0.02/Tick（§13）。
	BaseDiffuseRate = 20 // 0.02

	// WOverload Strain 中过载权重 0.60（§17）。
	WOverload = 600 // 0.60

	// WAffinity Strain 中五行不适权重 0.20（§17）。
	WAffinity = 200 // 0.20

	// WShock Strain 中流量冲击权重 0.20（§17）。
	WShock = 200 // 0.20

	// TransformHeatMax 是 TransformHeat 归一化上限 0.2（§17：0~0.2）。
	TransformHeatMax = 200 // 0.2

	// BaseRecovery 稳定性恢复 0.01/Tick（§18）。
	BaseRecovery = 10 // 0.01

	// EarthStabilizationBonus 土性稳定加成 0.20（§18）。
	EarthStabilizationBonus = 200 // 0.20

	// DamageThreshold 稳定性低于此值开始损伤 0.40（§19）。
	DamageThreshold = 400 // 0.40

	// DamageRate 损伤速率 0.03（§19）。
	DamageRate = 30 // 0.03

	// CapacityDamageFactor 损伤对容量的减半折算 0.5（§20）。
	CapacityDamageFactor = 500 // 0.5

	// LowPressureThreshold 自然恢复的压力条件 Pressure<0.4（§21）。
	LowPressureThreshold = 400 // 0.4

	// LowStrainThreshold 自然恢复的应变条件 Strain<0.05（§21）。
	LowStrainThreshold = 50 // 0.05

	// BaseDamageRecovery 自然损伤恢复 0.001/Tick（§21）。
	BaseDamageRecovery = 1 // 0.001

	// AdaptationSafeStability 适应增长需 Stability>=0.75（§22）。
	AdaptationSafeStability = 750 // 0.75

	// AdaptationSafePressure 适应增长需 Pressure<=0.8（§22）。
	AdaptationSafePressure = 800 // 0.8

	// MinimumEffectiveFlow 适应增长需 QiFlow[e] 高于此流量（§22）。
	MinimumEffectiveFlow = 1 // 0.001

	// AdaptationRate 适应增长速率 0.0005/Tick（§22）。
	// 慢速成长参数用微标度系数存储：gain_micro = rate × s_milli × g_milli
	// / MicroOne，其中 s/g 为千分位因子（见 tick.go）。
	AdaptationRate = 500 // 0.0005 × 10^6

	// AdaptationMax 适应上限 1.25（§22）。
	AdaptationMax = 1250 // 1.25（千分位，与微标度字段换算时除以 1000）

	// OpenSafePressureLo Openness 成长的适宜压力下限 0.30（§23）。
	OpenSafePressureLo = 300 // 0.30

	// OpenSafePressureHi Openness 成长的适宜压力上限 0.75（§23）。
	OpenSafePressureHi = 750 // 0.75

	// OpenSafeStability Openness 成长需 Stability>=0.80（§23）。
	OpenSafeStability = 800 // 0.80

	// OpenRate Openness 成长速率 0.0003/Tick（§23，微标度系数）。
	OpenRate = 300 // 0.0003 × 10^6

	// ChannelAdaptMax 经脉适应上限 1.30（§24）。
	ChannelAdaptMax = 1300 // 1.30

	// ChannelAdaptRate 经脉适应速率 0.0003/Tick（§24，微标度系数）。
	ChannelAdaptRate = 300 // 0.0003 × 10^6

	// MicroOne 是慢速成长参数（10^-6 量级）使用的微标度单位。
	MicroOne = 1000000

	// NaturalActivity 是活性恢复的目标值（§12 的 NaturalActivity）。
	// stage_3 未钉死该值；v0.1 取满活性 1.0——被克压制的活性最终回到全开。
	NaturalActivity = One

	// TimeFactor 是 §10 相生公式的时间因子，v0.1 每 Tick 恒为 1。
	TimeFactor = One

	// ShockEpsilon 是 §15 FlowShock 分母的 epsilon（防除零，千分位量）。
	ShockEpsilon = 1

	// BodyRecoveryModifier 是 §21 自然恢复的体质修正，v0.1 恒为 1
	//（stage_3 未定义体质字段，留作扩展点）。
	BodyRecoveryModifier = One

	// DiffuseShare 是 §9 传输损耗中归入 Diffuse 的比例 0.5（另 0.5 → 热）。
	DiffuseShare = 500 // 0.5

	// ---------------------------------------------------------------------------
	// 修订包（docs/specs/2026-09-21-stage3-revision-circulation-refinement.md）
	// ---------------------------------------------------------------------------

	// BaseWork 是逆流做功基数（A2）：每单位逆流传输量的做功代价系数 0.20。
	// 单位代价 = BaseWork × (1 + LoadRatio_dst × KBack) × (1 - Conductance)。
	BaseWork = 200 // 0.20 [v0.1 参数]

	// KBack 是逆流做功的目的地负载比敏感系数（A2）：推入越满，做功越贵。
	KBack = 500 // 0.5 [v0.1 参数]

	// KHeat 是逆流做功 → 热的转化比（A2）：ReverseHeat = ReverseWorkCost × KHeat，
	// 加到逆流目的地穴位的 ThermalLoad。
	KHeat = 500 // 0.5 [v0.1 参数]

	// HeatDissipation 是穴位 ThermalLoad 的每 Tick 散热衰减率（A2/B4）0.10。
	HeatDissipation = 100 // 0.10 [v0.1 参数]

	// KRefine 是炼化对逸散的敏感化乘子（D4-1）：Diffuse × (1 - Refine×KRefine)。
	KRefine = 500 // 0.5 [v0.1 参数]

	// KStab 是炼化对稳定性恢复的敏感化乘子（D4-2）：恢复 × (1 + Refine_avg×KStab)。
	KStab = 300 // 0.3 [v0.1 参数]

	// KTransform 是炼化对相生转化的敏感化乘子（D4-3）：Transform × (1 + Refine×KTransform)。
	KTransform = 300 // 0.3 [v0.1 参数]

	// KControl 是炼化对控制误差的敏感化乘子（D4-5）：ControlErrorMax × (1 - Refine×KControl)。
	KControl = 300 // 0.3 [v0.1 参数]

	// KShock 是炼化对流量冲击的敏感化乘子（D4-4）：FlowShock_effective × (1 - Refine×KShock)。
	KShock = 500 // 0.5 [v0.1 参数]
)

// RuleVersion 是底层规则版本标识（宪法 §18 Rule Versioning）。
// 0.1.0 → 0.2.0（2026-09-21-circulation-refinement）：逆流做功/逐元素筛选/
// 炼化乘子，Rule Semantic Change。
// 0.2.0 → 0.3.0（修订包 A1 v0.2 主环拓扑）：逆流发起判据由"纯汇点（无出向
// 经脉）"改为核点判据（CorePointOf，Capacity 最大穴位），Rule Semantic
// Change 而非 Parameter Tuning。
// 同 World 状态 + 同 Ops + 同 RuleVersion + 同 Seed ⇒ 逐位一致重演。
const RuleVersion = "0.3.0-mainloop"

// ---------------------------------------------------------------------------
// 定点算术
// ---------------------------------------------------------------------------

// fpMul 返回定点 a×b（千分位标度），四舍五入到最近千分位。
// 负数按远离零取整（半入），保证 |fpMul(a,b)-a*b| ≤ 1/2000。
func fpMul(a, b int64) int64 {
	p := a * b
	if p >= 0 {
		return (p + One/2) / One
	}
	return (p - One/2) / One
}

// fpDiv 返回定点 a/b，四舍五入到最近千分位。b 必须非零。
func fpDiv(a, b int64) int64 {
	p := a * One
	if (p >= 0) == (b >= 0) {
		return (p + b/2) / b
	}
	return (p - b/2) / b
}

// fpClamp 把 x 截到 [lo, hi]。
func fpClamp(x, lo, hi int64) int64 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

// fpSqrt 返回定点 x 的平方根（isqrt(x*1000)，千分位精度）。
// x 必须非负。
func fpSqrt(x int64) int64 {
	if x <= 0 {
		return 0
	}
	return isqrt(x * One)
}

// isqrt 返回 floor(sqrt(n))，牛顿迭代，n≥0。
func isqrt(n int64) int64 {
	if n < 2 {
		return n
	}
	x := n
	y := (x + 1) / 2
	for y < x {
		x = y
		y = (x + n/x) / 2
	}
	return x
}
