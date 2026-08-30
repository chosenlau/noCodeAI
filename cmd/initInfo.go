package main

import (
	"fmt"

	"github.com/chosenlau/noCodeAI/config"
)

func initInfo() {
	fmt.Printf("Server port:%d\n", config.GlobalConfig.Server.Port)
	fmt.Printf("Database host:%s\n", config.GlobalConfig.Database.Host)
	fmt.Printf("Ai model:%s\n", config.GlobalConfig.AI.Model)
	dsn := config.GlobalConfig.GetDatabaseDSN()
	fmt.Printf("Database dsn:%s\n", dsn)
}
