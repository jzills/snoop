// Command probe parses every local transcript and reports timing, as a
// smoke test of the parser and store against real data.
package main

import (
	"fmt"
	"time"

	"github.com/jzills/snoop/internal/store"
)

func main() {
	t0 := time.Now()
	infos, err := store.List(store.Root())
	if err != nil {
		panic(err)
	}
	fmt.Printf("list: %d sessions in %v\n", len(infos), time.Since(t0))
	for _, i := range infos[:min(5, len(infos))] {
		fmt.Printf("  %-60.60s  %s  %s\n", i.Name(), i.Branch, i.Project())
	}

	t0 = time.Now()
	var events, tools, bytes int64
	var slowest time.Duration
	var slowName string
	for _, i := range infos {
		t1 := time.Now()
		s, _, err := store.Load(i.Path)
		if err != nil {
			fmt.Println("ERR", i.Path, err)
			continue
		}
		if d := time.Since(t1); d > slowest {
			slowest, slowName = d, fmt.Sprintf("%s (%d MB)", i.ID, i.Size>>20)
		}
		events += int64(len(s.Events))
		for _, e := range s.Events {
			if e.Tool != nil {
				tools++
			}
		}
		bytes += i.Size
	}
	fmt.Printf("parse all: %d MB, %d events, %d tool calls in %v; slowest %v %s\n",
		bytes>>20, events, tools, time.Since(t0), slowest, slowName)
}
