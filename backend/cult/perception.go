package cult

// perception.go —— 感知解析器（M1 WP-2；stage_3 §42-44，stage_2 §22-25）。
//
// 本文件是感知隔离律的执行点（M1 计划 §0 铁律 1）：Observe 的输出只允许
// 定性词、粗档位与 band 序号——任何真值（qi / load / pressure / stability
// 数值）不得出现在 Observation 的任何字段或其 JSON 序列化结果里。
// perception_test.go 用"序列化后全文搜数字"的测试锁死这条律。
// 任何绕过本文件直读真值喂给 LLM / 玩家的代码路径都是 bug。
//
// 措辞模型（stage_3 §42-43 + 设计稿"感知映射 · 世界真值 → 屏上文字"表）：
//
//	PerceivedSignalStrength s = 真值 × CharacterPerception × Familiarity  (§42)
//	Familiarity = 0.5 + 0.5 × confidence
//
// v0.1 以神识 acuity 代替 confidence（acuity 1/2/3 → 0.50/0.75/1.00）；
// 0.5+0.5×KnowledgeConfidence 的知识联动留 WP-4，届时只需改 familiarity
// 一处（接口已按此留缝）。措辞 tier = acuity：
//
//	tier 0（acuity 1，初见）：只有钝化的感觉词——§43 低感知示例
//		"0.70 只感受到有些不适"级的措辞风格；
//	tier 1（acuity 2）：stage_3 §43 原文措辞；
//	tier 2（acuity 3，熟练）：设计稿"神识三层（熟练）"列，附"约几成"
//		级近似提示（全部汉字数字，不留真值痕迹；铁律 1 允许的粗档位）。
//
// 压感取 §47 边界交换后即时负载比（PostIntakeRatio²）——"胀"是灵气
// 涌入瞬间的感受（§47/§48 叙事口径）；世界尚未推进时用现场状态计算。
// PerceivedSignalStrength 允许措辞向下静默，但最多两档
// （felt ≥ 真值档-2）：低感知是对危险的钝化，不是全盲——剧痛不可能
// 被报成"轻微充盈"（§19 损伤机制也要求强压必然可感）。
//
// 本文件读取 World，并推进 World 上的私有感知状态（w.sensory，
// percept.go 的滞回记忆——每 Tick 备忘一次，同一 Tick 反复观察结果
// 深度相等，同 seed 重放逐位一致）；不写任何引擎状态，Advance 的
// 结果与是否观察无关。全程定点整数，无浮点。
//
// 流水线（境界文档 §8/§17）：真值 → PerceptToken（percept.go：趋势、
// 滞回、痛感分型、炼化档）→ 本文件的确定性模板渲染 → 定性文本。

// ---------------------------------------------------------------------------
// 词表（集中定义；stage_2 §25 类型词 + 设计稿"状态词表"五档）
// ---------------------------------------------------------------------------

// elementNames 是五行汉字名（下标顺序 §2.1）。
var elementNames = [ElementCount]string{"木", "火", "土", "金", "水"}

// pressureWords[band][tier] 是压力措辞（stage_3 §43 六档 × 神识三级）。
// tier 1 列即 §43 原文；tier 2 列采设计稿"神识三层（熟练）"列，
// 括号内为"约几成"级近似提示（汉字数字）。
var pressureWords = [6][3]string{
	{"无明显感觉", "无明显感觉", "空明，可容新纳（约莫一成）"},
	{"似有微涨", "轻微充盈", "渐盈（约三成）"},
	{"有些充盈", "明显胀感", "胀，然尚可续引（约五成）"},
	{"有些不适", "强烈胀感·不适", "满胀，阻力大增（约七成）"},
	{"疼痛", "疼痛", "灼痛，脉将不堪（约九成）"},
	{"剧痛·恐有失稳之患", "剧痛·恐有失稳之患", "过载！有撕裂之危（已超限）"},
}

// stabilityWords 是稳固五档（契约词表为准；stage_3 §44 六级映射：
// ≥0.90 稳如磐石 / 0.75~0.90 稳固 / 0.60~0.75 微澜 / 0.40~0.60 不稳 /
// 接近失控与结构崩溃并入"将溃"——契约词表只有五档，溃后与将溃在
// 感知上同级）。
var stabilityWords = []string{"稳如磐石", "稳固", "微澜", "不稳", "将溃"}

// opennessWords 是开度五档（契约词表；stage_3 §23 openness ∈ [0,1]）。
var opennessWords = []string{"闭", "微开", "半开", "大开", "洞开"}

// damageWords 是损伤五档（契约词表；stage_3 §19-20 damage ∈ [0,1]）。
var damageWords = []string{"无损", "隐伤", "作痛", "重创", "脉裂"}

// envQiWords 是环境灵气五档（设计稿状态词表：微茫→沛然）。
// 阈值为环境灵气密度（量，千分位）的 v0.1 实验值（stage_3 §52）。
var envQiWords = []struct {
	limit int64
	word  string
}{
	{100, "微茫"},
	{500, "稀薄"},
	{2000, "流转"},
	{6000, "浓郁"},
	{1 << 62, "沛然"},
}

// envActivityWords 是环境活性五档（同一词表，标度比率 ×1000）。
var envActivityWords = []struct {
	limit int64
	word  string
}{
	{100, "微茫"},
	{400, "稀薄"},
	{700, "流转"},
	{900, "浓郁"},
	{1 << 62, "沛然"},
}

// natureVague 是 acuity 2 的 nature 推断词（stage_2 §25：推断性措辞，
// 不作肯定判断——五行归属留待 WP-4 知识系统"已验证"）。
var natureVague = [ElementCount]string{
	"似有生发之意", "似有炽烈之性", "似有厚朴之意", "似有锐利之芒", "似有流转之意",
}

// natureTentative 是 acuity 3 的 nature 措辞（设计稿五行倾向词表
// "似偏木性（未证实）"档；"已验证"升级属知识系统，WP-4）。
var natureTentative = [ElementCount]string{
	"似偏木性（未证实）", "似偏火性（未证实）", "似偏土性（未证实）", "似偏金性（未证实）", "似偏水性（未证实）",
}

// 感受词（stage_2 §25 sensation：温热/清凉/胀/疼痛/阻塞/震荡）由
// PerceptToken（percept.go）的时间分析 + 滞回档驱动；热词只看热负荷
// （境界文档 §7.2/§9），不看属性名。阈值均为 v0.1 实验值（§52）：
const (
	// shockFeel：§15 FlowShock ≥ 0.2 时有震荡感。
	shockFeel = 200
	// blockedFeel：§7 反压 < 0.4（上一 Tick 负载比 > 1.22）时牵引受阻。
	blockedFeel = 400
	// flowEps：流量低于此值（量，千分位）视为停滞。
	flowEps = 50
	// natureMinLoad：负载低于此值时辨不出五行倾向（§24 Detection 以下）。
	natureMinLoad = 50
	// natureAcuity 是 nature 词解锁的神识（§24 Recognition 随神识增强开放）。
	natureAcuity = 2
	// maxMuteBands 是 PerceivedSignalStrength 允许的最大静默档数。
	maxMuteBands = 2
)

// ---------------------------------------------------------------------------
// 观察契约（M1 计划 §3.1，WP-3 消费，双方不得偏离）
// ---------------------------------------------------------------------------

// AcupointSense 是一个穴位的定性感知。除 PressureBand（band 序号，供
// UI 色带）外全部是词——没有任何真值字段。
type AcupointSense struct {
	Name         string   // 穴位名
	Feelings     []string // stage_2 §25 sensation 词（温热/胀/痛/阻滞…；[0] 为压力措辞）
	Flow         string   // stage_2 §25 flow 词（进入/停滞/加速…）
	Nature       []string // stage_2 §25 nature 词（生发/炽烈…——推断性措辞）
	Openness     string   // 状态词表五档：闭/微开/半开/大开/洞开
	Stability    string   // 状态词表五档：稳如磐石/稳固/微澜/不稳/将溃
	Damage       string   // 状态词表五档：无损/隐伤/作痛/重创/脉裂
	PressureBand int      // §43 压力档 0-5——感知后档（felt，所见即所得；与 Feelings[0] 同档）
}

// EnvSense 是环境的逐五行定性感知（微茫→沛然）。
type EnvSense struct {
	Qi       []string
	Activity []string
}

// Observation 是一次感知的全部输出。序列化后除 Tick 与 PressureBand
// （序号）外不含任何数字——感知隔离律（M1 计划 §0 铁律 1）。
type Observation struct {
	Tick   int64
	Env    EnvSense
	Points []AcupointSense
}

// Observe 把 World 真值翻译成定性感知。
// acuity 是神识 1..3：1=初见（钝化措辞、辨不出五行），2=§43 原文措辞、
// nature 推断词解锁，3=熟练措辞（"约几成"近似提示）。越界截到 1..3。
// 纯函数：不写 World；同状态同 acuity 两次调用结果深度相等。
func Observe(w *World, acuity int) *Observation {
	if acuity < 1 {
		acuity = 1
	}
	if acuity > 3 {
		acuity = 3
	}
	tier := acuity - 1
	fam := familiarityMilli(acuity)
	perception := fpClamp(w.Character.Perception, 0, One)
	tokens := sensoryTokens(w)

	obs := &Observation{
		Tick: w.Tick,
		Env: EnvSense{
			Qi:       make([]string, ElementCount),
			Activity: make([]string, ElementCount),
		},
	}
	for e := 0; e < ElementCount; e++ {
		obs.Env.Qi[e] = elementNames[e] + "气" + tieredWord(qiBand(w.Env.Qi[e]), tier, envQiWords)
		obs.Env.Activity[e] = elementNames[e] + "性" + tieredWord(activityBand(w.Env.Activity[e]), tier, envActivityWords)
	}

	for i := range w.Character.BodyGraph.Acupoints {
		a := &w.Character.BodyGraph.Acupoints[i]
		obs.Points = append(obs.Points, observePoint(w, a, i, tier, fam, perception, &tokens[i]))
	}
	return obs
}

// observePoint 把一个穴位的 PerceptToken 渲染成定性感知（§18 模板：
// 压感措辞 → 身体感觉 → 流动情况 → 疼痛/危险 → 趋势；单穴单条流，
// 不碎片化）。
func observePoint(w *World, a *Acupoint, idx, tier int, fam, perception int64, tok *pointToken) AcupointSense {
	// §47 口径的即时压感：吸收后负载比的平方。世界未推进时（无报告）
	// 用现场负载计算——同一公式（§14），同一标度。
	pressure := int64(0)
	if w.Last != nil && idx < len(w.Last.Points) {
		r := w.Last.Points[idx]
		pressure = fpMul(r.PostIntakeRatio, r.PostIntakeRatio)
	} else {
		lr := loadRatio(a, a.load())
		pressure = fpMul(lr, lr)
	}

	// §42 PerceivedSignalStrength + §43 分档，最多静默 maxMuteBands 档。
	// PressureBand 一律取感知后档 felt（所见即所得）：低感知角色看到的
	// 色带与其措辞同档，不得经 band 旁路泄真值档（真值档 band 只在此处
	// 参与静默上限计算，不出 Observation）。
	band := pressureBand(pressure)
	felt := pressureBand(fpMul(pressure, fpMul(perception, fam)))
	if band-felt > maxMuteBands {
		felt = band - maxMuteBands
	}

	sense := AcupointSense{
		Name:         a.Name,
		PressureBand: felt,
		Feelings:     []string{pressureWords[felt][tier]},
		Flow:         "停滞",
	}

	// 温度感只来自热负荷（§9/§7.2：热来自 ThermalLoad 的滞回档，不看
	// 属性名——同构成不同热负荷必然不同热词）。热负荷是穴位累积状态，
	// 无论世界是否推进过都可感。
	if tok.Thermal.level >= 1 {
		sense.Feelings = append(sense.Feelings, thermalWords[tok.Thermal.level])
	}

	// TickReport 变化量 → sensation / flow 词。
	if w.Last != nil && idx < len(w.Last.Points) {
		r := w.Last.Points[idx]

		// 震荡感来自流量冲击（§15）。
		if r.FlowShock >= shockFeel {
			sense.Feelings = append(sense.Feelings, "震荡")
		}

		// 阻滞感来自反压（§7.1：上一 Tick 超载 → 本 Tick 牵引阻力大增，§48）。
		if r.BackPressure < blockedFeel {
			sense.Feelings = append(sense.Feelings, "阻滞")
		}

		sense.Flow = flowWord(a, &r)
	}

	// 痛感分型（§11.2）：痛感强度档 + 主导成因措辞。
	if tok.Kind >= painDistension && tok.Pain.level >= 1 {
		sense.Feelings = append(sense.Feelings, painKindWords[tok.Kind-1][tok.Pain.level])
	}

	// 局部反馈耦合感知（0.7.0，评审稿 §22 感知映射）：已内化的「来流 →
	// 续推」以定性词显形——零数字零机器词，且只在耦合已成形、该转移确有
	// 来流时出现（感知隔离律：未被实验揭示的结构不预 render）。
	if w.Last != nil && idx < len(w.Last.Points) {
		if word := feedSenseWord(a); word != "" {
			sense.Feelings = append(sense.Feelings, word)
		}
	}

	// 炼化三档感知词（修订包 Part F）：只在有存气时渲染（空穴无"驳杂"可言）。
	if _, load := dominantElement(a.Qi); load >= natureMinLoad {
		sense.Feelings = append(sense.Feelings, refineWords[tok.Refine])
	}

	// 趋势（§14，单条）与复合模式（§16）。
	if w.Last != nil {
		if tw := trendWord(tok); tw != "" {
			sense.Feelings = append(sense.Feelings, tw)
		}
	}
	for _, p := range tok.Patterns {
		sense.Feelings = append(sense.Feelings, patternWords[p])
	}

	// §24 Recognition：神识不足时只有 Detection，辨不出五行倾向
	//（token 的 nature 签名由信号层判定，这里只过神识门槛）。
	if acuityOf(tier) >= natureAcuity && tok.Nature >= 0 {
		if acuityOf(tier) >= 3 {
			sense.Nature = []string{natureTentative[tok.Nature]}
		} else {
			sense.Nature = []string{natureVague[tok.Nature]}
		}
	}

	sense.Openness = opennessWord(a.opennessMilli())
	sense.Stability = stabilityWord(a.Stability)
	sense.Damage = damageWord(a.Damage)
	return sense
}

// flowWord 由流量变化量生成 flow 词（stage_2 §25）。
// 优先级：停滞 → 加速 → 进入 → 逸散/汇聚 → 流转。
// 判定只用两个量：Intake（本 Tick 意识驱动纳入增量，§7 边界交换的主动
// 吐纳口径）与 CurrentFlow（本 Tick 经脉总流量 = 流出 Actual + 流入
// Delivered，§8/§15）——AcupointReport 不含逐经脉明细，穴位口径足够定性。
// 被动交换基线（ExchangeIn）不进本判定：自然吐纳不成"进入"之感。
func flowWord(a *Acupoint, r *AcupointReport) string {
	intake := int64(0)
	for e := 0; e < ElementCount; e++ {
		intake += r.Intake[e]
	}

	if intake <= flowEps && r.CurrentFlow <= flowEps {
		return "停滞"
	}
	// 流量骤变（无论增减）在初感上皆如奔涌——§15 冲击的体验面。
	if r.FlowShock >= shockFeel && r.CurrentFlow > flowEps {
		return "加速"
	}
	if intake > flowEps && r.CurrentFlow <= intake/2 {
		return "进入" // 以环境吸收为主
	}
	if r.CurrentFlow > flowEps {
		if a.Retention < 500 {
			return "逸散" // 保留度低，存不住——散逸多于留存
		}
		return "汇聚"
	}
	return "流转"
}

// ---------------------------------------------------------------------------
// 档位函数（真值 → band 序号；band 序号是允许的输出）
// ---------------------------------------------------------------------------

// pressureBand 返回 stage_3 §43 压力档 0-5（千分位压力）：
//
//	<0.25 → 0；0.25~0.45 → 1；0.45~0.65 → 2；0.65~0.85 → 3；
//	0.85~1.0 → 4；>1.0 → 5。
func pressureBand(p int64) int {
	switch {
	case p < 250:
		return 0
	case p < 450:
		return 1
	case p < 650:
		return 2
	case p < 850:
		return 3
	case p <= 1000:
		return 4
	default:
		return 5
	}
}

// stabilityBand 返回稳固档 0-4（§44 六级 → 契约五档，见 stabilityWords）。
func stabilityBand(s int64) int {
	switch {
	case s >= 900:
		return 0
	case s >= 750:
		return 1
	case s >= 600:
		return 2
	case s >= 400:
		return 3
	default:
		return 4
	}
}

// opennessBand 返回开度档 0-4（openness 千分位；阈值 v0.1 实验值）。
func opennessBand(o int64) int {
	switch {
	case o < 250:
		return 0
	case o < 500:
		return 1
	case o < 750:
		return 2
	case o < 900:
		return 3
	default:
		return 4
	}
}

// damageBand 返回损伤档 0-4（damage 千分位；0 无损，阈值 v0.1 实验值）。
func damageBand(d int64) int {
	switch {
	case d <= 0:
		return 0
	case d < 250:
		return 1
	case d < 500:
		return 2
	case d < 750:
		return 3
	default:
		return 4
	}
}

// qiBand 返回环境灵气档 0-4。
func qiBand(q int64) int {
	for i, w := range envQiWords {
		if q < w.limit {
			return i
		}
	}
	return len(envQiWords) - 1
}

// activityBand 返回环境活性档 0-4。
func activityBand(v int64) int {
	for i, w := range envActivityWords {
		if v < w.limit {
			return i
		}
	}
	return len(envActivityWords) - 1
}

// ---------------------------------------------------------------------------
// 措辞与辅助
// ---------------------------------------------------------------------------

// stabilityWord / opennessWord / damageWord 是契约五档词。
func stabilityWord(s int64) string { return stabilityWords[stabilityBand(s)] }
func opennessWord(o int64) string  { return opennessWords[opennessBand(o)] }
func damageWord(d int64) string    { return damageWords[damageBand(d)] }

// tieredWord 把 band 映射到 tier 档措辞。tier 0（初见）把五档收敛为
// 三档（微茫/流转/沛然）——§24：Measurement 随神识逐步开放。
func tieredWord(band, tier int, table []struct {
	limit int64
	word  string
}) string {
	n := len(table)
	if tier == 0 && n == 5 {
		switch {
		case band <= 1:
			band = 0
		case band <= 3:
			band = 2
		default:
			band = 4
		}
	}
	return table[band].word
}

// familiarityMilli 返回 Familiarity（千分位）＝0.5+0.5×confidence。
// v0.1：confidence 以神识代替（acuity 1/2/3 → 0/0.5/1）。
// WP-4 接入 KnowledgeConfidence 时只改此函数（0.5+0.5×confidence 的
// 联动公式不变，stage_3 §42）。
func familiarityMilli(acuity int) int64 {
	conf := int64(acuity-1) * 500 // 0 / 500 / 1000
	if conf > One {
		conf = One
	}
	return 500 + conf/2
}

// acuityOf 把 tier 还原为 acuity（1..3）。
func acuityOf(tier int) int { return tier + 1 }

// dominantElement 返回储存灵气最多的元素（并列取下标最小者，确定性）
// 与总负载。
func dominantElement(qi [ElementCount]int64) (int, int64) {
	dom, load := 0, int64(0)
	for e := 0; e < ElementCount; e++ {
		load += qi[e]
		if qi[e] > qi[dom] {
			dom = e
		}
	}
	return dom, load
}
