package diagnostic

import (
	"net/url"
	"slices"
	"strings"
	"unicode"
)

type Redactor struct {
	exact    *strings.Replacer
	patterns []string
}

func NewRedactor(home string, secrets []string) *Redactor {
	type replacement struct{ from, to string }
	var replacements []replacement
	seen := make(map[string]bool)
	add := func(from, to string) {
		if from != "" && !seen[from] {
			seen[from] = true
			replacements = append(replacements, replacement{from, to})
		}
	}
	for _, secret := range secrets {
		if len(secret) < 4 {
			continue
		}
		add(secret, "[redacted]")
		add(url.QueryEscape(secret), "[redacted]")
		add(url.PathEscape(secret), "[redacted]")
	}
	if len(home) > 3 {
		add(home, "<home>")
		add(strings.ReplaceAll(home, "\\", "/"), "<home>")
	}
	r := &Redactor{}
	if len(replacements) > 0 {
		slices.SortStableFunc(replacements, func(a, b replacement) int { return len(b.from) - len(a.from) })
		pairs := make([]string, 0, len(replacements)*2)
		for _, pair := range replacements {
			pairs = append(pairs, pair.from, pair.to)
			r.patterns = append(r.patterns, pair.from)
		}
		r.exact = strings.NewReplacer(pairs...)
	}
	return r
}

func (r *Redactor) Text(text string) string {
	text = stripANSI(text)
	if strings.IndexFunc(text, func(c rune) bool { return unicode.IsControl(c) && c != '\n' && c != '\t' && c != '\r' }) >= 0 {
		text = strings.Map(func(c rune) rune {
			if unicode.IsControl(c) && c != '\n' && c != '\t' && c != '\r' {
				return -1
			}
			return c
		}, text)
	}
	if r != nil && r.exact != nil {
		for _, pattern := range r.patterns {
			if strings.Contains(text, pattern) {
				text = r.exact.Replace(text)
				break
			}
		}
	}
	text = redactFields(text)
	return redactAddresses(text)
}

func stripANSI(text string) string {
	if !strings.ContainsRune(text, '\x1b') {
		return text
	}
	var out strings.Builder
	for {
		esc := strings.IndexByte(text, '\x1b')
		if esc < 0 {
			out.WriteString(text)
			return out.String()
		}
		out.WriteString(text[:esc])
		text = text[esc+1:]
		if len(text) == 0 {
			continue
		}
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
}

func keyByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_' || c == '-'
}

func secretKey(text string) int {
	var key [64]byte
	n := 0
	for i := range len(text) {
		c := text[i]
		if c == '-' || c == '_' {
			continue
		}
		if n == len(key) {
			return 0
		}
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		key[n] = c
		n++
	}
	switch string(key[:n]) {
	case "authorization", "proxyauthorization", "cookie", "setcookie":
		return 2
	case "accesstoken", "refreshtoken", "idtoken", "clienttoken", "token", "apikey", "xapikey", "password", "passwd", "proxypassword", "secret", "clientsecret", "session", "sessionid", "username", "user", "uuid", "xuid", "email", "xamzsignature", "xamzcredential", "sig", "auth":
		return 1
	}
	return 0
}

func redactFields(text string) string {
	var out strings.Builder
	written := 0
	for i := 0; i < len(text); {
		if !keyByte(text[i]) {
			i++
			continue
		}
		start := i
		for i < len(text) && keyByte(text[i]) {
			i++
		}
		kind := secretKey(text[start:i])
		if kind == 0 {
			continue
		}
		end := i
		if start > 0 && (text[start-1] == '"' || text[start-1] == '\'') && end < len(text) && text[end] == text[start-1] {
			end++
		}
		for end < len(text) && (text[end] == ' ' || text[end] == '\t') {
			end++
		}
		if end < len(text) && (text[end] == '=' || text[end] == ':' || text[end] == ',' && strings.HasPrefix(text[start:i], "--")) {
			end++
		} else if !strings.HasPrefix(text[start:i], "--") || end == i {
			continue
		}
		for end < len(text) && (text[end] == ' ' || text[end] == '\t') {
			end++
		}
		if end == len(text) || text[end] == '\n' || text[end] == '\r' || strings.HasPrefix(text[end:], "--") {
			continue
		}
		value := end
		if strings.HasPrefix(text[end:], "[redacted]") {
			end += len("[redacted]")
		} else if text[end] == '"' || text[end] == '\'' {
			quote := text[end]
			end++
			value = end
			for end < len(text) && text[end] != quote {
				if text[end] == '\\' && end+1 < len(text) {
					end++
				}
				end++
			}
		} else if kind == 2 || text[end] == '{' || text[end] == '[' {
			for end < len(text) && text[end] != '\r' && text[end] != '\n' {
				end++
			}
		} else {
			for end < len(text) && !strings.ContainsRune(" \t\r\n,;&])}", rune(text[end])) {
				end++
			}
		}
		if end == value {
			continue
		}
		out.WriteString(text[written:value])
		out.WriteString("[redacted]")
		written = end
		i = end
	}
	if written == 0 {
		return text
	}
	out.WriteString(text[written:])
	return out.String()
}

func redactAddresses(text string) string {
	var out strings.Builder
	written := 0
	for i := 0; i < len(text); {
		if i+3 <= len(text) && text[i:i+3] == "://" {
			start := i + 3
			end := start
			for end < len(text) && !strings.ContainsRune("/ \t\r\n\"'?#", rune(text[end])) {
				end++
			}
			if at := strings.LastIndexByte(text[start:end], '@'); at >= 0 {
				out.WriteString(text[written:start])
				out.WriteString("[redacted]@")
				written = start + at + 1
			}
			i = end
			continue
		}
		if text[i] == '@' {
			start := i
			for start > written && emailByte(text[start-1]) {
				start--
			}
			end := i + 1
			for end < len(text) && emailByte(text[end]) {
				end++
			}
			if start < i && strings.Contains(text[i+1:end], ".") {
				out.WriteString(text[written:start])
				out.WriteString("[redacted]")
				written = end
			}
			i = end
			continue
		}
		i++
	}
	if written == 0 {
		return text
	}
	out.WriteString(text[written:])
	return out.String()
}

func emailByte(c byte) bool { return keyByte(c) || c == '.' || c == '+' || c == '%' }
