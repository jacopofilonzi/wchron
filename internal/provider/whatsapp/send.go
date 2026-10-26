package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/provider"
)

// retryDelay is the pause before the one retry of a send that failed on the
// network (a variable for the tests).
var retryDelay = 3 * time.Second

// Common office and archive types, so the MIME type doesn't depend on what
// the Windows registry says.
var mimeTypes = map[string]string{
	".pdf":  "application/pdf",
	".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	".xlsm": "application/vnd.ms-excel.sheet.macroEnabled.12",
	".xls":  "application/vnd.ms-excel",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
	".doc":  "application/msword",
	".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	".csv":  "text/csv",
	".txt":  "text/plain",
	".zip":  "application/zip",
	".png":  "image/png",
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
}

func mimeFor(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if t, ok := mimeTypes[ext]; ok {
		return t
	}
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	return "application/octet-stream"
}

// formatSize is a size for the texts: "12.3 MB", "512 KB", "800 B".
func formatSize(n int64) string {
	switch {
	case n >= 1<<20:
		return strconv.FormatFloat(float64(n)/(1<<20), 'f', 1, 64) + " MB"
	case n >= 1<<10:
		return strconv.FormatInt(n>>10, 10) + " KB"
	default:
		return strconv.FormatInt(n, 10) + " B"
	}
}

// Send delivers msg: see the package doc. Only the pipeline and notify call it.
func (c *channel) Send(ctx context.Context, raw json.RawMessage, msg provider.Message) error {
	t, err := parseTarget(raw)
	if err != nil {
		return err
	}
	if !c.connected() {
		return apperr.New(apperr.SendNotConnected, errors.New("whatsapp not connected"))
	}
	jid := t.jid()
	if len(msg.Attachments) == 0 {
		return c.withRetry(ctx, "", func() error { return c.cli.SendText(ctx, jid, msg.Text) })
	}
	for i, a := range msg.Attachments {
		name := a.Name
		if name == "" {
			name = filepath.Base(a.Path)
		}
		info, err := os.Stat(a.Path)
		if err != nil {
			return apperr.New(apperr.SendFailed, fmt.Errorf("stat attachment: %w", err)).With("file", name)
		}
		if info.Size() > MaxFileSize {
			return apperr.New(apperr.SendTooLarge, fmt.Errorf("%d bytes over the limit", info.Size())).
				With("file", name).With("size", formatSize(info.Size())).With("max", formatSize(MaxFileSize))
		}
		data, err := os.ReadFile(a.Path)
		if err != nil {
			return apperr.New(apperr.SendFailed, fmt.Errorf("read attachment: %w", err)).With("file", name)
		}
		doc := document{fileName: name, mime: a.MIME}
		if doc.mime == "" {
			doc.mime = mimeFor(name)
		}
		if i == 0 {
			doc.caption = msg.Text
		}
		if err := c.withRetry(ctx, name, func() error { return c.cli.SendDocument(ctx, jid, data, doc) }); err != nil {
			return err
		}
	}
	return nil
}

// withRetry runs send once more after a short pause when it failed on the
// network, then maps the failure to its send.* code.
func (c *channel) withRetry(ctx context.Context, file string, send func() error) error {
	err := send()
	var se *sendError
	if err != nil && errors.As(err, &se) && se.network {
		c.log.Info("whatsapp send failed on the network, retrying once", "err", err)
		select {
		case <-time.After(retryDelay):
		case <-ctx.Done():
			return apperr.New(apperr.SendFailed, ctx.Err()).With("file", file)
		}
		err = send()
	}
	if err == nil {
		return nil
	}
	code := apperr.SendFailed
	cause := err
	if errors.As(err, &se) {
		switch {
		case se.recipient:
			code = apperr.SendRecipientInvalid
			cause = fmt.Errorf("%w: %w", err, provider.ErrTargetInvalid)
		case se.upload:
			code = apperr.SendUploadFailed
		}
	}
	e := apperr.New(code, cause)
	if file != "" {
		e = e.With("file", file)
	}
	return e
}
