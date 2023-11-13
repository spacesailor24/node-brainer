package main

import (
	"fmt"
	"os"

	"github.com/spacesailor24/node-brainer/tui"
)

func main() {
	_ = clients.NewGethClient()
}

func startApp() {
	t := tui.NewTUI()
	err := t.Start()
	if err != nil {
		fmt.Printf("failed to start: %v\n", err)
		os.Exit(1)
	}
}
