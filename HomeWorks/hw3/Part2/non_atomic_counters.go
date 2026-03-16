package main

import (
	"fmt"
	"sync"
)

func main() {
	var ops uint64
	var wg sync.WaitGroup

	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				ops++  
			}
		}()
	}

	wg.Wait()
	fmt.Println("ops:", ops)
}
