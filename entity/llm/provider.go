package llm

import (
	"context"

	"github.com/soaringk/wechat-meeting-scribe/entity/chat"
)

type Capabilities struct {
	SupportsImage bool
	SupportsVideo bool
	SupportsAudio bool
	SupportsPDF   bool
}

type Provider interface {
	GenerateContent(ctx context.Context, systemPrompt string, contents []*chat.Content) (string, error)
}
