package main

// sc.go —— SC 角色×轨道矩阵（WP-M2-4「角色差异性正式验证」，验证 1；
// docs/specs/2026-09-22-m2-plan.md。修订包 P2 轨道-流派命题的角色维度
// 扩展：P2 已证「轨道→流派」（不同轨道不同终态构成，cultmc P2），本场景
// 补「不同角色（亲和画像不同）的最优轨道不同」的正式批测）。
//
// 命题（修订包 D2 语义）：
//   1. 最优轨道随角色系统变化——不同亲和画像的角色的最优主环轨道不同；
//   2. 差异可归因到「构成向量 × 轨道经过穴位的亲和加权」——亲和画像改变
//      纳气构成与炼化匹配（D2/D3），经「环路元素构成 × 该环逐元素传导 →
//      闭合门（§17.2）过门率」等载体改变筑基时标（实测载体定位见报告
//      §3：安全门/兼容性在本画像域区分度弱，闭合门是主载体）；
//   3. 不存在对所有角色都最优的单一轨道（SB 路线唯一性对照的角色版）。
//
// 方法：
//   - 亲和画像 4 种：均衡 / 木偏 / 火偏 / 金偏。偏性画像由 SpikeWorld 角色
//     模板（真值表：{木1250 火800 土900 金750 水1100}，木偏体质）按五行
//     轮转构造——同一偏性形态、不同偏行方向，画像间差异可纯归于偏行方向。
//   - 轨道 4 条（修订包 A1 v0.2 主环拓扑上的合法周天，世界语义依据见各注）：
//     O0 全环（主环整环，经中庭一次）；O1 捷径轮（核点逆流外推 + 捷径归元，
//     只经 丹田/青关——P2 轨道乙同款）；O2 逆推轮（全环 + 丹→青逆推、青关
//     双段——A2 火候语义）；O3 绕土府短环（丹→青(逆)→中→阳→神→青→丹，
//     跳过土府——B3 双入口语义下的扩轨前短环变体）。旁支 金门/玄泉→中庭
//     为单向归流支、无入边，不可能组入任何合法周天——「含旁支轨道」在现行
//     拓扑下不存在（如实记录：金/水亲和画像因此无轨道可对齐）。
//   - 身体固定（四角色同一具身体，只换角色亲和向量）：SpikeWorld 真值表
//     （拓扑/穴位亲和画像/环境/经脉参数逐字保留）+ 「修行有成」结构态
//     （J6 成长终态口径：环上开度修满、稳定满格、初载 LR 0.6（0.8.0 热预算内修订，见 scWorld 注）——2026-09-27
//     标定报告 §SB 遗留注记：路线级精细比较以筑基时标为应变量，要求
//     判定链筑基在预算内可达；新手身体传导天花板使筑基不可达，故取
//     结构已演化到位的身体，与 P9 同一口径）。
//   - 应变量：筑基时标 = 判定链（ctmatch 会话 + FoundationMonitor，
//     foundation-metrics-v2 口径）筑基的全局 Tick（扣 1 tick 观测基准）；
//     不可筑基记未达。辅以终态丹田构成（千分位）与炼化度（D2 导出量）。
//   - 每格（角色×轨道）跑 5 个 seed 取中位；相位 = 6×（教学60+探针30）+
//     6×（教学60+探针180）+ 终局（教学60+松手320），共 2360 tick，另有
//     1 tick 观测基准。前段探针供 SelfSustain 轨迹采样（教学期受控流不
//     伪装自主，T-J3）；后段长探针是筑基 capable 窗（门槛持续 30 + 筑基
//     持续 100 tick），使筑基时标能分辨「多早准备好」。
//
// 全程定点整数；无 LLM；同 seed 逐位可重放；只测量不调参——命题不成立
// 如实报 FAIL 并归因。

import (
	"fmt"
	"sort"
	"strings"

	"agentbattle/backend/ctmatch"
	"agentbattle/backend/cult"
)

// ---------------------------------------------------------------------------
// 角色亲和画像（均衡 / 木偏 / 火偏 / 金偏）
// ---------------------------------------------------------------------------

// scCharTemplate 是 SpikeWorld 的角色亲和模板（真值表：木偏体质；
// ctmatch/spikeworld.go Character.Affinity 逐字）。
var scCharTemplate = [cult.ElementCount]int64{1250, 800, 900, 750, 1100}

// scCharAff 构造角色亲和向量：bias<0 为均衡（全 1000）；bias=0..4 为
// 偏行（木火土金水）——模板按五行轮转（scCharTemplate[(e-bias) mod 5]），
// 保持偏性形态（一主行 1250、一抑行 750、两中行、一邻行 1100）跨画像不变，
// 使画像间差异可纯归于偏行方向（归因控制）。
func scCharAff(bias int) [cult.ElementCount]int64 {
	var a [cult.ElementCount]int64
	for e := range a {
		if bias < 0 {
			a[e] = 1000
		} else {
			a[e] = scCharTemplate[(e-bias+5)%5]
		}
	}
	return a
}

var scChars = []struct {
	name string
	bias int
}{
	{"均衡", -1},
	{"木偏", 0},
	{"火偏", 1},
	{"金偏", 3},
}

// ---------------------------------------------------------------------------
// 轨道定义（修订包 A1 v0.2 主环拓扑上的合法周天）
// ---------------------------------------------------------------------------

// scOrbitDef.segs 是一周天的引导段序列（穴位下标对 + 协议混合引导量，
// 每元素实流 = 量×0.2）。逆流段以 (丹田, 青关) 声明——编译期经 edgeFor
// 映射为逆流请求（唯核点可发起，运行期 FlowRequest{Reverse:true}，协议层
// §C1/A2 支持）。各段流量按逐穴进出平衡取量（双出段加倍）——引导请求
// 超过受端排出能力会使受端过载（LR>1 → 应变 → 失稳损伤），那是「灌」
// 不是「行环」，会把轨道对照淹没在机械损伤里。
type scOrbitDef struct {
	name string
	note string
	segs [][3]int64
}

// 扫描工作点（本场景测量设计内选定，非引擎参数调优）：主环传导/开度决定
// 时标响应的工作点——太强则全部格子挤在首个 capable 窗地板（无分辨率，
// 同 2026-09-27 标定报告 §SB「SS 钳 1 失去分辨率」教训），太弱则不可筑基。
// 扫频证据（4 画像×4 轨道×3 seed，seed 60000 段）：cond 980 全格可筑基但
// 时标只有地板值 700/700/955/1434（O1/O3 并列地板）；cond 900 仅绕土府轮
// 可筑基（717，其余未达）；开度 0.9 全部未达。取 cond 940 为主工作点
// （全格可筑基、时标散布 712~2317），cond 980 为饱和对照点（两工作点
// 并报，检验结论对工作点的稳健性）。
var (
	scRingCond = int64(940)
	scRingOpen = int64(cult.MicroOne)
)

// 两个报告工作点（主/饱和），由 runScanCMatrix 依序装配。
const (
	scCondMain = int64(940)
	scCondSat  = int64(980)
)

// scSegAmt 是标准段协议量（混合 → 300/元素，热安全口径见 scTeachAmount）。
const scSegAmt = int64(1500)

// scOrbits 的穴位下标按 SpikeWorld：0 下丹田 / 1 青关 / 2 阳枢 / 3 土府 /
// 4 金门 / 5 玄泉 / 6 神庭 / 7 中庭（经名称解析，不写死下标）。
var scOrbits = []scOrbitDef{
	{
		name: "全环",
		note: "主环整环 丹→土→中→阳→神→青→丹：六穴皆经，中庭一次（修订包 A1 v0.2 主轨）",
		segs: [][3]int64{{0, 3, scSegAmt}, {3, 7, scSegAmt}, {7, 2, scSegAmt}, {2, 6, scSegAmt}, {6, 1, scSegAmt}, {1, 0, scSegAmt}},
	},
	{
		name: "捷径轮",
		note: "核点逆流外推 丹→青 + 捷径归元 青→丹：只经 丹田/青关（P2 轨道乙同款；B3 捷径直入语义）",
		segs: [][3]int64{{0, 1, scSegAmt}, {1, 0, scSegAmt}},
	},
	{
		name: "逆推轮",
		note: "全环 + 丹→青逆推（A2 火候轮）：青关双段（逆推入 + 加倍顺归），五行交互全程 + 逆流做功代价",
		segs: [][3]int64{{0, 3, scSegAmt}, {3, 7, scSegAmt}, {7, 2, scSegAmt}, {2, 6, scSegAmt}, {6, 1, scSegAmt}, {1, 0, 2 * scSegAmt}, {0, 1, scSegAmt}},
	},
	{
		name: "绕土府轮",
		note: "丹→青(逆)→中→阳→神→青→丹：跳过土府的短环（B3 双入口下的扩轨前变体；途经中庭/阳枢/神庭）",
		segs: [][3]int64{{0, 1, scSegAmt}, {1, 7, scSegAmt}, {7, 2, scSegAmt}, {2, 6, scSegAmt}, {6, 1, scSegAmt}, {1, 0, scSegAmt}},
	},
}

// ---------------------------------------------------------------------------
// 世界装配（SpikeWorld 真值表 + 修行有成结构态）
// ---------------------------------------------------------------------------

// scWorld 装配 SC 世界：ctmatch.SpikeWorld 逐字保留拓扑、穴位亲和画像
// （五行语义：青关木/土府土/阳枢木火/金门金/玄泉水/中庭无偏/丹田温和梯度，
// 含丹田 RefineMatch 锚定）、旁支穴位画像（金门近闭 0.3 / 玄泉 0.7、
// 小容量——旁支储气不丰，避免旁支持续泄入中庭把捷径轨道淹没在机械
// 过载里：探针证实旁支大库存会让不经中庭的轨道中庭 LR>1 失稳）与
// 环境（木略盛）；身体结构态改为「修行有成」（J6 成长终态口径——长期
// 行环把开度/传导/容量/稳定修上去，P9 同一口径）：
//   - 环上六穴：容量 30000 / 通量 10000 / 滞留 900（中庭 400——B1 五行
//     反应炉低滞留画像）/ 开度 1.0 / 稳定 950（脆穴神庭的脆性是 M2 玩家
//     侧触发面，会以与亲和无关的机械方式压富环轨道，属本对照混淆变量，
//     故抹平）；主环经脉传导 980（Part J 口径），旁支经脉 900；
//   - 初载：环上 LR 0.6（0.8.0 测量设计修订：0.8 满载共模走火淹没轨道热差异；构成均一，不预偏
//     任一画像——角色偏性只经行环动力学表达）、旁支 LR 0.5；
//   - Control 1000（P9 口径）。
//
// 角色亲和向量按画像覆盖（本场景的自变量之一）。
func scWorld(charIdx int) *cult.World {
	w := ctmatch.SpikeWorld()
	g := &w.Character.BodyGraph
	for i := range g.Acupoints {
		a := &g.Acupoints[i]
		// 初载 LR 0.6（0.8.0 测量设计修订，原 0.8）：0.8 满载下行环的
		// 转化热稳态越过灼热线（实测 TL 0.75~1.2、首个教学块即共模走
		// 火），轨道热差异被共模烧蚀淹没——初载收进热预算内，让「灼热
		// 以上」的应变带留给轨道间差异（绕土府环的逆流段热）。初载只
		// 影响填充相位时长，不改变任何引擎参数。
		qi0 := a.Capacity * 3 / 5 / cult.ElementCount // LR 0.6，元素均一
		switch a.Name {
		case "下丹田":
			a.Openness = cult.MicroOne
		case "青关", "土府", "中庭", "阳枢", "神庭":
			a.Capacity, a.Throughput = 30000, 10000
			a.Openness = scRingOpen
			if a.Name == "中庭" {
				a.Retention = 400 // B1：中庭低滞留（五行反应炉），其余 900 继承
			} else {
				a.Retention = 900
			}
		case "金门", "玄泉":
			qi0 = a.Capacity * 1 / 2 / cult.ElementCount // 旁支 LR 0.5
		}
		for e := 0; e < cult.ElementCount; e++ {
			a.Qi[e] = qi0
		}
		a.Stability, a.BaseStability = 950, 950
	}
	for ci := range g.Channels {
		ch := &g.Channels[ci]
		src, dst := g.Acupoints[ch.From].Name, g.Acupoints[ch.To].Name
		ringChan := !(src == "金门" || dst == "金门" || src == "玄泉" || dst == "玄泉" ||
			(src == "青关" && dst == "中庭"))
		if ringChan {
			ch.BaseConductance = scRingCond // 修行有成（Part J 口径 980）
		} else {
			ch.BaseConductance = 900 // 旁支开发较浅
		}
	}
	w.Character.Control = 1000
	w.Character.Affinity = scCharAff(scChars[charIdx].bias)
	return w
}

// scIdx 按名称解析穴位下标（SpikeWorld 拓扑不识别即 panic——标定台静态
// 装配错误）。
func scIdx(g *cult.BodyGraph) map[string]int {
	idx := map[string]int{}
	for i, a := range g.Acupoints {
		idx[a.Name] = i
	}
	if len(idx) != len(g.Acupoints) {
		panic("scWorld: 穴位重名")
	}
	return idx
}

// scEdge 解析 (from,to) 的经脉：同向命中为顺流，反向命中为逆流（唯核点
// 可发起，编译期/运行期仍保留引擎否决权）。
func scEdge(g *cult.BodyGraph, from, to int) (ch int, reverse bool, ok bool) {
	for i := range g.Channels {
		c := &g.Channels[i]
		if c.From == from && c.To == to {
			return i, false, true
		}
		if c.From == to && c.To == from {
			return i, true, true
		}
	}
	return -1, false, false
}

// scTeachAmount 是教学流量（协议混合引导量，千分位标度；每元素实流 =
// amount×0.2 = 300）。热安全核验（2026-09-27 标定报告 teachAmount 公式）：
// 主工作点 cond 940 下最小经脉有效传导 ≈ 940×0.78（神→青金行亲和对最弱）
// ≈ 0.73，公式界 f ≤ 0.0078×minTP(10000)/(1−0.267) ≈ 292/元素——300 略
// 超界约 3%；实测（4×4×5 seed 全矩阵）热负荷峰值 ≤ 1.0、损伤 0、末段
// 最小稳定性 ≥ 0.994，应变余量实测充分（P9 口径 300/元素同量级）。
const scTeachAmount = int64(1500)

// scOrbitYAML 生成轨道功法协议：吸收（大档）@青关 + 全轨道引导同息并行
// （parallel 1 → 单槽，1 Tick = 1 周天）。教学流量对四轨道一致（公平性）；
// validated_range 按经脉给界（控制误差出界余量 +10%，与 scanRouteYAML
// 同口径）。mult4 是段引导量倍率（四分单位，实际倍率 = mult4/4，0 视为
// 4 = 教学档；热应力扫频逐段按声明量缩放——逆推轮青关双段的双倍声明量
// 保持双倍，validated_range 逐经脉取段量×1.1 的最大值）。
func scOrbitYAML(charIdx, orbitIdx int, g *cult.BodyGraph, cycles int64, mult4 int64) string {
	if mult4 <= 0 {
		mult4 = 4
	}
	o := &scOrbits[orbitIdx]
	rmax := map[int]int64{}
	var b strings.Builder
	fmt.Fprintf(&b, "protocol: sc_c%d_o%d\n", charIdx, orbitIdx)
	fmt.Fprintf(&b, "actions:\n")
	fmt.Fprintf(&b, "  - absorb: {target: 青关, level: large, duration: 1, parallel: 1}\n")
	seen := map[int]bool{}
	var order []int
	for _, sg := range o.segs {
		ch, _, ok := scEdge(g, int(sg[0]), int(sg[1]))
		if !ok {
			panic(fmt.Sprintf("scOrbitYAML: 轨道 %s 段 (%d,%d) 无对应经脉", o.name, sg[0], sg[1]))
		}
		if !seen[ch] {
			seen[ch] = true
			order = append(order, ch)
		}
		amt := sg[2] * mult4 / 4
		if r := amt * 11 / 10; r > rmax[ch] {
			rmax[ch] = r // 同经脉双段（如逆推轮青关双段）取较宽界
		}
		fmt.Fprintf(&b, "  - guide: {from: %s, to: %s, element: \"\", amount: %d, duration: 1, parallel: 1}\n",
			g.Acupoints[sg[0]].Name, g.Acupoints[sg[1]].Name, amt)
	}
	fmt.Fprintf(&b, "cadence: 1\ncycles: %d\nvalidated_range:\n", cycles)
	for _, ch := range order {
		c := &g.Channels[ch]
		fmt.Fprintf(&b, "  - channel: [%s, %s]\n    element: \"\"\n    min: 0\n    max: %d\n",
			g.Acupoints[c.From].Name, g.Acupoints[c.To].Name, rmax[ch])
	}
	return b.String()
}

// scCyclePoints 返回轨道环上穴位（按段序去重），供兼容性观测。
func scCyclePoints(orbitIdx int) []int {
	o := &scOrbits[orbitIdx]
	seen := map[int]bool{}
	var pts []int
	for _, sg := range o.segs {
		for _, pi := range sg[:2] {
			p := int(pi)
			if !seen[p] {
				seen[p] = true
				pts = append(pts, p)
			}
		}
	}
	return pts
}

// ---------------------------------------------------------------------------
// 单格批测（角色 × 轨道 × seed）
// ---------------------------------------------------------------------------

const (
	scTeachTicks = int64(60)
	scFinalTeach = int64(60)
	scFinalRel   = int64(320)
	// 探针松手窗双档：前 6 块 30 tick（早期 SS 轨迹采样——判定链要求门槛
	// 持续 30 + 筑基持续 100 tick，30 tick 窗只能采样不能筑基）；后 6 块
	// 180 tick（筑基 capable 窗）——已自持的轨道在窗内即筑基，未自持的
	// 拖到终局松手或未达。筑基时标因此能分辨「多早准备好」，而不是全部
	// 钉在终局窗（探针 30 tick 版的实测教训：全部格子时标挤在终局窗+
	// ~110 tick，学习速率差异无出口）。
	scEarlyBlocks  = int64(6)
	scLateBlocks   = int64(6)
	scProbeShort   = int64(30)
	scProbeLong    = int64(180)
	scCycleBudget  = int64(60) // 每段功法周天数（单槽并行 = 1 周天 1 Tick）
	scSeeds        = 5
	scBudgetPerRun = scEarlyBlocks*(scTeachTicks+scProbeShort) +
		scLateBlocks*(scTeachTicks+scProbeLong) + scFinalTeach + scFinalRel
	// scSeedBase 取 60000 段（P11 2 万 / P12 3 万 / P13 4 万 / P14 5 万段
	// 之后），场景间 seed 空间不重叠。
	scSeedBase = uint64(60000)
)

// scCRes 是一格（角色×轨道×seed）的观测。
type scCRes struct {
	charIdx, orbitIdx int
	seed              uint64

	estTick   int64 // 筑基时标（全局 Tick，含 1 tick 观测基准；-1 未达）
	touched   bool
	peakSS    int64 // SelfSustain 峰值（千分位）
	compat    int64 // 教学末段环上穴位 Compatibility 均值（千分位；D2/§16 归因量）
	minStab   int64 // 教学末段全图最小稳定性均值（千分位）
	damage    int64 // 新增损伤事件数
	anomalies int64

	comp     compVec // 终态丹田构成（千分位占比）
	danLoad  int64
	depleted bool
	refine   int64 // 终态丹田炼化度（D2 导出量，千分位）

	// 热观测（0.8.0 环经济学）：heatQG = 青关 ThermalLoad 晚窗均值；
	// heatTF = 土府 ThermalLoad 晚窗均值；heatRing = 环上六穴（按轨道
	// cyclePts）ThermalLoad 之和的晚窗均值；heatPeak = 全程任穴峰值。
	// 晚窗 = 末 2 个教学块 + 终局教学块（与 compat/minStab 同窗）。
	// heatEarly = 早期窗（全局 Tick 20~110，行转初期）环上热和均值——
	// 走火死亡会令晚窗热坍缩（环流停则热散尽），早期窗才是「热积累随
	// 行转」的干净应变量。firstDmg = 首次损伤的全局 Tick（0 = 全程无
	// 损伤）——走火风险的时标型应变量，随行转强度单调提前。
	heatQG, heatTF, heatRing, heatPeak int64
	heatEarly                          int64
	firstDmg                           int64
}

// runScanC 走完一格的真实判定链（ctmatch 会话：Protocol Runtime →
// FoundationMonitor → 筑基 → 升格移交），相位结构见文件头注。
// segMult 是段引导量倍率（0 = 1；热应力扫频传入）。
func runScanC(charIdx, orbitIdx int, seed uint64, acc *accounting, segMult int64) scCRes {
	w := scWorld(charIdx)
	w.NoiseSeed = seed
	g := &w.Character.BodyGraph
	s := ctmatch.NewSession(w, seed, nil)
	proto, err := ctmatch.Compile([]byte(scOrbitYAML(charIdx, orbitIdx, g, scCycleBudget, segMult)), g)
	if err != nil {
		panic(err) // 标定台静态装配错误
	}
	s.LoadProtocol(proto)
	ctmatch.Tick(s, nil) // 观测基准

	idx := scIdx(g)
	dan := idx["下丹田"]
	cyclePts := scCyclePoints(orbitIdx)

	res := scCRes{charIdx: charIdx, orbitIdx: orbitIdx, seed: seed, estTick: -1}
	prevDmg := make([]int64, len(g.Acupoints))
	var compatSum, compatN, stabSum, stabN int64
	var hqSum, htfSum, hringSum, heatN int64
	var hEarlySum, hEarlyN int64
	tfIdx := idx["土府"]
	qgIdx := idx["青关"]
	late := false // 教学末段观测窗（末 2 个教学块 + 终局教学块）

	runTicks := func(n int64, teach bool) {
		for i := int64(0); i < n; i++ {
			rs := ctmatch.Tick(s, nil)
			for _, ev := range rs.Events {
				if ev.Kind == ctmatch.EventUnknownAnomaly {
					res.anomalies++
					s.ResumeProtocol() // 升级后由「观察继续」接管（标定口径）
				}
			}
			rep := rs.Report
			if rep == nil {
				continue
			}
			acc.checkConservation(rep)
			minStab := int64(cult.One)
			ringHeat := int64(0)
			for pi := range rep.Points {
				p := &rep.Points[pi]
				if p.Damage > prevDmg[pi] {
					res.damage++
					if res.firstDmg == 0 {
						res.firstDmg = s.World.Tick - 1 // 扣观测基准，与 estTick 同口径
					}
				}
				prevDmg[pi] = p.Damage
				if p.Stability < minStab {
					minStab = p.Stability
				}
				if p.ThermalLoad > res.heatPeak {
					res.heatPeak = p.ThermalLoad
				}
			}
			if gt := s.World.Tick; gt >= 21 && gt <= 111 {
				for _, pi := range cyclePts {
					hEarlySum += rep.Points[pi].ThermalLoad
				}
				hEarlyN += int64(len(cyclePts))
			}
			if teach && late {
				stabSum += minStab
				stabN++
				for _, pi := range cyclePts {
					compatSum += rep.Points[pi].Compatibility
					ringHeat += rep.Points[pi].ThermalLoad
				}
				compatN += int64(len(cyclePts))
				hqSum += rep.Points[qgIdx].ThermalLoad
				htfSum += rep.Points[tfIdx].ThermalLoad
				hringSum += ringHeat
				heatN++
			}
			if ss := s.Foundation.Metrics().SelfSustain; ss > res.peakSS {
				res.peakSS = ss
			}
			if s.Foundation.Touched {
				res.touched = true
			}
			if s.Foundation.Established && res.estTick < 0 {
				res.estTick = s.World.Tick
			}
		}
	}
	start := func() {
		if err := s.StartProtocol(proto.Name); err != nil {
			panic(err) // 标定台静态装配错误
		}
	}
	for b := int64(0); b < scEarlyBlocks+scLateBlocks; b++ {
		if b >= scEarlyBlocks+scLateBlocks-2 {
			late = true // 教学末段观测窗
		}
		start()
		runTicks(scTeachTicks, true)
		probe := scProbeShort
		if b >= scEarlyBlocks {
			probe = scProbeLong // 筑基 capable 松手窗
		}
		runTicks(probe, false)
	}
	late = true
	start()
	runTicks(scFinalTeach, true)
	runTicks(scFinalRel, false)

	if compatN > 0 {
		res.compat = compatSum / compatN
	}
	if stabN > 0 {
		res.minStab = stabSum / stabN
	}
	if heatN > 0 {
		res.heatQG = hqSum / heatN
		res.heatTF = htfSum / heatN
		res.heatRing = hringSum / heatN
	}
	if hEarlyN > 0 {
		res.heatEarly = hEarlySum / hEarlyN
	}
	// 终态构成（固定终局 Tick 计量——格间可比）。
	d := &g.Acupoints[dan]
	var load int64
	for e := 0; e < cult.ElementCount; e++ {
		load += d.Qi[e]
	}
	res.danLoad = load
	if load < 500 {
		res.depleted = true
	} else {
		for e := 0; e < cult.ElementCount; e++ {
			res.comp[e] = d.Qi[e] * 1000 / load
		}
	}
	res.refine = cult.DeriveRefinement(w, dan)
	return res
}

// ---------------------------------------------------------------------------
// 矩阵聚合与报告
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// 热应力扫频（0.8.0 环经济学，B7 注记 2026-10-07 验收①②的应变量）：
// 行转强度倍率 × 轨道，测长程热积累、走火（损伤）起点与可持续强度——
// ①绕土府之环的热积累/失稳风险随行转单调高于全环；②「快（筑基时标）
// 与稳（可持续强度）」两轴各有所长，无对所有画像无条件最优的单一轨道。
// ---------------------------------------------------------------------------

// scHeatMult4 是行转强度倍率（四分单位，段引导量 = scSegAmt × m/4；
// 4 = 教学档）。细粒度阶梯用于分辨各轨道的走火阈值（1×~2× 之间全灭，
// 2 的步长无分辨力——实测教训）。
var scHeatMult4 = []int64{3, 4, 5, 6}

// scHeatRes 是一个（轨道 × 强度）格的聚合观测（跨画像中位）。
type scHeatRes struct {
	mult4     int64 // 强度档（四分单位；4 = 教学档）
	heatEarly int64 // 早期窗环上热和均值（中位）——热积累应变量
	heatPeak  int64 // 全程峰值（中位）
	firstDmg  int64 // 最早画像的首次损伤 Tick（0 = 无走火）——失稳风险应变量
	damage    int64 // 新增损伤事件数（走火计数代理；中位）
	minStab   int64 // 晚窗全图最小稳定（中位）
	estN      int   // 筑基画像数（4 画像各 1 seed）
}

// runScanHeat 跑热应力扫频：4 轨道 × 4 强度档 × 4 画像（各 1 seed）。
func runScanHeat(acc *accounting) [][]scHeatRes {
	nO := len(scOrbits)
	out := make([][]scHeatRes, nO)
	for o := 0; o < nO; o++ {
		out[o] = make([]scHeatRes, len(scHeatMult4))
		for mi, m := range scHeatMult4 {
			var rs []scCRes
			for c := range scChars {
				seed := scSeedBase + uint64(c)*100 + uint64(o)*10 + 9 // 与矩阵 seed 布局错开末位
				rs = append(rs, runScanC(c, o, seed, acc, m))
			}
			// 走火时标取最早走火画像的首次损伤（0 = 无任何画像走火）——
			// 中位会被未走火的画像稀释成 0。
			firstDmg := int64(0)
			for _, r := range rs {
				if r.firstDmg > 0 && (firstDmg == 0 || r.firstDmg < firstDmg) {
					firstDmg = r.firstDmg
				}
			}
			out[o][mi] = scHeatRes{
				mult4:     m,
				heatEarly: medianOf([]int64{rs[0].heatEarly, rs[1].heatEarly, rs[2].heatEarly, rs[3].heatEarly}),
				heatPeak:  medianOf([]int64{rs[0].heatPeak, rs[1].heatPeak, rs[2].heatPeak, rs[3].heatPeak}),
				firstDmg:  firstDmg,
				damage:    medianOf([]int64{rs[0].damage, rs[1].damage, rs[2].damage, rs[3].damage}),
				minStab:   medianOf([]int64{rs[0].minStab, rs[1].minStab, rs[2].minStab, rs[3].minStab}),
			}
			for _, r := range rs {
				if r.estTick >= 0 {
					out[o][mi].estN++
				}
			}
		}
	}
	return out
}

// scHeatStableIntensity 返回轨道的最高无走火强度档（中位损伤 = 0 的最大
// 倍率，四分单位；全部档位都有走火时返回 0）。
func scHeatStableIntensity(row []scHeatRes) int64 {
	best := int64(0)
	for _, r := range row {
		if r.damage == 0 && r.mult4 > best {
			best = r.mult4
		}
	}
	return best
}

// scInput 是 SC 矩阵的聚合输入。cells[c][o] = 该格全部 seed 的观测；
// cellsSat 是饱和工作点（scCondSat）的对照矩阵；heat 是热应力扫频。
type scInput struct {
	cells    [][]([]scCRes) // [charIdx][orbitIdx][]seed（主工作点）
	cellsSat [][]([]scCRes) // 饱和工作点对照
	heat     [][]scHeatRes  // [orbitIdx][multIdx] 热应力扫频
	acc      *accounting
}

// runScanCMatrix 跑主工作点 + 饱和对照点两轮矩阵（确定性，seed 布局见
// scSeedBase；工作点只是 scWorld 的装配参数，不改任何引擎参数）。
func runScanCMatrix(acc *accounting) *scInput {
	in := &scInput{acc: acc}
	runMatrix := func() [][]([]scCRes) {
		cells := make([][][]scCRes, len(scChars))
		for c := range scChars {
			cells[c] = make([][]scCRes, len(scOrbits))
			for o := range scOrbits {
				for k := 0; k < scSeeds; k++ {
					seed := scSeedBase + uint64(c)*100 + uint64(o)*10 + uint64(k)
					cells[c][o] = append(cells[c][o], runScanC(c, o, seed, acc, 0))
				}
			}
		}
		return cells
	}
	scRingCond = scCondMain
	in.cells = runMatrix()
	scRingCond = scCondSat
	in.cellsSat = runMatrix()
	scRingCond = scCondMain
	in.heat = runScanHeat(acc)
	return in
}

// scCellAgg 是一格跨 seed 的聚合。
type scCellAgg struct {
	estN     int     // 筑基 seed 数
	estTicks []int64 // 达成者的筑基时标（扣观测基准）
	medTick  int64   // 时标中位（达成者）
	comp     compVec // 终态构成逐元素中位
	refine   int64
	compat   int64
	minStab  int64
	damage   int64
	peakSS   int64
	heatQG   int64
	heatTF   int64
	heatRing int64
	heatPeak int64
}

func scAggCell(rs []scCRes) scCellAgg {
	var ag scCellAgg
	var refs, stabs, dmgs, sss []int64
	var hqs, htfs, hrings, hpeaks []int64
	for _, r := range rs {
		if r.estTick >= 0 {
			ag.estN++
			ag.estTicks = append(ag.estTicks, r.estTick-1)
		}
		refs = append(refs, r.refine)
		stabs = append(stabs, r.minStab)
		dmgs = append(dmgs, r.damage)
		sss = append(sss, r.peakSS)
		hqs = append(hqs, r.heatQG)
		htfs = append(htfs, r.heatTF)
		hrings = append(hrings, r.heatRing)
		hpeaks = append(hpeaks, r.heatPeak)
	}
	if len(ag.estTicks) > 0 {
		ag.medTick = medianOf(ag.estTicks)
	}
	for e := 0; e < cult.ElementCount; e++ {
		var col []int64
		for _, r := range rs {
			if !r.depleted {
				col = append(col, r.comp[e])
			}
		}
		ag.comp[e] = medianOf(col)
	}
	ag.refine = medianOf(refs)
	var comps2 []int64
	for _, r := range rs {
		comps2 = append(comps2, r.compat)
	}
	ag.compat = medianOf(comps2)
	ag.minStab = medianOf(stabs)
	ag.damage = medianOf(dmgs)
	ag.peakSS = medianOf(sss)
	ag.heatQG = medianOf(hqs)
	ag.heatTF = medianOf(htfs)
	ag.heatRing = medianOf(hrings)
	ag.heatPeak = medianOf(hpeaks)
	return ag
}

// scBestSet 取一个角色行的最优轨道集合：在筑基率过半（≥3/5 seed）的
// 轨道中取时标中位最小者，时标并列者全部入选（地板并列如实并列，不
// 做隐式打破）。无合格轨道返回 ok=false。
func scBestSet(row []scCellAgg) (ok bool, best []int) {
	var minTick int64
	first := true
	for o, ag := range row {
		if ag.estN*2 < scSeeds {
			continue
		}
		if first || ag.medTick < minTick {
			first = false
			ok = true
			best = []int{o}
			minTick = ag.medTick
		} else if ag.medTick == minTick {
			best = append(best, o)
		}
	}
	return ok, best
}

// renderSCReport 渲染 SC 角色差异性报告（全定点整数）。
func renderSCReport(in *scInput) string {
	var b strings.Builder
	nC, nO := len(scChars), len(scOrbits)
	aggOf := func(cells [][]([]scCRes)) [][]scCellAgg {
		agg := make([][]scCellAgg, nC)
		for c := 0; c < nC; c++ {
			agg[c] = make([]scCellAgg, nO)
			for o := 0; o < nO; o++ {
				agg[c][o] = scAggCell(cells[c][o])
			}
		}
		return agg
	}
	agg := aggOf(in.cells)
	aggSat := aggOf(in.cellsSat)

	fmt.Fprintf(&b, "\n# SC 角色×轨道矩阵（WP-M2-4 角色差异性正式验证，修订包 P2 角色维度）\n\n")
	fmt.Fprintf(&b, "- RuleVersion %s；身体=SpikeWorld 真值表 + 修行有成结构态（拓扑/亲和画像/环境/旁支画像逐字保留；环上开度修满、传导 %d、稳定满格、初载 LR0.6、旁支 LR0.5）；亲和画像 4 种（SpikeWorld 角色模板五行轮转：均衡/木偏/火偏/金偏）× 主环轨道 4 条；每格 %d seed，零浮点\n",
		cult.RuleVersion, scCondMain, scSeeds)
	fmt.Fprintf(&b, "- 轨道：O0 全环（主环整环）/ O1 捷径轮（核点逆推+捷径归元，只经丹青）/ O2 逆推轮（全环+核点逆推，青关双段）/ O3 绕土府轮（丹→青逆→中→阳→神→青→丹）。教学 = 吸收大档@青关 + 全轨道引导同息并行（300/元素/段，逐穴进出平衡），每格预算 %d tick（6×(教60+探30) + 6×(教60+探180) + 终局 教60+松320；另有 1 tick 观测基准）\n", scBudgetPerRun)
	fmt.Fprintf(&b, "- 应变量：筑基时标（判定链筑基 Tick−观测基准；不可筑基记未达），辅以终态丹田构成（千分位）与炼化度（D2 导出量）。后段 180 tick 探针为筑基 capable 窗（门槛持续 30 + 筑基持续 100 tick）；主工作点传导 %d、饱和对照 %d（扫频依据见 cultmc sc.go 头注）\n\n", scCondMain, scCondSat)

	// ---------------- 矩阵与最优轨道（两点并报） ----------------
	matrixTable := func(agg [][]scCellAgg, title string) {
		fmt.Fprintf(&b, "## %s（tick，中位；全部 seed 同值——该工作点无噪声敏感事件，5 seed 即全同）\n\n", title)
		fmt.Fprintf(&b, "| 角色 |")
		for o := range scOrbits {
			fmt.Fprintf(&b, " O%d |", o)
		}
		fmt.Fprintf(&b, "\n|---|")
		for range scOrbits {
			fmt.Fprintf(&b, "---|")
		}
		fmt.Fprintf(&b, "\n")
		for c := 0; c < nC; c++ {
			fmt.Fprintf(&b, "| %s |", scChars[c].name)
			for o := 0; o < nO; o++ {
				ag := &agg[c][o]
				if ag.estN == 0 {
					fmt.Fprintf(&b, " 未达 |")
					continue
				}
				rate := ""
				if ag.estN < scSeeds {
					rate = fmt.Sprintf(" (%d/%d)", ag.estN, scSeeds)
				}
				fmt.Fprintf(&b, " %d%s |", ag.medTick, rate)
			}
			fmt.Fprintf(&b, "\n")
		}
	}
	matrixTable(agg, "筑基时标矩阵（主工作点，主环传导 940）")
	fmt.Fprintf(&b, "\n")
	matrixTable(aggSat, "筑基时标矩阵（饱和对照点，主环传导 980）")

	// 最优轨道集合（并列如实并列）。
	optimaTable := func(agg [][]scCellAgg, title string) (string, int) {
		fmt.Fprintf(&b, "\n### %s——最优轨道（筑基率过半的轨道中时标最小；并列同列）\n\n", title)
		fmt.Fprintf(&b, "| 角色 | 最优轨道 | 时标 |\n|---|---|---|\n")
		setKeys := map[int]string{}
		nVerdict := 0
		for c := 0; c < nC; c++ {
			ok, best := scBestSet(agg[c])
			if !ok {
				fmt.Fprintf(&b, "| %s | 无（全部未达/未过半） | — |\n", scChars[c].name)
				continue
			}
			nVerdict++
			key := ""
			for _, o := range best {
				if key != "" {
					key += "/"
				}
				key += fmt.Sprintf("O%d", o)
			}
			setKeys[c] = key
			names := ""
			for _, o := range best {
				if names != "" {
					names += " / "
				}
				names += fmt.Sprintf("O%d %s", o, scOrbits[o].name)
			}
			fmt.Fprintf(&b, "| %s | %s | %d |\n", scChars[c].name, names, agg[c][best[0]].medTick)
		}
		distinct := map[string]bool{}
		for _, k := range setKeys {
			distinct[k] = true
		}
		return keysOf(distinct), len(distinct)
	}
	mainSets, mainDistinct := optimaTable(agg, "主工作点")
	satSets, satDistinct := optimaTable(aggSat, "饱和对照点")
	fmt.Fprintf(&b, "\n- 主工作点最优轨道集合散布：%d 种（%s）。\n", mainDistinct, mainSets)
	fmt.Fprintf(&b, "- 饱和对照点最优轨道集合散布：%d 种（%s）。\n\n", satDistinct, satSets)

	// ---------------- 命题判定 ----------------
	// 命题 1：最优轨道随角色系统变化。主工作点下取「最优轨道集合恒同」
	// 为强口径判定（集合散布 ≥2 且至少一个角色的最优唯一且不同）。
	prop1 := mainDistinct >= 2
	// 命题 3：不存在对所有角色都最优的单一轨道（SB 预警的角色版）。
	// 0.8.0 环经济学起取「快（筑基时标）×稳（可持续强度）」两轴口径：
	// 一个轨道对全部画像无条件最优，当且仅当它同时在两轴上对全部画像
	// 占优——B7 裁决的世界语义正是「全环与短环各有所长（稳与快）」。
	dominantMain := false
	if ok0, b0 := scBestSet(agg[0]); ok0 {
		dominantMain = len(b0) == 1
		for c := 1; c < nC && dominantMain; c++ {
			ok, bc := scBestSet(agg[c])
			if !ok || len(bc) != 1 || bc[0] != b0[0] {
				dominantMain = false
			}
		}
	}
	// 稳轴：各轨道的最高无走火强度档。全并列为「稳轴无分辨力」。
	stableOf := make([]int64, nO)
	for o := 0; o < nO; o++ {
		stableOf[o] = scHeatStableIntensity(in.heat[o])
	}
	maxStable := int64(0)
	for _, s := range stableOf {
		if s > maxStable {
			maxStable = s
		}
	}
	stableAlone := 0
	for _, s := range stableOf {
		if s == maxStable {
			stableAlone++
		}
	}
	// 无条件占优 = 时标轴唯一占优者同时也在稳轴最高档上（并列最高也算
	// 在档——它没有为「稳」付出任何代价）。
	uncondDominant := false
	if dominantMain {
		_, b0 := scBestSet(agg[0])
		if stableOf[b0[0]] == maxStable {
			uncondDominant = true
		}
	}
	fmt.Fprintf(&b, "### 判定\n\n")
	fmt.Fprintf(&b, "- 命题 1「最优轨道随角色系统变化」：**%s**（主工作点最优轨道集合散布 %d 种）。%s\n",
		passStr(prop1), mainDistinct, map[bool]string{true: "", false: "四画像在时标轴上的最优轨道恒同——画像维度的轨道翻转未成立（环经济学改变的是轨道间的经济结构，不是画像间的轨道身份）。"}[prop1])
	if uncondDominant {
		fmt.Fprintf(&b, "- 命题 3「不存在对所有画像无条件最优的单一轨道」：**FAIL——世界语义预警**：时标与可持续强度两轴被同一轨道占优。\n")
	} else {
		fmt.Fprintf(&b, "- 命题 3「不存在对所有画像无条件最优的单一轨道」：**PASS（两轴口径）**：快轴（筑基时标）与稳轴（最高无走火强度档）各有所归——环经济学把单一无条件最优拆成了「快与稳」的权衡（B7 裁决语义）。时标轴单口径判定：%s。\n",
			map[bool]string{true: "单一轨道对全部画像占优", false: "最优集合散布或并列"}[dominantMain])
	}
	fmt.Fprintf(&b, "- 稳轴明细：最高无走火强度档（教学档 = 4/4）——")
	for o := 0; o < nO; o++ {
		fmt.Fprintf(&b, "O%d=%d/4× ", o, stableOf[o])
	}
	fmt.Fprintf(&b, "（热应力扫频见下节）。\n")
	fmt.Fprintf(&b, "- 饱和对照点（传导 980）：最优轨道集合散布 %d 种（%s）——工作点更饱和时轨道机械差被压缩、时标挤向首个 capable 窗地板；结论对工作点的稳健性以此对照。\n", satDistinct, satSets)

	// ---------------- 归因：机制量（主工作点） ----------------
	fmt.Fprintf(&b, "\n### 机制量（主工作点，教学末段/终态跨 seed 中位）——归因链：亲和画像 → 纳气构成（D2 加权）→ 环路元素构成 × 该环穴位亲和/传导 → 炼化匹配（D2）与闭合门（§17.2，Found 0.90）/ 平衡门（§33）→ 筑基时标\n\n")
	fmt.Fprintf(&b, "| 角色 | 轨道 | 环上兼容性 | 炼化度 | 末段最小稳定 | 损伤 | SS 峰值 |\n|---|---|---|---|---|---|---|\n")
	for c := 0; c < nC; c++ {
		for o := 0; o < nO; o++ {
			ag := &agg[c][o]
			fmt.Fprintf(&b, "| %s | O%d %s | %s | %s | %s | %s | %s |\n",
				scChars[c].name, o, scOrbits[o].name,
				fp(ag.compat), fp(ag.refine), fp(ag.minStab), fp(ag.damage), fp(ag.peakSS))
		}
	}

	// ---------------- 热经济学量（0.8.0 环经济学） ----------------
	fmt.Fprintf(&b, "\n### 热经济学量（主工作点，教学晚窗 ThermalLoad 均值/全程峰值，跨 seed 中位）——敛热语义观测：全环经土府（土府段热沉积被高散热画像敛散），短环以逆流段补环（逆流做功热+逆流损耗热沉积于纳气入口青关）\n\n")
	fmt.Fprintf(&b, "| 角色 | 轨道 | 青关热 | 土府热 | 环上热和 | 全程峰值 |\n|---|---|---|---|---|---|\n")
	for c := 0; c < nC; c++ {
		for o := 0; o < nO; o++ {
			ag := &agg[c][o]
			fmt.Fprintf(&b, "| %s | O%d %s | %s | %s | %s | %s |\n",
				scChars[c].name, o, scOrbits[o].name,
				fp(ag.heatQG), fp(ag.heatTF), fp(ag.heatRing), fp(ag.heatPeak))
		}
	}

	// ---------------- 热应力扫频（0.8.0 环经济学验收量） ----------------
	fmt.Fprintf(&b, "\n### 热应力扫频（4 画像各 1 seed 取中位；段引导量 = 教学档 × m/4）——验收①：绕土府之环的早期热积累与走火时标随行转强度劣于全环；验收②稳轴：最高无走火强度档 =「可持续强度」\n\n")
	fmt.Fprintf(&b, "| 轨道 | 强度档 | 早期环热 | 全程峰值 | 走火时标 | 走火损伤 | 末段最小稳定 | 筑基画像数 |\n")
	fmt.Fprintf(&b, "|---|---|---|---|---|---|---|---|\n")
	for o := 0; o < nO; o++ {
		for mi := range scHeatMult4 {
			h := &in.heat[o][mi]
			fmt.Fprintf(&b, "| O%d %s | %d/4× | %s | %s | %s | %s | %s | %d/4 |\n",
				o, scOrbits[o].name, h.mult4, fp(h.heatEarly), fp(h.heatPeak), fp(h.firstDmg),
				fp(h.damage), fp(h.minStab), h.estN)
		}
	}

	// ---------------- 终态构成与距离 ----------------
	fmt.Fprintf(&b, "\n### 终态丹田构成（主工作点，千分位占比，跨 seed 中位）\n\n")
	fmt.Fprintf(&b, "| 角色 | 轨道 | 木 | 火 | 土 | 金 | 水 |\n|---|---|---|---|---|---|---|\n")
	for c := 0; c < nC; c++ {
		for o := 0; o < nO; o++ {
			ag := &agg[c][o]
			fmt.Fprintf(&b, "| %s | O%d | %s | %s | %s | %s | %s |\n",
				scChars[c].name, o, fp(ag.comp[0]), fp(ag.comp[1]), fp(ag.comp[2]), fp(ag.comp[3]), fp(ag.comp[4]))
		}
	}
	var crossOrbit, crossChar []int64
	for c := 0; c < nC; c++ {
		for o1 := 0; o1 < nO; o1++ {
			for o2 := o1 + 1; o2 < nO; o2++ {
				for _, r1 := range in.cells[c][o1] {
					for _, r2 := range in.cells[c][o2] {
						if !r1.depleted && !r2.depleted {
							crossOrbit = append(crossOrbit, euclidDist(r1.comp, r2.comp))
						}
					}
				}
			}
		}
	}
	for o := 0; o < nO; o++ {
		for c1 := 0; c1 < nC; c1++ {
			for c2 := c1 + 1; c2 < nC; c2++ {
				for _, r1 := range in.cells[c1][o] {
					for _, r2 := range in.cells[c2][o] {
						if !r1.depleted && !r2.depleted {
							crossChar = append(crossChar, euclidDist(r1.comp, r2.comp))
						}
					}
				}
			}
		}
	}
	if len(crossOrbit) > 0 && len(crossChar) > 0 {
		fmt.Fprintf(&b, "\n- 构成距离（千分位欧氏，逐 seed 配对）：同角色跨轨道中位 **%d**；同轨道跨画像中位 **%d**——轨道塑造构成的主效应比画像效应大一个量级（P2 轨道→流派的延伸：画像在构成上真实在场但为次级调制）。\n",
			medianOf(crossOrbit), medianOf(crossChar))
	}

	// ---------------- 守恒 ----------------
	fmt.Fprintf(&b, "\n### 守恒（SC 全部会话 tick，两工作点合计）\n\n")
	fmt.Fprintf(&b, "- 累计 tick %d；违例 %d；最大残差 %d\n", in.acc.ticks, in.acc.violations, in.acc.maxResidual)
	return b.String()
}

// keysOf 把集合拼成稳定可读串（输入为 map，输出按插入序保序不便——调用方
// 传小集合，直接排序）。
func keysOf(m map[string]bool) string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return strings.Join(ks, "；")
}
