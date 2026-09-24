package agent

type CodeGenAgentFactory struct {
	HtmlAgent      *HtmlCodeGenAgent
	MultiFileAgent *MultiFileCodeGenAgent
	VueAgent       *VueCodeGenAgent
}

func NewCodeGenAgentFactory(
	htmlAgent *HtmlCodeGenAgent,
	multiFileAgent *MultiFileCodeGenAgent,
	vueAgent *VueCodeGenAgent,
) *CodeGenAgentFactory {
	return &CodeGenAgentFactory{
		HtmlAgent:      htmlAgent,
		MultiFileAgent: multiFileAgent,
		VueAgent:       vueAgent,
	}
}
