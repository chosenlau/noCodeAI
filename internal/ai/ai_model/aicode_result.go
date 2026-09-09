package aimodel

type HtmlCodeResponse struct {
	HtmlCode    string `json:"htmlCode"`
	Description string `json:"description"`
}

type MultiFileCodeResponse struct {
	HtmlCodeResponse
	JsCode  string `json:"jsCode"`
	CssCode string `json:"cssCode"`
}
