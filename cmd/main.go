package main

import (
	"flag"
	"fmt"

	"github.com/chosenlau/noCodeAI/config"
	"github.com/chosenlau/noCodeAI/internal/router"
	"github.com/cloudwego/hertz/pkg/app/server"
)

func initServer() *server.Hertz {
	cfg := config.GlobalConfig

	h := server.Default(
		server.WithHostPorts(fmt.Sprintf(":%d", cfg.Server.Port)),
		server.WithBasePath(cfg.Server.ContextPath),
	)
	router.RegisterRoutes(h)
	return h
}
func main() {
	env := flag.String("env", "", "executing environment:local,dev,test")

	flag.Parse()
	if env == nil || *env == "" {
		panic("-env is required,executing environment:local,dev,test")
	}
	config.InitConfig(*env)
	initInfo()

	h:= initServer()
	h.Spin()
}
