package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"leaderless-kv/store"
)

type Node struct {
	NodeID        string
	Store         *store.Store
	ReplicaClient *ReplicaClient
}

func writeJSON(w http.ResponseWriter, s int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(s)
	_ = json.NewEncoder(w).Encode(v)
}

// Leaderless W=N:
// whoever receives the write becomes the write coordinator.
// It must write locally, replicate to all peers, wait for all to complete,
// and only then return 201.
func (n *Node) HandleSet(w http.ResponseWriter, r *http.Request) {
	var req SetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}

	if req.Key == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "key cannot be empty"})
		return
	}

	version := n.Store.NextVersion(req.Key)
	entry := store.Entry{
		Value:   req.Value,
		Version: version,
	}

	// coordinator writes locally first
	if err := n.Store.SetLocal(req.Key, entry); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	successCount := 1 // self
	required := len(n.ReplicaClient.Peers) + 1

	for _, peer := range n.ReplicaClient.Peers {
		err := n.ReplicaClient.ReplicateToPeer(ctx, peer, ReplicateRequest{
			Key:     req.Key,
			Value:   req.Value,
			Version: version,
		})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{
				Error: "failed to replicate to all peers",
			})
			return
		}
		successCount++
	}

	if successCount != required {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{
			Error: "W=N not satisfied",
		})
		return
	}

	writeJSON(w, http.StatusCreated, SetResponse{
		Message: "leaderless write committed",
		Key:     req.Key,
		Value:   req.Value,
		Version: version,
	})
}

// Leaderless R=1:
// client read returns only this node's local value.
func (n *Node) HandleGet(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "missing key"})
		return
	}

	entry, err := n.Store.GetLocal(key)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
		return
	}

	writeJSON(w, http.StatusOK, GetResponse{
		Key:     key,
		Value:   entry.Value,
		Version: entry.Version,
		Node:    n.NodeID,
	})
}

// internal replicated write.
// Keep artificial delay to expose inconsistency window.
func (n *Node) HandleReplicate(w http.ResponseWriter, r *http.Request) {
	var req ReplicateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "invalid request body"})
		return
	}

	time.Sleep(100 * time.Millisecond)

	if err := n.Store.SetLocal(req.Key, store.Entry{
		Value:   req.Value,
		Version: req.Version,
	}); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, SetResponse{
		Message: "replicated to peer",
		Key:     req.Key,
		Value:   req.Value,
		Version: req.Version,
	})
}

// local_read is still useful for testing-only direct local inspection.
func (n *Node) HandleLocalRead(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "missing key"})
		return
	}

	entry, err := n.Store.GetLocal(key)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
		return
	}

	writeJSON(w, http.StatusOK, GetResponse{
		Key:     key,
		Value:   entry.Value,
		Version: entry.Version,
		Node:    n.NodeID,
	})
}