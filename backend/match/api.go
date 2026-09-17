package match

import (
	"encoding/json"
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
	ID       string             `json:"id"`
	Seed     uint64             `json:"seed"`
	Pack     string             `json:"pack"`
	Pilot    string             `json:"pilot"`
	Status   string             `json:"status"`
	Turn     int64              `json:"turn"`
	TurnLimit int64             `json:"turn_limit"`
	Influence float64           `json:"influence"`
	Observation *engine.Observation `json:"observation"`
	Cognition *agent.Cognition  `json:"cognition"`
	Explored []engine.Vec2      `json:"explored"`
	Events   []engine.Event     `json:"events"`
	Guidance []Guidance         `json:"guidance"`
	Predictions []*Prediction    `json:"predictions"`
	Pending  *Prediction        `json:"pending"`
	QA       []QA               `json:"qa"`
	Review   *Review            `json:"review"`
	Costs    map[string]float64 `json:"costs"`
	// Materials is the player 物料库 (world_genesis §3.2): essence
	// 中文名 → units. Empty (not nil) when nothing is stored.
	Materials map[string]int `json:"materials"`
	// Order is the player entity's current move order (the To of the
	// most recent move intent), or null.
	Order *engine.Vec2 `json:"order"`
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

// Create starts a new match of mvp-0.1. seed 0 → fixed default.
func (r *Registry) Create(seed uint64) *Match {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	if seed == 0 {
		seed = 20260917
	}
	m := New(fmt.Sprintf("m%04d", r.next), seed, r.profile)
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
	explored := make([]engine.Vec2, 0, len(m.Explored))
	for p := range m.Explored {
		explored = append(explored, p)
	}
	sortVecs(explored)
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
	materials := map[string]int{}
	for e, n := range m.W.Materials["player"] {
		materials[e.String()] = n
	}
	var order *engine.Vec2
	for _, in := range m.lastIntents {
		if in.Type == "move" && in.To != nil {
			o := *in.To
			order = &o
			break
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
	}
}

func sortVecs(vs []engine.Vec2) {
	for i := 1; i < len(vs); i++ {
		for j := i; j > 0 && (vs[j].Y < vs[j-1].Y || (vs[j].Y == vs[j-1].Y && vs[j].X < vs[j-1].X)); j-- {
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
