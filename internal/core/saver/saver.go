package saver

import (
	"fmt"
	"os"
	"path/filepath"

	aimodel "github.com/chosenlau/noCodeAI/internal/ai/aimodel"
	"github.com/chosenlau/noCodeAI/pkg/enum"
	"github.com/chosenlau/noCodeAI/pkg/myfile"
)

type CodeSaver struct {
	baseDir string
}

// NewCodeSaver 初始化保存器实例，整个应用生命周期内只需要 New 一次
func NewCodeSaver() (*CodeSaver, error) {
	genPath, err := myfile.GetCodeOutputRoot()
	if err != nil {
		return nil, fmt.Errorf("获取存储根目录失败: %w", err)
	}
	return &CodeSaver{baseDir: genPath}, nil
}

func (s *CodeSaver) GetDirPath(codeType enum.CodeGenTypeEnum, appId int64) (string, error) {
	return s.buildDir(codeType, appId)
}

func (s *CodeSaver) SavedDirPath(codeType enum.CodeGenTypeEnum, appId int64) (string, error) {
	if appId <= 0 {
		return "", fmt.Errorf("invalid app ID: %d", appId)
	}
	return filepath.Join(s.baseDir, fmt.Sprintf("%s_%d", codeType, appId)), nil
}

// SaveHtml 保存单文件 (HTML)
func (s *CodeSaver) SaveHtml(appId int64, response *aimodel.HtmlCodeResponse) (string, error) {
	// 1. 强类型输入，直接校验
	if response == nil || response.HtmlCode == "" {
		return "", fmt.Errorf("HTML 代码结果为空")
	}

	// 2. 构建目录
	dirPath, err := s.buildDir(enum.HtmlCodeGen, appId)
	if err != nil {
		return "", err
	}

	// 3. 写入文件
	if err := s.writeToFile(dirPath, "index.html", response.HtmlCode); err != nil {
		return "", err
	}
	if err := s.writeToFile(dirPath, "description.md", response.Description); err != nil {
		return "", err
	}
	return dirPath, nil
}

// SaveMultiFile 保存多文件 (HTML/CSS/JS)
func (s *CodeSaver) SaveMultiFile(appId int64, response *aimodel.MultiFileCodeResponse) (string, error) {
	// 1. 强类型输入，直接校验
	if response == nil || response.HtmlCode == "" || response.CssCode == "" || response.JsCode == "" {
		return "", fmt.Errorf("多文件代码生成结果存在空值")
	}

	// 2. 构建目录
	dirPath, err := s.buildDir(enum.MultiFileGen, appId)
	if err != nil {
		return "", err
	}

	// 3. 依次写入文件
	if err := s.writeToFile(dirPath, "index.html", response.HtmlCode); err != nil {
		return "", err
	}
	if err := s.writeToFile(dirPath, "style.css", response.CssCode); err != nil {
		return "", err
	}
	if err := s.writeToFile(dirPath, "script.js", response.JsCode); err != nil {
		return "", err
	}
	if err := s.writeToFile(dirPath, "description.md", response.Description); err != nil {
		return "", err
	}

	return dirPath, nil
}

func (s *CodeSaver) buildDir(codeType enum.CodeGenTypeEnum, appId int64) (string, error) {
	if appId <= 0 {
		return "", fmt.Errorf("无效的应用ID: %d", appId)
	}

	uniqueDirName := fmt.Sprintf("%s_%d", codeType, appId)
	dirPath := filepath.Join(s.baseDir, uniqueDirName)

	if err := os.MkdirAll(dirPath, os.ModePerm); err != nil {
		return "", fmt.Errorf("创建目录失败: %w", err)
	}
	return dirPath, nil
}

// writeToFile [内部私有方法] 将内容写入文件并保存
func (s *CodeSaver) writeToFile(dirPath, fileName, content string) error {
	filePath := filepath.Join(dirPath, fileName)
	return os.WriteFile(filePath, []byte(content), os.ModePerm)
}
