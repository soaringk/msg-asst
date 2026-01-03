package llm

import (
	"context"
	"encoding/base64"
	"fmt"
	"sync/atomic"

	openai "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/shared"
	"github.com/soaringk/wechat-meeting-scribe/entity/chat"
	"github.com/soaringk/wechat-meeting-scribe/pkg/logging"
	"go.uber.org/zap"
)

type OpenAIProvider struct {
	client atomic.Pointer[openai.Client]
	model  string
	log    *zap.Logger
}

type OpenAIConfig struct {
	APIKey  string
	BaseURL string
	Model   string
}

func NewOpenAIProvider(cfg OpenAIConfig) *OpenAIProvider {
	p := &OpenAIProvider{
		model: cfg.Model,
		log:   logging.Named("openai"),
	}

	client := openai.NewClient(
		option.WithAPIKey(cfg.APIKey),
		option.WithBaseURL(cfg.BaseURL),
	)
	p.client.Store(&client)

	p.log.Info("OpenAI provider initialized",
		zap.String("model", cfg.Model),
		zap.String("baseURL", cfg.BaseURL))

	return p
}

func (p *OpenAIProvider) GenerateContent(ctx context.Context, systemPrompt string, contents []*chat.Content) (string, error) {
	client := p.client.Load()
	model := shared.ChatModel(p.model)

	parts := p.buildContentParts(contents)

	p.log.Debug("Sending request to OpenAI",
		zap.String("model", p.model),
		zap.Int("contentParts", len(parts)))

	resp, err := client.Chat.Completions.New(
		ctx,
		openai.ChatCompletionNewParams{
			Model: model,
			Messages: []openai.ChatCompletionMessageParamUnion{
				openai.SystemMessage(systemPrompt),
				openai.UserMessage(parts),
			},
		},
	)

	if err != nil {
		p.log.Error("OpenAI API error", zap.Error(err))
		return "", fmt.Errorf("OpenAI API error: %w", err)
	}

	if len(resp.Choices) == 0 {
		p.log.Warn("No response choices from OpenAI")
		return "", fmt.Errorf("no response from OpenAI")
	}

	result := resp.Choices[0].Message.Content
	p.log.Debug("Response received", zap.Int("length", len(result)))

	return result, nil
}

func (p *OpenAIProvider) buildContentParts(contents []*chat.Content) []openai.ChatCompletionContentPartUnionParam {
	var parts []openai.ChatCompletionContentPartUnionParam

	for _, c := range contents {
		switch c.Type {
		case chat.ContentTypeText:
			parts = append(parts, openai.TextContentPart(c.Text))

		case chat.ContentTypeImage:
			if len(c.Data) > 0 {
				dataURL := fmt.Sprintf("data:%s;base64,%s", c.MimeType, base64.StdEncoding.EncodeToString(c.Data))
				parts = append(parts, openai.ImageContentPart(openai.ChatCompletionContentPartImageImageURLParam{
					URL: dataURL,
				}))
				p.log.Debug("Added image part", zap.Int("size", len(c.Data)))
			} else {
				parts = append(parts, openai.TextContentPart(c.Description()))
			}

		case chat.ContentTypeAudio:
			if len(c.Data) > 0 {
				format := getAudioFormat(c.MimeType)
				base64Data := base64.StdEncoding.EncodeToString(c.Data)
				parts = append(parts, openai.InputAudioContentPart(openai.ChatCompletionContentPartInputAudioInputAudioParam{
					Data:   base64Data,
					Format: format,
				}))
				p.log.Debug("Added audio part", zap.Int("size", len(c.Data)), zap.String("format", format))
			} else {
				parts = append(parts, openai.TextContentPart(c.Description()))
			}

		case chat.ContentTypeVideo:
			parts = append(parts, openai.TextContentPart(c.Description()))
			p.log.Debug("Video not supported in OpenAI protocol, using placeholder")

		case chat.ContentTypePDF, chat.ContentTypeFile:
			parts = append(parts, openai.TextContentPart(c.Description()))
			p.log.Debug("File not supported in OpenAI protocol, using placeholder", zap.String("type", string(c.Type)))
		}
	}

	return parts
}

func getAudioFormat(mimeType string) string {
	switch mimeType {
	case "audio/wav":
		return "wav"
	case "audio/mpeg", "audio/mp3":
		return "mp3"
	default:
		return "wav"
	}
}
