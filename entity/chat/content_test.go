package chat

import (
	"testing"
)

func TestContentDescription(t *testing.T) {
	tests := []struct {
		name     string
		content  Content
		expected string
	}{
		{
			name:     "text content",
			content:  Content{Type: ContentTypeText, Text: "Hello"},
			expected: "Hello",
		},
		{
			name:     "image content",
			content:  Content{Type: ContentTypeImage, Data: []byte{1, 2, 3}},
			expected: "[图片]",
		},
		{
			name:     "video content",
			content:  Content{Type: ContentTypeVideo, Data: []byte{1, 2, 3}},
			expected: "[视频]",
		},
		{
			name:     "audio content",
			content:  Content{Type: ContentTypeAudio, Data: []byte{1, 2, 3}},
			expected: "[语音]",
		},
		{
			name:     "pdf content",
			content:  Content{Type: ContentTypePDF, FileName: "doc.pdf"},
			expected: "[文件: doc.pdf]",
		},
		{
			name:     "file content",
			content:  Content{Type: ContentTypeFile, FileName: "data.xlsx"},
			expected: "[文件: data.xlsx]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.content.Description(); got != tt.expected {
				t.Errorf("Description() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsMedia(t *testing.T) {
	tests := []struct {
		name     string
		content  Content
		expected bool
	}{
		{
			name:     "text is not media",
			content:  Content{Type: ContentTypeText, Text: "Hello"},
			expected: false,
		},
		{
			name:     "image with data is media",
			content:  Content{Type: ContentTypeImage, Data: []byte{1, 2, 3}},
			expected: true,
		},
		{
			name:     "image without data is not media",
			content:  Content{Type: ContentTypeImage, Text: "[图片]"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.content.IsMedia(); got != tt.expected {
				t.Errorf("IsMedia() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDetectMimeType(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		contentType ContentType
		expected    string
	}{
		{
			name:        "jpeg magic bytes",
			data:        []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01},
			contentType: ContentTypeImage,
			expected:    "image/jpeg",
		},
		{
			name:        "png magic bytes",
			data:        []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00, 0x00, 0x00, 0x0D},
			contentType: ContentTypeImage,
			expected:    "image/png",
		},
		{
			name:        "gif magic bytes",
			data:        []byte("GIF89a......"),
			contentType: ContentTypeImage,
			expected:    "image/gif",
		},
		{
			name:        "pdf magic bytes",
			data:        []byte("%PDF-1.4...."),
			contentType: ContentTypePDF,
			expected:    "application/pdf",
		},
		{
			name:        "amr magic bytes",
			data:        []byte("#!AMR\n....."),
			contentType: ContentTypeAudio,
			expected:    "audio/amr",
		},
		{
			name:        "unknown defaults to content type",
			data:        []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xAA, 0xBB},
			contentType: ContentTypeVideo,
			expected:    "video/mp4",
		},
		{
			name:        "short data uses default",
			data:        []byte{0x00},
			contentType: ContentTypeImage,
			expected:    "image/jpeg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := detectMimeType(tt.data, tt.contentType); got != tt.expected {
				t.Errorf("detectMimeType() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGetMimeTypeFromExt(t *testing.T) {
	tests := []struct {
		ext      string
		expected string
	}{
		{"jpg", "image/jpeg"},
		{"jpeg", "image/jpeg"},
		{"PNG", "image/png"},
		{"pdf", "application/pdf"},
		{"mp4", "video/mp4"},
		{"amr", "audio/amr"},
		{"wav", "audio/wav"},
		{"unknown", "application/octet-stream"},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			if got := getMimeTypeFromExt(tt.ext); got != tt.expected {
				t.Errorf("getMimeTypeFromExt(%q) = %v, want %v", tt.ext, got, tt.expected)
			}
		})
	}
}
