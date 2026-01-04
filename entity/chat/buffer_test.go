package chat

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/soaringk/msg-asst/entity/config"
)

func TestMain(m *testing.M) {
	os.Setenv("LLM_API_KEY", "test-key")
	_ = config.Parse()
	os.Exit(m.Run())
}

func TestSnapshotWithMedia(t *testing.T) {
	buf := New()

	buf.Add(Message{
		ID:         "msg1",
		Timestamp:  time.Now(),
		Sender:     "Alice",
		GroupTopic: "TestGroup",
		Content:    &Content{Type: ContentTypeText, Text: "Hello world"},
	})

	buf.Add(Message{
		ID:         "msg2",
		Timestamp:  time.Now(),
		Sender:     "Bob",
		GroupTopic: "TestGroup",
		Content:    &Content{Type: ContentTypeImage, Data: []byte{1, 2, 3}, MimeType: "image/jpeg"},
	})

	snapshot := buf.GetSnapshot("TestGroup")

	if snapshot.Count != 2 {
		t.Errorf("Expected count 2, got %d", snapshot.Count)
	}

	if len(snapshot.Contents) != 3 {
		t.Errorf("Expected 3 content parts (1 text merged, 1 header + 1 media), got %d", len(snapshot.Contents))
	}

	foundImage := false
	for _, c := range snapshot.Contents {
		if c.Type == ContentTypeImage {
			foundImage = true
			break
		}
	}
	if !foundImage {
		t.Error("Expected to find image content in snapshot")
	}
}

func TestSnapshotWithoutMedia(t *testing.T) {
	buf := New()

	buf.Add(Message{
		ID:         "msg1",
		Timestamp:  time.Now(),
		Sender:     "Alice",
		GroupTopic: "TestGroup",
		Content:    &Content{Type: ContentTypeText, Text: "Text only message"},
	})

	snapshot := buf.GetSnapshot("TestGroup")

	if snapshot.Count != 1 {
		t.Errorf("Expected count 1, got %d", snapshot.Count)
	}

	if len(snapshot.Contents) != 1 {
		t.Errorf("Expected 1 content (merged header + text), got %d", len(snapshot.Contents))
	}
}

func TestBufferRotation(t *testing.T) {
	// Override max buffer size for testing
	os.Setenv("MAX_BUFFER_SIZE", "3")
	defer os.Unsetenv("MAX_BUFFER_SIZE")
	_ = config.Parse()

	buf := New()
	group := "RotationGroup"

	// Add 3 messages (full capacity)
	for i := 1; i <= 3; i++ {
		buf.Add(Message{
			ID:         fmt.Sprintf("msg%d", i),
			Timestamp:  time.Now(),
			Sender:     "Sender",
			GroupTopic: group,
			Content:    &Content{Type: ContentTypeText, Text: fmt.Sprintf("Message %d", i)},
		})
	}

	snapshot := buf.GetSnapshot(group)
	if snapshot.Count != 3 {
		t.Errorf("Expected count 3, got %d", snapshot.Count)
	}
	if snapshot.Contents[0].Text != "[00:00] Sender: Message 1" { // Simplistic check, format depends on time
		// Ideally checking ID or content, but Snapshot returns *Content.
		// Content doesn't have ID. But we can check text.
	}

	// Add 4th message, should overwrite msg1
	buf.Add(Message{
		ID:         "msg4",
		Timestamp:  time.Now(),
		Sender:     "Sender",
		GroupTopic: group,
		Content:    &Content{Type: ContentTypeText, Text: "Message 4"},
	})

	snapshot = buf.GetSnapshot(group)
	if snapshot.Count != 3 {
		t.Errorf("Expected count 3 after rotation, got %d", snapshot.Count)
	}

	// First message in snapshot should now be msg2
	firstContent := snapshot.Contents[0].Text
	if !strings.Contains(firstContent, "Message 2") {
		t.Errorf("Expected first message to be 'Message 2', got %q", firstContent)
	}

	// Last message should be msg4
	lastContent := snapshot.Contents[len(snapshot.Contents)-1].Text
	if !strings.Contains(lastContent, "Message 4") {
		t.Errorf("Expected last message to be 'Message 4', got %q", lastContent)
	}
}

func TestShouldSummarize(t *testing.T) {
	os.Setenv("SUMMARY_MESSAGE_COUNT", "5")
	os.Setenv("MIN_MESSAGES_FOR_SUMMARY", "2")
	os.Setenv("SUMMARY_KEYWORD", "@bot summary")
	defer func() {
		os.Unsetenv("SUMMARY_MESSAGE_COUNT")
		os.Unsetenv("MIN_MESSAGES_FOR_SUMMARY")
		os.Unsetenv("SUMMARY_KEYWORD")
	}()
	_ = config.Parse()

	buf := New()
	group := "TriggerGroup"

	// 1. Not enough messages
	buf.Add(Message{
		ID:         "msg1",
		Timestamp:  time.Now(),
		Sender:     "Alice",
		GroupTopic: group,
		Content:    &Content{Type: ContentTypeText, Text: "Hi"},
	})

	if buf.ShouldSummarize(group, false) {
		t.Error("Should not summarize with 1 message (min 2)")
	}

	// 2. Keyword trigger with enough messages
	buf.Add(Message{
		ID:         "msg2",
		Timestamp:  time.Now(),
		Sender:     "Bob",
		GroupTopic: group,
		Content:    &Content{Type: ContentTypeText, Text: "@bot summary"},
	})

	if !buf.ShouldSummarize(group, true) {
		t.Error("Should summarize when triggered by keyword and min messages met")
	}

	// 3. Message count trigger
	for i := 3; i <= 5; i++ {
		buf.Add(Message{
			ID:         fmt.Sprintf("msg%d", i),
			Timestamp:  time.Now(),
			Sender:     "User",
			GroupTopic: group,
			Content:    &Content{Type: ContentTypeText, Text: "msg"},
		})
	}
	// Total 5 messages now

	if !buf.ShouldSummarize(group, false) {
		t.Error("Should summarize when message count reaches limit (5)")
	}
}
