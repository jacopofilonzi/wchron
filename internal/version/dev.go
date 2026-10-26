//go:build !production

package version

// Dev is true in dev builds (`make dev`, tests) and false in release builds,
// which are compiled with the `production` tag (see build/windows/Taskfile.yml).
const Dev = true
