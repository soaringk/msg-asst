# 🤖 WeChat Meeting Scribe

**Your AI Meeting Secretary for WeChat Groups**

A real-time WeChat bot that automatically tracks and summarizes group discussions, generating structured meeting minutes using AI. Supports multimodal content including text, images, audio, and PDF files.

## ✨ Features

- **Multimodal Support**: Understands text, images, voice messages, and PDF files
- **Flexible AI Backend**: Supports Google Gemini (native) and OpenAI-compatible providers
- **Smart Summarization**: Uses LLM to generate structured meeting minutes
- **Multiple Triggers**: Supports time-based, volume-based, and keyword triggers
- **Hot Reload**: Update configuration and target groups without restarting

## 📋 Summary Format

The bot generates meeting minutes with the following structure:

- **📋 Key Discussion Points**: Main topics and viewpoints
- **✅ Decisions Made**: Consensus reached during the discussion
- **📌 Action Items**: Follow-up tasks (with assignees if mentioned)
- **👥 Main Participants**: Active speakers
- **💡 Other Notes**: Additional important information

## 🚀 Quick Start

### Prerequisites

- Go 1.22+
- WeChat account
- LLM API access (Gemini or OpenAI)

### Installation

1. **Clone the repository**

```bash
git clone https://github.com/yourusername/msg-asst.git
cd msg-asst
```

2. **Configure environment variables**

Copy `.env.example` to `.env`:

```bash
cp .env.example .env
```

Edit `.env` with your settings:

```env
# AI Provider: gemini or openai
LLM_PROVIDER=gemini

# Gemini Configuration (Recommended)
LLM_BASE_URL=https://generativelanguage.googleapis.com
LLM_API_KEY=your_gemini_api_key_here
LLM_MODEL=gemini-2.5-flash

# OpenAI Configuration (alternative)
# LLM_PROVIDER=openai
# LLM_BASE_URL=https://api.openai.com/v1
# LLM_API_KEY=your_openai_api_key_here
# LLM_MODEL=gpt-4o

# Summarization Triggers
SUMMARY_INTERVAL_MINUTES=30
SUMMARY_MESSAGE_COUNT=50
SUMMARY_KEYWORD=@bot 总结
MIN_MESSAGES_FOR_SUMMARY=5

# Message buffer settings
MAX_BUFFER_SIZE=200

# Media Support (sizes: 10K, 10M, 10G)
MEDIA_IMAGE_ENABLED=true
MEDIA_VIDEO_ENABLED=true
MEDIA_AUDIO_ENABLED=true
MEDIA_PDF_ENABLED=true
MEDIA_MAX_IMAGE_SIZE=10M
MEDIA_MAX_VIDEO_SIZE=20M
```

3. **Run the bot**

```bash
go run main.go
```

### First Time Setup

1. Run the bot with `go run main.go -select-groups` to select groups to monitor.
2. Scan the QR code with WeChat (the URL will be printed in the console).
3. Confirm login on your phone.
4. Select the groups you want the bot to track from the list.
5. The bot will start monitoring selected groups. The selection is saved to `groups.json`.

## 🏗️ Project Structure

The project follows a clean architecture:

```
msg-asst/
├── entity/
│   ├── chat/           # Core chat entities (Message, Buffer, Content)
│   ├── config/         # Configuration logic
│   └── llm/            # LLM interfaces and provider implementations
├── logic/
│   ├── bot/            # Bot business logic
│   └── summary/        # Summary generation orchestration
├── pkg/
│   └── logging/        # Structured logging (zap)
├── main.go             # Application entry point
├── groups.json          # Target groups storage (auto-generated)
└── system_prompt.txt   # Customizable system prompt for LLM
```

## ⚙️ Configuration Reference

### Provider Selection
- **`gemini`**: Uses Google's GenAI SDK (default, supports native video/pdf).
- **`openai`**: Uses OpenAI-compatible API.

### Multimodal Capabilities
- **Images**: Analyzed for context in discussions.
- **Audio**: Transcribed and included in summaries.
- **PDF**: Parsed for content (Gemini only).
- **Video**: Video content understanding (Gemini only).

## 🛠️ Customization

### Modify System Prompt
Edit `system_prompt.txt` to change how the bot summarizes meetings. This file is hot-reloaded, so you can tweak it while the bot is running.

### Hot Reload Support

The following can be changed without restarting the bot:

| File/Setting | Hot Reload |
|--------------|------------|
| `.env` (most settings) | ✅ Yes |
| `groups.json` | ✅ Yes |
| `system_prompt.txt` | ✅ Yes |
| LLM Provider/Model/API Key | ✅ Yes |
| Summary triggers (keyword, count) | ✅ Yes |
| Media support settings | ✅ Yes |
| `SUMMARY_INTERVAL_MINUTES` | ❌ No (timer set at startup) |
| `MAX_BUFFER_SIZE` | ❌ No (affects new groups only) |

## 🐛 Troubleshooting

- **Login Issues**: If QR code scan fails, check your network. The bot behaves like a Desktop WeChat client.
- **No Summary**: Ensure you have selected groups using `-select-groups`. Check `MIN_MESSAGES_FOR_SUMMARY`.
- **Logs**: Check console output. We use structured logging, so you can filter for errors easily.

## 🤝 Contributing

We welcome contributions! Please follow these steps:

1. Fork the repository.
2. Create a feature branch.
3. Commit your changes.
4. Run tests: `go test ./...`
5. Submit a pull request.

## 📄 License

MIT

## 🙏 Acknowledgments

- [openwechat](https://github.com/eatmoreapple/openwechat)
- [google-genai-sdk](https://github.com/google/generative-ai-go)
- [zap](https://github.com/uber-go/zap)
