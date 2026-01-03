package bot

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eatmoreapple/openwechat"
	"github.com/soaringk/wechat-meeting-scribe/entity/chat"
	"github.com/soaringk/wechat-meeting-scribe/entity/config"
	"github.com/soaringk/wechat-meeting-scribe/logic/summary"
	"github.com/soaringk/wechat-meeting-scribe/pkg/logging"
	"go.uber.org/zap"
)

type Bot struct {
	bot             *openwechat.Bot
	buffer          *chat.MessageBuffer
	generator       *summary.Generator
	self            *openwechat.Self
	stopTimer       chan struct{}
	activeSummaries sync.Map // map[string]bool - tracks groups with in-progress summaries
	stopOnce        sync.Once
	ctx             context.Context
	cancel          context.CancelFunc
	wg              sync.WaitGroup
}

func New() *Bot {
	ctx, cancel := context.WithCancel(context.Background())

	return &Bot{
		bot:       openwechat.DefaultBot(openwechat.Desktop),
		buffer:    chat.New(),
		generator: summary.New(),
		stopTimer: make(chan struct{}),
		ctx:       ctx,
		cancel:    cancel,
	}
}

func (b *Bot) Start(selectRooms bool) error {
	logging.Info("Initializing WeChat Meeting Scribe...")

	b.bot.UUIDCallback = openwechat.PrintlnQrcodeUrl
	b.bot.MessageHandler = b.handleMessage

	reloadStorage := openwechat.NewFileHotReloadStorage("storage.json")
	defer reloadStorage.Close()

	logging.Info("Starting bot...")
	logging.Info("Attempting hot login...")

	err := b.bot.PushLogin(reloadStorage, openwechat.NewRetryLoginOption())
	if err != nil {
		logging.Error("Login failed", zap.Error(err))
		return err
	}

	self, err := b.bot.GetCurrentUser()
	if err != nil {
		logging.Error("Failed to get current user", zap.Error(err))
		return err
	}
	b.self = self

	logging.Info("Logged in successfully", zap.String("user", self.NickName))

	if selectRooms {
		if err := b.promptRoomSelection(); err != nil {
			return fmt.Errorf("room selection failed: %w", err)
		}
	}

	logging.Info("Bot is now active and monitoring messages")

	if config.GetConfig().SummaryTrigger.IntervalMinutes > 0 {
		b.startIntervalTimer()
	}

	b.bot.Block()
	return nil
}

func (b *Bot) promptRoomSelection() error {
	groups, err := b.self.Groups()
	if err != nil {
		return fmt.Errorf("failed to get groups: %w", err)
	}

	if len(groups) == 0 {
		logging.Info("No groups found")
		return nil
	}

	fmt.Println("\n📋 Available Groups:")
	for i, group := range groups {
		fmt.Printf("   [%d] %s\n", i+1, group.NickName)
	}

	fmt.Println("\nEnter room numbers (comma-separated), or 'all':")
	fmt.Print("> ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}
	input = strings.TrimSpace(input)

	var selectedRooms []string

	if strings.ToLower(input) == "all" {
		for _, group := range groups {
			selectedRooms = append(selectedRooms, group.NickName)
		}
		logging.Info("Selected all rooms", zap.Int("count", len(selectedRooms)))
	} else {
		parts := strings.Split(input, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			num, err := strconv.Atoi(part)
			if err != nil || num < 1 || num > len(groups) {
				logging.Warn("Invalid selection", zap.String("input", part))
				continue
			}
			selectedRooms = append(selectedRooms, groups[num-1].NickName)
		}
	}

	if len(selectedRooms) == 0 {
		logging.Info("No rooms selected, will monitor all rooms")
		return nil
	}

	if err := config.SaveRooms(selectedRooms); err != nil {
		return fmt.Errorf("failed to save rooms: %w", err)
	}

	fmt.Printf("\n✅ Saved %d rooms to rooms.json\n", len(selectedRooms))
	for _, room := range selectedRooms {
		fmt.Printf("   • %s\n", room)
	}

	return nil
}

func (b *Bot) Stop() {
	b.stopOnce.Do(func() {
		logging.Info("Stopping bot...")
		b.cancel()
		b.stopIntervalTimer()
		b.wg.Wait()
		b.generator.Close()
		config.StopWatchers()
		logging.Info("Bot stopped gracefully")
	})
}

func (b *Bot) handleMessage(msg *openwechat.Message) {
	if msg.IsSendBySelf() {
		return
	}

	if !b.isSupportedMessageType(msg) {
		return
	}

	sender, err := msg.Sender()
	if err != nil {
		return
	}

	if !sender.IsGroup() {
		return
	}

	group := openwechat.Group{User: sender}
	groupName := group.NickName

	if !b.isTargetRoom(groupName) {
		return
	}

	senderUser, err := msg.SenderInGroup()
	if err != nil {
		return
	}

	extractedContent, err := chat.ExtractFromMessage(msg)
	if err != nil {
		return
	}

	if !b.isMediaAllowed(extractedContent) {
		return
	}

	if extractedContent.Type == chat.ContentTypeText && strings.TrimSpace(extractedContent.Text) == "" {
		return
	}

	b.buffer.Add(chat.Message{
		ID:        msg.MsgId,
		Timestamp: time.Now(),
		Sender:    senderUser.NickName,
		RoomTopic: groupName,
		Content:   extractedContent,
	})

	if b.buffer.ShouldSummarize(groupName, b.checkKeywordTrigger(extractedContent.Text)) {
		b.triggerSummary(groupName)
	}
}

func (b *Bot) isSupportedMessageType(msg *openwechat.Message) bool {
	return msg.IsText() || msg.IsPicture() || msg.IsVideo() || msg.IsVoice() || msg.IsMedia()
}

func (b *Bot) isMediaAllowed(c *chat.Content) bool {
	cfg := config.GetConfig()
	ms := cfg.MediaSupport

	var enabled bool
	var maxBytes int64

	switch c.Type {
	case chat.ContentTypeText:
		return true
	case chat.ContentTypeImage:
		enabled, maxBytes = ms.ImageEnabled, ms.MaxImageBytes
	case chat.ContentTypeVideo:
		enabled, maxBytes = ms.VideoEnabled, ms.MaxVideoBytes
	case chat.ContentTypeAudio:
		enabled, maxBytes = ms.AudioEnabled, ms.MaxAudioBytes
	case chat.ContentTypePDF:
		enabled, maxBytes = ms.PDFEnabled, ms.MaxPDFBytes
	case chat.ContentTypeFile:
		return true
	default:
		return true
	}

	if !enabled {
		return false
	}
	return c.Data == nil || int64(len(c.Data)) <= maxBytes
}

func (b *Bot) isTargetRoom(roomName string) bool {
	targetRooms := config.GetTargetRooms()
	if len(targetRooms) == 0 {
		return true
	}

	roomNameLower := strings.ToLower(roomName)
	for _, target := range targetRooms {
		if strings.Contains(roomNameLower, strings.ToLower(target)) {
			return true
		}
	}
	return false
}

func (b *Bot) checkKeywordTrigger(text string) bool {
	keyword := config.GetConfig().SummaryTrigger.Keyword
	if keyword == "" {
		return false
	}
	return strings.Contains(text, keyword)
}

func (b *Bot) triggerSummary(roomTopic string) {
	if _, loaded := b.activeSummaries.LoadOrStore(roomTopic, true); loaded {
		return
	}

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		defer b.activeSummaries.Delete(roomTopic)
		b.generateAndSendSummary(roomTopic)
	}()
}

func (b *Bot) generateAndSendSummary(roomTopic string) {
	logging.Info("Generating summary", zap.String("room", roomTopic))

	result, err := b.generator.Generate(b.ctx, b.buffer, roomTopic)
	if err != nil {
		if err == context.Canceled {
			logging.Info("Summary generation cancelled", zap.String("room", roomTopic))
			return
		}
		logging.Error("Error generating summary", zap.String("room", roomTopic), zap.Error(err))
		return
	}

	if result.SkipReason != "" {
		logging.Info("Summary skipped", zap.String("room", roomTopic), zap.String("reason", result.SkipReason))
		b.buffer.Clear(roomTopic)
		return
	}

	if sendErr := b.sendToSelf(result.Text); sendErr != nil {
		logging.Error("Error sending summary", zap.Error(sendErr))
		return
	}

	b.buffer.Clear(roomTopic)
	logging.Info("Summary sent successfully", zap.String("room", roomTopic))
}

func (b *Bot) sendToSelf(message string) error {
	if b.self == nil {
		return fmt.Errorf("self user not available")
	}

	fileHelper := b.self.FileHelper()
	_, err := fileHelper.SendText(message)
	return err
}

func (b *Bot) startIntervalTimer() {
	intervalMinutes := config.GetConfig().SummaryTrigger.IntervalMinutes
	logging.Info("Starting interval timer", zap.Int("interval", intervalMinutes))

	ticker := time.NewTicker(time.Duration(intervalMinutes) * time.Minute)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				logging.Info("Interval timer triggered")
				roomTopics := b.buffer.GetRoomTopics()
				for _, topic := range roomTopics {
					if b.buffer.ShouldSummarize(topic, false) {
						logging.Info("Processing scheduled summary", zap.String("room", topic))
						b.triggerSummary(topic)
					}
				}
			case <-b.stopTimer:
				logging.Info("Interval timer stopped")
				return
			}
		}
	}()
}

func (b *Bot) stopIntervalTimer() {
	select {
	case b.stopTimer <- struct{}{}:
	default:
	}
}
