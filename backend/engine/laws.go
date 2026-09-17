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
		return RestraintResult{Zone: ZoneHeng, Damage: int(Fmul(p, HengMult) / 10)}
	default: // 乘：失控 ×2.5 + 溢出
		spill := ChengSpill
		if r >= ChengSpillR4 {
			spill = ChengSpillR4V
		}
		return RestraintResult{Zone: ZoneCheng, Damage: int(Fmul(p, ChengMult) / 10), Spill: int(spill)}
	}
}

// ApplyPower computes the outcome of projecting an essence with power
// onto a target of essence b. 相克走克曲线；非克关系一律 ×0.5
// （world_laws §4 律五：只有生克二门，生不是作用方式）.
func ApplyPower(a, b Essence, power, targetStrength int) RestraintResult {
	if Restrains(a, b) {
		return ResolveRestraint(power, targetStrength)
	}
	if power <= 0 || targetStrength <= 0 {
		return RestraintResult{Zone: ZoneWei}
	}
	return RestraintResult{Zone: ZoneHeng, Damage: int(Fmul(Fixed(power), NeutralMult) / 10)}
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
	DangerEnergyDrain = 2
	DangerHeat        = 3
	// StallThreshold: 输出低于此视为不作为 (world_laws §7 停滞阈).
	StallThreshold = 50
)
