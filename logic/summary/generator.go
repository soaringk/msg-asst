package summary

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/soaringk/wechat-meeting-scribe/entity/chat"
	"github.com/soaringk/wechat-meeting-scribe/entity/llm"
	"github.com/soaringk/wechat-meeting-scribe/pkg/logging"
	"go.uber.org/zap"
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

func (g *Generator) Generate(ctx context.Context, buf *chat.MessageBuffer, roomTopic string) (Result, error) {
	snapshot := buf.GetSnapshot(roomTopic)

	if snapshot.Count == 0 || len(snapshot.Contents) == 0 {
		return Result{SkipReason: "empty_buffer"}, nil
	}

	logging.Debug("Generating summary",
		zap.Int("count", snapshot.Count),
		zap.String("room", roomTopic))

	timeRange := g.buildTimeRange(snapshot)

	summary, err := g.llmService.GenerateSummary(ctx, roomTopic, timeRange, snapshot.Count, snapshot.Contents)
	if err != nil {
		return Result{}, fmt.Errorf("failed to generate summary: %w", err)
	}

	trimmed := strings.TrimSpace(summary)
	if trimmed == "" || trimmed == "暂无重要更新" {
		return Result{SkipReason: "no_important_update"}, nil
	}

	header := g.generateHeader(snapshot, roomTopic)
	return Result{Text: fmt.Sprintf("%s\n\n%s", header, trimmed)}, nil
}

func (g *Generator) Close() {
	g.llmService.Close()
}

func (g *Generator) generateHeader(snapshot chat.Snapshot, roomTopic string) string {
	now := time.Now()
	dateStr := now.Format("2006年1月2日 Monday")
	timeRange := g.buildTimeRange(snapshot)
	return fmt.Sprintf("# 🤖 %s 会议纪要\n📅 日期：%s\n⏰ 时间：%s\n", roomTopic, dateStr, timeRange)
}

func (g *Generator) buildTimeRange(snapshot chat.Snapshot) string {
	if snapshot.FirstMsgTime == nil || snapshot.LastMsgTime == nil {
		return "N/A"
	}
	return fmt.Sprintf("%s - %s", snapshot.FirstMsgTime.Format("15:04"), snapshot.LastMsgTime.Format("15:04"))
}
