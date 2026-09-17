package match

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"agentbattle/backend/agent"
	"agentbattle/backend/engine"
	"agentbattle/backend/profile"
)

// StateView is the full client-facing state of one match: observation,
// cognition, economy, logs and the explored fog overlay.
type StateView struct {
	ID          string              `json:"id"`
	Seed        uint64              `json:"seed"`
	Pack        string              `json:"pack"`
	Pilot       string              `json:"pilot"`
	Status      string              `json:"status"`
	Turn        int64               `json:"turn"`
	TurnLimit   int64               `json:"turn_limit"`
	Influence   float64             `json:"influence"`
	Observation *engine.Observation `json:"observation"`
	Cognition   *agent.Cognition    `json:"cognition"`
	Explored    []engine.Hex        `json:"explored"`
	Events      []engine.Event      `json:"events"`
	Guidance    []Guidance          `json:"guidance"`
	Predictions []*Prediction       `json:"predictions"`
	Pending     *Prediction         `json:"pending"`
	QA          []QA                `json:"qa"`
	Review      *Review             `json:"review"`
	Costs       map[string]float64  `json:"costs"`
	// Materials is the player 物料库 (world_genesis §3.2): essence
	// 中文名 → units. Empty (not nil) when nothing is stored.
	Materials map[string]int `json:"materials"`
	// Order is the player entity's standing move order (OrderTo), or
	// null — read straight off the entity, not reconstructed from the
	// last tick's intents.
	Order *engine.Hex `json:"order"`
	// InCombat reports whether the player is in a fight this tick
	// (M3.1 战斗状态).
	InCombat bool `json:"in_combat"`
	// Enemies lists the hostile entities inside the player's perception
	// radius (4 with the sensor on, 1 with it off).
	Enemies []EnemyView `json:"enemies"`
	// Tactics is the v0.6 克关系战术预览: one card per hostile inside
	// the same perception radius as Enemies (world_laws §5 克曲线的公开
	// 推论——落区由引擎演算，不扩大信息边界).
	Tactics []TacticsView `json:"tactics"`
	// LastTrigger is the most recent LLM trigger reason (design §3.2):
	// arrive | encounter | resource | threshold:* | heartbeat | combat |
	// guidance. Empty string = 尚无触发.
	LastTrigger string `json:"last_trigger"`
	// LLMCalls is the number of pilot decision invocations this match
	// (LLM pilot 下即 LLM 调用数). WP3 预算验收口径：一局 ≤ 50.
	LLMCalls int `json:"llm_calls"`
	// Commentary is the 战斗解说: the LLM pilot's combat battle_notes as
	// separate events. 它们同时以 Type "commentary" 混在 Events 展示流里
	// （前端渲染为助手气泡）；这里供需要单独消费的客户端。不进引擎事件
	// 判定，也不进录像帧的 ResultingEvents（重放不重跑 LLM，无此流）.
	Commentary []engine.Event `json:"commentary"`
}

// EnemyView is one hostile entity as the player perceives it.
type EnemyView struct {
	ID        engine.EntityID `json:"id"`
	Pos       engine.Hex      `json:"pos"`
	Distance  int             `json:"distance"`
	Essence   string          `json:"essence"`
	Integrity int             `json:"integrity"`
	Heat      int             `json:"heat"`
}

// Registry is an in-memory match registry (v0.1 single-process; the
// interface is the seam where persistence later attaches).
type Registry struct {
	mu      sync.Mutex
	matches map[string]*Match
	profile *profile.Store
	next    int
}

// NewRegistry creates an empty registry bound to the creator profile.
func NewRegistry(prof *profile.Store) *Registry {
	return &Registry{matches: map[string]*Match{}, profile: prof}
}

// Create starts a new match. scenarioName selects the pack — "mvp"
// (default), "v02" or "v04"; the variadic keeps old call sites
// unchanged. An unknown scenario name yields nil (callers turn that
// into a client-facing error). seed 0 → fixed default.
func (r *Registry) Create(seed uint64, scenarioName ...string) *Match {
	r.mu.Lock()
	defer r.mu.Unlock()
	name := ""
	if len(scenarioName) > 0 {
		name = scenarioName[0]
	}
	if seed == 0 {
		seed = 20260917
	}
	var m *Match
	switch name {
	case "", "mvp":
		m = New("", seed, r.profile)
	case "v02":
		m = NewV02("", seed, r.profile)
	case "v04":
		m = NewV04("", seed, r.profile)
	default: // 未知场景：不落库，交由调用方报错
		return nil
	}
	r.next++
	m.ID = fmt.Sprintf("m%04d", r.next)
	m.Explored[m.Spawn] = true
	r.matches[m.ID] = m
	return m
}

// Get returns the match with the given id, or nil.
func (r *Registry) Get(id string) *Match {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.matches[id]
}

// View builds the client-facing state of a match.
func (m *Match) View() *StateView {
	explored := make([]engine.Hex, 0, len(m.Explored))
	for p := range m.Explored {
		explored = append(explored, p)
	}
	sortHexes(explored)
	var obs *engine.Observation
	if m.Status == StatusRunning || m.W.Entities[m.PlayerID] != nil {
		obs = m.Observe()
	}
	qa := m.QA
	if qa == nil {
		qa = []QA{}
	}
	preds := m.Predictions
	if preds == nil {
		preds = []*Prediction{}
	}
	costs := map[string]float64{
		"whisper": CostWhisper, "guide": CostGuide, "deeplink": CostDeepLink,
		"information": CostInform, "pause": CostPause, "predict_reward": PredictReward,
	}
	commentary := m.Commentary
	if commentary == nil {
		commentary = []engine.Event{}
	}
	materials := map[string]int{}
	for e, n := range m.W.Materials["player"] {
		materials[e.String()] = n
	}
	var order *engine.Hex
	enemies := []EnemyView{}
	player := m.W.Entities[m.PlayerID]
	if player != nil {
		if player.OrderTo != nil { // 持久订单（M3.0）：直接读实体
			o := *player.OrderTo
			order = &o
		}
		for _, id := range m.visibleHostiles() { // 感知边界单一来源（与 Tactics 一致）
			e := m.W.Entities[id]
			if e == nil {
				continue
			}
			enemies = append(enemies, EnemyView{
				ID: id, Pos: e.Pos, Distance: engine.HexDist(player.Pos, e.Pos),
				Essence:   e.Essence.String(),
				Integrity: e.Integrity, Heat: e.Heat,
			})
		}
	}
	return &StateView{
		ID: m.ID, Seed: m.Seed, Pack: m.PackName, Pilot: m.PilotName,
		Status: m.Status, Turn: m.W.Tick, TurnLimit: m.TurnLimit,
		Influence:   m.Influence,
		Observation: obs,
		Cognition:   m.Cognition,
		Explored:    explored,
		Events:      m.Events,
		Guidance:    m.Guidance,
		Predictions: preds,
		Pending:     m.pending,
		QA:          qa,
		Review:      m.ReviewRes,
		Costs:       costs,
		Materials:   materials,
		Order:       order,
		InCombat:    m.InCombat(),
		Enemies:     enemies,
		Tactics:     m.Tactics(),
		LastTrigger: m.lastTrigger,
		LLMCalls:    m.llmCalls,
		Commentary:  commentary,
	}
}

// sortHexes orders hex coordinates deterministically (R ascending,
// then Q ascending) for stable JSON output.
func sortHexes(vs []engine.Hex) {
	for i := 1; i < len(vs); i++ {
		for j := i; j > 0 && (vs[j].R < vs[j-1].R || (vs[j].R == vs[j-1].R && vs[j].Q < vs[j-1].Q)); j-- {
			vs[j], vs[j-1] = vs[j-1], vs[j]
		}
	}
}

// WriteJSON writes v as a JSON response.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteErr writes a JSON error response.
func WriteErr(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}

// —— v0.7 对局录像与重放端点 ——

// ServeRecording handles GET /api/match/{id}/recording: the match's
// full replay record (world_genesis §7)。main.go 挂线：
//
//	mux.HandleFunc("GET /api/match/{id}/recording", reg.ServeRecording)
func (r *Registry) ServeRecording(w http.ResponseWriter, req *http.Request) {
	m := r.Get(req.PathValue("id"))
	if m == nil {
		WriteErr(w, http.StatusNotFound, "match not found")
		return
	}
	WriteJSON(w, http.StatusOK, m.Recording())
}

// ServeReplay handles POST /api/replay: body = Recording（内联）+
// {until_tick}。新建一次性 match 逐帧重演到 until_tick（不碰注册表），
// 返回其 StateView（含 observation/events）。重演事件与录像记录的
// ResultingEvents 不一致 → 500 {"error":"replay mismatch"}——决定性的
// 在线自证。main.go 挂线：
//
//	mux.HandleFunc("POST /api/replay", match.ServeReplay)
func ServeReplay(w http.ResponseWriter, req *http.Request) {
	var body ReplayRequest
	if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
		WriteErr(w, http.StatusBadRequest, "bad replay request: "+err.Error())
		return
	}
	m, err := Replay(&body.Recording, body.UntilTick)
	if err != nil {
		if errors.Is(err, ErrReplayMismatch) {
			WriteErr(w, http.StatusInternalServerError, "replay mismatch")
			return
		}
		WriteErr(w, http.StatusBadRequest, err.Error())
		return
	}
	WriteJSON(w, http.StatusOK, m.View())
}

// slidersPatch updates the persona dials; nil field = unchanged.
type slidersPatch struct {
	Adventure   *float64 `json:"adventure"`
	Cooperation *float64 `json:"cooperation"`
	Exploration *float64 `json:"exploration"`
}

// lessonTogglePatch flips one experience card on/off.
type lessonTogglePatch struct {
	Index   int  `json:"index"`
	Enabled bool `json:"enabled"`
}

// profilePatch is one POST /api/profile body. Each field is handled
// independently; an absent field is untouched.
type profilePatch struct {
	Sliders      *slidersPatch      `json:"sliders"`
	LessonToggle *lessonTogglePatch `json:"lesson_toggle"`
	Persona      *string            `json:"persona"`
}

// ApplyProfilePatch decodes and applies a profile update to the store.
// Any malformed or out-of-range field returns an error and nothing is
// applied (400 at the handler).
func ApplyProfilePatch(prof *profile.Store, body []byte) error {
	var patch profilePatch
	if err := json.Unmarshal(body, &patch); err != nil {
		return fmt.Errorf("bad profile patch: %v", err)
	}
	if patch.Sliders != nil {
		adv, coop, expl := prof.P.Sliders.Adventure, prof.P.Sliders.Cooperation, prof.P.Sliders.Exploration
		if patch.Sliders.Adventure != nil {
			adv = *patch.Sliders.Adventure
		}
		if patch.Sliders.Cooperation != nil {
			coop = *patch.Sliders.Cooperation
		}
		if patch.Sliders.Exploration != nil {
			expl = *patch.Sliders.Exploration
		}
		if err := prof.SetSliders(adv, coop, expl); err != nil {
			return err
		}
	}
	if patch.LessonToggle != nil {
		if err := prof.SetLessonEnabled(patch.LessonToggle.Index, patch.LessonToggle.Enabled); err != nil {
			return err
		}
	}
	if patch.Persona != nil {
		prof.SetPersona(*patch.Persona)
	}
	return nil
}
