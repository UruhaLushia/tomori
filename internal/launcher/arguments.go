package launcher

import (
	"errors"
	"strings"

	"github.com/UruhaLushia/tomori/locales"
)

// SplitArguments recognizes quotes without involving a shell or expanding variables.
func SplitArguments(s string) ([]string, error) {
	var args []string
	var word strings.Builder
	var quote byte
	started := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '\\' && quote != '\'' && i+1 < len(s) {
			next := s[i+1]
			if next == '\\' || next == '"' || (quote == 0 && (next == '\'' || next == ' ')) {
				word.WriteByte(next)
				started = true
				i++
				continue
			}
		}
		if quote != 0 {
			if ch == quote {
				quote = 0
			} else {
				word.WriteByte(ch)
			}
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
			started = true
		case ' ', '\t', '\r', '\n':
			if started {
				args = append(args, word.String())
				word.Reset()
				started = false
			}
		default:
			word.WriteByte(ch)
			started = true
		}
	}
	if quote != 0 {
		return nil, errors.New(locales.UnclosedQuoteInLaunchArguments())
	}
	if started {
		args = append(args, word.String())
	}
	return args, nil
}
