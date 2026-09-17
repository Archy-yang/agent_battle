// Package profile holds the Creator Profile (player_agent_relationship
// §16): the player-shaped persona and accumulated lessons that make the
// LLM agent itself a nurture target across matches — 养成的不只是角色，
// 还有驱动角色的 Agent.
package profile

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"
)

// MaxLessons caps the lesson list (complexity budget for the prompt).
const MaxLessons = 20

// Profile is the persistent, cross-match agent identity.
type Profile struct {
	Persona string    `json:"persona"`
	Lessons []string  `json:"lessons"`
	Updated time.Time `json:"updated"`
}

// Default returns the starting profile.
func Default() *Profile {
	return &Profile{
		Persona: "谨慎的探索者：重视生存，避免无意义战斗；为了关键目标愿意承担经过评估的风险；遇强敌先侦察，不逞强。",
		Lessons: []string{},
	}
}

// Store is a file-backed profile with a mutex.
type Store struct {
	mu   sync.Mutex
	path string
	P    *Profile
}

// Load reads the profile file, falling back to the default.
func Load(path string) *Store {
	p := Default()
	if data, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(data, p)
	}
	return &Store{path: path, P: p}
}

// AddLessons appends new lessons (deduplicated, capped).
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
			if have == l {
				dup = true
				break
			}
		}
		if !dup {
			s.P.Lessons = append(s.P.Lessons, l)
		}
	}
	if len(s.P.Lessons) > MaxLessons {
		s.P.Lessons = s.P.Lessons[len(s.P.Lessons)-MaxLessons:]
	}
	s.P.Updated = time.Now()
	_ = s.save()
}

// SetPersona updates the persona text.
func (s *Store) SetPersona(text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.P.Persona = strings.TrimSpace(text)
	s.P.Updated = time.Now()
	_ = s.save()
}

func (s *Store) save() error {
	data, err := json.MarshalIndent(s.P, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}
