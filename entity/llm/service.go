package llm

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
	"github.com/soaringk/wechat-meeting-scribe/entity/config"
)

type Service struct {
	client       atomic.Pointer[openai.Client]
	model        atomic.Pointer[shared.ChatModel]
	systemPrompt atomic.Value
	watcher      *fsnotify.Watcher
	stopWatcher  chan struct{}
}

type SummaryRequest struct {
	RoomTopic    string
	TimeRange    string
	Messages     []string
}

func (s *Service) loadSystemPrompt() error {
	cfg := config.GetConfig()
	systemPromptBytes, err := os.ReadFile(cfg.SystemPromptFile)
	if err != nil {
		return fmt.Errorf("failed to read system prompt: %w", err)
	}

	prompt := strings.TrimSpace(string(systemPromptBytes))
	s.systemPrompt.Store(prompt)

	log.Printf("[LLM] System prompt loaded (%d chars)", len(prompt))
	return nil
}

func (s *Service) getSystemPrompt() string {
	return s.systemPrompt.Load().(string)
}

func (s *Service) createClient() {
	cfg := config.GetConfig()
	client := openai.NewClient(
		option.WithAPIKey(cfg.LLMAPIKey),
		option.WithBaseURL(cfg.LLMBaseURL),
	)
	s.client.Store(&client)

	model := shared.ChatModel(cfg.LLMModel)
	s.model.Store(&model)

	log.Printf("[LLM] Client created with model: %s, base URL: %s", cfg.LLMModel, cfg.LLMBaseURL)
}

func New() *Service {
	s := &Service{
		stopWatcher: make(chan struct{}),
	}

	s.createClient()

	if err := s.loadSystemPrompt(); err != nil {
		log.Fatalf("[LLM] Failed to load initial system prompt: %v", err)
	}

	config.OnConfigChange(func() {
		log.Println("[LLM] Config changed, recreating client...")
		s.createClient()
	})

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("[LLM] Failed to create file watcher: %v", err)
	}
	s.watcher = watcher

	cfg := config.GetConfig()
	if err := watcher.Add(cfg.SystemPromptFile); err != nil {
		watcher.Close()
		log.Fatalf("[LLM] Failed to watch system prompt file: %v", err)
	}

	go func() {
		defer watcher.Close()
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					log.Println("[LLM] File watcher events channel closed")
					return
				}
				if event.Has(fsnotify.Write) {
					log.Printf("[LLM] System prompt file changed, reloading...")
					if err := s.loadSystemPrompt(); err != nil {
						log.Printf("[LLM] Error reloading system prompt: %v", err)
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					log.Println("[LLM] File watcher errors channel closed")
					return
				}
				log.Printf("[LLM] File watcher error: %v", err)
			case <-s.stopWatcher:
				log.Println("[LLM] File watcher stopped")
				return
			}
		}
	}()

	log.Printf("[LLM] File watcher started for: %s", cfg.SystemPromptFile)
	return s
}

func (s *Service) Close() {
	close(s.stopWatcher)
	if s.watcher != nil {
		s.watcher.Close()
	}
}

func (s *Service) GenerateSummary(ctx context.Context, roomTopic, timeRange string, messageCount int, messages []string) (string, error) {
	systemPrompt := s.getSystemPrompt()
	client := s.client.Load()
	model := s.model.Load()

	req := SummaryRequest{
		RoomTopic: roomTopic,
		TimeRange: timeRange,
		Messages:  messages,
	}
	userPrompt := s.buildUserPrompt(req)

	log.Printf("[LLM] Sending request to %s...", *model)

	resp, err := client.Chat.Completions.New(
		ctx,
		openai.ChatCompletionNewParams{
			Model: *model,
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.SystemMessage(systemPrompt),
				openai.UserMessage(userPrompt),
			},
		},
	)

	if err != nil {
		log.Printf("[LLM] Error: %v", err)
		return "", fmt.Errorf("LLM service error: %w", err)
	}

	if len(resp.Choices) == 0 {
		log.Println("[LLM] No content in response")
		return "", fmt.Errorf("no response from LLM")
	}

	content := resp.Choices[0].Message.Content
	log.Printf("[LLM] Response received (%d chars)", len(content))

	return content, nil
}

func (s *Service) buildUserPrompt(req SummaryRequest) string {
	conversationText := strings.Join(req.Messages, "\n")
	return fmt.Sprintf(
		"群聊名称：%s\n消息时间范围：%s\n消息数量：%d\n\n请基于以下消息生成纪要，只输出结果本身：\n<messages>\n%s\n</messages>",
		req.RoomTopic,
		req.TimeRange,
		len(req.Messages),
		conversationText,
	)
}
