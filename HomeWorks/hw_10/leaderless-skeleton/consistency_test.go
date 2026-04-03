package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

type SetResponse struct {
	Message string `json:"message"`
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version int64  `json:"version"`
}

type GetResponse struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Version int64  `json:"version"`
	Node    string `json:"node"`
}

func postSet(t *testing.T, baseURL, key, value string) SetResponse {
	t.Helper()

	body := fmt.Sprintf(`{"key":"%s","value":"%s"}`, key, value)

	resp, err := http.Post(
		baseURL+"/set",
		"application/json",
		bytes.NewBufferString(body),
	)
	if err != nil {
		t.Fatalf("set request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var sr SetResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		t.Fatalf("decode set response failed: %v", err)
	}
	return sr
}

func getValue(t *testing.T, url string) GetResponse {
	t.Helper()

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("get request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var gr GetResponse
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		t.Fatalf("decode get response failed: %v", err)
	}
	return gr
}

func TestLeaderlessCoordinatorReadConsistent(t *testing.T) {
	node1 := "http://localhost:8080"
	key := "leaderless-coordinator"
	value := "v1"

	postSet(t, node1, key, value)

	gr := getValue(t, node1+"/get?key="+key)
	if gr.Value != value {
		t.Fatalf("expected %q, got %q", value, gr.Value)
	}
}

func TestLeaderlessOtherNodeEventuallyConsistent(t *testing.T) {
	node1 := "http://localhost:8080"
	node2 := "http://localhost:8081"
	key := "leaderless-eventual"
	value := "v2"

	postSet(t, node1, key, value)

	time.Sleep(700 * time.Millisecond)

	gr := getValue(t, node2+"/get?key="+key)
	if gr.Value != value {
		t.Fatalf("expected %q, got %q", value, gr.Value)
	}
}

func TestLeaderlessMayExposeInconsistencyWindow(t *testing.T) {
	node1 := "http://localhost:8080"
	node2 := "http://localhost:8081"
	key := "leaderless-stale"

	postSet(t, node1, key, "old")
	time.Sleep(700 * time.Millisecond)

	foundStale := false

	for i := 0; i < 50; i++ {
		newValue := fmt.Sprintf("new-%d", i)
		newResp := postSet(t, node1, key, newValue)

		resp, err := http.Get(node2 + "/get?key=" + key)
		if err != nil {
			t.Fatalf("get failed: %v", err)
		}

		if resp.StatusCode == http.StatusOK {
			var gr GetResponse
			_ = json.NewDecoder(resp.Body).Decode(&gr)
			if gr.Version < newResp.Version {
				foundStale = true
				resp.Body.Close()
				break
			}
		}
		resp.Body.Close()

		time.Sleep(20 * time.Millisecond)
	}

	if !foundStale {
		t.Log("did not observe inconsistency in this run; timing dependent")
	}
}