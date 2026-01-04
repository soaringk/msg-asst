package llm

import (
	"context"

	"github.com/soaringk/msg-asst/entity/chat"
)

type Provider interface {
	GenerateContent(ctx context.Context, systemPrompt string, contents []*chat.Content) (string, error)
}
