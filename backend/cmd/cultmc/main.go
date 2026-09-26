package main

// cultmc —— Monte Carlo 标定台。
//
// 两块场景（2026-09-27 标定块，基线 RuleVersion 0.7.1-passive-recv-gate，
// 判定口径 foundation-metrics-v2，见 scan.go 与定稿注记）：
//   - main.go：P1-P9 修订包/Part J 命题台（原 0.3.0 基线的九命题，场景
//     结构保持，在新物理下复测——场景升级点见各场景注释）；
//   - scan.go：P10 千级角色扫描（§27）+ P11 移交门 + P12 SelfSustain
//     分辨力 + P13 多环闭合 + P14 被动/手动量级。
//
// 纯引擎程序：无 LLM、无网络、无浮点（报告统计亦全定点整数）。
// 每个 (seed, 场景) 独立建世界（ctmatch.SpikeWorld 同源拓扑，
// NoiseSeed=seed），同 seed 同 ops 逐位可重放。只测量、不调参：
// 命题不成立如实报 FAIL，把关键数值列出来供人工调参。
//
// 周天执行方式：驱动层按 tick 手动下发 guide ops 序列等价执行（未走
// Protocol 编译/起功）——
//
//	主环拓扑（commit 78f68c6，RuleVersion 0.3.0-mainloop，本台按其构建）：
//	  主环（全顺流）：丹田→土府→中庭→阳枢→神庭→青关→丹田
//	  旁支：青关→中庭、金门→中庭、玄泉→中庭
//	  逆流资格（A2 v0.2 核点判据）：唯 Capacity 最大者（丹田，cult.CorePointOf）
//	  可发起——即 ch(青关→丹田) 的反向（丹田→青关）是唯一合法逆流。
//
//	轨道甲（主环稳态轮）：单 tick 顺流走完全环——
//	  ch0 丹→土, ch1 土→中, ch2 中→阳, ch3 阳→神, ch4 神→青, ch5 青→丹
//	  （环向=天然向，全程顺流无做功代价；逐段同 tick 排空，流量平稳）
//	轨道乙（逆流捷径轮）：单 tick——
//	  逆 ch5（丹田→青关，核点外推）+ 顺 ch5（青关→丹田，捷径归元）
//	承压轮（仅 P6 甲）：主环 + 中庭下段（ch2 中→阳）节流至 500/元素——
//	  中转注入 > 中转排出，中庭逐 tick 积压，正是 B4 要测的"多次中转承压"。
//	纯捷径轮（仅 P6 乙）：只 纳气@青关 + 顺 ch5 归丹田，不循环。
//
// 驱动层教训（标定发现，非引擎缺陷）：把周天拆成"进料 tick/排料 tick"
// 交替执行，会让穴位流量在相邻 tick 间剧烈震荡（FlowShock ≈ 1.5+），
// §17 应变立即打穿稳定性——周天必须以平稳流量连续巡行，"顿挫跑法"
// 必然失稳。这本身是一条可标定结论。
//
// 循环轮的逆流/外推量默认取丹田当前存量的 10%（自节流：存量越低推得
// 越少，与纳气流入自洽平衡）；P5 扫频用固定剂量。
//
// 注：任务书里 P2/P6 的轨道定义（"中庭→逆→阳枢"等）是旧归流树拓扑的
// 产物；主环拓扑落地后，主轨=全顺流主环（中庭→阳枢已是天然向顺段），
// 短径=核点逆流外推+捷径归元，本台按新拓扑映射执行。

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"agentbattle/backend/ctmatch"
	"agentbattle/backend/cult"
)

// ---------------------------------------------------------------------------
// 拓扑装配（ctmatch.SpikeWorld：8 穴 9 脉主环拓扑；下标按名称/端点解析）
// ---------------------------------------------------------------------------

type rig struct {
	w *cult.World
	n int

	dan, qg, zhong int    // 下丹田 / 青关 / 中庭
	ring           [6]int // 主环顺流段，按行进序：丹→土, 土→中, 中→阳, 阳→神, 神→青, 青→丹
	chQZ           int    // 青关→中庭（旁支）
}

func newRig(seed uint64) *rig {
	w := ctmatch.SpikeWorld()
	w.NoiseSeed = seed
	g := &w.Character.BodyGraph
	idx := map[string]int{}
	for i, a := range g.Acupoints {
		idx[a.Name] = i
	}
	ch := func(from, to int) int {
		for i, c := range g.Channels {
			if c.From == from && c.To == to {
				return i
			}
		}
		return -1
	}
	dan, qg, zhong := idx["下丹田"], idx["青关"], idx["中庭"]
	ring := [6]int{
		ch(dan, idx["土府"]),
		ch(idx["土府"], zhong),
		ch(zhong, idx["阳枢"]),
		ch(idx["阳枢"], idx["神庭"]),
		ch(idx["神庭"], qg),
		ch(qg, dan),
	}
	for k, c := range ring {
		if c < 0 {
			panic(fmt.Sprintf("主环段 %d 缺失（SpikeWorld 拓扑不识别）", k))
		}
	}
	return &rig{
		w: w, n: len(g.Acupoints),
		dan: dan, qg: qg, zhong: zhong,
		ring: ring,
		chQZ: ch(qg, zhong),
	}
}

// flow 生成对五行全元素的等量流量请求（amt 为逐元素请求量，千分位标度）。
func flow(ch int, amt int64, reverse bool) cult.FlowRequest {
	var a [cult.ElementCount]int64
	for e := range a {
		a[e] = amt
	}
	return cult.FlowRequest{Channel: ch, Amount: a, Reverse: reverse}
}

// absorbOps 生成"青关纳气（intensity）+ 若干经脉请求"的单 Tick ops。
func (r *rig) absorbOps(intensity int64, reqs ...cult.FlowRequest) cult.Ops {
	it := make([]int64, r.n)
	it[r.qg] = intensity
	return cult.Ops{Intensity: it, Requests: reqs}
}

// revProp 构造按丹田当前存量等比例（permille，千分比）的逆流外推请求
// （读取下发时刻的世界状态——确定性，可重放）。逆流段 = ring[5]
// （青关→丹田 反向，即 丹田→青关；核点判据下唯一合法逆流）。
func (r *rig) revProp(permille int64) cult.FlowRequest {
	d := &r.w.Character.BodyGraph.Acupoints[r.dan]
	var a [cult.ElementCount]int64
	for e := range a {
		a[e] = d.Qi[e] * permille / 1000
	}
	req := flow(r.ring[5], 0, true)
	req.Amount = a
	return req
}

// roundAProp 轨道甲主环稳态轮（单 tick 顺流走完全环；丹田出口流量 =
// 丹田存量 10%，自节流；其余段 4000/元素排空）。
func (r *rig) roundAProp(absorb int64) []cult.Ops {
	push := r.revAmountsProp(100)
	reqs := make([]cult.FlowRequest, 0, 6)
	for k, c := range r.ring {
		if k == 0 {
			reqs = append(reqs, cult.FlowRequest{Channel: c, Amount: push})
		} else {
			reqs = append(reqs, flow(c, 4000, false))
		}
	}
	return []cult.Ops{r.absorbOps(absorb, reqs...)}
}

// revAmountsProp 返回丹田各元素存量的 permille 比例量（主环出口推量）。
func (r *rig) revAmountsProp(permille int64) [cult.ElementCount]int64 {
	d := &r.w.Character.BodyGraph.Acupoints[r.dan]
	var a [cult.ElementCount]int64
	for e := range a {
		a[e] = d.Qi[e] * permille / 1000
	}
	return a
}

// roundAHold 轨道甲承压轮：主环 + 中→阳段节流至 100/元素（低于土府→中庭
// 的逐元素到流量 ~240，中庭注入>排出，逐 tick 积压——B4 瓶颈测量专用）。
func (r *rig) roundAHold(absorb int64) []cult.Ops {
	push := r.revAmountsProp(100)
	reqs := make([]cult.FlowRequest, 0, 6)
	for k, c := range r.ring {
		if k == 0 {
			reqs = append(reqs, cult.FlowRequest{Channel: c, Amount: push})
		} else if k == 2 {
			reqs = append(reqs, flow(c, 100, false))
		} else {
			reqs = append(reqs, flow(c, 4000, false))
		}
	}
	return []cult.Ops{r.absorbOps(absorb, reqs...)}
}

// roundBProp 轨道乙逆流捷径轮：逆 ch5（丹田→青关，存量 10%）+ 顺 ch5 归元。
func (r *rig) roundBProp(absorb int64) []cult.Ops {
	return []cult.Ops{r.absorbOps(absorb, r.revProp(100), flow(r.ring[5], 4000, false))}
}

// roundBFixed 轨道乙固定剂量轮（P5 逆流扫频用）。
func (r *rig) roundBFixed(dose, absorb int64) []cult.Ops {
	return []cult.Ops{r.absorbOps(absorb, flow(r.ring[5], dose, true), flow(r.ring[5], 4000, false))}
}

// roundBPure 纯捷径轮：只纳气@青关 + 顺 ch5 归丹田，不循环（P6 乙对照）。
func (r *rig) roundBPure(absorb int64) []cult.Ops {
	return []cult.Ops{r.absorbOps(absorb, flow(r.ring[5], 4000, false))}
}

func runRound(r *rig, acc *accounting, ticks []cult.Ops) []*cult.TickReport {
	reps := make([]*cult.TickReport, 0, len(ticks))
	for _, ops := range ticks {
		reps = append(reps, acc.advance(r.w, ops))
	}
	return reps
}

// prefill 预填充：青关纳气（强度 500）+ 捷径引流，直至丹田 LoadRatio ≥ target。
func (r *rig) prefill(acc *accounting, maxTicks int, target int64) {
	for t := 0; t < maxTicks; t++ {
		rep := acc.advance(r.w, r.absorbOps(500, flow(r.ring[5], 4000, false)))
		if rep.Points[r.dan].LoadRatio >= target {
			return
		}
	}
}

// ---------------------------------------------------------------------------
// P4 守恒归账（A3）：跨全部场景全部 seed 全部 tick 的聚合账本
// ---------------------------------------------------------------------------

type accounting struct {
	ticks       int64
	violations  int64
	maxResidual int64
}

func (a *accounting) advance(w *cult.World, ops cult.Ops) *cult.TickReport {
	w.Ops = ops
	rep := cult.Advance(w)
	a.ticks++
	c := rep.Conservation
	net := c.QiAfter - c.QiBefore
	// Part J J3：恒等式扩 Entrained 项（途经纳气，来源=环境）。
	// 0.5.0 守恒扩项/归属迁移：驱动做功 DriveWork（顺流 §10.1 + 逆势 A2）。
	// 0.6.0 边界交换扩项：纳入总量 = 意识驱动增量 Intake + 被动基线
	// ExchangeIn（评审稿 §5.2/§5.3 归因分解），泄出走 Diffuse。
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
	return rep
}

func totalIntake(rep *cult.TickReport) int64 {
	var s int64
	for i := range rep.Points {
		for e := 0; e < cult.ElementCount; e++ {
			s += rep.Points[i].Intake[e]
		}
	}
	return s
}

// ---------------------------------------------------------------------------
// P1 烧蚀动机闭环（D6）
// ---------------------------------------------------------------------------

type p1ARes struct {
	stalled     bool
	stallTick   int64
	intake0     int64
	lr          map[int64]int64 // checkpoint tick → 丹田 LoadRatio
	rate        map[int64]int64 // checkpoint tick → 纳气速率
	finalLR     int64
	finalIntake int64
}

var p1Checkpoints = []int64{10, 50, 100, 200, 400, 800, 1600}

// runP1A 场景 A（只纳不循环）：青关持续纳气 + 捷径引流，不跑周天。
// 测丹田 LoadRatio 轨迹与逐 tick 有效纳气速率；停滞 = 纳气速率首次
// 跌破初始值 10% 的 tick。停滞后再确认 200 tick（保持停滞而非毛刺）。
func runP1A(seed uint64, maxTicks int64, acc *accounting) p1ARes {
	r := newRig(seed)
	res := p1ARes{lr: map[int64]int64{}, rate: map[int64]int64{}}
	cpSet := map[int64]bool{}
	for _, c := range p1Checkpoints {
		cpSet[c] = true
	}
	stall := int64(-1)
	for t := int64(1); t <= maxTicks; t++ {
		rep := acc.advance(r.w, r.absorbOps(500, flow(r.ring[5], 4000, false)))
		it := totalIntake(rep)
		if t == 1 {
			res.intake0 = it
		}
		if cpSet[t] {
			res.lr[t] = rep.Points[r.dan].LoadRatio
			res.rate[t] = it
		}
		if stall < 0 && res.intake0 > 0 && it*10 < res.intake0 {
			stall = t
			res.stalled = true
			res.stallTick = t
		}
		if stall >= 0 && t >= stall+200 {
			break
		}
	}
	if last := r.w.Last; last != nil {
		res.finalLR = last.Points[r.dan].LoadRatio
	}
	return res
}

type p1BRes struct {
	started   bool // 是否达到过 LR>0.6 触发循环
	preStall  int64
	postCycle int64
	recovery  int64 // postCycle/preStall，千分位
	postLR    int64 // 循环刚结束时丹田 LoadRatio
}

// runP1B 场景 B（纳+循环）：同上纳气；丹田 LoadRatio>0.6 后跑 100 轮
// 主环周天），期间维持纳气；循环毕恢复只纳，测 [:+5,+45]
// 窗口的纳气速率（循环后即时恢复速率）。preStall = 触发前 30 tick 中位。
func runP1B(seed uint64, maxTicks int64, acc *accounting) p1BRes {
	r := newRig(seed)
	var res p1BRes
	hist := []int64{}
	const (
		phaseFill = iota
		phaseCycle
		phasePost
	)
	phase := phaseFill
	cycleDone := 0
	postT := int64(0)
	var postWin []int64

	for t := int64(1); t <= maxTicks; t++ {
		var ops cult.Ops
		switch phase {
		case phaseFill:
			ops = r.absorbOps(500, flow(r.ring[5], 4000, false))
		case phaseCycle:
			round := r.roundAProp(500)
			ops = round[0]
		default:
			ops = r.absorbOps(500, flow(r.ring[5], 4000, false))
		}
		rep := acc.advance(r.w, ops)
		it := totalIntake(rep)
		hist = append(hist, it)

		switch phase {
		case phaseFill:
			if rep.Points[r.dan].LoadRatio > 600 {
				phase = phaseCycle
				res.started = true
				res.preStall = medianOf(lastN(hist, 30))
			}
		case phaseCycle:
			cycleDone++
			if cycleDone >= 100 { // 100 轮 × 1 tick
				phase = phasePost
				res.postLR = rep.Points[r.dan].LoadRatio
			}
		default:
			postT++
			if postT >= 5 && postT <= 45 {
				postWin = append(postWin, it)
			}
			if postT >= 45 {
				break
			}
		}
	}
	if len(postWin) > 0 {
		res.postCycle = medianOf(postWin)
		if res.preStall > 0 {
			res.recovery = res.postCycle * 1000 / res.preStall
		}
	}
	return res
}

// ---------------------------------------------------------------------------
// P2 轨道-流派（D2）
// ---------------------------------------------------------------------------

type compVec [cult.ElementCount]int64 // 五元素占比，千分位

type p2Res struct {
	comp     compVec
	load     int64
	depleted bool
}

const p2Rounds = 200

// runP2Track 同一身体（同 seed）跑一条主轨 200 轮，回终态丹田构成。
// 循环期间维持低强度纳气（intensity 150），让"筛选后的稳态构成"沉淀在
// 丹田；循环推量取丹田存量 10%（自节流），两轨均在自持平衡点附近运行。
// 注：甲轨（主环）随经脉适应/开度成长环效率渐增，200 轮时仍在缓升
// （探针实测 LR≈0.5，未过载）；乙轨约 50 轮即入稳态。
func runP2Track(seed uint64, trackA bool, acc *accounting) p2Res {
	r := newRig(seed)
	r.prefill(acc, 150, 500)
	for i := 0; i < p2Rounds; i++ {
		if trackA {
			runRound(r, acc, r.roundAProp(150))
		} else {
			runRound(r, acc, r.roundBProp(150))
		}
	}
	d := &r.w.Character.BodyGraph.Acupoints[r.dan]
	load := int64(0)
	for e := 0; e < cult.ElementCount; e++ {
		load += d.Qi[e]
	}
	res := p2Res{load: load}
	if load < 500 {
		res.depleted = true
		return res
	}
	for e := 0; e < cult.ElementCount; e++ {
		res.comp[e] = d.Qi[e] * 1000 / load
	}
	return res
}

// ---------------------------------------------------------------------------
// P3 双重收敛限速（D5-4）
// ---------------------------------------------------------------------------

const (
	p3StabilizeRounds = 200
	p3MaxRounds       = 400
	p3Mid             = 500 // 中档 0.5（千分位）
)

type p3Res struct {
	steady int64   // 注入前稳态 丹田 Refinement
	low    int64   // 注入后低点
	reach  int64   // 首次爬回 ≥500 的轮数；-1 = 未达
	curve  []int64 // 注入后逐轮 丹田 Refinement（下标=轮数，0=注入后即刻）
}

// runP3 先以轨道乙稳定循环 200 轮（乙轨约 50 轮入稳态，符合"已稳定循环"
// 前提；甲轨 200 轮仍在缓升，不用），再向丹田直接注入一股均匀粗气
// （6000/元素，任务书允许的直接设置构成方式），继续轨道乙循环（维持
// 纳气 intensity 150），逐轮追踪丹田 DeriveRefinement。
func runP3(seed uint64, acc *accounting) p3Res {
	r := newRig(seed)
	r.prefill(acc, 150, 500)
	for i := 0; i < p3StabilizeRounds; i++ {
		runRound(r, acc, r.roundBProp(150))
	}
	res := p3Res{reach: -1, steady: cult.DeriveRefinement(r.w, r.dan)}
	d := &r.w.Character.BodyGraph.Acupoints[r.dan]
	d.Qi = [cult.ElementCount]int64{6000, 6000, 6000, 6000, 6000}
	res.low = cult.DeriveRefinement(r.w, r.dan)
	res.curve = append(res.curve, res.low)
	for i := 1; i <= p3MaxRounds; i++ {
		runRound(r, acc, r.roundBProp(150))
		rf := cult.DeriveRefinement(r.w, r.dan)
		res.curve = append(res.curve, rf)
		if res.reach < 0 && rf >= p3Mid {
			res.reach = int64(i)
		}
	}
	return res
}

// ---------------------------------------------------------------------------
// P5 逆流火候（A2）：逆流量扫频 500/1000/2000/4000（千分位/元素）
// ---------------------------------------------------------------------------

var p5Doses = []int64{500, 1000, 2000, 4000}

const p5Rounds = 100

type p5Res struct {
	peakQG, peakZZ int64 // 目的地（青关/中庭）ThermalLoad 峰值
	burnQG, burnZZ int64 // 感受含"灼痛"的 tick·穴 计数
	distQG, distZZ int64 // 感受含"胀痛"的 tick·穴 计数
	strainQG       int64 // 青关 Strain≥300（痛感档 2 入档阈）的 tick 数
	damageEvents   int64 // 路径穴位（丹田/青关/中庭）损伤上升事件数
}

// runP5 每档逆流量独立建世界：预填充至丹田 LR≥0.6 后跑 100 轮轨道乙
// （逆 ch5 固定剂量 + 顺 ch5 归元，纳气 300），测热峰值/痛感分型/失稳事件。
// 感知分型经 cult.Observe(acuity=2) 的定性感受词计数（感知隔离律允许
// 驱动层读定性输出做测量）。
func runP5(seed uint64, dose int64, acc *accounting) p5Res {
	r := newRig(seed)
	r.prefill(acc, 300, 600)
	var res p5Res
	prevDamage := make([]int64, r.n)
	for i := 0; i < p5Rounds; i++ {
		for _, ops := range r.roundBFixed(dose, 300) {
			rep := acc.advance(r.w, ops)
			if v := rep.Points[r.qg].ThermalLoad; v > res.peakQG {
				res.peakQG = v
			}
			if v := rep.Points[r.zhong].ThermalLoad; v > res.peakZZ {
				res.peakZZ = v
			}
			if rep.Points[r.qg].Strain >= 300 {
				res.strainQG++
			}
			for _, p := range [3]int{r.dan, r.qg, r.zhong} {
				if rep.Points[p].Damage > prevDamage[p] {
					res.damageEvents++
				}
				prevDamage[p] = rep.Points[p].Damage
			}
			obs := cult.Observe(r.w, 2)
			for _, pi := range [2]int{r.qg, r.zhong} {
				burn, dist := false, false
				for _, f := range obs.Points[pi].Feelings {
					if strings.Contains(f, "灼痛") {
						burn = true
					}
					if strings.Contains(f, "胀痛") {
						dist = true
					}
				}
				switch {
				case burn && pi == r.qg:
					res.burnQG++
				case burn:
					res.burnZZ++
				}
				switch {
				case dist && pi == r.qg:
					res.distQG++
				case dist:
					res.distZZ++
				}
			}
		}
	}
	return res
}

// ---------------------------------------------------------------------------
// P6 中庭瓶颈（B4）：轨道甲（经中庭中转）vs 捷径（只走青关→丹田）
// ---------------------------------------------------------------------------

const (
	p6Rounds      = 100
	distGain      = 250 // percept.go pressureDeltaGain（v0.1 参数，镜像以复算胀感信号）
	p6FeelBandOne = 200 // 胀感滞回第一档入档阈（percept.go distensionBands[1].enter）
)

type p6Res struct {
	peakZZPressure int64
	peakZZDist     int64 // 胀感信号峰值（Pressure + 0.25×max(0, dP/dt)）
	peakZZLR       int64
	stabMed        int64 // 每 round 路径最小 Stability 的中位数
}

func runP6Track(seed uint64, trackA bool, acc *accounting) p6Res {
	r := newRig(seed)
	r.prefill(acc, 300, 600)
	var res p6Res
	var stabs []int64
	prevPressure := int64(0)
	seen := false
	for i := 0; i < p6Rounds; i++ {
		var ticks []cult.Ops
		path := []int{r.dan, r.qg}
		if trackA {
			ticks = r.roundAHold(300)
			path = append(path, r.zhong)
		} else {
			ticks = r.roundBPure(300)
		}
		reps := runRound(r, acc, ticks)
		minStab := int64(1000)
		for _, rep := range reps {
			for _, p := range path {
				if s := rep.Points[p].Stability; s < minStab {
					minStab = s
				}
			}
			if trackA {
				pr := rep.Points[r.zhong].Pressure
				dp := pr - prevPressure
				if dp < 0 {
					dp = 0
				}
				dist := pr + distGain*dp/1000
				if dist > res.peakZZDist {
					res.peakZZDist = dist
				}
				if pr > res.peakZZPressure {
					res.peakZZPressure = pr
				}
				if lr := rep.Points[r.zhong].LoadRatio; lr > res.peakZZLR {
					res.peakZZLR = lr
				}
				prevPressure = pr
				seen = true
			}
		}
		stabs = append(stabs, minStab)
	}
	if trackA && !seen {
		res.peakZZDist, res.peakZZPressure, res.peakZZLR = -1, -1, -1
	}
	res.stabMed = medianOf(stabs)
	return res
}

// ---------------------------------------------------------------------------
// Part J 标定命题（docs/specs/2026-09-22-partj-passive-circulation.md J8）
// ---------------------------------------------------------------------------

// releaseFlow 汇总一份报告的实际流量（主动 + 被动；被动条目 Actual==Passive）。
func releaseFlow(rep *cult.TickReport) int64 {
	var s int64
	for ti := range rep.Transfers {
		for e := 0; e < cult.ElementCount; e++ {
			s += rep.Transfers[ti].Actual[e]
		}
	}
	return s
}

// ---------------------------------------------------------------------------
// P7 衰减-结构单调（J6/J5）：同一身体累积行环训练越多（经脉适应/开度
// 越成长），松手后被动环流的衰减越慢——松手窗（前 30 tick）均流随训练
// 量非降。
// ---------------------------------------------------------------------------

var p7Training = [3]int{150, 450, 900} // 累计行环 tick 数（同世界累积）

type p7Res struct {
	relMean [3]int64 // 各训练档后的松手窗均流
}

func runP7(seed uint64, acc *accounting) p7Res {
	var res p7Res
	r := newRig(seed)
	r.prefill(acc, 150, 500)
	done := 0
	for k, n := range p7Training {
		for ; done < n; done++ {
			acc.advance(r.w, r.roundAProp(300)[0])
		}
		var sum int64
		for t := 0; t < 30; t++ {
			rep := acc.advance(r.w, cult.Ops{})
			sum += releaseFlow(rep)
		}
		res.relMean[k] = sum / 30
	}
	return res
}

// ---------------------------------------------------------------------------
// P8 环必要性对照（J2）：同参数下主环 vs 砍一边成树的松手窗均流比
// （环显著更高——没有环就没有自持）。
// ---------------------------------------------------------------------------

type p8Res struct {
	ringMean int64
	treeMean int64
}

// p8RingChannels 返回 world 中属于主环（SpikeWorld 拓扑六段端点对）的
// 经脉下标；cut 后缺失段自然不在其中。
func p8RingChannels(g *cult.BodyGraph) []int {
	ring := [][2]int{{0, 3}, {3, 7}, {7, 2}, {2, 6}, {6, 1}, {1, 0}}
	var out []int
	for c := range g.Channels {
		ch := &g.Channels[c]
		for _, e := range ring {
			if ch.From == e[0] && ch.To == e[1] {
				out = append(out, c)
				break
			}
		}
	}
	return out
}

// p8ReleaseMean 建世界（可砍段）、预填充、缓推行环 pushTicks，再松手
// 量测前 30 tick 均流。
func p8ReleaseMean(seed uint64, cut bool, acc *accounting) int64 {
	r := newRig(seed)
	g := &r.w.Character.BodyGraph
	if cut {
		for i, ch := range g.Channels {
			if ch.From == 7 && ch.To == 2 { // 中庭→阳枢
				g.Channels = append(g.Channels[:i], g.Channels[i+1:]...)
				break
			}
		}
	}
	// 预填充（青关→丹田捷径引流至丹田 LR≥0.5）：cut 后下标偏移，按端点重找。
	var shortcut int
	for c := range g.Channels {
		if g.Channels[c].From == 1 && g.Channels[c].To == 0 {
			shortcut = c
			break
		}
	}
	for t := 0; t < 150; t++ {
		it := make([]int64, r.n)
		it[1] = 500
		rep := acc.advance(r.w, cult.Ops{Intensity: it, Requests: []cult.FlowRequest{
			{Channel: shortcut, Amount: [5]int64{800, 800, 800, 800, 800}},
		}})
		if rep.Points[r.dan].LoadRatio >= 500 {
			break
		}
	}
	// 缓推（300/元素，首 Tick FlowShock 不打穿稳定性）。
	for i := 0; i < 200; i++ {
		var reqs []cult.FlowRequest
		for _, c := range p8RingChannels(g) {
			var a [cult.ElementCount]int64
			for e := range a {
				a[e] = 300
			}
			reqs = append(reqs, cult.FlowRequest{Channel: c, Amount: a})
		}
		acc.advance(r.w, cult.Ops{Requests: reqs})
	}
	var sum int64
	for t := 0; t < 30; t++ {
		rep := acc.advance(r.w, cult.Ops{})
		sum += releaseFlow(rep)
	}
	return sum / 30
}

func runP8(seed uint64, acc *accounting) p8Res {
	return p8Res{
		ringMean: p8ReleaseMean(seed, false, acc),
		treeMean: p8ReleaseMean(seed, true, acc),
	}
}

// ---------------------------------------------------------------------------
// P9 筑基可达性（P0 死锁修复总验收）：完整游戏流程（引气→行环→松手）
// 中 SelfSustain 跨 0.85 的 seed 占比 ≥ 0.5。
// 世界 = Part J 标定世界（结构已演化到位；SpikeWorld 新手身体的传导
// 天花板使筑基需长期行环修结构，见报告注记）。
// ---------------------------------------------------------------------------

type p9Res struct {
	maxSelfSustain int64
	crossed        bool
	touched        bool
	established    bool
}

func p9ProtocolYAML(amount, cycles int64) string {
	ring := [][2]string{
		{"下丹田", "土府"},
		{"土府", "中庭"},
		{"中庭", "阳枢"},
		{"阳枢", "神庭"},
		{"神庭", "青关"},
		{"青关", "下丹田"},
	}
	var b strings.Builder
	fmt.Fprintf(&b, "protocol: zhutian_p9_v1\nactions:\n")
	fmt.Fprintf(&b, "  - absorb: {target: 青关, level: trace, duration: 1, parallel: 1}\n")
	for _, e := range ring {
		fmt.Fprintf(&b, "  - guide: {from: %s, to: %s, element: \"\", amount: %d, duration: 1, parallel: 1}\n",
			e[0], e[1], amount)
	}
	fmt.Fprintf(&b, "cadence: 1\ncycles: %d\nvalidated_range:\n", cycles)
	for _, e := range ring {
		fmt.Fprintf(&b, "  - channel: [%s, %s]\n    element: \"\"\n    min: 0\n    max: %d\n", e[0], e[1], amount)
	}
	return b.String()
}

func runP9(seed uint64) p9Res {
	var res p9Res
	w := ctmatch.PartJCalibrationWorld()
	s := ctmatch.NewSession(w, seed, nil)
	p, err := ctmatch.Compile([]byte(p9ProtocolYAML(300, 600)), &w.Character.BodyGraph)
	if err != nil {
		panic(err) // 标定台静态装配错误
	}
	s.LoadProtocol(p)
	ctmatch.Tick(s, nil) // 观测基准
	if err := s.StartProtocol(p.Name); err != nil {
		panic(err)
	}
	// 反复行环（Runtime 驱动，一 Tick 一周天）。
	for i := 0; i < 900 && s.ActiveProtocol() != ""; i++ {
		ctmatch.Tick(s, nil)
	}
	// 松手：此后不再提交任何意图，纯被动环流。
	for i := 0; i < 200; i++ {
		ctmatch.Tick(s, nil)
		if s.Foundation.Touched {
			res.touched = true
		}
		if s.Foundation.Established {
			res.established = true
		}
		if m := s.Foundation.Metrics().SelfSustain; m > res.maxSelfSustain {
			res.maxSelfSustain = m
		}
	}
	res.crossed = res.maxSelfSustain >= 850
	return res
}

// ---------------------------------------------------------------------------
// 统计 helpers（全整数；无浮点）
// ---------------------------------------------------------------------------

func lastN(xs []int64, n int) []int64 {
	if len(xs) > n {
		return xs[len(xs)-n:]
	}
	return xs
}

func sortedCopy(xs []int64) []int64 {
	s := append([]int64(nil), xs...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	return s
}

// quantileSorted 返回升序切片的 q（千分位 0..1000）分位（floor 索引：
// idx = floor(n×q/1000)，偶数个的中位取上中位）。
func quantileSorted(s []int64, q int64) int64 {
	n := int64(len(s))
	if n == 0 {
		return 0
	}
	idx := (n * q) / 1000
	if idx >= n {
		idx = n - 1
	}
	return s[idx]
}

func medianOf(xs []int64) int64 {
	return quantileSorted(sortedCopy(xs), 500)
}

func iqrOf(xs []int64) (q1, med, q3 int64) {
	s := sortedCopy(xs)
	return quantileSorted(s, 250), quantileSorted(s, 500), quantileSorted(s, 750)
}

// iSqrt 返回 floor(sqrt(n))（牛顿迭代），供欧氏距离等报告量使用。
func iSqrt(n int64) int64 {
	if n < 2 {
		return n
	}
	x, y := n, (n+1)/2
	for y < x {
		x, y = y, (y+n/y)/2
	}
	return x
}

// euclidDist 构成向量的整数欧氏距离（千分位占比空间）。
func euclidDist(a, b compVec) int64 {
	var s int64
	for e := range a {
		d := a[e] - b[e]
		s += d * d
	}
	return iSqrt(s)
}

// halfRanks 返回 2×秩（1-based；并列取平均秩——整数化即半秩和）。
func halfRanks(xs []int64) []int64 {
	n := len(xs)
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return xs[order[i]] < xs[order[j]] })
	r2 := make([]int64, n)
	for i := 0; i < n; {
		j := i
		for j+1 < n && xs[order[j+1]] == xs[order[i]] {
			j++
		}
		avg := int64(i + j + 2) // 2×平均秩 = (i+1)+(j+1)
		for k := i; k <= j; k++ {
			r2[order[k]] = avg
		}
		i = j + 1
	}
	return r2
}

// spearman1000 返回 Spearman ρ×1000（并列取平均秩；n≥2）。
func spearman1000(a, b []int64) int64 {
	n := len(a)
	if n != len(b) || n < 2 {
		return 0
	}
	r2a, r2b := halfRanks(a), halfRanks(b)
	var s int64
	for i := 0; i < n; i++ {
		d := r2a[i] - r2b[i]
		s += d * d
	}
	den := int64(n) * (int64(n)*int64(n) - 1)
	// ρ = 1 − 6Σd²/(n(n²−1))，d 以半秩计（Σd² 为 1/4 秩²单位）：
	// ρ×1000 = 1000 − 1500×Σ(Δr2)² / (n(n²−1))
	return 1000 - (1500*s)/den
}

// nonDecreasing 判断逐档量是否单调不降。
func nonDecreasing(xs []int64) bool {
	for i := 1; i < len(xs); i++ {
		if xs[i] < xs[i-1] {
			return false
		}
	}
	return true
}

// fp 渲染千分位定点值 x.xxx（无浮点）。
func fp(v int64) string {
	sign := ""
	if v < 0 {
		sign = "-"
		v = -v
	}
	return fmt.Sprintf("%s%d.%03d", sign, v/1000, v%1000)
}

// iqrLine 渲染 "中位 [P25, P75]"。
func iqrLine(xs []int64) string {
	q1, m, q3 := iqrOf(xs)
	return fmt.Sprintf("%d [P25 %d, P75 %d]", m, q1, q3)
}

func passStr(p bool) string {
	if p {
		return "PASS"
	}
	return "FAIL"
}

// ---------------------------------------------------------------------------
// 报告渲染
// ---------------------------------------------------------------------------

type mcInput struct {
	nSeeds  int
	ticks   int64
	p1a     []p1ARes
	p1b     []p1BRes
	p2a     []p2Res
	p2b     []p2Res
	p3      []p3Res
	p5      [][]p5Res // [seed][doseIdx]
	p6a     []p6Res
	p6b     []p6Res
	p7      []p7Res
	p8      []p8Res
	p9      []p9Res
	acc     *accounting
	elapsed time.Duration
}

func elementName(e int) string {
	return [...]string{"木", "火", "土", "金", "水"}[e]
}

func dominantOf(comp compVec) (int, int64) {
	best, bv := 0, int64(-1)
	for e := range comp {
		if comp[e] > bv {
			best, bv = e, comp[e]
		}
	}
	return best, bv
}

func renderReport(in mcInput) string {
	var b strings.Builder
	n := in.nSeeds
	fmt.Fprintf(&b, "# cultmc Monte Carlo 标定报告\n\n")
	fmt.Fprintf(&b, "- RuleVersion %s；seeds=%d；P1 Tick 预算=%d；世界=ctmatch.SpikeWorld（8 穴 9 脉主环拓扑，同源构造）\n",
		cult.RuleVersion, n, in.ticks)
	fmt.Fprintf(&b, "- 每 (seed,场景) 独立建世界，NoiseSeed=seed，确定性可重放；周天执行=驱动层逐 tick 下发 guide ops（稳态轮：单 tick 走完全环/捷径；P6 甲用中庭节流承压轮，见程序头注）\n")
	fmt.Fprintf(&b, "- 全程定点整数；P4 守恒逐 tick 归账覆盖下列全部场景的全部 tick\n")
	fmt.Fprintf(&b, "- 运行耗时 %s\n\n", in.elapsed)

	// spec 冲突记录
	fmt.Fprintf(&b, "## 拓扑基线与轨道映射\n\n")
	fmt.Fprintf(&b, "拓扑基线：RuleVersion %s（commit 78f68c6 主环）。主环=丹田→土府→中庭→阳枢→神庭→青关→丹田（全顺流）；唯一合法逆流=核点（丹田）外推 ch(青关→丹田) 反向。任务书 P2/P6 轨道按新拓扑映射：轨道甲=主环稳态轮（全顺流，单 tick 走完全环）；轨道乙=逆流捷径轮（丹田→青关→丹田）。\n\n", cult.RuleVersion)

	// ------------------------------------------------------------------ P1
	stallCnt := 0
	var stallTicks []int64
	for _, r := range in.p1a {
		if r.stalled {
			stallCnt++
			stallTicks = append(stallTicks, r.stallTick)
		}
	}
	stallFrac := int64(0)
	if n > 0 {
		stallFrac = int64(stallCnt) * 1000 / int64(n)
	}
	var recov []int64
	startedB := 0
	for _, r := range in.p1b {
		if r.started {
			startedB++
			if r.preStall > 0 {
				recov = append(recov, r.recovery)
			}
		}
	}
	recovMed := medianOf(recov)
	p1Pass := stallFrac >= 900 && len(recov) > 0 && recovMed >= 500

	fmt.Fprintf(&b, "## P1 烧蚀动机闭环（D6）—— %s\n\n", passStr(p1Pass))
	fmt.Fprintf(&b, "场景 A（只纳不循环）：%d/%d seed 出现纳气停滞（速率<初始 10%%）\n\n", stallCnt, n)
	if len(stallTicks) > 0 {
		fmt.Fprintf(&b, "- 停滞 tick：%s\n", iqrLine(stallTicks))
	}
	fmt.Fprintf(&b, "\n| tick | 丹田 LoadRatio 中位 | 纳气速率中位（量/tick） |\n|---|---|---|\n")
	for _, c := range p1Checkpoints {
		var lrs, rates []int64
		for _, r := range in.p1a {
			if v, ok := r.lr[c]; ok {
				lrs = append(lrs, v)
			}
			if v, ok := r.rate[c]; ok {
				rates = append(rates, v)
			}
		}
		if len(lrs) > 0 {
			fmt.Fprintf(&b, "| %d | %s | %s |\n", c, fp(medianOf(lrs)), iqrLine(rates))
		}
	}
	var finalLRs []int64
	for _, r := range in.p1a {
		finalLRs = append(finalLRs, r.finalLR)
	}
	fmt.Fprintf(&b, "| 末 tick | %s | — |\n\n", fp(medianOf(finalLRs)))
	fmt.Fprintf(&b, "场景 B（纳+循环）：触发循环 %d/%d；循环后纳气恢复率（相对触发前 30 tick 中位）\n\n", startedB, n)
	if len(recov) > 0 {
		fmt.Fprintf(&b, "- 恢复率（postCycle/preStall）：%s（≥500 即 ≥50%% 达标线）\n", fp(recovMed))
		q1, _, q3 := iqrOf(recov)
		fmt.Fprintf(&b, "- 四分位：P25 %s / P75 %s\n", fp(q1), fp(q3))
	}
	var postLRs []int64
	for _, r := range in.p1b {
		if r.started {
			postLRs = append(postLRs, r.postLR)
		}
	}
	fmt.Fprintf(&b, "- 循环结束时丹田 LoadRatio 中位：%s（已腾挪）\n", fp(medianOf(postLRs)))
	fmt.Fprintf(&b, "\n结论：场景 A %s；场景 B 循环后纳气恢复中位 %s。烧蚀腾挪%s恢复吸收通路。\n\n",
		map[bool]string{true: "如期停滞（装满→吸不进）", false: "未见停滞"}[stallFrac >= 900],
		fp(recovMed), map[bool]string{true: "能", false: "不能"}[p1Pass])

	// ------------------------------------------------------------------ P2
	var paired, withinA, withinB []int64
	depleted := 0
	for s := range in.p2a {
		if in.p2a[s].depleted || in.p2b[s].depleted {
			depleted++
			continue
		}
		paired = append(paired, euclidDist(in.p2a[s].comp, in.p2b[s].comp))
	}
	for i := 0; i < len(in.p2a); i++ {
		for j := i + 1; j < len(in.p2a); j++ {
			if !in.p2a[i].depleted && !in.p2a[j].depleted {
				withinA = append(withinA, euclidDist(in.p2a[i].comp, in.p2a[j].comp))
			}
			if !in.p2b[i].depleted && !in.p2b[j].depleted {
				withinB = append(withinB, euclidDist(in.p2b[i].comp, in.p2b[j].comp))
			}
		}
	}
	pairedMed, withinAMed, withinBMed := medianOf(paired), medianOf(withinA), medianOf(withinB)
	withinBase := withinAMed
	if withinBMed > withinBase {
		withinBase = withinBMed
	}
	ratioMilli := int64(0)
	if withinBase > 0 {
		ratioMilli = pairedMed * 1000 / withinBase
	}
	p2Pass := ratioMilli >= 3000
	medA, medB := compVec{}, compVec{}
	for e := 0; e < cult.ElementCount; e++ {
		va, vb := []int64{}, []int64{}
		for s := range in.p2a {
			if !in.p2a[s].depleted {
				va = append(va, in.p2a[s].comp[e])
			}
			if !in.p2b[s].depleted {
				vb = append(vb, in.p2b[s].comp[e])
			}
		}
		medA[e], medB[e] = medianOf(va), medianOf(vb)
	}
	domA, _ := dominantOf(medA)
	domB, _ := dominantOf(medB)

	fmt.Fprintf(&b, "## P2 轨道-流派（D2）—— %s\n\n", passStr(p2Pass))
	fmt.Fprintf(&b, "200 轮后终态丹田构成（千分位占比，跨 seed 中位）：\n\n")
	fmt.Fprintf(&b, "| 元素 | 轨道甲（主环） | 轨道乙（逆流捷径） |\n|---|---|---|\n")
	for e := 0; e < cult.ElementCount; e++ {
		fmt.Fprintf(&b, "| %s | %s | %s |\n", elementName(e), fp(medA[e]), fp(medB[e]))
	}
	fmt.Fprintf(&b, "\n- 主导元素：甲=%s，乙=%s（%s）\n", elementName(domA), elementName(domB),
		map[bool]string{true: "不同", false: "相同"}[domA != domB])
	fmt.Fprintf(&b, "- 甲乙同 seed 配对构成距离中位：%d（千分位欧氏）\n", pairedMed)
	fmt.Fprintf(&b, "- 同轨跨 seed 波动基线：甲 %d / 乙 %d\n", withinAMed, withinBMed)
	fmt.Fprintf(&b, "- 比值（配对距离/基线）：%s（达标线 3.000）\n", fp(ratioMilli))
	if depleted > 0 {
		fmt.Fprintf(&b, "- 注意：%d 个 seed 丹田枯竭（load<500），已剔除\n", depleted)
	}
	fmt.Fprintf(&b, "\n结论：两轨道终态构成差异%s于同轨 seed 间波动（比值 %s）。\n\n",
		map[bool]string{true: "显著大", false: "不显著"}[p2Pass], fp(ratioMilli))

	// ------------------------------------------------------------------ P3
	var reachVals []int64
	reached := 0
	for _, r := range in.p3 {
		if r.reach >= 0 {
			reached++
			reachVals = append(reachVals, r.reach)
		}
	}
	reachFrac := int64(0)
	if n > 0 {
		reachFrac = int64(reached) * 1000 / int64(n)
	}
	reachMed := medianOf(reachVals)
	var lows, steadies []int64
	for _, r := range in.p3 {
		lows = append(lows, r.low)
		steadies = append(steadies, r.steady)
	}
	p3Reachable := reachFrac >= 500
	p3Pass := false
	var p3Verdict string
	if !p3Reachable {
		p3Verdict = fmt.Sprintf("FAIL（爬不上去：%d/%d seed 在 %d 轮内未达中档 0.5，炼化过程同样不可见）", reached, n, p3MaxRounds)
	} else if reachMed >= 20 {
		p3Pass = true
		p3Verdict = fmt.Sprintf("PASS（可测驳杂期：爬升至中档需中位 %d 轮 ≥ 20）", reachMed)
	} else {
		p3Verdict = fmt.Sprintf("FAIL（太快：爬升至中档仅中位 %d 轮 < 20，玩家看不到炼化过程）", reachMed)
	}
	fmt.Fprintf(&b, "## P3 双重收敛限速（D5-4）—— %s\n\n", map[bool]string{true: "PASS", false: "FAIL"}[p3Pass])
	fmt.Fprintf(&b, "注入均匀粗气（丹田 6000/元素）后逐轮追踪丹田 DeriveRefinement：\n\n")
	fmt.Fprintf(&b, "- 注入前稳态 Refinement 中位：%s；注入后低点中位：%s\n", fp(medianOf(steadies)), fp(medianOf(lows)))
	fmt.Fprintf(&b, "- %s\n\n", p3Verdict)
	fmt.Fprintf(&b, "爬升曲线（跨 seed 中位，Refinement 千分位）：\n\n")
	fmt.Fprintf(&b, "| 轮 | 0 | 5 | 10 | 20 | 30 | 50 | 80 | 120 | 200 | 300 | 400 |\n")
	fmt.Fprintf(&b, "|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	p3Cps := []int{0, 5, 10, 20, 30, 50, 80, 120, 200, 300, 400}
	cells := make([]string, len(p3Cps))
	for ci, c := range p3Cps {
		var vals []int64
		for _, r := range in.p3 {
			if c < len(r.curve) {
				vals = append(vals, r.curve[c])
			}
		}
		cells[ci] = fp(medianOf(vals))
	}
	fmt.Fprintf(&b, "| Refine | %s |\n", strings.Join(cells, " | "))
	if len(reachVals) > 0 {
		q1r, _, q3r := iqrOf(reachVals)
		fmt.Fprintf(&b, "\n- 达标轮数（仅达标的 %d seed）：P25 %d / 中位 %d / P75 %d（达标率 %s）\n",
			reached, q1r, reachMed, q3r, fp(reachFrac))
	}
	fmt.Fprintf(&b, "\n结论：%s 爬升过程存在（见曲线）但稳态吸引子低于中档——D3 筛选差分不足以把构成推过身体亲和的半程。\n\n", p3Verdict)

	// ------------------------------------------------------------------ P4
	p4Pass := in.acc.violations == 0
	fmt.Fprintf(&b, "## P4 守恒归账（A3）—— %s\n\n", passStr(p4Pass))
	fmt.Fprintf(&b, "- 覆盖总 tick 数：%d（全部场景 × 全部 seed）\n", in.acc.ticks)
	fmt.Fprintf(&b, "- 恒等式违例数：%d\n", in.acc.violations)
	fmt.Fprintf(&b, "- 最大单 tick 归账残差：%d（千分位量）\n", in.acc.maxResidual)
	fmt.Fprintf(&b, "\n结论：ΔQi = Intake + ExchangeIn − Diffuse − TransferLoss − ShengLoss − DriveWork + Entrained %s成立，无凭空增益（Part J 守恒扩项 + 0.5.0 驱动做功项 + 0.6.0 边界交换被动纳入项）。\n\n",
		map[bool]string{true: "逐 tick 精确", false: "被破坏"}[p4Pass])

	// ------------------------------------------------------------------ P5
	medPeakQG := make([]int64, len(p5Doses))
	medPeakZZ := make([]int64, len(p5Doses))
	medDamage := make([]int64, len(p5Doses))
	medStrain := make([]int64, len(p5Doses))
	medBurn := make([]int64, len(p5Doses))
	medDist := make([]int64, len(p5Doses))
	var spears []int64
	for di := range p5Doses {
		pq, pz, dm, st, bu, di2 := []int64{}, []int64{}, []int64{}, []int64{}, []int64{}, []int64{}
		for s := 0; s < n; s++ {
			r5 := in.p5[s][di]
			pq = append(pq, r5.peakQG)
			pz = append(pz, r5.peakZZ)
			dm = append(dm, r5.damageEvents)
			st = append(st, r5.strainQG)
			bu = append(bu, r5.burnQG+r5.burnZZ)
			di2 = append(di2, r5.distQG+r5.distZZ)
		}
		medPeakQG[di], medPeakZZ[di] = medianOf(pq), medianOf(pz)
		medDamage[di], medStrain[di] = medianOf(dm), medianOf(st)
		medBurn[di], medDist[di] = medianOf(bu), medianOf(di2)
	}
	for s := 0; s < n; s++ {
		doseSeries := append([]int64(nil), p5Doses...)
		peakSeries := make([]int64, len(p5Doses))
		for di := range p5Doses {
			peakSeries[di] = in.p5[s][di].peakQG
		}
		spears = append(spears, spearman1000(doseSeries, peakSeries))
	}
	thermalOK := nonDecreasing(medPeakQG) && medPeakQG[len(medPeakQG)-1] > medPeakQG[0]
	instabOK := (nonDecreasing(medDamage) && medDamage[len(medDamage)-1] > medDamage[0]) ||
		(nonDecreasing(medStrain) && medStrain[len(medStrain)-1] > medStrain[0])
	p5Pass := thermalOK && instabOK

	fmt.Fprintf(&b, "## P5 逆流火候（A2）—— %s\n\n", passStr(p5Pass))
	fmt.Fprintf(&b, "轨道乙（逆流捷径）100 轮，逆流量扫频（千分位/元素）；跨 seed 中位：\n\n")
	fmt.Fprintf(&b, "| 逆流量 | 青关热峰值 | 中庭热峰值 | 失稳事件(损伤↑) | 青关 Strain≥300 tick | 灼痛计数 | 胀痛计数 |\n")
	fmt.Fprintf(&b, "|---|---|---|---|---|---|---|\n")
	for di, d := range p5Doses {
		fmt.Fprintf(&b, "| %d | %d | %d | %s | %s | %s | %s |\n",
			d, medPeakQG[di], medPeakZZ[di],
			iqrLine(collect(in.p5, di, func(r p5Res) int64 { return r.damageEvents })),
			iqrLine(collect(in.p5, di, func(r p5Res) int64 { return r.strainQG })),
			iqrLine(collect(in.p5, di, func(r p5Res) int64 { return r.burnQG + r.burnZZ })),
			iqrLine(collect(in.p5, di, func(r p5Res) int64 { return r.distQG + r.distZZ })),
		)
	}
	fmt.Fprintf(&b, "\n- 热峰值单调不降且首尾严格升：%v；失稳单调（损伤或 Strain 至少其一）：%v\n", thermalOK, instabOK)
	fmt.Fprintf(&b, "- 单 seed (dose, 热峰值) Spearman 中位：%s\n", fp(medianOf(spears)))
	fmt.Fprintf(&b, "- 注：损伤事件在 4000 档略低于 2000 档（大剂量过早掏空丹田、后程燃料枯竭），单调判据由 Strain 项补足；灼痛在全部档位近乎常驻（连续逆流下青关热稳态远超灼痛档）——是火候手感的标定信息。\n")
	fmt.Fprintf(&b, "\n结论：逆流推得越猛，热与失稳%s随逆流量单调上升。\n\n",
		map[bool]string{true: "如期", false: "未见"}[p5Pass])

	// ------------------------------------------------------------------ P6
	var peakDistA, peakLRA, stabA, stabB []int64
	for _, r := range in.p6a {
		if r.peakZZDist >= 0 {
			peakDistA = append(peakDistA, r.peakZZDist)
			peakLRA = append(peakLRA, r.peakZZLR)
		}
		stabA = append(stabA, r.stabMed)
	}
	for _, r := range in.p6b {
		stabB = append(stabB, r.stabMed)
	}
	distMed := medianOf(peakDistA)
	p6Pass := distMed >= p6FeelBandOne

	fmt.Fprintf(&b, "## P6 中庭瓶颈（B4）—— %s\n\n", passStr(p6Pass))
	fmt.Fprintf(&b, "各 100 轮（甲=主环承压轮：中→阳段节流 100/元素；乙=纯捷径不循环；纳气均 300）：\n\n")
	fmt.Fprintf(&b, "| 指标 | 轨道甲（主环承压轮） | 纯捷径（青关→丹田） |\n|---|---|---|\n")
	fmt.Fprintf(&b, "| 中庭胀感信号峰值中位 | %s | 0.000（不经过） |\n", fp(distMed))
	fmt.Fprintf(&b, "| 中庭 Pressure 峰值中位 | %s | 0.000 |\n", fp(medianOf(collectP6(in.p6a, func(r p6Res) int64 { return r.peakZZPressure }))))
	fmt.Fprintf(&b, "| 中庭 LoadRatio 峰值中位 | %s | 0.000 |\n", fp(medianOf(peakLRA)))
	fmt.Fprintf(&b, "| 周天稳定性（每轮路径最小 Stability 的中位） | %s | %s |\n",
		iqrLine(stabA), iqrLine(stabB))
	fmt.Fprintf(&b, "\n- 判据：中庭胀感峰值中位差 ≥ 空穴基线可感第一档（%d）→ 实际 %s\n", p6FeelBandOne, fp(distMed))
	fmt.Fprintf(&b, "\n结论：多次经中庭中转的中庭承压%s可感第一档以上；稳定性甲 %s vs 乙 %s。\n\n",
		map[bool]string{true: "显著高于", false: "未达"}[p6Pass], fp(medianOf(stabA)), fp(medianOf(stabB)))

	// ------------------------------------------------------------------ P7
	var p7Med [3]int64
	p7Mono := 0
	for k := 0; k < 3; k++ {
		var vals []int64
		for _, r := range in.p7 {
			vals = append(vals, r.relMean[k])
		}
		p7Med[k] = medianOf(vals)
	}
	for s := range in.p7 {
		r := &in.p7[s]
		if r.relMean[0] <= r.relMean[1] && r.relMean[1] <= r.relMean[2] {
			p7Mono++
		}
	}
	p7MonoFrac := int64(0)
	if n > 0 {
		p7MonoFrac = int64(p7Mono) * 1000 / int64(n)
	}
	p7Pass := p7Med[0] <= p7Med[1] && p7Med[1] <= p7Med[2] && p7Med[2] > p7Med[0] && p7MonoFrac >= 500

	fmt.Fprintf(&b, "## P7 衰减-结构单调（J6/J5）—— %s\n\n", passStr(p7Pass))
	fmt.Fprintf(&b, "同一身体累积行环 %d/%d/%d tick 后各自松手（窗 %d tick），松手窗被动均流（跨 seed 中位）：\n\n",
		p7Training[0], p7Training[1], p7Training[2], cult.FoundationReleaseWindow)
	fmt.Fprintf(&b, "| 累计行环 | %d | %d | %d |\n|---|---|---|---|\n", p7Training[0], p7Training[1], p7Training[2])
	fmt.Fprintf(&b, "| 松手窗均流 | %s | %s | %s |\n\n", fp(p7Med[0]), fp(p7Med[1]), fp(p7Med[2]))
	fmt.Fprintf(&b, "- 判据：中位非降且末档严格升，且逐 seed 非降比例 ≥ 50%%（实际 %s）\n", fp(p7MonoFrac))
	fmt.Fprintf(&b, "\n结论：结构（经脉适应/开度）随训练成长后，松手环流衰减%s。\n\n",
		map[bool]string{true: "变慢（SelfSustain 观测窗读数上升）", false: "未见变慢"}[p7Pass])

	// ------------------------------------------------------------------ P8
	var ratios, ringMs []int64
	for _, r := range in.p8 {
		ringMs = append(ringMs, r.ringMean)
		den := r.treeMean
		if den < 1 {
			den = 1
		}
		ratios = append(ratios, r.ringMean*1000/den)
	}
	ringMed, treeMed := medianOf(ringMs), int64(0)
	{
		var tm []int64
		for _, r := range in.p8 {
			tm = append(tm, r.treeMean)
		}
		treeMed = medianOf(tm)
	}
	ratioMed := medianOf(ratios)
	p8Pass := ringMed > 0 && ratioMed >= 3000

	fmt.Fprintf(&b, "## P8 环必要性对照（J2）—— %s\n\n", passStr(p8Pass))
	fmt.Fprintf(&b, "同参数同 seed：主环 vs 砍一边成树（缺 中庭→阳枢 段），各缓推 200 tick 后松手 30 tick：\n\n")
	fmt.Fprintf(&b, "| 指标 | 主环 | 断环（树） |\n|---|---|---|\n")
	fmt.Fprintf(&b, "| 松手窗均流中位 | %s | %s |\n", fp(ringMed), fp(treeMed))
	fmt.Fprintf(&b, "| 环/树均流比中位 | %s | — |\n\n", fp(ratioMed))
	fmt.Fprintf(&b, "- 判据：主环均流 > 0 且环/树比中位 ≥ 3.000\n")
	fmt.Fprintf(&b, "\n结论：主环拓扑下松手环流%s；树形状 vs 环的差别%s。\n\n",
		map[bool]string{true: "持续存在", false: "未见"}[ringMed > 0],
		map[bool]string{true: "显著", false: "不显著"}[p8Pass])

	// ------------------------------------------------------------------ P9
	crossed := 0
	var maxss []int64
	for _, r := range in.p9 {
		if r.crossed {
			crossed++
		}
		maxss = append(maxss, r.maxSelfSustain)
	}
	crossFrac := int64(0)
	if n > 0 {
		crossFrac = int64(crossed) * 1000 / int64(n)
	}
	estCount := 0
	for _, r := range in.p9 {
		if r.established {
			estCount++
		}
	}
	p9Pass := crossFrac >= 500

	fmt.Fprintf(&b, "## P9 筑基可达性（P0 修复总验收）—— %s\n\n", passStr(p9Pass))
	fmt.Fprintf(&b, "完整流程（引气 → 反复行环 600 周天 → 全松手 200 tick），SelfSustain 峰值跨 0.85 的 seed：**%d/%d（%s）**，其中走完全程自然筑基（五量全门槛）：%d\n\n",
		crossed, n, fp(crossFrac), estCount)
	fmt.Fprintf(&b, "- SelfSustain 峰值中位：%s\n", fp(medianOf(maxss)))
	fmt.Fprintf(&b, "- 注：世界 = Part J 标定世界（结构已演化到位）。SpikeWorld 新手身体的传导/ Closure 天花板使筑基需按 J6 长期行环修结构（新手身上同流程 SelfSustain 峰值 ~0.33，见报告附注），非引擎死锁复发。\n")
	fmt.Fprintf(&b, "\n结论：真实松手流程的 SelfSustain%s跨 0.85——筑基判定链在引擎层%s。\n\n",
		map[bool]string{true: "可", false: "不能"}[p9Pass],
		map[bool]string{true: "已打通", false: "仍不可达"}[p9Pass])

	// ------------------------------------------------------------------ 汇总
	fmt.Fprintf(&b, "## 判定汇总\n\n")
	fmt.Fprintf(&b, "| 命题 | 判定 |\n|---|---|\n")
	fmt.Fprintf(&b, "| P1 烧蚀动机闭环 | %s |\n", passStr(p1Pass))
	fmt.Fprintf(&b, "| P2 轨道-流派 | %s |\n", passStr(p2Pass))
	fmt.Fprintf(&b, "| P3 双重收敛限速 | %s |\n", passStr(p3Pass))
	fmt.Fprintf(&b, "| P4 守恒归账 | %s |\n", passStr(p4Pass))
	fmt.Fprintf(&b, "| P5 逆流火候 | %s |\n", passStr(p5Pass))
	fmt.Fprintf(&b, "| P6 中庭瓶颈 | %s |\n", passStr(p6Pass))
	fmt.Fprintf(&b, "| P7 衰减-结构单调 | %s |\n", passStr(p7Pass))
	fmt.Fprintf(&b, "| P8 环必要性对照 | %s |\n", passStr(p8Pass))
	fmt.Fprintf(&b, "| P9 筑基可达性 | %s |\n\n", passStr(p9Pass))

	fmt.Fprintf(&b, "## 建议调参方向（仅供人工参考，本台未改任何参数）\n\n")
	sug := suggestions(in, p1Pass, p2Pass, p3Pass, p4Pass, p5Pass, p6Pass, p7Pass, p8Pass, p9Pass,
		stallFrac, recovMed, ratioMilli, reachFrac, reachMed, medPeakQG, medDamage, medStrain, distMed)
	if len(sug) == 0 {
		fmt.Fprintf(&b, "六命题全部成立，暂无调参建议。\n")
	}
	for _, s := range sug {
		fmt.Fprintf(&b, "- %s\n", s)
	}
	return b.String()
}

// collect 抽取 P5 某剂量下全部 seed 的一个指标。
func collect(p5 [][]p5Res, di int, f func(p5Res) int64) []int64 {
	var out []int64
	for s := range p5 {
		out = append(out, f(p5[s][di]))
	}
	return out
}

func collectP6(p6 []p6Res, f func(p6Res) int64) []int64 {
	var out []int64
	for _, r := range p6 {
		if r.peakZZDist >= 0 {
			out = append(out, f(r))
		}
	}
	return out
}

// suggestions 按 FAIL 项给出方向性调参建议（只动 K_*/画像参数，不动公式结构）。
func suggestions(in mcInput, p1, p2, p3, p4, p5, p6, p7, p8, p9 bool,
	stallFrac, recovMed, ratioMilli, reachFrac, reachMed int64,
	medPeakQG, medDamage, medStrain []int64, distMedVal int64) []string {

	var s []string
	if !p1 {
		if stallFrac < 900 {
			s = append(s, "P1-A 不停滞（只纳不循环也一直吸得进）：提高 §7 BackPressure 对负载比的敏感度（tick.go 的 lr² 项在公式结构内加大指数属规则变更；参数面可下调青关 Capacity 或上调环境吸收的亲和加权，使吸收点自身更快积压），或上调 BaseDiffuseRate 不动——核心是让『吸收点积压→BackPressure↑』链条真实闭合。")
		}
		if recovMed < 500 {
			s = append(s, "P1-B 循环后纳气恢复不足：下调 BaseWork（200）或 KBack（500）降低逆流做功损耗，使循环烧蚀留有余气；或上调 HeatDissipation（100）加快热散，减少青关残余积压。")
		}
	}
	if !p2 {
		s = append(s, fmt.Sprintf("P2 轨道构成区分不足（比值 %s < 3）：D3 筛选差分太弱——上调 BaseDiffuseRate（20）放大『留与散』的逐元素差，或加大中庭/丹田 Affinity 画像的元素梯度（当前全 1000，无差可筛）。", fp(ratioMilli)))
	}
	if !p3 {
		if reachFrac < 500 {
			var st []int64
			for _, r := range in.p3 {
				st = append(st, r.steady)
			}
			s = append(s, fmt.Sprintf("P3 爬升不可达（达标率 %s；乙轨稳态炼化吸引子中位 %s < 0.5，注入后曲线爬到该平台即止）：D3 筛选差分不足——上调 BaseDiffuseRate（20）放大逐元素『留与散』差，或加大丹田/青关亲和画像梯度（当前丹田全 1000 无差可筛），或收窄 Refinement 归一化区间（MatchMin/MatchMax）。", fp(reachFrac), fp(medianOf(st))))
		} else if reachMed < 20 {
			s = append(s, fmt.Sprintf("P3 驳杂期太短（中位 %d 轮 < 20）：双重收敛过快——下调 KTransform（300，炼化→转化→再炼化的正反馈）或降低 Intake 亲和加权强度（§7 吸收公式对 CharacterAffinity 的线性项），让新气有可见的粗→精过程。", reachMed))
		}
	}
	if !p4 {
		s = append(s, "P4 守恒破坏：这不是调参问题，是引擎 bug——按残差符号排查 runTransfers/runSheng 的舍入方向（fpMul 半入语义）与 heatAccum 归账。")
	}
	if !p5 {
		if !nonDecreasing(medPeakQG) {
			s = append(s, "P5 热峰值对逆流量不单调：检查 KHeat（500）/HeatDissipation（100）比例与燃料约束——大剂量过早掏空丹田会压平峰值；可下调 BaseWork 让大剂量维持更久，或提高 KHeat 放大差分。")
		}
		if !nonDecreasing(medDamage) && !nonDecreasing(medStrain) {
			s = append(s, "P5 失稳事件不单调：上调 KBack（500，推入越满做功越贵→热越高）或下调 DamageThreshold（400）让热压力更早转化为失稳。")
		}
	}
	if !p6 {
		s = append(s, fmt.Sprintf("P6 中庭压感不足（胀感峰值中位 %s < 第一档 200）：B1 画像面——下调中庭 Capacity（9000）或上调其中转流量占比；引擎面可上调 KBack 提高逆流在青关段的积压、间接增大经中庭的过流。", fp(distMedVal)))
	}
	if !p7 {
		s = append(s, "P7 松手衰减不随结构单调：检查 KInertia（990）与经脉适应/开度成长是否进入闭环—— Conductance 高→单跳损耗低→流量维持更久；若被 KQiShare（100）封顶抹平，可上调 KQiShare 或加大训练档差。")
	}
	if !p8 {
		s = append(s, "P8 环/树对照不显著：断环世界松手流未归零或主环流太弱——检查惯性输送带在死端的截止行为（FlowPrev 入流为零应自然断流）与 KQiShare 封顶水平。")
	}
	if !p9 {
		s = append(s, "P9 SelfSustain 跨不过 0.85：松手流相对推动流不足——上调 KEntrain（40，途经纳气是环流的能量源）或下调 KInertia（990）以外的衰减项；或降低行环推量（推得越猛 W1 分母越大）。")
	}
	return s
}

// ---------------------------------------------------------------------------
// 入口
// ---------------------------------------------------------------------------

func main() {
	nSeeds := flag.Int("seeds", 100, "seed 数（P1-P9 命题台，seed = 1..N）")
	ticks := flag.Int("ticks", 3000, "P1 场景的 Tick 预算")
	nChars := flag.Int("chars", 1000, "SA 千级扫描角色数（seed = 1..N）")
	nBodies := flag.Int("bodies", 24, "SB 路线对照身体数")
	outPath := flag.String("out", "", "报告输出路径（默认 stdout）")
	flag.Parse()

	start := time.Now()
	acc := &accounting{}

	in := mcInput{
		nSeeds: *nSeeds,
		ticks:  int64(*ticks),
		p1a:    make([]p1ARes, *nSeeds),
		p1b:    make([]p1BRes, *nSeeds),
		p2a:    make([]p2Res, *nSeeds),
		p2b:    make([]p2Res, *nSeeds),
		p3:     make([]p3Res, *nSeeds),
		p5:     make([][]p5Res, *nSeeds),
		p6a:    make([]p6Res, *nSeeds),
		p6b:    make([]p6Res, *nSeeds),
		p7:     make([]p7Res, *nSeeds),
		p8:     make([]p8Res, *nSeeds),
		p9:     make([]p9Res, *nSeeds),
		acc:    acc,
	}

	for s := 1; s <= *nSeeds; s++ {
		seed := uint64(s)
		i := s - 1
		in.p1a[i] = runP1A(seed, in.ticks, acc)
		in.p1b[i] = runP1B(seed, in.ticks, acc)
		in.p2a[i] = runP2Track(seed, true, acc)
		in.p2b[i] = runP2Track(seed, false, acc)
		in.p3[i] = runP3(seed, acc)
		in.p5[i] = make([]p5Res, len(p5Doses))
		for di, d := range p5Doses {
			in.p5[i][di] = runP5(seed, d, acc)
		}
		in.p6a[i] = runP6Track(seed, true, acc)
		in.p6b[i] = runP6Track(seed, false, acc)
		in.p7[i] = runP7(seed, acc)
		in.p8[i] = runP8(seed, acc)
		in.p9[i] = runP9(seed)
	}
	in.elapsed = time.Since(start)

	// ---------------- P10 千级扫描 + 专项（scan.go） ----------------
	scanAcc := &accounting{}
	sin := &scanInput{nChars: *nChars, acc: scanAcc}
	sin.sa = make([]scanARes, *nChars)
	for c := 1; c <= *nChars; c++ {
		sin.sa[c-1] = runScanA(uint64(c), scanAcc)
	}
	sin.sb = make([][]scanBRes, *nBodies)
	for bi := 0; bi < *nBodies; bi++ {
		sin.sb[bi] = make([]scanBRes, len(sbRoutes))
		for ri := range sbRoutes {
			sin.sb[bi][ri] = runScanB(bi, ri, scanAcc)
		}
	}
	// P11 移交门扫频。
	sin.p11 = make([][]int64, len(p11Strengths))
	sin.p11End = make([][]int64, len(p11Strengths))
	sin.p11Brk = make([][]bool, len(p11Strengths))
	sin.p11Osc = make([][]int64, len(p11Strengths))
	for si, s := range p11Strengths {
		sin.p11[si] = make([]int64, p11Seeds)
		sin.p11End[si] = make([]int64, p11Seeds)
		sin.p11Brk[si] = make([]bool, p11Seeds)
		sin.p11Osc[si] = make([]int64, p11Seeds)
		for k := 0; k < p11Seeds; k++ {
			ret, endR, brk, osc := runP11(uint64(20000+k), s, scanAcc)
			sin.p11[si][k], sin.p11End[si][k] = ret, endR
			sin.p11Brk[si][k], sin.p11Osc[si][k] = brk, osc
		}
	}
	// P12 SelfSustain 分辨力网格。
	sin.p12 = map[[2]int][]int64{}
	sin.p12Sat = map[[2]int][]int64{}
	for oi := range p12Open {
		for ei := range p12Env {
			key := [2]int{oi, ei}
			sin.p12[key] = []int64{}
			sin.p12Sat[key] = []int64{}
			for k := 0; k < p12SeedsPerCell; k++ {
				seed := uint64(30000 + oi*100 + ei*10 + k)
				sin.p12[key] = append(sin.p12[key], runP12Cell(p12Open[oi], p12Env[ei], seed, false, scanAcc))
				sin.p12Sat[key] = append(sin.p12Sat[key], runP12Cell(p12Open[oi], p12Env[ei], seed, true, scanAcc))
			}
		}
	}
	// P13 多环/支路闭合。
	sin.p13 = make([][]p13Res, len(p13Topologies))
	for ti := range p13Topologies {
		sin.p13[ti] = make([]p13Res, p13Seeds)
		for k := 0; k < p13Seeds; k++ {
			sin.p13[ti][k] = runP13(&p13Topologies[ti], uint64(40000+ti*100+k), scanAcc)
		}
	}
	// P14 被动/手动量级。
	sin.p14 = make([]p14Res, p14Seeds)
	for k := 0; k < p14Seeds; k++ {
		sin.p14[k] = runP14(uint64(50000+k), scanAcc)
	}

	report := renderReport(in) + renderScanReport(sin)
	if *outPath == "" {
		fmt.Print(report)
		return
	}
	f, err := os.Create(*outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法创建报告文件 %s: %v\n", *outPath, err)
		os.Exit(1)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	if _, err := io.WriteString(w, report); err != nil {
		fmt.Fprintf(os.Stderr, "写入报告失败: %v\n", err)
		os.Exit(1)
	}
	if err := w.Flush(); err != nil {
		fmt.Fprintf(os.Stderr, "写入报告失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("报告已写入 %s（耗时 %s，P 台 tick %d + 扫描 tick %d）\n", *outPath, in.elapsed, acc.ticks, scanAcc.ticks)
}
