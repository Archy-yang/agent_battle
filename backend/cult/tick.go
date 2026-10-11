package cult

// tick.go —— 每 Tick 的确定性结算（stage_3 §7-24）。
//
// 结算顺序固定：
//
//	§7  边界交换（势差版统一原语，0.6.0：吸收与逸散两条旧通路的并入；
//	    纳入方向 BackPressure 用上一 Tick 交换后负载比，§47→§48 链）
//	§8  灵气传输（传导/容量/控制误差 ε）
//	§9  传输损耗（50% Diffuse、50% 热 → 目标穴位 LocalStrain）
//	§10 相生转化（快照语义：本阶段所有转化量按阶段开始时的 Qi 计算）
//	§11 相克压活性（固定顺序、活值语义）
//	§12 活性恢复
//	§13 逸散（0.6.0 起并入 §7 泄出方向，不再单列）
//	§14 压力（本 Tick 结束后负载）
//	§15 FlowShock
//	§16 Compatibility
//	§17 Strain
//	§18 Stability
//	§19 Damage
//	§21 自然恢复
//	§22 Adaptation
//	§23 Openness 成长
//	§24 经脉 Adaptation
//
// 全程定点整数，无浮点；同 seed 同 Ops 流重演逐位一致。

// Ops 是调用方（Runtime / 测试）交给引擎的本 Tick 操作。
// 引擎不解释意图——它只知道每个穴位以多大强度驱动纳气、每条经脉请求多少流量。
type Ops struct {
	// Intensity[i] 是穴位 i 的功法强度 ∈ (0,1000]；0 表示本 Tick 无意识
	// 驱动（仅保留被动边界交换基线，0.6.0）。
	// 长度必须与穴位数一致（nil 视为全 0）。
	Intensity []int64
	// Requests 是本 Tick 的经脉流量请求，按给出顺序结算。
	Requests []FlowRequest
}

// FlowRequest 是一条经脉上按五行请求的流量（量，千分位）。
type FlowRequest struct {
	// Channel 是经脉在 BodyGraph.Channels 中的下标。
	Channel int
	// Amount[e] 是对元素 e 的请求流量；0 表示不请求。
	Amount [ElementCount]int64
	// Reverse 标记逆流传输（修订包 A2）：沿天然向反向行走。
	// 唯有核点可发起——核点判据自 0.5.0 起为「驱动容量显著高于其余
	// 节点者」（CorePointOf，定稿阶段二范围 1 / 裁决①；丹田的特殊性
	// 落在参数画像，宪法 §8）；其余一律被引擎拒绝。
	Reverse bool
}

// World 是一次修炼仿真的全部状态。
type World struct {
	Character Character
	Env       Environment

	// RuleVersion 是产生本状态的底层规则版本（宪法 §18；cult.RuleVersion）。
	RuleVersion string

	// NoiseSeed 是控制误差 ε 的确定性噪声种子（stage_3 §8.2）。
	NoiseSeed uint64

	// Tick 是已完成的 Tick 数。
	Tick int64

	// Ops 是调用方为下一 Tick 设置的操作。
	Ops Ops

	// Last 是最近一次 Advance 的报告；尚未推进时为 nil。
	Last *TickReport

	// sensory 是感知层的滞回状态存档（境界文档 §14-15；percept.go）：
	// 各穴位的滚动窗口、滞回档位与档位时长。私有中间态——不进入任何
	// 运算、不被序列化（encoding/json 只编导出字段），但同 seed 重放
	// 时随相同的观察序列逐位一致（DeepEqual 测试锁死）。
	sensory *sensoryState
}

// NewWorld 组装一个最小世界：指定角色与环境，噪声种子默认 1。
//
// 归一化：适应字段（Acupoint.Adaptation、Channel.ChannelAdaptation）
// 若为 0 视作未初始化，重置为 1.0（微标度）——0 亲和会使传导/转化
// 全部失效，不属于合法初值。Openness=0、Activity=0 是合法状态，不动。
func NewWorld(char Character, env Environment) *World {
	for i := range char.BodyGraph.Acupoints {
		a := &char.BodyGraph.Acupoints[i]
		for e := 0; e < ElementCount; e++ {
			if a.Adaptation[e] == 0 {
				a.Adaptation[e] = MicroOne
			}
		}
	}
	for c := range char.BodyGraph.Channels {
		if char.BodyGraph.Channels[c].ChannelAdaptation == 0 {
			char.BodyGraph.Channels[c].ChannelAdaptation = MicroOne
		}
	}
	// 局部反馈耦合预建（0.7.0，评审稿 §7.3/§24.3；feedback.go）：每穴按
	// 「进边 × 出边」组合预建耦合（进边下标外层、出边下标内层升序——
	// 确定性顺序），自转移（进边 == 出边）不成耦合。耦合初值全零：未受
	// 教学的身体不会自响应（§26.7）。
	for i := range char.BodyGraph.Acupoints {
		var cps []FeedbackCoupling
		for ic := range char.BodyGraph.Channels {
			if char.BodyGraph.Channels[ic].To != i {
				continue
			}
			for oc := range char.BodyGraph.Channels {
				if char.BodyGraph.Channels[oc].From != i || oc == ic {
					continue
				}
				cps = append(cps, newFeedbackCoupling(ic, oc))
			}
		}
		char.BodyGraph.Acupoints[i].Couplings = cps
	}
	return &World{Character: char, Env: env, NoiseSeed: 1, RuleVersion: RuleVersion}
}

// ---------------------------------------------------------------------------
// Tick 报告（供测试与上层观察；不进入下一 Tick 的运算）
// ---------------------------------------------------------------------------

// TransferReport 是一条经脉本 Tick 的传输明细（§8-9 + 修订包 A2）。
type TransferReport struct {
	Channel     int
	From        int
	To          int
	Conductance [ElementCount]int64
	Capacity    [ElementCount]int64
	Requested   [ElementCount]int64
	Actual      [ElementCount]int64
	Delivered   [ElementCount]int64
	Loss        [ElementCount]int64

	// Reverse 标记本条为逆流传输（A2）：Actual 沿天然向反向行走
	// （从 To 端流向 From 端），From/To 仍为经脉天然端点。
	Reverse bool
	// Rejected 标记逆流请求被引擎拒绝（发起者不是核点，0.5.0 核点判据
	// = 驱动容量显著主导），此时无任何流量发生。
	Rejected bool
	// WorkCost[e] 是逆势驱动的做功代价（A2 公式结构不变，归属迁移为
	// 统一驱动求解的逆势代价输出——定稿裁决①；从发起核点按元素扣除，
	// 去向=热）。
	WorkCost [ElementCount]int64
	// DriveWork[e] 是顺流驱动的做功代价（0.5.0 新增，评审稿 §10.1：
	// k_work × ActualDrive² / DriveEfficiency；从驱动穴位按元素扣除，
	// 去向=热）。守恒账 DriveWork = Σ WorkCost + Σ DriveWork。
	DriveWork [ElementCount]int64

	// Passive[e] 是本条经脉本 Tick 的被动输运实际流量（Part J J2：压差
	// 驱动，沿天然向）。被动流走独立报告条目（无主动请求的经脉也会产生
	// 条目），与主动流共用 §9 损耗管线（守恒/损耗/炼化筛选取叠加后的
	// 实际流）。被动条目上 Actual[e] == Passive[e]（统一"实际流"消费口径：
	// 守恒、筑基采样、回执汇总都直接读 Actual）；Passive 字段单独归因，
	// 供 SelfSustain 测量区分"推动之流"与"自走之流"。
	Passive [ElementCount]int64

	// Autonomous[e] 是本条经脉本 Tick 自主驱动的实际流量（0.7.0，评审稿
	// §9.3/§13：FeedbackCoupling → AutonomousCommand → 统一驱动求解）。
	// 自主流走独立报告条目（该出边该元素本 Tick 无主动请求才产生——
	// 受控流不伪装自主，T-J3 语义），与主动/被动流共用 §9 损耗管线；
	// Actual[e] == Autonomous[e]（统一"实际流"口径），Autonomous 字段
	// 单独归因（台账 AutonomousWork、内化度的来源）。
	Autonomous [ElementCount]int64
}

// AcupointReport 是一个穴位本 Tick 结束时的诊断快照。
type AcupointReport struct {
	Load            int64
	LoadRatio       int64
	Pressure        int64
	FlowShock       int64
	Compatibility   int64
	Strain          int64
	TransformHeat   int64
	HeatLoadStrain  int64 // 热负荷应变分量（0.8.0，B7 环经济学裁决；已计入 Strain）
	Stability       int64
	Damage          int64
	CurrentFlow     int64
	BackPressure    int64 // 本 Tick 边界交换纳入时使用的反压
	PostIntakeLoad  int64 // 边界交换后立即测得的负载（§47）
	PostIntakeRatio int64 // 边界交换后负载比（§47，也是下一 Tick 反压的输入）
	ThermalLoad     int64 // 热负荷累积（修订包 A2/B4；散热衰减后含本 Tick 新热）
	DriveStrain     int64 // 驱动应变分量（0.5.0，评审稿 §10.2；已计入 Strain）
	DriveWork       int64 // 本穴位支付的驱动做功合计（0.5.0，去向=热）
	Qi              [ElementCount]int64
	Activity        [ElementCount]int64
	Intake          [ElementCount]int64 // §7 边界交换的意识驱动纳入增量（主动吐纳回执口径；无意识 → 0；途经纳气另列 Entrained）
	ExchangeIn      [ElementCount]int64 // §7 边界交换的被动纳入基线（无意识参与的自然吐纳，0.6.0）
	Diffuse         [ElementCount]int64 // §7 边界交换泄出量（去向=环境；原 §13 逸散并入，0.6.0）
	Entrained       [ElementCount]int64 // Part J J3 途经纳气（环境纳入，随实际流量逐穴计量）
}

// Conservation 是一次 Advance 的守恒归账（修订包 A3：任何总量变化必须
// 逐项可归因，无凭空增益）。恒等式（精确整数，逐项与运算用同一舍入值）：
//
//	QiAfter - QiBefore == Intake + ExchangeIn - Diffuse - TransferLoss
//	                    - ShengLoss - DriveWork + Entrained
//
// 去向标注：Diffuse 与 TransferLoss 的一半 → 环境；TransferLoss 另一半、
// ShengLoss、DriveWork → 热（穴位 heatAccum / reverseHeatAccum /
// ThermalLoad）。Intake 与 ExchangeIn 是 §7 边界交换纳入方向的归因分解
// （0.6.0，评审稿 §5.2/§5.3）：Intake = 意识驱动放大的纳入增量（主动
// 吐纳回执口径，无意识 → 0）；ExchangeIn = 被动通透性基线的纳入
// （「人体自然吐纳」，无意识也在发生）。两者之和 = 本 Tick 边界交换
// 纳入总量。Entrained 是 Part J J3 途经纳气（来源=环境，随实际流量
// 逐穴计量，流动调制形态，裁决②）。0.5.0 语义迁移（定稿裁决①）：
// 原 ReverseWork 项并入 DriveWork——逆流做功（A2）与顺流做功（§10.1）
// 同为统一驱动求解的做功代价输出，不再单列。
type Conservation struct {
	QiBefore     int64 // Tick 开始时 Σ 穴位储气
	QiAfter      int64 // Tick 结束时 Σ 穴位储气
	Intake       int64 // Σ §7 边界交换意识驱动纳入增量（逐穴计量）
	ExchangeIn   int64 // Σ §7 边界交换被动纳入基线（0.6.0；来源=环境）
	Diffuse      int64 // Σ §7 边界交换泄出（去向=环境；原 §13 逸散并入，0.6.0）
	TransferLoss int64 // Σ §9 传输损耗
	ShengLoss    int64 // Σ §10 转化损耗（去向=热）
	DriveWork    int64 // Σ 驱动做功代价（0.5.0：顺流 §10.1 + 逆势 A2，去向=热）
	Entrained    int64 // Σ 途经纳气（Part J J3，来源=环境）
}

// TickReport 是一次 Advance 的完整报告。
type TickReport struct {
	Tick         int64
	Transfers    []TransferReport
	Points       []AcupointReport
	Conservation Conservation
	Work         WorkLedger // 做功台账（0.5.0，评审稿 §15/§24.6；只读不回写）
}

// ---------------------------------------------------------------------------
// 入口
// ---------------------------------------------------------------------------

// Advance 推进一个 Tick 并返回报告。纯确定性：同状态同 Ops 必得同结果。
func Advance(w *World) *TickReport {
	n := len(w.Character.BodyGraph.Acupoints)
	rep := &TickReport{Tick: w.Tick}
	w.Tick++

	backPressure := make([]int64, n)
	intake := make([][ElementCount]int64, n)
	exchangeIn := make([][ElementCount]int64, n)
	diffuse := make([][ElementCount]int64, n)
	postLoad := make([]int64, n)
	postRatio := make([]int64, n)

	// Refinement 导出量快照（修订包 D2：绝不落存储，从本 Tick 开始时的
	// 气构成与身体状态重算）。本 Tick 内所有 D4 乘子使用同一快照——
	// Refinement=0 时全部乘子 = 1，与旧公式行为一致。
	refines := make([]int64, n)
	loadSum, weightedRefine := int64(0), int64(0)
	for i := 0; i < n; i++ {
		a := &w.Character.BodyGraph.Acupoints[i]
		refines[i] = refineOf(a, &w.Character)
		ld := a.load()
		loadSum += ld
		weightedRefine += fpMul(refines[i], ld)
	}
	refineAvg := int64(0)
	if loadSum > 0 {
		refineAvg = fpDiv(weightedRefine, loadSum) // 负载加权的平均炼化度（D4-2 口径）
	}

	// 守恒归账（修订包 A3）。
	cons := &rep.Conservation
	for i := 0; i < n; i++ {
		cons.QiBefore += w.Character.BodyGraph.Acupoints[i].load()
	}

	// §7 BackPressure：由上一 Tick 交换后的负载比决定（§47→§48 链式定义）。
	for i := 0; i < n; i++ {
		a := &w.Character.BodyGraph.Acupoints[i]
		lr2 := fpMul(a.lastLoadRatioMilli, a.lastLoadRatioMilli)
		backPressure[i] = fpDiv(One, One+lr2)
	}

	// §7 边界交换（势差版统一原语，0.6.0；定稿裁决② + 阶段二范围 1/2；
	// 评审稿 §5 BoundaryExchange、§5.1 基本公式、§14.2 能量来源）：
	//
	//	ExchangeFlow ∝ 通透性 × 有效开放度 × 亲和 × (环境势 − 体内势) × 活跃度
	//
	// 环境势 = Environment.Qi[e]（环境灵气密度，不因交换消耗——stage_3 §6）；
	// 体内势 = 穴位该元素存量（与环境密度同标度的浓度势）。势差为正 → 纳入；
	// 为负 → 泄出（去向=环境；原 §13 逸散并入泄出方向，D3/D4-1 乘子结构
	// 原样，环境势为零时与旧 §13 公式逐位一致）。无势差 → 双向皆零
	// （零净流，评审稿 §28.5 防免费吸气禁令）。
	//
	// 方向门控（§28.5 约束清单逐项落位：势差/通透性/开放度/反压/亲和）：
	//   纳入方向 × BackPressure（§47→§48 反压链：满则难纳，D6 烧蚀闭环；
	//   且势差本身随体内势升高而闭合——满则吸不进在结构上更强）；
	//   泄出方向 × (1 − Retention_effective) × (1 − Refine×KRefine)
	//   （D3「留与散」+ D4-1 炼化抗逸散）。
	//
	// 意识驱动不是第二套物理（评审稿 §5.2）：intensity 只放大纳入方向的
	// 通透性 perm = KPermBase + KPermDrive×intensity（提高开放度/通透性）。
	// 泄出方向不受意识放大——散逸由结构保留度决定，不随意识开合。
	// 报告/守恒归因把纳入分解为：放大增量 → Intake（主动吐纳回执/感知
	// 口径与旧 §7 对齐：无意识 → 0），被动基线 → ExchangeIn（§5.3 人体
	// 自然吐纳）。分解单调无重叠（perm 单调 ⇒ 增量 ≥ 0）。黄金用例第一
	// Tick：体内势为零 → 势差=环境密度 → 总纳入与旧 §7 逐位一致
	//（KPermBase + KPermDrive×0.6 = 0.6 = 旧 intensity 档，乘法链同序）。
	for i := 0; i < n; i++ {
		a := &w.Character.BodyGraph.Acupoints[i]
		intensity := int64(0)
		if i < len(w.Ops.Intensity) {
			intensity = w.Ops.Intensity[i]
		}
		if intensity < 0 {
			intensity = 0
		}
		effOpen := a.effectiveOpenness()
		perm := KPermBase + fpMul(KPermDrive, intensity)
		for e := 0; e < ElementCount; e++ {
			gap := w.Env.Qi[e] - a.Qi[e] // 环境势 − 体内势
			if gap > 0 {
				// 纳入方向。乘法链与旧 §7 同序（gap 处于旧 EnvQi 位置、
				// perm 处于旧 intensity 位置）。
				actFactor := 500 + fpMul(500, w.Env.Activity[e]) // §7.2
				v := fpMul(gap, perm)
				v = fpMul(v, w.Character.Affinity[e])
				v = fpMul(v, a.Affinity[e])
				v = fpMul(v, effOpen)
				v = fpMul(v, backPressure[i])
				v = fpMul(v, actFactor)
				if v <= 0 {
					continue
				}
				a.Qi[e] += v
				// 边界交换是缓和交换，不计入 FlowShock/QiFlow
				//（见 body.go flowAccum 注释）。
				pv := v
				if intensity > 0 {
					// 被动基线（perm = KPermBase 的同链乘积）。
					pv = fpMul(gap, KPermBase)
					pv = fpMul(pv, w.Character.Affinity[e])
					pv = fpMul(pv, a.Affinity[e])
					pv = fpMul(pv, effOpen)
					pv = fpMul(pv, backPressure[i])
					pv = fpMul(pv, actFactor)
					if pv > v {
						pv = v // 单调性下不会发生；防御性钳制保恒等式
					}
				}
				exchangeIn[i][e] = pv
				intake[i][e] = v - pv
				cons.ExchangeIn += pv
				cons.Intake += v - pv
			} else if gap < 0 {
				// 泄出方向（原 §13 逸散并入，0.6.0）：驱动量由存量改为
				// 净势差（体内势 − 环境势）；环境势为零时即旧 §13 公式。
				// d 系数上界 = BaseDiffuseRate×1×1×1 = 2% ≪ 1，且
				// gap < 0 ⇒ −gap < a.Qi[e]，天然不透支存量。
				retEff := fpClamp(fpMul(a.Retention, a.refineAffinity(e)), 0, One)
				d := fpMul(fpMul(fpMul(-gap, BaseDiffuseRate), One-retEff), a.Activity[e])
				d = fpMul(d, One-fpMul(refines[i], KRefine))
				if d <= 0 {
					continue
				}
				a.Qi[e] -= d
				// 逸散是缓和交换，不计入 FlowShock/QiFlow（同上）。
				diffuse[i][e] = d
				cons.Diffuse += d
			}
		}
	}

	// §47 边界交换后立即测得的负载与压力（黄金用例口径）。
	for i := 0; i < n; i++ {
		a := &w.Character.BodyGraph.Acupoints[i]
		postLoad[i] = a.load()
		r := loadRatio(a, postLoad[i])
		postRatio[i] = r
		a.lastLoadRatioMilli = r // 下一 Tick 反压的输入
	}

	// §8-9 灵气传输与损耗（含修订包 A2 逆流做功、Part J 被动输运与途经纳气、
	// 0.5.0 统一驱动求解、0.7.0 自主命令与教学样本）。
	var entrainedArr [][ElementCount]int64
	var ds *driveSettle
	var ts *teachSamples
	rep.Transfers, entrainedArr, ds, ts = runTransfers(w, refines, postRatio)
	for _, tr := range rep.Transfers {
		for e := 0; e < ElementCount; e++ {
			cons.TransferLoss += tr.Loss[e]
			cons.DriveWork += tr.WorkCost[e] + tr.DriveWork[e]
		}
	}
	// Part J J3：途经纳气归账（来源=环境，单独成项）。报告逐穴单列
	// Entrained（不并入 §7 Intake），守恒归因走 cons.Entrained。
	for i := 0; i < n; i++ {
		for e := 0; e < ElementCount; e++ {
			cons.Entrained += entrainedArr[i][e]
		}
	}

	// §10 相生转化。
	cons.ShengLoss = runSheng(w, refines)

	// §11 相克压活性。
	runKe(w)

	// §12 活性恢复。
	for i := 0; i < n; i++ {
		a := &w.Character.BodyGraph.Acupoints[i]
		for e := 0; e < ElementCount; e++ {
			a.Activity[e] += fpMul(ActivityRecoveryRate, NaturalActivity-a.Activity[e])
		}
	}

	// §13 逸散已并入 §7 边界交换泄出方向（0.6.0，定稿裁决②）——泄出
	// 按 Tick 开始时的势差在 §7 结算；§10 转化的产物自下一 Tick 起参与
	// 势差交换（Tick 初结算的世界语义：交换先于流转与转化）。

	// §14-§24 逐穴位状态结算。
	rep.Points = make([]AcupointReport, n)
	for i := 0; i < n; i++ {
		a := &w.Character.BodyGraph.Acupoints[i]
		r := &rep.Points[i]
		r.BackPressure = backPressure[i]
		r.PostIntakeLoad = postLoad[i]
		r.PostIntakeRatio = postRatio[i]
		for e := 0; e < ElementCount; e++ {
			r.Intake[e] = intake[i][e]
			r.ExchangeIn[e] = exchangeIn[i][e]
			r.Diffuse[e] = diffuse[i][e]
			r.Entrained[e] = entrainedArr[i][e]
		}

		// §14 压力。
		r.Load = a.load()
		r.LoadRatio = loadRatio(a, r.Load)
		r.Pressure = fpMul(r.LoadRatio, r.LoadRatio)

		// §15 FlowShock。
		effTp := a.effectiveThroughput()
		denom := effTp
		if denom < ShockEpsilon {
			denom = ShockEpsilon
		}
		r.CurrentFlow = a.flowAccum
		r.FlowShock = fpDiv(abs64(r.CurrentFlow-a.PreviousFlow), denom)

		// §16 Compatibility。
		r.Compatibility = compatibility(a, r.Load)

		// §17 Strain（D4-4：FlowShock_effective × (1 - Refine×KShock)，
		// 炼化气更抗冲击；报告中的 FlowShock 保持原值）。
		r.TransformHeat = fpMul(fpClamp(fpDiv(a.heatAccum, denom), 0, One), TransformHeatMax)
		overload := r.LoadRatio - One
		if overload < 0 {
			overload = 0
		}
		affStrain := One - r.Compatibility
		if affStrain < 0 {
			affStrain = 0
		}
		shockEff := fpMul(r.FlowShock, One-fpMul(refines[i], KShock))
		r.Strain = fpMul(WOverload, fpMul(overload, overload)) +
			fpMul(WAffinity, fpMul(affStrain, affStrain)) +
			fpMul(WShock, fpMul(shockEff, shockEff)) +
			r.TransformHeat
		// 热负荷应变（0.8.0，修订包 B7 注记 2026-10-07 环经济学裁决）：
		// 长程热负荷进入应变——P5 标定项「ThermalLoad 只累积不进公式」
		// 就此收口，走火风险有了慢响应面。口径：(ThermalLoad − 无伤死区
		// HeatStrainFloor)/EffThroughput（与 TransformHeat 同一通量归一；
		// 大通量穴热容大，等量热负荷应变更低），比值钳 [0,1]，× KHeatLoad
		// （与 TransformHeatMax 同档）。死区语义：发热无碍，灼热以上
		// 方有走火之虞（与感知层 thermalBands 同源，见 KHeatLoad 注释）。
		// 读数取上一 Tick 结算后的 ThermalLoad（本 Tick 新热下一 Tick 起
		// 作用——慢通道一拍滞后，确定性不变）。热不进守恒账（热非守恒量），
		// 本项只改应变速率，不凭空生/灭气。
		heatRatio := fpDiv(a.ThermalLoad-HeatStrainFloor, denom)
		if heatRatio > One {
			heatRatio = One
		}
		if heatRatio < 0 {
			heatRatio = 0
		}
		r.HeatLoadStrain = fpMul(KHeatLoad, heatRatio)
		r.Strain += r.HeatLoadStrain
		// 驱动应变（0.5.0，评审稿 §10.2）：过驱 + 加力突变分量，由统一
		// 驱动求解在传输结算时逐穴位归并（最受力一路）。温和稳定驱动
		// （安全驱动比内、无突变）该分量为零——耗能不等于受伤（§26.5）。
		r.DriveStrain = ds.strain[i]
		r.Strain += r.DriveStrain
		r.DriveWork = ds.work[i]

		// §18 Stability（D4-2：稳定性恢复 × (1 + Refine_avg×KStab)）。
		earthRatio := int64(0)
		if r.Load > 0 {
			earthRatio = fpDiv(a.Qi[Earth], r.Load)
		}
		earthStab := One + fpMul(fpMul(earthRatio, a.Activity[Earth]), EarthStabilizationBonus)
		recovery := fpMul(fpMul(BaseRecovery, One-a.Damage), earthStab)
		recovery = fpMul(recovery, One+fpMul(refineAvg, KStab))
		a.Stability = fpClamp(a.Stability+recovery-r.Strain, 0, One)
		r.Stability = a.Stability

		// §19 Damage。
		if a.Stability < DamageThreshold {
			gain := fpMul(DamageThreshold-a.Stability, DamageRate)
			a.Damage = fpClamp(a.Damage+gain, 0, One)
		}

		// §21 自然恢复。
		if r.Pressure < LowPressureThreshold && r.Strain < LowStrainThreshold {
			a.Damage = fpClamp(a.Damage-fpMul(BaseDamageRecovery, BodyRecoveryModifier), 0, One)
		}
		r.Damage = a.Damage

		// ThermalLoad（修订包 A2/B4）：旧值先按散热衰减率衰减（0.8.0 起
		// 逐穴画像化——Acupoint.HeatDissipation，未画像用全局
		// HeatDissipation；敛热穴语义画在世界画像层，底层零专名），再累加
		// 本 Tick 的转化损耗热（§9/§10 → heatAccum）与逆流做功热
		// （reverseHeatAccum）。0.8.0 起经 §17 HeatLoadStrain 进入应变
		// （P5 标定项收口，见上方热负荷应变注释）。
		a.ThermalLoad = fpMul(a.ThermalLoad, One-a.heatDissipation()) + a.heatAccum + a.reverseHeatAccum
		r.ThermalLoad = a.ThermalLoad

		// §22 Adaptation。
		if a.Stability >= AdaptationSafeStability && r.Pressure <= AdaptationSafePressure {
			for e := 0; e < ElementCount; e++ {
				flow := a.elemFlowAccum[e]
				if flow <= MinimumEffectiveFlow {
					continue
				}
				sfr := fpDiv(flow, denom)
				if sfr > One {
					sfr = One
				}
				gap := AdaptationMax - int64(a.adaptationMilli(e))
				a.Adaptation[e] += AdaptationRate * sfr * gap / MicroOne
				if a.Adaptation[e] > AdaptationMax*1000 {
					a.Adaptation[e] = AdaptationMax * 1000
				}
			}
		}

		// §23 Openness 成长。
		if r.Pressure >= OpenSafePressureLo && r.Pressure <= OpenSafePressureHi && a.Stability >= OpenSafeStability {
			o := One - a.opennessMilli()
			if o > 0 {
				a.Openness += OpenRate * r.Pressure * o / MicroOne
				if a.Openness > MicroOne {
					a.Openness = MicroOne
				}
			}
		}

		// §15 的"上一 Tick 流量"滚动 + 清理本 Tick 累计。
		a.PreviousFlow = a.flowAccum
		a.flowAccum = 0
		a.heatAccum = 0
		a.reverseHeatAccum = 0
		a.elemFlowAccum = [ElementCount]int64{}
		a.prevDriveRatio = ds.ratio[i] // 驱动比滚动（k_ramp 突变项输入）

		r.Qi = a.Qi
		r.Activity = a.Activity
	}

	// 局部反馈耦合学习（0.7.0，评审稿 §12/§23 步骤 15）：以本 Tick 真实
	// 输入/输出样本更新耦合——教师门 × 输入信号 × 安全门 × 误差（§12.8）。
	// 放在状态结算后：安全门读取本 Tick 的稳定性/相容/损伤/应变。
	updateFeedbackCouplings(w, rep, ts)

	// §24 经脉 Adaptation。
	for c := range w.Character.BodyGraph.Channels {
		ch := &w.Character.BodyGraph.Channels[c]
		denom := ch.Throughput
		if denom < ShockEpsilon {
			denom = ShockEpsilon
		}
		sfr := fpDiv(ch.flowAccum, denom)
		if sfr > One {
			sfr = One
		}
		gap := ChannelAdaptMax - ch.ChannelAdaptation/1000
		ch.ChannelAdaptation += ChannelAdaptRate * sfr * gap / MicroOne
		if ch.ChannelAdaptation > ChannelAdaptMax*1000 {
			ch.ChannelAdaptation = ChannelAdaptMax * 1000
		}
		ch.flowAccum = 0
	}

	// 守恒归账收尾（修订包 A3）：Qi 在 §10 相生转化后不再变化（§7 边界
	// 交换与 §8-9 传输均已结算），此处汇总。
	for i := 0; i < n; i++ {
		cons.QiAfter += w.Character.BodyGraph.Acupoints[i].load()
	}

	// 做功台账（0.5.0，定稿阶段二范围 4；评审稿 §15/§24.6）。做功三分归因
	// 按传输条目逐元素判读：被动条目（Passive>0）归 PassiveWork，自主条目
	// （Autonomous>0，0.7.0 起经统一驱动求解接入）归 AutonomousWork，其余
	// 实际流量归 ManualWork（三类条目不混——被动条目只由被动输运循环产生，
	// 自主条目只由耦合命令循环产生，主动条目两者皆无）。内化度
	// （DeriveInternalization）随自主做功入账自然生效。
	led := &rep.Work
	for _, tr := range rep.Transfers {
		for e := 0; e < ElementCount; e++ {
			switch {
			case tr.Passive[e] > 0:
				led.PassiveWork += tr.Actual[e]
			case tr.Autonomous[e] > 0:
				led.AutonomousWork += tr.Actual[e]
			default:
				led.ManualWork += tr.Actual[e]
			}
		}
	}
	led.DriveWork = cons.DriveWork
	led.ExternalInput = cons.Intake + cons.ExchangeIn + cons.Entrained
	led.Loss = cons.TransferLoss + cons.Diffuse + cons.ShengLoss

	w.Last = rep
	return rep
}

// ---------------------------------------------------------------------------
// 阶段实现
// ---------------------------------------------------------------------------

// loadRatio 返回 Load/Capacity（千分位）；容量非正时视为 0。
func loadRatio(a *Acupoint, load int64) int64 {
	cap := a.effectiveCapacity()
	if cap <= 0 {
		return 0
	}
	return fpDiv(load, cap)
}

// CorePointOf 返回身体图的核点下标；无主导者时返回 -1。
//
// 0.5.0 判据（定稿阶段二范围 1 / 裁决①，取代 0.3.0 的「Capacity 最大」）：
// **驱动容量显著高于其余节点者**——DriveCapacity 须高于其余每一穴位的
// CoreDriveSignificance 倍，否则无核点（-1）。这是参数化结构判据而非
// 类型枚举（宪法 §8）：丹田画像的特殊性——唯可发起逆流外推（A2）——
// 全部落在「驱动容量显著主导」的参数事实上，无 type 字段、无专名判断；
// 「Capacity 最大 = 唯一主动核心」的存储/驱动混用规则就此废除（外部代码
// 审查第 6 条）。判据读画像原值（非损伤折算）：核点是结构身份，不随
// 当日伤势漂移；驱动能力本身的削弱由 effectiveDriveCapacity 承担。
// 旧判据「纯汇点（无出向经脉）」在主环拓扑（A1 v0.2）下失效：丹田是
// 环上的泵，有出向边 丹→土。至多一穴满足对全部他穴的严格倍数支配
// （两穴互为 2 倍不可能），遍历序不影响结果；无满足者返回 -1（含全体
// DriveCapacity 为 0 的未画像世界——谁都推不动逆势输运）。
// ctmatch 的协议编译/意图路径与本引擎核同用本函数，避免两份判据漂移。
func CorePointOf(g *BodyGraph) int {
	for i := range g.Acupoints {
		dominant := true
		for j := range g.Acupoints {
			if i != j && g.Acupoints[i].DriveCapacity <= fpMul(g.Acupoints[j].DriveCapacity, CoreDriveSignificance) {
				dominant = false
				break
			}
		}
		if dominant {
			return i
		}
	}
	return -1
}

// driveSettle 汇集一次传输结算的驱动侧派生量（0.5.0 统一驱动求解的
// 输出，Advance 内部管线传递用；评审稿 §24.5 DriveResult 的逐穴位归并）。
type driveSettle struct {
	// strain[i] 是穴位 i 本 Tick 的驱动应变（评审稿 §10.2：过驱项 + 加力
	// 突变项）。多路驱动取最受力一路（max）：应变集中于最吃力的那一路，
	// 与「均摊多路温和驱动不打稳定」的代价分层一致。
	strain []int64
	// ratio[i][e] 是穴位 i 元素 e 本 Tick 的驱动比（ActualDrive /
	// EffectiveDriveCapacity，千分位）——下一 Tick k_ramp 突变项的输入
	// （Advance 收尾滚动进 Acupoint.prevDriveRatio）。
	ratio [][ElementCount]int64
	// work[i] 是穴位 i 本 Tick 支付的驱动做功合计（顺流 + 逆势，去向=热）。
	work []int64
}

// runTransfers 实现 §8-9（含修订包 A2 逆流做功、Part J 被动输运与途经纳气、
// 0.5.0 统一驱动求解）。
//
// 统一驱动求解（0.5.0，定稿阶段二范围 2 / 裁决①；评审稿 §9.4 ResolveDrive）：
// 每笔主动请求逐元素走同一管线——请求驱动（控制误差 ε 后的请求量）→
// 校验驱动容量与可用能量 → 计算做功 → 产出废热与应变 → 实际驱动：
//
//	ActualDrive = min(请求量, §8.1 管线容量, 源穴存量,
//	                 源穴有效驱动容量,                      // 驱动容量限幅（§26.3）
//	                 可用能量 − 做功(actual))               // 能量收敛（§26.4）
//	做功：顺流 = k_work × ActualDrive² / DriveEfficiency（§10.1，从驱动穴位
//	      扣除、热化于驱动穴位）；逆势 = A2 公式（结构不变、归属迁移，
//	      从发起核点扣除、×KHeat 热化于逆流目的地）
//	无可用能量则实际驱动衰减为零（driveEnergyFit）——无凭空做功。
//
// 逆流语义（A2/B1）：请求带 Reverse 标记时，Actual 沿经脉天然向反向行走
// （从 To 端流向 From 端）。唯有核点可发起逆流；其余逆流请求被拒绝
// （Rejected）——逆经脉而行，唯驱动容量显著主导者推得动（0.5.0 核点
// 判据，宪法 §8 无专名）。
// 驱动应变（§10.2）逐穴位按最受力一路归并（driveSettle.strain），在 §17
// 并入 Strain 打稳定；温和稳定驱动（安全比内、无突变）应变为零。
//
// Part J J2 被动输运：全部主动请求结算完毕后，对每条经脉按天然气向追加
// 压差驱动的被动项（压差快照 = 本 Tick 边界交换后的负载比，对全部经脉同源，
// 与经脉遍历顺序无关）：
//
//	passiveFlow = clamp(chanCap×KPassive×ΔP×Conductance   // 压差梯度项
//	                    + KInertia×inflow(源穴),          // R4 惯性接力项
//	                   0, min(chanCap×Conductance,        // §8.1 管线容量
//	                          源穴储气×KQiShare,           // 压而不泻封顶
//	                          管线余量,                    // 主动已占容量不计
//	                          受端份额封顶))               // 0.7.1 受不至胀
//	ΔP = Pressure(a) - Pressure(b)（Pressure = LoadRatio²，交换后口径）
//
// 被动项是势差自然释放：不占驱动容量、不付做功、不打驱动应变（评审稿
// §15 PassiveWork 语义），与主动请求叠加（J2），但只填同一 §8.1 管线的
// 余量——总实际流（主动+被动）不超过管道容量，主动推力不被被动旁路
// 放大（R1）。压差为负则梯度项为零，惯性项沿天然向衰减延续（被动永不
// 逆天然向）。惯性接力的输入是"其他经脉流入源穴"的实际流量（上一 Tick
// 打底 + 本 Tick 送达 live 接力）；自环通道的自身回流不是来流，不计入
// （否则成做功无源的影子回声）。纯压差项在闭环上会均衡化归零（环上压差
// 快照同和为零，无泵不成环流），惯性项即 R4 预案的 Persistent Flow State
// 第二阶段。被动流与主动流共用同一 §9 损耗管线并同样触发途经纳气，使
// 守恒、损耗、炼化筛选都作用于叠加后的实际流；报告中以独立条目（Passive
// 字段）记账，与主动流可归因区分。
//
// Part J J3 途经纳气：每笔实际流量（主动/逆流/被动）在其接收端穴位卷入
// 环境气（来源=环境，守恒账 Entrained 项）：
//
//	entrained = F × Openness × EnvironmentQi × AcupointAffinity
//	            × KEntrain × (1 - LoadRatio)
//
// 返回传输报告、逐穴途经纳气量与驱动侧派生量（driveSettle）。
func runTransfers(w *World, refines []int64, postRatio []int64) ([]TransferReport, [][ElementCount]int64, *driveSettle, *teachSamples) {
	g := &w.Character.BodyGraph
	reports := make([]TransferReport, 0, len(w.Ops.Requests)+2*len(g.Channels))
	entrained := make([][ElementCount]int64, len(g.Acupoints))
	chanFlow := make([][ElementCount]int64, len(g.Channels))   // 本 Tick 逐经脉逐元素实际流量（→ FlowPrev）
	activeFlow := make([][ElementCount]int64, len(g.Channels)) // 本 Tick 逐经脉逐元素主动/自主流量（被动让行判据）
	controlErrMax := fpMul(ControlErrorBase, One-w.Character.Control)
	// 教学样本（0.7.0，feedback.go）：逐经脉天然向送达（进边来流）与
	// 手动驱动实流（教学出流），供学习遍消费。
	ts := &teachSamples{
		chanDelivered: make([][ElementCount]int64, len(g.Channels)),
		manualOut:     make([][ElementCount]int64, len(g.Channels)),
	}
	// 手动请求占道（0.7.0，T-J3 语义）：出边某元素被主动请求占用时，
	// 该元素本 Tick 不产生自主命令——协议执行期的受控流不得伪装成自主。
	manualReqOn := make([][ElementCount]bool, len(g.Channels))
	for _, req := range w.Ops.Requests {
		if req.Channel < 0 || req.Channel >= len(g.Channels) {
			continue
		}
		for e := 0; e < ElementCount; e++ {
			if req.Amount[e] > 0 {
				manualReqOn[req.Channel][e] = true
			}
		}
	}
	ds := &driveSettle{
		strain: make([]int64, len(g.Acupoints)),
		ratio:  make([][ElementCount]int64, len(g.Acupoints)),
		work:   make([]int64, len(g.Acupoints)),
	}

	// 途经纳气（Part J J3）：一笔流量 F 在接收端穴位 i（元素 e）卷入的环境气。
	entrainInto := func(i int, a *Acupoint, e int, flow int64) {
		if flow <= 0 || w.Env.Qi[e] <= 0 {
			return
		}
		v := fpMul(flow, a.effectiveOpenness())
		v = fpMul(v, w.Env.Qi[e])
		v = fpMul(v, a.effectiveAffinity(e))
		v = fpMul(v, KEntrain)
		v = fpMul(v, One-postRatio[i])
		if v <= 0 {
			return
		}
		a.Qi[e] += v
		entrained[i][e] += v
	}

	// 核点判据（0.5.0：驱动容量显著主导者，见 CorePointOf）：逆流发起资格
	// = 发起者（逆流请求的 To 端）是核点。
	core := CorePointOf(g)

	// chanCtx 汇集一条经脉一个方向的统一驱动求解静态项（评审稿 §9.4
	// ResolveDrive 的校验输入：传导/容量/驱动容量/效率/逆流做功单价）。
	// 手动请求与自主命令共用同一 buildCtx/resolve——Manual 与 Autonomous
	// 不得走两套「魔法通道」（评审稿 §9.4）。
	type chanCtx struct {
		ci             int
		srcIdx, dstIdx int
		reverse        bool
		cond, eff, cap [ElementCount]int64
		effDriveCap    int64
		effi           int64
		workUnit       int64 // 逆流专用：A2 做功单价
	}
	buildCtx := func(ci int, reverse bool) *chanCtx {
		ch := &g.Channels[ci]
		if ch.From < 0 || ch.From >= len(g.Acupoints) || ch.To < 0 || ch.To >= len(g.Acupoints) {
			return nil
		}
		src, dst := &g.Acupoints[ch.From], &g.Acupoints[ch.To]
		if reverse {
			src, dst = &g.Acupoints[ch.To], &g.Acupoints[ch.From]
		}
		ctx := &chanCtx{ci: ci, srcIdx: ch.From, dstIdx: ch.To, reverse: reverse}
		if reverse {
			ctx.srcIdx, ctx.dstIdx = ch.To, ch.From
		}

		chanAdapt := ch.ChannelAdaptation / 1000
		baseCond := fpMul(ch.BaseConductance, chanAdapt) // §24
		condFactor := fpMul(fpMul(src.effectiveOpenness(), dst.effectiveOpenness()), One-ch.Damage)
		chanCap := ch.Throughput
		if s := src.effectiveThroughput(); s < chanCap {
			chanCap = s
		}
		if d := dst.effectiveThroughput(); d < chanCap {
			chanCap = d
		}
		for e := 0; e < ElementCount; e++ {
			aff := fpSqrt(fpMul(src.effectiveAffinity(e), dst.effectiveAffinity(e)))
			cond := fpMul(fpMul(baseCond, aff), condFactor)
			ctx.cond[e] = cond
			ctx.eff[e] = fpClamp(cond, 0, One)
			ctx.cap[e] = fpMul(chanCap, cond) // §8.1 ChannelCapacity
		}

		// 驱动容量与驱动效率（0.5.0，评审稿 §9.4 校验项）：实际驱动不得
		// 超过驱动端穴位的有效驱动容量——存储容量（Capacity）不再替代
		// 驱动维度（§26.3；外部代码审查第 6 条）。逆流的驱动端是发起核点
		// （src 已按流向取对）。
		ctx.effDriveCap = src.effectiveDriveCapacity()
		ctx.effi = src.driveEfficiency()
		if reverse {
			// A2：逆流目的地的即时负载比（推入越满，做功越贵）。
			// workUnit = BaseWork × (1 + LoadRatio_dst × KBack)。
			lrDst := loadRatio(dst, dst.load())
			ctx.workUnit = fpMul(BaseWork, One+fpMul(lrDst, KBack))
		}
		return ctx
	}

	// resolve 统一驱动求解的逐元素执行（评审稿 §9.4/§10 定点管线）：
	// 请求驱动 → 限幅（管线容量 / 源穴存量 / 有效驱动容量，§26.3）→
	// 做功 → 能量收敛（§26.4：无可用能量则实际驱动衰减为零）→ 实际驱动
	// + 废热 + 应变。返回实际驱动量（0 = 未成驱动）。0.7.0 起手动与自主
	// 共用本闭包。
	resolve := func(ctx *chanCtx, e int, requested int64, tr *TransferReport) int64 {
		src := &g.Acupoints[ctx.srcIdx]
		dst := &g.Acupoints[ctx.dstIdx]
		actual := requested
		if src.Qi[e] < actual {
			actual = src.Qi[e]
		}
		if ctx.cap[e] < actual {
			actual = ctx.cap[e]
		}
		if ctx.effDriveCap < actual {
			actual = ctx.effDriveCap // 驱动容量限幅（§26.3）
		}
		// 做功公式：顺流 = §10.1 二次律；逆势 = A2（结构不变，裁决①）。
		// 逆势不做功效率折算：A2 公式结构原样保留。
		workOf := func(a int64) int64 {
			if ctx.reverse {
				return fpMul(a, fpMul(ctx.workUnit, One-ctx.eff[e]))
			}
			return driveWorkOf(a, ctx.effi)
		}
		actual = driveEnergyFit(actual, src.Qi[e], workOf) // 无可用能量则衰减为零（§26.4）
		if actual <= 0 {
			return 0
		}
		work := workOf(actual)

		// §9 传输损耗。
		delivered := fpMul(actual, ctx.eff[e])
		loss := actual - delivered

		// 做功从真实能源扣除（评审稿 §10.1：DriveWork 必须从可用能源扣）
		// ——fit 已保证 actual + work ≤ src.Qi[e]，扣后不透支。
		src.Qi[e] -= actual
		src.Qi[e] -= work
		dst.Qi[e] += delivered
		dst.heatAccum += fpMul(loss, One-DiffuseShare) // 传输损耗 50% → 热

		// Part J J3：接收端途经纳气（主动/自主流同样卷入环境气）。
		entrainInto(ctx.dstIdx, dst, e, actual)

		// 废热去向（定稿阶段二范围 2：做功去向=热）：
		//   顺流——热化于驱动穴位（heatAccum → ThermalLoad / §17 TransformHeat）；
		//   逆势——A2 结构不变：×KHeat 热化于逆流目的地（reverseHeatAccum）。
		if ctx.reverse {
			dst.reverseHeatAccum += fpMul(work, KHeat)
			tr.WorkCost[e] = work
		} else {
			src.heatAccum += work
			tr.DriveWork[e] = work
		}

		// 驱动应变与驱动比（评审稿 §10.2）：过驱 + 加力突变项逐穴位按
		// 最受力一路归并；驱动比滚动进 ds.ratio（收尾落 prevDriveRatio）。
		if ctx.effDriveCap > 0 {
			ratio := fpDiv(actual, ctx.effDriveCap)
			ds.ratio[ctx.srcIdx][e] = ratio
			if s := driveStrainOf(ratio, src.prevDriveRatio[e]); s > ds.strain[ctx.srcIdx] {
				ds.strain[ctx.srcIdx] = s
			}
		}
		ds.work[ctx.srcIdx] += work

		src.elemFlowAccum[e] += actual
		dst.elemFlowAccum[e] += delivered
		src.flowAccum += actual
		dst.flowAccum += delivered
		g.Channels[ctx.ci].flowAccum += actual
		chanFlow[ctx.ci][e] += actual
		if !ctx.reverse {
			// 天然向送达（To 端实收）——耦合学习的进边来流样本（0.7.0）。
			ts.chanDelivered[ctx.ci][e] += delivered
		}

		tr.Actual[e] = actual
		tr.Delivered[e] = delivered
		tr.Loss[e] = loss
		return actual
	}

	for _, req := range w.Ops.Requests {
		if req.Channel < 0 || req.Channel >= len(g.Channels) {
			continue
		}
		ch := &g.Channels[req.Channel]
		if ch.From < 0 || ch.From >= len(g.Acupoints) || ch.To < 0 || ch.To >= len(g.Acupoints) {
			continue
		}
		tr := TransferReport{Channel: req.Channel, From: ch.From, To: ch.To, Reverse: req.Reverse}

		// 端点选取：顺流 From→To；逆流 To→From（A2）。发起者不是核点
		// ——逆经脉而行，唯驱动容量显著主导者推得动：拒绝。
		if req.Reverse && ch.To != core {
			tr.Rejected = true
			reports = append(reports, tr)
			continue
		}
		ctx := buildCtx(req.Channel, req.Reverse)

		// D4-5：控制误差幅值 × (1 - Refine_src×KControl)。src 已按流向取对
		//（顺流源 = From 端，逆流源 = To 端）。控制误差是意识控制的属性
		//——自主命令不经 ε（见下方自主命令循环）。
		controlErrEff := fpMul(controlErrMax, One-fpMul(refines[ctx.srcIdx], KControl))

		for e := 0; e < ElementCount; e++ {
			tr.Requested[e] = req.Amount[e]
			if req.Amount[e] <= 0 {
				continue
			}
			tr.Conductance[e] = ctx.cond[e]
			tr.Capacity[e] = ctx.cap[e]

			// §8.2 控制误差 ε = deterministic_noise(seed, tick, channel, e)。
			eps := noiseSigned(w.NoiseSeed, w.Tick, req.Channel, e) * controlErrEff / 1000
			requested := fpMul(req.Amount[e], One+eps)

			if actual := resolve(ctx, e, requested, &tr); actual > 0 {
				activeFlow[req.Channel][e] += actual // 主动占道（R1：被动让行）
				if !req.Reverse {
					// 教学样本（0.7.0，评审稿 §12.3）：手动驱动的实流是
					// 「受控教学输出」；逆流是调度不是续推示范，不计入。
					ts.manualOut[req.Channel][e] += actual
				}
			}
		}
		reports = append(reports, tr)
	}

	// 自主命令（0.7.0，评审稿 §7.2/§13；定稿阶段三范围 3）：耦合产生的
	// 是响应命令，必须经统一驱动求解方成真实自主驱动——耦合不是隐藏
	// 能源（§28.2）。触发条件（§26.11 局部内化）：
	//   - 出边该元素本 Tick 无主动请求占道——协议执行期的受控流不得
	//     伪装成自主（T-J3 语义）；
	//   - 进边有来流（LatencyTicks=1 取上一 Tick 的进边总来流——被动
	//     送达与主动/自主送达同计入信号，「来流之后」天然一拍延迟；
	//     d > 1 取更早历史档）——无来流不得凭空启动（§26.7/§26.11）；
	//   - 来流信号在该耦合的教学档位容差内（评审稿 §24.3 Tolerance）；
	//   - 强度 > 0——未内化的转移永不自响应。
	// 命令量 = 强度 × 观测来流 × 习得响应（评审稿 §13.1 最小形式的定点
	// 化：§13.1 的归一化 InputSignal 在绝对流量空间取观测来流，使命令
	// 复现「来流 × 学得增益」；归一化信号用于学习门控与容差判别）。
	// 遍历序 = 穴位下标序 × 耦合序（NewWorld 预建序，确定性）。
	for i := range g.Acupoints {
		a := &g.Acupoints[i]
		if len(a.Couplings) == 0 {
			continue
		}
		effTP := a.effectiveThroughput()
		if effTP < ShockEpsilon {
			effTP = ShockEpsilon
		}
		for k := range a.Couplings {
			cp := &a.Couplings[k]
			if !cp.validFor(g, i) {
				continue
			}
			ctx := buildCtx(cp.OutChannel, false)
			if ctx == nil {
				continue
			}
			var tr TransferReport
			tr.Channel, tr.From, tr.To = cp.OutChannel, ctx.srcIdx, ctx.dstIdx
			fired := false
			for e := 0; e < ElementCount; e++ {
				if manualReqOn[cp.OutChannel][e] {
					continue // 受控流不伪装自主（T-J3 语义）
				}
				if cp.Strength[e] <= 0 {
					continue // 未内化：无自主响应（§26.7）
				}
				// 命令输入 = 进边来流历史（LatencyTicks=1：上一 Tick 总
				// 来流；被动送达发生在自主求解之后，当 Tick live 量对
				// 本 Tick 命令不可见——一拍延迟是「来流之后续上」的
				// 自然语义）。
				hi := cp.LatencyTicks - 1
				if hi < 0 {
					hi = 0
				}
				if hi >= MaxFeedLatency {
					hi = MaxFeedLatency - 1
				}
				inflow := cp.inflowHist[e][hi]
				if inflow <= 0 {
					continue // 无来流不得凭空启动（§26.11）
				}
				sig := fpClamp(fpDiv(inflow, effTP), 0, One)
				if d := sig - cp.LastInputSignal[e]; abs64(d) > cp.Tolerance {
					continue // 来流模式超出教学档位容差：身体「认不出」
				}
				cmd := fpMul(cp.Strength[e], fpMul(inflow, cp.LearnedResponse[e]))
				if cmd <= 0 {
					continue
				}
				tr.Conductance[e] = ctx.cond[e]
				tr.Capacity[e] = ctx.cap[e]
				if actual := resolve(ctx, e, cmd, &tr); actual > 0 {
					tr.Autonomous[e] = actual
					activeFlow[cp.OutChannel][e] += actual // 占道：被动让行（R1）
					fired = true
				}
			}
			if fired {
				reports = append(reports, tr)
			}
		}
	}

	// Part J J2：被动输运——压差驱动的背景环流（与主动请求叠加，见上注释）。
	// 压差快照取本 Tick 边界交换后的负载比（postRatio），与遍历顺序无关。
	// 惯性项（R4）按"输送带"语义：经脉的惯性输入 = 流入其源穴位的实际
	// 流量（节点入流：上一 Tick 流量打底 + 本 Tick 已到达的送达量live累加
	// ——经脉按下标序结算，主环拓扑下恰好沿行进方向逐段接力，环流连续
	// 不间断）。流转受源穴储气份额封顶（见 KQiShare），不会同 Tick 级联
	// 放大。
	nodeIn := make([][ElementCount]int64, len(g.Acupoints))
	for c := range g.Channels {
		ch := &g.Channels[c]
		if ch.To < 0 || ch.To >= len(g.Acupoints) {
			continue
		}
		for e := 0; e < ElementCount; e++ {
			nodeIn[ch.To][e] += ch.FlowPrev[e]
		}
	}
	for c := range g.Channels {
		ch := &g.Channels[c]
		if ch.From < 0 || ch.From >= len(g.Acupoints) || ch.To < 0 || ch.To >= len(g.Acupoints) {
			continue
		}
		pa, pb := postRatio[ch.From], postRatio[ch.To]
		dP := fpMul(pa, pa) - fpMul(pb, pb)
		// 压差只决定梯度项是否驱动（下述元素级判断）；惯性项不受压差门槛，
		// 否则均衡压差下整段经脉被跳过、环流断裂（弛豫振荡的根源）。
		src, dst := &g.Acupoints[ch.From], &g.Acupoints[ch.To]

		chanAdapt := ch.ChannelAdaptation / 1000
		baseCond := fpMul(ch.BaseConductance, chanAdapt)
		condFactor := fpMul(fpMul(src.effectiveOpenness(), dst.effectiveOpenness()), One-ch.Damage)
		chanCap := ch.Throughput
		if s := src.effectiveThroughput(); s < chanCap {
			chanCap = s
		}
		if d := dst.effectiveThroughput(); d < chanCap {
			chanCap = d
		}

		var tr TransferReport
		tr.Channel, tr.From, tr.To = c, ch.From, ch.To
		for e := 0; e < ElementCount; e++ {
			aff := fpSqrt(fpMul(src.effectiveAffinity(e), dst.effectiveAffinity(e)))
			cond := fpMul(fpMul(baseCond, aff), condFactor)
			tr.Conductance[e] = cond
			eff := fpClamp(cond, 0, One)
			pipeCap := fpMul(chanCap, cond) // 与主动流 §8.1 同一管线容量上限

			// 主动占道、被动让行（R1）：被动项只填 §8.1 管线在本元素上的
			// 余量（pipeCap − 主动实际流），与主动流共用同一容量上限——
			// 被动环流是压差均衡化的背景物理（J2"主动 Request 照旧叠加"），
			// 但总实际流不得超过管道容量，否则主动推力被被动旁路放大
			//（白嫖环流）。上游满压、下游有需求时余量输运正是被动项的
			//本职：压差在请求存在的 Tick 同样被均衡化，局部过压不积聚。
			// 压差梯度项（J2）：仅压差为正时驱动（压差为负不逆天然向）。
			var gf int64
			if dP > 0 {
				gf = fpMul(fpMul(fpMul(chanCap, KPassive), dP), cond)
			}
			// 惯性项（R4 第二阶段）：流入源穴位的实际流量沿行进方向衰减
			// 延续（输送带语义，沿天然向；live 入流含本 Tick 上游送达）。
			// 注意惯性项不受压差门槛——均衡压差下环流靠它延续。
			// 自环不接力：惯性输入只计"其他经脉流入源穴"的来流；自环的
			// 自身回流是它自己流量的延续而非新来流，计入即成做功无源的
			// 影子回声（自环又无压差可言，纯添一份损耗流）。
			inertIn := nodeIn[ch.From][e]
			if ch.To == ch.From {
				inertIn -= ch.FlowPrev[e]
			}
			inert := fpMul(KInertia, inertIn)
			// 源穴储气份额封顶：被动流转每 Tick 不得超过源穴存量的
			// KQiShare——压而不泻，防止流转过冲振荡（流量远大于储气时
			// 压差逐 Tick 反转，形成弛豫振荡打穿 §15/§18）。
			qiShare := fpMul(src.Qi[e], KQiShare)
			capPf := pipeCap
			if qiShare < capPf {
				capPf = qiShare
			}
			// 管道余量（R1）：本元素本 Tick 已被主动流占用的容量，被动项
			// 不得复用——总实际流（主动+被动）≤ §8.1 管线容量。
			if spare := pipeCap - activeFlow[c][e]; spare < capPf {
				capPf = spare
				if capPf < 0 {
					capPf = 0
				}
			}
			// 受端份额封顶（0.7.1 标定裁决，Monte Carlo 标定发现的世界语义
			// 修复；与源端「压而不泻」对称的「受不至胀」）：受端每元素每
			// Tick 的被动受流量 ≤ KQiShare ×（该元素存量 + 受端受纳余量）。
			// 归因：惯性项越过压差均衡点继续输运（被动永不逆天然向，无反向
			// 刹车），而源穴储气份额封顶（KQiShare）只闸源端——下游小穴
			// 「进得来、送不出」，被环流缓慢灌过 LR 1.0（应变→稳定崩→永久
			// 损伤）。M1 引气黄金路径（absorb@青关 + guide 青关→丹田）实测
			// 124 tick 内土府 damage 999——无意识参与的开局体验自毁邻穴，
			// 违反 0.6.0 注记④「静养应无伤」世界语义约束在输运维的延伸。
			// 份额分两段：过流段取「存量 × KQiShare」（满穴余量为零，只剩
			// 与源端出流份额同值的过流份额），环流可穿过满穴不衰减（J5 惯性
			// 接力语义保持）；缓充段取「余量 × KRecvShare」：空穴按余量 2%
			// 缓充，负载比渐近 1.0 而不得越过，且缓充速率温和到不打 §15
			// FlowShock——被动流永不构成过载源。门控只作用被动流：主动引气
			// 灌胀脆穴（神庭 large）是 M2 设计的「不适→自主调整」触发面，
			// 不受此门。
			dstCapEff := dst.effectiveCapacity()
			recvCap := fpMul(dst.Qi[e], KQiShare) + fpMul(dstCapEff-dst.load(), KRecvShare)
			pf := fpClamp(gf+inert, 0, capPf)
			if pf > recvCap {
				pf = recvCap
			}
			if pf > src.Qi[e] {
				pf = src.Qi[e]
			}
			if pf <= 0 {
				continue
			}

			// §9 损耗管线（与主动流同管线）。
			delivered := fpMul(pf, eff)
			loss := pf - delivered

			src.Qi[e] -= pf
			dst.Qi[e] += delivered
			dst.heatAccum += fpMul(loss, One-DiffuseShare)
			nodeIn[ch.To][e] += delivered       // live 入流：本 Tick 送达可即段接力
			ts.chanDelivered[c][e] += delivered // 耦合学习的进边来流样本（被动送达同计，0.7.0）

			// Part J J3：接收端途经纳气（被动流同样卷入环境气）。
			entrainInto(ch.To, dst, e, pf)

			src.elemFlowAccum[e] += pf
			dst.elemFlowAccum[e] += delivered
			src.flowAccum += pf
			dst.flowAccum += delivered
			ch.flowAccum += pf
			chanFlow[c][e] += pf

			tr.Passive[e] = pf
			tr.Actual[e] = pf // Actual=被动实际流量（统一"实际流"口径）；Passive 单独归因
			tr.Delivered[e] = delivered
			tr.Loss[e] = loss
		}
		if tr.Passive[Wood] > 0 || tr.Passive[Fire] > 0 || tr.Passive[Earth] > 0 ||
			tr.Passive[Metal] > 0 || tr.Passive[Water] > 0 {
			reports = append(reports, tr)
		}
	}

	// R4：持久流量状态落存（下一 Tick 惯性项的输入；随 World 状态演进，
	// 同 seed 重放逐位一致）。
	for c := range g.Channels {
		g.Channels[c].FlowPrev = chanFlow[c]
	}
	return reports, entrained, ds, ts
}

// runSheng 实现 §10（快照语义：所有转化量按本阶段开始时的 Qi 计算）。
// D4-3：转化量 × (1 + Refine×KTransform)，Refine=0 时乘子=1。
// 返回本 Tick 转化损耗总量（§10.1 剩余 → 热，守恒归账用）。
func runSheng(w *World, refines []int64) int64 {
	total := int64(0)
	for i := range w.Character.BodyGraph.Acupoints {
		a := &w.Character.BodyGraph.Acupoints[i]
		var snap [ElementCount]int64
		copy(snap[:], a.Qi[:])

		for _, src := range shengOrder {
			dst := ShengDst[src]
			if snap[src] <= 0 {
				continue
			}
			localAff := fpSqrt(fpMul(a.effectiveAffinity(src), a.effectiveAffinity(dst)))
			transform := fpMul(fpMul(fpMul(fpMul(snap[src], a.Activity[src]), BaseShengRate), localAff), TimeFactor)
			transform = fpMul(transform, One+fpMul(refines[i], KTransform)) // D4-3
			if transform <= 0 {
				continue
			}
			gain := fpMul(transform, ShengEfficiency)
			a.Qi[src] -= transform
			a.Qi[dst] += gain
			a.heatAccum += transform - gain // §10.1 剩余 → 热
			total += transform - gain
		}
	}
	return total
}

// runKe 实现 §11（固定顺序、活值语义）。
func runKe(w *World) {
	for i := range w.Character.BodyGraph.Acupoints {
		a := &w.Character.BodyGraph.Acupoints[i]
		for counter := 0; counter < ElementCount; counter++ {
			target := KeTarget[counter]
			cq, tq := a.Qi[counter], a.Qi[target]
			if cq <= 0 || tq <= 0 {
				continue
			}
			ratio := fpDiv(cq, cq+tq+KeEpsilon)
			localAff := fpSqrt(fpMul(a.effectiveAffinity(counter), a.effectiveAffinity(target)))
			suppression := fpMul(fpMul(fpMul(BaseKeRate, ratio), a.Activity[counter]), localAff)
			factor := fpClamp(One-suppression, 0, One)
			a.Activity[target] = fpMul(a.Activity[target], factor)
		}
	}
}

// compatibility 实现 §16。空穴位视为完全兼容（避免空载惩罚）。
func compatibility(a *Acupoint, total int64) int64 {
	if total <= 0 {
		return One
	}
	compat := int64(0)
	for e := 0; e < ElementCount; e++ {
		ratio := fpDiv(a.Qi[e], total)
		aff := a.effectiveAffinity(e)
		if aff > One {
			aff = One // §16：>1 的亲和优势不进入 Compatibility
		}
		if aff < 0 {
			aff = 0
		}
		compat += fpMul(ratio, aff)
	}
	return compat
}

// ---------------------------------------------------------------------------
// 确定性噪声（stage_3 §8.2）
// ---------------------------------------------------------------------------

// noiseSigned 返回 (seed, tick, a, b) 决定的 [-1000,1000] 整数。
// 本包自含 splitmix64 终结器混 Hash，不依赖任何全局 RNG，
// 也不与调用方的随机数状态互相干扰。
func noiseSigned(seed uint64, tick int64, a, b int) int64 {
	h := seed ^ 0x9e3779b97f4a7c15
	h = mixCult(h ^ uint64(tick))
	h = mixCult(h ^ uint64(uint32(a))<<32 ^ uint64(uint32(b)))
	h = mixCult(h)
	return int64(h%2001) - 1000
}

// mixCult 是 splitmix64 的三轮移位-乘法终结器。
func mixCult(z uint64) uint64 {
	z ^= z >> 33
	z *= 0xff51afd7ed558ccd
	z ^= z >> 33
	z *= 0xc4ceb9fe1a85ec53
	z ^= z >> 33
	return z
}

func abs64(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}
