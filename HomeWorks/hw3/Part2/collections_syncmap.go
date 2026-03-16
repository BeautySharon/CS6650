package main

import (
	"fmt"
	"sync"
	"time"
)

func main() {
	var m sync.Map

	start := time.Now()

	var wg sync.WaitGroup
	wg.Add(50)

	for g := 0; g < 50; g++ {
		g := g
		go func() {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				m.Store(g*1000+i, i)
			}
		}()
	}

	wg.Wait()

	// count entries
	count := 0
	m.Range(func(key, value any) bool {
		count++
		return true
	})

	elapsed := time.Since(start)

	fmt.Println("len(m) =", count)
	fmt.Println("time   =", elapsed)
}
