package llm

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
	"github.com/soaringk/msg-asst/entity/chat"
	"github.com/soaringk/msg-asst/entity/config"
	"github.com/soaringk/msg-asst/pkg/logging"
	"go.uber.org/zap"
)

type Service struct {
	provider     atomic.Pointer[Provider]
	systemPrompt atomic.Value
	watcher      *fsnotify.Watcher
	stopWatcher  chan struct{}
}

func New() *Service {
	s := &Service{
		stopWatcher: make(chan struct{}),
	}

	if err := s.loadSystemPrompt(); err != nil {
		logging.Fatal("Failed to load initial system prompt", zap.Error(err))
	}

	s.recreateProvider()

	config.OnConfigChange(func() {
		logging.Info("Config changed, recreating LLM provider")
		s.recreateProvider()
	})

	s.startSystemPromptWatcher()

	return s
}

func (s *Service) recreateProvider() {
	cfg := config.GetConfig()
	providerType := cfg.LLMProvider

	var p Provider
	var err error

	if providerType == "gemini" {
		p, err = NewGeminiProvider(context.Background(), GeminiConfig{
			APIKey: cfg.LLMAPIKey,
			Model:  cfg.LLMModel,
		})
	} else {
		// Default to OpenAI
		p = NewOpenAIProvider(OpenAIConfig{
			APIKey:  cfg.LLMAPIKey,
			BaseURL: cfg.LLMBaseURL,
			Model:   cfg.LLMModel,
		})
	}

	if err != nil {
		logging.Error("Failed to create provider", zap.Error(err))
		return
	}

	s.provider.Store(&p)
	logging.Info("LLM provider active", zap.String("type", providerType))
}

func (s *Service) loadSystemPrompt() error {
	cfg := config.GetConfig()
	systemPromptBytes, err := os.ReadFile(cfg.SystemPromptFile)
	if err != nil {
		return fmt.Errorf("failed to read system prompt: %w", err)
	}

	prompt := strings.TrimSpace(string(systemPromptBytes))
	s.systemPrompt.Store(prompt)

	logging.Info("System prompt loaded", zap.Int("length", len(prompt)))
	return nil
}

func (s *Service) getSystemPrompt() string {
	return s.systemPrompt.Load().(string)
}

func (s *Service) GenerateSummary(ctx context.Context, groupTopic string, timeRange string, messageCount int, messages []*chat.Content) (string, error) {
	p := s.provider.Load()
	if p == nil {
		return "", fmt.Errorf("provider not initialized")
	}

	// Prepare the preamble text
	preamble := fmt.Sprintf(
		"群聊名称：%s\n消息时间范围：%s\n消息数量：%d\n\n请基于以下消息生成纪要，只输出结果本身：\n<messages>\n",
		groupTopic, timeRange, messageCount,
	)

	var requestContents []*chat.Content
	requestContents = append(requestContents, &chat.Content{
		Type: chat.ContentTypeText,
		Text: preamble,
	})

	// Append actual messages
	requestContents = append(requestContents, messages...)

	// Append closing tag
	requestContents = append(requestContents, &chat.Content{
		Type: chat.ContentTypeText,
		Text: "\n</messages>",
	})

	return (*p).GenerateContent(ctx, s.getSystemPrompt(), requestContents)
}

func (s *Service) startSystemPromptWatcher() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logging.Fatal("Failed to create file watcher", zap.Error(err))
	}
	s.watcher = watcher

	cfg := config.GetConfig()
	if err := watcher.Add(cfg.SystemPromptFile); err != nil {
		watcher.Close()
		logging.Fatal("Failed to watch system prompt file", zap.Error(err))
	}

	go func() {
		defer watcher.Close()
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) {
					logging.Info("System prompt file changed, reloading...")
					if err := s.loadSystemPrompt(); err != nil {
						logging.Error("Error reloading system prompt", zap.Error(err))
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				logging.Error("File watcher error", zap.Error(err))
			case <-s.stopWatcher:
				return
			}
		}
	}()
	logging.Debug("File watcher started", zap.String("file", cfg.SystemPromptFile))
}

func (s *Service) Close() {
	close(s.stopWatcher)
	if s.watcher != nil {
		s.watcher.Close()
	}
}
