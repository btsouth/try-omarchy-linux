//go:build !windows

package main

import "testing"

func TestLocaleUILanguageSelection(t *testing.T) {
	catalogs, err := readUICatalogs(uiLocaleFiles)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"unset", nil, "en"},
		{"Simplified China", map[string]string{"LANG": "zh_CN.UTF-8"}, "zh-Hans"},
		{"Simplified Singapore", map[string]string{"LANG": "zh_SG.utf8"}, "zh-Hans"},
		{"Traditional Taiwan", map[string]string{"LANG": "zh_TW.UTF-8"}, "en"},
		{"Traditional Hong Kong", map[string]string{"LANG": "zh_HK.UTF-8"}, "en"},
		{"Traditional Macau", map[string]string{"LANG": "zh_MO.UTF-8"}, "en"},
		{"unspecified Chinese", map[string]string{"LANG": "zh.UTF-8"}, "en"},
		{"Korean", map[string]string{"LANG": "ko_KR.UTF-8"}, "ko"},
		{"unsupported", map[string]string{"LANG": "ja_JP.UTF-8"}, "en"},
		{"message locale", map[string]string{"LC_MESSAGES": "ko_KR.UTF-8", "LANG": "zh_CN.UTF-8"}, "ko"},
		{"all locale", map[string]string{"LC_ALL": "en_US.UTF-8", "LC_MESSAGES": "ko_KR.UTF-8", "LANG": "zh_CN.UTF-8"}, "en"},
		{"language priority", map[string]string{"LANGUAGE": "fr:ko:zh_CN", "LANG": "zh_CN.UTF-8"}, "ko"},
		{"traditional priority", map[string]string{"LANGUAGE": "zh_TW:ko", "LANG": "en_US.UTF-8"}, "ko"},
		{"English priority", map[string]string{"LANGUAGE": "en:ko", "LANG": "ko_KR.UTF-8"}, "en"},
		{"empty priority items", map[string]string{"LANGUAGE": ": ko_KR.UTF-8 :", "LANG": "en_US.UTF-8"}, "ko"},
		{"C ignores LANGUAGE", map[string]string{"LC_ALL": "C", "LANGUAGE": "ko", "LANG": "zh_CN.UTF-8"}, "en"},
		{"C UTF8 ignores LANGUAGE", map[string]string{"LANG": "C.UTF-8", "LANGUAGE": "ko"}, "en"},
		{"POSIX ignores LANGUAGE", map[string]string{"LANG": "POSIX", "LANGUAGE": "ko"}, "en"},
		{"modifier", map[string]string{"LANG": "ko_KR.UTF-8@euro"}, "ko"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			preferred := localeUILanguages(func(key string) string { return tc.env[key] })
			if got := launcherUILanguage("", preferred, catalogs); got != tc.want {
				t.Fatalf("%v selected %q, want %q", tc.env, got, tc.want)
			}
		})
	}
}
