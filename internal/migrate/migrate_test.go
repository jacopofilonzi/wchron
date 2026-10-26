package migrate

import (
	"errors"
	"testing"
)

// Test-only steps: v1 has "name", v2 renames it to "title", v3 adds "tags".
var testSteps = []Step{
	func(doc map[string]any) error {
		doc["title"] = doc["name"]
		delete(doc, "name")
		return nil
	},
	func(doc map[string]any) error {
		if _, ok := doc["tags"]; !ok {
			doc["tags"] = []any{}
		}
		return nil
	},
}

func TestApplyRunsTheMissingSteps(t *testing.T) {
	r, err := Apply([]byte(`{"schema_version":1,"name":"Report"}`), testSteps)
	if err != nil {
		t.Fatal(err)
	}
	if r.From != 1 || !r.Migrated || r.Doc["title"] != "Report" || r.Doc["name"] != nil || r.Doc[VersionKey] != 3 {
		t.Fatalf("got %+v", r)
	}
	r, err = Apply([]byte(`{"schema_version":2,"title":"x"}`), testSteps)
	if err != nil || r.From != 2 || r.Doc["title"] != "x" || r.Doc["tags"] == nil {
		t.Fatalf("got %+v %v", r, err)
	}
}

func TestApplyCurrentVersionIsUnchanged(t *testing.T) {
	r, err := Apply([]byte(`{"schema_version":3,"title":"x"}`), testSteps)
	if err != nil || r.Migrated || r.From != 3 || len(r.Doc) != 2 {
		t.Fatalf("got %+v %v", r, err)
	}
	if Current(nil) != 1 || Current(testSteps) != 3 {
		t.Fatal("current version")
	}
}

func TestApplyMissingVersionIsOne(t *testing.T) {
	r, err := Apply([]byte(`{"name":"x"}`), testSteps)
	if err != nil || r.From != 1 || r.Doc["title"] != "x" {
		t.Fatalf("got %+v %v", r, err)
	}
}

func TestApplyNewer(t *testing.T) {
	_, err := Apply([]byte(`{"schema_version":4}`), testSteps)
	var ne *NewerError
	if !errors.As(err, &ne) || ne.Found != 4 || ne.Supported != 3 {
		t.Fatalf("err %v", err)
	}
}

func TestApplyStepFailure(t *testing.T) {
	steps := []Step{func(map[string]any) error { return errors.New("boom") }}
	_, err := Apply([]byte(`{"schema_version":1}`), steps)
	var se *StepError
	if !errors.As(err, &se) || se.From != 1 || se.To != 2 {
		t.Fatalf("err %v", err)
	}
}

func TestApplyBadFormat(t *testing.T) {
	for _, in := range []string{``, `nope`, `[]`, `null`, `{"schema_version":"1"}`, `{"schema_version":0}`, `{"schema_version":1.5}`} {
		if _, err := Apply([]byte(in), testSteps); !errors.Is(err, ErrFormat) {
			t.Errorf("%q: err %v", in, err)
		}
	}
}
