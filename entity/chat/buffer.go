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

type Message struct {
	ID        string
	Timestamp time.Time
	Sender    string
	RoomTopic string
	Content   *Content
}

type roomData struct {
	mu              sync.RWMutex
	messages        []Message
	writeIndex      int
	count           int
	capacity        int
	lastSummaryTime time.Time
	messageIDs      map[string]struct{}
}

type MessageBuffer struct {
	rooms *haxmap.Map[string, *roomData]
}

func New() *MessageBuffer {
	return &MessageBuffer{
		rooms: haxmap.New[string, *roomData](),
	}
}

func (b *MessageBuffer) getOrCreateRoom(roomTopic string) *roomData {
	room, _ := b.rooms.GetOrCompute(roomTopic, func() *roomData {
		cap := config.GetConfig().MaxBufferSize
		return &roomData{
			messages:   make([]Message, cap),
			capacity:   cap,
			messageIDs: make(map[string]struct{}),
		}
	})
	return room
}

func (b *MessageBuffer) Add(msg Message) {
	room := b.getOrCreateRoom(msg.RoomTopic)
	room.mu.Lock()
	defer room.mu.Unlock()

	if _, ok := room.messageIDs[msg.ID]; ok {
		logging.Debug("Duplicate message ID detected, skipping",
			zap.String("id", msg.ID),
			zap.String("room", msg.RoomTopic))
		return
	}

	firstMsg := room.writeIndex
	if room.count == room.capacity {
		firstMsgID := room.messages[firstMsg].ID
		delete(room.messageIDs, firstMsgID)
	}

	room.messages[room.writeIndex] = msg
	room.messageIDs[msg.ID] = struct{}{}
	room.writeIndex = (room.writeIndex + 1) % room.capacity

	if room.count < room.capacity {
		room.count++
	}

	logging.Debug("Message added to buffer",
		zap.String("room", msg.RoomTopic),
		zap.Int("count", room.count))
}

func (b *MessageBuffer) GetRoomTopics() []string {
	topics := make([]string, 0)
	b.rooms.ForEach(func(topic string, _ *roomData) bool {
		topics = append(topics, topic)
		return true
	})
	return topics
}

func (b *MessageBuffer) Clear(roomTopic string) {
	room, ok := b.rooms.Get(roomTopic)
	if !ok {
		return
	}

	room.mu.Lock()
	defer room.mu.Unlock()

	logging.Info("Buffered messages cleared",
		zap.Int("count", room.count),
		zap.String("room", roomTopic))
	room.writeIndex = 0
	room.count = 0
	room.messageIDs = make(map[string]struct{})
	room.lastSummaryTime = time.Now()
}

func (b *MessageBuffer) ShouldSummarize(roomTopic string, triggeredByKeyword bool) bool {
	room, ok := b.rooms.Get(roomTopic)
	if !ok {
		return false
	}

	room.mu.RLock()
	defer room.mu.RUnlock()

	cfg := config.GetConfig()

	if room.count < cfg.SummaryTrigger.MinMessagesForSummary {
		logging.Debug("Not enough messages for summary",
			zap.String("room", roomTopic),
			zap.Int("count", room.count),
			zap.Int("min", cfg.SummaryTrigger.MinMessagesForSummary))
		return false
	}

	if triggeredByKeyword {
		logging.Info("Summary triggered by keyword", zap.String("room", roomTopic))
		return true
	}

	if cfg.SummaryTrigger.MessageCount > 0 &&
		room.count >= cfg.SummaryTrigger.MessageCount {
		logging.Info("Summary triggered by message count",
			zap.String("room", roomTopic),
			zap.Int("count", room.count),
			zap.Int("trigger", cfg.SummaryTrigger.MessageCount))
		return true
	}

	if cfg.SummaryTrigger.IntervalMinutes > 0 {
		if !room.lastSummaryTime.IsZero() {
			minutesSinceLast := time.Since(room.lastSummaryTime).Minutes()
			if minutesSinceLast >= float64(cfg.SummaryTrigger.IntervalMinutes) {
				logging.Info("Summary triggered by time interval",
					zap.String("room", roomTopic),
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

func (b *MessageBuffer) GetSnapshot(roomTopic string) Snapshot {
	room, ok := b.rooms.Get(roomTopic)
	if !ok {
		return Snapshot{Participants: make(map[string]struct{})}
	}

	room.mu.RLock()
	defer room.mu.RUnlock()

	snapshot := Snapshot{
		Count:        room.count,
		Participants: make(map[string]struct{}),
	}

	if room.count == 0 {
		return snapshot
	}

	startIndex := 0
	if room.count == room.capacity {
		startIndex = room.writeIndex
	}

	firstMsg := room.messages[startIndex]
	lastMsg := room.messages[(startIndex+room.count-1)%room.capacity]

	snapshot.FirstMsgTime = &firstMsg.Timestamp
	snapshot.LastMsgTime = &lastMsg.Timestamp
	snapshot.Contents = make([]*Content, 0, room.count*2)

	for i := 0; i < room.count; i++ {
		msgIndex := (startIndex + i) % room.capacity
		msg := room.messages[msgIndex]
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
