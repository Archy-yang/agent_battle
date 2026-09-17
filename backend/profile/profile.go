// Package profile holds the Creator Profile (player_agent_relationship
// §16): the player-shaped persona and accumulated lessons that make the
// LLM agent itself a nurture target across matches — 养成的不只是角色，
// 还有驱动角色的 Agent.
package profile

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// MaxLessons caps the lesson list (complexity budget for the prompt).
const MaxLessons = 20

// Sliders are the gamified persona dials (player_agent_relationship
// §17: 玩家调整的是决策方式，不是 Prompt). Each is 0..1:
//
//	Adventure   冒险 ↔ 谨慎
//	Cooperation 合作 ↔ 竞争
//	Exploration 探索 ↔ 利用
type Sliders struct {
	Adventure   float64 `json:"adventure"`
	Cooperation float64 `json:"cooperation"`
	Exploration float64 `json:"exploration"`
}

// Lesson is one experience card (player_agent_relationship §18).
// Disabled cards stay archived but no longer enter the agent's prompt.
type Lesson struct {
	Text    string `json:"text"`
	Enabled bool   `json:"enabled"`
}

// Profile is the persistent, cross-match agent identity.
type Profile struct {
	Persona string    `json:"persona"` // 自由补充人格（可选）
	Sliders Sliders   `json:"sliders"`
	Lessons []Lesson  `json:"lessons"`
	Updated time.Time `json:"updated"`
}

// Default returns the starting profile.
func Default() *Profile {
	return &Profile{
		Persona: "谨慎的探索者：重视生存，避免无意义战斗；为了关键目标愿意承担经过评估的风险；遇强敌先侦察，不逞强。",
		Sliders: Sliders{Adventure: 0.5, Cooperation: 0.7, Exploration: 0.5},
		Lessons: []Lesson{},
	}
}

// UnmarshalJSON accepts both the current lessons format
// ([{"text":..,"enabled":..}]) and the legacy plain string array
// (legacy cards load as Enabled:true).
func (p *Profile) UnmarshalJSON(data []byte) error {
	type alias Profile
	aux := struct {
		*alias
		Lessons json.RawMessage `json:"lessons"`
	}{alias: (*alias)(p)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if len(aux.Lessons) == 0 {
		return nil
	}
	var legacy []string
	if err := json.Unmarshal(aux.Lessons, &legacy); err == nil {
		for _, t := range legacy {
			p.Lessons = append(p.Lessons, Lesson{Text: t, Enabled: true})
		}
		return nil
	}
	var cards []Lesson
	if err := json.Unmarshal(aux.Lessons, &cards); err != nil {
		return err
	}
	p.Lessons = cards
	return nil
}

// clamp01 pins v into [0,1].
func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// sliderLine maps one slider onto its three-zone persona sentence.
func sliderLine(v float64, high, mid, low string) string {
	switch {
	case v > 0.6:
		return high
	case v < 0.4:
		return low
	default:
		return mid
	}
}

// PersonaText derives the Chinese persona description the agent sees
// from the sliders, plus the optional free-form Persona supplement.
func (p *Profile) PersonaText() string {
	var b strings.Builder
	b.WriteString(sliderLine(p.Sliders.Adventure,
		"倾向冒险，愿意承担经过评估的风险",
		"稳中求进",
		"谨慎为先，避免无谓风险"))
	b.WriteString("；")
	b.WriteString(sliderLine(p.Sliders.Cooperation,
		"倾向合作，优先寻求与其他 Agent 共赢",
		"合作与竞争视形势权衡",
		"倾向竞争，必要时正面交锋"))
	b.WriteString("；")
	b.WriteString(sliderLine(p.Sliders.Exploration,
		"偏好探索，主动收集新信息、勘察未知区域",
		"探索与利用并重",
		"偏好利用，专注于已掌握的资源与路径"))
	if s := strings.TrimSpace(p.Persona); s != "" {
		b.WriteString("。玩家补充：")
		b.WriteString(s)
	}
	return b.String()
}

// EnabledLessons returns the texts of the enabled experience cards.
func (p *Profile) EnabledLessons() []string {
	out := make([]string, 0, len(p.Lessons))
	for _, l := range p.Lessons {
		if l.Enabled && l.Text != "" {
			out = append(out, l.Text)
		}
	}
	return out
}

// Store is a file-backed profile with a mutex.
type Store struct {
	mu   sync.Mutex
	path string
	P    *Profile
}

// Load reads the profile file, falling back to the default. Profiles
// saved before the slider system (all sliders absent → zero) get the
// slider defaults; values are clamped into [0,1].
func Load(path string) *Store {
	p := Default()
	hasSliders := false
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, p)
		var probe struct {
			Sliders *json.RawMessage `json:"sliders"`
		}
		if json.Unmarshal(data, &probe) == nil {
			hasSliders = probe.Sliders != nil
		}
	}
	if !hasSliders { // 滑块系统之前的档案 → 默认滑块
		p.Sliders = Sliders{Adventure: 0.5, Cooperation: 0.7, Exploration: 0.5}
	}
	p.Sliders.Adventure = clamp01(p.Sliders.Adventure)
	p.Sliders.Cooperation = clamp01(p.Sliders.Cooperation)
	p.Sliders.Exploration = clamp01(p.Sliders.Exploration)
	return &Store{path: path, P: p}
}

// AddLessons appends new lessons (deduplicated, capped). New cards are
// enabled by default.
func (s *Store) AddLessons(lessons []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, l := range lessons {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		dup := false
		for _, have := range s.P.Lessons {
			if have.Text == l {
				dup = true
				break
			}
		}
		if !dup {
			s.P.Lessons = append(s.P.Lessons, Lesson{Text: l, Enabled: true})
		}
	}
	if len(s.P.Lessons) > MaxLessons {
		s.P.Lessons = s.P.Lessons[len(s.P.Lessons)-MaxLessons:]
	}
	s.P.Updated = time.Now()
	_ = s.save()
}

// SetPersona updates the free-form persona supplement.
func (s *Store) SetPersona(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.P.Persona = strings.TrimSpace(text)
	s.P.Updated = time.Now()
	_ = s.save()
}

// SetSliders updates the persona dials; each value must be in [0,1].
func (s *Store) SetSliders(adventure, cooperation, exploration float64) error {
	for name, v := range map[string]float64{
		"adventure": adventure, "cooperation": cooperation, "exploration": exploration,
	} {
		if v < 0 || v > 1 {
			return fmt.Errorf("滑块 %s 取值须在 0..1（得到 %v）", name, v)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.P.Sliders = Sliders{Adventure: adventure, Cooperation: cooperation, Exploration: exploration}
	s.P.Updated = time.Now()
	_ = s.save()
	return nil
}

// SetLessonEnabled toggles one experience card; index must be valid.
func (s *Store) SetLessonEnabled(index int, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if index < 0 || index >= len(s.P.Lessons) {
		return fmt.Errorf("经验卡序号 %d 越界（共 %d 张）", index, len(s.P.Lessons))
	}
	s.P.Lessons[index].Enabled = enabled
	s.P.Updated = time.Now()
	_ = s.save()
	return nil
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.P, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
