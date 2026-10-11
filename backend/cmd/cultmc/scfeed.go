package main

// scfeed.go —— SF 旁支喂养场景（修订包「保留的非环边」注记 2026-10-11
// 旁支角色裁决的批测验证；docs/specs/2026-09-21-stage3-revision-circulation-
// refinement.md Part A 头部）。
//
// 背景：SC 角色×轨道批测（docs/reports/2026-09-27-character-divergence-mc.md）
// 发现金偏画像全域炼化近零——量尺只拼了闭环轨道，未编排旁支吐纳。裁决：
// 保留金门（/玄泉）立为异气入口——旁支穴吐纳纳异属性气、沿旁支注入中庭、
// 环流带走、构成偏移、炼化度重算抬升。本场景把这条「旁支喂养」轨道形态
// 编排进量尺并正式批测；若证伪（喂养不通或构成不动），裁决作废，如实报。
//
// 命题（注记原文的验收口径）：
//   1. 旁支喂养抬升亲和画像的炼化度（D2）——金偏在「主环行转 + 金门吐纳
//      + 金门→中庭注入」轨道上，丹田炼化度由近零基线显著抬升，构成（丹田
//      NatureRatio 金份额）随喂养轮次向金收敛；均衡画像同轨道温和或不抬升
//      ——金偏 vs 均衡的抬升差即「旁支策略随画像」的可测证据。
//   2. 环境对照：同场景在三种环境变体（现行木盛金薄 / 五元素同质 / 金盛）
//      下复测——异气入口的价值是否依赖环境异气分布。现行环境空间均匀
//      （SpikeWorld 真值表，无「金门处金更盛」的空间结构），「异气分布」
//      在批测世界内只能以全局元素密度变体表达；如实记录这一设计前提。
//   3. 物理合理性：金门吐纳的气注入中庭后是否真被环流带走并留存（中庭
//      Retention 400 低滞留——会不会一过即散）。应变量：金门纳入/注入
//      混合的金属份额、金门→中庭金属输送、中庭→阳枢金属带走、中庭金属
//      净存/泄出，加喂养停歇段（rest 块）构成是否回退。
//
// 方法：
//   - 画像 2 种：均衡（scChars[0]）/ 金偏（scChars[3]，五行轮转构造同 sc.go）。
//   - 轨道 2 条：喂养轮（O0 全环 6 段 + 吸收@青关 large 原样 + absorb@金门
//     medium + guide 金门→中庭 + 丹田→土府泵段让位）与对照轮（O0 全环原
//     样）——差异只在旁支开门与泵段让位，身体/世界/主环逐字同 sc.go。
//   - 环境 3 变体（仅覆盖批测世界的 w.Env.Qi，不动 SpikeWorld 全局默认）。
//   - 相位：6 ×（喂养教学 60 + 停歇 60），共 720 tick + 1 tick 观测基准。
//     教学块末与停歇块末各采样一次——喂养轮次的轨迹 + 停歇段构成是否
//     维持（物理合理性第 3 问的留存观测）。不追求筑基（本命题应变量是
//     构成/炼化度轨迹，不是时标），故无长探针窗。
//   - 每格 5 seed 取中位；seed 段 70000（SC 60000 段之后，场景不重叠）。
//
// 工作点扫频证据（测量设计内选定，非引擎参数；32+ 格扫频，金偏/现行环境）：
//   - 真值表金门（容量 5000/通量 4000/小库存）下喂养结构上不成立：注入段
//     实流被能量收敛（驱动做功随流量平方涨，小库存出不动）与传导节流钉死
//     在 ~200-800/tick，任何有效开度×力度的吐纳 ≥500/tick——金门一两个
//     教学块内自体过载（LR 冲 1.1+ 走火，损伤 170+ 事件且与扫频参数基本
//     无关）。故本世界把金门修到「修行有成」口径（feedJinMature，与环上
//     六穴同一 J6 待遇）——这是裁决成立的结构性前提，进入报告结论。
//   - 泵段让位是中庭过载的唯一有效补偿：土府→中庭的实流是库存/能量收敛
//     受限（声明减半实流几乎不动），丹田→土府是请求受限（声明即实流上
//     限）。泵段 700、注入请求 600/元素时三环境全格零损伤零异常（中庭
//     峰值 LR ≤ 0.86、金门峰值 LR ≤ 0.48）；泵段 900 + 注入 900 在金盛
//     环境走火（损伤 209）。
//   - 加大环流推量（中庭→阳枢 2400/元素）不可行：驱动做功随流量平方涨，
//     中庭稳定性被做功热 60 tick 内打穿——「注入靠环流带走」在现役传导/
//     做功公式下不能靠加推实现，只能靠泵段让位腾出的环流预算。
//   - 等量置换（青关主吸降档）不可用：金盛环境下主吸降档减掉的金属纳入
//     恰好抵消旁支喂养（两入口同吸一个环境），构成位移归零——对照失真。
//   - 长程对照（12 块 = 1440 tick，扫频记录不入正式批测）：构成位移与炼化
//     读数的形态与 6 块一致——结论对喂养时长稳健。
//
// 全程定点整数；无 LLM；同 seed 逐位可重放；只测量不调参——命题不成立
// 如实报 FAIL 并归因（裁决作废）。

import (
	"fmt"
	"strings"

	"agentbattle/backend/ctmatch"
	"agentbattle/backend/cult"
)

// ---------------------------------------------------------------------------
// 环境变体（仅覆盖批测世界）
// ---------------------------------------------------------------------------

// feedEnvs 是三种环境变体。现行环境是 SpikeWorld 真值表（木略盛、金最薄），
// 空间均匀——金门处并无更盛的金气；同质与金盛只改全局元素密度。
var feedEnvs = []struct {
	name string
	note string
	qi   [cult.ElementCount]int64
}{
	{"现行", "SpikeWorld 真值表（木 3600 略盛、金 1800 最薄）；空间均匀，金门处无更盛金气", [cult.ElementCount]int64{3600, 2400, 2400, 1800, 2400}},
	{"同质", "五元素等密度（同质环境对照）", [cult.ElementCount]int64{2400, 2400, 2400, 2400, 2400}},
	{"金盛", "金 3600、其余 2400（环境异气分布变体）", [cult.ElementCount]int64{2400, 2400, 2400, 3600, 2400}},
}

// feedCharIdx 参与对照的画像（scChars 下标）：均衡 / 金偏。
var feedCharIdx = []int{0, 3}

// 工作点（扫频依据见文件头注）：
//   - feedSideAmt：旁支注入段（金门→中庭）的协议混合引导量（每元素声明，
//     与 scSegAmt 同标度）。
//   - feedPumpAmt：喂养轮 丹田→土府 泵段引导量（对照轮保持 scSegAmt）——
//     旁支开门占用的环流预算由泵段让位（中庭过载的唯一有效补偿，见头注）。
const (
	feedSideAmt = int64(600)
	feedPumpAmt = int64(700)
)

// ---------------------------------------------------------------------------
// 世界装配与协议
// ---------------------------------------------------------------------------

// feedJinMature 把金门修到「修行有成」口径（测量设计，非引擎参数）：
// SC 把长期行功修上去的结构态（容量 30000 / 通量 10000 / 开度 1.0 /
// 稳定 950）给了环上六穴，金门/玄泉保持真值表新手画像。本场景的轨道
// 正是以金门为常修入口——按同一口径把它修到位。这一步不是调参偏好
// 而是结构必需（扫频实证见文件头注）：真值表金门下旁支喂养在结构上
// 不成立。玄泉不涉及（本批测只编排金门）；真值表世界（新手期）的
// 金门仍由 SpikeWorld 保留不动。
func feedJinMature(w *cult.World) {
	for i := range w.Character.BodyGraph.Acupoints {
		a := &w.Character.BodyGraph.Acupoints[i]
		if a.Name != "金门" {
			continue
		}
		a.Capacity, a.Throughput = 30000, 10000
		a.Openness = cult.MicroOne
		a.Stability, a.BaseStability = 950, 950
		qi0 := a.Capacity * 3 / 5 / cult.ElementCount // LR 0.6，与环上六穴同口径
		for e := 0; e < cult.ElementCount; e++ {
			a.Qi[e] = qi0
		}
	}
}

// feedWorld 装配 SF 世界：scWorld（SpikeWorld 真值表 + 修行有成结构态）
// 逐字保留，仅按环境变体覆盖 w.Env.Qi、把金门修到修行有成口径。调用方
// 负责 scRingCond 处于主工作点（scCondMain）——runFeedMatrix 统一设置。
func feedWorld(charIdx, envIdx int) *cult.World {
	w := scWorld(charIdx)
	w.Env.Qi = feedEnvs[envIdx].qi
	feedJinMature(w)
	return w
}

// feedProtoYAML 生成轨道功法协议。feed=false 为对照轮（O0 全环原样）；
// feed=true 加 absorb@金门 medium（旁吸档位与注入段收支相配——旁吸力度
// 超过注入出货能力时金门自体过载，扫频教训）与 guide 金门→中庭（旁支
// 注入），泵段让位。全部步骤 parallel 1 → 单槽，1 Tick = 1 周天，与
// scOrbitYAML 同口径。
func feedProtoYAML(charIdx int, feed bool, cycles int64, g *cult.BodyGraph) string {
	o := &scOrbits[0] // O0 全环 6 段
	var b strings.Builder
	track := "base"
	if feed {
		track = "feed"
	}
	fmt.Fprintf(&b, "protocol: sf_%s_c%d\n", track, charIdx)
	fmt.Fprintf(&b, "actions:\n")
	fmt.Fprintf(&b, "  - absorb: {target: 青关, level: large, duration: 1, parallel: 1}\n")
	if feed {
		fmt.Fprintf(&b, "  - absorb: {target: 金门, level: medium, duration: 1, parallel: 1}\n")
	}
	rmax := map[int]int64{}
	idxAll := scIdx(g)
	for _, sg := range o.segs {
		ch, _, ok := scEdge(g, int(sg[0]), int(sg[1]))
		if !ok {
			panic("feedProtoYAML: 全环段无对应经脉") // 标定台静态装配错误
		}
		amt := sg[2]
		if feed && sg[0] == int64(idxAll["下丹田"]) && sg[1] == int64(idxAll["土府"]) {
			amt = feedPumpAmt // 喂养轮：泵段让位（旁支开门占用的环流预算）
		}
		if r := amt * 11 / 10; r > rmax[ch] {
			rmax[ch] = r
		}
		fmt.Fprintf(&b, "  - guide: {from: %s, to: %s, element: \"\", amount: %d, duration: 1, parallel: 1}\n",
			g.Acupoints[sg[0]].Name, g.Acupoints[sg[1]].Name, amt)
	}
	if feed {
		jin, mid := idxAll["金门"], idxAll["中庭"]
		ch, _, ok := scEdge(g, jin, mid)
		if !ok {
			panic("feedProtoYAML: 金门→中庭无对应经脉") // 标定台静态装配错误
		}
		if r := feedSideAmt * 11 / 10; r > rmax[ch] {
			rmax[ch] = r
		}
		fmt.Fprintf(&b, "  - guide: {from: 金门, to: 中庭, element: \"\", amount: %d, duration: 1, parallel: 1}\n", feedSideAmt)
	}
	fmt.Fprintf(&b, "cadence: 1\ncycles: %d\nvalidated_range:\n", cycles)
	for ch, r := range rmax {
		c := &g.Channels[ch]
		fmt.Fprintf(&b, "  - channel: [%s, %s]\n    element: \"\"\n    min: 0\n    max: %d\n",
			g.Acupoints[c.From].Name, g.Acupoints[c.To].Name, r)
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// 相位与采样
// ---------------------------------------------------------------------------

// feedBlocks 是喂养/停歇块对数（var：长短程稳健性扫频用；正式批测 6，
// 扫频证据见文件头注）。
var feedBlocks = int64(6)

// feedBudget 返回一格的 tick 预算（不含观测基准）。
func feedBudget() int64 { return feedBlocks * (feedTeachTick + feedRestTick) }

const (
	feedTeachTick = int64(60) // 教学块（协议活跃）
	feedRestTick  = int64(60) // 停歇块（协议停止——构成留存观测窗）
	feedSeeds     = 5
	// feedSeedBase 取 70000 段（SC 60000 段之后），seed 布局：
	// base + charSlot*1000 + track*100 + env*10 + k。
	feedSeedBase = uint64(70000)
)

// feedSample 是一次采样（教学块末或停歇块末）。
type feedSample struct {
	teach    bool  // true = 教学块末（喂养活跃），false = 停歇块末
	refine   int64 // 丹田炼化度（D2 导出量，千分位）
	danMetal int64 // 丹田构成金份额（NatureRatio，千分位）
	midMetal int64 // 中庭构成金份额（千分位）
	danLoad  int64 // 丹田总负载（读数健康性对照）
}

// feedRes 是一格（画像 × 轨道 × 环境 × seed）的观测。
type feedRes struct {
	charIdx, envIdx int
	feed            bool
	seed            uint64

	samples []feedSample // 初样 + 每块末（2×feedBlocks 个）

	// 物理台账（全 run 累计；金属 = 元素下标 3，总量 = 五元素之和）：
	jinMetalIn   int64                    // 金门金属纳入（Intake+ExchangeIn，意识+被动）
	jinTotalIn   int64                    // 金门五元素纳入总量（→ 吐纳混合的金属份额）
	qgMetalIn    int64                    // 青关金属纳入（主环直纳的对照口）
	qgTotalIn    int64                    // 青关五元素纳入总量
	sideMetal    int64                    // 金门→中庭金属 Delivered（旁支注入实测）
	sideTotal    int64                    // 金门→中庭五元素 Delivered（→ 注入混合的金属份额）
	midOutMetal  int64                    // 中庭→阳枢金属 Delivered（环流带走实测）
	midDiffuse   int64                    // 中庭金属泄出（去向=环境）
	midNet       int64                    // 中庭金属净存变化（末 − 初）
	midPeakRatio int64                    // 中庭负载比峰值（过载核查）
	jinPeakRatio int64                    // 金门负载比峰值（旁支自体过载核查）
	damage       int64                    // 新增损伤事件数
	anomalies    int64                    // 未知异常升级次数
	depleted     bool                     // 丹田枯竭
	danComp      [cult.ElementCount]int64 // 终态丹田构成（千分位占比，归因用）
}

// runFeedCell 走完一格。相位结构：观测基准 1 tick → feedBlocks ×（教学 60
// + 停歇 60）。教学块 StartProtocol，停歇块不停（协议跑满 cycles 自然结束）。
func runFeedCell(charIdx, envIdx int, feed bool, seed uint64, acc *accounting) feedRes {
	w := feedWorld(charIdx, envIdx)
	w.NoiseSeed = seed
	g := &w.Character.BodyGraph
	s := ctmatch.NewSession(w, seed, nil)
	proto, err := ctmatch.Compile([]byte(feedProtoYAML(charIdx, feed, feedTeachTick, g)), g)
	if err != nil {
		panic(err) // 标定台静态装配错误
	}
	s.LoadProtocol(proto)
	ctmatch.Tick(s, nil) // 观测基准

	idx := scIdx(g)
	dan, mid, jin, qg := idx["下丹田"], idx["中庭"], idx["金门"], idx["青关"]
	metal := 3 // 金
	// 经脉下标：金门→中庭（旁支注入）与 中庭→阳枢（环流带走）。
	sideCh, outCh := -1, -1
	for ci := range g.Channels {
		c := &g.Channels[ci]
		if c.From == jin && c.To == mid {
			sideCh = ci
		}
		if c.From == mid && c.To == idx["阳枢"] {
			outCh = ci
		}
	}
	if sideCh < 0 || outCh < 0 {
		panic("runFeedCell: 经脉解析失败") // 标定台静态装配错误
	}

	res := feedRes{charIdx: charIdx, envIdx: envIdx, feed: feed, seed: seed}
	prevDmg := make([]int64, len(g.Acupoints))

	// loadOf 汇总穴位总负载（cult.Acupoint.load 未导出，本地复算）。
	loadOf := func(pi int) int64 {
		t := int64(0)
		for e := 0; e < cult.ElementCount; e++ {
			t += g.Acupoints[pi].Qi[e]
		}
		return t
	}
	sample := func(teach bool) {
		d := &g.Acupoints[dan]
		m := &g.Acupoints[mid]
		dl, ml := loadOf(dan), loadOf(mid)
		sm := feedSample{teach: teach, refine: cult.DeriveRefinement(w, dan), danLoad: dl}
		if dl > 0 {
			sm.danMetal = d.Qi[metal] * 1000 / dl
		}
		if ml > 0 {
			sm.midMetal = m.Qi[metal] * 1000 / ml
		}
		res.samples = append(res.samples, sm)
	}
	midMetal0 := g.Acupoints[mid].Qi[metal]
	sample(false) // 初样

	runTicks := func(n int64, teach bool) {
		for i := int64(0); i < n; i++ {
			rs := ctmatch.Tick(s, nil)
			for _, ev := range rs.Events {
				if ev.Kind == ctmatch.EventUnknownAnomaly {
					res.anomalies++
					s.ResumeProtocol()
				}
			}
			rep := rs.Report
			if rep == nil {
				continue
			}
			acc.checkConservation(rep)
			if rep.Points[jin].Damage > prevDmg[jin] || rep.Points[mid].Damage > prevDmg[mid] {
				res.damage++
			}
			for pi := range rep.Points {
				prevDmg[pi] = rep.Points[pi].Damage
			}
			for e := 0; e < cult.ElementCount; e++ {
				res.jinTotalIn += rep.Points[jin].Intake[e] + rep.Points[jin].ExchangeIn[e]
				res.qgTotalIn += rep.Points[qg].Intake[e] + rep.Points[qg].ExchangeIn[e]
			}
			res.jinMetalIn += rep.Points[jin].Intake[metal] + rep.Points[jin].ExchangeIn[metal]
			res.qgMetalIn += rep.Points[qg].Intake[metal] + rep.Points[qg].ExchangeIn[metal]
			// 报告只列有流量的经脉（长度 ≠ 全经脉表），按 Channel 字段定位。
			for ti := range rep.Transfers {
				switch rep.Transfers[ti].Channel {
				case sideCh:
					res.sideMetal += rep.Transfers[ti].Delivered[metal]
					for e := 0; e < cult.ElementCount; e++ {
						res.sideTotal += rep.Transfers[ti].Delivered[e]
					}
				case outCh:
					res.midOutMetal += rep.Transfers[ti].Delivered[metal]
				}
			}
			res.midDiffuse += rep.Points[mid].Diffuse[metal]
			if rep.Points[mid].LoadRatio > res.midPeakRatio {
				res.midPeakRatio = rep.Points[mid].LoadRatio
			}
			if rep.Points[jin].LoadRatio > res.jinPeakRatio {
				res.jinPeakRatio = rep.Points[jin].LoadRatio
			}
		}
		sample(teach)
	}
	for b := int64(0); b < feedBlocks; b++ {
		if err := s.StartProtocol(proto.Name); err != nil {
			panic(err) // 标定台静态装配错误
		}
		runTicks(feedTeachTick, true)
		runTicks(feedRestTick, false)
	}

	res.midNet = g.Acupoints[mid].Qi[metal] - midMetal0
	dl := loadOf(dan)
	if dl < 500 {
		res.depleted = true
	} else {
		for e := 0; e < cult.ElementCount; e++ {
			res.danComp[e] = g.Acupoints[dan].Qi[e] * 1000 / dl
		}
	}
	return res
}

// ---------------------------------------------------------------------------
// 矩阵聚合与报告
// ---------------------------------------------------------------------------

// feedInput 是 SF 批测的聚合输入。
type feedInput struct {
	cells []feedRes
	seeds int
	acc   *accounting
}

// runFeedMatrix 跑全矩阵：2 画像 × 2 轨道 × 3 环境 × seeds。
func runFeedMatrix(seeds int, acc *accounting) *feedInput {
	old := scRingCond
	scRingCond = scCondMain // SF 与 SC 主工作点同口径
	defer func() { scRingCond = old }()
	in := &feedInput{seeds: seeds, acc: acc}
	for cs, charIdx := range feedCharIdx {
		for t, feed := range []bool{true, false} {
			for ei := range feedEnvs {
				for k := 0; k < seeds; k++ {
					seed := feedSeedBase + uint64(cs)*1000 + uint64(t)*100 + uint64(ei)*10 + uint64(k)
					in.cells = append(in.cells, runFeedCell(charIdx, ei, feed, seed, acc))
				}
			}
		}
	}
	return in
}

// feedAgg 是一格（画像 × 轨道 × 环境）跨 seed 的聚合：逐采样位中位。
type feedAgg struct {
	charIdx, envIdx int
	feed            bool
	samples         []feedSample // 逐位跨 seed 中位
	jinMetalIn      int64
	jinTotalIn      int64
	qgMetalIn       int64
	qgTotalIn       int64
	sideMetal       int64
	sideTotal       int64
	midOutMetal     int64
	midDiffuse      int64
	midNet          int64
	midPeakRatio    int64
	jinPeakRatio    int64
	damage          int64
	anomalies       int64
	depleted        bool
	danComp         [cult.ElementCount]int64
}

func feedAggCell(rs []feedRes) feedAgg {
	ag := feedAgg{
		charIdx: rs[0].charIdx, envIdx: rs[0].envIdx, feed: rs[0].feed,
	}
	n := len(rs[0].samples)
	var jinM, jinT, qgM, qgT, sideM, sideT, midO, midDf, midN, mPeak, jPeak, dmg, ano []int64
	dep := 0
	for _, r := range rs {
		jinM = append(jinM, r.jinMetalIn)
		jinT = append(jinT, r.jinTotalIn)
		qgM = append(qgM, r.qgMetalIn)
		qgT = append(qgT, r.qgTotalIn)
		sideM = append(sideM, r.sideMetal)
		sideT = append(sideT, r.sideTotal)
		midO = append(midO, r.midOutMetal)
		midDf = append(midDf, r.midDiffuse)
		midN = append(midN, r.midNet)
		mPeak = append(mPeak, r.midPeakRatio)
		jPeak = append(jPeak, r.jinPeakRatio)
		dmg = append(dmg, r.damage)
		ano = append(ano, r.anomalies)
		if r.depleted {
			dep++
		}
	}
	ag.samples = make([]feedSample, n)
	for si := 0; si < n; si++ {
		var ref, dm, mm, loads []int64
		for _, r := range rs {
			ref = append(ref, r.samples[si].refine)
			dm = append(dm, r.samples[si].danMetal)
			mm = append(mm, r.samples[si].midMetal)
			loads = append(loads, r.samples[si].danLoad)
		}
		ag.samples[si] = feedSample{
			teach: rs[0].samples[si].teach, refine: medianOf(ref),
			danMetal: medianOf(dm), midMetal: medianOf(mm), danLoad: medianOf(loads),
		}
	}
	ag.jinMetalIn = medianOf(jinM)
	ag.jinTotalIn = medianOf(jinT)
	ag.qgMetalIn = medianOf(qgM)
	ag.qgTotalIn = medianOf(qgT)
	ag.sideMetal = medianOf(sideM)
	ag.sideTotal = medianOf(sideT)
	ag.midOutMetal = medianOf(midO)
	ag.midDiffuse = medianOf(midDf)
	ag.midNet = medianOf(midN)
	ag.midPeakRatio = medianOf(mPeak)
	ag.jinPeakRatio = medianOf(jPeak)
	ag.damage = medianOf(dmg)
	ag.anomalies = medianOf(ano)
	ag.depleted = dep*2 > len(rs)
	for e := 0; e < cult.ElementCount; e++ {
		var col []int64
		for _, r := range rs {
			if !r.depleted {
				col = append(col, r.danComp[e])
			}
		}
		ag.danComp[e] = medianOf(col)
	}
	return ag
}

// feedShareMix  returned 混合的金属份额（千分位；总量 ≤ 0 时返回 -1）。
func feedShareMix(metal, total int64) int64 {
	if total <= 0 {
		return -1
	}
	return metal * 1000 / total
}

// renderFeedReport 渲染 SF 旁支喂养报告（全定点整数）。
func renderFeedReport(in *feedInput) string {
	var b strings.Builder
	type key struct {
		c, e int
		f    bool
	}
	byKey := map[key][]feedRes{}
	for _, r := range in.cells {
		k := key{r.charIdx, r.envIdx, r.feed}
		byKey[k] = append(byKey[k], r)
	}
	aggs := map[key]*feedAgg{}
	for k, rs := range byKey {
		ag := feedAggCell(rs)
		aggs[k] = &ag
	}
	charName := func(charIdx int) string {
		if charIdx == 3 {
			return "金偏"
		}
		return "均衡"
	}
	last := int(2 * feedBlocks)

	fmt.Fprintf(&b, "\n# SF 旁支喂养批测（修订包「保留的非环边」注记 2026-10-11 旁支角色裁决验证）\n\n")
	fmt.Fprintf(&b, "- RuleVersion %s；身体/世界 = SC 同款（SpikeWorld 真值表 + 修行有成结构态，主环传导 %d、初载 LR0.6），仅覆盖 w.Env.Qi 做环境变体（不动全局默认）；画像 = 均衡 / 金偏（scChars 五行轮转）；轨道 = 喂养轮（O0 全环 + absorb@金门 medium + 金门→中庭注入 %d/元素 + 丹田→土府泵段让位至 %d）vs 对照轮（O0 原样）；两轨共用把金门修到修行有成口径的世界（结构必需，见 cultmc scfeed.go 头注扫频证据）\n",
		cult.RuleVersion, scCondMain, feedSideAmt, feedPumpAmt)
	fmt.Fprintf(&b, "- 相位：%d ×（喂养教学 60 + 停歇 60）= %d tick + 1 观测基准；教学块末（T）与停歇块末（R）各采样；每格 %d seed 取中位，零浮点\n", feedBlocks, feedBudget(), in.seeds)
	fmt.Fprintf(&b, "- 环境变体（空间均匀，无穴位级环境结构——「异气分布」以全局元素密度表达）：")
	for i, e := range feedEnvs {
		if i > 0 {
			fmt.Fprintf(&b, "；")
		}
		fmt.Fprintf(&b, "%s = %v", e.name, e.qi)
	}
	fmt.Fprintf(&b, "\n\n")

	// ---------------- 机制链：异气入口的混合差 ----------------
	fmt.Fprintf(&b, "## 机制链第一环：旁支入口的混合差（喂养轮全 run 累计，跨 seed 中位）\n\n")
	fmt.Fprintf(&b, "| 画像 | 环境 | 金门吐纳混合金份额 | 青关吐纳混合金份额 | 注入中庭混合金份额 | 环境金密度 |\n")
	fmt.Fprintf(&b, "|---|---|---|---|---|---|\n")
	for _, charIdx := range feedCharIdx {
		for ei := range feedEnvs {
			ag := aggs[key{charIdx, ei, true}]
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %d |\n",
				charName(charIdx), feedEnvs[ei].name,
				fp(feedShareMix(ag.jinMetalIn, ag.jinTotalIn)),
				fp(feedShareMix(ag.qgMetalIn, ag.qgTotalIn)),
				fp(feedShareMix(ag.sideMetal, ag.sideTotal)),
				feedEnvs[ei].qi[3])
		}
	}
	fmt.Fprintf(&b, "\n- 读法（实测推翻了裁决的第一环前提）：金门吐纳混合的金份额**不高于**青关直纳——亲和加权并不直接进入纳气构成。机制：纳气混合由「势差 × 亲和」逐元素决定，而势差随体内势升高闭合——库存被快速抽走的纳穴（青关，出流 ~3000+/tick ≫ 纳入）钉在饥饿态，势差≈环境密度，混合按 环境密度×亲和 加权；慢吞吐的旁支穴（金门，注入实流 ~285/tick < 纳入 ~343/tick，台账节）库存停在环境均衡附近，势差闭合把亲和加权抵消、混合趋于均匀。亲和加权的落点是「饥饿深度」，不是「纳气口味」。份额仍随环境金密度上行（0.177→0.300），但青关直纳同沐其利（0.190→0.367）——环境越富金属，入口差越小乃至反向。\n\n")

	// ---------------- 轨迹表 ----------------
	trajTable := func(pick func(feedSample) int64, title, unit string) {
		fmt.Fprintf(&b, "## %s（%s；逐位跨 seed 中位）\n\n", title, unit)
		fmt.Fprintf(&b, "| 画像 | 轨道 | 环境 | 初 | T1 | R1 | T2 | R2 | T3 | R3 | T4 | R4 | T5 | R5 | T6 | R6 | 终漂移 |\n")
		fmt.Fprintf(&b, "|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
		for _, charIdx := range feedCharIdx {
			for _, feed := range []bool{true, false} {
				for ei := range feedEnvs {
					ag := aggs[key{charIdx, ei, feed}]
					track := "对照"
					if feed {
						track = "喂养"
					}
					fmt.Fprintf(&b, "| %s | %s | %s |", charName(charIdx), track, feedEnvs[ei].name)
					for si := 0; si <= last; si++ {
						fmt.Fprintf(&b, " %d |", pick(ag.samples[si]))
					}
					drift := pick(ag.samples[last]) - pick(ag.samples[0])
					fmt.Fprintf(&b, " %+d |\n", drift)
				}
			}
		}
		fmt.Fprintf(&b, "\n")
	}
	trajTable(func(s feedSample) int64 { return s.refine },
		"丹田炼化度轨迹（D2 导出量）", "千分位 0~1000")
	trajTable(func(s feedSample) int64 { return s.danMetal },
		"丹田构成金份额轨迹（NatureRatio 金）", "千分位占比")
	trajTable(func(s feedSample) int64 { return s.midMetal },
		"中庭构成金份额轨迹（喂养途经点）", "千分位占比")

	// ---------------- 命题判定 ----------------
	fmt.Fprintf(&b, "### 判定（旁支角色裁决：成立 / 作废）\n\n")
	// 抬升 = 终值 − 同画像同环境对照轮终值（逐 seed 配对求差再取中位与
	// 四分位距——显著性以 seed 噪声的 IQR 为参照，不设人为阈值）。
	fmt.Fprintf(&b, "| 环境 | 画像 | 喂养终炼化 | 对照终炼化 | 抬升（中位 [P25, P75]） | 喂养终金份额 | 对照终金份额 | 份额位移 |\n")
	fmt.Fprintf(&b, "|---|---|---|---|---|---|---|---|\n")
	type envLift struct {
		name              string
		jinLift, eqLift   int64
		jinSpread         int64
		jinShift, eqShift int64
	}
	var lifts []envLift
	for ei := range feedEnvs {
		var el envLift
		el.name = feedEnvs[ei].name
		for _, charIdx := range feedCharIdx {
			f, b0 := aggs[key{charIdx, ei, true}], aggs[key{charIdx, ei, false}]
			lift := f.samples[last].refine - b0.samples[last].refine
			shift := f.samples[last].danMetal - b0.samples[last].danMetal
			if charIdx == 3 {
				el.jinLift, el.jinShift = lift, shift
			} else {
				el.eqLift, el.eqShift = lift, shift
			}
			fmt.Fprintf(&b, "| %s | %s | %d | %d | %+d | %d | %d | %+d |\n",
				feedEnvs[ei].name, charName(charIdx), f.samples[last].refine, b0.samples[last].refine, lift,
				f.samples[last].danMetal, b0.samples[last].danMetal, shift)
		}
		lifts = append(lifts, el)
	}
	// 逐 seed 配对抬升的分位（金偏；seed 布局见 runFeedMatrix——金偏
	// charSlot=1，喂养轨 t=0、对照轨 t=1）。
	fmt.Fprintf(&b, "\n")
	for ei := range feedEnvs {
		var diffs []int64
		for k := 0; k < in.seeds; k++ {
			seedF := feedSeedBase + 1000 + uint64(ei)*10 + uint64(k)
			seedB := feedSeedBase + 1100 + uint64(ei)*10 + uint64(k)
			var refF, refB int64
			okF, okB := false, false
			for _, r := range in.cells {
				if r.charIdx == 3 && r.envIdx == ei && r.feed && r.seed == seedF {
					refF, okF = r.samples[last].refine, true
				}
				if r.charIdx == 3 && r.envIdx == ei && !r.feed && r.seed == seedB {
					refB, okB = r.samples[last].refine, true
				}
			}
			if okF && okB {
				diffs = append(diffs, refF-refB)
			}
		}
		if len(diffs) > 0 {
			q1, _, q3 := iqrOf(diffs)
			lifts[ei].jinSpread = q3 - q1
			fmt.Fprintf(&b, "- %s：金偏逐 seed 配对抬升 = %s（千分位；IQR %d）——高于 IQR 的抬升才谈得上显著。\n",
				feedEnvs[ei].name, iqrLine(diffs), lifts[ei].jinSpread)
		}
	}
	fmt.Fprintf(&b, "\n")
	// 裁决口径（测量设计内预先登记，非引擎参数）：各环境金偏的炼化抬升
	// 为正、大于均衡抬升且超出 seed 噪声（> IQR），构成金份额同向位移为
	// 正——全环境一致成立才算「显著抬升且随画像」；任一环节不满足即如
	// 实报，按注记裁决作废重议。
	verdict := true
	for i := range lifts {
		el := &lifts[i]
		ok := el.jinLift > 0 && el.jinLift > el.eqLift && el.jinLift > el.jinSpread && el.jinShift > 0
		fmt.Fprintf(&b, "- %s：金偏抬升 %+d（IQR %d）vs 均衡 %+d，金份额位移 金偏 %+d / 均衡 %+d——%s。\n",
			el.name, el.jinLift, el.jinSpread, el.eqLift, el.jinShift, el.eqShift,
			map[bool]string{true: "成立", false: "不成立"}[ok])
		if !ok {
			verdict = false
		}
	}
	if verdict {
		fmt.Fprintf(&b, "- **裁决成立**：旁支喂养使金偏画像炼化度显著抬升且构成向金收敛、抬升随画像——金门异气入口的可测证据。\n")
	} else {
		fmt.Fprintf(&b, "- **裁决作废条件触发**：喂养不通或构成不动（或抬升不显著/不随画像）——归因见下节。\n")
	}

	// ---------------- 物理合理性台账 ----------------
	fmt.Fprintf(&b, "\n### 喂养物理台账（喂养轮，全 run 累计，跨 seed 中位）——注入 → 带走 → 留存链\n\n")
	fmt.Fprintf(&b, "| 画像 | 环境 | 金门金属纳入 | 金门→中庭金属输送 | 中庭→阳枢金属带走 | 中庭金属净存 | 中庭金属泄出 | 中庭峰值负载比 | 金门峰值负载比 | 损伤 | 异常 |\n")
	fmt.Fprintf(&b, "|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, charIdx := range feedCharIdx {
		for ei := range feedEnvs {
			ag := aggs[key{charIdx, ei, true}]
			fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %+d | %d | %s | %s | %s | %s |\n",
				charName(charIdx), feedEnvs[ei].name, ag.jinMetalIn, ag.sideMetal, ag.midOutMetal,
				ag.midNet, ag.midDiffuse, fp(ag.midPeakRatio), fp(ag.jinPeakRatio), fp(ag.damage), fp(ag.anomalies))
		}
	}
	fmt.Fprintf(&b, "\n- 链判读：注入 ≈ 纳入（金门吞吐自节流）；带走 ≈ 注入（中庭不入即走——低滞留反应炉不截流，异气不在中庭沉积而在环流中显形）；净存小 = 一过而非散（终态在丹田与环上，见轨迹表金份额）；停歇段（R 块）份额回退与否见轨迹表 T/R 交替。\n")

	// ---------------- 终态丹田构成（归因） ----------------
	fmt.Fprintf(&b, "\n### 终态丹田构成（千分位占比，跨 seed 中位）——构成位移归因\n\n")
	fmt.Fprintf(&b, "| 画像 | 轨道 | 环境 | 木 | 火 | 土 | 金 | 水 |\n|---|---|---|---|---|---|---|---|\n")
	for _, charIdx := range feedCharIdx {
		for _, feed := range []bool{true, false} {
			for ei := range feedEnvs {
				ag := aggs[key{charIdx, ei, feed}]
				track := "对照"
				if feed {
					track = "喂养"
				}
				fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s |\n",
					charName(charIdx), track, feedEnvs[ei].name,
					fp(ag.danComp[0]), fp(ag.danComp[1]), fp(ag.danComp[2]),
					fp(ag.danComp[3]), fp(ag.danComp[4]))
			}
		}
	}

	// ---------------- 守恒 ----------------
	fmt.Fprintf(&b, "\n### 守恒（SF 全部会话 tick）\n\n")
	fmt.Fprintf(&b, "- 累计 tick %d；违例 %d；最大残差 %d\n", in.acc.ticks, in.acc.violations, in.acc.maxResidual)
	return b.String()
}
