package model

import (
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"pgregory.net/rapid"
)

// NFR-U4-TEST-02: arbitrary Go strings include invalid UTF-8 byte sequences.
// Every constructed diagnostic value must remain bounded, control-free, and
// valid UTF-8 regardless of that input.
func TestReviewU4DiagValueProperty(t *testing.T) {
	rapid.Check(t, func(t *rapid.T) {
		rawBytes := rapid.SliceOfN(rapid.Byte(), 0, 1024).Draw(t, "raw_bytes")
		raw := string(rawBytes)
		got := NewDiagValue(raw).String()

		if !utf8.ValidString(got) {
			t.Fatalf("NewDiagValue returned invalid UTF-8 for bytes %x: %x", rawBytes, []byte(got))
		}
		gotRunes := []rune(got)
		if len(gotRunes) > maxDiagValueRunes+1 {
			t.Fatalf("output contains %d runes, want at most %d", len(gotRunes), maxDiagValueRunes+1)
		}
		for _, r := range gotRunes {
			if unicode.IsControl(r) {
				t.Fatalf("output contains control rune %U: %q", r, got)
			}
		}

		if utf8.RuneCountInString(raw) > maxDiagValueRunes {
			if len(gotRunes) != maxDiagValueRunes+1 || gotRunes[len(gotRunes)-1] != '\u2026' {
				t.Fatalf("truncated output must contain %d runes ending in ellipsis: %q", maxDiagValueRunes+1, got)
			}
		}
	})
}

func TestReviewU4DiagValueSanitizesBeforeRuneTruncation(t *testing.T) {
	raw := strings.Repeat("界", maxDiagValueRunes-1) + "\n" + "終"
	got := NewDiagValue(raw).String()
	want := strings.Repeat("界", maxDiagValueRunes-1) + "\ufffd\u2026"
	if got != want {
		t.Fatalf("NewDiagValue boundary result = %q, want %q", got, want)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("multi-byte boundary produced invalid UTF-8: %x", []byte(got))
	}

	invalid := string([]byte{'a', 0xff, 'b', 0xfe})
	if got := NewDiagValue(invalid).String(); got != "a\ufffdb\ufffd" || !utf8.ValidString(got) {
		t.Fatalf("invalid UTF-8 was not replaced safely: %q (%x)", got, []byte(got))
	}
}

func TestReviewU4DiagnosticMessageBackstop(t *testing.T) {
	report := &SICMappingImportReport{}
	report.AddError(2, "Category_Name", "category_not_found", strings.Repeat("界", maxDiagMessageRunes+20))
	if len(report.Errors) != 1 {
		t.Fatalf("AddError retained %d diagnostics, want 1", len(report.Errors))
	}
	got := report.Errors[0].Message
	if utf8.RuneCountInString(got) != maxDiagMessageRunes+1 || !strings.HasSuffix(got, "\u2026") {
		t.Fatalf("message backstop produced %d runes without the required ellipsis", utf8.RuneCountInString(got))
	}
	if !utf8.ValidString(got) {
		t.Fatalf("message backstop split a multi-byte rune: %x", []byte(got))
	}
}
