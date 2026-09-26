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
	// [SA6 调参 v0.1 实测] 上调（20→40）虽放大 D3 筛选差分（P3 吸引子
	// 0.271→0.325），但同步压低主环通量，P6 中庭承压跌破可感第一档
	// （0.257→0.136）——逸散是全局项，无法只在丹田局部增强，故维持 20；
	// 筛选差分改由丹田亲和梯度 + 归一化锚定承担。
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

	// ---------------------------------------------------------------------------
	// Part J：被动环流（docs/specs/2026-09-22-partj-passive-circulation.md J2/J3/J8）
	// ---------------------------------------------------------------------------

	// KPassive 是被动输运强度（Part J J2/J8 [v0.1 参数]）：每单位压差
	// （Pressure = LoadRatio²，千分位）可驱动的通道容量比例（千分位）。
	//
	//	passiveFlow = clamp(chanCap × KPassive × ΔP × Conductance, 0, chanCap)
	//
	// 沿天然向；ΔP ≤ 0 时为 0（被动永不逆天然向）。chanCap 是经脉与两端
	// 穴位有效通量的最小值（与主动传输的 §8.1 容量同源）。[v0.1 参数]
	// J8 初值 0.3， cultmc P7/P9 标定后回填终值。
	KPassive = 300 // 0.3（标定探针中）

	// KEntrain 是途经纳气系数（Part J J3/J8 [v0.1 参数]）：气行经穴位时
	// 卷入环境气，来源=环境，计入守恒 Entrained 项。
	//
	//	entrained = F × Openness × EnvironmentQi × AcupointAffinity
	//	            × KEntrain × (1 - LoadRatio)
	//
	// 受 BackPressure（满则难纳）抑制；量级远小于主动纳气（环境灵气密度
	// ~2.4 作为乘子进入定点链，KEntrain=0.05 时实际卷入约为流量的 5%
	// 量级，随环境密度缩放）。
	KEntrain = 40 // 0.04 [v0.1 参数]（标定探针中）

	// FoundationReleaseWindow 是 SelfSustain 松手观察窗 W2 的长度（Part J
	// J5/J8 [v0.1 参数]）：松手后连续 N tick 全无请求，其均流为 W2。
	// ctmatch.FoundationMonitor 消费（J5 测量协议）。
	FoundationReleaseWindow = 30 // 30 tick [v0.1 参数]

	// KInertia 是被动输运的流量惯性系数（Part J R4 第二阶段 [v0.1 参数]）。
	// J2 纯压差项在闭环上会均衡化归零（压差快照同和为零，无泵不成环流），
	// R4 预案：被动项叠加惯性项——
	//
	//	passiveFlow = clamp(KPassive×ΔP×Conductance×chanCap
	//	                    + KInertia×FlowPrev, 0, pipeCap)
	//
	// FlowPrev 是经脉上一 Tick 的实际流量（Persistent Flow State）。
	// 结构因果（J6）：反复安全行环 → Openness/Conductance/适应上升 →
	// 梯度项与流量水平上升、衰减变慢 → SelfSustain 上升。
	KInertia = 990 // 0.99 [v0.1 参数][Part J R4]

	// KQiShare 是被动流转的源穴储气份额封顶（Part J 标定 [v0.1 参数]）：
	// 每 Tick 被动流量 ≤ 源穴位该元素存量 × KQiShare。流转过快会令压差
	// 逐 Tick 反转（弛豫振荡），经 §15 FlowShock 打穿稳定性。
	KQiShare = 100 // 0.10（标定探针中）

	// ---------------------------------------------------------------------------
	// 阶段二「驱动的代价」（docs/specs/2026-09-26-engine-drive-foundation-v1.md
	// 阶段二范围 + 裁决①；公式来源：评审稿
	// docs/cultivation_autonomous_loop_design_v0.2.md §4.4/§9.4/§10/§15/§24.6）
	// ---------------------------------------------------------------------------

	// KDriveWork 是驱动做功基数 k_work（评审稿 §10.1 [v0.1 参数]）：
	// DriveWork = k_work × ActualDrive² × Δt / DriveEfficiency（定点实现见
	// driveWorkOf）。做功从驱动穴位真实储气按元素扣除，去向=热（守恒账
	// DriveWork 项）。二次律保证温和驱动便宜、过驱昂贵——「火候」的代价面。
	KDriveWork = 2 // 0.002 [v0.1 参数]

	// SafeDriveRatio 是安全驱动比（评审稿 §10.2 [v0.1 参数]）：
	// DriveRatio = ActualDrive / EffectiveDriveCapacity 超过它才进入过驱。
	SafeDriveRatio = 800 // 0.8 [v0.1 参数]

	// KDriveOver 是过驱应变权重 k_over（评审稿 §10.2 [v0.1 参数]）：
	// DriveStrain += k_over × OverDrive²，OverDrive = max(0, Ratio−Safe)。
	// 过驱同时经做功二次律显著生热（§10.2 因果链）。
	KDriveOver = 1500 // 1.5 [v0.1 参数]

	// KDriveRamp 是驱动突变应变权重 k_ramp（评审稿 §10.2 [v0.1 参数]）：
	// DriveStrain += k_ramp × ΔDriveRatio²（只计加力方向——骤然松手由
	// §15 FlowShock 管辖，不双重惩罚）。逐穴位取最受力一路（max），
	// 见 tick.go 驱动应变归并注释。
	KDriveRamp = 200 // 0.2 [v0.1 参数]

	// CoreDriveSignificance 是核点判据的显著主导比（定稿阶段二范围 1
	// [v0.1 参数]）：驱动容量须高于其余每一穴位的该倍数才成核点——
	// 「驱动容量显著高于其余节点者」。取代 0.3.0 的「Capacity 最大」
	// 判据（宪法 §8：结构判据参数化，无专名；外部代码审查第 6 条）。
	CoreDriveSignificance = 2000 // 2.0 [v0.1 参数]
)

// RuleVersion 是底层规则版本标识（宪法 §18 Rule Versioning）。
// 0.1.0 → 0.2.0（2026-09-21-circulation-refinement）：逆流做功/逐元素筛选/
// 炼化乘子，Rule Semantic Change。
// 0.2.0 → 0.3.0（修订包 A1 v0.2 主环拓扑）：逆流发起判据由"纯汇点（无出向
// 经脉）"改为核点判据（CorePointOf，Capacity 最大穴位），Rule Semantic
// Change 而非 Parameter Tuning。
// 0.3.0 → 0.4.0（2026-09-22 Part J 被动环流）：新增压差驱动的被动输运项
// （J2）与途经纳气项（J3，守恒扩 Entrained），传输不再全由 Ops.Requests
// 驱动——Rule Semantic Change。
// 0.4.0 → 0.5.0（2026-09-26 阶段二「驱动的代价」，定稿
// specs/2026-09-26-engine-drive-foundation-v1.md）：①驱动容量独立成维
// （Acupoint.DriveCapacity，评审稿 §4.4 第四维），核点判据由「Capacity
// 最大」改为「DriveCapacity 显著主导」；②统一驱动求解（评审稿 §9.4）：
// 主动驱动做功扣真实能源（守恒账 ReverseWork 归属迁移为 DriveWork，
// 顺流另付 §10.1 二次律做功），无可用能量则实际驱动衰减；③驱动代价
// 分层（评审稿 §10.2）：过驱/突变生应变、二次做功生废热——Rule
// Semantic Change。
// 同 World 状态 + 同 Ops + 同 RuleVersion + 同 Seed ⇒ 逐位一致重演。
const RuleVersion = "0.5.0-drive-cost"

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
