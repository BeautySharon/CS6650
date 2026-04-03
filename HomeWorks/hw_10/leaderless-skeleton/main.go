package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"leaderless-kv/server"
	"leaderless-kv/store"
)

func getEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func parsePeers(raw string) []string {
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
	nodeID := getEnv("NODE_ID", "node1")
	port := getEnv("PORT", "8080")
	peers := parsePeers(os.Getenv("PEERS"))

	node := &server.Node{
		NodeID:        nodeID,
		Store:         store.NewStore(),
		ReplicaClient: server.NewReplicaClient(peers),
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/set", node.HandleSet)
	mux.HandleFunc("/get", node.HandleGet)
	mux.HandleFunc("/local_read", node.HandleLocalRead)
	mux.HandleFunc("/internal/replicate", node.HandleReplicate)

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	log.Printf("starting node=%s port=%s peers=%v", nodeID, port, peers)

	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}