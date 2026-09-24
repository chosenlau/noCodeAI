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

	deletedAtOpt := gen.FieldType("deleted_at", "gorm.DeletedAt")

	g.ApplyBasic(
		g.GenerateModel("app", gen.FieldJSONTag("id", "id,string"), deletedAtOpt),
		g.GenerateModel("user", gen.FieldJSONTag("id", "id,string"), deletedAtOpt),
		g.GenerateModel("chat_history", gen.FieldJSONTag("id", "id,string"), deletedAtOpt),
	)
	g.Execute()
}
