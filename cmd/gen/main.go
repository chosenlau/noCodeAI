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
		g.GenerateModel("run_checkpoint", gen.FieldType("document", "[]byte")),
		g.GenerateModel("app_execution"),
		g.GenerateModel("tool_execution"),
		g.GenerateModel("app", gen.FieldJSONTag("id", "id,string"), deletedAtOpt),
		g.GenerateModel("user", gen.FieldJSONTag("id", "id,string"), deletedAtOpt),
		g.GenerateModel("chat_history", gen.FieldJSONTag("id", "id,string"), deletedAtOpt),
		g.GenerateModel("agent_run", gen.FieldJSONTag("id", "id,string"), gen.FieldType("input_json", "[]byte"), gen.FieldType("active_key", "*string"), gen.FieldType("source_version_id", "*int64"), gen.FieldType("started_at", "*time.Time"), gen.FieldType("finished_at", "*time.Time")),
		g.GenerateModel("run_event", gen.FieldJSONTag("id", "id,string"), gen.FieldType("payload", "[]byte")),
		g.GenerateModel("source_version", gen.FieldJSONTag("id", "id,string"), gen.FieldType("files_json", "[]byte")),
		g.GenerateModel("task_outbox", gen.FieldJSONTag("id", "id,string"), gen.FieldType("payload", "[]byte"), gen.FieldType("attempts", "int"), gen.FieldType("claimed_at", "*time.Time"), gen.FieldType("delivered_at", "*time.Time")),
	)
	g.Execute()
}
