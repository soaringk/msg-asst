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
	"github.com/soaringk/msg-asst/entity/chat"
	"github.com/soaringk/msg-asst/entity/config"
	"github.com/soaringk/msg-asst/logic/summary"
	"github.com/soaringk/msg-asst/pkg/logging"
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

func (b *Bot) Start(selectGroups bool) error {
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

	if selectGroups {
		if err := b.promptGroupSelection(); err != nil {
			return fmt.Errorf("group selection failed: %w", err)
		}
	}

	logging.Info("Bot is now active and monitoring messages")

	if config.GetConfig().SummaryTrigger.IntervalMinutes > 0 {
		b.startIntervalTimer()
	}

	b.bot.Block()
	return nil
}

func (b *Bot) promptGroupSelection() error {
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

	fmt.Println("\nEnter group numbers (comma-separated), or 'all':")
	fmt.Print("> ")

	reader := bufio.NewReader(os.Stdin)
	input, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}
	input = strings.TrimSpace(input)

	var selectedGroups []string

	if strings.ToLower(input) == "all" {
		for _, group := range groups {
			selectedGroups = append(selectedGroups, group.NickName)
		}
		logging.Info("Selected all groups", zap.Int("count", len(selectedGroups)))
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
			selectedGroups = append(selectedGroups, groups[num-1].NickName)
		}
	}

	if len(selectedGroups) == 0 {
		logging.Info("No groups selected, will monitor all groups")
		return nil
	}

	if err := config.SaveGroups(selectedGroups); err != nil {
		return fmt.Errorf("failed to save groups: %w", err)
	}

	fmt.Printf("\n✅ Saved %d groups to groups.json\n", len(selectedGroups))
	for _, group := range selectedGroups {
		fmt.Printf("   • %s\n", group)
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

	if !b.isTargetGroup(groupName) {
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
		ID:         msg.MsgId,
		Timestamp:  time.Now(),
		Sender:     senderUser.NickName,
		GroupTopic: groupName,
		Content:    extractedContent,
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

func (b *Bot) isTargetGroup(groupName string) bool {
	targetGroups := config.GetTargetGroups()
	if len(targetGroups) == 0 {
		return true
	}

	groupNameLower := strings.ToLower(groupName)
	for _, target := range targetGroups {
		if strings.Contains(groupNameLower, strings.ToLower(target)) {
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

func (b *Bot) triggerSummary(groupTopic string) {
	if _, loaded := b.activeSummaries.LoadOrStore(groupTopic, true); loaded {
		return
	}

	b.wg.Add(1)
	go func() {
		defer b.wg.Done()
		defer b.activeSummaries.Delete(groupTopic)
		b.generateAndSendSummary(groupTopic)
	}()
}

func (b *Bot) generateAndSendSummary(groupTopic string) {
	logging.Info("Generating summary", zap.String("group", groupTopic))

	result, err := b.generator.Generate(b.ctx, b.buffer, groupTopic)
	if err != nil {
		if err == context.Canceled {
			logging.Info("Summary generation cancelled", zap.String("group", groupTopic))
			return
		}
		logging.Error("Error generating summary", zap.String("group", groupTopic), zap.Error(err))
		return
	}

	if result.SkipReason != "" {
		logging.Info("Summary skipped", zap.String("group", groupTopic), zap.String("reason", result.SkipReason))
		b.buffer.Clear(groupTopic)
		return
	}

	if sendErr := b.sendToSelf(result.Text); sendErr != nil {
		logging.Error("Error sending summary", zap.Error(sendErr))
		return
	}

	b.buffer.Clear(groupTopic)
	logging.Info("Summary sent successfully", zap.String("group", groupTopic))
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
				groupTopics := b.buffer.GetGroupTopics()
				for _, topic := range groupTopics {
					if b.buffer.ShouldSummarize(topic, false) {
						logging.Info("Processing scheduled summary", zap.String("group", topic))
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
