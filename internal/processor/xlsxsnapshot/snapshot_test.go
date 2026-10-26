package xlsxsnapshot

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/i18n"
	"github.com/jacopofilonzi/wchron/internal/platform"
	"github.com/jacopofilonzi/wchron/internal/processor"
)

// Every workbook here is synthetic: built by excelize, then the worksheet
// parts are replaced with hand-written XML so formulas can carry (or lack)
// cached values exactly as Excel saves them.

type sheetDef struct {
	name   string
	cells  string // the <sheetData> content
	hidden bool
}

type bookDef struct {
	sheets []sheetDef
	// names are defined names: {name, refersTo, scope ("" for the workbook)}.
	names  [][3]string
	active string
	vba    bool
	// setup runs on the excelize file before saving (styles, extra sheets).
	setup func(t *testing.T, f *excelize.File)
}

const wsHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" ` +
	`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
	`<sheetViews><sheetView workbookViewId="0"/></sheetViews><sheetData>`

// build writes the workbook as path (its extension decides the format).
func build(t *testing.T, path string, def bookDef) {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	for i, s := range def.sheets {
		if i == 0 {
			if err := f.SetSheetName("Sheet1", s.name); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if _, err := f.NewSheet(s.name); err != nil {
			t.Fatal(err)
		}
	}
	if def.setup != nil {
		def.setup(t, f)
	}
	for _, n := range def.names {
		if err := f.SetDefinedName(&excelize.DefinedName{Name: n[0], RefersTo: n[1], Scope: n[2]}); err != nil {
			t.Fatal(err)
		}
	}
	active := def.active
	if active == "" {
		active = def.sheets[0].name
	}
	idx, err := f.GetSheetIndex(active)
	if err != nil {
		t.Fatal(err)
	}
	f.SetActiveSheet(idx)
	for _, s := range def.sheets {
		if s.hidden {
			if err := f.SetSheetVisible(s.name, false); err != nil {
				t.Fatal(err)
			}
		}
	}
	if def.vba {
		// A fake VBA project: excelize only checks the OLE signature.
		bin := append([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}, bytes.Repeat([]byte{1}, 512)...)
		if err := f.AddVBAProject(bin); err != nil {
			t.Fatal(err)
		}
	}
	f.Path = path
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	// Replace the worksheet parts with the hand-written cells.
	p, err := openPackage(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	infos, err := p.sheets()
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string]string{}
	for _, s := range def.sheets {
		for _, in := range infos {
			if in.Name == s.name && in.Kind == kindWorksheet {
				parts[in.Part] = wsHeader + s.cells + `</sheetData></worksheet>`
			}
		}
	}
	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, zf := range p.zr.File {
		if x, ok := parts[zf.Name]; ok {
			w, err := zw.Create(zf.Name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.WriteString(w, x); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := zw.Copy(zf); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, out.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

// dateStyle is the style id of the date cells (created first in setup).
const dateStyle = 1

func withDateStyle(t *testing.T, f *excelize.File) {
	id, err := f.NewStyle(&excelize.Style{NumFmt: 14})
	if err != nil || id != dateStyle {
		t.Fatalf("date style: %d, %v", id, err)
	}
}

// sales has formulas of every result type, a shared formula, a missing
// cached value, a cross-sheet reference to a sheet that gets removed, a
// hidden sheet and defined names.
var sales = bookDef{
	sheets: []sheetDef{
		{name: "Data", cells: `<row r="1">` +
			`<c r="A1"><v>10</v></c>` +
			`<c r="B1" s="1"><f>TODAY()</f><v>46321</v></c>` +
			`<c r="C1" t="str"><f>"x"&amp;"y"</f><v>xy</v></c>` +
			`<c r="D1" t="b"><f>A1&gt;5</f><v>1</v></c>` +
			`<c r="E1" t="e"><f>1/0</f><v>#DIV/0!</v></c>` +
			`<c r="F1"><f>A1*2</f></c>` +
			`<c r="G1"><f t="shared" ref="G1:G3" si="0">A1*2</f><v>20</v></c>` +
			`<c r="H1" t="str"><f>""</f><v></v></c>` +
			`</row><row r="2"><c r="A2"><v>20</v></c><c r="G2"><f t="shared" si="0"/><v>40</v></c></row>` +
			`<row r="3"><c r="A3" s="1"><f>A1+A2</f><v>30</v></c><c r="G3"><f t="shared" si="0"/><v>60</v></c></row>`},
		{name: "Summary", cells: `<row r="1"><c r="A1"><f>'Data'!A3</f><v>30</v></c>` +
			`<c r="B1"><f>Secret!A1*1</f><v>99</v></c><c r="C1" t="inlineStr"><is><t>Totale</t></is></c></row>`},
		{name: "Secret", hidden: true, cells: `<row r="1"><c r="A1"><v>99</v></c></row>`},
	},
	names: [][3]string{
		{"Total", "Data!$A$3", ""},
		{"SecretVal", "Secret!$A$1", ""},
		{"Mixed", "SUM(Data!$A$1,'Secret'!$A$1)", ""},
		{"OnSecret", "Secret!$A$1", "Secret"},
		{"OnData", "Data!$A$1", "Data"},
	},
	setup: withDateStyle,
}

type env struct {
	t    *testing.T
	dir  string // where the originals are
	tmp  string // Env.TmpDir
	slot time.Time
}

func newEnv(t *testing.T) *env {
	return &env{t: t, dir: t.TempDir(), tmp: t.TempDir(), slot: time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)}
}

func (e *env) book(name string, def bookDef) processor.Source {
	e.t.Helper()
	p := filepath.Join(e.dir, name)
	build(e.t, p, def)
	return processor.Source{Path: p, Name: name}
}

func (e *env) process(src processor.Source, sheets ...string) (processor.Output, func(), error) {
	e.t.Helper()
	raw, _ := json.Marshal(Options{Sheets: sheets})
	return New().Process(context.Background(), src, raw, processor.Env{TmpDir: e.tmp, At: e.slot, Lang: i18n.IT})
}

func (e *env) mustProcess(src processor.Source, sheets ...string) (processor.Output, *excelize.File) {
	e.t.Helper()
	out, cleanup, err := e.process(src, sheets...)
	if err != nil {
		e.t.Fatalf("process: %v", err)
	}
	e.t.Cleanup(cleanup)
	f, err := excelize.OpenFile(out.Path)
	if err != nil {
		e.t.Fatalf("open output: %v", err)
	}
	e.t.Cleanup(func() { _ = f.Close() })
	return out, f
}

func hashFile(t *testing.T, p string) [32]byte {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(b)
}

func zipNames(t *testing.T, p string) []string {
	t.Helper()
	zr, err := zip.OpenReader(p)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	var out []string
	for _, f := range zr.File {
		out = append(out, f.Name)
	}
	return out
}

func zipPart(t *testing.T, p, name string) string {
	t.Helper()
	zr, err := zip.OpenReader(p)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer rc.Close()
			b, err := io.ReadAll(rc)
			if err != nil {
				t.Fatal(err)
			}
			return string(b)
		}
	}
	t.Fatalf("%s has no part %s", p, name)
	return ""
}

func codes(ws []*apperr.Error) []apperr.Code {
	var out []apperr.Code
	for _, w := range ws {
		out = append(out, w.Code)
	}
	return out
}

func noFormulas(t *testing.T, f *excelize.File) {
	t.Helper()
	for _, s := range f.GetSheetList() {
		rows, err := f.GetRows(s)
		if err != nil {
			t.Fatal(err)
		}
		for r := range rows {
			for c := range 10 {
				ref, _ := excelize.CoordinatesToCellName(c+1, r+1)
				if fm, _ := f.GetCellFormula(s, ref); fm != "" {
					t.Errorf("%s!%s still has the formula %q", s, ref, fm)
				}
			}
		}
	}
}

func raw(t *testing.T, f *excelize.File, sheet, cell string) string {
	t.Helper()
	v, err := f.GetCellValue(sheet, cell, excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func cellType(t *testing.T, f *excelize.File, sheet, cell string) excelize.CellType {
	t.Helper()
	ct, err := f.GetCellType(sheet, cell)
	if err != nil {
		t.Fatal(err)
	}
	return ct
}

func TestValuesKeptTypesAndStyles(t *testing.T) {
	e := newEnv(t)
	src := e.book("sales.xlsx", sales)
	before := hashFile(t, src.Path)
	info, _ := os.Stat(src.Path)

	out, f := e.mustProcess(src, "summary", "  DATA ")
	if out.Name != "sales.xlsx" || filepath.Dir(out.Path) != e.tmp {
		t.Fatalf("output %+v", out)
	}
	// Workbook order, not list order.
	if got := f.GetSheetList(); !slices.Equal(got, []string{"Data", "Summary"}) {
		t.Fatalf("sheets %v", got)
	}
	noFormulas(t, f)
	for _, c := range []struct {
		sheet, cell, want string
		typ               excelize.CellType
	}{
		{"Data", "A3", "30", excelize.CellTypeUnset},
		{"Data", "B1", "46321", excelize.CellTypeUnset},
		{"Data", "C1", "xy", excelize.CellTypeSharedString},
		{"Data", "D1", "1", excelize.CellTypeBool},
		{"Data", "E1", "#DIV/0!", excelize.CellTypeError},
		{"Data", "F1", "", excelize.CellTypeUnset},
		{"Data", "G1", "20", excelize.CellTypeUnset},
		{"Data", "G3", "60", excelize.CellTypeUnset},
		{"Data", "H1", "", excelize.CellTypeSharedString},
		{"Summary", "A1", "30", excelize.CellTypeUnset},
		// Cross reference to the removed sheet: the value stays.
		{"Summary", "B1", "99", excelize.CellTypeUnset},
		{"Summary", "C1", "Totale", excelize.CellTypeInlineString},
	} {
		if got := raw(t, f, c.sheet, c.cell); got != c.want {
			t.Errorf("%s!%s = %q, want %q", c.sheet, c.cell, got, c.want)
		}
		if got := cellType(t, f, c.sheet, c.cell); got != c.typ {
			t.Errorf("%s!%s type %v, want %v", c.sheet, c.cell, got, c.typ)
		}
	}
	// The date keeps its number format.
	for _, cell := range []string{"B1", "A3"} {
		if s, _ := f.GetCellStyle("Data", cell); s != dateStyle {
			t.Errorf("Data!%s style %d, want the date style", cell, s)
		}
	}
	if v, _ := f.GetCellValue("Data", "B1"); v != "10-26-26" {
		t.Errorf("formatted date %q", v)
	}
	// One formula without a cached value.
	if len(out.Warnings) != 1 || out.Warnings[0].Code != apperr.XlsxNoCachedValues ||
		out.Warnings[0].Params["count"] != "1" || out.Warnings[0].Params["sheet"] != "Data" {
		t.Fatalf("warnings %v", out.Warnings)
	}
	// Names pointing at the removed sheet are gone; the others stay.
	var names []string
	for _, dn := range f.GetDefinedName() {
		names = append(names, dn.Name+"@"+dn.Scope)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"OnData@Data", "Total@Workbook"}) {
		t.Fatalf("defined names %v", names)
	}
	parts := zipNames(t, out.Path)
	if slices.Contains(parts, "xl/calcChain.xml") {
		t.Error("calc chain kept")
	}
	sheetParts := 0
	for _, n := range parts {
		if strings.HasPrefix(n, "xl/worksheets/") && strings.HasSuffix(n, ".xml") {
			sheetParts++
			if x := zipPart(t, out.Path, n); strings.Contains(x, "<f") {
				t.Errorf("%s still holds a formula element: %s", n, x)
			}
		}
	}
	if sheetParts != 2 {
		t.Errorf("%d worksheet parts, want 2: %v", sheetParts, parts)
	}
	// The original is untouched.
	after, _ := os.Stat(src.Path)
	if hashFile(t, src.Path) != before || !after.ModTime().Equal(info.ModTime()) {
		t.Fatal("the original changed")
	}
}

func TestAllSheetsWhenListEmpty(t *testing.T) {
	e := newEnv(t)
	out, f := e.mustProcess(e.book("sales.xlsx", sales))
	if got := f.GetSheetList(); !slices.Equal(got, []string{"Data", "Summary", "Secret"}) {
		t.Fatalf("sheets %v", got)
	}
	noFormulas(t, f)
	if v, _ := f.GetSheetVisible("Secret"); v {
		t.Error("a hidden sheet became visible")
	}
	if !slices.Equal(codes(out.Warnings), []apperr.Code{apperr.XlsxNoCachedValues}) {
		t.Fatalf("warnings %v", out.Warnings)
	}
	if len(f.GetDefinedName()) != 5 {
		t.Fatalf("names %v", f.GetDefinedName())
	}
}

func TestMissingSheets(t *testing.T) {
	e := newEnv(t)
	src := e.book("sales.xlsx", sales)
	out, f := e.mustProcess(src, "Summary", "Nope", "nope", "Other")
	if got := f.GetSheetList(); !slices.Equal(got, []string{"Summary"}) {
		t.Fatalf("sheets %v", got)
	}
	var missing []string
	for _, w := range out.Warnings {
		if w.Code == apperr.XlsxSheetMissing {
			missing = append(missing, w.Params["sheet"])
		}
	}
	if !slices.Equal(missing, []string{"Nope", "Other"}) {
		t.Fatalf("missing %v (warnings %v)", missing, out.Warnings)
	}

	// None found: skipped, nothing left behind.
	tmp := t.TempDir()
	raw, _ := json.Marshal(Options{Sheets: []string{"Nope", "Other"}})
	_, cleanup, err := New().Process(context.Background(), src, raw, processor.Env{TmpDir: tmp, At: e.slot, Lang: i18n.IT})
	if ae := apperr.As(err); ae.Code != apperr.XlsxNoSheetFound || ae.Params["sheets"] != "Nope, Other" || cleanup != nil {
		t.Fatalf("none found: %v", err)
	}
	if es, _ := os.ReadDir(tmp); len(es) != 0 {
		t.Fatalf("leftovers %v", es)
	}
}

func TestTokenSheetName(t *testing.T) {
	e := newEnv(t)
	def := bookDef{sheets: []sheetDef{
		{name: "settembre 2026", cells: `<row r="1"><c r="A1"><v>9</v></c></row>`},
		{name: "Ottobre 2026", cells: `<row r="1"><c r="A1"><f>'settembre 2026'!A1+1</f><v>10</v></c></row>`},
	}}
	_, f := e.mustProcess(e.book("months.xlsx", def), "{date:MMMM YYYY}")
	if got := f.GetSheetList(); !slices.Equal(got, []string{"Ottobre 2026"}) {
		t.Fatalf("sheets %v", got)
	}
	if v := raw(t, f, "Ottobre 2026", "A1"); v != "10" {
		t.Fatalf("value %q", v)
	}
}

func TestHiddenAndActiveSheets(t *testing.T) {
	e := newEnv(t)
	src := e.book("sales.xlsx", sales)
	// Only hidden sheets kept: the first becomes visible and active.
	_, f := e.mustProcess(src, "Secret")
	if v, _ := f.GetSheetVisible("Secret"); !v {
		t.Fatal("the only sheet is hidden")
	}
	if f.GetActiveSheetIndex() != 0 {
		t.Fatalf("active %d", f.GetActiveSheetIndex())
	}

	// The active sheet removed: the active one is a visible kept sheet.
	def := sales
	def.active = "Data"
	src = e.book("active.xlsx", def)
	_, f = e.mustProcess(src, "Summary", "Secret")
	if got := f.GetSheetList(); !slices.Equal(got, []string{"Summary", "Secret"}) {
		t.Fatalf("sheets %v", got)
	}
	if v, _ := f.GetSheetVisible("Secret"); v {
		t.Error("Secret must stay hidden")
	}
	if name := f.GetSheetName(f.GetActiveSheetIndex()); name != "Summary" {
		t.Fatalf("active %q", name)
	}
}

func TestMacrosRemoved(t *testing.T) {
	e := newEnv(t)
	def := sales
	def.vba = true
	src := e.book("macro.xlsm", def)
	if !slices.Contains(zipNames(t, src.Path), "xl/vbaProject.bin") {
		t.Fatal("fixture without a VBA project")
	}
	out, f := e.mustProcess(src, "Data")
	if out.Name != "macro.xlsx" || filepath.Ext(out.Path) != ".xlsx" {
		t.Fatalf("output %+v", out)
	}
	for _, n := range zipNames(t, out.Path) {
		if strings.Contains(strings.ToLower(n), "vba") {
			t.Errorf("macro part %s kept", n)
		}
	}
	ct := zipPart(t, out.Path, "[Content_Types].xml")
	if strings.Contains(ct, "macroEnabled") || strings.Contains(ct, "vbaProject") ||
		!strings.Contains(ct, `PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"`) {
		t.Fatalf("content types: %s", ct)
	}
	rels := zipPart(t, out.Path, "xl/_rels/workbook.xml.rels")
	if strings.Contains(rels, "vbaProject") || strings.Contains(rels, "calcChain") {
		t.Fatalf("workbook rels: %s", rels)
	}
	noFormulas(t, f)
}

func TestChartSheetRemovedNextToOneWorksheet(t *testing.T) {
	e := newEnv(t)
	def := bookDef{
		sheets: []sheetDef{{name: "Data", cells: `<row r="1"><c r="A1"><v>1</v></c><c r="B1"><v>2</v></c></row>`}},
		setup: func(t *testing.T, f *excelize.File) {
			if err := f.AddChartSheet("Chart", &excelize.Chart{Type: excelize.Col,
				Series: []excelize.ChartSeries{{Name: "Data!$A$1", Values: "Data!$A$1:$B$1"}}}); err != nil {
				t.Fatal(err)
			}
		},
	}
	src := e.book("chart.xlsx", def)
	sheets, err := ReadSheets(src.Path)
	if err != nil || !slices.Equal(sheets, []string{"Data", "Chart"}) {
		t.Fatalf("read sheets %v, %v", sheets, err)
	}
	_, f := e.mustProcess(src, "Data")
	if got := f.GetSheetList(); !slices.Equal(got, []string{"Data"}) {
		t.Fatalf("sheets %v", got)
	}
	ct := zipPart(t, filepath.Join(e.tmp, "chart.xlsx"), "[Content_Types].xml")
	if strings.Contains(ct, "chartsheets") {
		t.Fatalf("chart sheet override left: %s", ct)
	}
}

func TestOpenFailures(t *testing.T) {
	e := newEnv(t)
	check := func(name string, data []byte, want apperr.Code) {
		t.Helper()
		p := filepath.Join(e.dir, name)
		if err := os.WriteFile(p, data, 0o600); err != nil {
			t.Fatal(err)
		}
		tmp := t.TempDir()
		_, cleanup, err := New().Process(context.Background(), processor.Source{Path: p, Name: name}, nil, processor.Env{TmpDir: tmp, At: e.slot})
		if ae := apperr.As(err); ae.Code != want || cleanup != nil {
			t.Errorf("%s: %v, want %s", name, err, want)
		}
		if es, _ := os.ReadDir(tmp); len(es) != 0 {
			t.Errorf("%s: leftovers %v", name, es)
		}
		if _, err := ReadSheets(p); apperr.As(err).Code != want {
			t.Errorf("ReadSheets %s: %v, want %s", name, err, want)
		}
	}
	check("corrupt.xlsx", []byte("this is not a workbook at all"), apperr.XlsxOpenFailed)
	check("old.xls", []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1, 0, 0}, apperr.XlsxNotXlsx)
	check("binary.xlsb", []byte("PK"), apperr.XlsxNotXlsx)
	check("notes.txt", []byte("x"), apperr.XlsxNotXlsx)

	// Password-protected (encrypted by excelize).
	f := excelize.NewFile()
	defer f.Close()
	var buf bytes.Buffer
	f.Path = "locked.xlsx"
	if err := f.Write(&buf, excelize.Options{Password: "secret"}); err != nil {
		t.Fatal(err)
	}
	check("protected.xlsx", buf.Bytes(), apperr.XlsxOpenFailed)

	// A zip without a workbook.
	var zb bytes.Buffer
	zw := zip.NewWriter(&zb)
	w, _ := zw.Create("hello.txt")
	_, _ = w.Write([]byte("hi"))
	_ = zw.Close()
	check("empty.xlsx", zb.Bytes(), apperr.XlsxOpenFailed)
}

func TestLockedOriginal(t *testing.T) {
	e := newEnv(t)
	src := e.book("held.xlsx", sales)
	lock, err := platform.OpenExclusive(src.Path)
	if err != nil {
		t.Skipf("no exclusive open on this platform: %v", err)
	}
	defer lock.Close()
	if _, _, err := e.process(src, "Data"); apperr.As(err).Code != apperr.FileLocked {
		t.Fatalf("locked: %v", err)
	}
	if _, err := ReadSheets(src.Path); apperr.As(err).Code != apperr.FileLocked {
		t.Fatalf("ReadSheets locked: %v", err)
	}
}

func TestCleanupAndCancel(t *testing.T) {
	e := newEnv(t)
	src := e.book("sales.xlsx", sales)
	out, cleanup, err := e.process(src, "Data")
	if err != nil {
		t.Fatal(err)
	}
	if es, _ := os.ReadDir(e.tmp); len(es) != 1 || es[0].Name() != "sales.xlsx" {
		t.Fatalf("temp dir holds %v, want only the output", es)
	}
	cleanup()
	if _, err := os.Stat(out.Path); !os.IsNotExist(err) {
		t.Fatal("output still there after cleanup")
	}
	if es, _ := os.ReadDir(e.tmp); len(es) != 0 {
		t.Fatalf("leftovers %v", es)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := New().Process(ctx, src, nil, processor.Env{TmpDir: e.tmp, At: e.slot}); err == nil {
		t.Fatal("cancelled run succeeded")
	}
	if es, _ := os.ReadDir(e.tmp); len(es) != 0 {
		t.Fatalf("leftovers after cancel %v", es)
	}
}

func TestReadSheets(t *testing.T) {
	e := newEnv(t)
	src := e.book("sales.xlsm", sales)
	got, err := ReadSheets(src.Path)
	if err != nil || !slices.Equal(got, []string{"Data", "Summary", "Secret"}) {
		t.Fatalf("got %v, %v", got, err)
	}
	if _, err := ReadSheets(filepath.Join(e.dir, "missing.xlsx")); apperr.As(err).Code != apperr.FileUnreadable {
		t.Fatalf("missing: %v", err)
	}
}

func TestValidateAndSchema(t *testing.T) {
	p := New()
	for _, c := range []struct {
		raw   string
		code  apperr.Code
		field string
	}{
		{`{}`, "", ""},
		{``, "", ""},
		{`{"sheets": []}`, "", ""},
		{`{"sheets": ["Summary", "{date:MMMM}"]}`, "", ""},
		{`{"sheets": ["Summary", "  "]}`, apperr.ProcessorOptionsInvalid, "sheets[1]"},
		{`{"sheets": "Summary"}`, apperr.ProcessorOptionsInvalid, "sheets"},
		{`{"sheets": ["{date:QQ}"]}`, apperr.TokenFormatInvalid, ""},
	} {
		err := p.Validate(json.RawMessage(c.raw))
		if c.code == "" {
			if err != nil {
				t.Errorf("%s: %v", c.raw, err)
			}
			continue
		}
		if ae := apperr.As(err); ae.Code != c.code || (c.field != "" && ae.Params["field"] != c.field) {
			t.Errorf("%s: %v, want %s %s", c.raw, err, c.code, c.field)
		}
	}
	r := processor.NewRegistry(p)
	s := r.Schemas()
	if len(s) != 1 || s[0].Type != "xlsx-snapshot" || !slices.Equal(s[0].Extensions, []string{".xlsx", ".xlsm", ".xltx", ".xltm"}) ||
		len(s[0].Options) != 1 || s[0].Options[0].Key != "sheets" || !s[0].Options[0].Tokens || s[0].Options[0].Kind != processor.FieldList {
		t.Fatalf("schema %+v", s)
	}
}

func TestSheetRefs(t *testing.T) {
	for f, want := range map[string][]string{
		"Data!$A$1":                     {"Data"},
		"'My ''odd'' sheet'!A1":         {"My 'odd' sheet"},
		"SUM(Data!A1,'Other one'!B2)":   {"Data", "Other one"},
		"Jan:Mar!A1":                    {"Jan", "Mar"},
		"[1]Remote!A1":                  nil,
		"'C:\\x\\[book.xlsx]Remote'!A1": nil,
		`IF(A1="Fake!A1",1,2)`:          nil,
		"#REF!":                         {"#REF"},
	} {
		if got := sheetRefs(f); !slices.Equal(got, want) {
			t.Errorf("sheetRefs(%q) = %q, want %q", f, got, want)
		}
	}
	if !refersToAny("Sheet9!#REF!", map[string]bool{}) || refersToAny("Data!A1", map[string]bool{"secret": true}) ||
		!refersToAny("'secret'!A1", map[string]bool{"secret": true}) {
		t.Fatal("refersToAny")
	}
}
