package apperr

// The registry of codes in use. Every code here has a row in ERRORS.md and an
// `error.<code>` text in both locales; tests fail otherwise. Add codes only
// when something returns them.
const (
	StorageUnavailable        Code = "storage.unavailable"
	StorageBackupFailed       Code = "storage.backup_failed"
	ConfigReadFailed          Code = "config.read_failed"
	ConfigWriteFailed         Code = "config.write_failed"
	ConfigNewerVersion        Code = "config.newer_version"
	ConfigMigrationFailed     Code = "config.migration_failed"
	ConfigInvalid             Code = "config.invalid"
	StateReadFailed           Code = "state.read_failed"
	StateWriteFailed          Code = "state.write_failed"
	DevenvInvalid             Code = "devenv.invalid"
	AutostartFailed           Code = "autostart.failed"
	KeepAwakeFailed           Code = "keepawake.failed"
	LogsReadFailed            Code = "logs.read_failed"
	TokenUnclosed             Code = "token.unclosed"
	TokenUnexpectedClose      Code = "token.unexpected_close"
	TokenUnknown              Code = "token.unknown"
	TokenFormatInvalid        Code = "token.format_invalid"
	TokenOffsetInvalid        Code = "token.offset_invalid"
	TokenEmpty                Code = "token.empty"
	CadenceIntervalInvalid    Code = "cadence.interval_invalid"
	CadenceAnchorInvalid      Code = "cadence.anchor_invalid"
	CadenceWindowInvalid      Code = "cadence.window_invalid"
	CadenceWindowEmpty        Code = "cadence.window_empty"
	CadenceNoTimes            Code = "cadence.no_times"
	CadenceTimeInvalid        Code = "cadence.time_invalid"
	CadenceTimeDuplicate      Code = "cadence.time_duplicate"
	ScheduleNoDays            Code = "schedule.no_days"
	SlotMissed                Code = "slot.missed"
	SlotLate                  Code = "slot.late"
	SlotDSTGap                Code = "slot.dst_gap"
	SlotDSTRepeat             Code = "slot.dst_repeat"
	SlotPaused                Code = "slot.paused"
	SlotOverlap               Code = "slot.overlap"
	ChannelNameRequired       Code = "channel.name_required"
	ChannelProviderUnknown    Code = "channel.provider_unknown"
	ChannelNotFound           Code = "channel.not_found"
	ChannelNotConnected       Code = "channel.not_connected"
	ChannelDisconnected       Code = "channel.disconnected"
	ChannelTargetsFailed      Code = "channel.targets_failed"
	ChannelLogoutFailed       Code = "channel.logout_failed"
	RecipientCheckUnavailable Code = "recipient.check_unavailable"
	RecipientTargetInvalid    Code = "recipient.target_invalid"
	RecipientCheckFailed      Code = "recipient.check_failed"
	WANumberInvalid           Code = "wa.number_invalid"
	WANotOnWhatsApp           Code = "wa.not_on_whatsapp"
	WAGroupNotFound           Code = "wa.group_not_found"
	WAClientOutdated          Code = "wa.client_outdated"
	WALoggedOut               Code = "wa.logged_out"
	WAPairTimeout             Code = "wa.pair_timeout"
	WAPairFailed              Code = "wa.pair_failed"
	WASessionCorrupt          Code = "wa.session_corrupt"
	WAStreamReplaced          Code = "wa.stream_replaced"
	WATemporaryBan            Code = "wa.temporary_ban"
	SendNotConnected          Code = "send.not_connected"
	SendTooLarge              Code = "send.too_large"
	SendUploadFailed          Code = "send.upload_failed"
	SendFailed                Code = "send.failed"
	SendRecipientInvalid      Code = "send.recipient_invalid"
	ScheduleNotFound          Code = "schedule.not_found"
	ScheduleBusy              Code = "schedule.busy"
	RecipientChannelMissing   Code = "recipient.channel_missing"
	FilespecPathRequired      Code = "filespec.path_required"
	FilespecPathNotAbsolute   Code = "filespec.path_not_absolute"
	FilespecGlobInDir         Code = "filespec.glob_in_dir"
	ProcessorUnknown          Code = "processor.unknown"
	ProcessorOptionsInvalid   Code = "processor.options_invalid"
	FileNotFound              Code = "file.not_found"
	FileMultipleMatches       Code = "file.multiple_matches"
	FileLocked                Code = "file.locked"
	FileUnreadable            Code = "file.unreadable"
	TempUnavailable           Code = "temp.unavailable"
	SendCaptionTooLong        Code = "send.caption_too_long"
	SendFilesUnsupported      Code = "send.files_unsupported"
	NotifyFailed              Code = "notify.failed"
	Internal                  Code = "internal"
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
	CadenceIntervalInvalid,
	CadenceAnchorInvalid,
	CadenceWindowInvalid,
	CadenceWindowEmpty,
	CadenceNoTimes,
	CadenceTimeInvalid,
	CadenceTimeDuplicate,
	ScheduleNoDays,
	SlotMissed,
	SlotLate,
	SlotDSTGap,
	SlotDSTRepeat,
	SlotPaused,
	SlotOverlap,
	ChannelNameRequired,
	ChannelProviderUnknown,
	ChannelNotFound,
	ChannelNotConnected,
	ChannelDisconnected,
	ChannelTargetsFailed,
	ChannelLogoutFailed,
	RecipientCheckUnavailable,
	RecipientTargetInvalid,
	RecipientCheckFailed,
	WANumberInvalid,
	WANotOnWhatsApp,
	WAGroupNotFound,
	WAClientOutdated,
	WALoggedOut,
	WAPairTimeout,
	WAPairFailed,
	WASessionCorrupt,
	WAStreamReplaced,
	WATemporaryBan,
	SendNotConnected,
	SendTooLarge,
	SendUploadFailed,
	SendFailed,
	SendRecipientInvalid,
	ScheduleNotFound,
	ScheduleBusy,
	RecipientChannelMissing,
	FilespecPathRequired,
	FilespecPathNotAbsolute,
	FilespecGlobInDir,
	ProcessorUnknown,
	ProcessorOptionsInvalid,
	FileNotFound,
	FileMultipleMatches,
	FileLocked,
	FileUnreadable,
	TempUnavailable,
	SendCaptionTooLong,
	SendFilesUnsupported,
	NotifyFailed,
	Internal,
}
