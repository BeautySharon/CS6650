package main

import (
	"bufio"
	"fmt"
	"os"
	"time"
)

const LINES = 100000

func writeUnbuffered(path string) time.Duration {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	start := time.Now()
	for i := 0; i < LINES; i++ {
		_, err := f.Write([]byte("hello\n"))
		if err != nil {
			panic(err)
		}
	}
	return time.Since(start)
}

func writeBuffered(path string) time.Duration {
	f, err := os.Create(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)

	start := time.Now()
	for i := 0; i < LINES; i++ {
		_, err := w.WriteString("hello\n")
		if err != nil {
			panic(err)
		}
	}

	if err := w.Flush(); err != nil {
		panic(err)
	}

	return time.Since(start)
}

func main() {
	d1 := writeUnbuffered("unbuffered.txt")
	d2 := writeBuffered("buffered.txt")

	fmt.Println("unbuffered =", d1)
	fmt.Println("buffered   =", d2)
}
