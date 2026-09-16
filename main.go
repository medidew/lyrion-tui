package main

import (
	"fmt"
	"os"

	"github.com/medidew/lyrion-tui/internal/lyrionapi"
	"github.com/medidew/lyrion-tui/internal/tui"
)

func main() {
	server, err := lyrionapi.Connect("192.168.1.4:9090")
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect: %v\n", err)
		os.Exit(1)
	}
	defer server.Close()

	if err := tui.NewApp(server).Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}
