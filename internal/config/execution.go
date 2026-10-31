package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/UruhaLushia/tomori/locales"
)

type Execution struct {
	Enabled        bool   `json:"enabled"`
	Before         string `json:"before,omitempty"`
	After          string `json:"after,omitempty"`
	Environment    string `json:"environment,omitempty"`
	TimeoutSeconds int    `json:"timeoutSeconds"`
}

func (e Execution) Timeout() int {
	if e.TimeoutSeconds <= 0 {
		return 60
	}
	return min(e.TimeoutSeconds, 3600)
}

func (e Execution) Validate() error {
	if !e.Enabled {
		return nil
	}
	if len(e.Before) > 16<<10 || len(e.After) > 16<<10 || strings.ContainsRune(e.Before, 0) || strings.ContainsRune(e.After, 0) {
		return errors.New(locales.LaunchCommandIsTooLongOrContainsInvalidCharacters())
	}
	if e.TimeoutSeconds < 0 || e.TimeoutSeconds > 3600 {
		return errors.New(locales.CommandTimeoutMustBe13600Seconds())
	}
	_, err := e.Variables()
	return err
}

func (e Execution) Variables() ([]string, error) {
	if len(e.Environment) > 32<<10 {
		return nil, errors.New(locales.EnvironmentVariablesExceed32KiB())
	}
	var variables []string
	lineNumber := 0
	for line := range strings.SplitSeq(e.Environment, "\n") {
		lineNumber++
		line = strings.TrimSuffix(line, "\r")
		if trimmed := strings.TrimSpace(line); trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		rawKey, value, ok := strings.Cut(line, "=")
		key := strings.TrimSpace(rawKey)
		valid := ok && key != "" && !strings.ContainsRune(value, 0)
		for i := 0; valid && i < len(key); i++ {
			b := key[i]
			valid = b == '_' || b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || i > 0 && b >= '0' && b <= '9'
		}
		if !valid {
			return nil, fmt.Errorf(locales.InvalidEnvironmentVariableOnLineUseNameValue(), lineNumber)
		}
		if strings.HasPrefix(strings.ToUpper(key), "TOMORI_") {
			return nil, errors.New(locales.TheTomoriPrefixIsReserved())
		}
		if len(variables) == 128 {
			return nil, errors.New(locales.UpTo128EnvironmentVariablesAreAllowed())
		}
		if key != rawKey {
			line = key + "=" + value
		}
		variables = append(variables, line)
	}
	return variables, nil
}

func (e Execution) Secrets() []string {
	if len(e.Environment) > 32<<10 {
		return nil
	}
	var secrets []string
	for line := range strings.SplitSeq(e.Environment, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "#") {
			continue
		}
		key, value, ok := strings.Cut(strings.TrimSuffix(line, "\r"), "=")
		if !ok || value == "" {
			continue
		}
		key = strings.ToUpper(strings.TrimSpace(key))
		sensitive := strings.Contains(key, "TOKEN") || strings.Contains(key, "SECRET") || strings.Contains(key, "PASS") || strings.Contains(key, "KEY") || strings.Contains(key, "AUTH") || strings.Contains(key, "COOKIE") || strings.Contains(key, "CREDENTIAL")
		if sensitive || len(value) >= 4 {
			secrets = append(secrets, value)
			if len(secrets) == 128 {
				break
			}
		}
	}
	return secrets
}
