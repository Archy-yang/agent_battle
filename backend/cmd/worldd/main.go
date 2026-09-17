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
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"agentbattle/backend/engine"
	"agentbattle/backend/match"
	"agentbattle/backend/web"
)

const version = "v0.2.0"

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
	flag.Parse()
	if *config != "" {
		loadEnvFile(*config)
	}

	reg := match.NewRegistry()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		match.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": version})
	})

	mux.HandleFunc("POST /api/match", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Seed uint64 `json:"seed"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body) // empty body allowed
		m := reg.Create(body.Seed)
		log.Printf("created %s (seed=%d, pilot=%s)", m.ID, m.Seed, m.PilotName)
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
		m.AddGuidance(body.Kind, body.Text)
		match.WriteJSON(w, http.StatusOK, m.View())
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
