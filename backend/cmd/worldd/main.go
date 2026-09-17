// Command worldd is the entry point of the agent_battle world server.
//
// Scaffold stage: it serves the embedded H5 frontend at / and a health
// endpoint at /api/health. The engine, scenario loader, agent runtime
// and match API land in subsequent milestones — see docs/world_rule.md
// §31 (v0.1 scope) and docs/world_genesis.md.
package main

import (
	"fmt"
	"io/fs"
	"log"
	"net/http"

	"agentbattle/backend/web"
)

const version = "v0.0.1"

func main() {
	dist, err := fs.Sub(web.FS, "dist")
	if err != nil {
		log.Fatalf("embedded frontend unavailable: %v", err)
	}

	http.HandleFunc("/api/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"status":"ok","version":%q}`+"\n", version)
	})
	http.Handle("/", http.FileServer(http.FS(dist)))

	addr := ":8080"
	log.Printf("agentbattle worldd %s listening on %s", version, addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
