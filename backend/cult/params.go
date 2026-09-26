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

	// BaseDiffuseRate 是边界交换泄出方向的通透率（0.6.0 起，评审稿 §5.1
	// ExchangeFlow 的泄出实现常数；原 §13 逸散速率并入边界交换泄出方向，
	// 定稿裁决②——去向=环境不变，D3「留与散」与 D4-1 炼化抗逸散的乘子
	// 结构原样）：
	//
	//	泄出 = max(0, 体内势 − 环境势) × BaseDiffuseRate × (1−Retention_eff)
	//	       × Activity × (1 − Refine×KRefine)
	//
	// 环境势为零时退化为旧 §13 公式（泄出 ∝ 存量）。环境势 ≥ 体内势时
	// 泄出为零——势差为零即零净流（评审稿 §28.5）。
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

	// HeatDissipation 是穴位 ThermalLoad 的每 Tick 散热衰减率（A2/B4）。
	// [0.7.1 标定回填 0.10 → 0.30] 依据：千级扫描探针——散热率决定身体
	// 的「热安全环流上界」（持续应变 ≤ 自然恢复 13 ⇒ 传输损耗热
	// 2.5×f×(1−eff) ≤ 0.0195×Throughput，见 cultmc teachAmount）：0.10 时
	// 热安全流量比可学习流量低一个量级，中位画像的反馈耦合在游戏时标内
	// 不可成形（热预算把教学流量锁死在信号链死区）；0.30 把中位画像的
	// 热安全流量抬进可学习带（教学 ~700 tick 成形），热稳态响应也从
	// ~10 tick 收敛加快到 ~3 tick（火候手感更跟手）。P5 逆流火候复测
	// 单调性保持（见 cultmc 报告 P5）。
	HeatDissipation = 300 // 0.30 [v0.1 参数→标定回填]

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

	// KRecvShare 是被动流转的受端缓充份额封顶（0.7.1 标定新增
	// [v0.1 参数]）：受端「新受纳」部分（受纳余量 × KRecvShare）与源端
	// 「压而不泻」（KQiShare）对称——「受不至胀」。与受端过流份额
	//（存量 × KQiShare，环流穿过满穴用）分列：缓充是穴位「咽气入藏」
	// 的速率，必须温和到不打 §15 FlowShock（受端以空穴受充时
	// FlowShock ≈ 5×KRecvShare×CapEff/Throughput ≈ 0.1，应变 < 自然
	// 恢复——静养应无伤，0.6.0 注记④在输运维的延伸）。取值由 cultmc
	// 千级扫描与 M1 引气黄金路径探针标定（0.7.1 前 10% 份额使空穴
	// FlowShock 0.8，稳定崩→永久损伤）。
	KRecvShare = 20 // 0.02 [v0.1 参数]

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

	// ---------------------------------------------------------------------------
	// 阶段二收尾块「势差版统一边界交换」（0.6.0；定稿
	// docs/specs/2026-09-26-engine-drive-foundation-v1.md 裁决② + 阶段二
	// 范围 1/2；公式来源：评审稿
	// docs/cultivation_autonomous_loop_design_v0.2.md §5 BoundaryExchange、
	// §5.2 与主动吐纳的关系、§14.2、§28.5 防免费吸气）
	// ---------------------------------------------------------------------------

	// KPermBase 是边界交换纳入方向的被动通透性基线（评审稿 §5.1
	// Permeability 的无意识分量 [v0.1 参数]）：无意识参与下环境势高于
	// 体内势时仍发生的自然纳入（§5.3「人体自然吐纳」；§26.1 ManualDrive=0
	// 时 ExchangeFlow > 0 的物理来源）。取值约束：
	//   ① KPermBase + KPermDrive×0.6 = 0.60 = 旧 §7 intensity 档——黄金
	//     用例第一 Tick 纳入总量与旧 §7 逐位一致；
	//   ② 足够小，使自然吐纳只「维持」不「充盈」——SpikeWorld 标定下
	//     行环穴位静息负载远低于转化热承压线（§17/§18 平衡不被被动
	//     纳入打破，T-J 系列里程碑链行为保持）。
	KPermBase = 2 // 0.002 [v0.1 参数]

	// KPermDrive 是意识驱动对纳入通透性的放大系数（评审稿 §5.2：主动
	// 吐纳不是另一套物理，只是意识驱动改变 Permeability/EffectiveOpenness
	// [v0.1 参数]）：
	//
	//	perm = KPermBase + KPermDrive × intensity
	//
	// 放大只作用于纳入方向——泄出由 Retention/Refine 门控（结构保留度，
	// 不随意识开合；实现口径见 tick.go 边界交换注释）。
	KPermDrive = 996 // 0.996 [v0.1 参数]（与 KPermBase 配平黄金用例，见上）
	// ---------------------------------------------------------------------------
	// 阶段三第一块「局部反馈耦合」（0.7.0；定稿
	// docs/specs/2026-09-26-engine-drive-foundation-v1.md 阶段三范围；
	// 公式来源：评审稿
	// docs/cultivation_autonomous_loop_design_v0.2.md §7 反馈耦合、
	// §12 教师/误差学习、§13 自主命令、§24.3 数据结构示意）
	// ---------------------------------------------------------------------------

	// KFeedAdapt 是耦合的误差学习速率 AdaptRate（评审稿 §12.8 [v0.1 参数]）：
	// ΔStrength = KFeedAdapt × TeacherGate × InputSignal × SafetyGate × Error。
	// 耦合不是次数成长（§12.1 禁令）：增长只来自「目标 − 预测」的误差驱动，
	// 预测逼近目标后更新量自然趋零（§12.9），且耦合无误差样本时不衰减
	// 也不成长。
	// [0.7.1 标定回填 80 → 400] 依据：千级扫描探针——在热安全教学流量
	//（cultmc teachAmount：f ≈ 0.0078×minTP/(1−eff̄)）下 gate×signal ≈
	// 0.3×0.02，初值 80 使耦合
	// 成形需 4000+ tick（超出游戏时标三个量级）；400 时温和教学
	// 150~300 tick 成形，且发散界 K×gate×signal×safety×LR < 1 在全参数域
	// 成立（极限 0.4×1×1×0.95×2 = 0.76 < 1，饱和门+信号满值也不发散）。
	KFeedAdapt = 400 // 0.4 [v0.1 参数→标定回填]

	// TeacherDriveRef 是教学参考基准（评审稿 §12.3 [v0.1 参数]）：
	// TeacherGate = saturate(手动驱动实流 / TeacherDriveRef)。达到正常教学
	// 强度即近满值，继续猛推不再增加——饱和而非线性增长（§26.8），禁止
	// 「主动驱动越大学得越快」的乘法（§12.1）。
	// [0.7.1 标定回填 2000 → 300，两轮] 依据：千级扫描探针——①教学流量
	// 受身体热预算约束（持续应变 ≤ 自然恢复 ⇒ f ≈ 0.0078×minTP/(1−eff̄)，
	// 随机画像中位 ≈ 90~230/元素），初值 2000 使教师门在热安全教学下只开
	// 0.05~0.15，反馈耦合不可成形；②筑基的自主环路增益 = S×LR×eff ≡ S
	//（LR 习得值 = 1/eff，恒等），S→1 的收敛时标 = 1/(K×gate×sig)——
	// ref=600 时中位画像仍需 3000+ tick；ref=300 把中位画像的筑基时标
	// 压到 ~600-1200 tick（教师门在热安全教学下近饱和，画像差异由输入
	// 信号 sig = 0.0078×eff/(1−eff) 承载），弱画像仍受热预算门控。
	TeacherDriveRef = 300 // 0.3 [v0.1 参数→标定回填]

	// KSigEMA 是输入信号与教学档位记忆的 EMA 降噪速率（评审稿 §12.2
	// [v0.1 参数]）：Signal ← Signal + KSigEMA × (raw − Signal)。
	KSigEMA = 300 // 0.3 [v0.1 参数]

	// KFeedResp 是习得响应（目标响应的稳定估计）的学习速率（评审稿
	// §12.4/§24.3 LearnedResponse [v0.1 参数]）：只在教师门开启的安全样本
	// 上向当期目标响应收敛。
	KFeedResp = 200 // 0.2 [v0.1 参数]

	// FeedResponseMax 是目标响应/习得响应的钳制上限（评审稿 §12.4 的实现
	// 口径 [v0.1 参数]）：TargetResponse = clamp(教学出流/来流, 0, 上限)。
	// 微小来流下的 out/in 比值不得爆表进学习——猛推的「超额部分」被钳制，
	// 与教师门饱和共同封死「猛推加速学习」。
	FeedResponseMax = 2000 // 2.0 [v0.1 参数]

	// FeedTolerance 是耦合的来流容差（评审稿 §24.3 Tolerance / §26.11
	// [v0.1 参数]）：归一化输入信号偏离教学档位记忆（LastInputSignal）
	// 超过它时，身体「认不出」该来流模式，不产生自主命令——有容差内
	// 来流才响应（局部内化不是万能开关）。
	FeedTolerance = 500 // 0.5 [v0.1 参数]

	// MaxFeedLatency 是耦合来流滚动历史的深度上限（评审稿 §24.3
	// LatencyTicks 的实现容量）。
	MaxFeedLatency = 4

	// DefaultFeedLatency 是新耦合的默认响应延迟（评审稿 §24.3 LatencyTicks
	// [v0.1 参数]）：1 = 响应上一 Tick 的进边来流——「来流之后」一拍续推
	//（§3.2 序列语义；> 1 时命令读取更早的来流历史档）。
	DefaultFeedLatency = 1

	// FeedSenseBand 是反馈耦合进入定性感知的强度门槛（感知映射，评审稿
	// §22 [v0.1 参数]）：耦合强度达到它、且该转移确有来流时，感知层才
	// 给出「气息似会自行续上」类定性词——未被内化的结构不预 render
	//（感知隔离律）。
	FeedSenseBand = 500 // 0.5 [v0.1 参数]

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
// 0.5.0 → 0.6.0（2026-09-26 阶段二收尾块「势差版统一边界交换」，定稿
// 裁决②）：①新增边界交换通用原语（评审稿 §5.1 定点化）：穴位与环境按
// 势差双向交换，环境势=Environment.Qi、体内势=穴位存量；②原 §7 灵气
// 吸收改为「意识驱动放大既有边界交换」（放大纳入方向通透性，评审稿
// §5.2），不再是独立于世界物理的第二套吸气；无意识参与的被动纳入基线
// 单列（守恒账扩 ExchangeIn 项）；③原 §13 逸散并入边界交换泄出方向
// （去向=环境、D3/D4-1 乘子结构不变，驱动量由存量改为势差；环境势为零
// 时与旧公式逐位一致）；④途经纳气保持流动调制形态不变（裁决②）；
// ⑤守恒恒等式扩 ExchangeIn 项。吸收饱和机制从「纯反压抑制」变为
// 「势差闭合 + 反压抑制」双重——满则吸不进（D6）在结构上更强——Rule
// Semantic Change。
// 0.6.0 → 0.7.0（2026-09-26 阶段三第一块「局部反馈耦合」，定稿
// specs/2026-09-26-engine-drive-foundation-v1.md 阶段三范围）：①新增局部
// 反馈耦合 FeedbackCoupling（评审稿 §7.3/§24.3）：耦合只挂在「进边→出边」
// 的局部转移上（某穴有稳定来流 → 该穴沿其出边续推），引擎状态中不存在
// 整条周天路径的耦合值（§8/§26.19——路径属于知识与协议层）；②教师/误差
// 学习（评审稿 §12 全公式定点化）：输入信号=来流/通量比 EMA、教师门饱和
// （猛推不加速学习）、误差驱动更新（预测逼近目标更新量趋零）、安全门
// （失稳即停学）；禁次数成长、禁「主动驱动越大学得越快」的乘法；③耦合
// 产生的是响应命令，必须经统一驱动求解方成真实自主驱动（评审稿 §7.2/
// §13）——驱动容量限幅、可用能量收敛、做功、废热应变与手动同管线，
// 耦合不是隐藏能源；做功台账 AutonomousWork 接入，内化度随之自然生效；
// ④受控流不伪装自主：出边被主动请求占用的元素不产生自主命令（T-J3
// 语义）——Rule Semantic Change。
// 0.7.0 → 0.7.1（2026-09-27 Monte Carlo 标定块，任务书授权「先修物理再
// 回填」）：被动输运加受端反压门控（1/(1+LR_dst²)，与 §7 边界交换纳入
// 方向同形、无新参数）。归因：惯性项越过压差均衡点继续输运（被动永不逆
// 天然向，无反向刹车），而 KQiShare 只闸源端——下游小穴「进得来、送不
// 出」，被环流缓慢灌过 LR 1.0（应变→稳定崩→永久损伤）。M1 引气黄金路径
// （absorb@青关 + guide 青关→丹田）实测 124 tick 内土府 damage 999——
// 无意识参与的开局体验自毁邻穴，违反 0.6.0 注记④「静养应无伤」世界
// 语义约束在输运维的延伸。门控只作用被动流：主动引气灌胀脆穴（神庭
// large）是 M2 设计的「不适→自主调整」触发面，不受此门——Rule
// Semantic Change。
// 同 World 状态 + 同 Ops + 同 RuleVersion + 同 Seed ⇒ 逐位一致重演。
const RuleVersion = "0.7.1-passive-recv-gate"

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
