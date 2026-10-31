package page

import (
	"strings"
	"time"

	"github.com/UruhaLushia/tomori/locales"
)

type LogLine struct {
	At      time.Time
	Text    string
	Clock   string
	Level   string
	Source  string
	Message string
}

func FormatLogLine(at time.Time, text string) LogLine {
	line := LogLine{At: at, Text: text, Level: "INFO", Source: "Tomori"}
	message := strings.TrimRight(stripLogANSI(text), "\r\n")
	if clock, level, source, body, ok := logPrefix(message); ok {
		if clock != "" {
			line.Clock = clock
		}
		line.Level, line.Source, message = level, source, body
	} else {
		switch {
		case strings.Contains(message, locales.Failed()), strings.Contains(message, locales.Error()),
			logStartsFold(message, "error:"), logStartsFold(message, "caused by:"),
			logStartsFold(message, "exception in thread "):
			line.Level = "ERROR"
		case strings.Contains(message, locales.Warning()), logStartsFold(message, "warning:"):
			line.Level = "WARN"
		}
	}
	if line.Clock == "" {
		line.Clock = at.Format("15:04:05")
	}
	line.Message = message
	return line
}

func FormatStoredLogLine(at time.Time, text string) LogLine {
	var source string
	if stamp, rest, ok := strings.Cut(text, " "); ok {
		if parsed, err := time.Parse(time.RFC3339, stamp); err == nil {
			at, text = parsed, rest
			header, tail, bracketed := logBracket(text)
			if bracketed && logSeverity(header) == "" && logClock(header) == "" && !strings.Contains(header, "/") {
				source, rest = header, tail
				text = rest
			}
		}
	}
	line := FormatLogLine(at, text)
	if source != "" {
		line.Source = source + " / " + line.Source
	}
	return line
}

func logPrefix(message string) (clock, level, source, body string, ok bool) {
	header, rest, found := logBracket(message)
	if !found {
		return
	}
	if clock = logClock(header); clock != "" {
		if header, tail, bracketed := logBracket(rest); bracketed {
			if slash := strings.LastIndexByte(header, '/'); slash > 0 {
				if level = logSeverity(header[slash+1:]); level != "" {
					source, body = header[:slash], tail
					if logger, tail, bracketed := logBracket(body); bracketed && strings.HasPrefix(tail, ":") {
						source, body = strings.TrimSuffix(logger, "/"), tail
					}
					return clock, level, source, logBody(body), true
				}
			}
		}
		if before, after, found := strings.Cut(rest, ":"); found {
			if level = logSeverity(before); level != "" {
				return clock, level, "Minecraft", strings.TrimLeft(after, " \t"), true
			}
		}
		return "", "", "", "", false
	}
	if level = logSeverity(header); level != "" {
		return "", level, "Tomori", logBody(rest), true
	}
	return "", "", "", "", false
}

func logBracket(text string) (header, rest string, ok bool) {
	if len(text) == 0 || text[0] != '[' {
		return "", text, false
	}
	if end := strings.IndexByte(text, ']'); end > 1 {
		return text[1:end], strings.TrimLeft(text[end+1:], " \t"), true
	}
	return "", text, false
}

func logBody(text string) string {
	return strings.TrimLeft(strings.TrimPrefix(text, ":"), " \t")
}

func logClock(text string) string {
	if len(text) >= 19 && text[4] == '-' && text[7] == '-' && text[10] == ' ' {
		text = text[11:]
	}
	if len(text) < 8 || text[2] != ':' || text[5] != ':' {
		return ""
	}
	for i := range len(text) {
		switch i {
		case 2, 5:
			continue
		case 8:
			if (text[i] == '.' || text[i] == ',') && len(text) > 9 {
				continue
			}
			return ""
		}
		if text[i] < '0' || text[i] > '9' {
			return ""
		}
	}
	return text
}

func logSeverity(text string) string {
	for _, level := range [...]string{"INFO", "WARN", "ERROR", "DEBUG", "TRACE", "FATAL"} {
		if strings.EqualFold(text, level) {
			return level
		}
	}
	if strings.EqualFold(text, "WARNING") {
		return "WARN"
	}
	return ""
}

func logStartsFold(text, prefix string) bool {
	return len(text) >= len(prefix) && strings.EqualFold(text[:len(prefix)], prefix)
}

func stripLogANSI(text string) string {
	esc := strings.IndexByte(text, '\x1b')
	if esc < 0 {
		return text
	}
	var out strings.Builder
	out.Grow(len(text))
	for esc >= 0 {
		out.WriteString(text[:esc])
		text = text[esc+1:]
		if len(text) > 0 {
			switch text[0] {
			case '[':
				i := 1
				for i < len(text) && (text[i] < 0x40 || text[i] > 0x7e) {
					i++
				}
				text = text[min(i+1, len(text)):]
			case ']':
				i := 1
				for i < len(text) {
					if text[i] == '\a' || text[i] == '\x1b' && i+1 < len(text) && text[i+1] == '\\' {
						break
					}
					i++
				}
				end := min(i+1, len(text))
				if i < len(text) && text[i] == '\x1b' {
					end = min(i+2, len(text))
				}
				text = text[end:]
			}
		}
		esc = strings.IndexByte(text, '\x1b')
	}
	out.WriteString(text)
	return out.String()
}
