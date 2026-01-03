package summary

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/soaringk/wechat-meeting-scribe/entity/buffer"
	"github.com/soaringk/wechat-meeting-scribe/entity/llm"
)

type Generator struct {
	llmService *llm.Service
}

type Result struct {
	Text       string
	SkipReason string
}

func New() *Generator {
	return &Generator{
		llmService: llm.New(),
	}
}

func (g *Generator) Generate(ctx context.Context, buf *buffer.MessageBuffer, roomTopic string) (Result, error) {
	snapshot := buf.GetSnapshot(roomTopic)

	if snapshot.Count == 0 {
		return Result{SkipReason: "empty_buffer"}, nil
	}

	log.Printf("[Summary] Generating summary for %d messages in room '%s'...", snapshot.Count, roomTopic)
	log.Printf("[Summary] Participants: %d", len(snapshot.Participants))
	log.Printf("[Summary] Time range: %s - %s", snapshot.FirstMsgTime.Format("15:04:05"), snapshot.LastMsgTime.Format("15:04:05"))

	if len(snapshot.FormattedMsg) == 0 {
		return Result{SkipReason: "empty_buffer"}, nil
	}
	timeRange := g.buildTimeRange(snapshot)

	summary, err := g.llmService.GenerateSummary(ctx, roomTopic, timeRange, snapshot.Count, snapshot.FormattedMsg)
	if err != nil {
		log.Printf("[Summary] Error generating summary for room '%s': %v", roomTopic, err)
		return Result{}, fmt.Errorf("failed to generate summary: %w", err)
	}

	trimmed := strings.TrimSpace(summary)
	if trimmed == "" || trimmed == "暂无重要更新" {
		log.Printf("[Summary] No important updates for room '%s'", roomTopic)
		return Result{SkipReason: "no_important_update"}, nil
	}

	header := g.generateHeader(snapshot, roomTopic)
	fullSummary := fmt.Sprintf("%s\n\n%s", header, trimmed)

	log.Printf("[Summary] Summary generated successfully for room '%s' (%d chars)", roomTopic, len(fullSummary))
	return Result{Text: fullSummary}, nil
}

func (g *Generator) Close() {
	g.llmService.Close()
}

func (g *Generator) generateHeader(snapshot buffer.Snapshot, roomTopic string) string {
	now := time.Now()
	dateStr := now.Format("2006年1月2日 Monday")

	timeRange := g.buildTimeRange(snapshot)

	return fmt.Sprintf("# 🤖 %s 会议纪要\n📅 日期：%s\n⏰ 时间：%s\n", roomTopic, dateStr, timeRange)
}

func (g *Generator) buildTimeRange(snapshot buffer.Snapshot) string {
	if snapshot.FirstMsgTime == nil || snapshot.LastMsgTime == nil {
		return "N/A"
	}
	start := snapshot.FirstMsgTime.Format("15:04")
	end := snapshot.LastMsgTime.Format("15:04")
	return fmt.Sprintf("%s - %s", start, end)
}
