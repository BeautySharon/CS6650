package main

import (
	"fmt"
	"sync"
	"time"
)

type SafeMap struct {
	mu sync.RWMutex
	m  map[int]int
}

func main() {
	sm := SafeMap{
		m: make(map[int]int),
	}

	start := time.Now()

	var wg sync.WaitGroup
	wg.Add(50)

	for g := 0; g < 50; g++ {
		g := g
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				key := g*1000 + i

				sm.mu.Lock()
				sm.m[key] = i
				sm.mu.Unlock()
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	fmt.Println("len(m) =", len(sm.m))
	fmt.Println("time   =", elapsed)
}
