package main

import (
	"fmt"
	"os"

	"github.com/medidew/lyrion-tui/internal/config"
	"github.com/medidew/lyrion-tui/internal/lyrionapi"
	"github.com/medidew/lyrion-tui/internal/tui"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	server, err := lyrionapi.Connect(cfg.ServerAddress)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer server.Close()

	if err := tui.NewApp(server).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
