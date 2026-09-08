package prompt

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/chosenlau/noCodeAI/config"

	"github.com/cloudwego/eino/components/prompt"
	"github.com/cloudwego/eino/schema"
)

var (
	htmlPrompt      string
	multiFilePrompt string
	promptOnce      sync.Once
)

func loadPromptFile(fileName string) (string, error) {
	projectRoot, err := config.GetProjectRootPath()
	if err != nil {
		return "", err
	}
	filePath := filepath.Join(projectRoot, "prompt", fileName)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func LoadPrompts() error {
	var err error
	promptOnce.Do(func() {
		htmlPrompt, err = loadPromptFile("codegen-html-system-prompt.txt")
		if err != nil {
			panic(err)
		}

		multiFilePrompt, err = loadPromptFile("codegen-multi-file-system-prompt.txt")
		if err != nil {
			panic(err)
		}

		if err != nil {
			panic(err)
		}
	})
	return err
}

func GetHtmlPrompt() string {
	return htmlPrompt
}

func GetMultiFilePrompt() string {
	return multiFilePrompt
}

func NewMultiFileChatTemplate() (prompt.ChatTemplate, error) {
	LoadPrompts()
	return newChatTemplate(GetMultiFilePrompt()), nil
}

func NewHtmlChatTemplate() (prompt.ChatTemplate, error) {
	LoadPrompts()
	return newChatTemplate(GetHtmlPrompt()), nil
}

func newChatTemplate(systemPrompt string) prompt.ChatTemplate {
	ctp := prompt.FromMessages(schema.GoTemplate, []schema.MessagesTemplate{
		schema.SystemMessage(systemPrompt),
		schema.MessagesPlaceholder("history", true),
		schema.UserMessage("{{.content}}"),
	}...)
	return ctp
}
