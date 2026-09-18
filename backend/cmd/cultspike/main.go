package main

// cultspike —— M1 SPIKE 实验台（无 UI）。
//
// 命题：让一个真 LLM（ctmatch.LLMPilot）在完全不知道规则的情况下，仅凭
// cult.Observe 的定性感知，摸索出稳定的"吐纳"（吸收-引流循环）。
//
// 实验纪律（M1 计划 §0 铁律 1 + SPIKE 任务书）：
//   - LLM 的 prompt 与观察载荷永无公式、参数值、阈值数字；真值只进转录
//     文件供研究者查看，绝不进 LLM 观察载荷。
//   - 神识固定 acuity=1（初见措辞，钝化档）。
//   - 调用预算默认 40 次：初次征询 + 每 10 Tick 心跳征询 + 不适（压力档
//     ≥3 的感受）上升沿唤醒；其余 Tick 沿用上一组意图（分身入定不语）。
//   - 里程碑由程序按引擎真值判定（非 LLM 自述）：
//       M1 首次成功吸收：某 Tick 有穴位 qi 净增加，且该 Tick 无新增损伤；
//       M2 首次自主调整：出现不适档（band≥3）后 5 Tick 内吸收强度下降
//           或新增/改变引流；
//       M3 稳定吐纳：连续 ≥30 Tick 全程压力 band≤2、无新增损伤、
//           且吸收-引流形成循环（进气总量>0 且出流总量>0）；
//       FAIL：Tick 预算耗尽未达 M3。
//
// 用法：
//
//	set -a && source llm.env && set +a && go run ./cmd/cultspike -out /tmp/cultspike-run1.md
//	go run ./cmd/cultspike -selftest -out /tmp/cultspike-selftest.md
//	    （selftest：不调 LLM，以一组脚本意图验证世界可行性与里程碑判定）
//
// 本程序是世界构造的唯一定义点；LLM 的 prompt 在 ctmatch/llm.go。

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"agentbattle/backend/ctmatch"
	"agentbattle/backend/cult"
)

// ---------------------------------------------------------------------------
// 世界构造（真值表——只进转录头供研究者查证，绝不进 LLM 载荷）
// ---------------------------------------------------------------------------
//
// 设计目标：存在一条"新手粗档位即可达成"的稳定吐纳路径，但不给任何提示。
//   - 青关：主纳穴。开度大开、通量宽（8000），混合引流 large（每行 1280）
//     足以对冲 trace/small/medium 各档吸收的反压平衡——单穴"吸+引"即可闭环。
//   - 下丹田：巨容汇点（200000），引流去处，30+ Tick 内不会自己顶到不适档。
//   - 神庭：脆穴（容量 3000、开度 0.90）。在它上面用 large 吸收会迅速过载
//     受损——为 M2 的"不适→自主调整"提供真实触发点。
//   - 环境五行接近均衡（木略盛）：混合引流才可能对冲混合吸收，
//     避免单行淤积成为纯数值陷阱。
//   - 金门开度仅 0.30（近闭）：吸收前先感开合，是感知信息的真实用途。
func buildWorld() *cult.World {
	pt := func(name string, aff [cult.ElementCount]int64, cap, tp, ret int64, open, stab int64) cult.Acupoint {
		return cult.Acupoint{
			Name: name, Affinity: aff, Capacity: cap, Throughput: tp,
			// open 以千分位传入（800 = 0.8）；微标度换算 ×1000（1.0 = 1e6 微）。
			Retention: ret, Openness: open * 1000, Stability: stab,
			Activity: [cult.ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		}
	}
	all := func(v int64) [cult.ElementCount]int64 {
		return [cult.ElementCount]int64{v, v, v, v, v}
	}
	char := cult.Character{
		Affinity:   [cult.ElementCount]int64{1250, 800, 900, 750, 1100},
		Control:    700,
		Perception: 500,
		BodyGraph: cult.BodyGraph{
			Acupoints: []cult.Acupoint{
				pt("下丹田", all(1000), 200000, 20000, 1000, 800, 950),                                       // 0 汇点
				pt("青关", [cult.ElementCount]int64{1200, 750, 900, 850, 1000}, 10000, 8000, 400, 800, 950), // 1 主纳
				pt("阳枢", [cult.ElementCount]int64{1100, 900, 800, 800, 800}, 6000, 4000, 500, 500, 900),   // 2
				pt("土府", [cult.ElementCount]int64{800, 850, 1250, 750, 800}, 8000, 5000, 800, 650, 950),   // 3
				pt("金门", [cult.ElementCount]int64{800, 750, 800, 1300, 850}, 5000, 4000, 600, 300, 920),   // 4 近闭
				pt("玄泉", [cult.ElementCount]int64{900, 800, 800, 800, 1200}, 7000, 4000, 700, 700, 940),   // 5
				pt("神庭", [cult.ElementCount]int64{1050, 1050, 800, 800, 800}, 3000, 3000, 300, 900, 880),  // 6 脆
				pt("中庭", [cult.ElementCount]int64{1000, 900, 950, 900, 950}, 9000, 6000, 500, 750, 930),   // 7 中枢
			},
			Channels: []cult.Channel{
				{Name: "青关→下丹田", From: 1, To: 0, BaseConductance: 950, Throughput: 8000},
				{Name: "青关→中庭", From: 1, To: 7, BaseConductance: 700, Throughput: 6000},
				{Name: "中庭→下丹田", From: 7, To: 0, BaseConductance: 850, Throughput: 12000},
				{Name: "阳枢→中庭", From: 2, To: 7, BaseConductance: 700, Throughput: 6000},
				{Name: "土府→中庭", From: 3, To: 7, BaseConductance: 700, Throughput: 6000},
				{Name: "金门→中庭", From: 4, To: 7, BaseConductance: 600, Throughput: 5000},
				{Name: "玄泉→中庭", From: 5, To: 7, BaseConductance: 700, Throughput: 6000},
				{Name: "神庭→青关", From: 6, To: 1, BaseConductance: 600, Throughput: 5000},
			},
		},
	}
	env := cult.Environment{
		Qi:          [cult.ElementCount]int64{3600, 2400, 2400, 1800, 2400}, // 木略盛
		Activity:    [cult.ElementCount]int64{1000, 1000, 1000, 1000, 1000},
		Temperature: 1000,
	}
	return cult.NewWorld(char, env)
}

// ---------------------------------------------------------------------------
// 意图画像（M2 判定用：强度下降 / 改引流）
// ---------------------------------------------------------------------------

type intentProfile struct {
	absorbs map[string]ctmatch.Level // 穴位名 → 吸收档位
	guides  map[string]bool          // "from→to" 引流路径集合
}

func profileOf(intents []ctmatch.Intent) intentProfile {
	p := intentProfile{absorbs: map[string]ctmatch.Level{}, guides: map[string]bool{}}
	for _, it := range intents {
		switch it.Kind {
		case ctmatch.IntentAbsorb:
			p.absorbs[it.Acupoint] = it.Level
		case ctmatch.IntentGuide:
			p.guides[it.From+"→"+it.To] = true
		}
	}
	return p
}

// relaxedFrom 报告 cur 相对 prev 是否"松了"：任一原有吸收档位下调或取消，
// 或新增了先前没有的引流路径。
func (cur intentProfile) relaxedFrom(prev intentProfile) bool {
	for target, lvl := range prev.absorbs {
		now, ok := cur.absorbs[target]
		if !ok || now < lvl {
			return true
		}
	}
	for g := range cur.guides {
		if !prev.guides[g] {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// 校验（LLM 意图 → 会话层可落地）：引用不存在穴位/经脉或缺档位的意图
// 就地丢弃并记入转录，避免无效意图逐 Tick 刷 InvalidIntent 事件。
// ---------------------------------------------------------------------------

func sanitizeIntents(g *cult.BodyGraph, intents []ctmatch.Intent) ([]ctmatch.Intent, []string) {
	names := map[string]bool{}
	for _, a := range g.Acupoints {
		names[a.Name] = true
	}
	var out []ctmatch.Intent
	var drops []string
	drop := func(why string) { drops = append(drops, why) }
	for _, it := range intents {
		switch it.Kind {
		case ctmatch.IntentSense, ctmatch.IntentStop:
			out = append(out, it)
		case ctmatch.IntentInspect:
			if names[it.Acupoint] {
				out = append(out, it)
			} else {
				drop("inspect 未知穴位 " + it.Acupoint)
			}
		case ctmatch.IntentAbsorb:
			if !names[it.Acupoint] {
				drop("absorb 未知穴位 " + it.Acupoint)
				continue
			}
			if it.Level == ctmatch.LevelUnset {
				drop("absorb 缺档位 " + it.Acupoint)
				continue
			}
			out = append(out, it)
		case ctmatch.IntentGuide:
			if !names[it.From] || !names[it.To] {
				drop("guide 未知穴位 " + it.From + "→" + it.To)
				continue
			}
			if channelIdx(g, it.From, it.To) < 0 {
				drop("guide 无直接经脉 " + it.From + "→" + it.To)
				continue
			}
			if it.Level == ctmatch.LevelUnset {
				drop("guide 缺档位 " + it.From + "→" + it.To)
				continue
			}
			out = append(out, it)
		}
	}
	return out, drops
}

func channelIdx(g *cult.BodyGraph, from, to string) int {
	idx := map[string]int{}
	for i, a := range g.Acupoints {
		idx[a.Name] = i
	}
	f, ok1 := idx[from]
	t, ok2 := idx[to]
	if !ok1 || !ok2 {
		return -1
	}
	for c, ch := range g.Channels {
		if ch.From == f && ch.To == t {
			return c
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// 实验台
// ---------------------------------------------------------------------------

const m3Window = 30 // M3 稳定吐纳需要的连续 Tick 数

// heartbeatDue 是心跳节奏（第 3 轮起前载）：前 120 Tick 每 5 Tick 一询
// （探路期密集决策），之后每 40 Tick 一询；预算仍由 pilot 统一掐算。
func heartbeatDue(t int64) bool {
	if t <= 0 {
		return false
	}
	if t <= 120 {
		return t%5 == 0
	}
	return t%40 == 0
}

type spikeRun struct {
	sess     *ctmatch.CultSession
	w        *cult.World
	g        *cult.BodyGraph
	pilot    *ctmatch.LLMPilot
	out      *bufio.Writer
	maxTicks int64
	intents  []ctmatch.Intent

	prevQi     []int64
	prevDamage []int64

	// 里程碑（<0 = 未达成）
	m1, m2, m3 int64
	streak     int

	// M2 布防：不适档上升沿记快照，5 Tick 窗口内观察是否自主放松
	m2Armed   bool
	m2T0      int64
	m2Profile intentProfile

	prevHot     bool // 上一 Tick 是否有穴位处于不适档
	pendingWake bool // 下 Tick 开头做一次不适唤醒
	llmCalls    int
}

func main() {
	outPath := flag.String("out", "/tmp/cultspike-transcript.md", "转录输出文件")
	maxTicks := flag.Int("ticks", 600, "最多推进的 Tick 数")
	budget := flag.Int("calls", ctmatch.LLMBudgetDefault, "LLM 调用预算")
	selftest := flag.Bool("selftest", false, "脚本意图自检：不调 LLM，验证世界可行性与里程碑判定")
	flag.Parse()

	f, err := os.Create(*outPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "无法创建转录文件 %s: %v\n", *outPath, err)
		os.Exit(1)
	}
	out := bufio.NewWriter(f)
	defer func() {
		out.Flush()
		f.Close()
	}()

	w := buildWorld()
	var pilot *ctmatch.LLMPilot
	var scripted []ctmatch.Intent
	if *selftest {
		// 实验者校准路径：主纳穴少量吸收 + 混合大量引流（粗档位可达）。
		scripted = []ctmatch.Intent{
			{Kind: ctmatch.IntentAbsorb, Acupoint: "青关", Level: ctmatch.LevelSmall},
			{Kind: ctmatch.IntentGuide, From: "青关", To: "下丹田", Element: -1, Level: ctmatch.LevelLarge},
		}
	} else {
		pilot = ctmatch.NewLLMPilot()
		if pilot == nil {
			fmt.Fprintln(os.Stderr, "AGENTBATTLE_LLM_BASE_URL/_API_KEY/_MODEL 未配置（见 backend/llm.env），无法进行 SPIKE。可先跑 -selftest 验证世界。")
			os.Exit(1)
		}
		pilot.MaxCalls = *budget
	}

	sess := ctmatch.NewSession(w, 1, pilot) // seed=1：控制误差确定性
	sess.Observe = func(x *cult.World) *ctmatch.Observation {
		return cult.Observe(x, 1) // acuity=1：初见措辞（感知隔离律：观察只出定性词）
	}

	r := &spikeRun{
		sess: sess, w: w, g: &w.Character.BodyGraph, pilot: pilot,
		out: out, maxTicks: int64(*maxTicks),
		m1: -1, m2: -1, m3: -1,
		prevQi:     make([]int64, len(w.Character.BodyGraph.Acupoints)),
		prevDamage: make([]int64, len(w.Character.BodyGraph.Acupoints)),
	}
	if scripted != nil {
		r.intents = scripted
	}
	r.writeHeader(*selftest, *budget)

	obs := cult.Observe(w, 1)
	if pilot != nil {
		r.decide(obs, "初次征询", 0)
	}

	var m3Done int64 = -1
	for t := int64(0); t < r.maxTicks; t++ {
		if pilot != nil {
			switch {
			case r.pendingWake:
				r.pendingWake = false
				r.decide(obs, "不适唤醒", t)
			case heartbeatDue(t):
				r.decide(obs, "心跳征询", t)
			}
		}

		fmt.Fprintf(out, "\n### Tick %d\n", t)
		fmt.Fprintf(out, "- 观察：%s\n", obsLine(obs))
		fmt.Fprintf(out, "- 意图：%s\n", intentsLine(r.intents))

		res := ctmatch.Tick(sess, r.intents)
		if !res.Advanced || res.Report == nil {
			fmt.Fprintf(out, "- 本 Tick 引擎未推进（出界拒绝）——L2 场景不应发生，检查会话层\n")
			break
		}
		obs = res.Observation
		rep := res.Report

		if len(res.Events) > 0 {
			var evs []string
			for _, e := range res.Events {
				evs = append(evs, string(e.Kind)+":"+e.Reason)
			}
			fmt.Fprintf(out, "- 事件：%s\n", strings.Join(evs, "；"))
		} else {
			fmt.Fprintf(out, "- 事件：—\n")
		}

		newDamage, hot, inTotal, outTotal := r.absorbTruth(rep, obs)
		fmt.Fprintf(out, "- 真值：%s\n", truthLine(r.g, rep, inTotal, outTotal))

		// ---- 里程碑判定（程序按真值判定）----
		if r.m1 < 0 && inTotal > 0 && !newDamage {
			// inTotal>0 即有穴位 qi 净增加（进气即增），配合无新增损伤。
			r.m1 = t
		}
		if hot && !r.prevHot {
			r.m2Armed = true
			r.m2T0 = t
			r.m2Profile = profileOf(r.intents)
			if r.pilot != nil && r.pilot.BudgetLeft() > 0 {
				r.pendingWake = true
			}
		}
		r.prevHot = hot
		if !newDamage && !hot && inTotal > 0 && outTotal > 0 {
			r.streak++
		} else {
			r.streak = 0
		}
		if r.streak >= m3Window && m3Done < 0 {
			m3Done = t
			r.m3 = t - m3Window + 1
			fmt.Fprintf(out, "\n> **M3 达成**：自 Tick %d 起连续 %d Tick 稳定吐纳（band≤2、无新损、有进有出）。\n", r.m3, m3Window)
			break
		}
	}

	r.writeSummary(m3Done)
	fmt.Printf("转录已写入 %s\n", *outPath)
	fmt.Printf("M1=%s M2=%s M3=%s LLM调用=%d\n",
		milestoneStr(r.m1), milestoneStr(r.m2), milestoneStr(r.m3), r.llmCalls)
	if r.m3 < 0 {
		fmt.Println("判定：FAIL（Tick 预算耗尽未达 M3）")
	} else {
		fmt.Printf("判定：PASS（M3 于 Tick %d 达成）\n", r.m3)
	}
}

func milestoneStr(t int64) string {
	if t < 0 {
		return "未达成"
	}
	return fmt.Sprintf("Tick %d", t)
}

// decide 唤醒一次 LLM：喂观察，取回意图（校验后采纳）。全程记录进转录。
func (r *spikeRun) decide(obs *cult.Observation, why string, t int64) {
	if r.pilot == nil || r.pilot.BudgetLeft() <= 0 {
		return
	}
	r.llmCalls++
	fmt.Fprintf(r.out, "\n#### LLM 调用 #%d（%s；喂给它的观察 = Tick %d 的感知）\n", r.pilot.Calls+1, why, obs.Tick)
	data, _ := json.MarshalIndent(obs, "", " ")
	fmt.Fprintf(r.out, "```json\n%s\n```\n", data)

	ni := r.pilot.Decide(obs)
	if err := r.pilot.LastError; err != "" {
		fmt.Fprintf(r.out, "- 调用失败：%s\n", err)
		return
	}
	fmt.Fprintf(r.out, "- cognition：%s\n", r.pilot.LastCognition)

	sanitized, drops := sanitizeIntents(r.g, ni)
	for _, d := range drops {
		fmt.Fprintf(r.out, "- 丢弃意图：%s\n", d)
	}
	if len(sanitized) > 0 {
		// M2：不适快照后 5 Tick 窗口内的自主放松。
		if r.m2Armed && t > r.m2T0 && t <= r.m2T0+5 && r.m2 < 0 {
			if profileOf(sanitized).relaxedFrom(r.m2Profile) {
				r.m2 = t
				fmt.Fprintf(r.out, "- **M2 触发**：不适档后 %d Tick 内自主调整\n", t-r.m2T0)
			}
		}
		r.intents = sanitized
	}
	fmt.Fprintf(r.out, "- 新意图：%s\n", intentsLine(r.intents))
}

// absorbTruth 提取本 Tick 的关键真值：进气/出流总量、是否新增损伤、
// 是否有穴位处于不适档（观察口径 band≥3）。只供程序判定与转录。
func (r *spikeRun) absorbTruth(rep *cult.TickReport, obs *cult.Observation) (newDamage, hot bool, inTotal, outTotal int64) {
	for i := range rep.Points {
		p := &rep.Points[i]
		q := int64(0)
		for e := 0; e < cult.ElementCount; e++ {
			q += p.Qi[e]
			inTotal += p.Intake[e]
		}
		r.prevQi[i] = q
		if p.Damage > r.prevDamage[i] {
			newDamage = true
		}
		r.prevDamage[i] = p.Damage
	}
	for _, tr := range rep.Transfers {
		for e := 0; e < cult.ElementCount; e++ {
			outTotal += tr.Actual[e]
		}
	}
	for i := range obs.Points {
		if obs.Points[i].PressureBand >= 3 {
			hot = true
		}
	}
	return
}

// ---------------------------------------------------------------------------
// 转录渲染（定性观察一行流 + 真值摘要一行流）
// ---------------------------------------------------------------------------

func obsLine(obs *cult.Observation) string {
	var b strings.Builder
	b.WriteString("环境[" + strings.Join(obs.Env.Qi, "/") + "；" + strings.Join(obs.Env.Activity, "/") + "] ")
	for i, p := range obs.Points {
		if i > 0 {
			b.WriteString(" ")
		}
		feel := "—"
		if len(p.Feelings) > 0 {
			feel = strings.Join(p.Feelings, "/")
		}
		nature := ""
		if len(p.Nature) > 0 {
			nature = "|" + strings.Join(p.Nature, "/")
		}
		fmt.Fprintf(&b, "%s[%s|%s|%s|%s|%s%s]", p.Name, feel, p.Flow, p.Openness, p.Stability, p.Damage, nature)
	}
	return b.String()
}

func truthLine(g *cult.BodyGraph, rep *cult.TickReport, inTotal, outTotal int64) string {
	var b strings.Builder
	for i := range rep.Points {
		if i > 0 {
			b.WriteString(" ")
		}
		p := &rep.Points[i]
		q := int64(0)
		for e := 0; e < cult.ElementCount; e++ {
			q += p.Qi[e]
		}
		fmt.Fprintf(&b, "%s(qi=%d,稳=%d,损=%d)", g.Acupoints[i].Name, q, p.Stability, p.Damage)
	}
	fmt.Fprintf(&b, " | 进=%d 出=%d", inTotal, outTotal)
	return b.String()
}

func intentsLine(intents []ctmatch.Intent) string {
	if len(intents) == 0 {
		return "（无——静息）"
	}
	var parts []string
	for _, it := range intents {
		switch it.Kind {
		case ctmatch.IntentSense:
			parts = append(parts, "sense()")
		case ctmatch.IntentStop:
			parts = append(parts, "stop()")
		case ctmatch.IntentInspect:
			parts = append(parts, "inspect("+it.Acupoint+")")
		case ctmatch.IntentAbsorb:
			parts = append(parts, fmt.Sprintf("absorb(%s,%s)", it.Acupoint, levelName(it.Level)))
		case ctmatch.IntentGuide:
			el := "混合"
			if it.Element >= 0 {
				el = [...]string{"木", "火", "土", "金", "水"}[it.Element]
			}
			parts = append(parts, fmt.Sprintf("guide(%s→%s,%s,%s)", it.From, it.To, el, levelName(it.Level)))
		}
	}
	return strings.Join(parts, " ")
}

func levelName(l ctmatch.Level) string {
	switch l {
	case ctmatch.LevelTrace:
		return "trace"
	case ctmatch.LevelSmall:
		return "small"
	case ctmatch.LevelMedium:
		return "medium"
	case ctmatch.LevelLarge:
		return "large"
	}
	return "unset"
}

// ---------------------------------------------------------------------------
// 转录头尾
// ---------------------------------------------------------------------------

func (r *spikeRun) writeHeader(selftest bool, budget int) {
	fmt.Fprintf(r.out, `# cultspike 转录 —— M1 SPIKE（真 LLM 盲修定性吐纳）

- 模式：%s；Tick 预算 %d；LLM 调用预算 %d（初次 + 每 10 Tick 心跳 + 不适唤醒）
- 神识 acuity=1（初见措辞）；感知隔离律：LLM 载荷只含定性词/档位/band 序号，真值只在"真值"行供研究者查看
- 会话 seed=1（控制误差确定性噪声）；里程碑由程序按引擎真值判定：
  M1=首次成功吸收（有进气且本 Tick 无新增损伤）；M2=不适档(band≥3)后 5 Tick 内强度下降或改引流；
  M3=连续 ≥30 Tick 压力 band≤2、无新增损伤、有进有出；耗尽未达 M3 = FAIL

## 世界构造（研究者真值表，非 LLM 所见）

亲和/容量/通量/保留/开度/稳定均为引擎千分位标度（×1000，开度为微标度×1000 注记）：

| # | 穴位 | 亲和(木火土金水) | 容量 | 通量 | 保留 | 开度 | 稳定 |
|---|------|------------------|------|------|------|------|------|
`, map[bool]string{true: "selftest（脚本意图，不调 LLM）", false: "真 LLM"}[selftest], r.maxTicks, budget)
	for i, a := range r.g.Acupoints {
		fmt.Fprintf(r.out, "| %d | %s | %d/%d/%d/%d/%d | %d | %d | %d | %d.%02d | %d |\n",
			i, a.Name,
			a.Affinity[0], a.Affinity[1], a.Affinity[2], a.Affinity[3], a.Affinity[4],
			a.Capacity, a.Throughput, a.Retention,
			a.Openness/cult.MicroOne, (a.Openness%cult.MicroOne)*100/cult.MicroOne, a.Stability)
	}
	fmt.Fprintf(r.out, "\n经脉（有向，传导/通量）：")
	for _, ch := range r.g.Channels {
		fmt.Fprintf(r.out, " %s(%d/%d)；", ch.Name, ch.BaseConductance, ch.Throughput)
	}
	fmt.Fprintf(r.out, "\n环境灵气：木3600 火2400 土2400 金1800 水2400（活性全满）；角色亲和 1250/800/900/750/1100，Control 0.70，Perception 0.50。\n")
}

func (r *spikeRun) writeSummary(m3Done int64) {
	fmt.Fprintf(r.out, "\n\n## 里程碑判定（程序判定，非 LLM 自述）\n\n")
	fmt.Fprintf(r.out, "- M1 首次成功吸收：%s\n", milestoneStr(r.m1))
	fmt.Fprintf(r.out, "- M2 首次自主调整：%s", milestoneStr(r.m2))
	if r.m2 >= 0 {
		fmt.Fprintf(r.out, "（不适档出现于 Tick %d 后 %d Tick 内调整）", r.m2T0, r.m2-r.m2T0)
	}
	fmt.Fprintf(r.out, "\n- M3 稳定吐纳（连续 ≥%d Tick）：%s", m3Window, milestoneStr(r.m3))
	if r.m3 >= 0 {
		fmt.Fprintf(r.out, "（达成于 Tick %d）", m3Done)
	}
	fmt.Fprintf(r.out, "\n- LLM 调用次数：%d\n", r.llmCalls)
	if r.m3 < 0 {
		fmt.Fprintf(r.out, "\n**判定：FAIL —— %d Tick 耗尽未达 M3。**\n", r.maxTicks)
	} else {
		fmt.Fprintf(r.out, "\n**判定：PASS —— M3 于 Tick %d 达成（M1@%d, M2@%s）。**\n",
			m3Done, r.m1, milestoneStr(r.m2))
	}

	// 末尾真值快照（研究者视角）。
	if rep := r.w.Last; rep != nil {
		fmt.Fprintf(r.out, "\n末 Tick 真值快照：%s\n", truthLine(r.g, rep, 0, 0))
	}
	if r.pilot != nil && len(r.pilot.Notes) > 0 {
		fmt.Fprintf(r.out, "\n## 分身修行笔记（LLM cognition 原文，按时间序）\n\n")
		for _, n := range r.pilot.Notes {
			fmt.Fprintf(r.out, "> %s\n\n", n)
		}
	}
}
