package main

import (
	"encoding/json"
	"os"
	"testing"
)

// Standalone helper tests use the source catalog, never a duplicate fixture.
// Production receives the resolved subset from its launcher before startup.
func init() {
	data, err := os.ReadFile("../app/ui-locales/en.json")
	if err != nil {
		panic(err)
	}
	if err := json.Unmarshal(data, &uiLabels); err != nil {
		panic(err)
	}
	for key, label := range uiLabels {
		if len(key) > 6 && key[len(key)-6:] == "@linux" {
			uiLabels[key[:len(key)-6]] = label
		}
	}
}

func TestResolvedUILabelsAndPlaceholders(t *testing.T) {
	previous := uiLabels
	defer func() { uiLabels = previous }()
	uiLabels = loadUILabels(`{"button":"닫기","prompt":"Remove {path}? {error}"}`)
	if got := uiText("button"); got != "닫기" {
		t.Fatal(got)
	}
	if got := uiTextWith("prompt", map[string]string{"path": "/tmp/{error}", "error": "detail"}); got != "Remove /tmp/{error}? detail" {
		t.Fatal(got)
	}
}
