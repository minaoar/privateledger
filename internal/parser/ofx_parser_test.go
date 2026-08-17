package parser

import (
	"io"
	"strings"
	"testing"

	"github.com/oronno/privateledger/internal/model"
)

// sgmlHeader is a well-formed OFX 1.x SGML header, as emitted by most banks.
const sgmlHeader = "OFXHEADER:100\r\n" +
	"DATA:OFXSGML\r\n" +
	"VERSION:102\r\n" +
	"SECURITY:NONE\r\n" +
	"ENCODING:USASCII\r\n" +
	"CHARSET:1252\r\n" +
	"COMPRESSION:NONE\r\n" +
	"OLDFILEUID:NONE\r\n" +
	"NEWFILEUID:NONE\r\n\r\n"

// sgmlBody is a minimal bank statement body starting at the <OFX> root element.
const sgmlBody = `<OFX>
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
<STMTTRN><TRNTYPE>DEBIT<DTPOSTED>20251202120000[-5:EST]<TRNAMT>-12.34<FITID>AAA1<NAME>CAFE NOIR</STMTTRN>
<STMTTRN><TRNTYPE>CREDIT<DTPOSTED>20251203120000[-5:EST]<TRNAMT>500.00<FITID>AAA2<NAME>PAYROLL DEPOSIT</STMTTRN>
</BANKTRANLIST>
<LEDGERBAL><BALAMT>100.00<DTASOF>20251215120000[-5:EST]</LEDGERBAL>
</STMTRS></STMTTRNRS></BANKMSGSRSV1>
</OFX>`

// assertParsedFixture checks the two transactions defined in sgmlBody.
func assertParsedFixture(t *testing.T, result *ParseResult) {
	t.Helper()

	if len(result.Transactions) != 2 {
		t.Fatalf("expected 2 transactions, got %d", len(result.Transactions))
	}
	if result.Currency != "CAD" {
		t.Errorf("expected currency CAD, got %q", result.Currency)
	}
	if result.AccountType != "CHECKING" {
		t.Errorf("expected account type CHECKING, got %q", result.AccountType)
	}

	debit := result.Transactions[0]
	if debit.FitID != "AAA1" {
		t.Errorf("expected fit_id AAA1, got %q", debit.FitID)
	}
	if debit.Amount != -12.34 {
		t.Errorf("expected amount -12.34, got %v", debit.Amount)
	}
	if debit.TransactionType != model.TransactionTypeDebit {
		t.Errorf("expected debit, got %v", debit.TransactionType)
	}

	credit := result.Transactions[1]
	if credit.FitID != "AAA2" {
		t.Errorf("expected fit_id AAA2, got %q", credit.FitID)
	}
	if credit.TransactionType != model.TransactionTypeCredit {
		t.Errorf("expected credit, got %v", credit.TransactionType)
	}
}

// TestParseOFXFile_HeaderVariants covers files that should parse successfully,
// with and without the standard OFX header block.
func TestParseOFXFile_HeaderVariants(t *testing.T) {
	tests := []struct {
		name    string
		content string
	}{
		{"with standard header", sgmlHeader + sgmlBody},
		{"header missing entirely", sgmlBody},
		{"header missing, leading whitespace", "\r\n\n   \t" + sgmlBody},
		{"header missing, trailing whitespace", sgmlBody + "\r\n\n   "},
		{"header missing, UTF-8 BOM", utf8BOM + sgmlBody},
		{"header missing, BOM after whitespace", "\n " + utf8BOM + sgmlBody},
		{"header present, UTF-8 BOM", utf8BOM + sgmlHeader + sgmlBody},
		{"header present, leading whitespace", "\n\n" + sgmlHeader + sgmlBody},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewOFXParser()

			result, err := parser.ParseOFXFile(strings.NewReader(tt.content), 7)
			if err != nil {
				t.Fatalf("ParseOFXFile() returned error: %v", err)
			}
			assertParsedFixture(t, result)

			for _, txn := range result.Transactions {
				if txn.AccountID != 7 {
					t.Errorf("expected account_id 7, got %d", txn.AccountID)
				}
			}

			if err := parser.ValidateOFXFile(strings.NewReader(tt.content)); err != nil {
				t.Errorf("ValidateOFXFile() returned error: %v", err)
			}
		})
	}
}

// TestParseOFXFile_InvalidInput covers files that must be rejected. Each case
// asserts on the message too, because it is surfaced directly to the user by
// the import handler.
func TestParseOFXFile_InvalidInput(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantMessage string
	}{
		{"empty file", "", "OFX file is empty"},
		{"whitespace only", "   \r\n\t\n  ", "OFX file is empty"},
		{"BOM only", utf8BOM, "OFX file is empty"},
		{"BOM and whitespace only", utf8BOM + "  \n ", "OFX file is empty"},
		{"plain text", "hello world, not an OFX file", "failed to parse OFX file"},
		{"HTML error page", "<html><body>Session expired</body></html>", "failed to parse OFX file"},
		{"truncated header", "OFXHEADER:100\r\nDATA:OFXSGML\r\n" + sgmlBody, "failed to parse OFX file"},
		{"root element but no statement", "<OFX></OFX>", "failed to parse OFX file"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewOFXParser()

			result, err := parser.ParseOFXFile(strings.NewReader(tt.content), 1)
			if err == nil {
				t.Fatalf("expected an error, got result with %d transactions", len(result.Transactions))
			}
			if !strings.Contains(err.Error(), tt.wantMessage) {
				t.Errorf("expected error containing %q, got %q", tt.wantMessage, err.Error())
			}

			if err := parser.ValidateOFXFile(strings.NewReader(tt.content)); err == nil {
				t.Error("ValidateOFXFile() accepted an invalid file")
			}
		})
	}
}

// TestParseOFXFile_PreservesUTF8 guards the synthetic header's encoding
// declaration: injecting it must not mangle non-ASCII merchant names.
func TestParseOFXFile_PreservesUTF8(t *testing.T) {
	const merchant = "CAFÉ NOÎR ÉPICERIE"
	content := strings.Replace(sgmlBody, "CAFE NOIR", merchant, 1)

	result, err := NewOFXParser().ParseOFXFile(strings.NewReader(content), 1)
	if err != nil {
		t.Fatalf("ParseOFXFile() returned error: %v", err)
	}
	if got := result.Transactions[0].TransactionDetails; got != merchant {
		t.Errorf("expected details %q, got %q", merchant, got)
	}
}

// TestParseOFXFile_RejectsOversizedFile ensures a runaway upload cannot be
// buffered into memory without bound.
func TestParseOFXFile_RejectsOversizedFile(t *testing.T) {
	oversized := io.MultiReader(
		strings.NewReader(sgmlHeader),
		&repeatReader{b: 'A', remaining: maxOFXFileSize},
	)

	_, err := NewOFXParser().ParseOFXFile(oversized, 1)
	if err == nil {
		t.Fatal("expected an error for an oversized file")
	}
	if !strings.Contains(err.Error(), "maximum supported size") {
		t.Errorf("expected a size error, got %q", err.Error())
	}
}

// TestIsMissingHeader covers the header-detection predicate directly.
func TestIsMissingHeader(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{"SGML root element", "<OFX>", true},
		{"SGML root element, lowercase", "<ofx>", true},
		{"SGML root element with newline", "<OFX>\n<SIGNONMSGSRSV1>", true},
		{"SGML header present", sgmlHeader + sgmlBody, false},
		{"XML declaration present", `<?xml version="1.0"?><OFX>`, false},
		{"OFX 2.x processing instruction", `<?xml version="1.0"?>` + "\n" + `<?OFX OFXHEADER="200"?>`, false},
		{"unrelated markup", "<html>", false},
		{"plain text", "not ofx", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMissingHeader(tt.body); got != tt.want {
				t.Errorf("isMissingHeader(%.20q) = %v, want %v", tt.body, got, tt.want)
			}
		})
	}
}

// repeatReader emits the same byte a fixed number of times without allocating
// the whole payload up front.
type repeatReader struct {
	b         byte
	remaining int
}

func (r *repeatReader) Read(p []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	n := len(p)
	if n > r.remaining {
		n = r.remaining
	}
	for i := 0; i < n; i++ {
		p[i] = r.b
	}
	r.remaining -= n
	return n, nil
}
