package chat

import (
	"fmt"
	"time"
)

type Message struct {
	ID         string
	Timestamp  time.Time
	Sender     string
	GroupTopic string
	Content    *Content
}

func (m Message) ToContentParts() []*Content {
	// If content is text, merge it with header for better LLM context
	if m.Content != nil && m.Content.Type == ContentTypeText {
		text := fmt.Sprintf("[%s] %s: %s", m.Timestamp.Format("15:04"), m.Sender, m.Content.Text)
		return []*Content{{
			Type: ContentTypeText,
			Text: text,
		}}
	}

	// For media, we must keep header separate to attribute the media to the sender
	header := fmt.Sprintf("[%s] %s:", m.Timestamp.Format("15:04"), m.Sender)
	parts := []*Content{{
		Type: ContentTypeText,
		Text: header,
	}}
	if m.Content != nil {
		parts = append(parts, m.Content)
	}
	return parts
}
