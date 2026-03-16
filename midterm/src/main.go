package main

import (
	"log"
	"math/rand"
	"net/http"
	"time"
)

var R *Resilience

func main() {
	// Seed RNG so failure injection isn't identical across restarts.
	rand.Seed(time.Now().UnixNano())

	generateProducts()

	// Initialize resilience component:
	// - Start with all toggles OFF (baseline mode).
	// - For crash demo, turn EnableInjection ON to introduce slow/flaky dependency.
	// - For recovery demo, enable FailFast / Bulkhead / CircuitBreaker as needed.
	R = NewResilience(ResilienceConfig{
		EnableInjection:      true,
		EnableFailFast:       false,
		EnableBulkhead:       false,
		EnableCircuitBreaker: true,

		InjectSleepMs: 2000, // slow dependency latency
		InjectFailPct: 100,   // failure probability (%)

		TimeoutMs:        300,   // fail-fast timeout
		MaxConcurrent:    50,    // bulkhead concurrency limit
		FailThreshold:    20,    // CB opens after N consecutive failures
		OpenDurationMs:   10000, // CB stays open for this duration
		HalfOpenMaxProbe: 5,     // max probe requests in half-open state
	})

	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/products/search", productSearchHandler)

	// NEW: view stats quickly (for screenshots!)
	http.HandleFunc("/resilience/stats", resilienceStatsHandler)

	http.HandleFunc("/products", listProductsHandler)
	http.HandleFunc("/products/", productByIDHandler)

	log.Println("Server running on :8080")

srv := &http.Server{
	Addr:         ":8080",
	ReadTimeout:  2 * time.Second,
	WriteTimeout: 2 * time.Second,
	IdleTimeout:  30 * time.Second,
}

log.Fatal(srv.ListenAndServe())
	
}