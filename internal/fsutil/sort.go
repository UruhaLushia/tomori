package fsutil

import (
	"cmp"
	"unicode"
	"unicode/utf8"
)

func CompareFold(a, b string) int {
	for len(a) > 0 && len(b) > 0 {
		x, nx := utf8.DecodeRuneInString(a)
		y, ny := utf8.DecodeRuneInString(b)
		x, y = unicode.ToLower(x), unicode.ToLower(y)
		if x != y {
			return cmp.Compare(x, y)
		}
		a, b = a[nx:], b[ny:]
	}
	return cmp.Compare(len(a), len(b))
}
