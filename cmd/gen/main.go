package main

import (
	"github.com/chosenlau/noCodeAI/config"
	"github.com/chosenlau/noCodeAI/internal/dal"
	"gorm.io/gen"
)

func main() {
	config.InitConfig()
	db := dal.InitDB(config.GlobalConfig)
	g := gen.NewGenerator(gen.Config{
		OutPath:      "./internal/dal/query",
		ModelPkgPath: "model",
		Mode:         gen.WithDefaultQuery | gen.WithQueryInterface,
	})
	g.UseDB(db)

	g.ApplyBasic(g.GenerateModel("user", gen.FieldJSONTag("id", "id,string")))
	g.Execute()
}
