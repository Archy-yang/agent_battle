package ctmatch

// match.go —— 修炼会话层（stage_2 §33-34 三级运行时与行为生命周期）。
//
// 编排顺序（每 Tick）：
//
//	意图入队（队列 + 本 Tick 意图）→ 记录意图日志
//	→ L1：有活跃 Protocol 且未暂停 → Runtime 产出 Ops
//	     （出界 → 不推进引擎，升级事件，本 Tick 结束）
//	→ L2：无活跃 Protocol（或被显式意图接管）→ 意图换 Ops
//	→ L0：cult.Advance（引擎不知道 LLM 存在）
//	→ 结算后检查：Protocol guards → 未知异常扫描
//	→ 升级挂起 → 下一 Tick 开始时唤醒 Pilot（带最新观察）
//
// 确定性：同 seed + 同意图流 → 逐位一致（stage_2 §2.2 / M1 铁律 2）。
// 本期先落意图日志（IntentLog）+ Replay 辅助；完整录像/重演沿用
// match 层既有模式，在 WP-4 持久化时接入。

import (
	"encoding/json"
	"fmt"

	"agentbattle/backend/cult"
)

// ---------------------------------------------------------------------------
// 事件与升级（stage_2 §36 / stage_3 §41）
// ---------------------------------------------------------------------------

// EventKind 是会话事件种类。GuardAdjusted / OutOfBoundary / UnknownAnomaly
// 三类是升级事件（stage_2 §36 / stage_3 §41 升级三段：
// 正常 → Guard 调整 → 出界即停上报）。
// 其中 GuardAdjusted(reduce_flow) 属"已知轻微异常→自动调整"，不唤醒 Pilot；
// stop 语义、出界、未知异常才置 TickResult.Escalated（下一 Tick 唤醒 Pilot）。
type EventKind string

const (
	EventGuardAdjusted    EventKind = "guard_adjusted"      // 已知轻微异常：Guard 自动调整
	EventOutOfBoundary    EventKind = "out_of_boundary"     // 出已知边界：不尝试，停并上报
	EventUnknownAnomaly   EventKind = "unknown_anomaly"     // 未知异常：停并上报
	EventProtocolStarted  EventKind = "protocol_started"    // Protocol 启动
	EventProtocolPaused   EventKind = "protocol_paused"     // Protocol 被外部意图接管
	EventProtocolFinished EventKind = "protocol_finished"   // 跑满 Cycles，正常结束
	EventPreconditionFail EventKind = "precondition_failed" // 启动被前置条件拒绝
	EventInvalidIntent    EventKind = "invalid_intent"      // 意图引用不存在的目标，已丢弃
)

// Event 是一条会话事件。
type Event struct {
	Kind     EventKind
	Tick     int64 // 事件发生时的世界 Tick
	Protocol string
	Reason   string
	Detail   map[string]int64
}

// IsEscalation 报告是否为升级事件（三类之一，进会话事件流并可供上层检索）。
func (e Event) IsEscalation() bool {
	switch e.Kind {
	case EventGuardAdjusted, EventOutOfBoundary, EventUnknownAnomaly:
		return true
	}
	return false
}

// TickResult 是一个 Tick 的编排结果。
type TickResult struct {
	Report      *cult.TickReport // L0 引擎报告；出界拒绝推进时为 nil
	Events      []Event          // 本 Tick 新产生的事件
	Receipts    []IntentReceipt  // 本 Tick 的意图回执（铁律 6：每意图恰一条）
	Escalated   bool             // 是否需要唤醒 Pilot（Guard stop / 出界 / 未知异常）
	Advanced    bool             // 引擎是否被推进
	Observation *Observation     // Observe 钩子接入时的最新观察（可能为 nil）
}

// ---------------------------------------------------------------------------
// 会话
// ---------------------------------------------------------------------------

// IntentLogEntry 是一条意图日志（确定性重演的最小基础）。
type IntentLogEntry struct {
	Tick      int64 // 意图提交时的世界 Tick
	FromPilot bool  // 是否来自 Pilot 唤醒
	Intent    Intent
}

// CultSession 是一次修炼会话：世界、Protocol 库、意图队列与 Pilot。
type CultSession struct {
	World *cult.World
	Seed  uint64 // 会话种子（= World.NoiseSeed，控制误差的确定性来源）
	Pilot Pilot  // L2 Agent；可为 nil（纯 Runtime 场景）

	// Observe 是感知观察钩子：由上层接入感知解析器（WP-2 合入后即
	// cult.Observe）。nil 时 Pilot 收到 nil 观察（SPIKE 前的桥接场景）。
	// 本包绝不自行从 World 读真值生成观察——感知隔离律（M1 铁律 1）。
	Observe func(*cult.World) *Observation

	Protocols map[string]*Protocol
	Queue     []Intent // 意图队列：Tick 前入队的意图，先于 Tick 参数意图

	// Ledger 是证据账本（SPIKE 结论二：三态事实凭据，假说校验的依据）；
	// Foundation 是 Loop 五量与筑基判定器（五量只进调试视图）。
	Ledger     *EvidenceLedger
	Foundation *FoundationMonitor

	Events    []Event
	IntentLog []IntentLogEntry

	active        *protocolState
	prevDamage    []int64 // 未知异常扫描用：上一 Tick 各穴位损伤
	lastEscalated bool    // 上一 Tick 是否升级（唤醒 Pilot 的条件）
	evCursor      int     // 事件流消费游标（drainNewEvents 用）

	Trial            *TrialMonitor    // 当前 Runtime 试跑的逐周天记录（stage_3 §28-29）
	lastProtocol     *Protocol        // 最近启动过的功法（筑基后升格 Loop 用）
	prevReport       *cult.TickReport // 上一 Tick 报告（回执压力方向比较用）
	lastReceipts     []IntentReceipt  // 最近一次交出意图的回执组（唤醒 Pilot 时随观察给出）
	loopEngaged      bool             // 筑基后是否已注册 AutonomousLoop（幂等）
	establishedNoted bool             // 筑基事件是否已发（幂等）
}

// NewSession 建会话；seed 同时写入 World.NoiseSeed（0 视为 1）。
func NewSession(w *cult.World, seed uint64, p Pilot) *CultSession {
	if seed == 0 {
		seed = 1
	}
	w.NoiseSeed = seed
	return &CultSession{
		World:      w,
		Seed:       seed,
		Pilot:      p,
		Protocols:  make(map[string]*Protocol),
		Ledger:     NewEvidenceLedger(),
		Foundation: NewFoundationMonitor(),
	}
}

// LoadProtocol 收录一份已编译协议。
func (s *CultSession) LoadProtocol(p *Protocol) { s.Protocols[p.Name] = p }

// LastReceipts 返回最近一次交出意图的回执组（转录/上层接线用；
// Pilot 征询载荷由会话在唤醒时经 ReceiptAware 自动给出）。
func (s *CultSession) LastReceipts() []IntentReceipt { return s.lastReceipts }

// LedgerView 返回证据账本的定性视图（转录/上层接线用）。
func (s *CultSession) LedgerView() []string {
	if s.Ledger == nil {
		return nil
	}
	return s.Ledger.AgentView()
}

// StartProtocol 启动一个 Protocol：先评估前置条件（stage_2 §35
// preconditions），不满足则拒绝启动并记录事件。前置条件需要一个
// 观测基准（World.Last，即至少推进过一个 Tick）。
func (s *CultSession) StartProtocol(name string) error {
	p, ok := s.Protocols[name]
	if !ok {
		return errf("protocol %q not loaded", name)
	}
	if s.World.Last == nil {
		return errf("protocol %q: no observation yet; advance at least one tick before starting", name)
	}
	for _, c := range p.Preconditions {
		if !condTrue(c, s.World.Last) {
			s.pushEvent(Event{
				Kind:     EventPreconditionFail,
				Tick:     s.World.Tick,
				Protocol: name,
				Reason: fmt.Sprintf("precondition unmet: %s %s %d at %s (actual %d)",
					c.Metric, c.Op, c.Value,
					s.World.Character.BodyGraph.Acupoints[c.PointIdx].Name,
					evaluateCond(c, s.World.Last)),
			})
			return errf("protocol %q: preconditions unmet", name)
		}
	}
	s.active = &protocolState{proto: p, flowScale: One}
	s.lastProtocol = p
	s.Trial = newTrialMonitor(p) // 每次 Runtime 试跑都记录逐周天账（stage_3 §28-29）
	s.pushEvent(Event{Kind: EventProtocolStarted, Tick: s.World.Tick, Protocol: name})
	return nil
}

// ResumeProtocol 恢复一个暂停中的 Protocol（Agent 处理完升级后的
// 重启入口）。流缩系数保留——Agent 未显式放宽时维持 Guard 结论。
func (s *CultSession) ResumeProtocol() {
	if s.active != nil {
		s.active.paused = false
	}
}

// ActiveProtocol 返回当前活跃（含暂停）的 Protocol 名；无则空串。
func (s *CultSession) ActiveProtocol() string {
	if s.active != nil && !s.active.done {
		return s.active.proto.Name
	}
	return ""
}

// FlowScale 暴露活跃 Protocol 的流量缩系数（调试/测试用）。
func (s *CultSession) FlowScale() int64 {
	if s.active == nil {
		return One
	}
	return s.active.flowScale
}

func (s *CultSession) pushEvent(e Event) { s.Events = append(s.Events, e) }

func errf(format string, args ...any) error { return fmt.Errorf(format, args...) }

// ---------------------------------------------------------------------------
// 每 Tick 编排
// ---------------------------------------------------------------------------

// Tick 推进一个 Tick。intents 是本 Tick 提交的意图（可为 nil）。
// 返回观察与报告；出界拒绝推进时 Report 为 nil 且引擎未被推进。
// 每 Tick 每意图产出一条回执（铁律 6），随观察一并可供 Pilot 感知。
func Tick(s *CultSession, intents []Intent) *TickResult {
	res := &TickResult{}

	// 0. 意图入队（队列优先，其次本 Tick 参数），全部记录日志。
	pending := append(s.Queue, intents...)
	s.Queue = nil
	for _, it := range pending {
		s.IntentLog = append(s.IntentLog, IntentLogEntry{Tick: s.World.Tick, Intent: it})
	}

	// 0.5 假说登记（stage_2 §26 第六接口；非动作意图，不打断 Runtime）。
	var receipts []IntentReceipt
	for _, it := range pending {
		if it.Kind != IntentHypothesis {
			continue
		}
		s.Ledger.RecordHypothesis(it.Text, s.World.Tick)
		receipts = append(receipts, IntentReceipt{
			Tick: s.World.Tick, Kind: it.Kind,
			Summary: intentSummary(it), Outcome: WordNoted,
		})
	}

	// 1. 升级挂起 → 唤醒 Pilot（stage_2 §34 行为生命周期最后一段）。
	// 征询载荷携带：最新观察 + 上一组意图的回执 + 证据账本视图
	//（全部定性措辞、零数字——铁律 1/6）。
	if s.lastEscalated && s.Pilot != nil {
		if ra, ok := s.Pilot.(ReceiptAware); ok {
			ra.GiveReceipts(s.lastReceipts)
		}
		if la, ok := s.Pilot.(LedgerAware); ok {
			la.GiveLedgerView(s.Ledger.AgentView())
		}
		var obs *Observation
		if s.Observe != nil {
			obs = s.Observe(s.World)
		}
		pi := s.Pilot.Decide(obs)
		for _, it := range pi {
			s.IntentLog = append(s.IntentLog, IntentLogEntry{Tick: s.World.Tick, FromPilot: true, Intent: it})
		}
		pending = append(pending, pi...)
	}
	s.lastEscalated = false

	// 2. 产出本 Tick 的 Ops。
	var ops cult.Ops
	var plans []intentPlan
	runtimePath := false
	if s.active != nil && !s.active.paused && !s.active.done {
		// 外部动作意图 = L2 显式接管（stage_2 §33：L2 处理异常/策略）。
		if hasActionIntent(pending) {
			s.active.paused = true
			s.pushEvent(Event{
				Kind: EventProtocolPaused, Tick: s.World.Tick, Protocol: s.active.proto.Name,
				Reason: "external action intents took over",
			})
		} else {
			// Sense/Inspect/RecordHypothesis 不打断 Runtime；它们只被记录。
			pending = filterNonAction(pending)
			var ev *Event
			ops, ev = s.active.runtimeOps(s)
			if ev != nil {
				// Knowledge Boundary：不尝试。引擎本 Tick 不推进。
				ev.Tick = s.World.Tick
				s.pushEvent(*ev)
				s.lastEscalated = true
				res.Escalated = true
				res.Receipts = s.settleReceipts(pending, nil, res, true)
				s.keepReceipts(res)
				res.Events = s.drainNewEvents()
				return res
			}
			runtimePath = true
			if s.active.done {
				s.pushEvent(Event{
					Kind: EventProtocolFinished, Tick: s.World.Tick, Protocol: s.active.proto.Name,
					Reason: "completed all cycles",
					Detail: map[string]int64{"cycles": int64(s.active.proto.Cycles)},
				})
			}
		}
	}

	if runtimePath {
		wasDone := s.active.done
		s.World.Ops = ops
		s.advanceEngine(res)

		// 试运行逐周天记账（stage_3 §28）。
		if s.Trial != nil && !s.active.loopMode {
			s.Trial.ObserveTick(res.Report)
		}

		// 4. Protocol guards 与未知异常扫描（结算后）。
		if !wasDone {
			guardFired := s.active.runGuards(s, res.Report)
			s.scanAnomalies(res, guardFired)
			if s.active != nil && s.active.paused && !res.Escalated {
				// Guard stop 暂停 → 升级（stage_2 §36 第三段）。
				s.lastEscalated = true
				res.Escalated = true
			}
		} else {
			s.rememberDamage()
		}
		if s.active != nil && s.active.done {
			s.finishTrial(res)
			s.active = nil
		}
	} else {
		// 3. L2 路径：意图 → Ops。
		ops, plans = s.opsFromIntents(pending, res)
		s.World.Ops = ops
		s.advanceEngine(res)
		s.rememberDamage()
	}

	// 5. 回执结算（铁律 6）：每 Tick 每意图恰一条，随观察一并可供 Pilot 感知。
	receipts = append(receipts, s.settleReceipts(pending, plans, res, runtimePath)...)
	res.Receipts = receipts
	s.keepReceipts(res)

	// 6. 证据账本：动作 → 后果方向（settleReceipts 内入账，
	// SPIKE 结论二：三态事实凭据）。

	// 7. Loop 五量与筑基判定（stage_3 §31-39；数值只进调试视图）。
	s.observeFoundation(res, opsHasControl(s.World.Ops))

	if res.Report != nil {
		s.prevReport = res.Report
	}
	res.Events = s.drainNewEvents()
	return res
}

// keepReceipts 更新"上一组意图的回执"：没有新意图就没有新回执，
// 沿用上一组（唤醒 Pilot 时给到的是最近一次交出的那组）。
func (s *CultSession) keepReceipts(res *TickResult) {
	if len(res.Receipts) > 0 {
		s.lastReceipts = res.Receipts
	}
}

// finishTrial 在试跑结束（跑满 Cycles）时做达标判定，达标即转正
// （stage_2 §29 / stage_3 §29：draft → validated）。
func (s *CultSession) finishTrial(res *TickResult) {
	if s.Trial == nil || s.active == nil || s.active.proto != s.Trial.proto {
		return
	}
	sum := s.Trial.Finish()
	if !sum.Qualified {
		return
	}
	s.active.proto.Status = StatusValidated
	s.pushEvent(Event{
		Kind: EventProtocolValidated, Tick: s.World.Tick, Protocol: s.active.proto.Name,
		Reason: "trial cycles held steady with losses well under one in ten and stability never wavering; promoted to validated",
		Detail: map[string]int64{
			"successful_cycles": int64(sum.SuccessfulCycles),
			"total_cycles":      int64(sum.TotalCycles),
			"success_rate":      sum.SuccessRate,
			"avg_min_stability": sum.AvgMinStability,
		},
	})
}

// advanceEngine 推进 L0 引擎（stage_2 §33：L0 绝不调用 LLM）。
func (s *CultSession) advanceEngine(res *TickResult) {
	res.Report = cult.Advance(s.World)
	res.Advanced = true
	if s.Observe != nil {
		res.Observation = s.Observe(s.World)
	}
}

// intentPlan 是一条意图在 L2 路径上的落地计划与评估要点（回执结算用）。
type intentPlan struct {
	it       Intent
	rejected bool   // 预判拒绝（意图无处落地）
	outcome  string // 拒绝回执主词
	detail   string // 拒绝补语

	pointIdx int // absorb 目标穴位下标（评估方向/进气用）
	toIdx    int // guide 目的穴位下标（评估方向用）
	channel  int // guide 经脉下标（核对实际流量用）
}

// opsFromIntents 把意图换算为 Ops（Intent → Intensity/FlowRequest），
// 同时为每条意图记下落地计划（回执在引擎结算后统一给出——铁律 6）。
// 意图引用未知目标时丢弃并记 EventInvalidIntent——Engine 决定实际结果，
// 也包括决定"这个意图无处落地"。
func (s *CultSession) opsFromIntents(intents []Intent, res *TickResult) (cult.Ops, []intentPlan) {
	g := &s.World.Character.BodyGraph
	nameIdx := pointIndex(g)
	ops := cult.Ops{Intensity: make([]int64, len(g.Acupoints))}
	plans := make([]intentPlan, 0, len(intents))

	drop := func(it Intent, why string) {
		s.pushEvent(Event{
			Kind: EventInvalidIntent, Tick: s.World.Tick,
			Reason: why + " (" + it.Kind.String() + ")",
		})
	}

	for _, it := range intents {
		plan := intentPlan{it: it, pointIdx: -1, toIdx: -1, channel: -1}
		switch it.Kind {
		case IntentAbsorb:
			idx, ok := nameIdx[it.Acupoint]
			if !ok {
				drop(it, "unknown acupoint "+it.Acupoint)
				plan.rejected, plan.outcome, plan.detail = true, WordRejected, WhyNoAcupoint
				plans = append(plans, plan)
				continue
			}
			if it.Level == LevelUnset {
				drop(it, "absorb without level")
				plan.rejected, plan.outcome, plan.detail = true, WordUnclear, WhyNoLevel
				plans = append(plans, plan)
				continue
			}
			plan.pointIdx = idx
			ops.Intensity[idx] = IntensityForLevel(it.Level)

		case IntentGuide:
			from, ok1 := nameIdx[it.From]
			to, ok2 := nameIdx[it.To]
			if !ok1 || !ok2 {
				drop(it, "unknown acupoint in guide")
				plan.rejected, plan.outcome, plan.detail = true, WordRejected, WhyNoAcupoint
				plans = append(plans, plan)
				continue
			}
			// 边解析（修订包 C1/A2）：天然向为顺流；反向则视为逆流意图，
			// 发起端须是核点（A1 v0.2：Capacity 最大穴位，唯丹田可发起外推）
			// ——否则走"此路不通"回执路径，产出定性回执，引擎不接触逆流请求。
			ch, rev, found := edgeFor(g, from, to)
			if !found {
				drop(it, "no channel "+it.From+"->"+it.To)
				plan.rejected, plan.outcome, plan.detail = true, WordRejected, WhyNoChannel
				plans = append(plans, plan)
				continue
			}
			if rev && !isCorePoint(g, from) {
				drop(it, "reverse flow denied "+it.From+"->"+it.To)
				plan.rejected, plan.outcome, plan.detail = true, WordRejected, WhyNotBackflow
				plans = append(plans, plan)
				continue
			}
			if it.Element < -1 || it.Element >= cult.ElementCount {
				drop(it, "bad element")
				plan.rejected, plan.outcome, plan.detail = true, WordUnclear, WhyBadElement
				plans = append(plans, plan)
				continue
			}
			flow := it.Amount
			if it.Amount <= 0 {
				if it.Level == LevelUnset {
					drop(it, "guide without level or amount")
					plan.rejected, plan.outcome, plan.detail = true, WordUnclear, WhyNoFlow
					plans = append(plans, plan)
					continue
				}
				flow = FlowForLevel(it.Level, g, from, to, ch)
			}
			plan.channel, plan.toIdx = ch, to
			req := cult.FlowRequest{Channel: ch, Reverse: rev}
			if it.Element >= 0 {
				req.Amount[it.Element] = flow
			} else {
				for e := 0; e < cult.ElementCount; e++ {
					req.Amount[e] = fpMul(flow, 200)
				}
			}
			ops.Requests = append(ops.Requests, req)

		case IntentStop:
			// 停：清空此前累积的意图效果（含活跃 Protocol 的暂停）。
			ops = cult.Ops{Intensity: make([]int64, len(g.Acupoints))}
			if s.active != nil && !s.active.done {
				s.active.paused = true
			}

		case IntentSense, IntentInspect, IntentHypothesis:
			// 不产生 Ops；观察由 Observe 钩子在结算后给出，假说已登记。
		}
		plans = append(plans, plan)
	}
	return ops, plans
}

// settleReceipts 在引擎结算后为每条意图给出回执（铁律 6）：
//   - 落地类：已生效 + 粗档后果方向（压力升/降/不变——只给方向词）；
//   - 拒绝类：此路不通（目标穴位/经脉不存在）/ 穴未开或满溢不入 /
//     意图不明。
//
// runtimePath 时 intents 只有感知/内视/假说（动作已被过滤），plans 为 nil。
func (s *CultSession) settleReceipts(intents []Intent, plans []intentPlan, res *TickResult, runtimePath bool) []IntentReceipt {
	out := make([]IntentReceipt, 0, len(intents))
	for i, it := range intents {
		switch it.Kind {
		case IntentSense, IntentInspect:
			out = append(out, IntentReceipt{
				Tick: s.World.Tick, Kind: it.Kind,
				Summary: intentSummary(it), Outcome: WordApplied,
			})
			continue
		case IntentStop:
			out = append(out, IntentReceipt{
				Tick: s.World.Tick, Kind: it.Kind,
				Summary: intentSummary(it), Outcome: WordApplied, Direction: WordPressureSame,
			})
			continue
		case IntentHypothesis:
			continue // 已在 Tick 开头给过回执（WordNoted）
		case IntentAbsorb, IntentGuide:
			// 走计划评估。
		}
		if runtimePath || i >= len(plans) {
			continue // Runtime 路径上不该出现动作意图；保守不给回执
		}
		plan := plans[i]
		if plan.rejected {
			rc := IntentReceipt{
				Tick: s.World.Tick, Kind: it.Kind,
				Summary: intentSummary(it), Outcome: plan.outcome, Detail: plan.detail,
			}
			out = append(out, rc)
			s.recordEvidence(it, rc)
			continue
		}
		switch it.Kind {
		case IntentAbsorb:
			intake := int64(0)
			if res.Report != nil && plan.pointIdx >= 0 && plan.pointIdx < len(res.Report.Points) {
				for e := 0; e < cult.ElementCount; e++ {
					intake += res.Report.Points[plan.pointIdx].Intake[e]
				}
			}
			if intake <= 0 {
				rc := IntentReceipt{
					Tick: s.World.Tick, Kind: it.Kind,
					Summary: intentSummary(it), Outcome: WordBlocked, Detail: WhyNotTakenIn,
				}
				out = append(out, rc)
				s.recordEvidence(it, rc)
				continue
			}
			rc := IntentReceipt{
				Tick: s.World.Tick, Kind: it.Kind,
				Summary:   intentSummary(it),
				Outcome:   WordApplied,
				Direction: pressureDirection(res.Report, s.prevReport, plan.pointIdx),
			}
			out = append(out, rc)
			s.recordEvidence(it, rc)
		case IntentGuide:
			moved := int64(0)
			if res.Report != nil && plan.channel >= 0 {
				for ti := range res.Report.Transfers {
					tr := &res.Report.Transfers[ti]
					if tr.Channel != plan.channel {
						continue
					}
					for e := 0; e < cult.ElementCount; e++ {
						moved += tr.Actual[e]
					}
				}
			}
			if moved <= 0 {
				rc := IntentReceipt{
					Tick: s.World.Tick, Kind: it.Kind,
					Summary: intentSummary(it), Outcome: WordBlocked, Detail: WhyNotTakenIn,
				}
				out = append(out, rc)
				s.recordEvidence(it, rc)
				continue
			}
			rc := IntentReceipt{
				Tick: s.World.Tick, Kind: it.Kind,
				Summary:   intentSummary(it),
				Outcome:   WordApplied,
				Direction: pressureDirection(res.Report, s.prevReport, plan.toIdx),
			}
			out = append(out, rc)
			s.recordEvidence(it, rc)
		}
	}
	return out
}

// recordEvidence 把动作意图的回执记入证据账本（SPIKE 结论二：账本是
// "动作 → 观察到的后果方向"的事实凭据；感知/内视/假说不入账）。
func (s *CultSession) recordEvidence(it Intent, r IntentReceipt) {
	key := ActionKey(it)
	if key == "" {
		return
	}
	outcome := ""
	if r.Outcome == WordApplied {
		outcome = r.Direction
	}
	s.Ledger.Record(EvidenceEntry{
		Tick:    r.Tick,
		Action:  key,
		Summary: r.Summary,
		Receipt: r.Outcome,
		Outcome: outcome,
	})
}

// scanAnomalies 在结算后扫描未知异常（stage_2 §36 第三段）：
// 无 Guard 覆盖却出现了新损伤 → Runtime 无法解释，停并升级。
func (s *CultSession) scanAnomalies(res *TickResult, guardFired bool) {
	if s.prevDamage == nil {
		s.rememberDamage()
		return
	}
	rep := res.Report
	for i := range rep.Points {
		if rep.Points[i].Damage > s.prevDamage[i] {
			prev := s.prevDamage[i]
			s.rememberDamage()
			if !guardFired {
				name := s.World.Character.BodyGraph.Acupoints[i].Name
				s.active.paused = true
				if s.Trial != nil && s.Trial.proto == s.active.proto {
					s.Trial.Record.UnknownAnomaly = true // §29：转正要求没有未知异常
				}
				s.pushEvent(Event{
					Kind:     EventUnknownAnomaly,
					Tick:     s.World.Tick,
					Protocol: s.active.proto.Name,
					Reason: fmt.Sprintf("new damage appeared at %s (%d -> %d) with no guard fired; runtime cannot explain, escalating to agent",
						name, prev, rep.Points[i].Damage),
					Detail: map[string]int64{"acupoint": int64(i), "damage": rep.Points[i].Damage, "prev": prev},
				})
				s.lastEscalated = true
				res.Escalated = true
			}
			return
		}
	}
	s.rememberDamage()
}

func (s *CultSession) rememberDamage() {
	if s.World.Last == nil {
		return
	}
	s.prevDamage = make([]int64, len(s.World.Last.Points))
	for i := range s.World.Last.Points {
		s.prevDamage[i] = s.World.Last.Points[i].Damage
	}
}

func (s *CultSession) drainNewEvents() []Event {
	// 返回自上次消费以来的全部新事件（会话事件流保留全量）。
	if s.evCursor > len(s.Events) {
		s.evCursor = len(s.Events)
	}
	cur := s.evCursor
	s.evCursor = len(s.Events)
	return s.Events[cur:]
}

func hasActionIntent(intents []Intent) bool {
	for _, it := range intents {
		switch it.Kind {
		case IntentAbsorb, IntentGuide, IntentStop:
			return true
		}
	}
	return false
}

// opsHasControl 报告一份 Ops 是否含主动推动（吸收/引流）——筑基判定的
// SelfSustain 观察窗据此区分"推动"与"松手"（stage_3 §34）。
func opsHasControl(ops cult.Ops) bool {
	for _, v := range ops.Intensity {
		if v > 0 {
			return true
		}
	}
	return len(ops.Requests) > 0
}

func filterNonAction(intents []Intent) []Intent {
	out := intents[:0]
	for _, it := range intents {
		switch it.Kind {
		case IntentSense, IntentInspect, IntentHypothesis:
			out = append(out, it)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// 确定性重演辅助
// ---------------------------------------------------------------------------

// MarshalReports 把一批报告序列化为 JSON（测试/录像用；同输入必同字节）。
func MarshalReports(reps []*cult.TickReport) ([]byte, error) {
	return json.Marshal(reps)
}

// fmtErrorf 已并入 errf（统一走 fmt.Errorf）。
