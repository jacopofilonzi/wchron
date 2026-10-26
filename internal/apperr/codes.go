package apperr

// The registry of codes in use. Every code here has a row in ERRORS.md and an
// `error.<code>` text in both locales; tests fail otherwise. Add codes only
// when something returns them.
const (
	AutostartFailed Code = "autostart.failed"
	KeepAwakeFailed Code = "keepawake.failed"
	Internal        Code = "internal"
)

// All lists every registered code, for the consistency tests.
var All = []Code{
	AutostartFailed,
	KeepAwakeFailed,
	Internal,
}
