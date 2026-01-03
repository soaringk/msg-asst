package bot

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/eatmoreapple/openwechat"
	"github.com/soaringk/wechat-meeting-scribe/entity/buffer"
	"github.com/soaringk/wechat-meeting-scribe/entity/config"
	"github.com/soaringk/wechat-meeting-scribe/logic/summary"
)

type Bot struct {
	bot          *openwechat.Bot
	buffer       *buffer.MessageBuffer
	generator    *summary.Generator
	self         *openwechat.Self
	stopTimer    chan struct{}
	summaryQueue chan string
	stopOnce     sync.Once
	ctx          context.Context
	cancel       context.CancelFunc
}

func New() *Bot {
	ctx, cancel := context.WithCancel(context.Background())

	return &Bot{
		bot:          openwechat.DefaultBot(openwechat.Desktop),
		buffer:       buffer.New(),
		generator:    summary.New(),
		stopTimer:    make(chan struct{}),
		summaryQueue: make(chan string, config.GetConfig().SummaryQueueSize),
		ctx:          ctx,
		cancel:       cancel,
	}
}

func (b *Bot) Start(selectRooms bool) error {
	log.Println("🤖 Initializing WeChat Meeting Scribe...")

	b.bot.UUIDCallback = openwechat.PrintlnQrcodeUrl
	b.bot.MessageHandler = b.handleMessage

	reloadStorage := openwechat.NewFileHotReloadStorage("storage.json")
	defer reloadStorage.Close()

	log.Println("🚀 Starting bot...")
	log.Println("⏳ Attempting hot login...")

	err := b.bot.PushLogin(reloadStorage, openwechat.NewRetryLoginOption())
	if err != nil {
		log.Printf("❌ Login failed: %v", err)
		return err
	}

	self, err := b.bot.GetCurrentUser()
	if err != nil {
		log.Printf("❌ Failed to get current user: %v", err)
		return err
	}
	b.self = self

	log.Printf("\n✅ User %s logged in successfully!", self.NickName)

	if selectRooms {
		if err := b.promptRoomSelection(); err != nil {
			return fmt.Errorf("room selection failed: %w", err)
		}
	}

	log.Println("   [Bot] Bot is now active and monitoring messages.")

	go b.summaryWorker()

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
		log.Println("No groups found.")
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
		log.Printf("✅ Selected all %d rooms", len(selectedRooms))
	} else {
		parts := strings.Split(input, ",")
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			num, err := strconv.Atoi(part)
			if err != nil || num < 1 || num > len(groups) {
				log.Printf("⚠️  Invalid selection: %s (skipping)", part)
				continue
			}
			selectedRooms = append(selectedRooms, groups[num-1].NickName)
		}
	}

	if len(selectedRooms) == 0 {
		log.Println("No rooms selected, will monitor all rooms.")
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
		log.Println("\n[Bot] Stopping bot...")
		b.cancel()
		b.stopIntervalTimer()
		close(b.summaryQueue)
		b.generator.Close()
		config.StopWatchers()
		log.Println("[Bot] Bot stopped gracefully")
	})
}

func (b *Bot) handleMessage(msg *openwechat.Message) {
	if msg.IsSendBySelf() || !msg.IsText() {
		return
	}

	sender, err := msg.Sender()
	if err != nil {
		log.Printf("Error getting message sender: %v", err)
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
		log.Printf("Error getting sender in group: %v", err)
		return
	}

	content := msg.Content
	if strings.TrimSpace(content) == "" {
		return
	}

	bufferedMsg := buffer.BufferedMessage{
		ID:        msg.MsgId,
		Timestamp: time.Now(),
		Sender:    senderUser.NickName,
		Content:   content,
		RoomTopic: groupName,
	}

	b.buffer.Add(bufferedMsg)

	if b.buffer.ShouldSummarize(groupName, b.checkKeywordTrigger(content)) {
		select {
		case b.summaryQueue <- groupName:
		default:
			log.Printf("[Bot] WARN: Summary queue is full, dropping request for room '%s'", groupName)
		}
	}
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

func (b *Bot) summaryWorker() {
	for roomTopic := range b.summaryQueue {
		b.generateAndSendSummary(roomTopic)
	}
	log.Println("[Bot] Summary worker stopped")
}

func (b *Bot) generateAndSendSummary(roomTopic string) {
	log.Printf("\n📝 [Bot] Generating summary for room '%s'...", roomTopic)

	result, err := b.generator.Generate(b.ctx, b.buffer, roomTopic)
	if err != nil {
		if err == context.Canceled {
			log.Printf("[Bot] Summary generation cancelled for room '%s'", roomTopic)
			return
		}
		log.Printf("❌ [Bot] Error generating summary for room '%s': %v", roomTopic, err)
		return
	}

	if result.SkipReason != "" {
		log.Printf("[Bot] Summary skipped for room '%s' (%s)", roomTopic, result.SkipReason)
		b.buffer.Clear(roomTopic)
		return
	}

	if sendErr := b.sendToSelf(result.Text); sendErr != nil {
		log.Printf("❌ [Bot] Error sending summary: %v", sendErr)
		return
	}

	b.buffer.Clear(roomTopic)
	log.Printf("✅ [Bot] Summary sent successfully for room '%s'\n", roomTopic)
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
	log.Printf("⏱️  [Bot] Starting interval timer (%d minutes)", intervalMinutes)

	ticker := time.NewTicker(time.Duration(intervalMinutes) * time.Minute)

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				log.Println("\n [Bot] Interval timer triggered")
				roomTopics := b.buffer.GetRoomTopics()
				for _, topic := range roomTopics {
					if b.buffer.ShouldSummarize(topic, false) {
						log.Printf("[Bot] Processing scheduled summary for room: %s", topic)
						select {
						case b.summaryQueue <- topic:
						default:
							log.Printf("[Bot] WARN: Summary queue is full, skipping scheduled summary for room '%s'", topic)
						}
					}
				}
			case <-b.stopTimer:
				log.Println(" [Bot] Interval timer stopped")
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
