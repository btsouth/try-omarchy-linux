package main

import "testing"

// TestHostVariantNamesThisComputer pins that the Linux launcher shows the
// "@linux" message, even in a language that has only the Windows wording.
func TestHostVariantNamesThisComputer(t *testing.T) {
	catalogs, err := readUICatalogs(uiLocaleFiles)
	if err != nil {
		t.Fatal(err)
	}
	for _, language := range []string{"en", "ko", uiPseudoLanguage} {
		got := uiTranslator{language: language, catalogs: catalogs}.text("setup.help.dns")
		want := catalogs["en"]["setup.help.dns@linux"]
		if language == uiPseudoLanguage {
			want = pseudoLocalize(want)
		}
		if got != want {
			t.Errorf("%s: %q, want %q", language, got, want)
		}
	}
	if got := (uiTranslator{language: "en", catalogs: catalogs}).text("reclaim.status.none"); got != catalogs["en"]["reclaim.status.none"] {
		t.Errorf("a message without a variant changed: %q", got)
	}
}
