package cult

// feedback.go —— 局部反馈耦合（FeedbackCoupling）与教师/误差学习（0.7.0，
// 定稿 docs/specs/2026-09-26-engine-drive-foundation-v1.md 阶段三第一块；
// 公式来源：评审稿 docs/cultivation_autonomous_loop_design_v0.2.md
// §7 反馈耦合是控制关系不是能量来源、§12 教师/误差学习、§13 自主命令、
// §24.3 数据结构示意）。
//
// 世界语义（评审稿 §31）：FeedbackCoupling 是身体在稳定教学样本中学会的
// 局部「来流 → 续推」输入输出关系。它是 Sensor → Control 映射，不是能源
// （§7.2 MUST NOT 直接产功）：耦合只产生响应命令 AutonomousCommand，命令
// 必须经 runTransfers 的统一驱动求解（驱动容量限幅、可用能量收敛、做功、
// 废热应变）才是真实自主驱动——耦合不是隐藏能源（§28.2）。
//
// 局部性（§8/§26.19）：耦合只挂在「进边 → 出边」的局部转移上（某穴有
// 稳定来流 → 该穴沿其出边续推），引擎状态中不存在整条周天路径的耦合值
// ——路径属于知识与协议层。
//
// 学习纪律（§12）：禁止按次数成长（没有误差样本就没有更新）；教师门
// 饱和——猛推不加速学习（§12.3/§26.8）；误差驱动收敛——预测逼近目标时
// 更新量自然趋零（§12.5-12.9/§26.9）；安全门——失稳即停学（§12.7/§26.10）。

// FeedbackCoupling 是一个穴位上的局部转移耦合（评审稿 §24.3 定点化）。
// 挂在 Acupoint.Couplings 上，键为（进边经脉, 出边经脉）下标对——只描述
// 「进边来流 → 出边续推」这一条局部转移，逐元素独立学习。
type FeedbackCoupling struct {
	// InChannel / OutChannel 是进边（To = 本穴）与出边（From = 本穴）的
	// 经脉下标（评审稿 §24.3 InputChannelID/OutputChannelID 的定点实现；
	// 自转移 ic == oc 不成耦合）。使用时按当前图校验端点：图谱被上层改造
	// （如测试砍边）后失效耦合静默休眠，不凭空改指。
	InChannel  int
	OutChannel int

	// Strength[e] 是耦合强度（千分位 ∈ [0,1000]；评审稿 §24.3 Strength）：
	// 「来流 → 续推」映射的成形度。误差驱动更新（§12.5-12.8）。
	Strength [ElementCount]int64
	// InputSignal[e] 是归一化输入信号的 EMA（评审稿 §12.2：clamp(来流/
	// max(Throughput, ε), 0, 1) 后降噪），每 Tick 随真实来流滚动。
	InputSignal [ElementCount]int64
	// LearnedResponse[e] 是习得响应（千分位；评审稿 §24.3 LearnedResponse，
	// §12.4 TargetResponse 的稳定估计）：教学样本中「教学出流 / 来流」
	// 比值在教师门与安全门约束下的 EMA——身体学到的「该续推多少」。
	LearnedResponse [ElementCount]int64
	// Tolerance 是来流容差（评审稿 §24.3 Tolerance / §26.11）：归一化输入
	// 信号偏离教学档位记忆超过它即「认不出」，不产生自主命令。
	Tolerance int64
	// LatencyTicks 是响应延迟（评审稿 §24.3 LatencyTicks）：1 = 响应上一
	// Tick 的进边来流（「来流之后」一拍续推）；d > 1 时命令读取更早的
	// 来流历史档。
	LatencyTicks int

	// LastInputSignal[e] 是教学档位记忆（评审稿 §24.3 LastInputSignal）：
	// 教师门开启时向当期输入信号收敛的 EMA——「教学时的来流档位」，容差
	// 判别的参考基准。
	LastInputSignal [ElementCount]int64
	// LastError[e] 是最近一次教学样本的学习误差（评审稿 §24.3 LastError，
	// §12.6：目标 − 预测；诊断/测试观察用，不回写世界）。
	LastError [ElementCount]int64

	// inflowHist[e][k] 是进边来流的滚动历史：k=0 为上一 Tick 的进边天然向
	// 送达量（被动 + 主动 + 自主同计），k 越大越旧（LatencyTicks = d 的
	// 命令取 hist[e][d-1]）。私有中间态：随 World 状态演进，同 seed 重放
	// 逐位一致。
	inflowHist [ElementCount][MaxFeedLatency]int64
}

// newFeedbackCoupling 按评审稿 §24.3 语义建一条局部转移耦合（参数取
// [v0.1 参数] 默认档）。
func newFeedbackCoupling(inChannel, outChannel int) FeedbackCoupling {
	return FeedbackCoupling{
		InChannel:    inChannel,
		OutChannel:   outChannel,
		Tolerance:    FeedTolerance,
		LatencyTicks: DefaultFeedLatency,
	}
}

// validFor 校验耦合在当前身体图上仍然成立：进边仍流入本穴、出边仍从本穴
// 流出。图谱被上层改造（砍边/重排下标）后失效的耦合静默休眠。
func (cp *FeedbackCoupling) validFor(g *BodyGraph, node int) bool {
	if cp.InChannel < 0 || cp.InChannel >= len(g.Channels) ||
		cp.OutChannel < 0 || cp.OutChannel >= len(g.Channels) {
		return false
	}
	return g.Channels[cp.InChannel].To == node && g.Channels[cp.OutChannel].From == node
}

// teacherGateOf 计算教师门（评审稿 §12.3 定点实现）：
//
//	TeacherGate = saturate(手动驱动实流 / TeacherDriveReference)
//
// 手动驱动太低 → 教学信号不清晰；达到教学强度 → 接近 1；继续猛推 →
// 不再增长（饱和）——「用力十倍、内化十倍快」在门上就被封死（§26.8）。
func teacherGateOf(manualOut int64) int64 {
	if manualOut <= 0 {
		return 0
	}
	return fpClamp(fpDiv(manualOut, TeacherDriveRef), 0, One)
}

// safetyGateOf 计算安全门（评审稿 §12.7 定点实现，各量先 clamp 到合法域）：
//
//	SafetyGate = Stability × Compatibility × (1 − Damage) × (1 − OverStrain)
//
// OverStrain 取本 Tick 穴位应变（含过驱/突变/冲击/热）超出安全带的归一
// 份额：应变满 1.0 即完全抑制学习。失稳（稳定↓/损伤↑/过应变↑）→ 门↓ →
// 停学（§26.10）。
func safetyGateOf(stability, compatibility, damage, strain int64) int64 {
	overStrain := fpClamp(fpDiv(strain, One), 0, One)
	g := fpMul(fpClamp(stability, 0, One), fpClamp(compatibility, 0, One))
	g = fpMul(g, One-fpClamp(damage, 0, One))
	g = fpMul(g, One-overStrain)
	return fpClamp(g, 0, One)
}

// teachSamples 汇集一次 Advance 的教学样本（runTransfers 产出，学习遍
// 消费）：逐经脉的天然向送达量（进边来流）与手动驱动实流（教学出流）。
type teachSamples struct {
	// chanDelivered[c][e] 是经脉 c 本 Tick 沿天然向送达 To 端的实流合计
	// （主动 + 自主 + 被动；逆流送达落在 From 端，不构成任何穴的天然向
	// 来流，不计入）。
	chanDelivered [][ElementCount]int64
	// manualOut[c][e] 是经脉 c 本 Tick 手动驱动（顺流请求）从驱动端发出
	// 的实流合计——教师样本的「受控教学输出」（评审稿 §12.3：ManualDrive
	// 的作用是宣告本次输出是教学样本）。逆流请求是调度不是续推示范，
	// 不计入。
	manualOut [][ElementCount]int64
}

// updateFeedbackCouplings 以本 Tick 的真实输入/输出样本更新全部耦合
// （评审稿 §12 全公式定点化；§23 引擎 Tick 步骤 15：在状态结算后进行，
// 安全门读取本 Tick 的稳定性/相容/损伤/应变）。
//
//	输入信号 raw   = clamp(进边送达 / max(穴位有效通量, ε), 0, 1)   （§12.2）
//	教师门 gate    = saturate(手动驱动实流 / TeacherDriveRef)        （§12.3）
//	目标响应 target = clamp(教学出流 / max(来流, ε), 0, FeedResponseMax)（§12.4）
//	预测 predicted = 强度 × 习得响应                                  （§12.5）
//	误差 error    = target − predicted                                （§12.6）
//	安全门 safety = 稳定 × 相容 × (1−损伤) × (1−过应变)               （§12.7）
//	Δ强度         = AdaptRate × gate × InputSignal × safety × error   （§12.8）
//
// 无教学样本的 Tick（无来流或无手动驱动）只滚动输入信号 EMA 与来流历史
// ——耦合不衰减也不成长（§12.1 禁次数成长）。习得响应与教学档位记忆
// 同样只在教学样本上收敛，且同受安全门约束——失稳的样本不污染已学。
func updateFeedbackCouplings(w *World, rep *TickReport, ts *teachSamples) {
	g := &w.Character.BodyGraph
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
			for e := 0; e < ElementCount; e++ {
				// 来流历史滚动（LatencyTicks > 1 的命令输入源）。
				for d := MaxFeedLatency - 1; d >= 1; d-- {
					cp.inflowHist[e][d] = cp.inflowHist[e][d-1]
				}
				inflow := ts.chanDelivered[cp.InChannel][e]
				cp.inflowHist[e][0] = inflow

				// 输入信号 EMA（§12.2）：每 Tick 随真实来流滚动，无论是否
				// 在教学——它是身体对来流模式的持续观察。
				raw := fpClamp(fpDiv(inflow, effTP), 0, One)
				cp.InputSignal[e] += fpMul(KSigEMA, raw-cp.InputSignal[e])

				// 教师门（§12.3）：无受控教学输出则本 Tick 无样本。
				gate := teacherGateOf(ts.manualOut[cp.OutChannel][e])
				if gate <= 0 || inflow <= 0 {
					continue
				}
				safety := safetyGateOf(rep.Points[i].Stability, rep.Points[i].Compatibility,
					rep.Points[i].Damage, rep.Points[i].Strain)

				// 目标响应（§12.4）与习得响应收敛：只学安全的受控样本。
				target := fpClamp(fpDiv(ts.manualOut[cp.OutChannel][e], inflow), 0, FeedResponseMax)
				cp.LastInputSignal[e] += fpMul(KSigEMA, raw-cp.LastInputSignal[e])
				lr := cp.LearnedResponse[e] + fpMul(fpMul(KFeedResp, gate), fpMul(safety, target-cp.LearnedResponse[e]))
				cp.LearnedResponse[e] = fpClamp(lr, 0, FeedResponseMax)

				// 误差驱动更新（§12.5-12.8）：预测逼近目标后误差趋零，
				// 更新量自然趋零（§12.9）；过冲时误差反号自然回修。
				predicted := fpMul(cp.Strength[e], cp.LearnedResponse[e])
				err := target - predicted
				cp.LastError[e] = err
				delta := fpMul(KFeedAdapt, fpMul(gate, fpMul(cp.InputSignal[e], fpMul(safety, err))))
				cp.Strength[e] = fpClamp(cp.Strength[e]+delta, 0, One)
			}
		}
	}
}

// feedSenseWord 返回反馈耦合的定性感知词（评审稿 §22 感知映射；感知
// 隔离律：零数字零机器词，且只在「已内化 + 该转移确有来流」时给出——
// 未被实验揭示的结构不预 render）。返回 "" 表示无可感反馈。
func feedSenseWord(a *Acupoint) string {
	if len(a.Couplings) == 0 {
		return ""
	}
	best := int64(0)
	for k := range a.Couplings {
		cp := &a.Couplings[k]
		for e := 0; e < ElementCount; e++ {
			// 有来流（上一 Tick 进边送达）且强度达可感门槛才显形。
			if cp.inflowHist[e][0] > 0 && cp.Strength[e] > best {
				best = cp.Strength[e]
			}
		}
	}
	if best < FeedSenseBand {
		return ""
	}
	if best < 800 {
		return "来流之后，气息似会自行续上前行"
	}
	return "此处的气息已能自行接续前行，不待牵引"
}
