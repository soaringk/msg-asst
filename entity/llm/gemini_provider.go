package llm

import (
	"context"
	"fmt"

	"github.com/soaringk/msg-asst/entity/chat"
	"github.com/soaringk/msg-asst/pkg/logging"
	"go.uber.org/zap"
	"google.golang.org/genai"
)

type GeminiProvider struct {
	client *genai.Client
	model  string
	log    *zap.Logger
}

type GeminiConfig struct {
	APIKey string
	Model  string
}

func NewGeminiProvider(ctx context.Context, cfg GeminiConfig) (*GeminiProvider, error) {
	log := logging.Named("gemini")

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.APIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	log.Info("Gemini provider initialized", zap.String("model", cfg.Model))

	return &GeminiProvider{
		client: client,
		model:  cfg.Model,
		log:    log,
	}, nil
}

func (p *GeminiProvider) GenerateContent(ctx context.Context, systemPrompt string, contents []*chat.Content) (string, error) {
	parts := p.buildParts(contents)

	userContent := &genai.Content{
		Role:  genai.RoleUser,
		Parts: parts,
	}

	p.log.Debug("Sending request to Gemini",
		zap.String("model", p.model),
		zap.Int("parts", len(parts)))

	result, err := p.client.Models.GenerateContent(
		ctx,
		p.model,
		[]*genai.Content{userContent},
		&genai.GenerateContentConfig{
			SystemInstruction: &genai.Content{
				Role:  genai.RoleUser,
				Parts: []*genai.Part{{Text: systemPrompt}},
			},
		},
	)

	if err != nil {
		p.log.Error("Gemini API error", zap.Error(err))
		return "", fmt.Errorf("Gemini API error: %w", err)
	}

	text := result.Text()
	p.log.Debug("Response received", zap.Int("length", len(text)))

	return text, nil
}

func (p *GeminiProvider) buildParts(contents []*chat.Content) []*genai.Part {
	var parts []*genai.Part

	for _, c := range contents {
		switch c.Type {
		case chat.ContentTypeText:
			parts = append(parts, &genai.Part{Text: c.Text})

		case chat.ContentTypeImage:
			if len(c.Data) > 0 {
				parts = append(parts, &genai.Part{
					InlineData: &genai.Blob{
						MIMEType: c.MimeType,
						Data:     c.Data,
					},
				})
				p.log.Debug("Added image part", zap.Int("size", len(c.Data)))
			} else {
				parts = append(parts, &genai.Part{Text: c.Description()})
			}

		case chat.ContentTypeVideo:
			if len(c.Data) > 0 {
				parts = append(parts, &genai.Part{
					InlineData: &genai.Blob{
						MIMEType: c.MimeType,
						Data:     c.Data,
					},
				})
				p.log.Debug("Added video part", zap.Int("size", len(c.Data)))
			} else {
				parts = append(parts, &genai.Part{Text: c.Description()})
			}

		case chat.ContentTypeAudio:
			if len(c.Data) > 0 {
				parts = append(parts, &genai.Part{
					InlineData: &genai.Blob{
						MIMEType: c.MimeType,
						Data:     c.Data,
					},
				})
				p.log.Debug("Added audio part", zap.Int("size", len(c.Data)))
			} else {
				parts = append(parts, &genai.Part{Text: c.Description()})
			}

		case chat.ContentTypePDF:
			if len(c.Data) > 0 {
				parts = append(parts, &genai.Part{
					InlineData: &genai.Blob{
						MIMEType: "application/pdf",
						Data:     c.Data,
					},
				})
				p.log.Debug("Added PDF part", zap.Int("size", len(c.Data)))
			} else {
				parts = append(parts, &genai.Part{Text: c.Description()})
			}

		case chat.ContentTypeFile:
			parts = append(parts, &genai.Part{Text: c.Description()})
			p.log.Debug("Generic file using placeholder", zap.String("fileName", c.FileName))
		}
	}

	return parts
}
