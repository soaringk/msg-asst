package llm

import (
	"context"

	"github.com/soaringk/wechat-meeting-scribe/entity/chat"
)

type Provider interface {
	GenerateContent(ctx context.Context, systemPrompt string, contents []*chat.Content) (string, error)
}
