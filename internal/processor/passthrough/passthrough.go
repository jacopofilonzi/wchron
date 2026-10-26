// Package passthrough is the default processor: the file unchanged, sent as
// a temp copy made at the slot (so the original is never touched and a file
// being rewritten is not read twice). It has no options.
package passthrough

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/processor"
	"github.com/jacopofilonzi/wchron/internal/resolver"
)

// Processor is the passthrough processor.
type Processor struct{}

// New returns the passthrough processor.
func New() Processor { return Processor{} }

func (Processor) Type() string                     { return config.ProcessorPassthrough }
func (Processor) OptionsSchema() []processor.Field { return []processor.Field{} }

// Extensions is empty: any file.
func (Processor) Extensions() []string { return []string{} }

// Validate accepts any options (there are none; extra keys are ignored).
func (Processor) Validate(json.RawMessage) error { return nil }

// OutputName is the original's name: the copy is unchanged.
func (Processor) OutputName(name string) string { return name }

// Process copies src into env.TmpDir.
func (Processor) Process(ctx context.Context, src processor.Source, _ json.RawMessage, env processor.Env) (processor.Output, func(), error) {
	dst, cleanup, err := Copy(ctx, src, env.TmpDir)
	if err != nil {
		return processor.Output{}, nil, err
	}
	return processor.Output{Path: dst, Name: src.Name}, cleanup, nil
}

// Copy copies the original src into dir under its own name, reading it with
// a shared open (file.locked, file.unreadable); a write failure is
// temp.unavailable. The returned cleanup removes the copy. A failed copy
// leaves nothing behind. Other processors start from it too.
func Copy(ctx context.Context, src processor.Source, dir string) (string, func(), error) {
	in, err := resolver.Open(src.Path)
	if err != nil {
		return "", nil, err
	}
	defer in.Close()
	dst := filepath.Join(dir, src.Name)
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", nil, apperr.New(apperr.TempUnavailable, fmt.Errorf("create temp copy: %w", err)).With("path", dir)
	}
	remove := func() { _ = os.Remove(dst) }
	_, cerr := io.Copy(out, readerCtx{ctx, in})
	if err := errors.Join(cerr, out.Close()); err != nil {
		remove()
		var re readErr
		if errors.As(err, &re) {
			if ctx.Err() != nil {
				return "", nil, ctx.Err()
			}
			return "", nil, resolver.ReadError(src.Path, re.err)
		}
		return "", nil, apperr.New(apperr.TempUnavailable, fmt.Errorf("write temp copy: %w", err)).With("path", dir)
	}
	return dst, remove, nil
}

// readErr marks a failure on the original's side of the copy.
type readErr struct{ err error }

func (e readErr) Error() string { return e.err.Error() }
func (e readErr) Unwrap() error { return e.err }

// readerCtx stops a long copy when the run is cancelled and tags read errors.
type readerCtx struct {
	ctx context.Context
	r   io.Reader
}

func (r readerCtx) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, readErr{err}
	}
	n, err := r.r.Read(p)
	if err != nil && err != io.EOF {
		err = readErr{err}
	}
	return n, err
}
