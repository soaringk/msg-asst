package llm

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/soaringk/msg-asst/entity/chat"
	"github.com/soaringk/msg-asst/entity/config"
)

// MockProvider implements Provider for testing
type MockProvider struct {
	LastSystemPrompt string
	LastContents     []*chat.Content
	MockResponse     string
	MockError        error
}

func (m *MockProvider) GenerateContent(ctx context.Context, systemPrompt string, contents []*chat.Content) (string, error) {
	m.LastSystemPrompt = systemPrompt
	m.LastContents = contents
	return m.MockResponse, m.MockError
}

func TestGenerateSummaryPromptFormatting(t *testing.T) {
	// Setup env
	os.Setenv("LLM_API_KEY", "test-key")
	os.Setenv("SYSTEM_PROMPT_FILE", "test_prompt.txt")
	defer func() {
		os.Unsetenv("LLM_API_KEY")
		os.Unsetenv("SYSTEM_PROMPT_FILE")
		os.Remove("test_prompt.txt")
	}()

	// Write dummy prompt file
	err := os.WriteFile("test_prompt.txt", []byte("You are a bot"), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Load config
	if err := config.Parse(); err != nil {
		t.Fatal(err)
	}

	// Initialize Service - this will create a real provider based on config but we'll swap it
	svc := New()
	defer svc.Close()

	// Inject Mock Provider
	mockProvider := &MockProvider{
		MockResponse: "Summary Result",
	}
	var p Provider = mockProvider
	svc.provider.Store(&p)

	// Prepare input
	messages := []*chat.Content{
		{Type: chat.ContentTypeText, Text: "User: Hello"},
	}
	group := "Test Group"
	timeRange := "12:00 - 12:05"
	count := 5

	// Execute
	_, err = svc.GenerateSummary(context.Background(), group, timeRange, count, messages)
	if err != nil {
		t.Fatalf("GenerateSummary failed: %v", err)
	}

	// Verify System Prompt
	if mockProvider.LastSystemPrompt != "You are a bot" {
		t.Errorf("Expected system prompt 'You are a bot', got %q", mockProvider.LastSystemPrompt)
	}

	// Verify Content Structure: Preamble + Messages + Closing
	contents := mockProvider.LastContents
	if len(contents) != 3 {
		t.Fatalf("Expected 3 content parts (preamble, msg, closing), got %d", len(contents))
	}

	// Check Preamble
	preamble := contents[0].Text
	if !strings.Contains(preamble, "Test Group") {
		t.Error("Preamble missing group name")
	}
	if !strings.Contains(preamble, "12:00 - 12:05") {
		t.Error("Preamble missing time range")
	}
	if !strings.Contains(preamble, "<messages>") {
		t.Error("Preamble missing opening tag")
	}

	// Check Message
	if contents[1].Text != "User: Hello" {
		t.Error("Reflected message content mismatch")
	}

	// Check Closing
	if contents[2].Text != "\n</messages>" {
		t.Errorf("Expected closing tag, got %q", contents[2].Text)
	}
}
