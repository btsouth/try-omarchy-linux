//go:build linux

package main

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Check both modules: Linux's keys exist in the one source catalog, and every
// control label used by GTK is supplied before it creates any widgets.
func TestLinuxUIMessageKeysExistAndAreForwarded(t *testing.T) {
	catalogs, err := readUICatalogs(uiLocaleFiles)
	if err != nil {
		t.Fatal(err)
	}
	labels := linuxUILabels()
	usedByGTK := map[string]bool{}
	for _, pattern := range []string{"*_linux.go", "../linux-ui/*.go"} {
		files, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") || filepath.Base(path) == "ui_messages.go" {
				continue
			}
			fs := token.NewFileSet()
			file, err := parser.ParseFile(fs, path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				function, ok := call.Fun.(*ast.Ident)
				if !ok || (function.Name != "uiText" && function.Name != "uiTextWith" && function.Name != "uiTemplate") || len(call.Args) == 0 {
					return true
				}
				literal, ok := call.Args[0].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					t.Errorf("%s: key must be literal", fs.Position(call.Pos()))
					return true
				}
				key, err := strconv.Unquote(literal.Value)
				if err != nil {
					t.Fatal(err)
				}
				if catalogs["en"][key] == "" {
					t.Errorf("%s: unknown Linux message %q", fs.Position(call.Pos()), key)
				}
				if strings.HasPrefix(path, "../linux-ui/") {
					usedByGTK[key] = true
					if labels[key] == "" {
						t.Errorf("GTK label %q is not forwarded", key)
					}
				}
				return true
			})
		}
	}
	for key := range labels {
		if !usedByGTK[key] {
			t.Errorf("forwarded label %q is unused by GTK", key)
		}
	}
	// Linux limits each environment entry to 128 KiB. Leave room for longer
	// translations and ensure the helper's transport cannot exceed that limit.
	for _, language := range []string{"en", "zh-Hans", "ko", uiPseudoLanguage} {
		translator := uiTranslator{language: language, catalogs: catalogs}
		resolved := map[string]string{}
		for key := range labels {
			resolved[key] = translator.text(key)
		}
		encoded, err := json.Marshal(resolved)
		if err != nil {
			t.Fatal(err)
		}
		if len(encoded) > 120*1024 {
			t.Errorf("%s label transport is too large: %d bytes", language, len(encoded))
		}
	}
}
