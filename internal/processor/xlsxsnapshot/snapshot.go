// Package xlsxsnapshot is the "values only" Excel processor (type
// xlsx-snapshot; the default "original" mode is passthrough): it sends a copy
// of the workbook with no formulas and no macros, reduced to the chosen
// sheets, always saved as .xlsx.
//
// Inputs: .xlsx, .xlsm, .xltx, .xltm (the same package format; templates
// become a plain workbook). Anything else, notably the old binary .xls and
// .xlsb, is xlsx.not_xlsx (param `value`: the file name). A file that isn't a
// valid package, is corrupt, or is password-protected/encrypted (an OLE
// container, which excelize can't open without the password: unsupported) is
// xlsx.open_failed.
//
// Options: {"sheets": ["Summary", "{date:MMMM}"]}: sheet names typed by hand
// (the file name can be dynamic, so the app can't always read the sheet list;
// the editor offers ReadSheets when the path resolves now). Each is a token
// template rendered at the slot time in the configured language. Matching is
// case-insensitive and ignores leading/trailing spaces, like Excel. An empty
// list keeps every sheet. Kept sheets stay in the workbook's own order,
// whatever the list order. A listed sheet that doesn't exist is the warning
// xlsx.sheet_missing (param `sheet`): the file is still sent with the sheets
// found; if none of the listed sheets exists the file is skipped with
// xlsx.no_sheet_found (param `sheets`: the rendered names, comma-separated).
// Validate rejects a malformed list (processor.options_invalid, field
// `sheets` or `sheets[i]` for an empty name) and malformed tokens (token.*).
//
// Steps, on a temp copy only (passthrough.Copy: shared read of the original,
// which is never opened for writing; file.locked / file.unreadable):
//
//  1. The package structure is read directly (workbook.go): the sheets in
//     order with their kind and visibility, and per kept worksheet the raw
//     formula cells with their cached values. Macro and dialog sheets (Excel
//     4/5 macro content) are never kept nor offered by ReadSheets.
//  2. excelize opens the copy (its temp files go in Env.TmpDir). The calc
//     chain is dropped up front: no formula survives, and excelize filters it
//     once per removed formula otherwise (quadratic on big sheets).
//  3. Every formula cell of every kept worksheet becomes its cached value,
//     keeping the cell's style (so the number format: dates stay dates):
//     numbers, dates, booleans and error values (#DIV/0!, #N/A…) keep the
//     saved type and value as they are (an error stays an error value, as if
//     typed); string results become shared strings. excelize never
//     recalculates, and neither does this package (no CalcCellValue): a
//     formula cell without a cached value becomes blank and is counted in
//     the warning xlsx.no_cached_values (params `sheet`, `count`), one per
//     sheet; the file is still sent. Values are taken before any sheet is
//     removed, so references to removed sheets keep their values.
//  4. The other sheets are deleted. Defined names that point at a removed
//     sheet or hold #REF! are deleted too (excelize already drops the names
//     scoped to a removed sheet), so Excel doesn't complain on open.
//  5. Visibility: hidden kept sheets stay hidden; if every kept sheet is
//     hidden the first one is shown (Excel needs one visible sheet). The
//     active sheet becomes a visible kept one when it isn't.
//  6. Saved as .xlsx, then the package is cleaned (clean.go): VBA project
//     and signature removed, dangling relationships and content types
//     dropped. Output.Name is the original name with the extension ".xlsx".
//
// Any other failure while editing or saving is xlsx.process_failed; a temp
// folder that can't be written is temp.unavailable. The cleanup removes the
// output; the source copy is removed as soon as the output is written (and
// on any failure).
//
// Known limitations (README "Excel"): charts, pivot tables, conditional
// formats and data validation that refer to a removed sheet lose those
// references (Excel may show them empty or offer to repair); external links
// keep their last cached values but stay links; images and shapes of kept
// sheets are kept; dynamic-array cells keep their value but lose the spill.
package xlsxsnapshot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/processor"
	"github.com/jacopofilonzi/wchron/internal/processor/passthrough"
	"github.com/jacopofilonzi/wchron/internal/resolver"
	"github.com/jacopofilonzi/wchron/internal/tokens"
)

// Type is the processor id in config.FileSpec.Processor.
const Type = "xlsx-snapshot"

// extensions are the accepted inputs.
var extensions = []string{".xlsx", ".xlsm", ".xltx", ".xltm"}

// Options are the processor options.
type Options struct {
	Sheets []string `json:"sheets"`
}

// Processor is the values-only Excel processor.
type Processor struct{}

// New returns the processor.
func New() Processor { return Processor{} }

func (Processor) Type() string { return Type }

func (Processor) Extensions() []string { return slices.Clone(extensions) }

func (Processor) OptionsSchema() []processor.Field {
	return []processor.Field{{Key: "sheets", Kind: processor.FieldList, Tokens: true}}
}

// Validate checks the options (see the package doc).
func (Processor) Validate(raw json.RawMessage) error {
	_, err := parseOptions(raw)
	return err
}

func parseOptions(raw json.RawMessage) (Options, error) {
	var o Options
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &o); err != nil {
			return o, apperr.New(apperr.ProcessorOptionsInvalid, fmt.Errorf("options: %w", err)).With("field", "sheets")
		}
	}
	for i, s := range o.Sheets {
		if strings.TrimSpace(s) == "" {
			return o, apperr.New(apperr.ProcessorOptionsInvalid, errors.New("empty sheet name")).With("field", "sheets["+strconv.Itoa(i)+"]")
		}
		if _, err := tokens.Parse(s); err != nil {
			return o, err
		}
	}
	return o, nil
}

// checkExtension returns xlsx.not_xlsx for a file this processor can't take.
func checkExtension(name string) error {
	if !slices.Contains(extensions, strings.ToLower(filepath.Ext(name))) {
		return apperr.New(apperr.XlsxNotXlsx, fmt.Errorf("%s is not an Open XML workbook", name)).With("value", name)
	}
	return nil
}

func openFailed(name string, err error) error {
	return apperr.New(apperr.XlsxOpenFailed, fmt.Errorf("open workbook %s: %w", name, err)).With("file", name)
}

func processFailed(name string, err error) error {
	return apperr.New(apperr.XlsxProcessFailed, fmt.Errorf("process workbook %s: %w", name, err)).With("file", name)
}

// ReadSheets returns the sheet names of the workbook at path, in workbook
// order, without macro and dialog sheets (for the editor's "read sheets
// from file"): xlsx.not_xlsx, file.locked / file.unreadable, xlsx.open_failed.
func ReadSheets(path string) ([]string, error) {
	name := filepath.Base(path)
	if err := checkExtension(name); err != nil {
		return nil, err
	}
	f, err := resolver.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(f); err != nil {
		return nil, resolver.ReadError(path, err)
	}
	p, err := openPackage(buf.Bytes())
	if err != nil {
		return nil, openFailed(name, err)
	}
	sheets, err := p.sheets()
	if err != nil {
		return nil, openFailed(name, err)
	}
	out := []string{}
	for _, s := range sheets {
		if s.Kind != kindMacro {
			out = append(out, s.Name)
		}
	}
	return out, nil
}

// Process makes the values-only copy (see the package doc).
func (Processor) Process(ctx context.Context, src processor.Source, raw json.RawMessage, env processor.Env) (processor.Output, func(), error) {
	if err := checkExtension(src.Name); err != nil {
		return processor.Output{}, nil, err
	}
	opts, err := parseOptions(raw)
	if err != nil {
		return processor.Output{}, nil, err
	}
	srcDir := filepath.Join(env.TmpDir, "source")
	if err := os.MkdirAll(srcDir, 0o700); err != nil {
		return processor.Output{}, nil, apperr.New(apperr.TempUnavailable, fmt.Errorf("create source dir: %w", err)).With("path", env.TmpDir)
	}
	defer func() { _ = os.RemoveAll(srcDir) }()
	copyPath, _, err := passthrough.Copy(ctx, src, srcDir)
	if err != nil {
		return processor.Output{}, nil, err
	}
	data, err := os.ReadFile(copyPath)
	if err != nil {
		return processor.Output{}, nil, processFailed(src.Name, err)
	}

	p, err := openPackage(data)
	if err != nil {
		return processor.Output{}, nil, openFailed(src.Name, err)
	}
	all, err := p.sheets()
	if err != nil {
		return processor.Output{}, nil, openFailed(src.Name, err)
	}
	kept, warnings, err := choose(all, opts, env, src.Name)
	if err != nil {
		return processor.Output{}, nil, err
	}

	f, err := excelize.OpenReader(bytes.NewReader(data), excelize.Options{TmpDir: env.TmpDir})
	if err != nil {
		return processor.Output{}, nil, openFailed(src.Name, err)
	}
	defer func() { _ = f.Close() }()

	// No formula survives: drop the calc chain before excelize filters it
	// once per removed formula.
	f.Pkg.Delete("xl/calcChain.xml")
	f.CalcChain = nil

	for _, s := range kept {
		if ctx.Err() != nil {
			return processor.Output{}, nil, ctx.Err()
		}
		if s.Kind != kindWorksheet {
			continue
		}
		missing, err := freeze(ctx, f, p, s)
		if err != nil {
			if ctx.Err() != nil {
				return processor.Output{}, nil, ctx.Err()
			}
			return processor.Output{}, nil, processFailed(src.Name, fmt.Errorf("sheet %q: %w", s.Name, err))
		}
		if missing > 0 {
			warnings = append(warnings, apperr.New(apperr.XlsxNoCachedValues,
				fmt.Errorf("%d formula cells without a cached value", missing)).
				With("sheet", s.Name).With("count", strconv.Itoa(missing)))
		}
	}
	if err := reduce(f, all, kept); err != nil {
		return processor.Output{}, nil, processFailed(src.Name, err)
	}

	outName := strings.TrimSuffix(src.Name, filepath.Ext(src.Name)) + ".xlsx"
	outPath := filepath.Join(env.TmpDir, outName)
	f.Path = outPath // WriteTo picks the .xlsx content type from it
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		return processor.Output{}, nil, processFailed(src.Name, fmt.Errorf("save: %w", err))
	}
	clean, err := cleanPackage(buf.Bytes())
	if err != nil {
		return processor.Output{}, nil, processFailed(src.Name, fmt.Errorf("clean package: %w", err))
	}
	remove := func() { _ = os.Remove(outPath) }
	if err := os.WriteFile(outPath, clean, 0o600); err != nil {
		remove()
		return processor.Output{}, nil, apperr.New(apperr.TempUnavailable, fmt.Errorf("write output: %w", err)).With("path", env.TmpDir)
	}
	return processor.Output{Path: outPath, Name: outName, Warnings: warnings}, remove, nil
}

// choose picks the sheets to keep, in workbook order.
func choose(all []sheetInfo, opts Options, env processor.Env, file string) ([]sheetInfo, []*apperr.Error, error) {
	var candidates []sheetInfo
	for _, s := range all {
		if s.Kind != kindMacro {
			candidates = append(candidates, s)
		}
	}
	if len(opts.Sheets) == 0 {
		if len(candidates) == 0 {
			return nil, nil, apperr.New(apperr.XlsxNoSheetFound, errors.New("the workbook has no sheet to keep")).With("file", file).With("sheets", "")
		}
		return candidates, nil, nil
	}
	want := map[int]bool{}
	var names []string
	var warnings []*apperr.Error
	for _, tpl := range opts.Sheets {
		name, err := tokens.Render(tpl, env.At, env.Lang)
		if err != nil {
			return nil, nil, err
		}
		name = strings.TrimSpace(name)
		if slices.ContainsFunc(names, func(n string) bool { return strings.EqualFold(n, name) }) {
			continue
		}
		names = append(names, name)
		i := slices.IndexFunc(candidates, func(s sheetInfo) bool { return strings.EqualFold(strings.TrimSpace(s.Name), name) })
		if i < 0 {
			warnings = append(warnings, apperr.New(apperr.XlsxSheetMissing, fmt.Errorf("sheet %q not in the workbook", name)).With("sheet", name))
			continue
		}
		want[i] = true
	}
	var kept []sheetInfo
	for i, s := range candidates {
		if want[i] {
			kept = append(kept, s)
		}
	}
	if len(kept) == 0 {
		return nil, nil, apperr.New(apperr.XlsxNoSheetFound, errors.New("none of the listed sheets is in the workbook")).
			With("file", file).With("sheets", strings.Join(names, ", "))
	}
	return kept, warnings, nil
}

// freeze replaces every formula of a worksheet with its cached value and
// returns how many had none.
func freeze(ctx context.Context, f *excelize.File, p *pkg, s sheetInfo) (int, error) {
	cells, err := p.formulas(s.Part)
	if err != nil {
		return 0, err
	}
	missing := 0
	for i, c := range cells {
		if i%4096 == 0 && ctx.Err() != nil {
			return 0, ctx.Err()
		}
		switch {
		case !c.HasValue:
			// Blank, style kept. Never recalculated.
			missing++
			err = f.SetCellDefault(s.Name, c.Ref, "")
		case c.Type == "str" || c.Type == "inlineStr":
			err = f.SetCellStr(s.Name, c.Ref, c.Value)
		default:
			// Numbers (dates), booleans, errors: removing the formula keeps
			// the cell's type, value and style as saved.
			err = f.SetCellFormula(s.Name, c.Ref, "")
		}
		if err != nil {
			return 0, fmt.Errorf("cell %s: %w", c.Ref, err)
		}
	}
	return missing, nil
}

// reduce deletes the sheets not kept and the names pointing at them, and
// fixes visibility and the active sheet.
func reduce(f *excelize.File, all, kept []sheetInfo) error {
	keep := map[string]bool{}
	for _, s := range kept {
		keep[s.Name] = true
	}
	// excelize counts only the worksheet parts and refuses to delete "the last
	// sheet" by that count, so a chart or macro sheet next to a single kept
	// worksheet would stay: count every sheet (one is always kept).
	f.SheetCount = len(f.GetSheetList())
	removed := map[string]bool{}
	for _, s := range all {
		if keep[s.Name] {
			continue
		}
		removed[strings.ToLower(s.Name)] = true
		if err := f.DeleteSheet(s.Name); err != nil {
			return fmt.Errorf("delete sheet %q: %w", s.Name, err)
		}
	}
	if got := f.GetSheetList(); len(got) != len(kept) {
		return fmt.Errorf("%d sheets left, want %d", len(got), len(kept))
	}
	if len(removed) > 0 {
		for _, dn := range f.GetDefinedName() {
			if !refersToAny(dn.RefersTo, removed) {
				continue
			}
			if err := f.DeleteDefinedName(&excelize.DefinedName{Name: dn.Name, Scope: dn.Scope}); err != nil {
				return fmt.Errorf("delete defined name %q: %w", dn.Name, err)
			}
		}
	}

	firstVisible := -1
	for i, s := range kept {
		if !s.Hidden {
			firstVisible = i
			break
		}
	}
	if firstVisible < 0 {
		firstVisible = 0
		if err := f.SetSheetVisible(kept[0].Name, true); err != nil {
			return fmt.Errorf("show sheet %q: %w", kept[0].Name, err)
		}
	}
	active := f.GetSheetName(f.GetActiveSheetIndex())
	i := slices.IndexFunc(kept, func(s sheetInfo) bool { return s.Name == active })
	if i < 0 || (kept[i].Hidden && i != firstVisible) {
		f.SetActiveSheet(firstVisible)
	}
	return nil
}
