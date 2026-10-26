package xlsxsnapshot

// Reading the package structure directly (archive/zip + encoding/xml):
// excelize doesn't expose the sheet kinds (worksheet, chart, dialog, macro
// sheet) or the raw formula cells with their cached values, so the processor
// finds them here and then edits the workbook through excelize.

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// sheetKind is what a workbook sheet entry points to.
type sheetKind int

const (
	kindWorksheet sheetKind = iota
	kindChartsheet
	// kindMacro is an Excel 4 macro sheet or an Excel 5 dialog sheet: macro
	// content, never kept in a values-only copy.
	kindMacro
	kindOther
)

// sheetInfo is one entry of the workbook's sheet list, in workbook order.
type sheetInfo struct {
	Name   string
	Part   string // zip path of its XML, e.g. "xl/worksheets/sheet1.xml"
	Kind   sheetKind
	Hidden bool // hidden or very hidden
}

// pkg is an opened OOXML package.
type pkg struct {
	zr    *zip.Reader
	files map[string]*zip.File // by lowercase name (part names are case-insensitive)
}

func openPackage(data []byte) (*pkg, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("not a zip package: %w", err)
	}
	p := &pkg{zr: zr, files: make(map[string]*zip.File, len(zr.File))}
	for _, f := range zr.File {
		p.files[strings.ToLower(f.Name)] = f
	}
	return p, nil
}

// has reports whether the part exists (case-insensitive, also URL-escaped).
func (p *pkg) has(part string) bool {
	if _, ok := p.files[strings.ToLower(part)]; ok {
		return true
	}
	if u, err := url.PathUnescape(part); err == nil && u != part {
		_, ok := p.files[strings.ToLower(u)]
		return ok
	}
	return false
}

// maxXMLPart bounds the small XML parts read whole (rels, workbook, content
// types): a corrupt or hostile file can't make us allocate gigabytes.
const maxXMLPart = 64 << 20

func (p *pkg) open(part string) (io.ReadCloser, error) {
	f, ok := p.files[strings.ToLower(part)]
	if !ok {
		return nil, fmt.Errorf("part %s missing", part)
	}
	return f.Open()
}

func (p *pkg) read(part string) ([]byte, error) {
	rc, err := p.open(part)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	b, err := io.ReadAll(io.LimitReader(rc, maxXMLPart+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", part, err)
	}
	if len(b) > maxXMLPart {
		return nil, fmt.Errorf("part %s too large", part)
	}
	return b, nil
}

type relationships struct {
	Rels []relationship `xml:"Relationship"`
}

type relationship struct {
	ID         string `xml:"Id,attr"`
	Type       string `xml:"Type,attr"`
	Target     string `xml:"Target,attr"`
	TargetMode string `xml:"TargetMode,attr"`
}

// relsPart is the relationships part of a part ("" for the package root).
func relsPart(part string) string {
	if part == "" {
		return "_rels/.rels"
	}
	return path.Join(path.Dir(part), "_rels", path.Base(part)+".rels")
}

// sourceOfRels is the inverse of relsPart.
func sourceOfRels(rels string) string {
	dir, base := path.Split(rels)
	dir = strings.TrimSuffix(strings.TrimSuffix(dir, "/"), "_rels")
	return strings.TrimPrefix(dir+strings.TrimSuffix(base, ".rels"), "/")
}

// resolveTarget is the part a relationship of source points to.
func resolveTarget(source, target string) string {
	if strings.HasPrefix(target, "/") {
		return strings.TrimPrefix(target, "/")
	}
	return path.Join(path.Dir(source), target)
}

func (p *pkg) rels(part string) ([]relationship, error) {
	b, err := p.read(relsPart(part))
	if err != nil {
		return nil, err
	}
	var r relationships
	if err := xml.Unmarshal(b, &r); err != nil {
		return nil, fmt.Errorf("parse %s: %w", relsPart(part), err)
	}
	return r.Rels, nil
}

// workbookPart finds the main workbook part from the package relationships.
func (p *pkg) workbookPart() (string, error) {
	rels, err := p.rels("")
	if err != nil {
		return "", err
	}
	for _, r := range rels {
		if strings.HasSuffix(r.Type, "/officeDocument") {
			return resolveTarget("", r.Target), nil
		}
	}
	return "", errors.New("no workbook in the package")
}

type workbookXML struct {
	Sheets []struct {
		Name  string     `xml:"name,attr"`
		State string     `xml:"state,attr"`
		Attrs []xml.Attr `xml:",any,attr"`
	} `xml:"sheets>sheet"`
}

// sheets lists the workbook's sheets in order.
func (p *pkg) sheets() ([]sheetInfo, error) {
	wbPart, err := p.workbookPart()
	if err != nil {
		return nil, err
	}
	b, err := p.read(wbPart)
	if err != nil {
		return nil, err
	}
	var wb workbookXML
	if err := xml.Unmarshal(b, &wb); err != nil {
		return nil, fmt.Errorf("parse workbook: %w", err)
	}
	rels, err := p.rels(wbPart)
	if err != nil {
		return nil, err
	}
	byID := map[string]relationship{}
	for _, r := range rels {
		byID[r.ID] = r
	}
	out := make([]sheetInfo, 0, len(wb.Sheets))
	for _, s := range wb.Sheets {
		var rid string
		for _, a := range s.Attrs {
			if a.Name.Local == "id" && a.Name.Space != "" {
				rid = a.Value
			}
		}
		rel, ok := byID[rid]
		if !ok {
			return nil, fmt.Errorf("sheet %q: relationship %q missing", s.Name, rid)
		}
		info := sheetInfo{Name: s.Name, Part: resolveTarget(wbPart, rel.Target), Hidden: s.State == "hidden" || s.State == "veryHidden"}
		switch t := rel.Type; {
		case strings.HasSuffix(t, "/worksheet"):
			info.Kind = kindWorksheet
		case strings.HasSuffix(t, "/chartsheet"):
			info.Kind = kindChartsheet
		case strings.HasSuffix(t, "/dialogsheet"), strings.HasSuffix(t, "/xlMacrosheet"), strings.HasSuffix(t, "/xlIntlMacrosheet"):
			info.Kind = kindMacro
		default:
			info.Kind = kindOther
		}
		out = append(out, info)
	}
	return out, nil
}

// formulaCell is a cell with a formula, as saved in the file.
type formulaCell struct {
	Ref string
	// Type is the cell's t attribute: "" or "n" number (dates too), "str"
	// string, "b" boolean, "e" error, "d" ISO date.
	Type  string
	Value string
	// HasValue is false when the file has no cached value (no <v>): files
	// written by scripts that never ran Excel's calculation.
	HasValue bool
}

type cellXML struct {
	R string `xml:"r,attr"`
	T string `xml:"t,attr"`
	F *struct {
		Text string `xml:",chardata"`
	} `xml:"f"`
	V *string `xml:"v"`
}

// formulas streams a worksheet part and returns its formula cells.
func (p *pkg) formulas(part string) ([]formulaCell, error) {
	rc, err := p.open(part)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	d := xml.NewDecoder(rc)
	var out []formulaCell
	row, col := 0, 0
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("parse %s: %w", part, err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "row":
			row++
			for _, a := range se.Attr {
				if a.Name.Local == "r" {
					if n, err := strconv.Atoi(a.Value); err == nil {
						row = n
					}
				}
			}
			col = 0
		case "c":
			var c cellXML
			if err := d.DecodeElement(&c, &se); err != nil {
				return nil, fmt.Errorf("parse %s: %w", part, err)
			}
			col++
			ref := c.R
			if ref != "" {
				if cn, rn, err := excelize.CellNameToCoordinates(ref); err == nil {
					col, row = cn, rn
				}
			} else if ref, err = excelize.CoordinatesToCellName(col, row); err != nil {
				return nil, fmt.Errorf("%s: cell without reference: %w", part, err)
			}
			if c.F == nil {
				continue
			}
			fc := formulaCell{Ref: ref, Type: c.T}
			if c.V != nil {
				fc.Value, fc.HasValue = *c.V, true
			}
			out = append(out, fc)
		}
	}
}
