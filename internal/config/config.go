// Package config is config.json: the user's settings, channels and schedules,
// edited from the GUI. It is durable user data (see AGENTS.md "Upgrading never
// loses data").
//
// Format (current version: CurrentVersion, see migrations.go), snake_case JSON:
//
//	{
//	  "schema_version": 1,
//	  "settings": {"error_recipients": [Recipient], "keep_awake_default": false,
//	               "language": "it" | "en", "theme": "system" | "light" | "dark"},
//	  "channels": [Channel],
//	  "schedules": [Schedule]
//	}
//
// Autostart is not stored here: the HKCU Run key is the only truth (the user
// can change it from Windows too). The time zone is always the system one.
//
// Loading (Load): a missing file gives the defaults (written on the first
// Save, never at startup); a file that can't be read or parsed is refused
// (config.read_failed) and left as is; a `schema_version` newer than the app is
// refused (config.newer_version) and never overwritten; an older one is
// migrated in memory (the caller saves it after the backup). After decoding,
// Normalize fills the fields missing in older or hand-written files (nil
// lists become empty, empty enums get their default) and Validate checks the
// structure only: unique non-empty ids, valid enums (config.invalid, with the
// JSON path of the field). The semantic checks (cadence values, tokens,
// recipient targets, processor options) belong to the scheduler, tokens,
// provider and processor packages and are added with their milestones: hook
// them into Validate, keeping structure checks here.
//
// Saving (Save) normalizes, validates and writes atomically (temp file in
// the same dir + fsync + rename, mode 0600): a failure leaves the previous
// file intact (config.write_failed).
package config

import (
	"encoding/json"

	"github.com/jacopofilonzi/wchron/internal/i18n"
)

// Theme is the UI colour scheme.
type Theme string

const (
	ThemeSystem Theme = "system"
	ThemeLight  Theme = "light"
	ThemeDark   Theme = "dark"
)

// Themes lists the valid themes.
var Themes = []Theme{ThemeSystem, ThemeLight, ThemeDark}

// Config is the whole config.json.
type Config struct {
	SchemaVersion int        `json:"schema_version"`
	Settings      Settings   `json:"settings"`
	Channels      []Channel  `json:"channels"`
	Schedules     []Schedule `json:"schedules"`
}

// Settings are the app-wide settings.
type Settings struct {
	// ErrorRecipients get the run error reports, on any channel (0 or more).
	ErrorRecipients []Recipient `json:"error_recipients"`
	// KeepAwakeDefault turns keep-awake on at every start.
	KeepAwakeDefault bool      `json:"keep_awake_default"`
	Language         i18n.Lang `json:"language"`
	Theme            Theme     `json:"theme"`
}

// Channel is one configured account of a provider (several per provider allowed).
type Channel struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	// Name is the user label (e.g. "WhatsApp personale").
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	// Settings are provider-specific and never secret: secrets and sessions
	// live in channels/<id>/.
	Settings json.RawMessage `json:"settings"`
}

// Recipient is a target on a channel.
type Recipient struct {
	ID        string `json:"id"`
	ChannelID string `json:"channel_id"`
	// Target is provider-specific (WhatsApp: {"kind":"number","number"} in
	// E.164 or {"kind":"group","jid"}).
	Target json.RawMessage `json:"target"`
	// DisplayName is the last known name, for display only.
	DisplayName string `json:"display_name"`
}

// Schedule sends its files to its recipients at the slots of its cadence.
type Schedule struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Enabled    bool        `json:"enabled"`
	Cadence    Cadence     `json:"cadence"`
	Recipients []Recipient `json:"recipients"`
	// Caption is an optional token template ("" for none).
	Caption string     `json:"caption"`
	Files   []FileSpec `json:"files"`
}

// CadenceMode selects how the slots are computed.
type CadenceMode string

const (
	CadenceInterval CadenceMode = "interval"
	CadenceFixed    CadenceMode = "fixed"
)

// Cadence describes the slots of a schedule (computed by the scheduler).
type Cadence struct {
	Mode CadenceMode `json:"mode"`
	// Interval and Anchor ("HH:MM") are used in interval mode.
	Interval Interval `json:"interval"`
	Anchor   string   `json:"anchor"`
	// Window limits interval mode to a time range; nil means all day.
	Window *Window `json:"window"`
	// Times ("HH:MM") are used in fixed mode.
	Times []string  `json:"times"`
	Days  []Weekday `json:"days"`
}

// Interval is the period of interval mode.
type Interval struct {
	Hours   int `json:"hours"`
	Minutes int `json:"minutes"`
}

// Window is a daily time range, "HH:MM" to "HH:MM".
type Window struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// Weekday is a lowercase three-letter day name.
type Weekday string

// Weekdays lists the valid days, Monday first.
var Weekdays = []Weekday{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}

// MatchMode says what to do when a glob matches several files.
type MatchMode string

const (
	MatchError  MatchMode = "error"
	MatchNewest MatchMode = "newest"
	MatchAll    MatchMode = "all"
)

// ProcessorPassthrough sends the file as is (the default processor).
const ProcessorPassthrough = "passthrough"

// FileSpec is one file (or glob) of a schedule.
type FileSpec struct {
	ID string `json:"id"`
	// PathPattern is an absolute path with tokens and/or a glob in the file name.
	PathPattern string    `json:"path_pattern"`
	Match       MatchMode `json:"match"`
	// Processor is a type from the processor registry ("passthrough",
	// "xlsx-sheets"…); its options are processor-specific.
	Processor        string          `json:"processor"`
	ProcessorOptions json.RawMessage `json:"processor_options"`
	// Caption overrides the schedule caption when set (nil: inherit).
	Caption *string `json:"caption,omitempty"`
}

// Default is the configuration of a fresh install.
func Default() Config {
	c := Config{
		SchemaVersion: CurrentVersion,
		Settings:      Settings{Language: i18n.Default, Theme: ThemeSystem},
	}
	c.Normalize()
	return c
}

var emptyObject = json.RawMessage(`{}`)

// Normalize fills what older or hand-written files may lack: nil lists
// become empty, empty enums get their default, missing raw objects become {}.
func (c *Config) Normalize() {
	if c.Settings.Language == "" {
		c.Settings.Language = i18n.Default
	}
	if c.Settings.Theme == "" {
		c.Settings.Theme = ThemeSystem
	}
	c.Settings.ErrorRecipients = normalizeRecipients(c.Settings.ErrorRecipients)
	if c.Channels == nil {
		c.Channels = []Channel{}
	}
	for i := range c.Channels {
		c.Channels[i].Settings = rawObject(c.Channels[i].Settings)
	}
	if c.Schedules == nil {
		c.Schedules = []Schedule{}
	}
	for i := range c.Schedules {
		s := &c.Schedules[i]
		s.Recipients = normalizeRecipients(s.Recipients)
		if s.Cadence.Times == nil {
			s.Cadence.Times = []string{}
		}
		if s.Cadence.Days == nil {
			s.Cadence.Days = []Weekday{}
		}
		if s.Files == nil {
			s.Files = []FileSpec{}
		}
		for j := range s.Files {
			f := &s.Files[j]
			if f.Match == "" {
				f.Match = MatchError
			}
			if f.Processor == "" {
				f.Processor = ProcessorPassthrough
			}
			f.ProcessorOptions = rawObject(f.ProcessorOptions)
		}
	}
}

func normalizeRecipients(rs []Recipient) []Recipient {
	if rs == nil {
		return []Recipient{}
	}
	for i := range rs {
		rs[i].Target = rawObject(rs[i].Target)
	}
	return rs
}

func rawObject(r json.RawMessage) json.RawMessage {
	if len(r) == 0 || string(r) == "null" {
		return emptyObject
	}
	return r
}
