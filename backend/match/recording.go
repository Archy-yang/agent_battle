package match

// v0.7 对局录像与重放 (world_genesis §7 / §7.1).
//
// Replay = Engine + seed + 记录的 Intent 流 + Guidance 流（§7 硬规则一：
// 不重跑任何 LLM）。帧里记录的是"该 tick 实际进入引擎的玩家侧意图"
// ——TickAuto 时是 pilot 的产出，手动 tick 时是手动意图；未知体/怪物
// 行为是 seed 确定性的（engine/replay_test.go 重放门禁已证明），回放时
// 由确定性逻辑重演，无需记录。Guidance 不重新消化：它只影响 LLM 上下
// 文，不进引擎——回放局不装载 Guidance，经济侧（Influence）随之中立。
//
// 回放校验：逐帧比较重演产生的事件与帧里记录的 ResultingEvents，任何
// 一帧不一致即 ErrReplayMismatch——决定性的在线自证。

import (
	"errors"
	"fmt"
	"slices"

	"agentbattle/backend/engine"
	"agentbattle/backend/profile"
)

// replayDefaultSeed 与 Registry.Create 的缺省 seed 一致（seed 0 视为
// 未指定，落同一缺省）。
const replayDefaultSeed = 20260917

// Frame is one recorded tick (v0.7): 该 tick 实际进入引擎的玩家侧意图
// 与随之产生的事件。Auto 标记 pilot 自动决策（TickAuto）；回放只消费
// PlayerIntents 与 RivalIntents，Auto 仅为审计/前端播发器所需。
// v0.9：RivalIntents 是 rival 该 tick 实际进入引擎的意图——rival 与
// 玩家同规则（不重跑任何 pilot，LLM rival 亦然），回放按记录重演.
type Frame struct {
	Tick            int64           `json:"tick"`
	PlayerIntents   []engine.Intent `json:"player_intents"`   // Tick 调用的原样记录
	RivalIntents    []engine.Intent `json:"rival_intents"`    // v0.9：rival 该 tick 进入引擎的意图
	Auto            bool            `json:"auto"`             // 是否 pilot 自动决策
	ResultingEvents []engine.Event  `json:"resulting_events"` // 该 tick 产生的事件（回放校验基准）
}

// Recording is one full match's replay record (world_genesis §7)：
// Engine + seed + Intent 流 + Guidance 流 = 可精确重放。
type Recording struct {
	ID       string     `json:"id"`
	Seed     uint64     `json:"seed"`
	Scenario string     `json:"scenario"` // 场景包名（如 scenario/infinite-0.4）
	Frames   []Frame    `json:"frames"`   // 每 tick 一帧
	Guidance []Guidance `json:"guidance,omitempty"`
}

// ErrReplayMismatch is returned when the replayed events diverge from
// the recorded ones — the decisive online self-proof of determinism.
var ErrReplayMismatch = errors.New("replay mismatch")

// Recording returns the match's recording. The match keeps recording
// frames live (Match.rec), so the snapshot is safe to hand to clients
// and stays valid as the match continues.
func (m *Match) Recording() *Recording {
	rec := &Recording{
		ID:       m.ID,
		Seed:     m.Seed,
		Scenario: m.PackName,
		Frames:   []Frame{},
	}
	if m.rec != nil {
		rec.Frames = m.rec.Frames
	}
	rec.Guidance = append([]Guidance(nil), m.Guidance...)
	return rec
}

// ReplayRequest is one POST /api/replay body: the recording (inlined)
// plus the tick to replay up to. until_tick 缺省（0）= 全程重演。
type ReplayRequest struct {
	Recording
	UntilTick int64 `json:"until_tick"`
}

// replayConstructors maps both the API-facing scenario names and the
// canonical pack names onto the match constructors, so a Recording is
// replayable whichever spelling its Scenario field carries.
var replayConstructors = map[string]func(string, uint64, *profile.Store) *Match{
	"mvp":                   New,
	"v02":                   NewV02,
	"v04":                   NewV04,
	"scenario/mvp-0.1":      New,
	"scenario/v02-0.2":      NewV02,
	"scenario/infinite-0.4": NewV04,
}

// Replay rebuilds the recorded match from scratch (同 seed 同场景的新建
// World) and re-drives it frame by frame through the recorded player
// intents up to untilTick, verifying each tick's events against the
// recording. 不碰任何注册表：返回的一次性 match 由调用方自行处置.
//
// LLM 永不被重跑（§7 硬规则一）：回放局不装载 pilot，逐帧直喂记录的
// 意图；未知体/怪物行为由其确定性脚本重演（engine/replay_test.go 门禁）。
func Replay(rec *Recording, untilTick int64) (*Match, error) {
	if rec == nil {
		return nil, errors.New("replay: 录像为空")
	}
	ctor, ok := replayConstructors[rec.Scenario]
	if !ok {
		return nil, fmt.Errorf("replay: 未知场景 %q", rec.Scenario)
	}
	seed := rec.Seed
	if seed == 0 {
		seed = replayDefaultSeed
	}
	m := ctor("replay", seed, nil)
	if rec.ID != "" {
		m.ID = rec.ID + "/replay"
	}
	m.Pilot, m.PilotName = nil, "replay" // 绝不咨询 pilot，更不重跑 LLM
	m.RivalPilot = nil                   // v0.9：rival 同规则——绝不重跑，意图按帧记录重演
	m.rec = nil                          // 回放局自身不录像
	if untilTick <= 0 {                  // 缺省：重演到录像尽头
		if n := len(rec.Frames); n > 0 {
			untilTick = rec.Frames[n-1].Tick
		}
	}
	for _, f := range rec.Frames {
		if f.Tick > untilTick {
			break
		}
		if f.Tick != m.W.Tick+1 {
			return nil, fmt.Errorf("replay: 帧序断裂：帧 tick %d，期望 %d", f.Tick, m.W.Tick+1)
		}
		m.TickWith(f.PlayerIntents, f.RivalIntents) // v0.9：rival 意图按帧记录重演
		if !slices.Equal(m.lastEvs, f.ResultingEvents) {
			return nil, fmt.Errorf("%w（tick %d 事件流分歧）", ErrReplayMismatch, f.Tick)
		}
	}
	if last := int64(len(rec.Frames)); last > 0 && rec.Frames[last-1].Tick < untilTick {
		return nil, fmt.Errorf("replay: until_tick %d 超出录像范围（最后一帧 tick %d）", untilTick, rec.Frames[last-1].Tick)
	}
	return m, nil
}
