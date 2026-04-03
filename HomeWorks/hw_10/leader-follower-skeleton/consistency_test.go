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
		t.Fatalf("expected status 201, got %d", resp.StatusCode)
	}

	var sr SetResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		t.Fatalf("failed to decode set response: %v", err)
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
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var gr GetResponse
	if err := json.NewDecoder(resp.Body).Decode(&gr); err != nil {
		t.Fatalf("failed to decode get response: %v", err)
	}
	return gr
}

func TestLeaderReadConsistent(t *testing.T) {
	leader := "http://localhost:8080"
	key := "consistency-leader"
	value := "v1"

	postSet(t, leader, key, value)

	gr := getValue(t, leader+"/get?key="+key)

	if gr.Value != value {
		t.Fatalf("expected leader value %q, got %q", value, gr.Value)
	}
	if gr.Version < 1 {
		t.Fatalf("expected version >= 1, got %d", gr.Version)
	}
}

func TestFollowerEventuallyConsistent(t *testing.T) {
	leader := "http://localhost:8080"
	follower1 := "http://localhost:8081"

	key := "consistency-follower"
	value := "v2"

	postSet(t, leader, key, value)

	// 给复制一点时间
	time.Sleep(700 * time.Millisecond)

	gr := getValue(t, follower1+"/local_read?key="+key)

	if gr.Value != value {
		t.Fatalf("expected follower value %q, got %q", value, gr.Value)
	}
	if gr.Version < 1 {
		t.Fatalf("expected version >= 1, got %d", gr.Version)
	}
}

func TestFollowerLocalReadMayBeStaleDuringReplication(t *testing.T) {
	leader := "http://localhost:8080"
	follower1 := "http://localhost:8081"

	key := "stale-window"

	// 先写一个旧值
	oldResp := postSet(t, leader, key, "old")
	time.Sleep(700 * time.Millisecond)

	foundStale := false

	for i := 0; i < 50; i++ {
		newValue := fmt.Sprintf("new-%d", i)
		newResp := postSet(t, leader, key, newValue)

		// 立刻去 follower 本地读，尽量卡在复制窗口内
		resp, err := http.Get(follower1 + "/local_read?key=" + key)
		if err != nil {
			t.Fatalf("local_read failed: %v", err)
		}

		if resp.StatusCode == http.StatusOK {
			var gr GetResponse
			_ = json.NewDecoder(resp.Body).Decode(&gr)

			// 如果 follower 还没追上 new version，就算观察到 stale
			if gr.Version < newResp.Version {
				foundStale = true
				resp.Body.Close()
				break
			}
		}
		resp.Body.Close()

		// 下一轮把 oldResp 更新一下，避免变量没意义
		oldResp = newResp
		_ = oldResp

		time.Sleep(20 * time.Millisecond)
	}

	if !foundStale {
		t.Log("did not observe stale read in this run; this may happen depending on timing")
	}
}