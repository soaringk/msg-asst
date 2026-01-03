package chat

import (
	"fmt"
	"sync"
	"time"

	"github.com/alphadose/haxmap"
	"github.com/soaringk/wechat-meeting-scribe/entity/config"
	"github.com/soaringk/wechat-meeting-scribe/pkg/logging"
	"go.uber.org/zap"
)

type groupData struct {
	mu              sync.RWMutex
	messages        []Message
	writeIndex      int
	count           int
	capacity        int
	lastSummaryTime time.Time
	messageIDs      map[string]struct{}
}

type MessageBuffer struct {
	groups *haxmap.Map[string, *groupData]
}

func New() *MessageBuffer {
	return &MessageBuffer{
		groups: haxmap.New[string, *groupData](),
	}
}

func (b *MessageBuffer) getOrCreateGroup(groupTopic string) *groupData {
	group, _ := b.groups.GetOrCompute(groupTopic, func() *groupData {
		cap := config.GetConfig().MaxBufferSize
		return &groupData{
			messages:   make([]Message, cap),
			capacity:   cap,
			messageIDs: make(map[string]struct{}),
		}
	})
	return group
}

func (b *MessageBuffer) Add(msg Message) {
	group := b.getOrCreateGroup(msg.GroupTopic)
	group.mu.Lock()
	defer group.mu.Unlock()

	if _, ok := group.messageIDs[msg.ID]; ok {
		logging.Debug("Duplicate message ID detected, skipping",
			zap.String("id", msg.ID),
			zap.String("group", msg.GroupTopic))
		return
	}

	firstMsg := group.writeIndex
	if group.count == group.capacity {
		firstMsgID := group.messages[firstMsg].ID
		delete(group.messageIDs, firstMsgID)
	}

	group.messages[group.writeIndex] = msg
	group.messageIDs[msg.ID] = struct{}{}
	group.writeIndex = (group.writeIndex + 1) % group.capacity

	if group.count < group.capacity {
		group.count++
	}

	logging.Debug("Message added to buffer",
		zap.String("group", msg.GroupTopic),
		zap.Int("count", group.count))
}

func (b *MessageBuffer) GetGroupTopics() []string {
	topics := make([]string, 0)
	b.groups.ForEach(func(topic string, _ *groupData) bool {
		topics = append(topics, topic)
		return true
	})
	return topics
}

func (b *MessageBuffer) Clear(groupTopic string) {
	group, ok := b.groups.Get(groupTopic)
	if !ok {
		return
	}

	group.mu.Lock()
	defer group.mu.Unlock()

	logging.Info("Buffered messages cleared",
		zap.Int("count", group.count),
		zap.String("group", groupTopic))
	group.writeIndex = 0
	group.count = 0
	group.messageIDs = make(map[string]struct{})
	group.lastSummaryTime = time.Now()
}

func (b *MessageBuffer) ShouldSummarize(groupTopic string, triggeredByKeyword bool) bool {
	group, ok := b.groups.Get(groupTopic)
	if !ok {
		return false
	}

	group.mu.RLock()
	defer group.mu.RUnlock()

	cfg := config.GetConfig()

	if group.count < cfg.SummaryTrigger.MinMessagesForSummary {
		logging.Debug("Not enough messages for summary",
			zap.String("group", groupTopic),
			zap.Int("count", group.count),
			zap.Int("min", cfg.SummaryTrigger.MinMessagesForSummary))
		return false
	}

	if triggeredByKeyword {
		logging.Info("Summary triggered by keyword", zap.String("group", groupTopic))
		return true
	}

	if cfg.SummaryTrigger.MessageCount > 0 &&
		group.count >= cfg.SummaryTrigger.MessageCount {
		logging.Info("Summary triggered by message count",
			zap.String("group", groupTopic),
			zap.Int("count", group.count),
			zap.Int("trigger", cfg.SummaryTrigger.MessageCount))
		return true
	}

	if cfg.SummaryTrigger.IntervalMinutes > 0 {
		if !group.lastSummaryTime.IsZero() {
			minutesSinceLast := time.Since(group.lastSummaryTime).Minutes()
			if minutesSinceLast >= float64(cfg.SummaryTrigger.IntervalMinutes) {
				logging.Info("Summary triggered by time interval",
					zap.String("group", groupTopic),
					zap.Float64("minutesSinceLast", minutesSinceLast),
					zap.Int("interval", cfg.SummaryTrigger.IntervalMinutes))
				return true
			}
		}
	}

	return false
}

type Snapshot struct {
	Count        int
	FirstMsgTime *time.Time
	LastMsgTime  *time.Time
	Participants map[string]struct{}
	Contents     []*Content
}

func (b *MessageBuffer) GetSnapshot(groupTopic string) Snapshot {
	group, ok := b.groups.Get(groupTopic)
	if !ok {
		return Snapshot{Participants: make(map[string]struct{})}
	}

	group.mu.RLock()
	defer group.mu.RUnlock()

	snapshot := Snapshot{
		Count:        group.count,
		Participants: make(map[string]struct{}),
	}

	if group.count == 0 {
		return snapshot
	}

	startIndex := 0
	if group.count == group.capacity {
		startIndex = group.writeIndex
	}

	firstMsg := group.messages[startIndex]
	lastMsg := group.messages[(startIndex+group.count-1)%group.capacity]

	snapshot.FirstMsgTime = &firstMsg.Timestamp
	snapshot.LastMsgTime = &lastMsg.Timestamp
	snapshot.Contents = make([]*Content, 0, group.count*2)

	for i := 0; i < group.count; i++ {
		msgIndex := (startIndex + i) % group.capacity
		msg := group.messages[msgIndex]
		snapshot.Participants[msg.Sender] = struct{}{}

		header := fmt.Sprintf("[%s] %s:", msg.Timestamp.Format("15:04"), msg.Sender)
		snapshot.Contents = append(snapshot.Contents, &Content{
			Type: ContentTypeText,
			Text: header,
		})

		if msg.Content != nil {
			snapshot.Contents = append(snapshot.Contents, msg.Content)
		}
	}

	return snapshot
}
