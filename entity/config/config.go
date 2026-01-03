package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"
	"github.com/joho/godotenv"
)

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
	SystemPromptFile string
	BotName          string
	SummaryTrigger   SummaryTriggerConfig
	MaxBufferSize    int
	SummaryQueueSize int
}

var (
	configPtr       atomic.Pointer[Config]
	targetRooms     atomic.Pointer[[]string]
	configWatcher   *fsnotify.Watcher
	roomsWatcher    *fsnotify.Watcher
	callbacksMu     sync.RWMutex
	configCallbacks []func()
	stopWatchers    chan struct{}
)

const roomsFile = "rooms.json"

// GetConfig returns the current config (thread-safe)
func GetConfig() *Config {
	return configPtr.Load()
}

// GetTargetRooms returns the current target rooms (thread-safe)
func GetTargetRooms() []string {
	rooms := targetRooms.Load()
	if rooms == nil {
		return nil
	}
	return *rooms
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

// Load initializes config, loads rooms, and starts file watchers
func Load() error {
	stopWatchers = make(chan struct{})

	if err := Parse(); err != nil {
		return err
	}

	if err := LoadRooms(); err != nil {
		log.Printf("[Config] No rooms.json found, will use all rooms: %v", err)
	}

	if err := startConfigWatcher(); err != nil {
		return fmt.Errorf("failed to start config watcher: %w", err)
	}

	if err := startRoomsWatcher(); err != nil {
		log.Printf("[Config] Warning: rooms watcher not started: %v", err)
	}

	return nil
}

// Parse reads .env and updates config atomically
func Parse() error {
	if err := godotenv.Load(); err != nil {
		log.Println("[Config] No .env file found, using environment variables")
	}

	cfg := &Config{
		LLMAPIKey:        getEnv("LLM_API_KEY", ""),
		LLMBaseURL:       getEnv("LLM_BASE_URL", "https://generativelanguage.googleapis.com/v1beta/openai/"),
		LLMModel:         getEnv("LLM_MODEL", "gemini-2.5-flash"),
		SystemPromptFile: getEnv("SYSTEM_PROMPT_FILE", "system_prompt.txt"),
		BotName:          getEnv("BOT_NAME", "meeting-minutes-bot"),
		SummaryTrigger: SummaryTriggerConfig{
			IntervalMinutes:       getEnvInt("SUMMARY_INTERVAL_MINUTES", 30),
			MessageCount:          getEnvInt("SUMMARY_MESSAGE_COUNT", 50),
			Keyword:               getEnv("SUMMARY_KEYWORD", "@bot 总结"),
			MinMessagesForSummary: getEnvInt("MIN_MESSAGES_FOR_SUMMARY", 5),
		},
		MaxBufferSize:    getEnvInt("MAX_BUFFER_SIZE", 200),
		SummaryQueueSize: getEnvInt("CONCURRENT_SUMMARY", 10),
	}

	if err := cfg.validate(); err != nil {
		return err
	}

	configPtr.Store(cfg)
	return nil
}

// LoadRooms loads target rooms from rooms.json
func LoadRooms() error {
	data, err := os.ReadFile(roomsFile)
	if err != nil {
		return err
	}

	var rooms []string
	if err := json.Unmarshal(data, &rooms); err != nil {
		return fmt.Errorf("failed to parse rooms.json: %w", err)
	}

	targetRooms.Store(&rooms)
	log.Printf("[Config] Loaded %d target rooms from rooms.json", len(rooms))
	return nil
}

// SaveRooms saves target rooms to rooms.json
func SaveRooms(rooms []string) error {
	data, err := json.MarshalIndent(rooms, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal rooms: %w", err)
	}

	if err := os.WriteFile(roomsFile, data, 0644); err != nil {
		return fmt.Errorf("failed to write rooms.json: %w", err)
	}

	targetRooms.Store(&rooms)
	log.Printf("[Config] Saved %d rooms to rooms.json", len(rooms))
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
					log.Println("[Config] .env changed, reloading...")
					if err := Parse(); err != nil {
						log.Printf("[Config] Error reloading config: %v", err)
					} else {
						log.Println("[Config] Config reloaded successfully")
						notifyConfigCallbacks()
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("[Config] Watcher error: %v", err)
			case <-stopWatchers:
				return
			}
		}
	}()

	log.Println("[Config] Watching .env for changes")
	return nil
}

func startRoomsWatcher() error {
	if _, err := os.Stat(roomsFile); os.IsNotExist(err) {
		return nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	roomsWatcher = watcher

	if err := watcher.Add(roomsFile); err != nil {
		watcher.Close()
		return fmt.Errorf("failed to watch rooms.json: %w", err)
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
					log.Println("[Config] rooms.json changed, reloading...")
					if err := LoadRooms(); err != nil {
						log.Printf("[Config] Error reloading rooms: %v", err)
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				log.Printf("[Config] Rooms watcher error: %v", err)
			case <-stopWatchers:
				return
			}
		}
	}()

	log.Println("[Config] Watching rooms.json for changes")
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
	if c.SummaryQueueSize <= 0 {
		log.Printf("[Config] Invalid SummaryQueueSize: %d", c.SummaryQueueSize)
	}

	log.Println("✓ Configuration loaded successfully")
	log.Printf("  - Bot name: %s", c.BotName)
	log.Printf("  - LLM base URL: %s", c.LLMBaseURL)
	log.Printf("  - LLM model: %s", c.LLMModel)
	log.Printf("  - System prompt file: %s", c.SystemPromptFile)

	rooms := GetTargetRooms()
	if len(rooms) > 0 {
		log.Printf("  - Target rooms: %s", strings.Join(rooms, ", "))
	} else {
		log.Println("  - Target rooms: All rooms")
	}

	log.Println("  - Summary triggers:")
	if c.SummaryTrigger.IntervalMinutes > 0 {
		log.Printf("    • Time-based: every %d minutes", c.SummaryTrigger.IntervalMinutes)
	} else {
		log.Println("    • Time-based: disabled")
	}

	if c.SummaryTrigger.MessageCount > 0 {
		log.Printf("    • Volume-based: every %d messages", c.SummaryTrigger.MessageCount)
	} else {
		log.Println("    • Volume-based: disabled")
	}

	if c.SummaryTrigger.Keyword != "" {
		log.Printf("    • Keyword: %s", c.SummaryTrigger.Keyword)
	} else {
		log.Println("    • Keyword: disabled")
	}

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
		log.Printf("Warning: Invalid integer value for %s, using default %d", key, defaultValue)
		return defaultValue
	}
	return intValue
}
