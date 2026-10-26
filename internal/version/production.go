//go:build production

package version

// Dev is false in release builds (compiled with the `production` tag).
const Dev = false
