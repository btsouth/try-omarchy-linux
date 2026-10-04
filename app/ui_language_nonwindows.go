//go:build !windows

package main

import (
	"os"
	"strings"
)

func preferredUILanguages() []string { return localeUILanguages(os.Getenv) }

// LC_ALL overrides the message locale, which overrides LANG. GNU LANGUAGE
// orders message languages within that locale, except in the C/POSIX locale.
// Flatpak passes these locale variables through to the launcher.
func localeUILanguages(getenv func(string) string) []string {
	locale := ""
	for _, name := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if locale = strings.TrimSpace(getenv(name)); locale != "" {
			break
		}
	}
	tag := localeLanguageTag(locale)
	if strings.EqualFold(tag, "C") || strings.EqualFold(tag, "POSIX") {
		return []string{"en"}
	}
	var preferred []string
	for _, language := range strings.Split(getenv("LANGUAGE"), ":") {
		if language = localeLanguageTag(language); language != "" {
			if strings.EqualFold(language, "C") || strings.EqualFold(language, "POSIX") {
				language = "en"
			}
			preferred = append(preferred, language)
		}
	}
	if tag != "" {
		preferred = append(preferred, tag)
	}
	return preferred
}

func localeLanguageTag(locale string) string {
	locale = strings.TrimSpace(locale)
	locale, _, _ = strings.Cut(locale, "@")
	locale, _, _ = strings.Cut(locale, ".")
	return strings.ReplaceAll(locale, "_", "-")
}
