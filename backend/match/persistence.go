package match

// persistence.go —— WP-B 对局持久化 (world_genesis §7 重放规则的落盘应用).
//
// 机制 = 确定性重演，不是状态快照：每个对局一个 JSON 文件
// `<matchID>.json`（目录由 Registry 持有，worldd flag -matches 覆盖，
// 默认 "matches"），内容是 seed、场景名、已消耗 tick 数、逐帧 Recording
// （Intent 流本身就是完整的重演输入）、Creator Profile 快照与对局元
// 信息。重启后 Registry 只加载元信息（惰性）：Get 首次访问该对局时，
// 用原 seed + 场景重建 match、Pilot 置空（绝不重跑 LLM），经
// replayFrames——与 POST /api/replay 同一条重演路径——逐帧重演到当前
// tick，再按当前环境装配 pilot、恢复元状态、续接录像。同 seed +
// Intent 流 = 逐字节一致（铁律）；重演不一致（ErrReplayMismatch）或
// 文件损坏 → 跳过该对局并 log，不阻塞其它对局与启动.
//
// 写盘时机：Tick/TickAuto 结束后同步写（对局是小 JSON），经济侧变更
// （Guidance/Predict/Pause/Ask）即时写；终局由 meta.status 标记.
//
// 元状态（Influence、Guidance、预测、解说、触发日志、分类器去抖状态
// 等）不属引擎决定论管辖，随文件作为数据恢复——重演只负责引擎世界；
// 分类器快照保证恢复后的事件驱动触发节奏与中断前逐 tick 一致.

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"agentbattle/backend/agent"
	"agentbattle/backend/engine"
)

// ProfileSnapshot is the Creator Profile slice a restored LLM pilot
// needs（player_agent_relationship §16）：快照 PersonaText 与启用经验卡
// 的产出，恢复时注入（SetProfile），保证 persona/lessons 与中断前一致
// ——不受档案此后变动影响.
type ProfileSnapshot struct {
	Persona string   `json:"persona"` // PersonaText() 快照（滑块 + 补充语合成）
	Lessons []string `json:"lessons"` // EnabledLessons() 快照
}

// encounterSnapshot is one hostile's encounter debounce state (JSON 形态
// 的 events.go encounterState).
type encounterSnapshot struct {
	Armed        bool  `json:"armed"`
	LastSeenTick int64 `json:"last_seen_tick"`
}

// classifierSnapshot is the EventClassifier's deterministic debounce
// state（events.go：全部去抖状态都是普通确定性字段）——随元状态落盘，
// 恢复后触发节奏与中断前逐 tick 一致.
type classifierSnapshot struct {
	LastTriggerTick int64                                 `json:"last_trigger_tick"`
	ProcessedTick   int64                                 `json:"processed_tick"`
	BlockedStreak   int                                   `json:"blocked_streak"`
	Encounters      map[engine.EntityID]encounterSnapshot `json:"encounters"`
	PickupSeen      bool                                  `json:"pickup_seen"`
	SpawnSeen       map[engine.EntityID]bool              `json:"spawn_seen"`
	BatteryArmed    bool                                  `json:"battery_armed"`
	HeatArmed       bool                                  `json:"heat_armed"`
	IntegrityArmed  bool                                  `json:"integrity_armed"`
	PendingGuidance bool                                  `json:"pending_guidance"`
}

// snapshotClassifier copies the classifier's debounce state out.
func snapshotClassifier(c *EventClassifier) *classifierSnapshot {
	if c == nil {
		return nil
	}
	s := &classifierSnapshot{
		LastTriggerTick: c.lastTriggerTick,
		ProcessedTick:   c.processedTick,
		BlockedStreak:   c.blockedStreak,
		Encounters:      map[engine.EntityID]encounterSnapshot{},
		PickupSeen:      c.pickupSeen,
		SpawnSeen:       map[engine.EntityID]bool{},
		BatteryArmed:    c.batteryArmed,
		HeatArmed:       c.heatArmed,
		IntegrityArmed:  c.integrityArmed,
		PendingGuidance: c.pendingGuidance,
	}
	for id, st := range c.encounters {
		s.Encounters[id] = encounterSnapshot{Armed: st.armed, LastSeenTick: st.lastSeenTick}
	}
	for id, seen := range c.spawnSeen {
		s.SpawnSeen[id] = seen
	}
	return s
}

// apply writes the snapshot back into a freshly built classifier（保留其
// HeartbeatEvery 等缺省字段），返回同一实例.
func (s *classifierSnapshot) apply(c *EventClassifier) *EventClassifier {
	c.lastTriggerTick = s.LastTriggerTick
	c.processedTick = s.ProcessedTick
	c.blockedStreak = s.BlockedStreak
	for id, st := range s.Encounters {
		c.encounters[id] = &encounterState{armed: st.Armed, lastSeenTick: st.LastSeenTick}
	}
	c.pickupSeen = s.PickupSeen
	for id, seen := range s.SpawnSeen {
		c.spawnSeen[id] = seen
	}
	c.batteryArmed = s.BatteryArmed
	c.heatArmed = s.HeatArmed
	c.integrityArmed = s.IntegrityArmed
	c.pendingGuidance = s.PendingGuidance
	return c
}

// persistedMeta is everything a continued match needs beyond the engine
// world（引擎世界由重演复原）：经济、干预、预测、问答、解说、事件展示
// 流、触发日志与两侧分类器去抖状态。全部是数据恢复，不参与重演判定.
type persistedMeta struct {
	Status          string              `json:"status"`
	Influence       float64             `json:"influence"`
	PausedOnce      bool                `json:"paused_once"`
	PauseNext       bool                `json:"pause_next"`
	Guidance        []Guidance          `json:"guidance,omitempty"`
	Predictions     []*Prediction       `json:"predictions,omitempty"`
	Pending         *Prediction         `json:"pending,omitempty"`
	QA              []QA                `json:"qa,omitempty"`
	Commentary      []engine.Event      `json:"commentary,omitempty"`
	Events          []engine.Event      `json:"events,omitempty"`
	Cognition       *agent.Cognition    `json:"cognition,omitempty"`
	Review          *Review             `json:"review,omitempty"`
	TriggerLog      []string            `json:"trigger_log,omitempty"`
	LastTrigger     string              `json:"last_trigger,omitempty"`
	LLMCalls        int                 `json:"llm_calls"`
	RivalTriggerLog []string            `json:"rival_trigger_log,omitempty"`
	RivalLLMCalls   int                 `json:"rival_llm_calls"`
	RivalNote       string              `json:"rival_note,omitempty"`
	Cls             *classifierSnapshot `json:"cls,omitempty"`
	RivalCls        *classifierSnapshot `json:"rival_cls,omitempty"`
}

// persistedMatch is one on-disk match file (`<matchID>.json`).
type persistedMatch struct {
	ID        string           `json:"id"`
	Seed      uint64           `json:"seed"`
	Scenario  string           `json:"scenario"`           // 场景包名（replayConstructors 口径）
	CreatedAt time.Time        `json:"created_at"`         // 对局创建时间
	Ticks     int64            `json:"ticks"`              // 已消耗 tick 数（落盘时刻）
	Finished  string           `json:"finished,omitempty"` // 终局状态；空 = 进行中
	Recording *Recording       `json:"recording"`          // 逐帧录像（重演输入）
	Profile   *ProfileSnapshot `json:"profile,omitempty"`  // Creator Profile 快照
	Meta      *persistedMeta   `json:"meta"`
}

// persistNow fires the persistence hook（nil = 不落盘）.
func (m *Match) persistNow() {
	if m.persist != nil {
		m.persist(m)
	}
}

// snapshotMatch builds the on-disk representation of a live match.
func (r *Registry) snapshotMatch(m *Match) *persistedMatch {
	pm := &persistedMatch{
		ID:        m.ID,
		Seed:      m.Seed,
		Scenario:  m.PackName,
		CreatedAt: m.CreatedAt,
		Ticks:     m.W.Tick,
		Recording: m.Recording(),
		Meta:      snapshotMeta(m),
	}
	if m.Status != StatusRunning {
		pm.Finished = m.Status
	}
	if r.profile != nil {
		pm.Profile = &ProfileSnapshot{
			Persona: r.profile.P.PersonaText(),
			Lessons: r.profile.P.EnabledLessons(),
		}
	}
	return pm
}

// snapshotMeta copies the match's meta state (经济/干预/解说/触发节奏).
func snapshotMeta(m *Match) *persistedMeta {
	return &persistedMeta{
		Status:          m.Status,
		Influence:       m.Influence,
		PausedOnce:      m.pausedOnce,
		PauseNext:       m.pauseNext,
		Guidance:        append([]Guidance(nil), m.Guidance...),
		Predictions:     append([]*Prediction(nil), m.Predictions...),
		Pending:         m.pending,
		QA:              append([]QA(nil), m.QA...),
		Commentary:      append([]engine.Event(nil), m.Commentary...),
		Events:          append([]engine.Event(nil), m.Events...),
		Cognition:       m.Cognition,
		Review:          m.ReviewRes,
		TriggerLog:      append([]string(nil), m.TriggerLog...),
		LastTrigger:     m.lastTrigger,
		LLMCalls:        m.llmCalls,
		RivalTriggerLog: append([]string(nil), m.rivalTriggerLog...),
		RivalLLMCalls:   m.rivalLLMCalls,
		RivalNote:       m.rivalNote,
		Cls:             snapshotClassifier(m.cls),
		RivalCls:        snapshotClassifier(m.rivalCls),
	}
}

// persistMatch writes one match file（原子写：tmp + rename）. 落盘失败只
// log——持久化从不阻断对局.
func (r *Registry) persistMatch(m *Match) {
	if r.dir == "" {
		return
	}
	data, err := json.MarshalIndent(r.snapshotMatch(m), "", "  ")
	if err != nil {
		log.Printf("matches: 对局 %s 序列化失败：%v", m.ID, err)
		return
	}
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	tmp := filepath.Join(r.dir, m.ID+".json.tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		log.Printf("matches: 对局 %s 落盘失败：%v", m.ID, err)
		return
	}
	if err := os.Rename(tmp, filepath.Join(r.dir, m.ID+".json")); err != nil {
		log.Printf("matches: 对局 %s 落盘失败：%v", m.ID, err)
		_ = os.Remove(tmp)
	}
}

// loadPersisted scans the match directory at startup, loading metadata
// only（惰性恢复：重演推迟到 Get 首次访问）。损坏文件跳过并 log.
func (r *Registry) loadPersisted() {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		log.Printf("matches: 目录 %s 不可读（%v），无存量对局", r.dir, err)
		return
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(r.dir, name))
		if err != nil {
			log.Printf("matches: 跳过不可读的对局文件 %s：%v", name, err)
			continue
		}
		var pm persistedMatch
		if err := json.Unmarshal(data, &pm); err != nil || pm.ID == "" || pm.Recording == nil {
			log.Printf("matches: 跳过损坏的对局文件 %s：%v", name, err)
			continue
		}
		if n, ok := matchNumber(pm.ID); ok && n > r.next {
			r.next = n // 新建对局的编号不与存量冲突
		}
		r.pending[pm.ID] = &pm
	}
	if len(r.pending) > 0 {
		log.Printf("matches: 目录 %s 有 %d 个存量对局（惰性恢复：首次访问时重演）", r.dir, len(r.pending))
	}
}

// matchNumber parses the numeric suffix of a "m%04d" match id.
func matchNumber(id string) (int, bool) {
	n, err := strconv.Atoi(strings.TrimPrefix(id, "m"))
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// restoreMatch rebuilds a match from its on-disk form：原 seed + 场景重
// 建，经 replayFrames（与 POST /api/replay 同一条重演路径）逐帧重演到
// 落盘时刻，再装配 pilot（Profile 快照注入）、恢复元状态、续接录像.
func (r *Registry) restoreMatch(pm *persistedMatch) (*Match, error) {
	rec := pm.Recording
	if rec == nil {
		return nil, fmt.Errorf("恢复：对局 %s 缺少录像", pm.ID)
	}
	name := pm.Scenario
	if name == "" {
		name = rec.Scenario
	}
	ctor, ok := replayConstructors[name]
	if !ok {
		return nil, fmt.Errorf("恢复：未知场景 %q", name)
	}
	seed := pm.Seed
	if seed == 0 {
		seed = replayDefaultSeed
	}
	m := ctor(pm.ID, seed, r.profile)
	m.ID = pm.ID
	m.Explored[m.Spawn] = true
	// 重演期间 pilot 必为 nil（不重跑 LLM，§7 硬规则一）；重演完成后再
	// 按当前环境装配回——与 Replay 现状一致。
	pilot, pilotName, rivalPilot := m.Pilot, m.PilotName, m.RivalPilot
	m.Pilot, m.PilotName, m.RivalPilot = nil, "restore", nil
	m.rec = nil // 重演自身不记帧：录像以落盘的帧为准，恢复后续接
	if err := replayFrames(m, rec, 0); err != nil {
		return nil, fmt.Errorf("恢复：对局 %s 重演失败：%w", pm.ID, err)
	}
	m.Pilot, m.PilotName, m.RivalPilot = pilot, pilotName, rivalPilot
	if pm.Profile != nil { // Creator Profile 快照注入（persona/lessons 一致）
		if la, ok := m.Pilot.(*agent.LLMAgent); ok {
			la.SetProfile(pm.Profile.Persona, pm.Profile.Lessons)
		}
	}
	if mt := pm.Meta; mt != nil {
		if mt.Status != "" {
			m.Status = mt.Status
		}
		m.Influence = mt.Influence
		m.pausedOnce, m.pauseNext = mt.PausedOnce, mt.PauseNext
		m.Guidance = mt.Guidance
		m.Predictions = mt.Predictions
		m.pending = mt.Pending
		m.QA = mt.QA
		m.Commentary = mt.Commentary
		m.Events = mt.Events
		m.Cognition = mt.Cognition
		m.ReviewRes = mt.Review
		m.TriggerLog = mt.TriggerLog
		m.lastTrigger = mt.LastTrigger
		m.llmCalls = mt.LLMCalls
		m.rivalTriggerLog = mt.RivalTriggerLog
		m.rivalLLMCalls = mt.RivalLLMCalls
		m.rivalNote = mt.RivalNote
		if mt.Cls != nil {
			m.cls = mt.Cls.apply(m.cls)
		}
		if mt.RivalCls != nil {
			m.rivalCls = mt.RivalCls.apply(m.rivalCls)
		}
	}
	m.rec = &Recording{Seed: m.Seed, Scenario: m.PackName, Frames: rec.Frames} // 续接录像
	return m, nil
}
