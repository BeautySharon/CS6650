package server

import (
    "bytes"
    "context"
    "encoding/json"
    "net/http"
    "strings"
    "time"
)

type ReplicaClient struct {
    Client    *http.Client
    Followers []string
}

func NewReplicaClient(f []string) *ReplicaClient {
    return &ReplicaClient{
        Client: &http.Client{Timeout: 3 * time.Second},
        Followers: f,
    }
}

func (rc *ReplicaClient) ReplicateToFollower(ctx context.Context, url string, req ReplicateRequest) error {
    body, _ := json.Marshal(req)

    httpReq, _ := http.NewRequestWithContext(ctx,
        http.MethodPost,
        strings.TrimRight(url, "/")+"/internal/replicate",
        bytes.NewReader(body),
    )
    httpReq.Header.Set("Content-Type", "application/json")

    resp, err := rc.Client.Do(httpReq)
    time.Sleep(200 * time.Millisecond)
    if err != nil {
        return err
    }
    resp.Body.Close()
    return nil
}
