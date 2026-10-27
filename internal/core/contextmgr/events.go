package contextmgr

import "github.com/cloudwego/eino/schema"

// Event metadata must bypass code parsers and conversation persistence.
func EventMessage(kind string) *schema.Message {
	return &schema.Message{Extra: map[string]any{"context_event": kind}}
}

func EventKind(msg *schema.Message) string {
	if msg != nil {
		kind, _ := msg.Extra["context_event"].(string)
		if kind == "context_compacting" || kind == "context_ready" {
			return kind
		}
	}
	return ""
}
