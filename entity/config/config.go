package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
	"github.com/joho/godotenv"
	"github.com/soaringk/wechat-meeting-scribe/pkg/logging"
	"go.uber.org/zap"
)

type MediaSupportConfig struct {
	ImageEnabled  bool
	VideoEnabled  bool
	AudioEnabled  bool
	PDFEnabled    bool
	MaxImageBytes int64
	MaxVideoBytes int64
	MaxAudioBytes int64
	MaxPDFBytes   int64
}

type SummaryTriggerConfig struct {
	IntervalMinutes       int
	MessageCount          int
	Keyword               string
	MinMessagesForSummary int
}

type Config struct {
	LLMAPIKey        string
	LLMBaseURL       string
	LLMModel         string
	LLMProvider      string // "openai" or "gemini"
	SystemPromptFile string
	BotName          string
	SummaryTrigger   SummaryTriggerConfig
	MediaSupport     MediaSupportConfig
	MaxBufferSize    int
}

var (
	configPtr       atomic.Pointer[Config]
	targetGroups    atomic.Pointer[[]string]
	configWatcher   *fsnotify.Watcher
	groupsWatcher   *fsnotify.Watcher
	callbacksMu     sync.RWMutex
	configCallbacks []func()
	stopWatchers    chan struct{}
)

const groupsFile = "groups.json"

// GetConfig returns the current config (thread-safe)
func GetConfig() *Config {
	return configPtr.Load()
}

func GetTargetGroups() []string {
	groups := targetGroups.Load()
	if groups == nil {
		return nil
	}
	return *groups
}

// OnConfigChange registers a callback to be called when config changes
func OnConfigChange(callback func()) {
	callbacksMu.Lock()
	defer callbacksMu.Unlock()
	configCallbacks = append(configCallbacks, callback)
}

func notifyConfigCallbacks() {
	callbacksMu.RLock()
	defer callbacksMu.RUnlock()
	for _, cb := range configCallbacks {
		go cb()
	}
}

// Load initializes config, loads groups, and starts file watchers
func Load() error {
	stopWatchers = make(chan struct{})

	if err := Parse(); err != nil {
		return err
	}

	if err := LoadGroups(); err != nil {
		logging.Warn("No groups.json found", zap.Error(err))
	}

	if err := startConfigWatcher(); err != nil {
		return fmt.Errorf("failed to start config watcher: %w", err)
	}

	if err := startGroupsWatcher(); err != nil {
		logging.Warn("Groups watcher not started", zap.Error(err))
	}

	return nil
}

// Parse reads .env and updates config atomically
func Parse() error {
	if err := godotenv.Load(); err != nil {
		logging.Info("No .env file found, using environment variables")
	}

	cfg := &Config{
		LLMAPIKey:        getEnv("LLM_API_KEY", ""),
		LLMBaseURL:       getEnv("LLM_BASE_URL", "https://generativelanguage.googleapis.com"),
		LLMModel:         getEnv("LLM_MODEL", "gemini-2.5-flash"),
		LLMProvider:      getEnv("LLM_PROVIDER", "gemini"),
		SystemPromptFile: getEnv("SYSTEM_PROMPT_FILE", "system_prompt.txt"),
		BotName:          getEnv("BOT_NAME", "meeting-minutes-bot"),
		SummaryTrigger: SummaryTriggerConfig{
			IntervalMinutes:       getEnvInt("SUMMARY_INTERVAL_MINUTES", 30),
			MessageCount:          getEnvInt("SUMMARY_MESSAGE_COUNT", 50),
			Keyword:               getEnv("SUMMARY_KEYWORD", "@bot 总结"),
			MinMessagesForSummary: getEnvInt("MIN_MESSAGES_FOR_SUMMARY", 5),
		},
		MediaSupport: MediaSupportConfig{
			ImageEnabled:  getEnvBool("MEDIA_IMAGE_ENABLED", true),
			VideoEnabled:  getEnvBool("MEDIA_VIDEO_ENABLED", true),
			AudioEnabled:  getEnvBool("MEDIA_AUDIO_ENABLED", true),
			PDFEnabled:    getEnvBool("MEDIA_PDF_ENABLED", true),
			MaxImageBytes: getEnvBytes("MEDIA_MAX_IMAGE_SIZE", 10*1024*1024),
			MaxVideoBytes: getEnvBytes("MEDIA_MAX_VIDEO_SIZE", 20*1024*1024),
			MaxAudioBytes: getEnvBytes("MEDIA_MAX_AUDIO_SIZE", 10*1024*1024),
			MaxPDFBytes:   getEnvBytes("MEDIA_MAX_PDF_SIZE", 10*1024*1024),
		},
		MaxBufferSize: getEnvInt("MAX_BUFFER_SIZE", 200),
	}

	if err := cfg.validate(); err != nil {
		return err
	}

	configPtr.Store(cfg)
	return nil
}

// LoadGroups loads target groups from groups.json
func LoadGroups() error {
	data, err := os.ReadFile(groupsFile)
	if err != nil {
		return err
	}

	var groups []string
	if err := json.Unmarshal(data, &groups); err != nil {
		return fmt.Errorf("failed to parse groups.json: %w", err)
	}

	targetGroups.Store(&groups)
	logging.Info("Loaded target groups from groups.json", zap.Int("count", len(groups)))
	return nil
}

// SaveGroups saves target groups to groups.json
func SaveGroups(groups []string) error {
	data, err := json.MarshalIndent(groups, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal groups: %w", err)
	}

	if err := os.WriteFile(groupsFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write groups.json: %w", err)
	}

	targetGroups.Store(&groups)
	logging.Info("Saved groups to groups.json", zap.Int("count", len(groups)))
	return nil
}

func startConfigWatcher() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	configWatcher = watcher

	if err := watcher.Add(".env"); err != nil {
		watcher.Close()
		return fmt.Errorf("failed to watch .env: %w", err)
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
					logging.Info(".env changed, reloading...")
					if err := Parse(); err != nil {
						logging.Error("Error reloading config", zap.Error(err))
					} else {
						logging.Info("Config reloaded successfully")
						notifyConfigCallbacks()
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				logging.Error("Watcher error", zap.Error(err))
			case <-stopWatchers:
				return
			}
		}
	}()

	logging.Info("Watching .env for changes")
	return nil
}

func startGroupsWatcher() error {
	if _, err := os.Stat(groupsFile); os.IsNotExist(err) {
		return nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	groupsWatcher = watcher

	if err := watcher.Add(groupsFile); err != nil {
		watcher.Close()
		return fmt.Errorf("failed to watch groups.json: %w", err)
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
					logging.Info("groups.json changed, reloading...")
					if err := LoadGroups(); err != nil {
						logging.Error("Error reloading groups", zap.Error(err))
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				logging.Error("Groups watcher error", zap.Error(err))
			case <-stopWatchers:
				return
			}
		}
	}()

	logging.Info("Watching groups.json for changes")
	return nil
}

// StopWatchers stops all file watchers
func StopWatchers() {
	if stopWatchers != nil {
		close(stopWatchers)
	}
}

func (c *Config) validate() error {
	if c.LLMAPIKey == "" {
		return fmt.Errorf("LLM_API_KEY is required")
	}
	if c.SystemPromptFile == "" {
		return fmt.Errorf("SYSTEM_PROMPT_FILE is required")
	}

	logging.Info("Configuration loaded successfully")
	logging.Info("Bot settings",
		zap.String("name", c.BotName),
		zap.String("model", c.LLMModel),
		zap.String("baseURL", c.LLMBaseURL),
		zap.String("promptFile", c.SystemPromptFile))

	groups := GetTargetGroups()
	if len(groups) > 0 {
		logging.Info("Target groups", zap.Strings("groups", groups))
	} else {
		logging.Info("Target groups: All")
	}

	logging.Info("Summary triggers",
		zap.Int("interval", c.SummaryTrigger.IntervalMinutes),
		zap.Int("messageCount", c.SummaryTrigger.MessageCount),
		zap.Int("minMessages", c.SummaryTrigger.MinMessagesForSummary),
		zap.String("keyword", c.SummaryTrigger.Keyword))

	return nil
}

func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func getEnvInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	intValue, err := strconv.Atoi(value)
	if err != nil {
		logging.Warn("Invalid integer value, using default",
			zap.String("key", key),
			zap.Int("default", defaultValue),
			zap.Error(err))
		return defaultValue
	}
	return intValue
}

func getEnvBytes(key string, defaultValue int64) int64 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	bytes, err := parseBytes(value)
	if err != nil {
		logging.Warn("Invalid byte size value, using default",
			zap.String("key", key),
			zap.Int64("default", defaultValue),
			zap.Error(err))
		return defaultValue
	}
	return bytes
}

func parseBytes(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty value")
	}

	s = strings.ToUpper(s)
	var multiplier int64 = 1
	suffix := s[len(s)-1]

	switch suffix {
	case 'K':
		multiplier = 1024
		s = s[:len(s)-1]
	case 'M':
		multiplier = 1024 * 1024
		s = s[:len(s)-1]
	case 'G':
		multiplier = 1024 * 1024 * 1024
		s = s[:len(s)-1]
	case 'B':
		if len(s) >= 2 {
			prev := s[len(s)-2]
			switch prev {
			case 'K':
				multiplier = 1024
				s = s[:len(s)-2]
			case 'M':
				multiplier = 1024 * 1024
				s = s[:len(s)-2]
			case 'G':
				multiplier = 1024 * 1024 * 1024
				s = s[:len(s)-2]
			default:
				s = s[:len(s)-1]
			}
		} else {
			s = s[:len(s)-1]
		}
	}

	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return val * multiplier, nil
}

func getEnvBool(key string, defaultValue bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	boolValue, err := strconv.ParseBool(value)
	if err != nil {
		logging.Warn("Invalid boolean value, using default",
			zap.String("key", key),
			zap.Bool("default", defaultValue),
			zap.Error(err))
		return defaultValue
	}
	return boolValue
}
