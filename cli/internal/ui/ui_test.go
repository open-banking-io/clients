package ui

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestResolveFormatPrecedence(t *testing.T) {
	var buf bytes.Buffer // a buffer is never a terminal
	cases := []struct {
		outFmt string
		want   Format
	}{
		{"", FormatJSON}, // non-TTY auto-defaults to json
		{"table", FormatTable},
		{"json", FormatJSON},
		{"csv", FormatCSV},
		{"TABLE", FormatTable}, // case-insensitive
	}
	for _, tc := range cases {
		e, err := Resolve(&buf, &buf, &buf, tc.outFmt, false)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", tc.outFmt, err)
		}
		if e.Format != tc.want {
			t.Errorf("Resolve(%q).Format = %d, want %d", tc.outFmt, e.Format, tc.want)
		}
	}
	if _, err := Resolve(&buf, &buf, &buf, "yaml", false); err == nil {
		t.Error("expected an error for an unknown output format")
	}
}

func TestResolveColorDisabledOnNonTTY(t *testing.T) {
	var buf bytes.Buffer
	e, _ := Resolve(&buf, &buf, &buf, "table", false)
	if e.Colored() {
		t.Error("color must be off when stdout is not a terminal")
	}
}

func sampleTable() Table {
	return Table{
		Headers: []string{"NAME", "AMOUNT"},
		Rows: [][]Cell{
			{{Text: "Drift"}, {Text: "-50.00", Style: StyleNegative}},
			{{Text: "Salary"}, {Text: "1250.00", Style: StylePositive}},
		},
		Note: "2 shown",
	}
}

type rowView struct {
	Name   string `json:"name"`
	Amount string `json:"amount"`
}

func TestRenderTableAligns(t *testing.T) {
	var out bytes.Buffer
	e := Custom(nil, &out, &out, FormatTable, false, false)
	if err := e.Render(sampleTable(), nil); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"NAME", "AMOUNT", "Drift", "-50.00", "2 shown"} {
		if !strings.Contains(got, want) {
			t.Errorf("table output missing %q\n%s", want, got)
		}
	}
	// No ANSI escapes when color is off.
	if strings.Contains(got, "\x1b[") {
		t.Errorf("unexpected ANSI escape in plain table output:\n%q", got)
	}
}

func TestRenderTableColorized(t *testing.T) {
	var out bytes.Buffer
	e := Custom(nil, &out, &out, FormatTable, true, true)
	if err := e.Render(sampleTable(), nil); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, ansiRed) || !strings.Contains(got, ansiGreen) {
		t.Errorf("expected red/green ANSI codes in colorized output:\n%q", got)
	}
}

func TestRenderCSV(t *testing.T) {
	var out bytes.Buffer
	e := Custom(nil, &out, &out, FormatCSV, false, false)
	if err := e.Render(sampleTable(), nil); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "NAME,AMOUNT") {
		t.Errorf("csv missing header row:\n%s", got)
	}
	if !strings.Contains(got, "Drift,-50.00") {
		t.Errorf("csv missing data row:\n%s", got)
	}
	if strings.Contains(got, "2 shown") {
		t.Errorf("csv must not include the table Note:\n%s", got)
	}
}

func TestRenderJSONUsesView(t *testing.T) {
	var out bytes.Buffer
	e := Custom(nil, &out, &out, FormatJSON, false, false)
	view := []rowView{{Name: "Drift", Amount: "-50.00"}}
	if err := e.Render(sampleTable(), view); err != nil {
		t.Fatal(err)
	}
	var back []rowView
	if err := json.Unmarshal(out.Bytes(), &back); err != nil {
		t.Fatalf("output is not valid json: %v\n%s", err, out.String())
	}
	if len(back) != 1 || back[0].Name != "Drift" || back[0].Amount != "-50.00" {
		t.Errorf("json round-trip mismatch: %+v", back)
	}
	// JSON must come from the view (lowerCamel keys), not the table headers.
	if strings.Contains(out.String(), "NAME") {
		t.Errorf("json should use view keys, not table headers:\n%s", out.String())
	}
}

func TestSelectNumberedFallback(t *testing.T) {
	var errOut bytes.Buffer
	in := strings.NewReader("2\n")
	e := Custom(in, &bytes.Buffer{}, &errOut, FormatTable, false, false)
	opts := []Option{{Label: "Checking", Value: "acct-1"}, {Label: "Savings", Value: "acct-2"}}
	got, err := e.Select("Pick an account", opts)
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if got.Value != "acct-2" {
		t.Errorf("selected %q, want acct-2", got.Value)
	}
	if !strings.Contains(errOut.String(), "Pick an account") {
		t.Errorf("prompt should go to stderr:\n%s", errOut.String())
	}
}

func TestSelectNumberedInvalid(t *testing.T) {
	in := strings.NewReader("9\n")
	e := Custom(in, &bytes.Buffer{}, &bytes.Buffer{}, FormatTable, false, false)
	_, err := e.Select("Pick", []Option{{Label: "a", Value: "1"}})
	if err == nil {
		t.Fatal("expected an error for an out-of-range selection")
	}
}

func TestSelectNumberedEmptyIsNoInput(t *testing.T) {
	e := Custom(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{}, FormatTable, false, false)
	_, err := e.Select("Pick", []Option{{Label: "a", Value: "1"}})
	if err != ErrNoInput {
		t.Errorf("got %v, want ErrNoInput", err)
	}
}

func TestColorOnlyWhenEnabled(t *testing.T) {
	off := Custom(nil, &bytes.Buffer{}, &bytes.Buffer{}, FormatTable, false, false)
	if got := off.Color("x", StyleNegative); got != "x" {
		t.Errorf("color off should pass text through, got %q", got)
	}
	on := Custom(nil, &bytes.Buffer{}, &bytes.Buffer{}, FormatTable, true, true)
	if got := on.Color("x", StyleNegative); !strings.Contains(got, ansiRed) {
		t.Errorf("color on should wrap with red, got %q", got)
	}
	if got := on.Color("", StyleNegative); got != "" {
		t.Errorf("empty string should not be colored, got %q", got)
	}
}

func TestAmountAndStatusStyle(t *testing.T) {
	if AmountStyle("-5") != StyleNegative || AmountStyle("5") != StylePositive {
		t.Error("AmountStyle direction wrong")
	}
	if StatusStyle("Active") != StyleStatusOK || StatusStyle("expired") != StyleNegative {
		t.Error("StatusStyle mapping wrong")
	}
	if StatusStyle("pending") != StyleStatusWarn || StatusStyle("") != StyleNone {
		t.Error("StatusStyle fallback wrong")
	}
}

func TestMessageHelpersWriteToStderr(t *testing.T) {
	var out, errOut bytes.Buffer
	e := Custom(nil, &out, &errOut, FormatTable, false, false)
	e.Successf("done %d", 1)
	e.Errorf("oops")
	e.Infof("note")
	if out.Len() != 0 {
		t.Errorf("messages must not touch stdout, got %q", out.String())
	}
	for _, want := range []string{"done 1", "oops", "note"} {
		if !strings.Contains(errOut.String(), want) {
			t.Errorf("stderr missing %q:\n%s", want, errOut.String())
		}
	}
}

func TestSpinnerNoopOnNonTTY(t *testing.T) {
	var errOut bytes.Buffer
	e := Custom(nil, &bytes.Buffer{}, &errOut, FormatTable, false, false)
	stop := e.Spinner("working")
	stop()
	stop() // safe to call twice
	if errOut.Len() != 0 {
		t.Errorf("spinner must be silent on a non-terminal stderr, wrote: %q", errOut.String())
	}
}
