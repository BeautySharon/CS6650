package main

import (
	"fmt"
	"hash/fnv"
	"log"
	"sync"
)

// ---- Model ----

type Product struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Brand       string `json:"brand"`
}

// ---- Requirements ----

const (
	TotalProducts  = 100_000
	CheckPerSearch = 100
	MaxResults     = 20
)

// ---- In-memory store ----
// products is thread-safe for concurrent reads under load tests.
// productIDs is a stable list for deterministic bounded iteration.

var products sync.Map // key: int (id), value: Product
var productIDs []int

func generateProducts() {
	brands := []string{"Alpha", "Beta", "Gamma", "Delta", "Epsilon", "Zeta", "Omega", "Nova"}
	categories := []string{"Electronics", "Books", "Home", "Sports", "Beauty", "Toys", "Grocery", "Clothing"}

	productIDs = make([]int, 0, TotalProducts)

	for i := 1; i <= TotalProducts; i++ {
		brand := brands[(i-1)%len(brands)]
		category := categories[(i-1)%len(categories)]

		p := Product{
			ID:          i,
			Name:        fmt.Sprintf("Product %s %d", brand, i),
			Category:    category,
			Description: fmt.Sprintf("This is a sample description for product %d in %s.", i, category),
			Brand:       brand,
		}

		products.Store(i, p)
		productIDs = append(productIDs, i)
	}

	log.Printf("[INIT] Generated %d products\n", TotalProducts)
}

// ---- Utils ----

func hashToUint32(s string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	return h.Sum32()
}