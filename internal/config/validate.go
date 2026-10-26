package config

import (
	"fmt"
	"slices"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/i18n"
)

// Validate checks the structure of c (see the package doc): it returns a
// config.invalid error whose `field` param is the JSON path of the first
// offending field, e.g. `schedules[2].cadence.mode`.
func (c *Config) Validate() error {
	if !i18n.Valid(c.Settings.Language) {
		return invalid("settings.language", "unknown language %q", c.Settings.Language)
	}
	if !slices.Contains(Themes, c.Settings.Theme) {
		return invalid("settings.theme", "unknown theme %q", c.Settings.Theme)
	}
	if err := validateRecipients("settings.error_recipients", c.Settings.ErrorRecipients); err != nil {
		return err
	}
	ids := map[string]bool{}
	for i, ch := range c.Channels {
		p := fmt.Sprintf("channels[%d]", i)
		if err := uniqueID(ids, p, ch.ID); err != nil {
			return err
		}
		// Whether the provider exists is checked by the provider registry
		// (channel.provider_unknown), not here: an unknown provider must not
		// stop the app.
		if ch.Provider == "" {
			return invalid(p+".provider", "empty provider")
		}
	}
	ids = map[string]bool{}
	for i := range c.Schedules {
		if err := validateSchedule(fmt.Sprintf("schedules[%d]", i), &c.Schedules[i], ids); err != nil {
			return err
		}
	}
	return nil
}

func validateSchedule(p string, s *Schedule, ids map[string]bool) error {
	if err := uniqueID(ids, p, s.ID); err != nil {
		return err
	}
	switch s.Cadence.Mode {
	case CadenceInterval, CadenceFixed:
	default:
		return invalid(p+".cadence.mode", "unknown cadence mode %q", s.Cadence.Mode)
	}
	for j, d := range s.Cadence.Days {
		if !slices.Contains(Weekdays, d) {
			return invalid(fmt.Sprintf("%s.cadence.days[%d]", p, j), "unknown day %q", d)
		}
	}
	// Cadence values (interval range, HH:MM, window, days) are
	// scheduler.Validate (cadence.*, schedule.no_days): not checked on load,
	// so a bad hand edit only makes the scheduler ignore that schedule.
	if err := validateRecipients(p+".recipients", s.Recipients); err != nil {
		return err
	}
	files := map[string]bool{}
	for j, f := range s.Files {
		fp := fmt.Sprintf("%s.files[%d]", p, j)
		if err := uniqueID(files, fp, f.ID); err != nil {
			return err
		}
		switch f.Match {
		case MatchError, MatchNewest, MatchAll:
		default:
			return invalid(fp+".match", "unknown match mode %q", f.Match)
		}
		// Processor types and options: the processor registry (processor.*), checked by the editor and at run time.
	}
	return nil
}

func validateRecipients(p string, rs []Recipient) error {
	ids := map[string]bool{}
	for i, r := range rs {
		rp := fmt.Sprintf("%s[%d]", p, i)
		if err := uniqueID(ids, rp, r.ID); err != nil {
			return err
		}
		// Whether the channel exists is a run-time skip (recipient.channel_missing),
		// the target format is the provider's check (recipient.target_invalid).
		if r.ChannelID == "" {
			return invalid(rp+".channel_id", "empty channel id")
		}
		if !slices.Contains(VerifiedStates, r.Verified) {
			return invalid(rp+".verified", "unknown verified state %q", r.Verified)
		}
	}
	return nil
}

func uniqueID(seen map[string]bool, p, id string) error {
	if id == "" {
		return invalid(p+".id", "empty id")
	}
	if seen[id] {
		return invalid(p+".id", "duplicate id %q", id)
	}
	seen[id] = true
	return nil
}

func invalid(field, format string, args ...any) error {
	return apperr.New(apperr.ConfigInvalid, fmt.Errorf("%s: %s", field, fmt.Sprintf(format, args...))).With("field", field)
}
