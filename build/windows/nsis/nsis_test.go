// Package nsis holds the per-user installer script; this test guards its
// data-safety rule (AGENTS.md "Upgrades never delete data"): installing over an
// existing install, by hand or silently (/S, the self-update), never removes
// user data. Only the explicit "uninstall" section deletes anything.
package nsis

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// Statements that remove something or run another program.
var destructive = regexp.MustCompile(`(?i)^\s*(RMDir|Delete|DeleteRegKey|DeleteRegValue|ExecWait|Exec|ExecShell|ExecShellWait|nsExec::\w+)\b`)

func TestOnlyTheUninstallSectionDeletes(t *testing.T) {
	b, err := os.ReadFile("project.nsi")
	if err != nil {
		t.Fatal(err)
	}
	inUninstall, sawUninstall := false, false
	for n, line := range strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
		code := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(code, `Section "uninstall"`):
			inUninstall, sawUninstall = true, true
			continue
		case strings.HasPrefix(code, "SectionEnd"):
			inUninstall = false
			continue
		}
		if inUninstall || strings.HasPrefix(code, "#") {
			continue
		}
		// closeRunningApp (install and uninstall .onInit) only lists and
		// closes the running app: tasklist / taskkill through nsExec.
		if destructive.MatchString(code) && !strings.Contains(code, "tasklist.exe") && !strings.Contains(code, "taskkill.exe") {
			t.Errorf("line %d removes or runs something outside the uninstall section: %s", n+1, code)
		}
		if strings.Contains(strings.ToLower(code), "uninstall.exe") || strings.Contains(code, "wails.deleteUninstaller") {
			t.Errorf("line %d runs or removes the uninstaller outside the uninstall section: %s", n+1, code)
		}
	}
	if !sawUninstall {
		t.Fatal(`no Section "uninstall"`)
	}
	// What the uninstall section must remove (README "Uninstall").
	for _, want := range []string{
		`RMDir /r "$APPDATA\${INFO_PROJECTNAME}"`,
		`RMDir /r "$TEMP\${INFO_PROJECTNAME}"`,
		`DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "${INFO_PROJECTNAME}"`,
		`DeleteRegKey HKCU "Software\Classes\AppUserModelId\${INFO_PROJECTNAME}"`,
		`DeleteRegKey HKCU "Software\Classes\CLSID\$0"`,
	} {
		if !strings.Contains(string(b), want) {
			t.Errorf("the uninstall section must contain %s", want)
		}
	}
}
