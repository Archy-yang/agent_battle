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
	"strings"

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
	Advice      []string         // 转向提示（M2 WP-M2-3：僵局时的定性附言，
	// 零数字零机器词；无僵局时为空。随回执/观察附加语呈现给上层与 Pilot）
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

	// —— 认知辅助：无效重复检测（M2 WP-M2-3，B2 探索固执）——
	// 全部是会话侧确定性记账（同回执序列必得同提示）；分身侧只见定性词、
	// 零数字（计数与阈值绝不外泄）。DisableStallAdvice 供 MC 对照实验
	// 开关检测（生产默认开启）。
	DisableStallAdvice bool

	stallKey     string   // 当前连续 streak 的行动键（同语义动作）
	stallSig     string   // 最近回执签名（主词|补语|方向）
	stallCount   int      // 连续同质回执计数（工程口径，绝不进分身侧）
	stallAdvised bool     // 本轮僵局是否已生成转向提示
	advice       []string // 活跃的转向提示（僵局解除即清，给空即撤）
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

	// 1. 升级挂起 → 唤醒 Pilot（stage_2 §34 行为生命周期最后一段）。
	// 征询载荷携带：最新观察 + 上一组意图的回执 + 证据账本视图
	//（全部定性措辞、零数字——铁律 1/6），外加认知层的转向提示
	//（M2 WP-M2-3：僵局持续期间每次唤醒重给，解除时给空——是建议，
	// 不是强制；分身可以听，也可以固执）。
	if s.lastEscalated && s.Pilot != nil {
		advice := s.currentAdvice()
		if aa, ok := s.Pilot.(AdviceAware); ok {
			aa.GiveAdvice(advice)
		}
		if ra, ok := s.Pilot.(ReceiptAware); ok {
			ra.GiveReceipts(s.lastReceipts)
		}
		if la, ok := s.Pilot.(LedgerAware); ok {
			lv := s.Ledger.AgentView()
			if len(advice) > 0 {
				// 僵局期间在账本视图中凸显未试方向（只基于分身自己的
				// 尝试记录推导，零泄露——knowledge.go UntriedDirections）。
				for _, d := range s.Ledger.UntriedDirections() {
					lv = append(lv, "未试方向："+d)
				}
			}
			la.GiveLedgerView(lv)
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

	// 1.5 记账类意图结算（非动作意图，不打断 Runtime）：假说登记（stage_2
	// §26 第六接口）；功法草案提交与起功试跑（M2 WP-M2-2——分身只提交
	// Intent，编译校验/起功资格/固化由会话层与世界法则裁决）。放在唤醒
	// 之后统一结算：分身本息交出的记账类意图与队列/参数意图同一口径，
	// 每条恰一回执（修正旧序中分身假说拿不到回执的缺口）。
	var receipts []IntentReceipt
	var kept []Intent
	for _, it := range pending {
		switch it.Kind {
		case IntentHypothesis:
			s.Ledger.RecordHypothesis(it.Text, s.World.Tick)
			receipts = append(receipts, IntentReceipt{
				Tick: s.World.Tick, Kind: it.Kind,
				Summary: intentSummary(it), Outcome: WordNoted,
			})
		case IntentProtocolDraft:
			receipts = append(receipts, s.settleProtocolDraft(it))
		case IntentStartProtocol:
			receipts = append(receipts, s.settleProtocolStart(it))
		default:
			kept = append(kept, it)
		}
	}
	pending = kept

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
				// 本 Tick 已落的记账类回执（假说/草案/起功）一并保留
				//（铁律 6：每意图恰一条，升级 Tick 不吞回执）。
				res.Receipts = append(receipts, s.settleReceipts(pending, nil, res, true)...)
				s.keepReceipts(res)
				res.Advice = s.currentAdvice()
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

	var conclusion IntentReceipt
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
			conclusion = s.trialConclusion() // 试跑总结回执：固化或修订交回分身（M2 WP-M2-2）
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
	// 试跑总结回执（若有）排在记账类回执之后、动作回执之前。
	if conclusion.Tick > 0 || conclusion.Summary != "" {
		receipts = append(receipts, conclusion)
	}
	receipts = append(receipts, s.settleReceipts(pending, plans, res, runtimePath)...)
	res.Receipts = receipts
	s.keepReceipts(res)
	// 转向提示随本 Tick 结果一并给出（M2 WP-M2-3：以回执/观察附加语
	// 形式呈现；给 Pilot 的注入在下一息唤醒路径统一做）。
	res.Advice = s.currentAdvice()

	// 6. 证据账本：动作 → 后果方向（settleReceipts 内入账，
	// SPIKE 结论二：三态事实凭据）。

	// 7. Loop 五量与筑基判定（stage_3 §31-39；数值只进调试视图）。
	// ControlActive 按主会话裁决（2026-09-26）：本 Tick 台账存在手动驱动
	// 做功（ManualWork > 0——玩家意图与协议的主动操作都表现为手动驱动）
	// 或存在主动请求（吸收强度/经脉请求，含被引擎拒绝者——发了请求即控制）。
	// 自主驱动与被动流不经 Ops、永不计入控制；Protocol Runtime 是否在执行
	// 不再单独计入——它是否构成控制由它实际发出的手动驱动与请求判读。
	s.observeFoundation(res, opsHasControl(s.World.Ops) ||
		(res.Report != nil && res.Report.Work.ManualWork > 0))

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

// ---------------------------------------------------------------------------
// 认知辅助：无效重复检测与转向提示（M2 WP-M2-3，B2 探索固执）
// ---------------------------------------------------------------------------

// 无效重复检测的工程参数（会话侧阈值；分身侧只见"几次"等定性词，
// 阈值与计数绝不进任何 Agent 可见载荷——任务书第 5 条口径）。
const (
	// stallAdviceAfter：同一行动键连续得到同质回执的次数达到该值即生成
	// 转向提示（"连续多次"的口径：含首次，即连续第 3 条同质回执触发）。
	stallAdviceAfter = 3
	// stallAdviceMaxUntried：转向提示中最多列举的未试方向条数（防刷屏）。
	stallAdviceMaxUntried = 4
)

// noteStall 更新无效重复检测（会话/认知层，非引擎物理）。口径：
//   - "同语义动作"= 同一行动键（ActionKey：档位/属性/流量的变化不改键
//     ——换了力度还撞同一堵墙，仍是撞墙）；
//   - "同质回执"= 回执主词 + 补语 + 方向词全部与上一条相同；
//   - "无效"= 被拒 / 不入 / 意图不明，或已生效而压力不变；已生效且压力
//     有方向回应（升/降）不算固执——那是节律，不是撞墙，反而清账。
//
// streak 单槽：以最近一条动作回执为锚（任务书"同意图连续多次"的口径；
// 两种僵局动作交替的情形不在本期覆盖，见 WP-M2-3 遗留）。全部确定性：
// 同回执序列必得同提示（重演一致性的组成部分）。
func (s *CultSession) noteStall(key string, r IntentReceipt) {
	if key == "" || s.DisableStallAdvice {
		return
	}
	if r.Outcome == WordApplied && r.Direction != WordPressureSame {
		s.clearStall() // 身体在回应：这是节律，不是固执
		return
	}
	sig := r.Outcome + "|" + r.Detail + "|" + r.Direction
	if key != s.stallKey || sig != s.stallSig {
		s.stallKey, s.stallSig, s.stallCount, s.stallAdvised = key, sig, 1, false
		s.advice = nil
		return
	}
	s.stallCount++
	if s.stallCount >= stallAdviceAfter && !s.stallAdvised {
		s.stallAdvised = true
		s.advice = s.buildAdvice(key)
	}
}

// clearStall 清除僵局计数与活跃提示。
func (s *CultSession) clearStall() {
	s.stallKey, s.stallSig, s.stallCount, s.stallAdvised = "", "", 0, false
	s.advice = nil
}

// currentAdvice 返回当前活跃的转向提示（副本；无僵局时为 nil）。
func (s *CultSession) currentAdvice() []string {
	if len(s.advice) == 0 {
		return nil
	}
	out := make([]string, len(s.advice))
	copy(out, s.advice)
	return out
}

// buildAdvice 生成一条转向提示（分身自述口吻的定性附言，零数字零
// 机器词）。未试方向取自分身自己的账本（UntriedDirections：只回喂
// 分身说过的名字，不泄露未探明事实）；账本里推不出新路时，给定性
// 的换法建议而非具体方向。
func (s *CultSession) buildAdvice(key string) []string {
	label := s.Ledger.labelOf(key)
	lines := []string{
		"「" + label + "」这条路你已试过几次，气息的回应并无不同。可曾想过换一个方向？",
	}
	untried := s.Ledger.UntriedDirections()
	if len(untried) > stallAdviceMaxUntried {
		untried = untried[:stallAdviceMaxUntried]
	}
	if len(untried) > 0 {
		lines = append(lines, "未试方向："+strings.Join(untried, "；")+"。")
	} else {
		lines = append(lines, "账本里还没有现成的新路——可换个力度、换个次序，或先静静感受，再看气息的回应。")
	}
	return lines
}

// finishTrial 在试跑结束（跑满 Cycles）时做达标判定，达标即转正
// （stage_2 §29 / stage_3 §29：draft → validated），并固化在案（M2
// WP-M2-2：进台账，成为可复用的功法；同名草案此后不可覆盖）。
func (s *CultSession) finishTrial(res *TickResult) {
	if s.Trial == nil || s.active == nil || s.active.proto != s.Trial.proto {
		return
	}
	sum := s.Trial.Finish()
	if !sum.Qualified {
		return
	}
	p := s.active.proto
	p.Status = StatusValidated
	s.Ledger.Solidify(p.Name, protocolPath(p, &s.World.Character.BodyGraph))
	s.pushEvent(Event{
		Kind: EventProtocolValidated, Tick: s.World.Tick, Protocol: p.Name,
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
		case IntentSense:
			out = append(out, IntentReceipt{
				Tick: s.World.Tick, Kind: it.Kind,
				Summary: intentSummary(it), Outcome: WordApplied,
			})
			continue
		case IntentInspect:
			// 凝神细察（渐进感知修订包 §1，specs/2026-10-18-progressive-
			// perception-v1.md）：熟识度 +3（cult.InspectFame）；凝神无感
			// 之穴 → 回执「无从察起」（凝神之功仍在——熟识度照涨，次息
			// 方见）。称谓解析：显名之穴用真名，未显名之穴以方位语指称
			//（§4 零泄名口径）——玩家即以真名凝神未显名之穴，回执亦不落
			// 真名；称谓取凝神前档位（本息越显名阈者，其名由下息的显名
			// 预算授予，本息仍以方位语称）。
			label := it.Acupoint
			if idx, ok := resolvePointLabel(&s.World.Character.BodyGraph, it.Acupoint); ok {
				pre := cult.InspectFame(s.World, idx)
				// 称谓从穴位真名出发按档选定：意图里的 target 可能本身
				// 就是方位语（分身的观察口径），不能直接再过方位语映射
				//（否则退化成「内视某处」）。
				name := s.World.Character.BodyGraph.Acupoints[idx].Name
				if pre < cult.FameNamed {
					label = cult.RegionPhrase(name)
				} else {
					label = name
				}
				rc := IntentReceipt{
					Tick: s.World.Tick, Kind: it.Kind,
					Summary: inspectSummary(label), Outcome: WordApplied,
				}
				if pre == cult.FameNone {
					rc.Detail = WordNothingToSense
				}
				out = append(out, rc)
				continue
			}
			out = append(out, IntentReceipt{
				Tick: s.World.Tick, Kind: it.Kind,
				Summary: inspectSummary(label), Outcome: WordApplied,
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

// resolvePointLabel 把意图里的穴位称谓解析为下标：显名之穴用真名，
// 未显名之穴以方位语指称（渐进感知修订包 §4）——两套称谓都可落地，
// 方位语是地形描述，不泄身份。解析不出返回 false（称谓无处落地）。
func resolvePointLabel(g *cult.BodyGraph, label string) (int, bool) {
	if strings.TrimSpace(label) == "" {
		return -1, false
	}
	for i := range g.Acupoints {
		if g.Acupoints[i].Name == label {
			return i, true
		}
	}
	for i := range g.Acupoints {
		if cult.RegionPhrase(g.Acupoints[i].Name) == label {
			return i, true
		}
	}
	return -1, false
}

// recordEvidence 把动作意图的回执记入证据账本（SPIKE 结论二：账本是
// "动作 → 观察到的后果方向"的事实凭据；感知/内视/假说不入账），并
// 同步无效重复检测（M2 WP-M2-3）。
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
		Why:     r.Detail,
	})
	s.noteStall(key, r)
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

// opsHasControl 报告一份 Ops 是否含主动请求（吸收强度或经脉请求）——
// 控制活跃口径的「存在主动请求」半边（主会话裁决 2026-09-26；另一半是
// 台账 ManualWork > 0）。被引擎拒绝的请求也算控制（发了请求即控制）。
// 自主驱动与被动环流不经过 Ops，天然不计入控制。
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
