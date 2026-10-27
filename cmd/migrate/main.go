// migrate applies only the additive task-runtime schema; it never runs down SQL.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/chosenlau/noCodeAI/config"
	"github.com/chosenlau/noCodeAI/internal/dal"
)

func main() {
	cfg := config.InitConfig()
	db := dal.InitDB(cfg)
	root, err := config.GetProjectRootPath()
	if err != nil {
		panic(err)
	}
	paths, err := filepath.Glob(filepath.Join(root, "migrations", "*.up.sql"))
	if err != nil {
		panic(err)
	}
	if err := db.Exec("CREATE TABLE IF NOT EXISTS schema_migration (name VARCHAR(255) PRIMARY KEY, applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP)").Error; err != nil {
		panic(err)
	}
	for _, path := range paths {
		var count int64
		if err := db.Table("schema_migration").Where("name = ?", filepath.Base(path)).Count(&count).Error; err != nil {
			panic(err)
		}
		if count > 0 {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			panic(err)
		}
		for _, statement := range strings.Split(string(content), ";") {
			if strings.TrimSpace(statement) == "" {
				continue
			}
			if err := db.Exec(statement).Error; err != nil {
				panic(err)
			}
		}
		if err := db.Exec("INSERT INTO schema_migration (name) VALUES (?)", filepath.Base(path)).Error; err != nil {
			panic(err)
		}
	}
	fmt.Println("Task runtime schema applied (additive, idempotent).")
}
