package parser

import (
	"strings"
	"testing"

	"github.com/oronno/privateledger/internal/model"
)

// sicBody builds a single-transaction SGML statement, optionally carrying a
// <SIC> element. It mirrors the shape used by the existing parser fixtures.
func sicBody(sicElement string) string {
	return `<OFX>
<SIGNONMSGSRSV1><SONRS>
<STATUS><CODE>0<SEVERITY>INFO</STATUS>
<DTSERVER>20251215120000[-5:EST]
<LANGUAGE>ENG
</SONRS></SIGNONMSGSRSV1>
<BANKMSGSRSV1><STMTTRNRS>
<TRNUID>1
<STATUS><CODE>0<SEVERITY>INFO</STATUS>
<STMTRS>
<CURDEF>CAD
<BANKACCTFROM><BANKID>004<ACCTID>123456<ACCTTYPE>CHECKING</BANKACCTFROM>
<BANKTRANLIST>
<DTSTART>20251201120000[-5:EST]
<DTEND>20251215120000[-5:EST]
<STMTTRN><TRNTYPE>DEBIT<DTPOSTED>20251202120000[-5:EST]<TRNAMT>-12.34<FITID>SIC1` +
		sicElement + `<NAME>CAFE NOIR</STMTTRN>
</BANKTRANLIST>
<LEDGERBAL><BALAMT>100.00<DTASOF>20251215120000[-5:EST]</LEDGERBAL>
</STMTRS></STMTTRNRS></BANKMSGSRSV1>
</OFX>`
}

// TestParseOFXFile_SICExtraction covers FR1 / US-01: a present <SIC> is stored
// canonically, and absent or zero <SIC> leaves the value unset (BR-SIC-01).
func TestParseOFXFile_SICExtraction(t *testing.T) {
	tests := []struct {
		name    string
		element string
		want    *model.SICCode // nil means the transaction must carry no SIC
	}{
		{name: "no SIC element", element: "", want: nil},
		{name: "explicit zero SIC", element: "<SIC>0", want: nil},
		{name: "zero padded zero SIC", element: "<SIC>0000", want: nil},
		{name: "typical four digit SIC", element: "<SIC>5812", want: sicPtr("5812")},
		{name: "minimum positive SIC", element: "<SIC>1", want: sicPtr("1")},
		{name: "leading zeros are canonicalized", element: "<SIC>0005812", want: sicPtr("5812")},
		{name: "surrounding whitespace tolerated", element: "<SIC> 5812 ", want: sicPtr("5812")},
		{name: "max int64 SIC", element: "<SIC>9223372036854775807", want: sicPtr("9223372036854775807")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := NewOFXParser().ParseOFXFile(strings.NewReader(sgmlHeader+sicBody(tt.element)), 3)
			if err != nil {
				t.Fatalf("ParseOFXFile() returned error: %v", err)
			}
			if len(result.Transactions) != 1 {
				t.Fatalf("expected 1 transaction, got %d", len(result.Transactions))
			}
			txn := result.Transactions[0]

			if tt.want == nil {
				if txn.SICCode != nil {
					t.Fatalf("expected no SIC, got %q", *txn.SICCode)
				}
				return
			}
			if txn.SICCode == nil {
				t.Fatalf("expected SIC %q, got nil", *tt.want)
			}
			if *txn.SICCode != *tt.want {
				t.Errorf("expected SIC %q, got %q", *tt.want, *txn.SICCode)
			}
		})
	}
}

// TestParseOFXFile_SICDoesNotDisturbExistingFields is the US-01 / BR-TXN-03
// regression guard: adding SIC must not change any other parsed field.
func TestParseOFXFile_SICDoesNotDisturbExistingFields(t *testing.T) {
	withoutSIC, err := NewOFXParser().ParseOFXFile(strings.NewReader(sgmlHeader+sicBody("")), 3)
	if err != nil {
		t.Fatalf("parse without SIC: %v", err)
	}
	withSIC, err := NewOFXParser().ParseOFXFile(strings.NewReader(sgmlHeader+sicBody("<SIC>5812")), 3)
	if err != nil {
		t.Fatalf("parse with SIC: %v", err)
	}

	a, b := withoutSIC.Transactions[0], withSIC.Transactions[0]
	if a.FitID != b.FitID || a.TrnType != b.TrnType || a.Amount != b.Amount ||
		a.TransactionDetails != b.TransactionDetails || a.TransactionType != b.TransactionType ||
		!a.DatePosted.Equal(b.DatePosted) || a.AccountID != b.AccountID {
		t.Errorf("SIC presence changed other transaction fields:\n without=%+v\n with=%+v", a, b)
	}
	if withoutSIC.Currency != withSIC.Currency || withoutSIC.AccountType != withSIC.AccountType {
		t.Errorf("SIC presence changed statement-level fields")
	}
}

// TestParseOFXFile_SICFreeFileRegression confirms NFR-U1-COMP-01: an entirely
// SIC-free file parses exactly as it did before UOW-1.
func TestParseOFXFile_SICFreeFileRegression(t *testing.T) {
	result, err := NewOFXParser().ParseOFXFile(strings.NewReader(sgmlHeader+sgmlBody), 7)
	if err != nil {
		t.Fatalf("ParseOFXFile() returned error: %v", err)
	}
	assertParsedFixture(t, result)
	for i, txn := range result.Transactions {
		if txn.SICCode != nil {
			t.Errorf("transaction %d in a SIC-free file carries SIC %q", i, *txn.SICCode)
		}
	}
}

// TestParseOFXFile_NonNumericSICFailsWholeFile pins BR-TXN-04 / TD-U1-02:
// UOW-1 deliberately does not recover a non-numeric <SIC>; ofxgo rejects the
// whole file, which is the documented pre-existing boundary.
func TestParseOFXFile_NonNumericSICFailsWholeFile(t *testing.T) {
	for _, sic := range []string{"<SIC>ABCD", "<SIC>58-12", "<SIC>58.12"} {
		t.Run(sic, func(t *testing.T) {
			_, err := NewOFXParser().ParseOFXFile(strings.NewReader(sgmlHeader+sicBody(sic)), 3)
			if err == nil {
				t.Fatalf("expected the whole file to be rejected for %q", sic)
			}
			if !strings.Contains(err.Error(), "failed to parse OFX file") {
				t.Errorf("expected a whole-file parse error, got %q", err.Error())
			}
		})
	}
}

// TestParseOFXFile_SICOverflowIsWholeFileFailure documents that a <SIC> beyond
// int64 also fails inside ofxgo rather than reaching UOW-1 normalization.
func TestParseOFXFile_SICOverflowIsWholeFileFailure(t *testing.T) {
	_, err := NewOFXParser().ParseOFXFile(strings.NewReader(sgmlHeader+sicBody("<SIC>9223372036854775808")), 3)
	if err == nil {
		t.Fatal("expected an error for an int64-overflow SIC")
	}
	t.Logf("overflow SIC rejected at the ofxgo boundary: %v", err)
}

// TestParseOFXFile_NegativeSICRemainsUnset covers the sign boundary: ofxgo can
// hold a negative int64, and BR-SIC-01 requires it to produce no SIC value.
func TestParseOFXFile_NegativeSICRemainsUnset(t *testing.T) {
	result, err := NewOFXParser().ParseOFXFile(strings.NewReader(sgmlHeader+sicBody("<SIC>-5812")), 3)
	if err != nil {
		t.Skipf("ofxgo rejects a negative SIC at the parse boundary: %v", err)
	}
	if len(result.Transactions) != 1 {
		t.Fatalf("expected 1 transaction, got %d", len(result.Transactions))
	}
	if result.Transactions[0].SICCode != nil {
		t.Errorf("a negative SIC must not produce a stored SIC value, got %q", *result.Transactions[0].SICCode)
	}
}

// TestParseOFXFile_MultipleTransactionsMixedSIC verifies per-transaction
// isolation so one SIC value cannot bleed into a neighbouring transaction.
func TestParseOFXFile_MultipleTransactionsMixedSIC(t *testing.T) {
	body := strings.Replace(sgmlBody,
		"<STMTTRN><TRNTYPE>CREDIT<DTPOSTED>20251203120000[-5:EST]<TRNAMT>500.00<FITID>AAA2<NAME>PAYROLL DEPOSIT</STMTTRN>",
		"<STMTTRN><TRNTYPE>CREDIT<DTPOSTED>20251203120000[-5:EST]<TRNAMT>500.00<FITID>AAA2<SIC>7011<NAME>PAYROLL DEPOSIT</STMTTRN>\n"+
			"<STMTTRN><TRNTYPE>DEBIT<DTPOSTED>20251204120000[-5:EST]<TRNAMT>-1.00<FITID>AAA3<NAME>NO SIC HERE</STMTTRN>", 1)

	result, err := NewOFXParser().ParseOFXFile(strings.NewReader(sgmlHeader+body), 1)
	if err != nil {
		t.Fatalf("ParseOFXFile() returned error: %v", err)
	}
	if len(result.Transactions) != 3 {
		t.Fatalf("expected 3 transactions, got %d", len(result.Transactions))
	}
	want := []string{"", "7011", ""}
	for i, txn := range result.Transactions {
		got := ""
		if txn.SICCode != nil {
			got = string(*txn.SICCode)
		}
		if got != want[i] {
			t.Errorf("transaction %d (%s): SIC = %q, want %q", i, txn.FitID, got, want[i])
		}
	}
}

func sicPtr(s string) *model.SICCode {
	c := model.SICCode(s)
	return &c
}
