package chat

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/eatmoreapple/openwechat"
	"github.com/soaringk/msg-asst/entity/config"
	"github.com/soaringk/msg-asst/pkg/logging"
	"go.uber.org/zap"
)

type ContentType string

const (
	ContentTypeText  ContentType = "text"
	ContentTypeImage ContentType = "image"
	ContentTypeVideo ContentType = "video"
	ContentTypeAudio ContentType = "audio"
	ContentTypePDF   ContentType = "pdf"
	ContentTypeFile  ContentType = "file"
)

type Content struct {
	Type     ContentType
	Text     string
	Data     []byte
	MimeType string
	FileName string
}

func (c *Content) IsMedia() bool {
	return c.Type != ContentTypeText && c.Data != nil
}

func (c *Content) Description() string {
	switch c.Type {
	case ContentTypeText:
		return c.Text
	case ContentTypeImage:
		return "[图片]"
	case ContentTypeVideo:
		return "[视频]"
	case ContentTypeAudio:
		return "[语音]"
	case ContentTypePDF:
		return fmt.Sprintf("[文件: %s]", c.FileName)
	case ContentTypeFile:
		return fmt.Sprintf("[文件: %s]", c.FileName)
	default:
		return "[未知内容]"
	}
}

func ExtractFromMessage(msg *openwechat.Message) (*Content, error) {
	log := logging.Named("content")

	if msg.IsText() {
		return &Content{
			Type: ContentTypeText,
			Text: msg.Content,
		}, nil
	}

	if msg.IsPicture() {
		log.Debug("Extracting image content")
		return extractMedia(msg, ContentTypeImage, msg.GetPicture)
	}

	if msg.IsVideo() {
		log.Debug("Extracting video content")
		return extractMedia(msg, ContentTypeVideo, msg.GetVideo)
	}

	if msg.IsVoice() {
		log.Debug("Extracting voice content")
		return extractMedia(msg, ContentTypeAudio, msg.GetVoice)
	}

	if msg.IsMedia() {
		log.Debug("Extracting file/media content")
		return extractFileContent(msg)
	}

	return &Content{
		Type: ContentTypeText,
		Text: msg.Content,
	}, nil
}

type mediaGetter func() (*http.Response, error)

func extractMedia(msg *openwechat.Message, contentType ContentType, getter mediaGetter) (*Content, error) {
	log := logging.Named("content")
	cfg := config.GetConfig()

	var maxBytes int64
	switch contentType {
	case ContentTypeImage:
		maxBytes = cfg.MediaSupport.MaxImageBytes
	case ContentTypeVideo:
		maxBytes = cfg.MediaSupport.MaxVideoBytes
	case ContentTypeAudio:
		maxBytes = cfg.MediaSupport.MaxAudioBytes
	case ContentTypePDF:
		maxBytes = cfg.MediaSupport.MaxPDFBytes
	default:
		maxBytes = 100 * 1024 * 1024 // 100MB default for others
	}

	resp, err := getter()
	if err != nil {
		log.Error("Failed to get media", zap.Error(err))
		return &Content{
			Type: ContentTypeText,
			Text: fmt.Sprintf("[获取%s失败]", contentType),
		}, nil
	}
	defer resp.Body.Close()

	if resp.ContentLength > maxBytes {
		log.Warn("Media too large, skipping",
			zap.String("type", string(contentType)),
			zap.Int64("size", resp.ContentLength),
			zap.Int64("limit", maxBytes))
		return &Content{
			Type: ContentTypeText,
			Text: fmt.Sprintf("[文件过大: %s]", contentType),
		}, nil
	}

	// Read up to maxBytes + 1 to detect if it exceeds limit when ContentLength is unknown
	reader := io.LimitReader(resp.Body, maxBytes+1)
	data, err := io.ReadAll(reader)
	if err != nil {
		log.Error("Failed to read media body", zap.Error(err))
		return &Content{
			Type: ContentTypeText,
			Text: fmt.Sprintf("[读取%s失败]", contentType),
		}, nil
	}

	if int64(len(data)) > maxBytes {
		log.Warn("Media stream exceeded limit, skipping",
			zap.String("type", string(contentType)),
			zap.Int64("limit", maxBytes))
		return &Content{
			Type: ContentTypeText,
			Text: fmt.Sprintf("[文件过大: %s]", contentType),
		}, nil
	}

	mimeType := resp.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = detectMimeType(data, contentType)
	}

	log.Debug("Media extracted",
		zap.String("type", string(contentType)),
		zap.Int("size", len(data)),
		zap.String("mimeType", mimeType))

	return &Content{
		Type:     contentType,
		Data:     data,
		MimeType: mimeType,
	}, nil
}

func extractFileContent(msg *openwechat.Message) (*Content, error) {
	log := logging.Named("content")

	appData, err := msg.MediaData()
	if err != nil {
		log.Debug("Failed to get app message data, treating as generic file", zap.Error(err))
		return extractMedia(msg, ContentTypeFile, msg.GetFile)
	}

	fileName := appData.AppMsg.Title
	fileExt := strings.ToLower(appData.AppMsg.AppAttach.FileExt)

	log.Debug("App message info",
		zap.String("fileName", fileName),
		zap.String("fileExt", fileExt))

	contentType := ContentTypeFile
	if fileExt == "pdf" {
		contentType = ContentTypePDF
	}

	content, err := extractMedia(msg, contentType, msg.GetFile)
	if err != nil {
		return nil, err
	}

	content.FileName = fileName
	if content.MimeType == "" {
		content.MimeType = getMimeTypeFromExt(fileExt)
	}

	return content, nil
}

func detectMimeType(data []byte, contentType ContentType) string {
	if len(data) < 12 {
		return getDefaultMimeType(contentType)
	}

	if bytes.HasPrefix(data, []byte{0xFF, 0xD8, 0xFF}) {
		return "image/jpeg"
	}
	if bytes.HasPrefix(data, []byte{0x89, 0x50, 0x4E, 0x47}) {
		return "image/png"
	}
	if bytes.HasPrefix(data, []byte("GIF8")) {
		return "image/gif"
	}
	if bytes.HasPrefix(data, []byte("RIFF")) && bytes.Contains(data[:12], []byte("WEBP")) {
		return "image/webp"
	}

	if bytes.HasPrefix(data, []byte{0x00, 0x00, 0x00}) && len(data) > 4 && data[4] == 0x66 {
		return "video/mp4"
	}

	if bytes.HasPrefix(data, []byte("#!AMR")) {
		return "audio/amr"
	}
	if bytes.HasPrefix(data, []byte("RIFF")) && bytes.Contains(data[:12], []byte("WAVE")) {
		return "audio/wav"
	}

	if bytes.HasPrefix(data, []byte("%PDF")) {
		return "application/pdf"
	}

	return getDefaultMimeType(contentType)
}

func getDefaultMimeType(contentType ContentType) string {
	switch contentType {
	case ContentTypeImage:
		return "image/jpeg"
	case ContentTypeVideo:
		return "video/mp4"
	case ContentTypeAudio:
		return "audio/amr"
	case ContentTypePDF:
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}

func getMimeTypeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "mp4":
		return "video/mp4"
	case "mov":
		return "video/quicktime"
	case "avi":
		return "video/x-msvideo"
	case "amr":
		return "audio/amr"
	case "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "pdf":
		return "application/pdf"
	default:
		return "application/octet-stream"
	}
}
