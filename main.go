package main

import (
	"fmt"

	lyrionapi "github.com/medidew/lyrion-tui/internal"
)

func main() {
	//lyrion_tui := tview.NewApplication()
	//box := tview.NewBox().SetBorder(true).SetTitle("Hello, world!")

	lyrion_server, err := lyrionapi.Connect("192.168.1.4:9090")
	if err != nil {
		panic(err)
	}
	defer lyrion_server.Close()

	fmt.Printf("lyrion_server: %v\n", lyrion_server)

	count, err := lyrion_server.GetPlayerCount()
	if err != nil {
		panic(err)
	}
	fmt.Printf("count: %v\n", count)

	player, err := lyrion_server.GetPlayer(0)
	if err != nil {
		panic(err)
	}
	fmt.Printf("id: %v\n", player)

	lyrion_server.GetGenres(0, 1)

	//if err := lyrion_tui.SetRoot(box, true).Run(); err != nil {
	//	panic(err)
	//}
}
