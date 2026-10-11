package model

import (
	"strconv"
	"strings"
	"testing"
)

// TestParseSICCode_Boundaries derives its expectations from the approved
// functional design (business-logic-model.md "Flow 3 - SIC Normalization and
// Validation" and business-rules.md BR-SIC-02/BR-SIC-03).
func TestParseSICCode_Boundaries(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		want    SICCode
		wantErr bool
	}{
		// Documented examples from business-logic-model.md Flow 3.
		{name: "leading zeros and whitespace", raw: " 0111 ", want: "111"},
		{name: "minimum accepted value", raw: "1", want: "1"},
		{name: "all zeros rejected", raw: "000", wantErr: true},
		{name: "non digit rejected", raw: "12A4", wantErr: true},
		{name: "max int64 accepted", raw: "9223372036854775807", want: "9223372036854775807"},
		{name: "int64 overflow rejected", raw: "9223372036854775808", wantErr: true},

		// Boundary and wrong-type inputs.
		{name: "empty rejected", raw: "", wantErr: true},
		{name: "whitespace only rejected", raw: "   ", wantErr: true},
		{name: "single zero rejected", raw: "0", wantErr: true},
		{name: "zero padded max accepted", raw: "0009223372036854775807", want: "9223372036854775807"},
		{name: "twenty digit value rejected", raw: "12345678901234567890", wantErr: true},
		{name: "nineteen digit overflow rejected", raw: "9999999999999999999", wantErr: true},
		{name: "negative sign rejected", raw: "-1", wantErr: true},
		{name: "positive sign rejected", raw: "+1", wantErr: true},
		{name: "decimal point rejected", raw: "1.0", wantErr: true},
		{name: "internal space rejected", raw: "11 11", wantErr: true},
		{name: "tab and newline trimmed", raw: "\t 5812 \n", want: "5812"},
		{name: "arabic indic digits rejected", raw: "٥٨١٢", wantErr: true},
		{name: "fullwidth digits rejected", raw: "５８１２", wantErr: true},
		{name: "nul byte rejected", raw: "58\x0012", wantErr: true},
		{name: "typical four digit sic", raw: "5812", want: "5812"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParseSICCode(tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ParseSICCode(%q) = %q, want error", tc.raw, got)
				}
				if got != "" {
					t.Errorf("ParseSICCode(%q) returned %q alongside error, want empty", tc.raw, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSICCode(%q) unexpected error: %v", tc.raw, err)
			}
			if got != tc.want {
				t.Errorf("ParseSICCode(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestParseSICCode_ErrorsAreSafe enforces BR-ERR-01 / NFR-U1-SEC-02: validation
// errors must not echo the rejected payload back into logs.
func TestParseSICCode_ErrorsAreSafe(t *testing.T) {
	secret := "4111111111111111ACCOUNT"
	_, err := ParseSICCode(secret)
	if err == nil {
		t.Fatalf("expected an error for %q", secret)
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("validation error leaks the rejected input: %q", err.Error())
	}
}

// TestSICCodeEquivalenceClasses enforces BR-SIC-05: equivalent numeric inputs
// canonicalize to one identity, so 0111 and 111 are duplicates.
func TestSICCodeEquivalenceClasses(t *testing.T) {
	group := []string{"111", "0111", " 00111 ", "000000111"}
	var canonical SICCode
	for i, raw := range group {
		got, err := ParseSICCode(raw)
		if err != nil {
			t.Fatalf("ParseSICCode(%q): %v", raw, err)
		}
		if i == 0 {
			canonical = got
			continue
		}
		if got != canonical {
			t.Errorf("ParseSICCode(%q) = %q, want %q so duplicates collapse", raw, got, canonical)
		}
	}
}

func TestNormalizeSICCode(t *testing.T) {
	cases := []struct{ raw, want string }{
		{" 0111 ", "111"},
		{"000", "0"},
		{"", "0"},
		{"5812", "5812"},
		{"0", "0"},
	}
	for _, tc := range cases {
		if got := NormalizeSICCode(tc.raw); got != tc.want {
			t.Errorf("NormalizeSICCode(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

// TestSICMappingDisplayDescription covers the FR10 description fallback that
// UOW-1 stores and later units display.
func TestSICMappingDisplayDescription(t *testing.T) {
	cases := []struct {
		name       string
		mapping    *SICMapping
		want       string
		wantHasCat bool
	}{
		{name: "nil mapping", mapping: nil, want: ""},
		{name: "description wins", mapping: &SICMapping{Description: "Eating Places", DescriptionDetail: "detail"}, want: "Eating Places"},
		{name: "detail fallback", mapping: &SICMapping{DescriptionDetail: "detail"}, want: "detail"},
		{name: "both empty", mapping: &SICMapping{}, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.mapping.DisplayDescription(); got != tc.want {
				t.Errorf("DisplayDescription() = %q, want %q", got, tc.want)
			}
			if got := tc.mapping.HasCategory(); got != tc.wantHasCat {
				t.Errorf("HasCategory() = %v, want %v", got, tc.wantHasCat)
			}
		})
	}

	id := 7
	m := NewSICMapping("5812", "d", "dd", &id)
	if !m.HasCategory() {
		t.Errorf("NewSICMapping with category id should report HasCategory")
	}
	if m.SICCode != "5812" || m.Description != "d" || m.DescriptionDetail != "dd" || *m.CategoryID != 7 {
		t.Errorf("NewSICMapping did not preserve its inputs: %+v", m)
	}
	if m.CreatedAt.IsZero() {
		t.Errorf("NewSICMapping should stamp CreatedAt")
	}
}

// TestParseSICCode_ExhaustiveInt64Boundary walks the exact accept/reject edge
// of the approved positive-int64 domain.
func TestParseSICCode_ExhaustiveInt64Boundary(t *testing.T) {
	const maxInt64 = "9223372036854775807"
	for delta := -2; delta <= 2; delta++ {
		big, ok := new(bigDecimal).setString(maxInt64)
		if !ok {
			t.Fatal("fixture parse failed")
		}
		raw := big.addSmall(delta).String()
		_, err := ParseSICCode(raw)
		accepted := err == nil
		want := delta <= 0
		if accepted != want {
			t.Errorf("ParseSICCode(%q) accepted=%v, want %v", raw, accepted, want)
		}
	}
}

// bigDecimal is a tiny decimal helper so the boundary test does not depend on
// int64 arithmetic that would itself overflow.
type bigDecimal struct{ digits []byte }

func (b *bigDecimal) setString(s string) (*bigDecimal, bool) {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return nil, false
		}
	}
	b.digits = []byte(s)
	return b, true
}

func (b *bigDecimal) addSmall(delta int) *bigDecimal {
	n, err := strconv.ParseInt(string(b.digits[len(b.digits)-4:]), 10, 64)
	if err != nil {
		panic(err)
	}
	n += int64(delta)
	tail := strconv.FormatInt(n, 10)
	for len(tail) < 4 {
		tail = "0" + tail
	}
	return &bigDecimal{digits: append(append([]byte{}, b.digits[:len(b.digits)-4]...), tail...)}
}

func (b *bigDecimal) String() string { return string(b.digits) }
