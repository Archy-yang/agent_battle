package ctmatch

// partj_test.go —— Part J 被动环流测试（docs/specs/2026-09-22-partj-passive-
// circulation.md J7 测试计划 T-J1/T-J2/T-J3/T-J6）。
//
// T-J4/T-J5 在 cult 包（cult/partj_test.go，守恒扩项与金手指是引擎层口径）。
//
// 全部测试只用真实 tick 流驱动（协议 Runtime 行环 → 跑满/停止 → 松手），
// 不人工喂 FoundationSample——这正是 T-J6 的验收口径。

import (
	"fmt"
	"strings"
	"testing"

	"agentbattle/backend/cult"
)

// ---------------------------------------------------------------------------
// 共用：主环行环功法（SpikeWorld 拓扑：丹田→土府→中庭→阳枢→神庭→青关→丹田）
// ---------------------------------------------------------------------------

// partjRingYAML 生成一份"纳气 + 全环顺流"的行环功法，elem 为 "" 时混合
// 五均分、否则单行；流量 amount，跑 cycles 个周天（cadence 1、同息并行 →
// 一 Tick 一周天）。
func partjRingYAML(elem string, amount, cycles int64) string {
	ring := [][2]string{
		{"下丹田", "土府"},
		{"土府", "中庭"},
		{"中庭", "阳枢"},
		{"阳枢", "神庭"},
		{"神庭", "青关"},
		{"青关", "下丹田"},
	}
	var b strings.Builder
	fmt.Fprintf(&b, "protocol: zhutian_partj_v1\nactions:\n")
	fmt.Fprintf(&b, "  - absorb: {target: 青关, level: trace, duration: 1, parallel: 1}\n")
	for _, e := range ring {
		fmt.Fprintf(&b, "  - guide: {from: %s, to: %s, element: \"%s\", amount: %d, duration: 1, parallel: 1}\n",
			e[0], e[1], elem, amount)
	}
	fmt.Fprintf(&b, "cadence: 1\ncycles: %d\nvalidated_range:\n", cycles)
	for _, e := range ring {
		fmt.Fprintf(&b, "  - channel: [%s, %s]\n    element: \"%s\"\n    min: 0\n    max: %d\n", e[0], e[1], elem, amount)
	}
	return b.String()
}

// tickTotalFlow 汇总一份报告的实际流量（主动 + 被动；被动项在 Part J 后
// 记入 TransferReport.Passive，并同步计入 Actual 便于统一口径）。
func tickTotalFlow(rep *cult.TickReport) int64 {
	if rep == nil {
		return 0
	}
	var s int64
	for ti := range rep.Transfers {
		for e := 0; e < cult.ElementCount; e++ {
			s += rep.Transfers[ti].Actual[e]
		}
	}
	return s
}

// runRingProtocol 建会话、预填丹田、载入并启动行环功法，推进 cycles 个
// Tick（Runtime 路径，ControlActive=true）。返回会话（协议恰好跑满，active 已清）。
func runRingProtocol(t *testing.T, seed uint64, amount, cycles int64) *CultSession {
	t.Helper()
	w := SpikeWorld()
	w.Character.BodyGraph.Acupoints[0].Qi[cult.Wood] = 60000 // 预填丹田（引气已成的测试前提）
	s := NewSession(w, seed, nil)
	p, err := Compile([]byte(partjRingYAML("wood", amount, cycles)), &s.World.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	s.LoadProtocol(p)
	Tick(s, nil) // 静息观测基准（StartProtocol 前置条件需要）
	if err := s.StartProtocol(p.Name); err != nil {
		t.Fatalf("start: %v", err)
	}
	for i := int64(0); i < cycles; i++ {
		res := Tick(s, nil)
		if !res.Advanced {
			t.Fatalf("行环 tick %d 未推进: %+v", i, res.Events)
		}
	}
	if s.ActiveProtocol() != "" {
		t.Fatalf("协议应已跑满: active=%q", s.ActiveProtocol())
	}
	return s
}

// ---------------------------------------------------------------------------
// T-J1 死锁回归：行环 N 轮 → 全松手，松手窗均流随 N 单调上升。
// 旧引擎（传输全由 Requests 驱动）松手后恒无流——本测试在修复前必须 FAIL。
// ---------------------------------------------------------------------------

func TestPartJDeadlockRegression(t *testing.T) {
	// amount 取 500（0.8.0 热预算内档）：木单行 2000/元素在引气期新手身体
	// 上是火候过猛——0.8.0 热负荷应变（B7 环经济学裁决）下土府在首个
	// 教学窗内即走火（实测 TL~1040 = 感知「灼热」档，稳定 14 tick 内
	// 崩穿）。本命题变量是「松手均流随轮数单调上升」（被动环流成长），
	// 与强度无关，故收进热安全档；2000 档的走火本身是 0.8.0 的预期世界
	// 语义，由 cult 热负荷法则测试覆盖。
	const (
		amount       = 500
		releaseTicks = 40
	)
	// 轮数取 100/300/900：amount 收进热预算后成长带前移，300 轮已近饱和
	//（实测 300/900/1800 → SS 870/1000/1000，两点钳顶失去分辨力，同
	// §SB「钳 1 失去分辨率」教训）——测量点随之内移，命题（单调上升）
	// 不变。
	rounds := []int64{100, 300, 900}
	means := make([]int64, len(rounds))
	sustains := make([]int64, len(rounds))
	for k, n := range rounds {
		s := runRingProtocol(t, 11, amount, n)
		var sum int64
		for i := 0; i < releaseTicks; i++ {
			res := Tick(s, nil)
			sum += tickTotalFlow(res.Report)
			if m := s.Foundation.Metrics().SelfSustain; m > sustains[k] {
				sustains[k] = m
			}
		}
		means[k] = sum / releaseTicks
	}
	t.Logf("松手窗均流（N=300/900/1800）：%d / %d / %d", means[0], means[1], means[2])
	t.Logf("松手期 SelfSustain 峰值（千分位）：%d / %d / %d", sustains[0], sustains[1], sustains[2])
	if !(sustains[0] < sustains[1] && sustains[1] < sustains[2]) {
		t.Errorf("T-J1: SelfSustain 应随行环轮数 N 单调上升，got %v", sustains)
	}
	if means[2] <= 0 {
		t.Errorf("T-J1: 松手后无任何气流（死锁未修）：N=1800 松手均流 = %d", means[2])
	}
}

// ---------------------------------------------------------------------------
// T-J2 断环不自持：主环砍一边成树，松手后流衰减至 ~0（主环物理必然性）。
// ---------------------------------------------------------------------------

func TestPartJTreeDoesNotSelfSustain(t *testing.T) {
	const (
		pushTicks    = 200
		releaseTicks = 60
		amount       = 300
	)

	// 环世界 / 树世界（partjWorld 六穴主环，砍掉 中庭→阳枢 一边）。
	// 驱动层直接下发 Ops（协议引用被砍经脉将无法编译，正好说明树上
	// 行环功法不存在）。世界见 partjWorld：丹田已满、诸穴 LR≈0.8。
	buildWorld := func(cut bool) *cult.World {
		w := PartJCalibrationWorld()
		if !cut {
			return w
		}
		g := &w.Character.BodyGraph
		for i, ch := range g.Channels {
			if ch.From == 3 && ch.To == 4 { // 中庭→阳枢
				g.Channels = append(g.Channels[:i], g.Channels[i+1:]...)
				break
			}
		}
		return w
	}

	// pushOps 主环稳态轮：全环混合等量缓推（300/元素，与 T-J6 行环功法同
	// 档——重推首 Tick 的 FlowShock 会打穿稳定性，非本命题变量）。
	pushOps := func(w *cult.World, cut bool) cult.Ops {
		g := &w.Character.BodyGraph
		var reqs []cult.FlowRequest
		for c := range g.Channels {
			var a [cult.ElementCount]int64
			for e := range a {
				a[e] = amount
			}
			reqs = append(reqs, cult.FlowRequest{Channel: c, Amount: a})
		}
		return cult.Ops{Requests: reqs}
	}

	releaseMean := func(cut bool) int64 {
		w := buildWorld(cut)
		for i := 0; i < pushTicks; i++ {
			w.Ops = pushOps(w, cut)
			cult.Advance(w)
		}
		var sum int64
		for i := 0; i < releaseTicks; i++ {
			w.Ops = cult.Ops{}
			rep := cult.Advance(w)
			sum += tickTotalFlow(rep)
		}
		return sum / releaseTicks
	}

	ringMean := releaseMean(false)
	treeMean := releaseMean(true)
	t.Logf("松手窗均流：主环=%d，断环（树）=%d", ringMean, treeMean)
	if ringMean <= 0 {
		t.Fatalf("T-J2: 主环松手后应仍有被动环流，got %d", ringMean)
	}
	if treeMean*5 > ringMean {
		t.Errorf("T-J2: 断环成树后松手流应衰减至 ~0（应 < 主环 1/5）: tree=%d vs ring=%d", treeMean, ringMean)
	}
}

// ---------------------------------------------------------------------------
// T-J3 假自持拒绝：Protocol Runtime 持续运行时 ControlActive=true，
// 流量再大也不计入 SelfSustain 分子（Runtime 跑协议不是身体自持）。
// ---------------------------------------------------------------------------

func TestPartJRuntimeIsControlActive(t *testing.T) {
	// amount 取 500（0.8.0 热预算内档，归因同 TestPartJDeadlockRegression：
	// 木单行 2000/元素在引气期新手身体上走火，与本命题「受控流不计入
	// SelfSustain」的口径无关）。
	const cycles = 150
	w := SpikeWorld()
	w.Character.BodyGraph.Acupoints[0].Qi[cult.Wood] = 60000 // 预填丹田
	s := NewSession(w, 13, nil)
	p, err := Compile([]byte(partjRingYAML("wood", 500, cycles)), &s.World.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	s.LoadProtocol(p)
	Tick(s, nil)
	if err := s.StartProtocol(p.Name); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Runtime 行环期间：每 Tick 都有流量（主动+被动叠加），但 SelfSustain
	// 必须恒 0——Runtime 每 Tick 发主动请求（台账 ManualWork > 0），按
	// 控制活跃口径（主会话裁决 2026-09-26）即为控制，不存在"松手观测窗"。
	var maxFlow int64
	for i := 0; i < cycles; i++ {
		res := Tick(s, nil)
		if !res.Advanced {
			t.Fatalf("行环 tick %d 未推进: %+v", i, res.Events)
		}
		maxFlow += tickTotalFlow(res.Report)
		if m := s.Foundation.Metrics().SelfSustain; m != 0 {
			t.Fatalf("T-J3: Runtime 运行时 SelfSustain 应恒 0（tick %d got %d）", i, m)
		}
	}
	if maxFlow <= 0 {
		t.Fatal("T-J3: 测试前提失效——行环期间应有流量")
	}

	// 松手（协议已跑满）：纯被动环流出现，SelfSustain 才第一次有真实来源。
	seen := false
	for i := 0; i < 90; i++ {
		Tick(s, nil)
		if m := s.Foundation.Metrics().SelfSustain; m > 0 {
			seen = true
			break
		}
	}
	if !seen {
		t.Error("T-J3: 松手后被动环流应产生非零 SelfSustain（否则与死锁无异）")
	}
}

// ---------------------------------------------------------------------------
// T-J6 里程碑链：真实松手流程自然筑基——协议行环 → 跑满 → 松手，
// 触碰门槛与筑基事件全部由真实 tick 样本触发，无任何人工喂样。
// 世界用 Part J 标定世界（结构已"修行有成"：全开度高传导，见 partjWorld）。
// ---------------------------------------------------------------------------

func TestPartJNaturalFoundation(t *testing.T) {
	s := NewSession(PartJCalibrationWorld(), 7, nil)
	p, err := Compile([]byte(partjRingYAML("", 300, 600)), &s.World.Character.BodyGraph)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	s.LoadProtocol(p)
	Tick(s, nil)
	if err := s.StartProtocol(p.Name); err != nil {
		t.Fatalf("start: %v", err)
	}
	// 推动相：Runtime 在执行即 ControlActive（Part J J4），任何触碰/筑基
	// 事件都是假自持（T-J3 口径的流程级断言）；未知异常说明行环本身
	// 打伤了身体，场景即不成立。
	for i := 0; i < 900 && s.ActiveProtocol() != ""; i++ {
		res := Tick(s, nil)
		if !res.Advanced {
			t.Fatalf("行环 tick %d 未推进: %+v", i, res.Events)
		}
		for _, ev := range res.Events {
			switch ev.Kind {
			case EventFoundationTouched, EventFoundationEstablished:
				t.Fatalf("T-J6: 协议执行期出现 %s（假自持）: %+v", ev.Kind, ev)
			case EventUnknownAnomaly:
				t.Fatalf("T-J6: 行环期出现未知异常，场景不成立: %+v", ev)
			}
		}
	}
	if s.ActiveProtocol() != "" {
		t.Fatalf("T-J6: 协议未在 900 Tick 内跑满，场景不成立: active=%q", s.ActiveProtocol())
	}

	// 松手：协议已跑满，此后不再提交任何意图——纯被动环流。
	// 判定以会话级事实（Foundation.Established）为准；事件扫描用于
	// 核对叙事事件确在松手期发出。
	established := false
	for i := 0; i < 400 && !established; i++ {
		res := Tick(s, nil)
		for _, ev := range res.Events {
			if ev.Kind == EventFoundationEstablished {
				established = true
			}
		}
		if s.Foundation.Established {
			established = true
		}
	}
	mt := s.Foundation.Metrics()
	t.Logf("五量：Closure=%d RuleBalance=%d SelfSustain=%d DynStability=%d MinStab=%d",
		mt.Closure, mt.RuleBalance, mt.SelfSustain, mt.DynamicStability, mt.MinStability)
	if !established {
		t.Fatalf("T-J6: 真实松手流程未能自然筑基: %+v", mt)
	}
	if !s.Foundation.Touched {
		t.Error("T-J6: 筑基前应先触碰门槛")
	}
}
