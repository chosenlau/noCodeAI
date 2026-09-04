package main

import (
	"github.com/chosenlau/noCodeAI/config"
	"github.com/chosenlau/noCodeAI/wire"
)

func main() {

	config.InitConfig()

	h, err := wire.InitializeApp()
	if err != nil {
		panic(err)
	}
	h.Spin()
}
