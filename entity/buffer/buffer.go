package buffer

import (
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/alphadose/haxmap"
	"github.com/soaringk/wechat-meeting-scribe/entity/config"
)

type BufferedMessage struct {
	ID        string
	Timestamp time.Time
	Sender    string
	Content   string
	RoomTopic string
}

type roomData struct {
	mu              sync.Mutex
	messages        []BufferedMessage
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
	room, ok := b.rooms.Get(roomTopic)
	if !ok {
		cap := config.GetConfig().MaxBufferSize
		room = &roomData{
			messages:   make([]BufferedMessage, cap),
			capacity:   cap,
			messageIDs: make(map[string]struct{}),
		}
		b.rooms.Set(roomTopic, room)
	}
	return room
}

func (b *MessageBuffer) Add(msg BufferedMessage) {
	room := b.getOrCreateRoom(msg.RoomTopic)
	room.mu.Lock()
	defer room.mu.Unlock()

	if _, ok := room.messageIDs[msg.ID]; ok {
		log.Printf("[Buffer] Duplicate message ID '%s' detected in room '%s', skipping", msg.ID, msg.RoomTopic)
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

	log.Printf("[Buffer] Message added to room '%s'. Total: %d (ring buffer)", msg.RoomTopic, room.count)
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

	log.Printf("[Buffer] Clear %d messages from room '%s'", room.count, roomTopic)
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

	room.mu.Lock()
	defer room.mu.Unlock()

	cfg := config.GetConfig()

	if room.count < cfg.SummaryTrigger.MinMessagesForSummary {
		log.Printf("[Buffer] Not enough messages in room '%s' for summary (%d/%d)",
			roomTopic, room.count, cfg.SummaryTrigger.MinMessagesForSummary)
		return false
	}

	if triggeredByKeyword {
		log.Printf("[Buffer] Summary triggered by keyword in room '%s'", roomTopic)
		return true
	}

	if cfg.SummaryTrigger.MessageCount > 0 &&
		room.count >= cfg.SummaryTrigger.MessageCount {
		log.Printf("[Buffer] Summary triggered by message count in room '%s' (%d/%d)",
			roomTopic, room.count, cfg.SummaryTrigger.MessageCount)
		return true
	}

	if cfg.SummaryTrigger.IntervalMinutes > 0 {
		if !room.lastSummaryTime.IsZero() {
			minutesSinceLast := time.Since(room.lastSummaryTime).Minutes()
			if minutesSinceLast >= float64(cfg.SummaryTrigger.IntervalMinutes) {
				log.Printf("[Buffer] Summary triggered by time interval in room '%s' (%.1f/%d minutes)",
					roomTopic, minutesSinceLast, cfg.SummaryTrigger.IntervalMinutes)
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
	FormattedMsg []string
}

func (b *MessageBuffer) GetSnapshot(roomTopic string) Snapshot {
	room, ok := b.rooms.Get(roomTopic)
	if !ok {
		return Snapshot{
			Count:        0,
			Participants: make(map[string]struct{}),
			FormattedMsg: nil,
		}
	}

	room.mu.Lock()
	defer room.mu.Unlock()

	snapshot := Snapshot{
		Count:        room.count,
		Participants: make(map[string]struct{}),
	}

	if room.count > 0 {
		startIndex := 0
		if room.count == room.capacity {
			startIndex = room.writeIndex
		}

		firstMsg := room.messages[startIndex]
		lastMsg := room.messages[(startIndex+room.count-1)%room.capacity]

		snapshot.FirstMsgTime = &firstMsg.Timestamp
		snapshot.LastMsgTime = &lastMsg.Timestamp

		snapshot.FormattedMsg = make([]string, room.count)
		for i := 0; i < room.count; i++ {
			msgIndex := (startIndex + i) % room.capacity
			msg := room.messages[msgIndex]
			snapshot.Participants[msg.Sender] = struct{}{}
			timeStr := msg.Timestamp.Format("15:04")
			snapshot.FormattedMsg[i] = fmt.Sprintf("[%s] %s: %s", timeStr, msg.Sender, msg.Content)
		}
	}

	return snapshot
}
