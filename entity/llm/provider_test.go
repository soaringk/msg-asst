package llm

import (
	"testing"
)

func TestOpenAICapabilities(t *testing.T) {
	caps := openAICapabilities

	if !caps.SupportsImage {
		t.Error("OpenAI should support images")
	}
	if caps.SupportsVideo {
		t.Error("OpenAI should NOT support video via OpenAI protocol")
	}
	if !caps.SupportsAudio {
		t.Error("OpenAI should support audio")
	}
	if caps.SupportsPDF {
		t.Error("OpenAI should NOT support PDF via OpenAI protocol")
	}
}

func TestGeminiCapabilities(t *testing.T) {
	caps := geminiCapabilities

	if !caps.SupportsImage {
		t.Error("Gemini should support images")
	}
	if !caps.SupportsVideo {
		t.Error("Gemini native SDK should support video")
	}
	if !caps.SupportsAudio {
		t.Error("Gemini should support audio")
	}
	if !caps.SupportsPDF {
		t.Error("Gemini should support PDF")
	}
}

func TestGetAudioFormat(t *testing.T) {
	tests := []struct {
		mimeType string
		expected string
	}{
		{"audio/wav", "wav"},
		{"audio/mpeg", "mp3"},
		{"audio/mp3", "mp3"},
		{"audio/amr", "wav"},
		{"unknown", "wav"},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			result := getAudioFormat(tt.mimeType)
			if result != tt.expected {
				t.Errorf("getAudioFormat(%q) = %q, want %q", tt.mimeType, result, tt.expected)
			}
		})
	}
}
