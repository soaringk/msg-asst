package chat

import (
	"os"
	"testing"
	"time"

	"github.com/soaringk/wechat-meeting-scribe/entity/config"
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

	if len(snapshot.Contents) != 4 {
		t.Errorf("Expected 4 content parts, got %d", len(snapshot.Contents))
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

	if len(snapshot.Contents) != 2 {
		t.Errorf("Expected 2 contents (header + text), got %d", len(snapshot.Contents))
	}
}
