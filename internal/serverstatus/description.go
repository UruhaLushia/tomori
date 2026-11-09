package serverstatus

import (
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"
)

func chatText(raw []byte, out *strings.Builder, depth int) {
	if depth > 16 || out.Len() >= 4096 || len(raw) == 0 {
		return
	}
	if raw[0] == '"' {
		var text string
		if json.Unmarshal(raw, &text) == nil {
			out.WriteString(clip(text, 4096-out.Len()))
		}
		return
	}
	if raw[0] == '[' {
		var parts []json.RawMessage
		if json.Unmarshal(raw, &parts) == nil {
			for _, part := range parts {
				chatText(part, out, depth+1)
				if out.Len() >= 4096 {
					break
				}
			}
		}
		return
	}
	var part struct {
		Text, Fallback, Translate string
		Extra                     []json.RawMessage
	}
	if json.Unmarshal(raw, &part) != nil {
		return
	}
	text := part.Text
	if text == "" {
		text = part.Fallback
	}
	if text == "" {
		text = part.Translate
	}
	out.WriteString(clip(text, 4096-out.Len()))
	for _, child := range part.Extra {
		chatText(child, out, depth+1)
		if out.Len() >= 4096 {
			break
		}
	}
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func plainText(s string) string {
	var b strings.Builder
	format := false
	for _, r := range s {
		if format {
			format = false
			continue
		}
		if r == '§' {
			format = true
			continue
		}
		if !unicode.IsControl(r) || r == '\n' {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}
