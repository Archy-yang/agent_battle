// Command worldd is the entry point of the agent_battle world server.
//
// It serves the embedded H5 frontend at / and the match API under
// /api: create matches, advance turns with player intents (or the
// scripted pilot), send guidance and read observations. The true
// World State never leaves this process (world_rule §32).
package main

import (
	"encoding/json"
	"flag"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"agentbattle/backend/ctmatch"
	"agentbattle/backend/engine"
	"agentbattle/backend/match"
	"agentbattle/backend/profile"
	"agentbattle/backend/web"
)

const version = "v0.8.0"

// loadEnvFile reads KEY=VALUE lines and sets any not already present
// in the environment (server-side LLM config; the key never reaches
// clients).
func loadEnvFile(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return // 缺省配置不存在时静默跳过
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		if _, exists := os.LookupEnv(k); !exists {
			_ = os.Setenv(k, v)
		}
	}
	log.Printf("loaded config from %s", path)
}

func main() {
	config := flag.String("config", "llm.env", "env file (KEY=VALUE) loaded at startup")
	profilePath := flag.String("profile", "profile.json", "creator profile (agent 养成档案)")
	matchesDir := flag.String("matches", "matches", "对局落盘目录（相对工作目录；空串关闭持久化）")
	flag.Parse()
	if *config != "" {
		loadEnvFile(*config)
	}
	prof := profile.Load(*profilePath)

	// WP-B：对局落盘 + 重启后经确定性重演惰性恢复（match/persistence.go）.
	reg := match.NewRegistryPersisted(prof, *matchesDir)

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		match.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": version})
	})

	// WP-5：修炼会话 API（/api/cult/*）。感知隔离律在 API 层成立：
	// 玩家界面载荷只有定性词/band，真值仅在 ?debug=1 下输出。
	cultreg := ctmatch.NewCultRegistry()
	mux.HandleFunc("POST /api/cult/session", cultreg.HandleCreate)
	mux.HandleFunc("GET /api/cult/session/{id}", cultreg.HandleView)
	mux.HandleFunc("POST /api/cult/session/{id}/intents", cultreg.HandleIntents)
	mux.HandleFunc("POST /api/cult/session/{id}/ask", cultreg.HandleAsk)
	mux.HandleFunc("POST /api/cult/session/{id}/guidance", cultreg.HandleGuidance)
	mux.HandleFunc("POST /api/cult/session/{id}/tick", cultreg.HandleTick)
	mux.HandleFunc("POST /api/cult/session/{id}/protocols", cultreg.HandleProtocols)
	mux.HandleFunc("POST /api/cult/session/{id}/protocols/{name}/start", cultreg.HandleStartProtocol)

	mux.HandleFunc("GET /api/match/{id}/recording", reg.ServeRecording)
	mux.HandleFunc("POST /api/replay", match.ServeReplay)

	mux.HandleFunc("POST /api/match", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Seed     uint64 `json:"seed"`
			Scenario string `json:"scenario"` // "mvp"(默认) | "v02" | "v04"
		}
		_ = json.NewDecoder(r.Body).Decode(&body) // empty body allowed
		m := reg.Create(body.Seed, body.Scenario)
		if m == nil { // 未知场景名
			match.WriteErr(w, http.StatusBadRequest, "未知场景 "+body.Scenario)
			return
		}
		log.Printf("created %s (seed=%d, scenario=%s, pilot=%s)", m.ID, m.Seed, m.PackName, m.PilotName)
		match.WriteJSON(w, http.StatusCreated, m.View())
	})

	mux.HandleFunc("GET /api/match/{id}/state", func(w http.ResponseWriter, r *http.Request) {
		m := reg.Get(r.PathValue("id"))
		if m == nil {
			match.WriteErr(w, http.StatusNotFound, "match not found")
			return
		}
		match.WriteJSON(w, http.StatusOK, m.View())
	})

	mux.HandleFunc("POST /api/match/{id}/tick", func(w http.ResponseWriter, r *http.Request) {
		m := reg.Get(r.PathValue("id"))
		if m == nil {
			match.WriteErr(w, http.StatusNotFound, "match not found")
			return
		}
		var body struct {
			Intents []engine.Intent `json:"intents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			match.WriteErr(w, http.StatusBadRequest, "bad intents: "+err.Error())
			return
		}
		m.Tick(body.Intents)
		match.WriteJSON(w, http.StatusOK, m.View())
	})

	mux.HandleFunc("POST /api/match/{id}/auto", func(w http.ResponseWriter, r *http.Request) {
		m := reg.Get(r.PathValue("id"))
		if m == nil {
			match.WriteErr(w, http.StatusNotFound, "match not found")
			return
		}
		m.TickAuto()
		match.WriteJSON(w, http.StatusOK, m.View())
	})

	mux.HandleFunc("POST /api/match/{id}/guidance", func(w http.ResponseWriter, r *http.Request) {
		m := reg.Get(r.PathValue("id"))
		if m == nil {
			match.WriteErr(w, http.StatusNotFound, "match not found")
			return
		}
		var body match.Guidance
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Text == "" {
			match.WriteErr(w, http.StatusBadRequest, "bad guidance")
			return
		}
		if err := m.AddGuidance(body.Kind, body.Text); err != nil {
			match.WriteErr(w, http.StatusBadRequest, err.Error())
			return
		}
		match.WriteJSON(w, http.StatusOK, m.View())
	})

	mux.HandleFunc("POST /api/match/{id}/predict", func(w http.ResponseWriter, r *http.Request) {
		m := reg.Get(r.PathValue("id"))
		if m == nil {
			match.WriteErr(w, http.StatusNotFound, "match not found")
			return
		}
		var body struct {
			Choice string `json:"choice"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			match.WriteErr(w, http.StatusBadRequest, "bad predict")
			return
		}
		if err := m.Predict(body.Choice); err != nil {
			match.WriteErr(w, http.StatusBadRequest, err.Error())
			return
		}
		match.WriteJSON(w, http.StatusOK, m.View())
	})

	mux.HandleFunc("POST /api/match/{id}/ask", func(w http.ResponseWriter, r *http.Request) {
		m := reg.Get(r.PathValue("id"))
		if m == nil {
			match.WriteErr(w, http.StatusNotFound, "match not found")
			return
		}
		var body struct {
			Question string `json:"question"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Question == "" {
			match.WriteErr(w, http.StatusBadRequest, "bad question")
			return
		}
		ans, err := m.Ask(body.Question)
		if err != nil {
			match.WriteErr(w, http.StatusBadRequest, err.Error())
			return
		}
		match.WriteJSON(w, http.StatusOK, map[string]string{"answer": ans})
	})

	mux.HandleFunc("POST /api/match/{id}/pause", func(w http.ResponseWriter, r *http.Request) {
		m := reg.Get(r.PathValue("id"))
		if m == nil {
			match.WriteErr(w, http.StatusNotFound, "match not found")
			return
		}
		if err := m.Pause(); err != nil {
			match.WriteErr(w, http.StatusBadRequest, err.Error())
			return
		}
		match.WriteJSON(w, http.StatusOK, m.View())
	})

	mux.HandleFunc("GET /api/profile", func(w http.ResponseWriter, _ *http.Request) {
		match.WriteJSON(w, http.StatusOK, prof.P)
	})

	mux.HandleFunc("POST /api/profile", func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			match.WriteErr(w, http.StatusBadRequest, "bad profile")
			return
		}
		if err := match.ApplyProfilePatch(prof, data); err != nil {
			match.WriteErr(w, http.StatusBadRequest, err.Error())
			return
		}
		match.WriteJSON(w, http.StatusOK, prof.P)
	})

	dist, err := fs.Sub(web.FS, "dist")
	if err != nil {
		log.Fatalf("embedded frontend unavailable: %v", err)
	}
	mux.Handle("/", http.FileServer(http.FS(dist)))

	addr := os.Getenv("PORT")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("agentbattle worldd %s listening on %s", version, addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
