package chat

import (
	"time"
)

type Message struct {
	ID         string
	Timestamp  time.Time
	Sender     string
	GroupTopic string
	Content    *Content
}
