package main

import (
	"os"
	"strings"

	"github.com/egoist/mygo/i18n"
)

func Execute() error {
	if len(os.Args) > 1 && !strings.HasPrefix(strings.ToLower(os.Args[1]), "tomori:") {
		setLocaleFromEnvironment()
	}
	return NewRootCommand().Execute()
}

func setLocaleFromEnvironment() {
	for _, key := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if value := os.Getenv(key); value != "" {
			tag, _, _ := strings.Cut(value, ".")
			i18n.SetLocaleFunc(func() string { return strings.ReplaceAll(tag, "_", "-") })
			return
		}
	}
}
