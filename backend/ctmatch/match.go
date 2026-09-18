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

	Events    []Event
	IntentLog []IntentLogEntry

	active        *protocolState
	prevDamage    []int64 // 未知异常扫描用：上一 Tick 各穴位损伤
	lastEscalated bool    // 上一 Tick 是否升级（唤醒 Pilot 的条件）
	evCursor      int     // 事件流消费游标（drainNewEvents 用）
}

// NewSession 建会话；seed 同时写入 World.NoiseSeed（0 视为 1）。
func NewSession(w *cult.World, seed uint64, p Pilot) *CultSession {
	if seed == 0 {
		seed = 1
	}
	w.NoiseSeed = seed
	return &CultSession{
		World:     w,
		Seed:      seed,
		Pilot:     p,
		Protocols: make(map[string]*Protocol),
	}
}

// LoadProtocol 收录一份已编译协议。
func (s *CultSession) LoadProtocol(p *Protocol) { s.Protocols[p.Name] = p }

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
func Tick(s *CultSession, intents []Intent) *TickResult {
	res := &TickResult{}

	// 0. 意图入队（队列优先，其次本 Tick 参数），全部记录日志。
	pending := append(s.Queue, intents...)
	s.Queue = nil
	for _, it := range pending {
		s.IntentLog = append(s.IntentLog, IntentLogEntry{Tick: s.World.Tick, Intent: it})
	}

	// 1. 升级挂起 → 唤醒 Pilot（stage_2 §34 行为生命周期最后一段）。
	if s.lastEscalated && s.Pilot != nil {
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
			// Sense/Inspect 不打断 Runtime；它们只被记录。
			pending = filterNonAction(pending)
			var ev *Event
			ops, ev = s.active.runtimeOps(s)
			if ev != nil {
				// Knowledge Boundary：不尝试。引擎本 Tick 不推进。
				ev.Tick = s.World.Tick
				s.pushEvent(*ev)
				s.lastEscalated = true
				res.Escalated = true
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
			s.active = nil
		}
	} else {
		// 3. L2 路径：意图 → Ops。
		s.World.Ops = s.opsFromIntents(pending, res)
		s.advanceEngine(res)
		s.rememberDamage()
	}

	res.Events = s.drainNewEvents()
	return res
}

// advanceEngine 推进 L0 引擎（stage_2 §33：L0 绝不调用 LLM）。
func (s *CultSession) advanceEngine(res *TickResult) {
	res.Report = cult.Advance(s.World)
	res.Advanced = true
	if s.Observe != nil {
		res.Observation = s.Observe(s.World)
	}
}

// opsFromIntents 把意图换算为 Ops（Intent → Intensity/FlowRequest）。
// 意图引用未知目标时丢弃并记 EventInvalidIntent——Engine 决定实际结果，
// 也包括决定"这个意图无处落地"。
func (s *CultSession) opsFromIntents(intents []Intent, res *TickResult) cult.Ops {
	g := &s.World.Character.BodyGraph
	ops := cult.Ops{Intensity: make([]int64, len(g.Acupoints))}

	drop := func(it Intent, why string) {
		s.pushEvent(Event{
			Kind: EventInvalidIntent, Tick: s.World.Tick,
			Reason: why + " (" + it.Kind.String() + ")",
		})
	}

	for _, it := range intents {
		switch it.Kind {
		case IntentAbsorb:
			idx, ok := pointIndex(g)[it.Acupoint]
			if !ok {
				drop(it, "unknown acupoint "+it.Acupoint)
				continue
			}
			if it.Level == LevelUnset {
				drop(it, "absorb without level")
				continue
			}
			ops.Intensity[idx] = IntensityForLevel(it.Level)

		case IntentGuide:
			from, ok1 := pointIndex(g)[it.From]
			to, ok2 := pointIndex(g)[it.To]
			if !ok1 || !ok2 {
				drop(it, "unknown acupoint in guide")
				continue
			}
			ch := -1
			for c, c2 := range g.Channels {
				if c2.From == from && c2.To == to {
					ch = c
					break
				}
			}
			if ch < 0 {
				drop(it, "no channel "+it.From+"->"+it.To)
				continue
			}
			if it.Element < -1 || it.Element >= cult.ElementCount {
				drop(it, "bad element")
				continue
			}
			flow := it.Amount
			if it.Amount <= 0 {
				if it.Level == LevelUnset {
					drop(it, "guide without level or amount")
					continue
				}
				flow = FlowForLevel(it.Level, g, from, to, ch)
			}
			req := cult.FlowRequest{Channel: ch}
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

		case IntentSense, IntentInspect:
			// 不产生 Ops；观察由 Observe 钩子在结算后给出。
		}
	}
	return ops
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

func filterNonAction(intents []Intent) []Intent {
	out := intents[:0]
	for _, it := range intents {
		switch it.Kind {
		case IntentSense, IntentInspect:
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
