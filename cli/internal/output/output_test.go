package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRenderJSONMarshalsRawValue(t *testing.T) {
	var buf bytes.Buffer
	payload := map[string]any{"id": 42, "name": "Widget"}
	if err := Render(&buf, FormatJSON, Renderable{JSON: payload}); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if got["name"] != "Widget" {
		t.Fatalf("name = %v, want Widget", got["name"])
	}
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Fatal("JSON output should end with a newline")
	}
}

func TestRenderCSV(t *testing.T) {
	var buf bytes.Buffer
	tbl := Table{
		Columns: []string{"id", "name"},
		Rows: [][]string{
			{"1", "Widget"},
			{"2", "Gadget, deluxe"}, // embedded comma must be quoted
		},
	}
	if err := Render(&buf, FormatCSV, Renderable{Table: tbl}); err != nil {
		t.Fatal(err)
	}
	want := "id,name\n1,Widget\n2,\"Gadget, deluxe\"\n"
	if buf.String() != want {
		t.Fatalf("CSV mismatch:\n got %q\nwant %q", buf.String(), want)
	}
}

func TestRenderCSVEmptyStillEmitsHeader(t *testing.T) {
	var buf bytes.Buffer
	tbl := Table{Columns: []string{"id", "name"}}
	if err := Render(&buf, FormatCSV, Renderable{Table: tbl}); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "id,name\n" {
		t.Fatalf("empty CSV = %q, want header only", buf.String())
	}
}

func TestRenderTableContainsHeadersAndValues(t *testing.T) {
	var buf bytes.Buffer
	tbl := Table{
		Columns: []string{"ID", "NAME"},
		Rows:    [][]string{{"1", "Widget"}},
	}
	if err := Render(&buf, FormatTable, Renderable{Table: tbl}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"ID", "NAME", "Widget"} {
		if !strings.Contains(out, want) {
			t.Fatalf("table output missing %q:\n%s", want, out)
		}
	}
}

func TestRenderTableEmptyShowsNoResults(t *testing.T) {
	var buf bytes.Buffer
	tbl := Table{Columns: []string{"ID", "NAME"}}
	if err := Render(&buf, FormatTable, Renderable{Table: tbl}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(buf.String()), "no results") {
		t.Fatalf("empty table should say no results, got:\n%s", buf.String())
	}
}

func TestRenderUnknownFormatErrors(t *testing.T) {
	var buf bytes.Buffer
	if err := Render(&buf, "xml", Renderable{}); err == nil {
		t.Fatal("want error for unknown format")
	}
}

func TestParseFormat(t *testing.T) {
	cases := map[string]struct {
		in      string
		jsonOpt bool
		want    string
		wantErr bool
	}{
		"default is table":          {"", false, FormatTable, false},
		"explicit csv":              {"csv", false, FormatCSV, false},
		"json flag wins":            {"", true, FormatJSON, false},
		"json flag over table flag": {"table", true, FormatJSON, false},
		"bad format":                {"yaml", false, "", true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := ParseFormat(tc.in, tc.jsonOpt)
			if tc.wantErr {
				if err == nil {
					t.Fatal("want error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("ParseFormat(%q,%v) = %q, want %q", tc.in, tc.jsonOpt, got, tc.want)
			}
		})
	}
}
