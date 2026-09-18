// Package match glues a scenario pack, the engine and agent policies
// into one playable session: turn loop, Influence economy, guidance,
// prediction, ask, end-of-match review, and win/lose adjudication
// (strage_1 MVP + player_agent_relationship).
package match

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"agentbattle/backend/agent"
	"agentbattle/backend/engine"
	"agentbattle/backend/profile"
	"agentbattle/backend/scenario"
)

// Status values of a match.
const (
	StatusRunning = "running"
	StatusWon     = "won"
	StatusLost    = "lost"
	StatusTimeout = "timeout"
)

// Influence economy (strage_1 §四 数值表 v0.1).
const (
	InfluenceStart = 5.0
	InfluenceCap   = 5.0
	InfluenceRegen = 1.0 // 每 10 回合
	RegenEvery     = 10
	CostWhisper    = 0.5 // 轻度建议，可忽略
	CostGuide      = 1.0 // 明确方向，重新评估
	CostDeepLink   = 2.0 // 重新评估整个战略
	CostInform     = 1.0 // 提供玩家推断
	CostPause      = 3.0 // 紧急暂停，每局一次
	PredictReward  = 0.5 // 猜对奖励
)

// Guidance kinds → costs. 玩家永远发偏好，不发动作。
var GuidanceCosts = map[string]float64{
	"whisper":     CostWhisper,
	"guide":       CostGuide,
	"deeplink":    CostDeepLink,
	"information": CostInform,
}

// Guidance is one player intervention (player_agent_relationship §5).
type Guidance struct {
	Turn int64  `json:"turn"`
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// Prediction is the player's guess about the agent's next action
// (strage_1 五动作：Predict，猜对 +0.5).
type Prediction struct {
	Turn     int64  `json:"turn"`
	Choice   string `json:"choice"`
	Resolved bool   `json:"resolved"`
	Actual   string `json:"actual,omitempty"`
	Hit      bool   `json:"hit"`
}

// Prediction choices and their matchers.
var PredictionChoices = []string{"move", "sense", "attack", "pickup", "assemble", "wait"}

// QA is one player question and the agent's answer.
type QA struct {
	Turn     int64  `json:"turn"`
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

// Review is the end-of-match summary + distilled lessons
// (player_agent_relationship §18).
type Review struct {
	Summary string   `json:"summary"`
	Lessons []string `json:"lessons"`
}

// v04PackName selects the v0.4 infinite-world pack: only there does the
// chunk lazy-activation protocol run (scenario/infinite.go).
const v04PackName = "scenario/infinite-0.4"

// v05PackName selects the v0.5 pack: V04 的世界 + 敌对 Agent（rival），
// 懒激活协议同样运行（V05 基于同一套 chunk 纯函数）。
const v05PackName = "scenario/infinite-0.5"

// isInfinitePack reports whether the pack runs the chunk lazy-activation
// protocol（v0.4 起的无限世界包共用一套 PendingSpawns 纯函数）.
func isInfinitePack(name string) bool {
	return name == v04PackName || name == v05PackName
}

// Match is one session of one scenario pack.
type Match struct {
	ID        string
	Seed      uint64
	PackName  string
	W         *engine.World
	TurnLimit int64
	Spawn     engine.Hex

	PlayerID   engine.EntityID
	UnknownID  engine.EntityID
	FragmentID engine.EntityID
	// MonsterIDs lists the hand-placed monsters (v0.2); empty for mvp.
	// （v0.9：懒激活怪战斗化——激活怪也并入本列表按 minds 三模板行事，
	// 见 lazySpawns；rival 在列表内但不走 monsterAct，由自己的
	// DecisionMaker 驱动，见 rival.go。）
	MonsterIDs []engine.EntityID

	Pilot     agent.DecisionMaker
	PilotName string // "llm" | "scripted"

	// —— v0.9 敌对 Agent（rival, world_genesis §5.2 敌人走 Agent
	// Protocol）—— RivalID 是 rival 实体 id（0 = 本包无 rival）；
	// RivalPilot 与玩家 Pilot 完全同接口（rivalPilot = DecisionMaker），
	// ScriptedRival 先行、LLM rival 经同一接入点挂入；rivalCls 是 rival
	// 专属 EventClassifier 实例（与 m.cls 状态互不污染）；rivalTriggerLog
	// 记录 rival 侧每次触发原因（观测/测试口径）。
	RivalID         engine.EntityID
	RivalPilot      rivalPilot
	rivalCls        *EventClassifier
	rivalTriggerLog []string
	// rivalLLMCalls 是 rival 侧 pilot 决策调用计数（v1.0 LLM rival 下即
	// LLM 调用数）——与玩家 llmCalls 分开的独立预算口径。scripted rival
	// 不经桥接器，恒为 0；重放局不装配 rival pilot，亦不增长.
	rivalLLMCalls int
	// rivalNote is the rival's pending battle_note：桥接器在引擎推进前
	// 置入（combat 触发），Tick 在帧记录后经 flushRivalCommentary 落进
	// 解说流——与玩家 pilotTick 的时序对称（先推进、后解说）.
	rivalNote string

	Status string

	// CreatedAt is the match's creation time（持久化元信息，persistence.go）.
	CreatedAt time.Time
	// persist is the persistence hook（WP-B）：Registry 注入，每次对局
	// 推进或经济侧变更后同步落盘；nil = 不落盘（回放局/内存注册表）.
	persist func(*Match)

	Explored map[engine.Hex]bool
	Events   []engine.Event
	// Commentary is the 战斗解说 mirror of the display stream: the
	// combat battle_notes the LLM pilot produced, appended to Events as
	// Type "commentary" AFTER the tick's frame was recorded. Agent 的
	// 话，不是世界事实——不进引擎事件流（lastEvs）、不进录像帧的
	// ResultingEvents、不影响任何引擎判定.
	Commentary  []engine.Event
	Guidance    []Guidance
	Predictions []*Prediction
	pending     *Prediction
	QA          []QA
	Cognition   *agent.Cognition
	ReviewRes   *Review

	Influence  float64
	pausedOnce bool
	pauseNext  bool

	// activatedChunks 是 v0.4 怪物懒激活的"已激活 chunk 集合"——调用方
	// 持有的确定性状态（design §2.5：纯函数 + 调用方持状态），进重放
	// 语义：同 seed + 同 Intent 流 → 同一激活序列。scenario 侧的
	// PendingSpawns 是纯函数，不持任何跨对局状态.
	activatedChunks map[[2]int]bool

	// —— v0.8 WPC 行为模板 (world_genesis §5.1 Tier 1) ——
	// minds 持每只怪物的行为状态（模板/锚点/哨卫警戒态），键为实体 id。
	// 全部字段随 tick 演化、无随机特权：与 activatedChunks 同属确定性
	// 重放状态——同 seed 同 Intent 流 → 同 mind 演化 (design §2.5)。
	minds map[engine.EntityID]*monsterMind

	// —— v0.4 LLM 介入层 (design §3) ——
	// cls 的全部去抖状态是确定性字段；TriggerLog 记录每次触发原因，
	// lastTrigger 是最近一次触发（View.last_trigger），llmCalls 是本局
	// pilot 决策调用计数（LLM pilot 下即 LLM 调用数，预算口径 ≤50/局）。
	cls         *EventClassifier
	TriggerLog  []string
	lastTrigger string
	llmCalls    int

	profile    *profile.Store
	unknownDir engine.Hex
	lastEvs    []engine.Event
	// lastGatherEvs 是上一 tick 采集结算生成的分类器镜像事件（gather →
	// pickup，见 resolveGathers）；只进 classifyTick 的喂入，不进事件流.
	lastGatherEvs []engine.Event
	lastIntents   []engine.Intent

	// —— v0.7 录像 (world_genesis §7) ——
	// rec 持每 tick 一帧的录像；nil = 不录像（回放局）。
	rec *Recording
	// autoTick 标记本 tick 是否 pilot 自动决策：TickAuto 置位，Tick
	// 记帧后清零（手动 tick 的帧 Auto=false）。
	autoTick bool
}

// New creates a match of scenario/mvp-0.1. The pilot is the LLM agent
// when AGENTBATTLE_LLM_* is configured; the scripted agent otherwise.
func New(id string, seed uint64, prof *profile.Store) *Match {
	return newMatch(id, seed, prof, scenario.MVP(seed))
}

// NewV02 creates a match of scenario/v02-0.2: the hex-translated v0.2
// field with three hand-placed monsters in addition to the Unknown Agent.
func NewV02(id string, seed uint64, prof *profile.Store) *Match {
	return newMatch(id, seed, prof, scenario.V02(seed))
}

// NewV04 creates a match of scenario/infinite-0.4: the v0.4 minimal
// infinite-world pack (design §2.4).
func NewV04(id string, seed uint64, prof *profile.Store) *Match {
	return newMatch(id, seed, prof, scenario.V04(seed))
}

// NewV05 creates a match of scenario/infinite-0.5: V04 的世界 + 敌对
// Agent（rival，world_genesis §5.2）。rival 的智能与玩家同构接通（v1.0）：
// LLMFromEnv() 非空时 rival 挂 RivalLLM（对手人格 prompt，见
// agent/rival_llm.go），否则 ScriptedRival——调度、感知、录像、重放各层
// 对两种实现无差别（rival.go 接入点）。
func NewV05(id string, seed uint64, prof *profile.Store) *Match {
	return newMatch(id, seed, prof, scenario.V05(seed))
}

func newMatch(id string, seed uint64, prof *profile.Store, pack *scenario.Pack) *Match {
	w, player, unknown, frag, monsters := pack.Genesis()
	m := &Match{
		ID: id, Seed: seed, PackName: pack.Name,
		W: w, TurnLimit: pack.TurnLimit, Spawn: pack.PlayerSpawn,
		PlayerID: player, UnknownID: unknown, FragmentID: frag,
		MonsterIDs: monsters,
		Explored:   map[engine.Hex]bool{},
		Status:     StatusRunning,
		Influence:  InfluenceStart,
		profile:    prof,
		cls:        NewEventClassifier(),
		unknownDir: engine.HexDirs[0], // E：六方向固定顺序游荡

		activatedChunks: map[[2]int]bool{},                           // v0.4 懒激活：调用方持状态
		rec:             &Recording{Seed: seed, Scenario: pack.Name}, // v0.7 录像：每 tick 一帧
		rivalCls:        NewEventClassifier(),                        // v0.9 rival 独立分类器（无 rival 时闲置）
		CreatedAt:       time.Now(),                                  // WP-B 持久化元信息
	}
	// v0.8 WPC：手摆怪物按 pack 声明登记行为模板（空/未知值回落
	// wanderer 基线，旧包 V02/MVP 行为兼容），锚点即出生位。
	m.minds = make(map[engine.EntityID]*monsterMind, len(monsters))
	for i, id := range monsters {
		t := scenario.TemperWanderer
		if i < len(pack.MonsterSpawns) {
			t = normalizeTemper(pack.MonsterSpawns[i].Temper)
		}
		m.minds[id] = &monsterMind{temper: t, anchor: pack.MonsterSpawns[i].Pos}
	}
	// v0.9 rival：TemperRival 标记的手摆实体升级为敌对 Agent——身体升级
	// （scenario.PromoteRival：Sensor 加身、储气扩容）、挂 DecisionMaker
	// （与玩家 pilot 同接口）与独立 EventClassifier（rivalCls 已建）。
	// rival 的 id 保持 in MonsterIDs：玩家侧的遭遇/战斗/战术预览把它
	// 当普通敌对实体（铁律三：敌人是诚实的）。
	if idx := scenario.RivalSpecIndex(pack); idx >= 0 && idx < len(monsters) {
		m.RivalID = scenario.PromoteRival(m.W, monsters[idx])
		m.RivalPilot = NewScriptedRival(pack.MonsterSpawns[idx].Pos, pack.PlayerSpawn, pack.Passable)
	}
	if cfg := agent.LLMFromEnv(); cfg != nil {
		p := agent.NewLLM(*cfg)
		if prof != nil {
			p.SetProfile(prof.P.PersonaText(), prof.P.EnabledLessons())
		}
		m.Pilot, m.PilotName = p, "llm"
		// v1.0 LLM rival：有 rival 的包（V05）把同一 LLM 配置接给对手——
		// 敌人走同一 Agent Protocol 与同一事件驱动调度（rivalCls），桥接
		// 器负责触发语境注入、独立计数与 battle_note 对称解说.
		if m.RivalID != 0 {
			m.RivalPilot = &rivalLLMPilot{m: m, r: agent.NewRivalLLM(*cfg)}
		}
	} else {
		m.Pilot = agent.NewScripted(pack.PlayerSpawn, pack.FragmentPos, pack.Passable)
		m.PilotName = "scripted"
	}
	return m
}

// LastRivalTrigger returns the rival's most recent trigger reason
// (rivalTriggerLog 的末项；rival 从未触发或本包无 rival 为 "")——
// View.rival_trigger 的唯一读取口 (v0.9，对称于 lastTrigger).
func (m *Match) LastRivalTrigger() string {
	if len(m.rivalTriggerLog) == 0 {
		return ""
	}
	return m.rivalTriggerLog[len(m.rivalTriggerLog)-1]
}

// Tick advances one turn with explicit player intents. rival 意图由其
// 独立的事件驱动调度在本 tick 现算（scheduleRival，v0.9）；帧记录后
// 冲刷 rival 的战斗解说（v1.0，与玩家 pilotTick 的时序对称）；最后
// 落盘（WP-B 持久化钩子，persistence.go）。
func (m *Match) Tick(intents []engine.Intent) {
	m.tickAdvance(intents)
	m.persistNow()
}

// tickAdvance is Tick's advance body without the persistence tail: the
// TickAuto 路径复用它，让一个 auto tick（推进 + 双侧解说）只在 TickAuto
// 末尾统一落盘一次；重演（Replay/恢复）也走 TickWith，绝不触发落盘.
func (m *Match) tickAdvance(intents []engine.Intent) {
	m.TickWith(intents, m.scheduleRival())
	m.flushRivalCommentary()
}

// flushRivalCommentary records the rival's pending battle_note as one
// commentary event with Entity = RivalID（对手也在战斗文字流里"出声"，
// v1.0 对称解说）。调用时机在 TickWith（帧已记录）之后：不进引擎事件流
// 与录像帧——rival 的话也不是世界事实。形散后不再解说.
func (m *Match) flushRivalCommentary() {
	note := m.rivalNote
	m.rivalNote = ""
	if note != "" {
		m.recordCommentary(note, m.RivalID)
	}
}

// rivalLLMPilot bridges the LLM rival into the frozen rival scheduler
// (rival.go scheduleRival)：它以 rivalPilot（= agent.DecisionMaker）+
// orderPilot 的身份挂在 Match.RivalPilot 上，被唤醒时——
//   - 从 rivalTriggerLog 末项取本轮触发原因（scheduleRival 恰在唤醒前
//     追加），以参数注入 RivalLLM（对手版触发语境）；
//   - 独立计数 rivalLLMCalls（与玩家 llmCalls 分开的预算口径）；
//   - combat 触发时暂存 battle_note，Tick 冲刷为 Entity = RivalID 的
//     commentary（对称解说）。
//
// 调度、感知边界、录像、重放各层对桥接器与 ScriptedRival 无差别.
type rivalLLMPilot struct {
	m *Match
	r *agent.RivalLLM
}

func (p *rivalLLMPilot) Decide(obs *engine.Observation) []engine.Intent {
	return p.decide(obs)
}

func (p *rivalLLMPilot) DecideOrder(obs *engine.Observation) []engine.Intent {
	return p.decide(obs)
}

func (p *rivalLLMPilot) decide(obs *engine.Observation) []engine.Intent {
	m := p.m
	reason := m.LastRivalTrigger() // 唤醒即有值：scheduleRival 先记 log 再调 pilot
	m.rivalLLMCalls++
	intents := p.r.DecideTriggered(obs, reason)
	if reason == TriggerCombat {
		m.rivalNote = p.r.LastBattleNote() // TickWith 后由 flushRivalCommentary 落流
	}
	return intents
}

// TickWith advances one turn with explicit intents for both the player
// and the rival。回放走这里：rival 用录像帧记录的 rival_intents（与玩家
// 同规则——不重跑任何 pilot，LLM rival 亦然，world_genesis §7）；对局内
// 则由 Tick 先经 scheduleRival 现算。
func (m *Match) TickWith(intents, rivalIntents []engine.Intent) {
	defer func() { m.autoTick = false }() // 帧记完后复位（含提前返回路径）
	if m.Status != StatusRunning {
		return
	}
	acts := m.monstersAct()
	if m.RivalID != 0 && len(rivalIntents) > 0 { // v0.9 rival：意图进同一条物理链
		acts[m.RivalID] = rivalIntents
	}
	// v0.5 WPA 经济循环：指向晶簇的 transfer 不进引擎（引擎 resolveTransfer
	// 只认 fragment），在 match 层于推进后结算采集
	order, gathers := m.splitGathers(intents)
	acts[m.PlayerID] = order
	evs := m.W.Advance(acts)
	evs = append(evs, m.resolveGathers(gathers)...)
	evs = append(evs, m.lazySpawns()...) // v0.4/v0.5 怪物/晶簇懒激活进事件流
	m.lastEvs = evs
	m.lastIntents = intents
	m.Events = append(m.Events, evs...)
	if m.rec != nil { // v0.7 录像：每 tick 一帧（意图原样快照 + 事件基准；v0.9 起 rival 意图同规）
		m.rec.Frames = append(m.rec.Frames, Frame{
			Tick:            m.W.Tick,
			PlayerIntents:   append([]engine.Intent(nil), intents...),
			RivalIntents:    append([]engine.Intent(nil), rivalIntents...),
			Auto:            m.autoTick,
			ResultingEvents: append([]engine.Event(nil), evs...),
		})
	}
	if obs := m.W.Observe(m.PlayerID, nil); obs != nil { // 形散后无感知
		for _, p := range obs.Visible {
			m.Explored[p] = true
		}
	}
	m.regenInfluence()
	m.judgePending()
	m.checkEnd()
}

// lazySpawns runs the v0.4 chunk lazy-activation protocol
// (scenario/infinite.go 文件头【match 接入点】): 在玩家所在 chunk 的 3×3
// 激活圈内覆盖尚未激活的 chunk，把出怪/出晶簇 chunk 的激活事件（Entity
// 回填为新实体 id 后）并入本回合事件流，并把覆盖到的 chunk 键记入
// activatedChunks（无论是否出怪/晶簇——"只激活一次"对 chunk 成立）。
// PendingSpawns 是纯函数，激活判定状态完全由 m.activatedChunks 持有，
// 懒激活是 (seed, chunk, activated 演化) 的确定结果，重放不受影响
// (design §2.5)。仅 v0.4 包接线。
func (m *Match) lazySpawns() []engine.Event {
	if !isInfinitePack(m.PackName) {
		return nil
	}
	p := m.W.Entities[m.PlayerID]
	if p == nil {
		return nil
	}
	if m.activatedChunks == nil {
		m.activatedChunks = map[[2]int]bool{}
	}
	var evs []engine.Event
	for _, cs := range scenario.PendingSpawns(m.W, p.Pos, m.activatedChunks) {
		m.activatedChunks[[2]int{cs.QC, cs.RC}] = true
		for _, ps := range []*scenario.PendingSpawn{cs.Spawn, cs.Resource} {
			if ps == nil {
				continue // 覆盖到位但该 chunk 无此物
			}
			e := m.W.Spawn(ps.Entity)
			ev := ps.Event
			ev.Entity = e.ID // 实体 id 在 Spawn 后才存在
			if e.Kind == "monster" {
				// v0.8 WPC：懒激活怪物登记行为模板（激活 hash 分配、
				// 锚点即现身格）。
				m.minds[e.ID] = &monsterMind{temper: normalizeTemper(ps.Temper), anchor: e.Pos}
				// v0.9 懒怪战斗化：激活怪并入 MonsterIDs——自本版起懒激活
				// 怪按 minds 三模板行事（v0.4~v0.7 的静默置景退役）。副作用
				// 已知并已处置：recording_test 依赖的"scripted 跑满 60 回合"
				// seed 随之失效，已同步重采样（见 recording_test.go）。
				m.MonsterIDs = append(m.MonsterIDs, e.ID)
			}
			evs = append(evs, ev)
		}
	}
	return evs
}

// splitGathers partitions the player's intents into engine-bound intents
// and resource-gather requests：transfer 指向 Kind "resource" 的实体时，
// 引擎 resolveTransfer 会以"只能搬取元气结晶"拒绝（engine 冻结）——晶
// 簇采集在 match 层结算，剥离后不进引擎。指向 fragment 的 transfer 与
// 其余意图原样放行。
func (m *Match) splitGathers(intents []engine.Intent) (rest, gathers []engine.Intent) {
	for _, in := range intents {
		if in.Type == "transfer" {
			if t := m.W.Entities[in.Target]; t != nil && t.Kind == "resource" {
				gathers = append(gathers, in)
				continue
			}
		}
		rest = append(rest, in)
	}
	return rest, gathers
}

// resolveGathers settles the stripped gather requests（在 Advance 之后结
// 算：晶簇可能被本 tick 的溢出波及形散，先推进后采集）. 成功：实体从
// 世界移除、物料入库（w.Materials["player"]）、事件 "gather"。
//
// 同时为事件分类器生成 pickup 镜像事件（m.lastGatherEvs）：events.go 冻
// 结，resource 触发通道只认 Type "pickup" 的 pickupSeen 去抖——镜像让
// 首次采集晶簇与首次拾取结晶协同共用同一个"首次拾取"。镜像只进
// classifyTick 的喂入，不进事件流与观察。
func (m *Match) resolveGathers(gathers []engine.Intent) []engine.Event {
	m.lastGatherEvs = nil
	var evs []engine.Event
	for _, in := range gathers {
		p := m.W.Entities[m.PlayerID]
		if p == nil {
			return evs // 形散：无从采集
		}
		res := m.W.Entities[in.Target]
		switch {
		case res == nil || res.Kind != "resource":
			evs = append(evs, engine.Event{Tick: m.W.Tick, Type: "intent_rejected",
				Entity: p.ID, Text: "晶簇已消散，无从采集"})
		case res.Pos != p.Pos:
			evs = append(evs, engine.Event{Tick: m.W.Tick, Type: "intent_rejected",
				Entity: p.ID, Text: "必须站在晶簇所在格"})
		default:
			ess := res.Essence
			delete(m.W.Entities, res.ID)
			inv := m.W.Materials["player"]
			if inv == nil {
				inv = map[engine.Essence]int{}
				m.W.Materials["player"] = inv
			}
			inv[ess]++
			evs = append(evs, engine.Event{Tick: m.W.Tick, Type: "gather", Entity: p.ID,
				Text: fmt.Sprintf("采集 %s×1，仓库存料 %d", ess, inv[ess])})
			m.lastGatherEvs = append(m.lastGatherEvs,
				engine.Event{Tick: m.W.Tick, Type: "pickup", Entity: p.ID})
		}
	}
	return evs
}

// orderPilot is a pilot that speaks in standing move_to orders (执行层,
// design §3.1/§3.4 条 1)：引擎逐 tick 自动步进，两次事件之间零次决策；
// pilot 只在 EventClassifier 唤醒时被咨询。
type orderPilot interface {
	agent.DecisionMaker
	DecideOrder(obs *engine.Observation) []engine.Intent
}

// TickAuto advances one turn under the v0.4 four-layer event-driven
// model (design §3)：EventClassifier 判定本 tick 是否值得一次决策。
//
// 不触发 → 不调用 pilot，仅推进引擎（执行层订单照走、怪物照动、环境
// 照常），0 次调用；触发 → 唤醒 pilot，触发原因经 SetTrigger 进入
// LLM 的触发语境与 user payload（scripted pilot 以 move_to 订单应答）。
func (m *Match) TickAuto() {
	if m.Status != StatusRunning {
		return
	}
	defer m.persistNow() // WP-B：整个 auto tick（含解说/复盘）结束后落盘一次
	m.autoTick = true    // 本 tick 由 pilot 自动决策（录像 Auto 标记）
	if m.pauseNext {     // 紧急暂停：Agent 本回合待命（strage_1 影响表）
		m.pauseNext = false
		m.tickAdvance(nil)
		return
	}
	trigger := m.classifyTick()
	if trigger == "" { // 执行层：0 次调用
		m.tickAdvance(nil)
		return
	}
	m.lastTrigger = trigger
	m.TriggerLog = append(m.TriggerLog, trigger)
	m.llmCalls++ // 计数口径：本局 pilot 决策调用数（LLM pilot 下即 LLM 调用数）

	if la, ok := m.Pilot.(*agent.LLMAgent); ok {
		m.pilotTick(la, trigger)
		return
	}
	obs := m.W.Observe(m.PlayerID, m.lastEvs)
	if op, ok := m.Pilot.(orderPilot); ok {
		m.tickAdvance(op.DecideOrder(obs))
		return
	}
	m.tickAdvance(m.Pilot.Decide(obs))
}

// pilotTick is TickAuto's LLM branch: wake the pilot for this trigger,
// advance one tick on its intents, then collect its cognition and — for
// combat triggers — its battle_note as one commentary event.
func (m *Match) pilotTick(la *agent.LLMAgent, trigger string) {
	la.SetGuidance(m.guidanceNotes())
	la.SetTrigger(trigger)
	obs := m.W.Observe(m.PlayerID, m.lastEvs)
	m.tickAdvance(la.DecideTriggered(obs))
	if c := la.LastCognition(); c != nil {
		m.Cognition = c
	}
	if trigger == TriggerCombat { // 战斗解说：Agent 的话流入战斗文字流
		m.recordCommentary(la.LastBattleNote(), m.PlayerID)
	}
}

// battleNoteMax caps one commentary text (协议要求 ≤50 字，超长兜底截断).
const battleNoteMax = 50

// recordCommentary appends one battle_note to the display stream as a
// Type "commentary" event, attributed to entity by（v1.0：玩家与 rival
// 共用一条入口——对称解说）. 调用时机在 Tick（帧已记录）之后：引擎事件流
// lastEvs 与录像帧的 ResultingEvents 均不受影响——commentary 是 Agent 的
// 话，不是世界事实，重放（不重跑 LLM）也无须再生它. 形散后不再解说.
func (m *Match) recordCommentary(note string, by engine.EntityID) {
	note = strings.TrimSpace(note)
	if note == "" || m.W.Entities[by] == nil {
		return
	}
	if r := []rune(note); len(r) > battleNoteMax {
		note = string(r[:battleNoteMax])
	}
	ev := engine.Event{Tick: m.W.Tick, Type: "commentary", Entity: by, Text: note}
	m.Events = append(m.Events, ev)
	m.Commentary = append(m.Commentary, ev)
}

// classifyTick feeds the classifier with the facts of the tick just
// ended（真实事件流 + 采集的 pickup 镜像，见 resolveGathers）.
func (m *Match) classifyTick() string {
	return m.cls.Classify(ClassifyInput{
		Tick:            m.W.Tick,
		PlayerID:        m.PlayerID,
		Events:          m.classifierEvents(),
		Self:            m.W.Entities[m.PlayerID],
		VisibleHostiles: m.visibleHostiles(),
		InCombat:        m.InCombat(),
	})
}

// classifierEvents is the classifier's feed for the last tick: the real
// event stream plus the gather→pickup mirror events.
func (m *Match) classifierEvents() []engine.Event {
	if len(m.lastGatherEvs) == 0 {
		return m.lastEvs
	}
	return append(append([]engine.Event{}, m.lastEvs...), m.lastGatherEvs...)
}

// visibleHostiles lists the hostile ids inside the player's current
// perception radius (被动 1，感识开启 4)，按 hostiles() 固定序。
func (m *Match) visibleHostiles() []engine.EntityID {
	p := m.W.Entities[m.PlayerID]
	if p == nil {
		return nil
	}
	radius := 1
	if s := p.FindComponent(engine.CSensor); s != nil && s.Active {
		radius = 4
	}
	var out []engine.EntityID
	for _, id := range m.hostiles() {
		if e := m.W.Entities[id]; e != nil && engine.HexDist(p.Pos, e.Pos) <= radius {
			out = append(out, id)
		}
	}
	return out
}

func (m *Match) guidanceNotes() []agent.GuidanceNote {
	notes := make([]agent.GuidanceNote, 0, len(m.Guidance))
	for _, g := range m.Guidance {
		notes = append(notes, agent.GuidanceNote{Turn: g.Turn, Kind: g.Kind, Text: g.Text})
	}
	return notes
}

// Observe returns the player agent's current perception.
func (m *Match) Observe() *engine.Observation {
	return m.W.Observe(m.PlayerID, m.lastEvs)
}

// AddGuidance records and charges one player intervention.
func (m *Match) AddGuidance(kind, text string) error {
	cost, ok := GuidanceCosts[kind]
	if !ok {
		return fmt.Errorf("未知的 Guidance 类型 %q", kind)
	}
	if m.Influence < cost {
		return fmt.Errorf("Influence 不足（需 %.1f，余 %.1f）", cost, m.Influence)
	}
	m.Influence -= cost
	m.Guidance = append(m.Guidance, Guidance{Turn: m.W.Tick, Kind: kind, Text: text})
	m.cls.ArmGuidance() // Guidance 层：下一 tick 必触发一次（design §3.1）
	m.persistNow()      // WP-B：经济侧变更即时落盘（tick 之前重启不丢干预）
	return nil
}

// Pause executes the once-per-match emergency pause (3.0).
func (m *Match) Pause() error {
	if m.pausedOnce {
		return errors.New("紧急暂停每局只能使用一次")
	}
	if m.Influence < CostPause {
		return fmt.Errorf("Influence 不足（需 %.1f，余 %.1f）", CostPause, m.Influence)
	}
	m.Influence -= CostPause
	m.pausedOnce = true
	m.pauseNext = true
	m.persistNow()
	return nil
}

// Predict registers the player's guess about the agent's next action.
func (m *Match) Predict(choice string) error {
	valid := false
	for _, c := range PredictionChoices {
		if c == choice {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("未知的预测选项 %q", choice)
	}
	if m.pending != nil && !m.pending.Resolved {
		m.pending.Choice = choice // 覆盖未结算的预测
		m.persistNow()
		return nil
	}
	m.pending = &Prediction{Turn: m.W.Tick, Choice: choice}
	m.Predictions = append(m.Predictions, m.pending)
	m.persistNow()
	return nil
}

// Ask lets the player question the agent (免费；不改变决策状态).
func (m *Match) Ask(question string) (string, error) {
	la, ok := m.Pilot.(*agent.LLMAgent)
	if !ok {
		return "", errors.New("Ask 需要 LLM pilot")
	}
	ans := la.Ask(m.Observe(), question)
	if ans == "" {
		return "", errors.New("Agent 暂时无法回答")
	}
	m.QA = append(m.QA, QA{Turn: m.W.Tick, Question: question, Answer: ans})
	m.persistNow()
	return ans, nil
}

func (m *Match) regenInfluence() {
	if m.W.Tick%RegenEvery == 0 && m.Influence < InfluenceCap {
		m.Influence = minF(InfluenceCap, m.Influence+InfluenceRegen)
	}
}

// judgePending settles the pending prediction against what the agent
// actually tried to do this tick.
func (m *Match) judgePending() {
	p := m.pending
	if p == nil || p.Resolved || p.Turn >= m.W.Tick {
		return
	}
	p.Resolved = true
	p.Actual = m.classifyIntents()
	if p.Actual == p.Choice {
		p.Hit = true
		m.Influence = minF(InfluenceCap, m.Influence+PredictReward)
	}
	m.pending = nil
}

func (m *Match) classifyIntents() string {
	player := m.W.Entities[m.PlayerID]
	if player == nil {
		return "wait"
	}
	if len(m.lastIntents) == 0 {
		return "wait"
	}
	for _, in := range m.lastIntents {
		switch in.Type {
		case "move", "move_to": // WP3：订单也是"移动"的预测口径
			return "move"
		case "transfer":
			return "pickup"
		case "assemble":
			return "assemble"
		case "equip": // WPB：equip 是装配类意图（与 assemble 同级），预测口径并入同类创造行为
			return "assemble"
		case "activate":
			if c := player.Component(in.Component); c != nil {
				if c.Type == engine.CSensor {
					return "sense"
				}
				if c.Type == engine.CEmitter {
					return "attack"
				}
			}
		}
	}
	return "other"
}

func (m *Match) checkEnd() {
	p := m.W.Entities[m.PlayerID]
	if p != nil && carries(p, m.FragmentID) && p.Pos == m.Spawn {
		m.Status = StatusWon
	} else if p == nil || p.Integrity <= 0 {
		m.Status = StatusLost
	} else if m.W.Tick >= m.TurnLimit {
		m.Status = StatusTimeout
	} else {
		return
	}
	m.buildReview()
}

// buildReview runs once at match end: LLM 复盘 + 经验教训入 Profile
// （养成的不只是角色，还有驱动角色的 Agent）。
func (m *Match) buildReview() {
	rev := &Review{Summary: m.basicSummary(), Lessons: nil}
	if la, ok := m.Pilot.(*agent.LLMAgent); ok {
		payload := map[string]any{
			"status": m.Status, "turn": m.W.Tick,
			"events":      tail(m.Events, 60),
			"guidance":    m.Guidance,
			"predictions": m.Predictions,
		}
		s, lessons := la.Review(payload)
		if s != "" {
			rev.Summary = s
		}
		rev.Lessons = lessons
		if m.profile != nil && len(lessons) > 0 {
			m.profile.AddLessons(lessons)
		}
	}
	m.ReviewRes = rev
}

func (m *Match) basicSummary() string {
	p := m.W.Entities[m.PlayerID]
	integ := 0
	if p != nil {
		integ = p.Integrity
	}
	return fmt.Sprintf("%s：第 %d 回合结束，Agent 余形 %d，共发生事件 %d 条。",
		m.Status, m.W.Tick, integ, len(m.Events))
}

func tail[T any](s []T, n int) []T {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func minF(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func carries(e *engine.Entity, id engine.EntityID) bool {
	for _, c := range e.Carrying {
		if c == id {
			return true
		}
	}
	return false
}

// unknownAct is the Unknown Agent's v0.1 behaviour — a deterministic
// stand-in for its Logic DSL (world_genesis §5.2): wander the infinite
// plain along a rotating hex direction; flee if the player gets close.
// Same seed → same walk.
func (m *Match) unknownAct() []engine.Intent {
	u := m.W.Entities[m.UnknownID]
	if u == nil {
		return nil
	}
	try := func(p engine.Hex) bool {
		return m.W.At(p).Passable && m.W.EntityAt(p) == nil
	}
	if p := m.W.Entities[m.PlayerID]; p != nil && engine.HexDist(p.Pos, u.Pos) <= 2 {
		// 逃离：按"远离玩家的距离"降序尝试六邻格（固定次序打破平局）。
		cands := engine.HexStepToward(u.Pos, p.Pos)
		for i := 5; i >= 0; i-- {
			if cand := cands[i]; cand != u.Pos && try(cand) {
				return []engine.Intent{{Type: "move", To: &cand, Power: 600}}
			}
		}
	}
	for i := 0; i < 6; i++ {
		cand := u.Pos.Add(m.unknownDir)
		if try(cand) {
			return []engine.Intent{{Type: "move", To: &cand, Power: 400}}
		}
		m.unknownDir = rotate(m.unknownDir)
	}
	return nil
}

// hostiles lists every hostile entity id (the Unknown Agent + all
// monsters), in deterministic order.
func (m *Match) hostiles() []engine.EntityID {
	ids := make([]engine.EntityID, 0, 1+len(m.MonsterIDs))
	if m.UnknownID != 0 {
		ids = append(ids, m.UnknownID)
	}
	return append(ids, m.MonsterIDs...)
}

// monstersAct gathers each hostile entity's intents for one tick: the
// Unknown Agent keeps its v0.1 behaviour, monsters follow the M3.1
// Logic-DSL stand-in below. Same seed → same intents.
func (m *Match) monstersAct() map[engine.EntityID][]engine.Intent {
	acts := map[engine.EntityID][]engine.Intent{}
	if a := m.unknownAct(); len(a) > 0 {
		acts[m.UnknownID] = a
	}
	for _, id := range m.MonsterIDs {
		if id == m.RivalID {
			continue // v0.9 rival 走自己的 DecisionMaker（scheduleRival），不归三模板
		}
		if a := m.monsterAct(id); len(a) > 0 {
			acts[id] = a
		}
	}
	return acts
}

// —— v0.8 WPC 行为模板 (world_genesis §5.1 Tier 1：行为模板 + 参数变体)
// ——
//
// 三模板共用同一条物理链（§5.2 铁律：Intent → Engine，无特权 AI、无特
// 例物理）；所有随机性走 world Rng 同链，同 seed 同激活序列 → 同行为.
const (
	wandererAlertRadius  = 3 // 游荡者：玩家 ≤3 追击（基线）
	sentinelAlertRadius  = 3 // 哨卫：警戒圈（入圈即警戒）
	sentinelPatrolRadius = 2 // 哨卫：锚点 ±2 内驻守/巡逻（几乎不游荡）
	sentinelLostGrace    = 3 // 哨卫：玩家离开追击圈的宽限 tick，过后回锚
	predatorAlertRadius  = 5 // 掠食者：更远的警戒圈
)

// temperAttackPower is each template's melee emitter power（相邻攻击的
// 参数变体）：游荡者 400 / 哨卫 550 / 掠食者 450。
var temperAttackPower = map[string]int{
	scenario.TemperWanderer: 400,
	scenario.TemperSentinel: 550,
	scenario.TemperPredator: 450,
}

// monsterMind is one monster's behaviour state (v0.8 WPC)：模板、哨卫
// 锚点与警戒态。全部随 tick 确定性演化，进重放语义（见 Match.minds）。
type monsterMind struct {
	temper    string     // wanderer | sentinel | predator
	anchor    engine.Hex // 哨卫锚点（出生/现身格）
	hot       bool       // 哨卫：警戒中（追击）
	lostTicks int        // 哨卫：玩家离开警戒圈以来的连续 tick 数
}

// normalizeTemper falls unknown/empty declarations back to the wanderer
// baseline (旧包 MonsterSpec 未声明模板时行为不变).
func normalizeTemper(t string) string {
	switch t {
	case scenario.TemperSentinel, scenario.TemperPredator:
		return t
	default:
		return scenario.TemperWanderer
	}
}

// monsterAct is the v0.2 monster behaviour promoted to the v0.8 WPC
// 三模板分派 — a deterministic stand-in for its Logic DSL (world_genesis
// §5.2 铁律二：敌人走 Agent Protocol). At most one intent per tick:
//
//	hex 相邻        → 攻击：activate 离火作用于玩家（功率按模板）
//	警戒圈内        → 追击：朝玩家最短路径移动一格（不可穿障碍，可入危险区）
//	警戒圈外        → 游荡/驻守：Rng 驱动（模板各异）
//
// 模板参数（world_genesis §5.1 参数变体）：
//   - 游荡者 wanderer（基线）：≤3 追击，攻击 400，游荡原地停留 1/3
//   - 哨卫 sentinel：锚点 ±2 驻守，≤3 警戒，攻击 550；玩家离圈 3 tick
//     后解除警戒返回锚点
//   - 掠食者 predator：≤5 即追击，攻击 450，游荡不停留（大范围巡逻）
func (m *Match) monsterAct(id engine.EntityID) []engine.Intent {
	mon := m.W.Entities[id]
	if mon == nil {
		return nil
	}
	p := m.W.Entities[m.PlayerID]
	if p == nil {
		return nil
	}
	mind := m.minds[id]
	temper := scenario.TemperWanderer
	if mind != nil {
		temper = mind.temper
	}
	try := func(pos engine.Hex) bool { // 不可穿障碍；危险区可入（Passable）
		return m.W.At(pos).Passable && m.W.EntityAt(pos) == nil
	}
	// 相邻（HexDist == 1，六方向等价）：攻击（功率按模板）
	if engine.Adjacent(p.Pos, mon.Pos) {
		if em := mon.FindComponent(engine.CEmitter); em != nil {
			return []engine.Intent{{Type: "activate", Component: em.ID, Target: p.ID, Power: temperAttackPower[temper]}}
		}
	}
	chase := func() []engine.Intent { // 追击：贪心最短路径（逃跑玩家亦追最短路）
		return m.stepToward(mon, p.Pos, try)
	}
	switch temper {
	case scenario.TemperSentinel:
		return m.sentinelAct(mind, mon, p.Pos, engine.HexDist(p.Pos, mon.Pos), chase, try)
	case scenario.TemperPredator:
		if engine.HexDist(p.Pos, mon.Pos) <= predatorAlertRadius {
			return chase()
		}
		return m.wanderStep(mon, false, try) // 主动巡逻大范围：不停留
	default: // 游荡者（基线）
		if engine.HexDist(p.Pos, mon.Pos) <= wandererAlertRadius {
			return chase()
		}
		return m.wanderStep(mon, true, try) // 游荡：原地停留概率 1/3
	}
}

// sentinelAct is the sentinel template：警戒圈内追击；离圈连续
// sentinelLostGrace tick 后解除警戒、返回锚点 ±2 内驻守/巡逻。
func (m *Match) sentinelAct(mind *monsterMind, mon *engine.Entity, playerPos engine.Hex, dist int, chase func() []engine.Intent, try func(engine.Hex) bool) []engine.Intent {
	if dist <= sentinelAlertRadius {
		mind.hot, mind.lostTicks = true, 0 // 入圈：警戒
	} else if mind.hot {
		mind.lostTicks++
		if mind.lostTicks > sentinelLostGrace {
			mind.hot = false // 追击圈离开 3 tick：放弃，返回锚点
		}
	}
	if mind.hot { // 宽限期内仍朝玩家最短路径追击
		return chase()
	}
	if engine.HexDist(mon.Pos, mind.anchor) > sentinelPatrolRadius {
		return m.stepToward(mon, mind.anchor, try) // 脱岗：返回锚点
	}
	if m.W.Rng.Intn(3) != 0 { // 2/3 原地驻守（几乎不游荡）
		return nil
	}
	start := m.W.Rng.Intn(6)
	for i := 0; i < 6; i++ { // 1/3 巡逻：限制仍在锚点 ±2 内
		cand := mon.Pos.Add(engine.HexDirs[(start+i)%6])
		if try(cand) && engine.HexDist(cand, mind.anchor) <= sentinelPatrolRadius {
			return []engine.Intent{{Type: "move", To: &cand, Power: 400}}
		}
	}
	return nil
}

// stepToward moves one greedy step along the shortest hex path to to
// (HexStepToward 固定次序破平局；被地形卡死则本回合驻足).
func (m *Match) stepToward(mon *engine.Entity, to engine.Hex, try func(engine.Hex) bool) []engine.Intent {
	for _, cand := range engine.HexStepToward(mon.Pos, to) {
		if cand != mon.Pos && try(cand) {
			return []engine.Intent{{Type: "move", To: &cand, Power: 400}}
		}
	}
	return nil
}

// wanderStep is the Rng-driven wander：stayRoll 时原地停留概率 1/3（游
// 荡者基线），否则必走一步（掠食者大范围巡逻）。
func (m *Match) wanderStep(mon *engine.Entity, stayRoll bool, try func(engine.Hex) bool) []engine.Intent {
	if stayRoll && m.W.Rng.Intn(3) == 0 {
		return nil
	}
	start := m.W.Rng.Intn(6)
	for i := 0; i < 6; i++ {
		cand := mon.Pos.Add(engine.HexDirs[(start+i)%6])
		if try(cand) {
			return []engine.Intent{{Type: "move", To: &cand, Power: 400}}
		}
	}
	return nil
}

// InCombat reports whether the player is in a fight this tick: any
// hostile within HexDist 2, or a player/hostile acted upon by an
// attack, backlash or spill in the most recent tick's events.
func (m *Match) InCombat() bool {
	p := m.W.Entities[m.PlayerID]
	if p != nil {
		for _, id := range m.hostiles() {
			if e := m.W.Entities[id]; e != nil && engine.HexDist(p.Pos, e.Pos) <= 2 {
				return true
			}
		}
	}
	hostile := map[engine.EntityID]bool{m.PlayerID: true, m.UnknownID: true}
	for _, id := range m.MonsterIDs {
		hostile[id] = true
	}
	for _, ev := range m.lastEvs {
		switch ev.Type {
		case "attack", "backlash", "spill":
			if hostile[ev.Entity] {
				return true
			}
		}
	}
	return false
}

// —— v0.6 克关系战术预览 (docs/world_laws.md §5 克曲线) ——

// tacticPowerGuess is the assumed output power of the preview shot:
// the engine's default projection strength, used both as the curve
// input and as the my_power_guess field handed to the UI.
const tacticPowerGuess = 600

// TacticsView is one 克关系战术预览 card: the engine-side consequence
// of "my fire onto its essence". This is the public corollary of the
// 克曲线 (the laws say the agent must judge both essences before
// striking) computed for the player — not a leak: the card exists only
// for hostiles already inside the perception radius (信息边界不变).
type TacticsView struct {
	EnemyID       engine.EntityID `json:"enemy_id"`
	MyEssence     string          `json:"my_essence"`     // 我方离火本源（中文名；无离火为空）
	EnemyEssence  string          `json:"enemy_essence"`  // 敌方本源（中文名）
	PredictedZone string          `json:"predicted_zone"` // 侮|微|衡|乘|中性；无离火为空
	MyPowerGuess  int             `json:"my_power_guess"` // 预估作用功率（默认 600）
	Note          string          `json:"note"`           // 一句战术建议
}

// tacticNotes maps each 克曲线 zone to its one-line tactical advice.
var tacticNotes = map[engine.CurveZone]string{
	engine.ZoneWu:    "以弱击强，会反噬",
	engine.ZoneWei:   "杯水车薪",
	engine.ZoneHeng:  "正常克制",
	engine.ZoneCheng: "失控放大，会波及周围",
}

// Tactics computes the 克关系战术预览 for every hostile inside the
// player's perception radius (与 Enemies 同一边界，visibleHostiles 定序):
// 以我方离火本源对敌本源做一次默认功率 (600) 的 ApplyPower 演算。
// 非克关系不落克曲线（律五：只有生克二门），报"中性"；无离火则无法
// 反击，zone 留空。
func (m *Match) Tactics() []TacticsView {
	cards := []TacticsView{}
	player := m.W.Entities[m.PlayerID]
	if player == nil {
		return cards
	}
	em := player.FindComponent(engine.CEmitter)
	for _, id := range m.visibleHostiles() {
		e := m.W.Entities[id]
		if e == nil {
			continue
		}
		card := TacticsView{
			EnemyID:      id,
			EnemyEssence: e.Essence.String(),
			MyPowerGuess: tacticPowerGuess,
		}
		if em == nil {
			card.Note = "无离火，无法反击"
			cards = append(cards, card)
			continue
		}
		card.MyEssence = em.Essence.String()
		if !engine.Restrains(em.Essence, e.Essence) {
			card.PredictedZone = "中性"
			card.Note = "非克关系，作用减半"
			cards = append(cards, card)
			continue
		}
		strength := e.MaxIntegrity
		if strength <= 0 {
			strength = e.Integrity
		}
		res := engine.ApplyPower(em.Essence, e.Essence, tacticPowerGuess, strength)
		card.PredictedZone = res.Zone.String()
		card.Note = tacticNotes[res.Zone]
		cards = append(cards, card)
	}
	return cards
}

// rotate advances a hex direction to the next in the fixed HexDirs
// order (E→NE→NW→W→SW→SE→E).
func rotate(d engine.Hex) engine.Hex {
	for i, dir := range engine.HexDirs {
		if dir == d {
			return engine.HexDirs[(i+1)%6]
		}
	}
	return engine.HexDirs[0]
}

// Summary returns a compact one-line state for logs and tests.
func (m *Match) Summary() string {
	p := m.W.Entities[m.PlayerID]
	pos, integ, batt := "—", 0, 0
	if p != nil {
		pos, integ, batt = fmt.Sprintf("%v", p.Pos), p.Integrity, p.Battery
	}
	return fmt.Sprintf("turn=%d status=%s player=%s 形=%d 气=%d", m.W.Tick, m.Status, pos, integ, batt)
}
