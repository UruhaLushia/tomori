package main

import (
	"os"
	"strings"

	"github.com/egoist/mygo/i18n"
)

func Execute() error {
	setLocaleFromEnvironment()
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
