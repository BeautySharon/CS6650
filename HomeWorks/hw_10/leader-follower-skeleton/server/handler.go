package server

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"leader-follower/store"
)

type Node struct {
	Role          string
	NodeID        string
	Store         *store.Store
	ReplicaClient *ReplicaClient
	ReadMode      string
	WriteMode     string
}

func writeJSON(w http.ResponseWriter, s int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(s)
	_ = json.NewEncoder(w).Encode(v)
}

// HandleSet supports:
// - W1: leader writes locally, then replicates asynchronously
// - W5: leader writes locally, then waits for all 4 followers
// - W3: leader writes locally, then waits until total successes >= 3
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

	if n.Role != "leader" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "writes must go to leader"})
		return
	}

	version := n.Store.NextVersion(req.Key)
	entry := store.Entry{
		Value:   req.Value,
		Version: version,
	}

	// leader writes locally first
	if err := n.Store.SetLocal(req.Key, entry); err != nil {
		writeJSON(w, http.StatusInternalServerError, ErrorResponse{Error: err.Error()})
		return
	}

	switch n.WriteMode {
	case "W1":
		// Async replication: do not wait for followers
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			for _, f := range n.ReplicaClient.Followers {
				_ = n.ReplicaClient.ReplicateToFollower(ctx, f, ReplicateRequest{
					Key:     req.Key,
					Value:   req.Value,
					Version: version,
				})
			}
		}()

		writeJSON(w, http.StatusCreated, SetResponse{
			Message: "W1 async write",
			Key:     req.Key,
			Value:   req.Value,
			Version: version,
		})
		return

	case "W5":
		// Wait for all followers
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		successCount := 1 // leader itself

		for _, f := range n.ReplicaClient.Followers {
			err := n.ReplicaClient.ReplicateToFollower(ctx, f, ReplicateRequest{
				Key:     req.Key,
				Value:   req.Value,
				Version: version,
			})
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, ErrorResponse{
					Error: "failed to replicate to all followers",
				})
				return
			}
			successCount++
		}

		if successCount != 5 {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{
				Error: "W5 not satisfied",
			})
			return
		}

		writeJSON(w, http.StatusCreated, SetResponse{
			Message: "W5 synchronous write",
			Key:     req.Key,
			Value:   req.Value,
			Version: version,
		})
		return

	case "W3":
		// Quorum write: leader + any 2 followers
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		successCount := 1 // leader itself

		for _, f := range n.ReplicaClient.Followers {
			err := n.ReplicaClient.ReplicateToFollower(ctx, f, ReplicateRequest{
				Key:     req.Key,
				Value:   req.Value,
				Version: version,
			})
			if err == nil {
				successCount++
			}

			if successCount >= 3 {
				writeJSON(w, http.StatusCreated, SetResponse{
					Message: "W3 quorum write",
					Key:     req.Key,
					Value:   req.Value,
					Version: version,
				})
				return
			}
		}

		writeJSON(w, http.StatusInternalServerError, ErrorResponse{
			Error: "W3 quorum not satisfied",
		})
		return

	default:
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "unknown write mode"})
		return
	}
}

// HandleGet supports:
// - R1: leader local read only
// - R5: leader + all followers, choose newest version
// - R3: leader + any 2 followers, choose newest version
func (n *Node) HandleGet(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "missing key"})
		return
	}

	switch n.ReadMode {
	case "R1":
		// Read only from leader local state
		if n.Role != "leader" {
			writeJSON(w, http.StatusBadRequest, ErrorResponse{
				Error: "for R1, reads should go to leader",
			})
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
		return

	case "R5":
		// Read from leader + all followers, choose newest version
		results := []GetResponse{}

		if e, err := n.Store.GetLocal(key); err == nil {
			results = append(results, GetResponse{
				Key:     key,
				Value:   e.Value,
				Version: e.Version,
				Node:    n.NodeID,
			})
		}

		for _, f := range n.ReplicaClient.Followers {
			resp, err := http.Get(f + "/internal/read_local?key=" + key)
			if err != nil {
				continue
			}

			var gr GetResponse
			if err := json.NewDecoder(resp.Body).Decode(&gr); err == nil {
				results = append(results, gr)
			}
			resp.Body.Close()
		}

		if len(results) == 0 {
			writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
			return
		}

		latest := results[0]
		for _, rr := range results {
			if rr.Version > latest.Version {
				latest = rr
			}
		}

		writeJSON(w, http.StatusOK, latest)
		return

	case "R3":
		// Quorum read: leader + any 2 followers, choose newest version
		results := []GetResponse{}

		if e, err := n.Store.GetLocal(key); err == nil {
			results = append(results, GetResponse{
				Key:     key,
				Value:   e.Value,
				Version: e.Version,
				Node:    n.NodeID,
			})
		}

		successFollowerReads := 0
		for _, f := range n.ReplicaClient.Followers {
			if successFollowerReads >= 2 {
				break
			}

			resp, err := http.Get(f + "/internal/read_local?key=" + key)
			if err != nil {
				continue
			}

			var gr GetResponse
			if err := json.NewDecoder(resp.Body).Decode(&gr); err == nil {
				results = append(results, gr)
				successFollowerReads++
			}
			resp.Body.Close()
		}

		if len(results) < 3 {
			writeJSON(w, http.StatusInternalServerError, ErrorResponse{
				Error: "R3 quorum not satisfied",
			})
			return
		}

		latest := results[0]
		for _, rr := range results {
			if rr.Version > latest.Version {
				latest = rr
			}
		}

		writeJSON(w, http.StatusOK, latest)
		return

	default:
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "unknown read mode"})
		return
	}
}

// Followers receive replicated writes from leader.
// Assignment requires follower to sleep 100ms before applying the replicated write.
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
		Message: "replicated",
		Key:     req.Key,
		Value:   req.Value,
		Version: req.Version,
	})
}

// local_read is a testing-only endpoint.
// It bypasses consistency logic and directly returns this node's local value.
func (n *Node) HandleLocalRead(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "missing key"})
		return
	}

	e, err := n.Store.GetLocal(key)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
		return
	}

	writeJSON(w, http.StatusOK, GetResponse{
		Key:     key,
		Value:   e.Value,
		Version: e.Version,
		Node:    n.NodeID,
	})
}

// internal/read_local is used by leader when it needs replica reads
// for R=5 or R=3. Assignment requires follower to sleep 50ms
// when responding to a read request from leader.
func (n *Node) HandleInternalReadLocal(w http.ResponseWriter, r *http.Request) {
	key := r.URL.Query().Get("key")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, ErrorResponse{Error: "missing key"})
		return
	}

	if n.Role == "follower" {
		time.Sleep(50 * time.Millisecond)
	}

	e, err := n.Store.GetLocal(key)
	if err != nil {
		writeJSON(w, http.StatusNotFound, ErrorResponse{Error: "not found"})
		return
	}

	writeJSON(w, http.StatusOK, GetResponse{
		Key:     key,
		Value:   e.Value,
		Version: e.Version,
		Node:    n.NodeID,
	})
}