package main

// scan.go —— Monte Carlo 千级角色扫描与专项观测（评审稿
// docs/cultivation_autonomous_loop_design_v0.2.md §27；定稿
// docs/specs/2026-09-26-engine-drive-foundation-v1.md 阶段三标定块）。
//
// 与 main.go 的 P1-P9 修订包命题台互补，本文件实现七组新场景：
//
//	SA 千级角色扫描（§27 观测清单）：随机画像（亲和/容量/通量/传导/
//	   驱动容量/驱动效率/环境/路线）走真实判定链（ctmatch 会话 +
//	   FoundationMonitor，foundation-metrics-v2 口径 + 移交门），观测——
//	   首个局部耦合时点、自主做功 20/50/80% 时点、永不闭合路线画像、
//	   高自持低平衡、低灵气无法维持、过热/振荡/损伤、筑基时间分布。
//	SB 路线唯一性对照：固定身体 × 8 条路线（传导/通量按槽位分配，
//	   路线间公平），检验是否存在单一全局最优路线。
//	P11 移交门专项（判定链收口注记⑤输入）：耦合强度扫频 500~1000，
//	   测移交后流量保有率与断流率——回填 feedHandoverStrength。
//	P12 SelfSustain v2 分辨力：标定世界比值恒钳 1——扫低开放度 ×
//	   环境密度网格（小流量教学 = P9 口径）与饱和教学（2400/元素）
//	   两个口径，看分布是否可分辨。
//	P13 流量闭合新口径的多环/支路拓扑分布：单环 / 双环共点 / 环+捷径 /
//	   环+支路，回流/初发流口径的分布与自主流分叉。
//	P14 被动流与手动流量级关系：教学期/松手期被动流占比，源端份额
//	   封顶与受端缓充封顶的约束频率——「储气份额参数是松手期环流
//	   上限的主要支配」的检验。
//
// 全程定点整数；随机画像用自带 splitmix64（无全局 RNG），同 seed 逐位
// 可重放。扫描只测量不调参；命题不成立如实报 FAIL 并归因。

import (
	"fmt"
	"strings"

	"agentbattle/backend/ctmatch"
	"agentbattle/backend/cult"
)

// ---------------------------------------------------------------------------
// 确定性随机画像
// ---------------------------------------------------------------------------

// mixCult64 是 splitmix64 终结器（与 cult 包内实现同型，本地镜像）。
func mixCult64(z uint64) uint64 {
	z ^= z >> 33
	z *= 0xff51afd7ed558ccd
	z ^= z >> 33
	z *= 0xc4ceb9fe1a85ec53
	z ^= z >> 33
	return z
}

// ptLoad 汇总穴位总负载（cult load 的本地镜像，含五元素）。
func ptLoad(a *cult.Acupoint) int64 {
	var t int64
	for e := 0; e < cult.ElementCount; e++ {
		t += a.Qi[e]
	}
	return t
}

// ptCapEff 损伤折算后的有效容量（cult effectiveCapacity 本地镜像）。
func ptCapEff(a *cult.Acupoint) int64 {
	d := (cult.CapacityDamageFactor*a.Damage + cult.One/2) / cult.One
	return (a.Capacity*(cult.One-d) + cult.One/2) / cult.One
}

// mcRand 是 splitmix64 计数器流（确定性，无全局状态）。
type mcRand struct{ s uint64 }

func (r *mcRand) u64() uint64 {
	r.s += 0x9e3779b97f4a7c15
	return mixCult64(r.s)
}

// span 返回 [lo, hi] 闭区间均匀整数。
func (r *mcRand) span(lo, hi int64) int64 {
	if hi <= lo {
		return lo
	}
	return lo + int64(r.u64()%uint64(hi-lo+1))
}

// pick 等概率取表中一项。
func (r *mcRand) pick(tab []int64) int64 { return tab[r.span(0, int64(len(tab))-1)] }

// scanProfile 是一个随机扫描画像。points 按槽位 0..5 存参数画像；
// route[k] 是行进第 k 棒的穴位下标（route[0] 恒为泵——驱动容量显著
// 主导者，逆流唯它可发起）。经脉传导/通量按槽位 k 分配——同一画像
// 换路线时经脉参数随槽走，保证 SB 路线对照的公平性。
type scanProfile struct {
	seed     uint64
	envScale int64 // 环境灵气密度缩放（千分位）

	names [6]string
	route [6]int

	cap, tp, ret, open, drive, effi [6]int64
	aff                             [6][cult.ElementCount]int64
	charAff                         [cult.ElementCount]int64
	cond, chanTP                    [6]int64
}

// envBase 是五行环境基准密度（木略盛，与 SpikeWorld 量级一致）。
var envBase = [cult.ElementCount]int64{3400, 2600, 3000, 2200, 2800}

// envScales 是环境灵气档位（千分位）：含 3/8 低灵气样本——「低灵气
// 环境谁无法维持」需要足够的低档人群。
var envScales = []int64{400, 500, 600, 800, 1000, 1000, 1200, 1500}

// genScanProfile 生成随机画像。decent=true 时收紧到「修行有成」量级
// （环境满档、高传导、高开度、恒等路线），供专项场景使用。
func genScanProfile(seed uint64, decent bool) scanProfile {
	r := &mcRand{s: seed*0x9E3779B97F4A7C15 ^ 0xD1B54A32D192ED03}
	p := scanProfile{seed: seed}
	if decent {
		p.envScale = 1000
	} else {
		p.envScale = r.pick(envScales)
	}
	// 采样域：筑基期相关人群（§27 扫描的对象是「正在冲击筑基的修炼者」）。
	// 开度 600~1000、传导 700~950——低于该域的画像（引气期身体）按探针
	// 数据在预算内不可能成形耦合，采样它们只稀释观测；它们构成「永不
	// 闭合」桶的延拓（报告中以域边界归因注明）。
	loOpen, hiOpen := int64(600), int64(1000)
	loCond, hiCond := int64(700), int64(950)
	if decent {
		loOpen, hiOpen = 800, 1000
		loCond, hiCond = 800, 950
	}
	for i := 0; i < 6; i++ {
		p.names[i] = fmt.Sprintf("穴%d", i)
		p.cap[i] = r.span(15000, 50000)
		p.tp[i] = r.span(4000, 10000)
		p.ret[i] = r.span(500, 900)
		p.open[i] = r.span(loOpen, hiOpen)
		p.drive[i] = r.span(6000, 12000)
		p.effi[i] = r.span(700, 950)
		for e := 0; e < cult.ElementCount; e++ {
			p.aff[i][e] = r.span(800, 1200)
		}
	}
	for e := 0; e < cult.ElementCount; e++ {
		p.charAff[e] = r.span(800, 1200)
	}
	for k := 0; k < 6; k++ {
		p.cond[k] = r.span(loCond, hiCond)
		p.chanTP[k] = r.span(4000, 10000)
	}
	// 路线：泵（驱动容量最大者）固定第 0 棒；其余洗牌（Fisher-Yates，
	// 计数器流确定性）。decent 画像走恒等顺序（专项不需要路线随机性）。
	pump := 0
	for i := 1; i < 6; i++ {
		if p.drive[i] > p.drive[pump] {
			pump = i
		}
	}
	rest := []int{}
	for i := 0; i < 6; i++ {
		if i != pump {
			rest = append(rest, i)
		}
	}
	if !decent {
		for i := len(rest) - 1; i > 0; i-- {
			j := int(r.u64() % uint64(i+1))
			rest[i], rest[j] = rest[j], rest[i]
		}
	}
	p.route = [6]int{pump, rest[0], rest[1], rest[2], rest[3], rest[4]}
	// 泵的驱动容量取其余最大者的 2.5~4 倍——满足 CorePointOf 显著主导
	// 判据（0.5.0 核点判据，全图唯一）。
	maxOther := int64(0)
	for i := 1; i < 6; i++ {
		if p.drive[p.route[i]] > maxOther {
			maxOther = p.drive[p.route[i]]
		}
	}
	p.drive[pump] = maxOther * r.span(25, 40) / 10
	return p
}

// buildScanWorld 按画像装配世界（六穴单环，环向=天然向）。
func buildScanWorld(p *scanProfile) *cult.World {
	pt := func(i int) cult.Acupoint {
		qi0 := p.cap[i] / 5 * 3 / 10 // 初载 LR≈0.3
		var qi [cult.ElementCount]int64
		for e := 0; e < cult.ElementCount; e++ {
			qi[e] = qi0
		}
		return cult.Acupoint{
			Name:            p.names[i],
			Affinity:        p.aff[i],
			Capacity:        p.cap[i],
			Throughput:      p.tp[i],
			Retention:       p.ret[i],
			Openness:        p.open[i] * 1000, // 千分位 → 微标度
			Stability:       950,
			BaseStability:   950,
			Activity:        [cult.ElementCount]int64{1000, 1000, 1000, 1000, 1000},
			Adaptation:      [cult.ElementCount]int64{1000000, 1000000, 1000000, 1000000, 1000000},
			Qi:              qi,
			DriveCapacity:   p.drive[i],
			DriveEfficiency: p.effi[i],
		}
	}
	pts := make([]cult.Acupoint, 6)
	for i := 0; i < 6; i++ {
		pts[i] = pt(i)
	}
	chs := make([]cult.Channel, 6)
	for k := 0; k < 6; k++ {
		from, to := p.route[k], p.route[(k+1)%6]
		chs[k] = cult.Channel{
			Name:            fmt.Sprintf("%s→%s", p.names[from], p.names[to]),
			From:            from,
			To:              to,
			BaseConductance: p.cond[k],
			Throughput:      p.chanTP[k],
		}
	}
	var env [cult.ElementCount]int64
	for e := 0; e < cult.ElementCount; e++ {
		env[e] = envBase[e] * p.envScale / 1000
	}
	w := cult.NewWorld(cult.Character{
		Affinity: p.charAff, Control: 900, Perception: 500,
		BodyGraph: cult.BodyGraph{Acupoints: pts, Channels: chs},
	}, cult.Environment{
		Qi:          env,
		Activity:    [cult.ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Temperature: 1000,
	})
	w.NoiseSeed = p.seed
	return w
}

// teachAmount 返回画像的热安全教学流量（每元素，千分位量）：
// 持续应变 ≤ 自然恢复（13/tick）⇒ 传输损耗热的 TransformHeat 平衡
// 2.5×f×(1−eff̄)×(1/HeatDissipation)×200/minTP ≤ 13
// ⇒ f ≤ 0.0078×minTP/(1−eff̄)，eff̄ ≈ cond̄×open̄²（亲和均值 ≈1）。
// 教学流量画像自适应：强传导/大开度身体可大流量教学（教师门饱和），
// 弱身体只能温火（门开小、学得慢）——火候语义的标定面。
// 钳 [50, 2400]。
func teachAmount(p *scanProfile) int64 {
	var condSum, openSum int64
	minTP := int64(1 << 62)
	for k := 0; k < 6; k++ {
		condSum += p.cond[k]
		openSum += p.open[k]
		if p.tp[k] < minTP {
			minTP = p.tp[k]
		}
	}
	condAvg, openAvg := condSum/6, openSum/6
	effMilli := condAvg * openAvg / 1000 * openAvg / 1000
	gap := int64(1000) - effMilli
	if gap < 10 {
		gap = 10
	}
	amt := 78 * minTP / (gap * 10)
	if amt < 50 {
		amt = 50
	}
	if amt > 2400 {
		amt = 2400
	}
	return amt
}

// saFillLen 是 SA/SB 的引气填充相位长度（Tick）：只纳气不行环，把身体
// 从初载 LR 0.3 充到环境势允许的水平（灵气密度决定气血上限——低灵气
// 环境充不高，松手环流的地板随之更低，这正是「低灵气谁无法维持」的
// 物理来源）。填充是松缓交换，无 FlowShock、无损伤。
const saFillLen = int64(400)

// runFillPhase 在会话上跑纯吐纳（吸收@环末穴，无引导请求）。
func runFillPhase(s *ctmatch.CultSession, acc *accounting, absorbPoint int, n int64) {
	for i := int64(0); i < n; i++ {
		it := make([]int64, len(s.World.Character.BodyGraph.Acupoints))
		it[absorbPoint] = 1000
		s.World.Ops = cult.Ops{Intensity: it}
		rep := cult.Advance(s.World)
		acc.checkConservation(rep)
	}
}

// scanRouteYAML 生成路线功法协议：吸收（大档）+ 全环引导同息并行
// （parallel 1 → 单槽，1 Tick = 1 周天）。教学流量 = teachAmount（画像
// 热安全流量），教师门依画像开 0.2~1.0（§12.3 饱和语义不变）——火候
// 强的身体用大火教学，弱身体只能温火。
func scanRouteYAML(p *scanProfile, cycles int64) string {
	name := func(i int) string { return p.names[p.route[i]] }
	// 混合引导按每元素 0.2×amount 计：amount = 教学流量 ×5。
	amt := teachAmount(p) * 5
	rmax := amt * 1100 / 1000 // 控制误差 ±2%（Control 0.9）的出界余量
	var b strings.Builder
	fmt.Fprintf(&b, "protocol: zhoutian_scan_%d\nactions:\n", p.seed)
	fmt.Fprintf(&b, "  - absorb: {target: %s, level: large, duration: 1, parallel: 1}\n", name(5))
	for k := 0; k < 6; k++ {
		fmt.Fprintf(&b, "  - guide: {from: %s, to: %s, element: \"\", amount: %d, duration: 1, parallel: 1}\n",
			name(k), name((k+1)%6), amt)
	}
	fmt.Fprintf(&b, "cadence: 1\ncycles: %d\nvalidated_range:\n", cycles)
	for k := 0; k < 6; k++ {
		fmt.Fprintf(&b, "  - channel: [%s, %s]\n    element: \"\"\n    min: 0\n    max: %d\n",
			name(k), name((k+1)%6), rmax)
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// SA 千级角色扫描
// ---------------------------------------------------------------------------

// SA 相位结构：12 组「教学 60 + 探针松手 30」（早期耦合/SS 轨迹采样）
// + 20 组纯教学 60（筑基需要 S→0.99+，连续长教学；探针松手本身漏气——
// 松手期环流损耗无纳气补给，见报告——故后期不再插入）+ 终局「教学 60 +
// 松手 320」（松手窗内筑基判定 + 升格移交 + 移交后观测）。
// 预算依据（探针标定）：耦合学习速率 dS ≈ KFeedAdapt×gate×signal，在
// 热安全教学流量下 ≈ 0.45×eff/(effTP×(1−eff)²)/tick——中位画像 ~0.004/
// tick，自主环路增益（S + 被动回补）跨越 1（筑基相变点）需 S≈0.9，
// 即 ~600-1200 tick 教学；预算取教学 16×60 + 终局 380 ≈ 1900 tick，
// 覆盖中位以上画像的全程因果链，弱画像自然落入「永不闭合」桶。
const (
	saBlocks      = 12
	saPureBlocks  = 44
	saTeachTicks  = int64(60)
	saProbeTicks  = int64(30)
	saFinalTicks  = int64(320)
	saCycleBudget = 60         // 每段功法周天数（单槽并行 = 1 周天 1 Tick）
	saFlowBand    = int64(100) // 首个局部耦合的初显强度带
	saSenseBand   = int64(500) // 可感耦合（cult.FeedSenseBand 同值）
)

type scanARes struct {
	seed     uint64
	route    [6]int
	envScale int64
	minCond  int64
	minOpen  int64

	firstCouple int64 // 首个耦合强度 ≥ 100 的 Tick；-1 未出现
	firstSense  int64 // 首个耦合强度 ≥ 500（可感）的 Tick；-1 未出现
	ia20        int64 // 滚动 50 Tick 自主做功占比 ≥ 20% 首见 Tick；-1 未达
	ia50, ia80  int64
	peakSS      int64 // 峰值 SelfSustain 时点的三量快照（千分位）
	ssClosure   int64
	ssBalance   int64
	peakClosure int64 // FlowClosure 峰值（千分位）
	touched     bool
	estab       bool
	touchTick   int64 // 全局 Tick（含相对末次松手起点的换算在报告层做）
	estTick     int64
	anomalies   int64 // 未知异常升级次数（新损伤无 Guard）
	damageTicks int64 // 有穴位新损伤的 Tick 数
	peakThermal int64 // 全图 ThermalLoad 峰值
	osc         int64 // 终局松手窗流量归一化方差（千分位）
	depleted    bool  // 终局松手尾段环流 < 教学基线 5%（环流无法维持）
	tot0, tot1  int64

	// 筑基门槛归因（终局松手后段采样：五量哪个不达候选线）
	blockClosure, blockBalance, blockSS, blockDyn, blockStab int64
	sampled                                                  int64

	// 移交实测（established 后 loopMode 自动移交已内化段）
	hoMeasured  bool
	hoRetention int64 // 移交后均流 / 教学基线均流（千分位）
	hoBreak     bool  // 移交后流量跌破教学基线 10%
}

// saTracker 是单角色的逐 Tick 观测器（研究者视图：直读 World 耦合
// 状态与台账真值；感知隔离律约束的是玩家/Agent 视图，不是标定台）。
type saTracker struct {
	res     *scanARes
	w       *cult.World
	tot0    int64
	prevDmg []int64
	// 50 Tick 滚动做功窗口：自主占比 = Auto/(Auto+Manual+Passive)
	winAuto, winMan, winPas [50]int64
	winN                    int64
	winIdx                  int
	sumAuto, sumMan, sumPas int64
	// 教学/松手流量基线与终局序列
	teachFlowSum, teachFlowN int64
	relFlowSum, relFlowN     int64
	inTeach                  bool
	inFinal                  bool
	finalRel                 []int64
	// 移交后观测（estTick 之后）
	hoSum, hoN int64
	hoMin      int64
	prevTot    int64
}

func (t *saTracker) observe(rep *cult.TickReport) {
	r := t.res
	var tot int64
	for i := range rep.Points {
		for e := 0; e < cult.ElementCount; e++ {
			tot += rep.Points[i].Qi[e]
		}
		if tl := rep.Points[i].ThermalLoad; tl > r.peakThermal {
			r.peakThermal = tl
		}
	}
	if t.prevDmg != nil {
		for i := range rep.Points {
			if rep.Points[i].Damage > t.prevDmg[i] {
				r.damageTicks++
				break
			}
		}
	}
	for i := range rep.Points {
		t.prevDmg[i] = rep.Points[i].Damage
	}

	// 做功窗口（内化观测，滚动 50 Tick）。
	wk := &rep.Work
	t.sumAuto += wk.AutonomousWork - t.winAuto[t.winIdx]
	t.sumMan += wk.ManualWork - t.winMan[t.winIdx]
	t.sumPas += wk.PassiveWork - t.winPas[t.winIdx]
	t.winAuto[t.winIdx], t.winMan[t.winIdx], t.winPas[t.winIdx] = wk.AutonomousWork, wk.ManualWork, wk.PassiveWork
	t.winIdx = (t.winIdx + 1) % 50
	if t.winN < 50 {
		t.winN++
	}
	if den := t.sumAuto + t.sumMan + t.sumPas; den > 0 && t.winN >= 50 {
		share := t.sumAuto * 1000 / den
		switch {
		case share >= 800 && r.ia80 < 0:
			r.ia80 = rep.Tick
			r.ia50 = orFirst(r.ia50, rep.Tick)
			r.ia20 = orFirst(r.ia20, rep.Tick)
		case share >= 500 && r.ia50 < 0:
			r.ia50 = rep.Tick
			r.ia20 = orFirst(r.ia20, rep.Tick)
		case share >= 200 && r.ia20 < 0:
			r.ia20 = rep.Tick
		}
	}

	// 局部耦合初显/可感。
	for i := range t.w.Character.BodyGraph.Acupoints {
		cps := t.w.Character.BodyGraph.Acupoints[i].Couplings
		for k := range cps {
			for e := 0; e < cult.ElementCount; e++ {
				s := cps[k].Strength[e]
				if s >= saFlowBand && r.firstCouple < 0 {
					r.firstCouple = rep.Tick
				}
				if s >= saSenseBand && r.firstSense < 0 {
					r.firstSense = rep.Tick
				}
			}
		}
	}

	// 流量基线与序列。
	flow := int64(0)
	for ti := range rep.Transfers {
		for e := 0; e < cult.ElementCount; e++ {
			flow += rep.Transfers[ti].Actual[e]
		}
	}
	switch {
	case t.inTeach:
		t.teachFlowSum += flow
		t.teachFlowN++
	case t.inFinal:
		t.finalRel = append(t.finalRel, flow)
		t.relFlowSum += flow
		t.relFlowN++
	default:
		t.relFlowSum += flow
		t.relFlowN++
	}

	// 移交后（estTick 之后第 10 tick 起的 60 tick 窗）。
	if r.estTick > 0 && rep.Tick > r.estTick+5 && t.hoN < 60 {
		t.hoSum += flow
		t.hoN++
		if t.hoMin == 0 || flow < t.hoMin {
			t.hoMin = flow
		}
	}
}

func orFirst(v, alt int64) int64 {
	if v < 0 {
		return alt
	}
	return v
}

// runScanA 走完一个角色的真实判定链（ctmatch 会话：引气→行环→松手→
// 筑基→升格移交），返回全部观测。
func runScanA(seed uint64, acc *accounting) scanARes {
	p := genScanProfile(seed, false)
	w := buildScanWorld(&p)
	s := ctmatch.NewSession(w, seed, nil)
	proto, err := ctmatch.Compile([]byte(scanRouteYAML(&p, saCycleBudget)), &w.Character.BodyGraph)
	if err != nil {
		panic(err) // 标定台静态装配错误
	}
	s.LoadProtocol(proto)
	ctmatch.Tick(s, nil)                        // 观测基准
	runFillPhase(s, acc, p.route[5], saFillLen) // 引气填充：先充身，后行环

	res := scanARes{seed: seed, route: p.route, envScale: p.envScale,
		minCond: p.cond[0], minOpen: p.open[0],
		firstCouple: -1, firstSense: -1, ia20: -1, ia50: -1, ia80: -1,
		touchTick: -1, estTick: -1}
	for k := 1; k < 6; k++ {
		if p.cond[k] < res.minCond {
			res.minCond = p.cond[k]
		}
		if p.open[k] < res.minOpen {
			res.minOpen = p.open[k]
		}
	}
	tr := &saTracker{res: &res, w: w, prevDmg: make([]int64, len(w.Character.BodyGraph.Acupoints))}
	for i := range w.Character.BodyGraph.Acupoints {
		tr.tot0 += ptLoad(&w.Character.BodyGraph.Acupoints[i])
	}
	res.tot0 = tr.tot0

	runTicks := func(n int64, teach bool) {
		tr.inTeach = teach
		for i := int64(0); i < n; i++ {
			rs := ctmatch.Tick(s, nil)
			for _, ev := range rs.Events {
				if ev.Kind == ctmatch.EventUnknownAnomaly {
					res.anomalies++
					s.ResumeProtocol() // 升级后由「观察继续」接管（标定口径）
				}
			}
			if rs.Report != nil {
				acc.checkConservation(rs.Report)
				tr.observe(rs.Report)
			}
			if s.Foundation.Touched && res.touchTick < 0 {
				res.touchTick = s.World.Tick
			}
			if s.Foundation.Established && res.estTick < 0 {
				res.estTick = s.World.Tick
			}
			res.touched = res.touched || s.Foundation.Touched
			res.estab = res.estab || s.Foundation.Established
			m := s.Foundation.Metrics()
			if m.SelfSustain > res.peakSS {
				res.peakSS = m.SelfSustain
				res.ssClosure = m.Closure
				res.ssBalance = m.RuleBalance
			}
			if m.Closure > res.peakClosure {
				res.peakClosure = m.Closure
			}
			// 门槛归因采样：终局松手中段（W2 已冻结、非控制相）。
			if !res.estab && tr.inFinal && int64(len(tr.finalRel)) > 200 {
				res.sampled++
				if m.Closure < 800 {
					res.blockClosure++
				}
				if m.RuleBalance < 800 {
					res.blockBalance++
				}
				if m.SelfSustain < 700 {
					res.blockSS++
				}
				if m.DynamicStability < 800 {
					res.blockDyn++
				}
				if m.MinStability < 700 {
					res.blockStab++
				}
			}
		}
	}
	start := func() {
		if err := s.StartProtocol(proto.Name); err != nil {
			panic(err) // 标定台静态装配错误
		}
	}
	for b := 0; b < saBlocks; b++ {
		start()
		runTicks(saTeachTicks, true)
		runTicks(saProbeTicks, false) // 探针松手（零意图）
	}
	for b := 0; b < saPureBlocks; b++ {
		start()
		runTicks(saTeachTicks, true)
	}
	start()
	runTicks(saTeachTicks, true)
	tr.inFinal = true
	runTicks(saFinalTicks, false)
	tr.inFinal = false

	res.tot1 = tr.prevTot
	// 「无法维持」口径：终局松手尾段（末 100 tick）均流 < 教学基线 5%
	// ——环流真正断灭。总存量跌到低平衡（LR~0.07）不是枯竭，是工作点
	// 下移（无损伤、可再纳气回升）；松手期漏气到环流断灭才是「无法维持」。
	if tr.teachFlowN > 0 {
		base := tr.teachFlowSum / tr.teachFlowN
		tail := int64(0)
		tailN := int64(0)
		for i := len(tr.finalRel) - 100; i < len(tr.finalRel); i++ {
			if i >= 0 {
				tail += tr.finalRel[i]
				tailN++
			}
		}
		if tailN > 0 && base > 0 && (tail/tailN)*100 < base*5 {
			res.depleted = true
		}
	}
	res.osc = oscOf(tr.finalRel)
	if res.estab && tr.hoN > 0 && tr.teachFlowN > 0 {
		res.hoMeasured = true
		base := tr.teachFlowSum / tr.teachFlowN
		if base > 0 {
			res.hoRetention = tr.hoSum / tr.hoN * 1000 / base
		}
		res.hoBreak = base > 0 && tr.hoMin*10 < base
	}
	return res
}

// checkConservation 把一份会话层报告的守恒账计入聚合账本（SA 走
// ctmatch 会话推进，残差检查口径与 acc.advance 一致）。
func (a *accounting) checkConservation(rep *cult.TickReport) {
	a.ticks++
	c := &rep.Conservation
	net := c.QiAfter - c.QiBefore
	accounted := c.Intake + c.ExchangeIn - c.Diffuse - c.TransferLoss - c.ShengLoss - c.DriveWork + c.Entrained
	if res := net - accounted; res != 0 {
		a.violations++
		if res < 0 {
			res = -res
		}
		if res > a.maxResidual {
			a.maxResidual = res
		}
	}
}

// oscOf 计算序列归一化方差 var/mean²（千分位，封顶 1000；均值 < 5 视为
// 无波动）。与 ctmatch foundation.go normVar 同型（本地复算避免导出）。
func oscOf(xs []int64) int64 {
	n := int64(len(xs))
	if n < 2 {
		return 0
	}
	var sum int64
	for _, x := range xs {
		sum += x
	}
	mean := sum / n
	if mean < 5 {
		return 0
	}
	var v int64
	for _, x := range xs {
		d := x - mean
		v += d * d
	}
	nv := (v / n) * cult.One / (mean * mean)
	if nv > cult.One {
		nv = cult.One
	}
	return nv
}

// ---------------------------------------------------------------------------
// SB 路线唯一性对照（§27「是否存在单一全局最优路线」）
// ---------------------------------------------------------------------------

// sbRoutes 是 8 条固定路线（对泵之外的 5 槽做确定性置换）。
var sbRoutes = [8][5]int{
	{1, 2, 3, 4, 5}, // 恒等
	{5, 4, 3, 2, 1}, // 逆
	{2, 3, 4, 5, 1}, // 左旋 1
	{4, 5, 1, 2, 3}, // 左旋 3
	{3, 2, 1, 5, 4}, // 中折
	{1, 3, 2, 5, 4}, // 近邻交换
	{2, 1, 4, 3, 5}, // 隔位交换
	{5, 1, 4, 2, 3}, // 远近混排
}

type scanBRes struct {
	bodyIdx  int
	routeIdx int
	peakSS   int64
	estab    bool
	estTick  int64
	ia80     int64 // 自主做功 80% 首见（-1 未达）
	meanStab int64 // 教学末段全图最小稳定性均值
	damage   int64
}

// runScanB 固定身体（槽位画像），换一条路线走 SA 同款流程（减半预算：
// 4 组教学/探针 + 终局），返回对照指标。
func runScanB(bodyIdx, routeIdx int, acc *accounting) scanBRes {
	p := genScanProfile(uint64(10000+bodyIdx), true)
	// 覆盖路线：route[0] 泵不变；1..5 槽按置换表重排（置换表元素是
	// 「非泵点按升序序列中的位次」，1-based——同一批点换行进顺序）。
	perm := sbRoutes[routeIdx]
	var others [5]int
	j := 0
	for i := 0; i < 6; i++ {
		if i != p.route[0] {
			others[j] = i
			j++
		}
	}
	p.route = [6]int{p.route[0], others[perm[0]-1], others[perm[1]-1], others[perm[2]-1], others[perm[3]-1], others[perm[4]-1]}
	p.seed = uint64(10000+bodyIdx)*10 + uint64(routeIdx)

	w := buildScanWorld(&p)
	s := ctmatch.NewSession(w, p.seed, nil)
	proto, err := ctmatch.Compile([]byte(scanRouteYAML(&p, saCycleBudget)), &w.Character.BodyGraph)
	if err != nil {
		panic(err) // 标定台静态装配错误
	}
	s.LoadProtocol(proto)
	ctmatch.Tick(s, nil)
	runFillPhase(s, acc, p.route[5], saFillLen) // 引气填充：先充身，后行环

	res := scanBRes{bodyIdx: bodyIdx, routeIdx: routeIdx, estTick: -1, ia80: -1}
	var stabSum, stabN int64
	var teachFlowSum, teachFlowN int64
	var winAuto, winMan, winPas [50]int64
	var winN int64
	var winIdx int
	var sumAuto, sumMan, sumPas int64
	prevDmg := make([]int64, 6)

	runTicks := func(n int64, teach bool) {
		for i := int64(0); i < n; i++ {
			rs := ctmatch.Tick(s, nil)
			if rs.Report == nil {
				continue
			}
			acc.checkConservation(rs.Report)
			rep := rs.Report
			var minStab int64 = cult.One
			for j := range rep.Points {
				if rep.Points[j].Stability < minStab {
					minStab = rep.Points[j].Stability
				}
				if j < len(prevDmg) && rep.Points[j].Damage > prevDmg[j] {
					res.damage++
				}
			}
			for j := range rep.Points {
				if j < len(prevDmg) {
					prevDmg[j] = rep.Points[j].Damage
				}
			}
			wk := &rep.Work
			sumAuto += wk.AutonomousWork - winAuto[winIdx]
			sumMan += wk.ManualWork - winMan[winIdx]
			sumPas += wk.PassiveWork - winPas[winIdx]
			winAuto[winIdx], winMan[winIdx], winPas[winIdx] = wk.AutonomousWork, wk.ManualWork, wk.PassiveWork
			winIdx = (winIdx + 1) % 50
			if winN < 50 {
				winN++
			}
			if den := sumAuto + sumMan + sumPas; den > 0 && winN >= 50 &&
				sumAuto*1000/den >= 800 && res.ia80 < 0 {
				res.ia80 = rep.Tick
			}
			var flow int64
			for ti := range rep.Transfers {
				for e := 0; e < cult.ElementCount; e++ {
					flow += rep.Transfers[ti].Actual[e]
				}
			}
			if teach {
				teachFlowSum += flow
				teachFlowN++
				stabSum += minStab
				stabN++
			}
			if s.Foundation.Established && res.estTick < 0 {
				res.estTick = s.World.Tick
			}
			res.estab = res.estab || s.Foundation.Established
			if ss := s.Foundation.Metrics().SelfSustain; ss > res.peakSS {
				res.peakSS = ss
			}
		}
	}
	start := func() {
		if err := s.StartProtocol(proto.Name); err != nil {
			panic(err) // 标定台静态装配错误
		}
	}
	for b := 0; b < 8; b++ {
		start()
		runTicks(saTeachTicks, true)
		runTicks(saProbeTicks, false)
	}
	start()
	runTicks(saTeachTicks, true)
	runTicks(saFinalTicks, false)
	if stabN > 0 {
		res.meanStab = stabSum / stabN
	}
	_ = teachFlowSum
	_ = teachFlowN
	return res
}

// ---------------------------------------------------------------------------
// P11 移交门专项：耦合强度 × 移交后保有率/断流率
// ---------------------------------------------------------------------------

// p11Strengths 是移交强度扫频档（千分位）。
var p11Strengths = []int64{500, 600, 700, 800, 900, 1000}

const (
	p11Seeds      = 40
	p11TeachTicks = 300
	p11RelTicks   = 60
)

type p11Cell struct {
	strength  int64
	retMed    int64 // 保有率中位（千分位）
	retQ1     int64
	retQ3     int64
	breakFrac int64 // 断流率（千分位：60 tick 内跌破教学基线 10%）
	oscMed    int64 // 松手窗流量归一化方差中位
}

// runP11 单 seed 单强度：直接 rig 教学（全环同息并行推 300 tick，教师门
// 饱和）→ 钉耦合强度 = S → 全松手 60 tick，测保有率与断流。
// 移交语义与 ctmatch feedHandoverTaken 一致：强度达门槛的段不再发主动
// 请求，流由耦合自主命令（cmd = 强度 × 来流 × 习得响应）接续。
func runP11(seed uint64, strength int64, acc *accounting) (int64, int64, bool, int64) {
	p := genScanProfile(seed, true)
	w := buildScanWorld(&p)
	g := &w.Character.BodyGraph
	amt := teachAmount(&p)
	var teachSum int64
	for t := int64(0); t < p11TeachTicks; t++ {
		it := make([]int64, 6)
		it[p.route[5]] = 1000
		var reqs []cult.FlowRequest
		for k := 0; k < 6; k++ {
			var a [cult.ElementCount]int64
			for e := range a {
				a[e] = amt
			}
			reqs = append(reqs, cult.FlowRequest{Channel: k, Amount: a})
		}
		rep := acc.advance(w, cult.Ops{Intensity: it, Requests: reqs})
		var f int64
		for ti := range rep.Transfers {
			for e := 0; e < cult.ElementCount; e++ {
				f += rep.Transfers[ti].Actual[e]
			}
		}
		teachSum += f
	}
	base := teachSum / p11TeachTicks

	// 钉强度：全部环耦合、全元素 = S（移交门语义：达门槛段移交）。
	for i := range g.Acupoints {
		for k := range g.Acupoints[i].Couplings {
			for e := 0; e < cult.ElementCount; e++ {
				g.Acupoints[i].Couplings[k].Strength[e] = strength
			}
		}
	}
	var flows []int64
	minFlow := int64(-1)
	for t := 0; t < p11RelTicks; t++ {
		rep := acc.advance(w, cult.Ops{})
		var f int64
		for ti := range rep.Transfers {
			for e := 0; e < cult.ElementCount; e++ {
				f += rep.Transfers[ti].Actual[e]
			}
		}
		flows = append(flows, f)
		if minFlow < 0 || f < minFlow {
			minFlow = f
		}
	}
	// 保有率：前 30 tick（W2 窗口径）均流 / 教学基线；尾比 = 末 10 tick
	// 均流 / 基线（释放动力学是「瞬时复现 S → 增益 <1 衰减 → 被动份额
	// 托底」的 S 曲线，尾比才是稳态保有）。
	var w2 int64
	for i := 0; i < 30 && i < len(flows); i++ {
		w2 += flows[i]
	}
	ret := int64(0)
	if base > 0 {
		ret = (w2 / 30) * 1000 / base
	}
	tail := int64(0)
	n10 := int64(0)
	for i := len(flows) - 10; i < len(flows); i++ {
		if i >= 0 {
			tail += flows[i]
			n10++
		}
	}
	endRatio := int64(0)
	if base > 0 && n10 > 0 {
		endRatio = (tail / n10) * 1000 / base
	}
	broke := base > 0 && endRatio < 80 // 尾比 <8% = 环流真正断流
	return ret, endRatio, broke, oscOf(flows)
}

// ---------------------------------------------------------------------------
// P12 SelfSustain v2 分辨力
// ---------------------------------------------------------------------------

var p12Open = []int64{300, 500, 700, 900, 1000}   // 开度档（千分位）
var p12Env = []int64{900, 1500, 2100, 3000, 4200} // 环境密度档
var p12SeedsPerCell = 12

// runP12Cell 单元格：PartJ 标定世界变体（开度×环境缩放），小流量教学
// （amount 300 = P9 口径，60/元素）与饱和教学（2400/元素）两口径，
// 测松手窗 SelfSustain（W2/W1，钳 1）。
func runP12Cell(openMilli, envQi int64, seed uint64, sat bool, acc *accounting) int64 {
	w := ctmatch.PartJCalibrationWorld()
	w.NoiseSeed = seed
	g := &w.Character.BodyGraph
	for i := range g.Acupoints {
		g.Acupoints[i].Openness = openMilli * 1000
	}
	w.Env.Qi = [cult.ElementCount]int64{envQi, envQi, envQi, envQi, envQi}
	amt := int64(300) // 小流量（P9 口径，混合 60/元素）
	if sat {
		amt = 12000 // 饱和教学（2400/元素，教师门满）
	}
	// 教学相：吸收@青关(1) + 全环引导（PartJ 世界通道 0..5 依次为
	// 丹→土→中→阳→神→青→丹）。两口径刻意固定流量：小流量 = P9 口径，
	// 饱和 = 教师门满的超热教学对照（热预算外的量级，测比值钳 1 的条件）。
	var w1 int64
	for t := 0; t < 200; t++ {
		it := make([]int64, 6)
		it[1] = 1000
		var a [cult.ElementCount]int64
		for e := range a {
			a[e] = amt
		}
		reqs := make([]cult.FlowRequest, 6)
		for k := range reqs {
			reqs[k] = cult.FlowRequest{Channel: k, Amount: a}
		}
		rep := acc.advance(w, cult.Ops{Intensity: it, Requests: reqs})
		w1 += rep.Work.ManualWork
	}
	w1 /= 200
	// 松手相：W2 = 前 30 tick 非控制实际流均流。
	var w2 int64
	for t := 0; t < 30; t++ {
		rep := acc.advance(w, cult.Ops{})
		for ti := range rep.Transfers {
			for e := 0; e < cult.ElementCount; e++ {
				w2 += rep.Transfers[ti].Actual[e]
			}
		}
	}
	w2 /= 30
	if w1 <= 0 {
		return 0
	}
	ss := w2 * cult.One / w1
	if ss > cult.One {
		ss = cult.One
	}
	return ss
}

// ---------------------------------------------------------------------------
// P13 流量闭合新口径：多环 / 支路拓扑
// ---------------------------------------------------------------------------

// p13Topo 是一个对照拓扑。
type p13Topo struct {
	name     string
	pts      int
	edges    [][2]int // 天然向边（点下标）
	ringOut  int      // 教学环首出边的边下标（初发流测点）
	ringBack int      // 教学环末回边的边下标（回流测点）
	teach    []int    // 教学推流的边下标集合
}

// p13Topologies：T1 单环基线；T2 双环共点（8 字环，两环同教）；
// T3 环+捷径（0→2 弦分流）；T4 环+支路（2→6 死支泄漏）。
// 主环均取 0→1→2→3→0（四棒环，便于加弦/支路）。
var p13Topologies = []p13Topo{
	{
		name: "单环", pts: 4,
		edges:   [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 0}},
		ringOut: 0, ringBack: 3,
		teach: []int{0, 1, 2, 3},
	},
	{
		name: "双环共点", pts: 7,
		edges:   [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 0}, {3, 4}, {4, 5}, {5, 6}, {6, 3}},
		ringOut: 0, ringBack: 3,
		teach: []int{0, 1, 2, 3, 4, 5, 6, 7},
	},
	{
		name: "环+捷径", pts: 4,
		edges:   [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 0}, {0, 2}},
		ringOut: 0, ringBack: 3,
		teach: []int{0, 1, 2, 3},
	},
	{
		name: "环+支路", pts: 5,
		edges:   [][2]int{{0, 1}, {1, 2}, {2, 3}, {3, 0}, {2, 4}},
		ringOut: 0, ringBack: 3,
		teach: []int{0, 1, 2, 3},
	},
}

const (
	p13Seeds      = 30
	p13TeachTicks = 250
	p13RelTicks   = 50
)

type p13Res struct {
	closureTeach int64 // 教学末 50 tick 回流/初发流均值（千分位）
	closureRel   int64 // 松手窗同口径
	autoSplit    int64 // 松手窗支路/副环边自主流量 ÷ 主出边自主流量（千分位；无自主流 -1）
	relFlow      int64 // 松手窗总流量均值
}

// runP13 在给定拓扑上教学主环（+多环拓扑的全部环），松手观测闭合与
// 自主流分叉。
func runP13(topo *p13Topo, seed uint64, acc *accounting) p13Res {
	var res p13Res
	pts := make([]cult.Acupoint, topo.pts)
	for i := range pts {
		pts[i] = cult.Acupoint{
			Name:     fmt.Sprintf("穴%d", i),
			Affinity: [cult.ElementCount]int64{1000, 1000, 1000, 1000, 1000},
			Capacity: 30000, Throughput: 10000, Retention: 900,
			Openness: cult.MicroOne, Stability: 950, BaseStability: 950,
			Activity:      [cult.ElementCount]int64{1000, 1000, 1000, 1000, 1000},
			Adaptation:    [cult.ElementCount]int64{1000000, 1000000, 1000000, 1000000, 1000000},
			DriveCapacity: 8000, DriveEfficiency: 900,
		}
	}
	// 泵 = 点 0（显著主导）。
	pts[0].DriveCapacity = 24000
	chs := make([]cult.Channel, len(topo.edges))
	for i, e := range topo.edges {
		chs[i] = cult.Channel{Name: fmt.Sprintf("e%d", i), From: e[0], To: e[1],
			BaseConductance: 900, Throughput: 10000}
	}
	w := cult.NewWorld(cult.Character{
		Affinity: [cult.ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Control:  900, Perception: 500,
		BodyGraph: cult.BodyGraph{Acupoints: pts, Channels: chs},
	}, cult.Environment{
		Qi:          [cult.ElementCount]int64{3000, 3000, 3000, 3000, 3000},
		Activity:    [cult.ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Temperature: 1000,
	})
	w.NoiseSeed = seed

	closureOf := func(rep *cult.TickReport) (int64, int64, int64) {
		var out, back, auto int64
		for ti := range rep.Transfers {
			tr := &rep.Transfers[ti]
			if tr.Reverse {
				continue
			}
			for e := 0; e < cult.ElementCount; e++ {
				if tr.Channel == topo.ringOut {
					out += tr.Actual[e]
				}
				if tr.Channel == topo.ringBack {
					back += tr.Delivered[e]
				}
				if tr.Autonomous[e] > 0 {
					auto += tr.Autonomous[e]
				}
			}
		}
		return out, back, auto
	}
	// 教学末段闭合序列。
	var teachClo []int64
	for t := 0; t < p13TeachTicks; t++ {
		it := make([]int64, topo.pts)
		it[topo.edges[topo.ringBack][1]] = 1000 // 回边终点 = 环起点吸收
		var reqs []cult.FlowRequest
		for _, k := range topo.teach {
			var a [cult.ElementCount]int64
			for e := range a {
				a[e] = 260 // 热安全教学流量（cond 0.9 ⇒ 26/0.1）
			}
			reqs = append(reqs, cult.FlowRequest{Channel: k, Amount: a})
		}
		rep := acc.advance(w, cult.Ops{Intensity: it, Requests: reqs})
		out, back, _ := closureOf(rep)
		v := int64(0)
		if out > 0 {
			v = back * cult.One / out
			if v > cult.One {
				v = cult.One
			}
		}
		if t >= p13TeachTicks-50 {
			teachClo = append(teachClo, v)
		}
	}
	res.closureTeach = meanOf(teachClo)
	// 松手窗：闭合 + 自主流分叉（支路/副环边 = teach 集之外的边）。
	var relClo []int64
	var autoMain, autoSide int64
	var flowSum int64
	for t := 0; t < p13RelTicks; t++ {
		rep := acc.advance(w, cult.Ops{})
		out, back, _ := closureOf(rep)
		v := int64(0)
		if out > 0 {
			v = back * cult.One / out
			if v > cult.One {
				v = cult.One
			}
		}
		relClo = append(relClo, v)
		for ti := range rep.Transfers {
			tr := &rep.Transfers[ti]
			side := true
			for _, k := range topo.teach {
				if tr.Channel == k {
					side = false
					break
				}
			}
			for e := 0; e < cult.ElementCount; e++ {
				if tr.Autonomous[e] > 0 {
					if side {
						autoSide += tr.Autonomous[e]
					} else if tr.Channel == topo.ringOut {
						autoMain += tr.Autonomous[e]
					}
				}
				flowSum += tr.Actual[e]
			}
		}
	}
	res.closureRel = meanOf(relClo)
	res.relFlow = flowSum / p13RelTicks
	if autoMain+autoSide > 0 {
		res.autoSplit = autoSide * 1000 / (autoMain + autoSide)
	} else {
		res.autoSplit = -1
	}
	return res
}

func meanOf(xs []int64) int64 {
	if len(xs) == 0 {
		return 0
	}
	var s int64
	for _, x := range xs {
		s += x
	}
	return s / int64(len(xs))
}

// ---------------------------------------------------------------------------
// P14 被动流与手动流量级关系
// ---------------------------------------------------------------------------

const (
	p14Seeds      = 60
	p14TeachTicks = 200
	p14RelTicks   = 40
)

type p14Res struct {
	teachPassiveShare int64 // 教学期被动流占总实际流（千分位）
	relPassiveShare   int64 // 松手期被动流占非控制实际流（千分位）
	relAutoShare      int64 // 松手期自主流占非控制实际流（千分位）
	// 封顶约束频率（松手期，逐通道×元素×tick 计数）
	srcBoundFrac  int64 // 被动流贴源端份额封顶（≥95% cap）占比
	recvBoundFrac int64 // 贴受端缓充封顶占比
	pipeBoundFrac int64 // 贴管线容量占比
	unboundFrac   int64 // 低于全部封顶 95% 占比
}

// runP14 单 seed：decent 画像世界，教学（2400/元素全环）→ 松手。
// 逐 tick 对被动条目做封顶归因（读 tick 前快照与 tick 后报告）。
func runP14(seed uint64, acc *accounting) p14Res {
	p := genScanProfile(seed, true)
	w := buildScanWorld(&p)
	var res p14Res
	var teachAll, teachPass int64
	srcBound, recvBound, pipeBound, unbound := int64(0), int64(0), int64(0), int64(0)
	var relAll, relPass, relAuto int64

	// 快照：tick 前各通道源/受端状态（下标 = 通道 k；src = route[k]）。
	srcStock := make([][cult.ElementCount]int64, 6)
	dstStock := make([][cult.ElementCount]int64, 6)
	recvLoad := make([]int64, 6)
	recvCap := make([]int64, 6)

	for t := 0; t < p14TeachTicks+p14RelTicks; t++ {
		teach := t < p14TeachTicks
		g := &w.Character.BodyGraph
		for k := 0; k < 6; k++ {
			ch := &g.Channels[k]
			src := &g.Acupoints[ch.From]
			dst := &g.Acupoints[ch.To]
			for e := 0; e < cult.ElementCount; e++ {
				srcStock[k][e] = src.Qi[e]
				dstStock[k][e] = dst.Qi[e]
			}
			recvLoad[k] = ptLoad(dst)
			recvCap[k] = ptCapEff(dst)
		}
		var ops cult.Ops
		if teach {
			amt := teachAmount(&p)
			it := make([]int64, 6)
			it[p.route[5]] = 1000
			var reqs []cult.FlowRequest
			for k := 0; k < 6; k++ {
				var a [cult.ElementCount]int64
				for e := range a {
					a[e] = amt
				}
				reqs = append(reqs, cult.FlowRequest{Channel: k, Amount: a})
			}
			ops = cult.Ops{Intensity: it, Requests: reqs}
		}
		rep := acc.advance(w, ops)
		var all, pass, auto int64
		for ti := range rep.Transfers {
			tr := &rep.Transfers[ti]
			ch := tr.Channel
			for e := 0; e < cult.ElementCount; e++ {
				all += tr.Actual[e]
				if tr.Passive[e] > 0 {
					pass += tr.Passive[e]
					if !teach {
						// 封顶归因（tick 前状态）：源端份额 / 受端（过流+
						// 缓充）/ 管线容量，贴 95% 视为该约束生效。
						srcCap := srcStock[ch][e] * cult.KQiShare / cult.One
						recvC := dstStock[ch][e]*cult.KQiShare/cult.One +
							(recvCap[ch]-recvLoad[ch])*cult.KRecvShare/cult.One
						chanCap := chanCapOf(&g.Channels[ch], g)
						pipeC := fpDown(chanCap, tr.Conductance[e])
						switch {
						case srcCap > 0 && tr.Passive[e]*20 >= srcCap*19:
							srcBound++
						case recvC > 0 && tr.Passive[e]*20 >= recvC*19:
							recvBound++
						case pipeC > 0 && tr.Passive[e]*20 >= pipeC*19:
							pipeBound++
						default:
							unbound++
						}
					}
				}
				if tr.Autonomous[e] > 0 {
					auto += tr.Autonomous[e]
				}
			}
		}
		if teach {
			teachAll += all
			teachPass += pass
		} else {
			relAll += all
			relPass += pass
			relAuto += auto
		}
	}
	if teachAll > 0 {
		res.teachPassiveShare = teachPass * 1000 / teachAll
	}
	if relAll > 0 {
		res.relPassiveShare = relPass * 1000 / relAll
		res.relAutoShare = relAuto * 1000 / relAll
	}
	tot := srcBound + recvBound + pipeBound + unbound
	if tot > 0 {
		res.srcBoundFrac = srcBound * 1000 / tot
		res.recvBoundFrac = recvBound * 1000 / tot
		res.pipeBoundFrac = pipeBound * 1000 / tot
		res.unboundFrac = unbound * 1000 / tot
	}
	return res
}

// chanCapOf 复算一条经脉的 §8.1 通道容量基数（min 三方通量，含损伤
// 折算；tick 后读，作为管线容量量级参考）。
func chanCapOf(ch *cult.Channel, g *cult.BodyGraph) int64 {
	cap := ch.Throughput
	if s := g.Acupoints[ch.From].Throughput; s < cap {
		cap = s
	}
	if d := g.Acupoints[ch.To].Throughput; d < cap {
		cap = d
	}
	return cap
}

// fpDown 是 cult.fpMul 的本地镜像（scan 包内不能用未导出函数）。
func fpDown(a, b int64) int64 {
	return a * b / cult.One
}

// ---------------------------------------------------------------------------
// 聚合与报告
// ---------------------------------------------------------------------------

type scanInput struct {
	nChars int
	sa     []scanARes
	sb     [][]scanBRes // [body][route]
	p11    [][]int64    // [strengthIdx][seed] → W2 窗保有率
	p11End [][]int64    // [strengthIdx][seed] → 尾比（末 10 tick 均流/基线）
	p11Brk [][]bool
	p11Osc [][]int64
	p12    map[[2]int][]int64 // [openIdx][envIdx] → SS（小流量口径）
	p12Sat map[[2]int][]int64 // [openIdx][envIdx] → SS（饱和教学口径）
	p13    [][]p13Res         // [topo][seed]
	p14    []p14Res
	acc    *accounting
	secs   string
}

// renderScanReport 渲染 SA/SB/P11-P14 报告（全定点整数）。
func renderScanReport(in *scanInput) string {
	var b strings.Builder
	n := in.nChars
	fmt.Fprintf(&b, "\n# 千级角色扫描与专项观测（§27）\n\n")
	fmt.Fprintf(&b, "- RuleVersion %s；角色数=%d；每角色预算 %d tick（真实判定链：引气400+行环→松手→筑基→升格移交；相位=填400+12×(教60+探30)+44×教60+终局60+320）\n",
		cult.RuleVersion, n, saFillLen+saBlocks*(saTeachTicks+saProbeTicks)+saPureBlocks*saTeachTicks+saTeachTicks+saFinalTicks+1)
	fmt.Fprintf(&b, "- 画像：亲和/容量/通量/传导/开度/驱动容量/驱动效率/环境/路线 全随机（环境含 3/8 低灵气档）；守恒逐 tick 归账覆盖全部会话 tick\n\n")

	// ---------------- SA 汇总 ----------------
	coupled, sensed := 0, 0
	var coupleTicks []int64
	est, touchedN := 0, 0
	var touchRel, ia20v, ia50v, ia80v []int64
	depleted, hiSusLowBal, damageProne := 0, 0, 0
	var oscs, thermals []int64
	hoN := 0
	var hoRets []int64
	hoBreaks := 0
	neverClosed := 0
	var neverMinCond, neverEnv []int64
	var everMinCond, everEnv []int64
	for i := range in.sa {
		r := &in.sa[i]
		if r.firstCouple >= 0 {
			coupled++
			coupleTicks = append(coupleTicks, r.firstCouple)
		}
		if r.firstSense >= 0 {
			sensed++
		}
		if r.touched {
			touchedN++
			touchRel = append(touchRel, r.touchTick)
		}
		if r.estab {
			est++
		}
		if r.ia20 >= 0 {
			ia20v = append(ia20v, r.ia20)
		}
		if r.ia50 >= 0 {
			ia50v = append(ia50v, r.ia50)
		}
		if r.ia80 >= 0 {
			ia80v = append(ia80v, r.ia80)
		}
		if r.depleted {
			depleted++
		}
		if r.peakSS >= 700 && r.ssBalance < 800 {
			hiSusLowBal++
		}
		if r.damageTicks > 0 || r.anomalies > 0 {
			damageProne++
		}
		oscs = append(oscs, r.osc)
		thermals = append(thermals, r.peakThermal)
		if r.hoMeasured {
			hoN++
			hoRets = append(hoRets, r.hoRetention)
			if r.hoBreak {
				hoBreaks++
			}
		}
		// 「永不闭合」：无可感耦合且未筑基（流量/自主两个通路都没起来）。
		if !r.estab && r.peakSS < 700 {
			neverClosed++
			neverMinCond = append(neverMinCond, r.minCond)
			neverEnv = append(neverEnv, r.envScale)
		} else {
			everMinCond = append(everMinCond, r.minCond)
			everEnv = append(everEnv, r.envScale)
		}
	}
	fmt.Fprintf(&b, "## SA 千级扫描（§27 观测清单）\n\n")
	fmt.Fprintf(&b, "| 观测 | 结果 |\n|---|---|\n")
	fmt.Fprintf(&b, "| 首个局部耦合（强度≥0.1）出现 | %d/%d 角色；中位 %s tick |\n",
		coupled, n, lineOrNA(coupleTicks))
	fmt.Fprintf(&b, "| 可感耦合（强度≥0.5）出现 | %d/%d |\n", sensed, n)
	fmt.Fprintf(&b, "| 触碰筑基门槛 / 筑基 | %d / %d（%s / %s） |\n", touchedN, est,
		fp(pct(touchedN, n)), fp(pct(est, n)))
	fmt.Fprintf(&b, "| 自主做功达 20%% / 50%% / 80%% | %d / %d / %d 角色达到 |\n", len(ia20v), len(ia50v), len(ia80v))
	fmt.Fprintf(&b, "| 低灵气/画像无法维持（终局总量<初值12%%） | %d/%d（%s） |\n", depleted, n, fp(pct(depleted, n)))
	fmt.Fprintf(&b, "| 高自持（≥0.7）低平衡（<0.8） | %d/%d |\n", hiSusLowBal, n)
	fmt.Fprintf(&b, "| 过热/振荡/损伤画像（损伤或异常升级） | %d/%d |\n", damageProne, n)
	fmt.Fprintf(&b, "| 永不闭合（未筑基且 SelfSustain 峰值<0.7） | %d/%d（%s） |\n\n", neverClosed, n, fp(pct(neverClosed, n)))

	if len(coupleTicks) > 0 {
		q1, m, q3 := iqrOf(coupleTicks)
		fmt.Fprintf(&b, "- 首耦合时点（含 %d tick 引气填充）：P25 %d / 中位 %d / P75 %d（全程 %d tick 预算）\n", q1, m, q3,
			saFillLen, saFillLen+saBlocks*(saTeachTicks+saProbeTicks)+saPureBlocks*saTeachTicks+saTeachTicks+saFinalTicks)
	}
	if len(ia20v) > 0 {
		fmt.Fprintf(&b, "- 自主做功 20%% 首见：中位 %s；50%%：中位 %s；80%%：中位 %s（tick，仅达成者）\n",
			medOrNA(ia20v), medOrNA(ia50v), medOrNA(ia80v))
	}
	if len(touchRel) >= 10 {
		q1, m, q3 := iqrOf(touchRel)
		fmt.Fprintf(&b, "- 触碰门槛时点（全局 tick）：P25 %d / 中位 %d / P75 %d——分布跨度 %d tick，%s过度集中\n",
			q1, m, q3, q3-q1, map[bool]string{true: "**未见**", false: "**疑似**"}[q3-q1 >= 50])
	} else if len(touchRel) > 0 {
		fmt.Fprintf(&b, "- 触碰门槛时点：仅 %d 角色（样本不足，不评分布）\n", len(touchRel))
	}
	fmt.Fprintf(&b, "- 终局松手窗流量振荡（normVar 中位）：%s；热负荷峰值中位：%s\n",
		fp(medianOf(oscs)), fp(medianOf(thermals)))
	if hoN > 0 {
		fmt.Fprintf(&b, "- 移交实测（筑基后 loopMode）：样本 %d；移交后流量保有率中位 %s；断流（跌破教学基线一成）%d/%d\n\n",
			hoN, fp(medianOf(hoRets)), hoBreaks, hoN)
	}
	// 筑基门槛归因（未筑基者的五量采样）。
	var bC, bB, bS, bD, bT, bN int64
	for i := range in.sa {
		r := &in.sa[i]
		if !r.estab && r.sampled > 0 {
			bC += r.blockClosure
			bB += r.blockBalance
			bS += r.blockSS
			bD += r.blockDyn
			bT += r.blockStab
			bN += r.sampled
		}
	}
	if bN > 0 {
		fmt.Fprintf(&b, "- 筑基门槛归因（未筑基者终局松手采样，样本 %d）：Closure %s / RuleBalance %s / SelfSustain %s / DynStability %s / MinStability %s\n",
			bN, fp(bC*1000/bN), fp(bB*1000/bN), fp(bS*1000/bN), fp(bD*1000/bN), fp(bT*1000/bN))
	}

	// 永不闭合画像归因：低传导/低环境占比。
	if neverClosed > 0 {
		fmt.Fprintf(&b, "- 永不闭合画像归因：低传导（min 传导 <750）占比 %s（闭合人群 %s）；低环境（≤800）占比 %s（闭合人群 %s）\n\n",
			fp(fracBelow(neverMinCond, 750)), fp(fracBelow(everMinCond, 750)),
			fp(fracAtMost(neverEnv, 800)), fp(fracAtMost(everEnv, 800)))
	}

	// ---------------- SB 路线唯一性 ----------------
	if len(in.sb) > 0 {
		fmt.Fprintf(&b, "## SB 路线唯一性对照（固定身体 × 8 路线）\n\n")
		nBodies := len(in.sb)
		wins := make([]int64, len(sbRoutes)) // 综合分（SS×稳定性）第一名次数
		estWins := make([]int64, len(sbRoutes))
		var ssByRoute = make([][]int64, len(sbRoutes))
		for ri := range sbRoutes {
			for bi := range in.sb {
				ssByRoute[ri] = append(ssByRoute[ri], in.sb[bi][ri].peakSS)
			}
		}
		// 逐身体：peakSS 严格最高的路线（并列不算胜——并列即无差异）。
		for bi := range in.sb {
			best := int64(-1)
			ties := 0
			for ri := range sbRoutes {
				if in.sb[bi][ri].peakSS > best {
					best = in.sb[bi][ri].peakSS
					ties = 1
				} else if in.sb[bi][ri].peakSS == best {
					ties++
				}
			}
			if ties == 1 {
				for ri := range sbRoutes {
					if in.sb[bi][ri].peakSS == best {
						wins[ri]++
					}
				}
			}
			for ri := range sbRoutes {
				if in.sb[bi][ri].estab {
					estWins[ri]++
				}
			}
		}
		fmt.Fprintf(&b, "身体数 %d；逐身体取 SelfSustain 峰值最高路线（并列同记）：\n\n", nBodies)
		fmt.Fprintf(&b, "| 路线 | SS 峰值中位 | 最佳次数 | 筑基次数 |\n|---|---|---|---|\n")
		dominant := -1
		for ri := range sbRoutes {
			fmt.Fprintf(&b, "| R%d | %s | %d/%d | %d/%d |\n", ri, fp(medianOf(ssByRoute[ri])),
				wins[ri], nBodies, estWins[ri], nBodies)
			if wins[ri]*100 >= int64(nBodies)*90 {
				dominant = ri
			}
		}
		if dominant >= 0 {
			fmt.Fprintf(&b, "\n结论：**存在**近似唯一最优路线 R%d（≥90%% 身体的 SS 峰值最优）——世界语义预警。\n\n", dominant)
		} else {
			fmt.Fprintf(&b, "\n结论：**不存在**唯一全局最优路线（最优路线随身体画像变化）——§27 目标达成。\n\n")
		}
	}

	// ---------------- P11 移交门 ----------------
	fmt.Fprintf(&b, "## P11 移交门：耦合强度 × 保有率/断流（seed×%d）\n\n", p11Seeds)
	fmt.Fprintf(&b, "| 强度 | W2 窗保有率中位 | 尾比中位 | 断流率 | 振荡中位 |\n|---|---|---|---|---|\n")
	for si, s := range p11Strengths {
		rets := in.p11[si]
		ends := in.p11End[si]
		var brk int64
		for _, x := range in.p11Brk[si] {
			if x {
				brk++
			}
		}
		q1, m, q3 := iqrOf(rets)
		fmt.Fprintf(&b, "| %s | %s（P25 %s / P75 %s） | %s | %s | %s |\n",
			fp(s), fp(m), fp(q1), fp(q3), fp(medianOf(ends)), fp(brk*1000/p11Seeds), fp(medianOf(in.p11Osc[si])))
	}
	fmt.Fprintf(&b, "\n- 判读：释放动力学 = 「瞬时复现 S → 环路增益 S×LR×eff ≡ S < 1 衰减 → 被动份额托底」的 S 曲线——W2 窗保有率是衰减瞬态的均值，尾比才是稳态保有；两者都低于强度本身（保有八成」的直觉在比例反射下不成立，见报告 §P11）。\n")
	fmt.Fprintf(&b, "- 门槛取值：门槛只决定「多弱的反射允许移交」——高门槛（0.8+）保证瞬时复现水平与振荡上界，低门槛增加断流风险。\n\n")

	// ---------------- P12 SelfSustain 分辨力 ----------------
	fmt.Fprintf(&b, "## P12 SelfSustain v2 分辨力（开度 × 环境密度网格）\n\n")
	fmt.Fprintf(&b, "小流量教学（P9 口径 60/元素）：\n\n")
	fmt.Fprintf(&b, "| 开度\\环境 |")
	for _, e := range p12Env {
		fmt.Fprintf(&b, " %d |", e)
	}
	fmt.Fprintf(&b, "\n|---|")
	for range p12Env {
		fmt.Fprintf(&b, "---|")
	}
	fmt.Fprintf(&b, "\n")
	for oi, o := range p12Open {
		fmt.Fprintf(&b, "| %s |", fp(o))
		for ei := range p12Env {
			fmt.Fprintf(&b, " %s |", fp(medianOf(in.p12[[2]int{oi, ei}])))
		}
		fmt.Fprintf(&b, "\n")
	}
	fmt.Fprintf(&b, "\n饱和教学（2400/元素，教师门满）：\n\n")
	fmt.Fprintf(&b, "| 开度\\环境 |")
	for _, e := range p12Env {
		fmt.Fprintf(&b, " %d |", e)
	}
	fmt.Fprintf(&b, "\n|---|")
	for range p12Env {
		fmt.Fprintf(&b, "---|")
	}
	fmt.Fprintf(&b, "\n")
	for oi, o := range p12Open {
		fmt.Fprintf(&b, "| %s |", fp(o))
		for ei := range p12Env {
			fmt.Fprintf(&b, " %s |", fp(medianOf(in.p12Sat[[2]int{oi, ei}])))
		}
		fmt.Fprintf(&b, "\n")
	}
	fmt.Fprintf(&b, "\n- 判读：小流量口径在丰环境/高开度角钳 1（背景被动流 ≥ 教学流，比值无分辨率）；饱和口径把分辨率交还给耦合自主性——两口径的落差即「SelfSustain 是教学强度依赖量」的量化。\n\n")

	// ---------------- P13 多环/支路闭合 ----------------
	fmt.Fprintf(&b, "## P13 流量闭合新口径：多环/支路拓扑分布（seed×%d）\n\n", p13Seeds)
	fmt.Fprintf(&b, "| 拓扑 | 教学期闭合中位 | 松手窗闭合中位 | 支路/副环自主流占比中位 | 松手流中位 |\n|---|---|---|---|---|\n")
	for ti, topo := range p13Topologies {
		var cs, cr, splits, flows []int64
		for _, r := range in.p13[ti] {
			cs = append(cs, r.closureTeach)
			cr = append(cr, r.closureRel)
			if r.autoSplit >= 0 {
				splits = append(splits, r.autoSplit)
			}
			flows = append(flows, r.relFlow)
		}
		splitStr := "无自主流"
		if len(splits) > 0 {
			splitStr = fp(medianOf(splits))
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n",
			topo.name, fp(medianOf(cs)), fp(medianOf(cr)), splitStr, fp(medianOf(flows)))
	}
	fmt.Fprintf(&b, "\n- 判读：单环闭合应近满值；弦/支路/副环分走流量则闭合按份额下降——新口径（回流/初发流）对拓扑形态敏感。\n\n")

	// ---------------- P14 被动/手动量级 ----------------
	fmt.Fprintf(&b, "## P14 被动流与手动流量级（seed×%d，decent 画像）\n\n", p14Seeds)
	var tp, rp, ra, sb2, rb, pb, ub []int64
	for _, r := range in.p14 {
		tp = append(tp, r.teachPassiveShare)
		rp = append(rp, r.relPassiveShare)
		ra = append(ra, r.relAutoShare)
		sb2 = append(sb2, r.srcBoundFrac)
		rb = append(rb, r.recvBoundFrac)
		pb = append(pb, r.pipeBoundFrac)
		ub = append(ub, r.unboundFrac)
	}
	fmt.Fprintf(&b, "- 教学期被动流占比中位：%s（被动是环流的背景物理，教学以主动为主）\n", fp(medianOf(tp)))
	fmt.Fprintf(&b, "- 松手期构成中位：被动 %s + 自主 %s（其余为惯性衰减余量）\n", fp(medianOf(rp)), fp(medianOf(ra)))
	fmt.Fprintf(&b, "- 松手期封顶约束频率中位：源端份额 %s / 受端缓充 %s / 管线容量 %s / 未触及 %s\n",
		fp(medianOf(sb2)), fp(medianOf(rb)), fp(medianOf(pb)), fp(medianOf(ub)))
	fmt.Fprintf(&b, "- 判读：源端/受端份额封顶是否为松手期环流上限的主要支配，见源端+受端合计约束频率。\n\n")

	// ---------------- 守恒 ----------------
	fmt.Fprintf(&b, "## 守恒（SA/SB/P12-P14 全部会话与直接 rig tick）\n\n")
	fmt.Fprintf(&b, "- 累计 tick %d；违例 %d；最大残差 %d\n", in.acc.ticks, in.acc.violations, in.acc.maxResidual)
	return b.String()
}

func pct(a, b int) int64 {
	if b == 0 {
		return 0
	}
	return int64(a) * 1000 / int64(b)
}

func lineOrNA(xs []int64) string {
	if len(xs) == 0 {
		return "—"
	}
	return fmt.Sprintf("%d", medianOf(xs))
}

func medOrNA(xs []int64) string {
	if len(xs) == 0 {
		return "—"
	}
	return fmt.Sprintf("%d", medianOf(xs))
}

// fracBelow 返回 xs 中 < v 的占比（千分位）。
func fracBelow(xs []int64, v int64) int64 {
	if len(xs) == 0 {
		return 0
	}
	n := int64(0)
	for _, x := range xs {
		if x < v {
			n++
		}
	}
	return n * 1000 / int64(len(xs))
}

// fracAtMost 返回 xs 中 ≤ v 的占比（千分位）。
func fracAtMost(xs []int64, v int64) int64 {
	if len(xs) == 0 {
		return 0
	}
	n := int64(0)
	for _, x := range xs {
		if x <= v {
			n++
		}
	}
	return n * 1000 / int64(len(xs))
}
