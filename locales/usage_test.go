package locales

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// Keys built at run time from a prefix (`ui.nav.${page}`, "tray.channels."+state,
// error.<code>…): they can't be found as literals, so their prefix counts as a use.
var dynamicPrefixes = []string{"error.", "ui.nav.", "ui.day.", "ui.processor.", "tray.channels."}

// Provider schema labels: provider.<type>.<schema>.<field key>, read by the
// generic schema form even when a provider's own picker doesn't use them.
var schemaLabel = regexp.MustCompile(`^provider\.[a-z0-9-]+\.(target|settings)\.[a-z0-9_]+$`)

// Go calls with a literal key: i18n.T(lang, "tray.open", …).
var goLiteralKey = regexp.MustCompile(`i18n\.T\(\w+, "([^"]+)"`)

// sources returns the app's Go (no tests) and frontend sources.
func sources(t *testing.T) []string {
	t.Helper()
	var out []string
	add := func(root string, keep func(string) bool) {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if n := d.Name(); n == "node_modules" || n == "bindings" || n == "dist" {
					return filepath.SkipDir
				}
				return nil
			}
			if keep(p) {
				b, err := os.ReadFile(p)
				if err != nil {
					return err
				}
				out = append(out, string(b))
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	goSrc := func(p string) bool { return strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") }
	add(filepath.Join("..", "internal"), goSrc)
	add(filepath.Join("..", "main.go"), goSrc)
	add(filepath.Join("..", "frontend", "src"), func(p string) bool {
		return strings.HasSuffix(p, ".ts") || strings.HasSuffix(p, ".svelte")
	})
	return out
}

// TestNoUnusedOrMissingKeys flags catalog keys nothing uses (delete them from
// both files) and Go literal keys missing from the catalog. Missing frontend
// keys are caught by svelte-check (it types t() keys as keyof en).
func TestNoUnusedOrMissingKeys(t *testing.T) {
	b, err := FS.ReadFile("en.json")
	if err != nil {
		t.Fatal(err)
	}
	var en map[string]string
	if err := json.Unmarshal(b, &en); err != nil {
		t.Fatal(err)
	}
	src := strings.Join(sources(t), "\n")
	var unused []string
	for k := range en {
		if schemaLabel.MatchString(k) || slices.ContainsFunc(dynamicPrefixes, func(p string) bool { return strings.HasPrefix(k, p) }) {
			continue
		}
		if !strings.Contains(src, `"`+k+`"`) && !strings.Contains(src, "'"+k+"'") && !strings.Contains(src, "`"+k+"`") {
			unused = append(unused, k)
		}
	}
	slices.Sort(unused)
	if len(unused) > 0 {
		t.Errorf("unused keys in locales/*.json:\n%s", strings.Join(unused, "\n"))
	}
	for _, m := range goLiteralKey.FindAllStringSubmatch(src, -1) {
		if k := m[1]; !strings.HasSuffix(k, ".") {
			if _, ok := en[k]; !ok {
				t.Errorf("Go uses the missing key %q", k)
			}
		}
	}
}
