package enum

type CodeGenTypeEnum string

const (
	HtmlCodeGen  CodeGenTypeEnum = "html"
	MultiFileGen CodeGenTypeEnum = "multi_file"
	VueCodeGen   CodeGenTypeEnum = "vue_project"
)

var CodeGenTypeTextMap = map[CodeGenTypeEnum]string{
	HtmlCodeGen:  "SingleFileGen",
	MultiFileGen: "MultiFileGen",
	VueCodeGen:   "VueCodeGen",
}
