package engine

// Essence is 五行本源 (docs/world_laws.md §5): the five stable forms of
// condensed qi. 木=少阳 火=老阳 土=和 金=少阴 水=老阴.
type Essence int

const (
	Wood Essence = iota // 木 曲直
	Fire                // 火 炎上
	Earth               // 土 稼穑
	Metal               // 金 从革
	Water               // 水 润下
)

var essenceNames = [...]string{"木", "火", "土", "金", "水"}

// String returns the Chinese name of the essence.
func (e Essence) String() string {
	if e < Wood || e > Water {
		return "?"
	}
	return essenceNames[e]
}

// All lists the five essences in cycle order 木火土金水.
func All() []Essence { return []Essence{Wood, Fire, Earth, Metal, Water} }

// Generates reports 相生: a 生 b (world_laws §5 相生表).
// 木生火，火生土，土生金，金生水，水生木.
func Generates(a, b Essence) bool { return (a+1)%5 == b }

// Restrains reports 相克: a 克 b (world_laws §5 相克表).
// 木克土，土克水，水克火，火克金，金克木.
func Restrains(a, b Essence) bool { return (a+2)%5 == b }

// CurveZone is a segment of the 克曲线 侮—微—衡—乘 (world_laws §5).
type CurveZone int

const (
	ZoneWu CurveZone = iota // 侮 反噬：克制失效且自伤
	ZoneWei                 // 微 杯水车薪：效果趋零
	ZoneHeng                // 衡 线性克制
	ZoneCheng               // 乘 克太过：失控放大并溢出
)

var zoneNames = [...]string{"侮", "微", "衡", "乘"}

// String returns the Chinese name of the zone.
func (z CurveZone) String() string {
	if z < ZoneWu || z > ZoneCheng {
		return "?"
	}
	return zoneNames[z]
}

// Curve parameters (world_laws §7 参数表).
const (
	ZoneWuUpper    Fixed = 250  // r < 0.25 → 侮
	ZoneWeiUpper   Fixed = 500  // r < 0.50 → 微
	ZoneHengUpper  Fixed = 2000 // r < 2.00 → 衡
	HengMult       Fixed = 1500 // 衡区 ×1.5
	ChengMult      Fixed = 2500 // 乘区 ×2.5
	ChengSpill     Fixed = 250  // 乘区溢出 25%
	ChengSpillR4   Fixed = 4000 // r ≥ 4 溢出翻倍
	ChengSpillR4V  Fixed = 500
	WuBacklash     Fixed = 500 // 侮区反噬 = 自身作用强度 ×0.5
	NeutralMult    Fixed = 500 // 非克关系作用 ×0.5
)

// ChengCapPermill is the 乘区封顶 (world-laws/0.2 平衡修订)：单次作用
// 的伤害上限 = 目标其质（MaxIntegrity）× 600‰。乘依然致命（两击可毙
// 满形之敌），但不再一击秒杀——死亡窗口的存在让 Influence 紧急暂停
// 与战术撤退有介入空间。
const ChengCapPermill = 600

// capDamage folds one raw damage value by the 其质封顶：单次作用 ≤
// 目标其质 × ChengCapPermill（world-laws/0.2）。设计意图：乘区保留
// 失控放大的语义，但任何一击都可存活。
func capDamage(raw, targetStrength int) int {
	if cap := targetStrength * ChengCapPermill / 1000; raw > cap {
		return cap
	}
	return raw
}

// RestraintResult is the outcome of applying essence a with power p
// onto essence b (the 克曲线 evaluated at r = p / target strength).
type RestraintResult struct {
	Zone     CurveZone
	Damage   int // 对目标的作用量（点）
	Backlash int // 反噬（作用方自伤，侮区）
	Spill    int // 溢出比例（千分比），作用于邻近实体
}

// ResolveRestraint evaluates the 克曲线 for a power applied onto a
// target whose 本源强度 is targetStrength (both in integer points).
// Damage/backlash are returned in 形-loss points: the raw curve value
// is folded by 1/10 so that a full-power (1000) hit in the 衡 zone
// costs 150 形 — deadly to a 100-形 agent only at close to full power.
// All damage is additionally folded by the 其质封顶 capDamage
// (world-laws/0.2)：单次作用 ≤ 其质 ×0.6，乘依然致命但可存活.
//
// 推论 (world_laws §5)：动手前必须评估双方本源强度——
// 估错方向，轻则无效（微），重则反噬（侮）或失控（乘）。
func ResolveRestraint(power, targetStrength int) RestraintResult {
	if targetStrength <= 0 || power <= 0 {
		return RestraintResult{Zone: ZoneWei}
	}
	r := Fdiv(Fixed(power), Fixed(targetStrength))
	p := Fixed(power)
	switch {
	case r < ZoneWuUpper: // 侮：反噬
		return RestraintResult{Zone: ZoneWu, Backlash: int(Fmul(p, WuBacklash) / 10)}
	case r < ZoneWeiUpper: // 微：杯水车薪
		return RestraintResult{Zone: ZoneWei}
	case r < ZoneHengUpper: // 衡：线性克制 ×1.5
		return RestraintResult{Zone: ZoneHeng, Damage: capDamage(int(Fmul(p, HengMult)/10), targetStrength)}
	default: // 乘：失控 ×2.5 + 溢出（封顶后不再一击秒杀，world-laws/0.2）
		spill := ChengSpill
		if r >= ChengSpillR4 {
			spill = ChengSpillR4V
		}
		return RestraintResult{Zone: ZoneCheng, Damage: capDamage(int(Fmul(p, ChengMult)/10), targetStrength), Spill: int(spill)}
	}
}

// ApplyPower computes the outcome of projecting an essence with power
// onto a target of essence b. 相克走克曲线；非克关系一律 ×0.5
// （world_laws §4 律五：只有生克二门，生不是作用方式）。伤害同样受
// 其质封顶约束（world-laws/0.2，与克曲线同规则）.
func ApplyPower(a, b Essence, power, targetStrength int) RestraintResult {
	if Restrains(a, b) {
		return ResolveRestraint(power, targetStrength)
	}
	if power <= 0 || targetStrength <= 0 {
		return RestraintResult{Zone: ZoneWei}
	}
	return RestraintResult{Zone: ZoneHeng, Damage: capDamage(int(Fmul(Fixed(power), NeutralMult)/10), targetStrength)}
}

// 阴阳曲线 (world_laws §3). x is the 阳性输出强度 in permille (0..1000):
// 效果 ∝ x，耗气 ∝ x，生热 ∝ x²，显迹 ∝ x².

// EnergyCost returns the energy cost of an output at strength x.
// Full power (1000) costs 10 energy.
func EnergyCost(x int) int { return x / 100 }

// HeatGain returns the heat generated at strength x (∝ x²).
// Full power generates 10 heat — 越接近极限，热得越凶.
func HeatGain(x int) int { return x * x / 100000 }

// SignalEmission returns the trace emitted at strength x (∝ x²,
// world_laws 律六：任何行为必留其迹).
func SignalEmission(x int) int { return HeatGain(x) }

// Environment constants (world_laws §3 互根律 + §7 参数表).
const (
	// PassiveCooling is the per-tick heat decay of any entity.
	PassiveCooling = 2
	// OverheatThreshold: heat above this damages Integrity each tick (阳极自毁).
	OverheatThreshold = 25
	// OverheatDamage: integrity lost per tick while overheated.
	OverheatDamage = 2
	// DangerEnergyDrain / DangerHeat: 火阳过盛 cells drain and heat (world_genesis §8).
	// 0.2 平衡修订：DangerHeat 3 → 2（行进净热归零，危险区可穿越、
	// 不可驻留）；DangerEnergyDrain 2 → 1（危险格蓄息净 +1：驻留可
	// 自愈脱困，但仍劣于平地——若为 2，则与 RestRegen 恰好抵消，低气
	// 者踏上危险格即被永久锁死）.
	DangerEnergyDrain = 1
	DangerHeat        = 2
	// RestRegen is the 蓄息律 (world-laws/0.2 新增)：本 tick 未执行任何
	// move/activate/equip（站桩不动即阴）且气未满 → 回 RestRegen 气。
	// 阴阳互根（world_laws 律一）：阳动耗气生热，阴静蓄气回神——停留
	// 有代价也有收益，深探-折返-再深探的节奏由此成立。怪物同规则
	// （对称物理）.
	RestRegen = 2
	// StallThreshold: 输出低于此视为不作为 (world_laws §7 停滞阈).
	StallThreshold = 50
)
