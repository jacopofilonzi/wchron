package xlsxsnapshot

// Final pass on the package excelize wrote. Saved as .xlsx, excelize sets the
// workbook's content type to the plain (macro-free) one but keeps every other
// part it loaded: the VBA project, its signature, the parts of deleted chart
// or macro sheets, and their content-type overrides and relationships. Excel
// refuses an .xlsx with a VBA project and reports dangling relationships as
// corruption, so this pass:
//   - drops the VBA parts (vbaProject*.bin, vbaData.xml) whatever references them;
//   - drops every internal relationship whose target part no longer exists,
//     and every VBA relationship;
//   - drops content-type overrides of parts that no longer exist (and VBA
//     ones), and the "bin" default when it is the VBA type (replaced by the
//     printer-settings type if other .bin parts remain without an override).
// Parts that nothing references any more (an orphaned chart of a removed
// sheet, say) are left alone: harmless to Excel.

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"
)

const (
	contentTypeVBA        = "application/vnd.ms-office.vbaProject"
	contentTypePrinter    = "application/vnd.openxmlformats-officedocument.spreadsheetml.printerSettings"
	contentTypesPart      = "[Content_Types].xml"
	nsContentTypes        = "http://schemas.openxmlformats.org/package/2006/content-types"
	nsRelationships       = "http://schemas.openxmlformats.org/package/2006/relationships"
	relTypeVBASuffix      = "/vbaProject"
	relTypeVBASigSuffix   = "/vbaProjectSignature"
	xmlHeader             = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"
	targetModeExternalStr = "External"
)

// isMacroPart reports whether a zip entry is VBA content.
func isMacroPart(name string) bool {
	base := strings.ToLower(path.Base(name))
	return (strings.HasPrefix(base, "vbaproject") && strings.HasSuffix(base, ".bin")) || base == "vbadata.xml"
}

type contentTypes struct {
	Defaults []struct {
		Extension   string `xml:"Extension,attr"`
		ContentType string `xml:"ContentType,attr"`
	} `xml:"Default"`
	Overrides []struct {
		PartName    string `xml:"PartName,attr"`
		ContentType string `xml:"ContentType,attr"`
	} `xml:"Override"`
}

// cleanPackage applies the final pass to a written workbook.
func cleanPackage(data []byte) ([]byte, error) {
	src, err := openPackage(data)
	if err != nil {
		return nil, err
	}
	// The parts that remain.
	keep := &pkg{zr: src.zr, files: map[string]*zip.File{}}
	for _, f := range src.zr.File {
		if !isMacroPart(f.Name) {
			keep.files[strings.ToLower(f.Name)] = f
		}
	}
	replaced := map[string][]byte{}
	for _, f := range src.zr.File {
		if isMacroPart(f.Name) || !strings.HasSuffix(strings.ToLower(f.Name), ".rels") {
			continue
		}
		b, changed, err := cleanRels(keep, f.Name)
		if err != nil {
			return nil, err
		}
		if changed {
			replaced[f.Name] = b
		}
	}
	ct, err := cleanContentTypes(keep)
	if err != nil {
		return nil, err
	}
	replaced[contentTypesPart] = ct

	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	for _, f := range src.zr.File {
		if isMacroPart(f.Name) {
			continue
		}
		b, ok := replaced[f.Name]
		if !ok {
			if err := zw.Copy(f); err != nil {
				return nil, fmt.Errorf("copy %s: %w", f.Name, err)
			}
			continue
		}
		w, err := zw.Create(f.Name)
		if err == nil {
			_, err = w.Write(b)
		}
		if err != nil {
			return nil, fmt.Errorf("write %s: %w", f.Name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close package: %w", err)
	}
	return out.Bytes(), nil
}

// cleanRels drops the VBA and dangling internal relationships of a .rels part.
func cleanRels(p *pkg, name string) ([]byte, bool, error) {
	b, err := p.read(name)
	if err != nil {
		return nil, false, err
	}
	var rs relationships
	if err := xml.Unmarshal(b, &rs); err != nil {
		return nil, false, fmt.Errorf("parse %s: %w", name, err)
	}
	source := sourceOfRels(name)
	kept := rs.Rels[:0:0]
	for _, r := range rs.Rels {
		vba := strings.HasSuffix(r.Type, relTypeVBASuffix) || strings.HasSuffix(r.Type, relTypeVBASigSuffix)
		dangling := r.TargetMode != targetModeExternalStr && !p.has(resolveTarget(source, r.Target))
		if !vba && !dangling {
			kept = append(kept, r)
		}
	}
	if len(kept) == len(rs.Rels) {
		return nil, false, nil
	}
	var w bytes.Buffer
	w.WriteString(xmlHeader)
	w.WriteString(`<Relationships xmlns="` + nsRelationships + `">`)
	for _, r := range kept {
		w.WriteString(`<Relationship Id="` + attr(r.ID) + `" Type="` + attr(r.Type) + `" Target="` + attr(r.Target) + `"`)
		if r.TargetMode != "" {
			w.WriteString(` TargetMode="` + attr(r.TargetMode) + `"`)
		}
		w.WriteString(`/>`)
	}
	w.WriteString(`</Relationships>`)
	return w.Bytes(), true, nil
}

// cleanContentTypes rewrites [Content_Types].xml for the remaining parts.
func cleanContentTypes(p *pkg) ([]byte, error) {
	b, err := p.read(contentTypesPart)
	if err != nil {
		return nil, err
	}
	var ct contentTypes
	if err := xml.Unmarshal(b, &ct); err != nil {
		return nil, fmt.Errorf("parse content types: %w", err)
	}
	overridden := map[string]bool{}
	var w bytes.Buffer
	w.WriteString(xmlHeader)
	w.WriteString(`<Types xmlns="` + nsContentTypes + `">`)
	var overrides bytes.Buffer
	for _, o := range ct.Overrides {
		part := strings.TrimPrefix(o.PartName, "/")
		if strings.HasPrefix(o.ContentType, contentTypeVBA) || !p.has(part) {
			continue
		}
		overridden[strings.ToLower(part)] = true
		overrides.WriteString(`<Override PartName="` + attr(o.PartName) + `" ContentType="` + attr(o.ContentType) + `"/>`)
	}
	needBin := false
	for name, f := range p.files {
		if strings.HasSuffix(name, ".bin") && !overridden[name] && !isMacroPart(f.Name) {
			needBin = true
		}
	}
	seen := map[string]bool{}
	for _, d := range ct.Defaults {
		ext := strings.ToLower(d.Extension)
		if seen[ext] {
			continue
		}
		typ := d.ContentType
		if ext == "bin" && strings.HasPrefix(typ, contentTypeVBA) {
			if !needBin {
				continue
			}
			typ = contentTypePrinter
		}
		seen[ext] = true
		w.WriteString(`<Default Extension="` + attr(d.Extension) + `" ContentType="` + attr(typ) + `"/>`)
	}
	if needBin && !seen["bin"] {
		w.WriteString(`<Default Extension="bin" ContentType="` + contentTypePrinter + `"/>`)
	}
	_, _ = io.Copy(&w, &overrides)
	w.WriteString(`</Types>`)
	return w.Bytes(), nil
}

// attr escapes an XML attribute value.
func attr(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
