package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"leader-follower/server"
	"leader-follower/store"
)

func getEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func parseFollowers(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}

	parts := strings.Split(raw, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func main() {
	role := getEnv("ROLE", "leader")
	nodeID := getEnv("NODE_ID", "node-1")
	port := getEnv("PORT", "8080")
	writeMode := getEnv("WRITE_MODE", "W1")
	readMode := getEnv("READ_MODE", "R5")
	followers := parseFollowers(os.Getenv("FOLLOWERS"))

	node := &server.Node{
		Role:          role,
		NodeID:        nodeID,
		Store:         store.NewStore(),
		ReplicaClient: server.NewReplicaClient(followers),
		ReadMode:      readMode,
		WriteMode:     writeMode,
	}

	mux := http.NewServeMux()

	// client endpoints
	mux.HandleFunc("/set", node.HandleSet)
	mux.HandleFunc("/get", node.HandleGet)
	mux.HandleFunc("/local_read", node.HandleLocalRead)

	// internal endpoints
	mux.HandleFunc("/internal/replicate", node.HandleReplicate)
	mux.HandleFunc("/internal/read_local", node.HandleInternalReadLocal)

	// health
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	log.Printf("starting node=%s role=%s port=%s writeMode=%s readMode=%s followers=%v",
		nodeID, role, port, writeMode, readMode, followers)

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}