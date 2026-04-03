package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type ReplicaClient struct {
	Client *http.Client
	Peers  []string
}

func NewReplicaClient(peers []string) *ReplicaClient {
	return &ReplicaClient{
		Client: &http.Client{
			Timeout: 3 * time.Second,
		},
		Peers: peers,
	}
}

// Leaderless write coordinator sends updates to all other nodes.
// To make inconsistency window visible, we keep the same artificial delay style.
func (rc *ReplicaClient) ReplicateToPeer(ctx context.Context, peerURL string, req ReplicateRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		strings.TrimRight(peerURL, "/")+"/internal/replicate",
		bytes.NewReader(body),
	)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := rc.Client.Do(httpReq)

	// artificial propagation delay
	time.Sleep(200 * time.Millisecond)

	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("replication failed, peer=%s status=%d", peerURL, resp.StatusCode)
	}

	return nil
}