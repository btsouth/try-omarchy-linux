package main

import (
	"encoding/json"
	"os"
	"regexp"
)

// Labels arrive already resolved by the launcher, including English fallback
// and pseudo-localization. GTK has no second catalog or language policy.
var uiLabels = loadUILabels(os.Getenv("TRY_OMARCHY_UI_MESSAGES"))
var uiPlaceholder = regexp.MustCompile(`\{[a-z][a-z0-9_]*\}`)

func loadUILabels(encoded string) map[string]string {
	if encoded == "" {
		return nil
	}
	var labels map[string]string
	if err := json.Unmarshal([]byte(encoded), &labels); err != nil {
		panic(err)
	}
	return labels
}

func uiText(key string) string {
	if label := uiLabels[key]; label != "" {
		return label
	}
	panic("missing launcher UI label: " + key)
}

func uiTextWith(key string, values map[string]string) string {
	return uiPlaceholder.ReplaceAllStringFunc(uiText(key), func(placeholder string) string {
		name := placeholder[1 : len(placeholder)-1]
		value, ok := values[name]
		if !ok {
			panic("missing launcher UI value: " + name)
		}
		return value
	})
}
