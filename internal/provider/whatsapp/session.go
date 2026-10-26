package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"go.mau.fi/whatsmeow/store/sqlstore"
	waLog "go.mau.fi/whatsmeow/util/log"
	_ "modernc.org/sqlite" // the "sqlite" database/sql driver, pure Go

	"github.com/jacopofilonzi/wchron/internal/clock"
)

// SessionFile is the whatsmeow store in the channel dir.
const SessionFile = "session.db"

// dsn opens path with the pragmas whatsmeow needs: foreign keys (its
// Upgrade refuses to run without them) and a busy timeout. modernc.org/sqlite
// takes the query after a plain path, so Windows paths need no escaping.
func dsn(path string) string {
	return path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)"
}

// openSession opens dir/session.db and its first device. A file that can't
// be opened is set aside (see quarantine) and a fresh one is started:
// corrupt is then true. Only a failure on the fresh file is an error.
func openSession(ctx context.Context, dir string, clk clock.Clock, log waLog.Logger) (*realClient, bool, error) {
	path := filepath.Join(dir, SessionFile)
	rc, err := openStore(ctx, path, log)
	if err == nil {
		return rc, false, nil
	}
	if _, statErr := os.Stat(path); statErr != nil {
		return nil, false, err
	}
	if qerr := quarantine(path, clk); qerr != nil {
		return nil, false, errors.Join(err, qerr)
	}
	rc, err2 := openStore(ctx, path, log)
	if err2 != nil {
		return nil, false, fmt.Errorf("new session after a corrupt one (%w): %w", err, err2)
	}
	return rc, true, nil
}

func openStore(ctx context.Context, path string, log waLog.Logger) (*realClient, error) {
	container, err := sqlstore.New(ctx, "sqlite", dsn(path), log.Sub("Database"))
	if err != nil {
		return nil, fmt.Errorf("open session store: %w", err)
	}
	dev, err := container.GetFirstDevice(ctx)
	if err != nil {
		_ = container.Close()
		return nil, fmt.Errorf("read session device: %w", err)
	}
	return newRealClient(container, dev, log), nil
}

// quarantine renames a session that can't be opened to
// session.db.corrupt-<timestamp> (and its SQLite side files likewise),
// keeping it for inspection: it is never deleted by the app.
func quarantine(path string, clk clock.Clock) error {
	if clk == nil {
		clk = clock.Real{}
	}
	suffix := ".corrupt-" + clk.Now().Format("20060102-150405")
	for _, side := range []string{"", "-wal", "-shm", "-journal"} {
		p := path + side
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if err := os.Rename(p, p+suffix); err != nil {
			return fmt.Errorf("set aside %s: %w", filepath.Base(p), err)
		}
	}
	return nil
}
