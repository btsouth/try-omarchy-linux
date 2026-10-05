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
	"unicode"
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
				if selector, selected := call.Fun.(*ast.SelectorExpr); selected && selector.Sel.Name == "setCatalogStatus" {
					function, ok = selector.Sel, true
				}
				if !ok || (function.Name != "uiText" && function.Name != "uiTextWith" && function.Name != "uiTemplate" && function.Name != "setCatalogStatus") || len(call.Args) == 0 {
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

// Guard the screen-state fields and GTK control sinks as well as their keys.
// Protocol IDs, commands, diagnostic logging and numeric units are not text
// owned by these sinks. Details interpolated into catalog messages may still
// come from the OS or an internal error.
func TestLinuxScreenTextComesFromTheCatalog(t *testing.T) {
	fields := map[string]bool{"Status": true, "Detail": true, "Title": true, "Heading": true, "Label": true, "Primary": true, "Secondary": true, "Notice": true, "Headline": true, "ErrorTitle": true}
	sinks := map[string]bool{"setStatus": true, "fatal": true, "showLinuxRuntimeError": true, "showLinuxRuntimeErrorWindow": true, "showSettingsProblem": true,
		"NewLabel": true, "NewButtonWithLabel": true, "NewCheckButtonWithLabel": true, "NewExpander": true, "NewDropDownFromStrings": true,
		"SetTitle": true, "SetText": true, "SetLabel": true, "SetTooltipText": true, "SetAcceptLabel": true, "SetButtonLabel": true,
		"named": true, "formHelp": true, "formField": true, "formLabel": true, "beginGroup": true, "entry": true}
	check := func(fs *token.FileSet, expression ast.Expr) {
		for _, literal := range textLiterals(expression) {
			text, _ := strconv.Unquote(literal.Value)
			if text == "%s" || text == "" {
				continue
			}
			if strings.IndexFunc(text, unicode.IsLetter) >= 0 {
				t.Errorf("%s: screen text %q must come from the catalog", fs.Position(literal.Pos()), text)
			}
		}
	}
	for _, pattern := range []string{"*_linux.go", "../linux-ui/*.go"} {
		files, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range files {
			if strings.HasSuffix(path, "_test.go") {
				continue
			}
			fs := token.NewFileSet()
			file, err := parser.ParseFile(fs, path, nil, 0)
			if err != nil {
				t.Fatal(err)
			}
			ast.Inspect(file, func(node ast.Node) bool {
				switch n := node.(type) {
				case *ast.KeyValueExpr:
					if field, ok := n.Key.(*ast.Ident); ok && fields[field.Name] {
						check(fs, n.Value)
					}
				case *ast.CallExpr:
					name := ""
					switch fn := n.Fun.(type) {
					case *ast.Ident:
						name = fn.Name
					case *ast.SelectorExpr:
						name = fn.Sel.Name
					}
					if sinks[name] {
						for index, argument := range n.Args {
							// beginGroup's second argument is a stable page ID.
							if name == "beginGroup" && index == 1 {
								continue
							}
							check(fs, argument)
						}
					}
				}
				return true
			})
		}
	}
}
