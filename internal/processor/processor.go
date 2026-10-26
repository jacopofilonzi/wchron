// Package processor turns a resolved original into the file that is sent.
//
// A Processor is chosen by config.FileSpec.Processor through a Registry
// (wired in main.go, like the providers: a processor sub-package imports this
// one, so this package can't import them). Each processor exposes an options
// schema (OptionsSchema) that the GUI renders, so a new processor needs
// minimal frontend work, and checks its options with Validate
// (processor.options_invalid). An unknown type is processor.unknown.
//
// Process always works on a copy in the run's temp dir (Env.TmpDir, under
// %TEMP%\wchron\<run-id>\, created by the pipeline): the original is never
// modified, and a file being rewritten at the slot is copied once and sent
// as that copy. It returns a cleanup func that removes what it wrote; the
// cleanup is non-nil whenever the error is nil, the pipeline always calls it
// (and also removes the whole run dir at the end, so a failed Process leaves
// nothing behind either). A temp dir that can't be written is
// temp.unavailable; an original that can't be read is file.locked /
// file.unreadable (resolver.Open).
//
// Processors (labels: ui.processor.<type>.name / .description):
//   - passthrough (default; "Original" in the GUI): the file unchanged,
//     through the temp copy. Any file type.
//   - xlsx-snapshot ("Values only"): an Excel workbook without formulas or
//     macros, reduced to the chosen sheets, always saved as .xlsx (package
//     xlsxsnapshot). Only for the Extensions it lists.
//
// Output.Name is the processed file's own name (e.g. "report.xlsx" for
// "report.xlsm"); the pipeline then applies the spec's send name
// (resolver.SendName).
package processor

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/i18n"
)

// Source is the resolved original.
type Source struct {
	Path string
	Name string
}

// Env is what a processor gets besides the source and its options.
type Env struct {
	// TmpDir is an empty folder of this file in the run's temp dir.
	TmpDir string
	// At is the slot time, for options with tokens (sheet names); Lang the
	// language they are rendered in.
	At   time.Time
	Lang i18n.Lang
}

// Output is the file to send.
type Output struct {
	// Path is the processed copy in Env.TmpDir; Name the name the recipient sees.
	Path string
	Name string
	// Warnings are problems that don't stop the send (e.g.
	// xlsx.no_cached_values).
	Warnings []*apperr.Error
}

// FieldKind is the input an option needs.
type FieldKind string

const (
	// FieldText is one token template.
	FieldText FieldKind = "text"
	// FieldList is a list of token templates (e.g. sheet names).
	FieldList FieldKind = "list"
)

// Field describes one option for the GUI; its label is the i18n key
// ui.processor.<type>.<key>.
type Field struct {
	Key      string    `json:"key"`
	Kind     FieldKind `json:"kind"`
	Required bool      `json:"required"`
	// Tokens says the value is a token template (the GUI shows the token
	// hint and the live preview).
	Tokens bool `json:"tokens"`
}

// Processor is one kind of processing.
type Processor interface {
	// Type is the id stored in config.FileSpec.Processor.
	Type() string
	// Extensions are the file extensions it handles (lowercase, with the
	// dot); empty means any file. For the editor: at run time a file of
	// another type is the processor's own error.
	Extensions() []string
	OptionsSchema() []Field
	// OutputName is the name Process gives the output of an original named
	// name (e.g. "report.xlsx" for "report.xlsm"), for the editor's preview of
	// the send names without processing anything.
	OutputName(name string) string
	// Validate checks the options (processor.options_invalid, or the
	// processor's own codes).
	Validate(opts json.RawMessage) error
	// Process makes the file to send from src (see the package doc).
	Process(ctx context.Context, src Source, opts json.RawMessage, env Env) (Output, func(), error)
}

// Registry holds the processors by type.
type Registry struct {
	byType map[string]Processor
	order  []string
}

// NewRegistry returns a registry with ps, in that order.
func NewRegistry(ps ...Processor) *Registry {
	r := &Registry{byType: map[string]Processor{}}
	for _, p := range ps {
		if _, dup := r.byType[p.Type()]; !dup {
			r.order = append(r.order, p.Type())
		}
		r.byType[p.Type()] = p
	}
	return r
}

// Get returns the processor of a type, or processor.unknown.
func (r *Registry) Get(typ string) (Processor, error) {
	p, ok := r.byType[typ]
	if !ok {
		return nil, apperr.New(apperr.ProcessorUnknown, fmt.Errorf("unknown processor %q", typ)).With("type", typ)
	}
	return p, nil
}

// Schema is a processor's options schema for the GUI.
type Schema struct {
	Type string `json:"type"`
	// Extensions the processor handles (empty: any file).
	Extensions []string `json:"extensions"`
	Options    []Field  `json:"options"`
}

// Schemas returns every processor's schema, in registration order.
func (r *Registry) Schemas() []Schema {
	out := make([]Schema, 0, len(r.order))
	for _, t := range r.order {
		p := r.byType[t]
		ext := p.Extensions()
		if ext == nil {
			ext = []string{}
		}
		out = append(out, Schema{Type: t, Extensions: ext, Options: p.OptionsSchema()})
	}
	return out
}
