package app

// Settings bindings beyond the Snapshot toggles: the error recipients, the
// data and log folders, the last backup.

import (
	"errors"
	"strings"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/backup"
	"github.com/jacopofilonzi/wchron/internal/config"
)

// ErrorRecipients returns the recipients of the error reports.
func (s *Service) ErrorRecipients() []config.Recipient {
	return s.store.Config().Settings.ErrorRecipients
}

// SaveErrorRecipients replaces the error recipients (0 or more, on any
// channel). Problems (recipient.channel_missing, recipient.duplicate, field
// "error_recipients.<id>") come back in the result and nothing is saved;
// errors are config.write_failed.
func (s *Service) SaveErrorRecipients(rs []config.Recipient) ([]FieldProblem, error) {
	wrap := config.Config{Settings: config.Settings{ErrorRecipients: rs}}
	wrap.Normalize()
	rs = wrap.Settings.ErrorRecipients
	if ps := s.validator().recipients("error_recipients", "", rs); len(ps) > 0 {
		return ps, nil
	}
	err := s.store.UpdateConfig(func(c *config.Config) error {
		c.Settings.ErrorRecipients = rs
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.log.Info("error recipients saved", "count", len(rs))
	return []FieldProblem{}, nil
}

// DataDir is the data folder, for Settings.
func (s *Service) DataDir() string { return s.dataDir }

// OpenDataDir shows the data folder in Explorer (folder.open_failed).
func (s *Service) OpenDataDir() error { return s.open(s.dataDir) }

// OpenLogsDir shows the log folder in Explorer (folder.open_failed).
func (s *Service) OpenLogsDir() error { return s.open(s.logsDir) }

func (s *Service) open(path string) error {
	if strings.TrimSpace(path) == "" || s.openFolder == nil {
		return apperr.New(apperr.FolderOpenFailed, errors.New("no folder or opener configured")).With("path", path)
	}
	if err := s.openFolder(path); err != nil {
		return apperr.New(apperr.FolderOpenFailed, err).With("path", path)
	}
	return nil
}

// LastBackup is the time of the most recent backup of the data, null when
// there is none (or the folder can't be read: only logged).
func (s *Service) LastBackup() *time.Time {
	if s.backupsDir == "" {
		return nil
	}
	t, ok, err := backup.Latest(s.backupsDir, s.loc)
	if err != nil {
		s.log.Warn("read backups", "err", err)
	}
	if !ok {
		return nil
	}
	return &t
}
