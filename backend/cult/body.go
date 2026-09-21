package cult

// body.go —— 修炼世界的数据结构（stage_3 §3-6）。
//
// 标度约定见 params.go 顶部注释：
//   量（qi/capacity/flow）与比率（openness/stability/…）为千分位（×1000）；
//   Adaptation / Openness / ChannelAdaptation 增长极慢（每 Tick ~10^-4），
//   以微标度（×10^6）存储避免逐 Tick 截断为 0（见 MicroOne）。

// Character 是修炼者（stage_3 §3）。
type Character struct {
	// Affinity[e] 先天五行亲和，千分位，普通范围 750~1250（§2.2/§3）。
	Affinity [ElementCount]int64
	// Control ∈ [0,1000]，影响流量控制误差（§3 control）。
	Control int64
	// Perception ∈ [0,1000]，感知强度（WP-2 使用，本包只携带）。
	Perception int64
	// BodyGraph 是身体结构：穴位与经脉（§4/§5）。
	BodyGraph BodyGraph
}

// BodyGraph 是角色的身体结构。
type BodyGraph struct {
	// Acupoints 是全部穴位；Channel 的 From/To 为其下标。
	Acupoints []Acupoint
	// Channels 是全部经脉（有向：qi 只沿 From→To 流动）。
	Channels []Channel
}

// Acupoint 是穴位（stage_3 §4 全字段）。
type Acupoint struct {
	// Name 仅用于调试与测试报告，不参与运算。
	Name string

	// Affinity[e] 穴位五行亲和，千分位（§4）。
	Affinity [ElementCount]int64

	// Capacity 容量（量，千分位）。
	Capacity int64
	// Throughput 通量（量/Tick，千分位）。
	Throughput int64
	// Retention ∈ [0,1000] 保留度（§13）。
	Retention int64

	// Openness 开度，微标度（1.0 = 1e6，§23 成长）。
	Openness int64
	// BaseStability 先天稳定性资质（§4 字段；v0.1 公式未使用，仅携带，
	// 留给后续功法/体质扩展）。
	BaseStability int64
	// Stability 当前稳定性 ∈ [0,1000]（§18）。
	Stability int64
	// Damage ∈ [0,1000] 结构损伤（§19-21）。
	Damage int64

	// Adaptation[e] 后天适应，微标度（1.0 = 1e6，§22 成长）。
	Adaptation [ElementCount]int64

	// Qi[e] 当前储存灵气（量，千分位）。
	Qi [ElementCount]int64
	// Activity[e] 五行活性 ∈ [0,1000]（§11-12）。
	Activity [ElementCount]int64

	// RefineMatchMin / RefineMatchMax 是炼化度归一化区间的锚定覆盖
	// （修订包 D2/SA4-③ [v0.1 参数]）：0 = 自动推导（MatchMin=亲和有效值
	// 均值、MatchMax=最大值，refinement.go）。非 0 时钉死该穴位的归一化
	// 区间——用于修真世界丹田画像：相生循环使完全单行构成不可达（木恒有
	// ~4%/Tick 转化为火），推导 MatchMax（max_e AffinityEff）高估收敛锚，
	// 区间偏宽导致炼化吸引子到不了中档（SA4 根因③）。仍是输入侧参数：
	// 公式结构不变，且逐穴位设置，保留 D2"同一股气在甲身精、乙身粗"的
	// 相对性。不参与任何序列化断言（T4 只锁 refinement 不落存储）。
	RefineMatchMin int64
	RefineMatchMax int64

	// PreviousFlow 上一 Tick 总流量（量，千分位，§15 FlowShock）。
	// CurrentFlow 只统计经脉流量（进入的 Delivered + 流出的 Actual）：
	// §15 的语境是"突然猛推一股灵气"的 Sequence 流量冲击；环境吸收是
	// 缓和交换，不构成冲击——否则黄金用例（§46-49）的首轮吸收会立即
	// 打穿稳定性并受伤，与该节的叙事矛盾。§22 的 QiFlow 同口径。
	PreviousFlow int64

	// ThermalLoad 是穴位热负荷（修订包 A2/B4：累积量，千分位）。
	// 累积来源 = 逆流做功热（reverseHeatAccum）+ 转化损耗热（heatAccum，
	// §9/§10 损耗）；每 Tick 按 HeatDissipation 衰减。是慢响应累积量，
	// 与 §17 的瞬时 TransformHeat 口径不同，二者互不替代。
	ThermalLoad int64

	// lastLoadRatioMilli 是上一 Tick 吸收后的负载比（千分位），
	// 供下一 Tick 的 BackPressure 使用——stage_3 §47→§48 的链式定义
	// （§48 直接以吸收后 LoadRatio 计算"下一 Tick BackPressure"）。
	lastLoadRatioMilli int64

	// flowAccum / elemFlowAccum 是本 Tick 流量累计，Advance 内部使用。
	flowAccum     int64
	elemFlowAccum [ElementCount]int64
	heatAccum     int64 // 本 Tick 热量累计（§9/§10 损耗 → §17 TransformHeat）

	// reverseHeatAccum 是本 Tick 逆流做功热累计（修订包 A2 → ThermalLoad），
	// Advance 内部使用。
	reverseHeatAccum int64
}

// Channel 是经脉（stage_3 §5）。
type Channel struct {
	// From/To 是 Acupoints 的下标；qi 沿 From→To 流动。
	From int
	To   int

	// Name 仅用于调试。
	Name string

	// BaseConductance 基础传导（比率，千分位；§8 中亦作为 TransferEfficiency
	// 的来源，clamp 到 [0,1]）。
	BaseConductance int64
	// Throughput 经脉通量（量/Tick，千分位）。
	Throughput int64
	// Damage ∈ [0,1000] 经脉损伤（§8）。
	Damage int64

	// ChannelAdaptation 经脉适应，微标度（1.0 = 1e6，§24 成长）。
	ChannelAdaptation int64

	// flowAccum 是本 Tick 该经脉总流量，Advance 内部使用。
	flowAccum int64
}

// Environment 是环境（stage_3 §6）。
type Environment struct {
	// Qi[e] 环境灵气密度（量，千分位；吸收公式不消耗环境值）。
	Qi [ElementCount]int64
	// Activity[e] 环境五行活性 ∈ [0,1000]（§7.2）。
	Activity [ElementCount]int64
	// Temperature 温度（千分位；v0.1 未进入任何公式，仅携带）。
	Temperature int64
}

// 有效值换算（stage_3 §20：损伤真正改变身体运行能力）。

// effectiveOpenness 返回 Openness×(1-Damage)（千分位）。
func (a *Acupoint) effectiveOpenness() int64 {
	return fpMul(a.opennessMilli(), One-a.Damage)
}

// effectiveCapacity 返回 Capacity×(1-0.5×Damage)（千分位，§20）。
func (a *Acupoint) effectiveCapacity() int64 {
	return fpMul(a.Capacity, One-fpMul(CapacityDamageFactor, a.Damage))
}

// effectiveThroughput 返回 Throughput×(1-Damage)（千分位，§20）。
func (a *Acupoint) effectiveThroughput() int64 {
	return fpMul(a.Throughput, One-a.Damage)
}

// opennessMilli 返回微标度 Openness 的千分位近似。
func (a *Acupoint) opennessMilli() int64 { return a.Openness / 1000 }

// adaptationMilli 返回微标度 Adaptation[e] 的千分位近似。
func (a *Acupoint) adaptationMilli(e int) int64 { return a.Adaptation[e] / 1000 }

// effectiveAffinity 返回 EffectiveAffinity[i,e] = Affinity×Adaptation（§8）。
func (a *Acupoint) effectiveAffinity(e int) int64 {
	return fpMul(a.Affinity[e], int64(a.adaptationMilli(e)))
}

// refineAffinity 返回炼化口径的穴位有效亲和（修订包 D2/D3 的
// AcupointAffinity[i,e] 实现口径）：Affinity × Adaptation × (1 - Damage)。
// 身体变化即重算（D2：受伤、Adaptation 变化 → 今日精纯明日变粗）。
func (a *Acupoint) refineAffinity(e int) int64 {
	return fpMul(a.effectiveAffinity(e), One-a.Damage)
}

// load 返回穴位当前总负载 Σ Qi[e]（§7.1/§14）。
func (a *Acupoint) load() int64 {
	total := int64(0)
	for e := 0; e < ElementCount; e++ {
		total += a.Qi[e]
	}
	return total
}
