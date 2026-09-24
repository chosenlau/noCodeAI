package main

import (
	"github.com/chosenlau/noCodeAI/wire"
)

func main() {

	h, err := wire.InitializeApp()
	if err != nil {
		panic(err)
	}
	h.Spin()
}
