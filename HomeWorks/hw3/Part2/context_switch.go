package main

import (
	"fmt"
	"runtime"
	"time"
)

const N = 1_000_000

func pingPong() time.Duration {
	ch := make(chan struct{})
	done := make(chan struct{})

	start := time.Now()

	// goroutine A
	go func() {
		for i := 0; i < N; i++ {
			<-ch
			ch <- struct{}{}
		}
		done <- struct{}{}
	}()

	// goroutine B
	go func() {
		for i := 0; i < N; i++ {
			ch <- struct{}{}
			<-ch
		}
		done <- struct{}{}
	}()

	<-done
	<-done
	return time.Since(start)
}

func main() {
	// 1. 1 OS thread
	runtime.GOMAXPROCS(1)
	d1 := pingPong()
	avg1 := float64(d1.Nanoseconds()) / float64(2*N)
	fmt.Println("GOMAXPROCS = 1")
	fmt.Println("Total time =", d1)
	fmt.Println("Avg switch time (ns) =", avg1)

	fmt.Println()

	// 2. multiple OS threads
	runtime.GOMAXPROCS(runtime.NumCPU())
	d2 := pingPong()
	avg2 := float64(d2.Nanoseconds()) / float64(2*N)
	fmt.Println("GOMAXPROCS = NumCPU")
	fmt.Println("Total time =", d2)
	fmt.Println("Avg switch time (ns) =", avg2)
}
