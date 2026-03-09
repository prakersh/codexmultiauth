package main

import (
	"fmt"
	"io"
	"os"
	"regexp"

	"github.com/prakersh/codexmultiauth/cmd"
)

var execute = cmd.Execute
var exit = os.Exit
var stderr io.Writer = os.Stderr

var sensitiveErrorPatterns = []struct {
	pattern *regexp.Regexp
	value   string
}{
	{pattern: regexp.MustCompile(`pass:[^\s"']+`), value: "pass:[REDACTED]"},
	{pattern: regexp.MustCompile(`"access_token":"[^"]*"`), value: `"access_token":"[REDACTED]"`},
	{pattern: regexp.MustCompile(`"refresh_token":"[^"]*"`), value: `"refresh_token":"[REDACTED]"`},
	{pattern: regexp.MustCompile(`"id_token":"[^"]*"`), value: `"id_token":"[REDACTED]"`},
	{pattern: regexp.MustCompile(`"OPENAI_API_KEY":"[^"]*"`), value: `"OPENAI_API_KEY":"[REDACTED]"`},
}

func sanitizeErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	message := err.Error()
	for _, replacement := range sensitiveErrorPatterns {
		message = replacement.pattern.ReplaceAllString(message, replacement.value)
	}
	return message
}

func main() {
	if err := execute(); err != nil {
		fmt.Fprintf(stderr, "Error: %s\n", sanitizeErrorMessage(err))
		exit(1)
	}
}
