package launcher

import (
	"context"
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
	"time"
)

func DecodeLogLines(ctx context.Context, lines []string, emit func(string)) error {
	output := logOutput{emit: emit}
	for _, line := range lines {
		if err := ctx.Err(); err != nil {
			return err
		}
		output.write(line)
	}
	output.flush()
	return nil
}

type logOutput struct {
	emit    func(string)
	pending strings.Builder
}

func (o *logOutput) write(line string) {
	if o.emit == nil {
		return
	}
	trimmed := strings.TrimSpace(line)
	start := strings.HasPrefix(trimmed, "<log4j:Event ") || strings.HasPrefix(trimmed, "<Event ")
	if start && o.pending.Len() != 0 {
		o.flush()
	}
	if !start && o.pending.Len() == 0 {
		if strings.HasPrefix(trimmed, "<?xml ") || strings.HasPrefix(trimmed, "<log4j:EventSet") ||
			trimmed == "</log4j:EventSet>" || trimmed == "<Events>" || trimmed == "</Events>" {
			return
		}
		o.emit(line)
		return
	}
	if o.pending.Len()+len(line) > 1<<20 {
		o.flush()
		o.emit(line)
		return
	}
	if o.pending.Len() > 0 {
		o.pending.WriteByte('\n')
	}
	o.pending.WriteString(line)
	if !strings.HasSuffix(trimmed, "</log4j:Event>") && !strings.HasSuffix(trimmed, "</Event>") {
		return
	}
	var event struct {
		Logger    string `xml:"logger,attr"`
		Timestamp string `xml:"timestamp,attr"`
		TimeMilli string `xml:"timeMillis,attr"`
		Level     string `xml:"level,attr"`
		Thread    string `xml:"thread,attr"`
		Message   string `xml:"Message"`
		Throwable string `xml:"Throwable"`
	}
	if err := xml.Unmarshal([]byte(o.pending.String()), &event); err != nil {
		o.flush()
		return
	}
	o.pending.Reset()
	at := time.Now()
	stamp := event.Timestamp
	if stamp == "" {
		stamp = event.TimeMilli
	}
	if millis, err := strconv.ParseInt(stamp, 10, 64); err == nil {
		at = time.UnixMilli(millis).Local()
	}
	if event.Level == "" {
		event.Level = "INFO"
	}
	if event.Thread == "" {
		event.Thread = "main"
	}
	if event.Logger == "" {
		event.Logger = "Minecraft"
	}
	message := strings.TrimSpace(event.Message)
	if throwable := strings.TrimSpace(event.Throwable); throwable != "" {
		if message != "" {
			message += "\n"
		}
		message += throwable
	}
	o.emit(fmt.Sprintf("[%s] [%s/%s] [%s]: %s", at.Format("15:04:05.000"), event.Thread, event.Level, event.Logger, message))
}

func (o *logOutput) flush() {
	if o.pending.Len() == 0 {
		return
	}
	if o.emit != nil {
		o.emit(o.pending.String())
	}
	o.pending.Reset()
}
