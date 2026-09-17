package profile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPersonaTextSliders covers the three-zone derivation of each of
// the three persona dials (high / mid / low).
func TestPersonaTextSliders(t *testing.T) {
	cases := []struct {
		name string
		set  func(s *Sliders)
		want string
	}{
		{"adventure high", func(s *Sliders) { s.Adventure = 0.8 }, "倾向冒险，愿意承担经过评估的风险"},
		{"adventure mid", func(s *Sliders) { s.Adventure = 0.5 }, "稳中求进"},
		{"adventure low", func(s *Sliders) { s.Adventure = 0.2 }, "谨慎为先，避免无谓风险"},
		{"adventure boundary 0.6", func(s *Sliders) { s.Adventure = 0.6 }, "稳中求进"},
		{"cooperation high", func(s *Sliders) { s.Cooperation = 0.9 }, "倾向合作，优先寻求与其他 Agent 共赢"},
		{"cooperation mid", func(s *Sliders) { s.Cooperation = 0.5 }, "合作与竞争视形势权衡"},
		{"cooperation low", func(s *Sliders) { s.Cooperation = 0.1 }, "倾向竞争，必要时正面交锋"},
		{"exploration high", func(s *Sliders) { s.Exploration = 0.7 }, "偏好探索，主动收集新信息、勘察未知区域"},
		{"exploration mid", func(s *Sliders) { s.Exploration = 0.4 }, "探索与利用并重"},
		{"exploration low", func(s *Sliders) { s.Exploration = 0.0 }, "偏好利用，专注于已掌握的资源与路径"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &Profile{Sliders: Sliders{Adventure: 0.5, Cooperation: 0.5, Exploration: 0.5}}
			tc.set(&p.Sliders)
			got := p.PersonaText()
			if !strings.Contains(got, tc.want) {
				t.Errorf("PersonaText() = %q, want substring %q", got, tc.want)
			}
		})
	}
}

// TestPersonaTextAppendsPersona covers the optional free-form
// supplement joining the derived text.
func TestPersonaTextAppendsPersona(t *testing.T) {
	p := &Profile{Sliders: Sliders{Adventure: 0.8, Cooperation: 0.2, Exploration: 0.5}, Persona: "  讨厌浪费回合  "}
	got := p.PersonaText()
	if !strings.Contains(got, "倾向冒险") || !strings.Contains(got, "倾向竞争") {
		t.Errorf("PersonaText() missing slider lines: %q", got)
	}
	if !strings.Contains(got, "玩家补充：讨厌浪费回合") {
		t.Errorf("PersonaText() missing supplement: %q", got)
	}
	empty := (&Profile{}).PersonaText()
	if strings.Contains(empty, "玩家补充") {
		t.Errorf("empty Persona must not add supplement: %q", empty)
	}
}

// TestLegacyLessonsCompat loads the old format (plain string array)
// and expects Enabled:true cards.
func TestLegacyLessonsCompat(t *testing.T) {
	legacy := `{"persona":"老档案","lessons":["别硬闯火区","先侦察再行动"],"updated":"2026-01-01T00:00:00Z"}`
	var p Profile
	if err := json.Unmarshal([]byte(legacy), &p); err != nil {
		t.Fatalf("unmarshal legacy: %v", err)
	}
	if len(p.Lessons) != 2 {
		t.Fatalf("lessons = %+v, want 2 cards", p.Lessons)
	}
	for i, want := range []string{"别硬闯火区", "先侦察再行动"} {
		if p.Lessons[i].Text != want || !p.Lessons[i].Enabled {
			t.Errorf("lessons[%d] = %+v, want {Text:%q Enabled:true}", i, p.Lessons[i], want)
		}
	}
	if got := p.EnabledLessons(); len(got) != 2 {
		t.Errorf("EnabledLessons() = %v, want 2", got)
	}

	// New format round-trips through the same entry point.
	current := `{"lessons":[{"text":"新卡","enabled":false}]}`
	var p2 Profile
	if err := json.Unmarshal([]byte(current), &p2); err != nil {
		t.Fatalf("unmarshal current: %v", err)
	}
	if len(p2.Lessons) != 1 || p2.Lessons[0].Text != "新卡" || p2.Lessons[0].Enabled {
		t.Errorf("lessons = %+v, want one disabled card", p2.Lessons)
	}
	if got := p2.EnabledLessons(); len(got) != 0 {
		t.Errorf("EnabledLessons() = %v, want empty", got)
	}
}

// TestAddLessonsDedupAndCap covers dedup, blank filtering, the
// MaxLessons cap and enabled-by-default semantics.
func TestAddLessonsDedupAndCap(t *testing.T) {
	s := Load(filepath.Join(t.TempDir(), "profile.json"))
	s.AddLessons([]string{"先侦察再行动", "  先侦察再行动  ", "", "   "})
	s.AddLessons([]string{"别硬闯火区"})
	if n := len(s.P.Lessons); n != 2 {
		t.Fatalf("lessons = %d, want 2 (dedup + blank filtered)", n)
	}
	for _, l := range s.P.Lessons {
		if !l.Enabled {
			t.Errorf("new card %q must default to enabled", l.Text)
		}
	}
	for i := 0; i < MaxLessons+5; i++ {
		s.AddLessons([]string{"卡" + strings.Repeat("x", i+1)})
	}
	if len(s.P.Lessons) != MaxLessons {
		t.Errorf("lessons = %d, want capped at %d", len(s.P.Lessons), MaxLessons)
	}
}

// TestSaveLoadRoundtrip covers the persisted file surviving a Load,
// including sliders, disabled lessons and the legacy-slider default.
func TestSaveLoadRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	s := Load(path)
	if err := s.SetSliders(0.9, 0.3, 0.6); err != nil {
		t.Fatalf("SetSliders: %v", err)
	}
	s.SetPersona("讨厌浪费回合")
	s.AddLessons([]string{"先侦察再行动"})
	if err := s.SetLessonEnabled(0, false); err != nil {
		t.Fatalf("SetLessonEnabled: %v", err)
	}

	s2 := Load(path)
	if s2.P.Sliders != (Sliders{Adventure: 0.9, Cooperation: 0.3, Exploration: 0.6}) {
		t.Errorf("sliders = %+v, want 0.9/0.3/0.6", s2.P.Sliders)
	}
	if s2.P.Persona != "讨厌浪费回合" {
		t.Errorf("persona = %q", s2.P.Persona)
	}
	if len(s2.P.Lessons) != 1 || s2.P.Lessons[0].Enabled {
		t.Errorf("lessons = %+v, want one disabled card", s2.P.Lessons)
	}
	if got := s2.P.EnabledLessons(); len(got) != 0 {
		t.Errorf("EnabledLessons() = %v, want empty", got)
	}
	if !strings.Contains(s2.P.PersonaText(), "倾向竞争") {
		t.Errorf("PersonaText() should reflect cooperation 0.3: %q", s2.P.PersonaText())
	}

	// Pre-slider profile file: sliders absent → defaults, legacy
	// string lessons → enabled.
	oldPath := filepath.Join(t.TempDir(), "old.json")
	old := `{"persona":"旧","lessons":["老经验"]}`
	if err := os.WriteFile(oldPath, []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	s3 := Load(oldPath)
	if s3.P.Sliders != (Sliders{Adventure: 0.5, Cooperation: 0.7, Exploration: 0.5}) {
		t.Errorf("legacy sliders = %+v, want defaults", s3.P.Sliders)
	}
	if len(s3.P.Lessons) != 1 || !s3.P.Lessons[0].Enabled {
		t.Errorf("legacy lessons = %+v, want one enabled card", s3.P.Lessons)
	}
}

// TestSetSlidersValidation covers the 0..1 range check.
func TestSetSlidersValidation(t *testing.T) {
	s := Load(filepath.Join(t.TempDir(), "profile.json"))
	if err := s.SetSliders(1.5, 0.5, 0.5); err == nil {
		t.Error("SetSliders(1.5,..) should fail")
	}
	if err := s.SetSliders(0.5, -0.1, 0.5); err == nil {
		t.Error("SetSliders(-0.1) should fail")
	}
	if s.P.Sliders.Adventure != 0.5 || s.P.Sliders.Cooperation != 0.7 {
		t.Errorf("rejected update must not change sliders: %+v", s.P.Sliders)
	}
}

// TestSetLessonEnabledValidation covers the index range check.
func TestSetLessonEnabledValidation(t *testing.T) {
	s := Load(filepath.Join(t.TempDir(), "profile.json"))
	if err := s.SetLessonEnabled(0, true); err == nil {
		t.Error("toggle on empty list should fail")
	}
	s.AddLessons([]string{"先侦察再行动"})
	if err := s.SetLessonEnabled(1, true); err == nil {
		t.Error("out-of-range index should fail")
	}
	if err := s.SetLessonEnabled(0, false); err != nil {
		t.Errorf("valid toggle: %v", err)
	}
	if len(s.P.EnabledLessons()) != 0 {
		t.Errorf("card should be disabled now: %+v", s.P.Lessons)
	}
}
