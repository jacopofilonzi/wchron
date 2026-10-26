package apperr

// The registry of codes in use. Every code here has a row in ERRORS.md and an
// `error.<code>` text in both locales; tests fail otherwise. Add codes only
// when something returns them.
const (
	StorageUnavailable    Code = "storage.unavailable"
	StorageBackupFailed   Code = "storage.backup_failed"
	ConfigReadFailed      Code = "config.read_failed"
	ConfigWriteFailed     Code = "config.write_failed"
	ConfigNewerVersion    Code = "config.newer_version"
	ConfigMigrationFailed Code = "config.migration_failed"
	ConfigInvalid         Code = "config.invalid"
	StateReadFailed       Code = "state.read_failed"
	StateWriteFailed      Code = "state.write_failed"
	DevenvInvalid         Code = "devenv.invalid"
	AutostartFailed       Code = "autostart.failed"
	KeepAwakeFailed       Code = "keepawake.failed"
	LogsReadFailed        Code = "logs.read_failed"
	TokenUnclosed         Code = "token.unclosed"
	TokenUnexpectedClose  Code = "token.unexpected_close"
	TokenUnknown          Code = "token.unknown"
	TokenFormatInvalid    Code = "token.format_invalid"
	TokenOffsetInvalid    Code = "token.offset_invalid"
	TokenEmpty            Code = "token.empty"
	Internal              Code = "internal"
)

// All lists every registered code, for the consistency tests.
var All = []Code{
	StorageUnavailable,
	StorageBackupFailed,
	ConfigReadFailed,
	ConfigWriteFailed,
	ConfigNewerVersion,
	ConfigMigrationFailed,
	ConfigInvalid,
	StateReadFailed,
	StateWriteFailed,
	DevenvInvalid,
	AutostartFailed,
	KeepAwakeFailed,
	LogsReadFailed,
	TokenUnclosed,
	TokenUnexpectedClose,
	TokenUnknown,
	TokenFormatInvalid,
	TokenOffsetInvalid,
	TokenEmpty,
	Internal,
}
