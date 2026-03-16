package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ---- Small helpers ----

func requireGET(w http.ResponseWriter, r *http.Request) bool {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func matchesQuery(p Product, qLower string) bool {
	return strings.Contains(strings.ToLower(p.Name), qLower) ||
		strings.Contains(strings.ToLower(p.Category), qLower)
}

func startIndexForQuery(q string) int {
	if len(productIDs) == 0 {
		return 0
	}
	return int(hashToUint32(q) % uint32(len(productIDs)))
}

// ---- Handlers ----

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func productSearchHandler(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}

	startTime := time.Now()
	qLower := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	start := startIndexForQuery(qLower)

	checked := 0
	totalFound := 0
	results := make([]Product, 0, MaxResults)

	// Bounded iteration: EXACTLY CheckPerSearch inspections per request
	for i := 0; i < CheckPerSearch; i++ {
		id := productIDs[(start+i)%len(productIDs)]

		// Count every inspected item as work (even if not found)
		checked++

		v, ok := products.Load(id)
		if !ok {
			continue
		}

		// Empty query: still do bounded work, but no matches
		if qLower == "" {
			continue
		}

		p := v.(Product)

		if matchesQuery(p, qLower) {
			totalFound++
			if len(results) < MaxResults {
				results = append(results, p)
			}
		}
	}

	// ---- Resilience demo hook (failure injection + recovery patterns) ----
	// We simulate an "external dependency" that can be slow or flaky.
	// Even if the external call fails, we DO NOT fail the whole request.
	// Instead, we return the local search result and mark the response as degraded.
	extErr := R.CallExternal(r.Context())

	degraded := false
	extStatus := "ok"
	if extErr != nil {
		degraded = true
		extStatus = extErr.Error()
		// Fallback behavior:
		// - still return local search results
		// - client can observe degraded=true and external_status for evidence
	}


	resp := map[string]any{
		"products":       results,
		"total_found":    totalFound,
		"checked":        checked,
		"search_time_ms": time.Since(startTime).Milliseconds(),

		// Evidence fields for the midterm crash/recovery demo
		"external_status": extStatus,
		"degraded":        degraded,
	}
	writeJSON(w, resp)
}

func listProductsHandler(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}

	all := make([]Product, 0, TotalProducts)
	products.Range(func(_, value any) bool {
		all = append(all, value.(Product))
		return true
	})

	writeJSON(w, all)
}

func productByIDHandler(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}

	idStr := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/products/"))
	if idStr == "" {
		http.Error(w, "Missing id", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		http.Error(w, "Invalid id", http.StatusBadRequest)
		return
	}

	v, ok := products.Load(id)
	if !ok {
		http.Error(w, "Product not found", http.StatusNotFound)
		return
	}

	writeJSON(w, v.(Product))
}

func resilienceStatsHandler(w http.ResponseWriter, r *http.Request) {
	if !requireGET(w, r) {
		return
	}
	writeJSON(w, R.Stats())
}