package modpack

import "strings"

// DefaultCurseForgeKey is the built-in application key, replaceable with
// -ldflags -X. A configured key takes precedence.
var DefaultCurseForgeKey = "$2a$10$OTQO7DcV6b.aSHcy1GWfUurQ1NntvnNhe1xUe5CRZOpQaFmCJYjNO"

func CurseForgeKey(configured string) string {
	if value := strings.TrimSpace(configured); value != "" {
		return value
	}
	return DefaultCurseForgeKey
}
