package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jacopofilonzi/wchron/internal/apperr"
	"github.com/jacopofilonzi/wchron/internal/clock"
	"github.com/jacopofilonzi/wchron/internal/config"
	"github.com/jacopofilonzi/wchron/internal/logging"
	"github.com/jacopofilonzi/wchron/internal/processor"
	"github.com/jacopofilonzi/wchron/internal/processor/passthrough"
	"github.com/jacopofilonzi/wchron/internal/processor/xlsxsnapshot"
	"github.com/jacopofilonzi/wchron/internal/provider"
	"github.com/jacopofilonzi/wchron/internal/state"
)

// schedEnv is a Service over a store with one channel, the real processors
// and a fake clock at Monday 26 October 2026 09:00 (Europe/Rome).
type schedEnv struct {
	svc     *Service
	st      *Store
	dir     string // files the patterns point at
	changed []string
	saved   int // config saves
}

func newSchedEnv(t *testing.T) *schedEnv {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		t.Fatal(err)
	}
	e := &schedEnv{dir: t.TempDir()}
	cfg := config.Default()
	cfg.Channels = []config.Channel{{ID: "ch", Provider: "fake", Name: "Phone", Enabled: true, Settings: json.RawMessage(`{}`)}}
	e.st = NewStore(Deps{Inhibitor: &fakeInhibitor{}, Autostart: &fakeAutostart{}, Config: cfg, State: state.Default(),
		SaveConfig: func(config.Config) error { e.saved++; return nil }})
	e.svc = NewService(e.st, provider.NewRegistry(), nil, nil, t.TempDir(), clock.NewFake(time.Date(2026, 10, 26, 9, 0, 0, 0, loc)))
	e.svc.loc = loc
	Configure(e.svc, Extras{Processors: processor.NewRegistry(passthrough.New(), xlsxsnapshot.New()),
		Changed: func(id string) { e.changed = append(e.changed, id) }})
	return e
}

func (e *schedEnv) file(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join(e.dir, name)
	if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// valid is a schedule that passes every check.
func (e *schedEnv) valid(t *testing.T) config.Schedule {
	sc := e.svc.NewSchedule()
	sc.Name = "Sales"
	sc.Recipients = []config.Recipient{{ID: "r1", ChannelID: "ch", Target: json.RawMessage(`{"to":"a"}`), DisplayName: "Anna", Verified: config.VerifiedYes}}
	f := e.svc.NewFileSpec()
	f.PathPattern = e.file(t, "report.pdf")
	sc.Files = []config.FileSpec{f}
	sc.Caption = "Report {date:DD-MM-YYYY}"
	return sc
}

func fields(ps []FieldProblem) string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Field+"="+string(p.Code))
	}
	return strings.Join(out, " ")
}

func TestSaveScheduleNewAndUpdate(t *testing.T) {
	e := newSchedEnv(t)
	sc := e.valid(t)
	sc.Name = "  Sales  "
	res, err := e.svc.SaveSchedule(sc)
	if err != nil || len(res.Problems) != 0 || res.Schedule == nil || res.Schedule.ID == "" || res.Schedule.Name != "Sales" {
		t.Fatalf("save: %+v %v", res, err)
	}
	id := res.Schedule.ID
	if len(e.changed) != 1 || e.changed[0] != id || e.saved != 1 {
		t.Fatalf("changed %v saved %d", e.changed, e.saved)
	}
	list := e.svc.ListSchedules()
	// Monday 09:00, every hour on weekdays from 00:00: the next is 10:00.
	if len(list) != 1 || list[0].Schedule.ID != id || list[0].NextRun == nil ||
		list[0].NextRun.Format("15:04") != "10:00" || list[0].LastResult != "" {
		t.Fatalf("list %+v", list)
	}
	got, err := e.svc.GetSchedule(id)
	if err != nil || got.Name != "Sales" {
		t.Fatalf("get %+v %v", got, err)
	}

	got.Name = "Sales North"
	got.Enabled = false
	if res, err = e.svc.SaveSchedule(got); err != nil || len(res.Problems) != 0 {
		t.Fatalf("update %+v %v", res, err)
	}
	list = e.svc.ListSchedules()
	if len(list) != 1 || list[0].Schedule.Name != "Sales North" || list[0].NextRun != nil {
		t.Fatalf("after update %+v", list)
	}
	got.ID = "gone"
	if _, err := e.svc.SaveSchedule(got); apperr.As(err).Code != apperr.ScheduleNotFound {
		t.Fatalf("missing id: %v", err)
	}
	if _, err := e.svc.GetSchedule("gone"); apperr.As(err).Code != apperr.ScheduleNotFound {
		t.Fatalf("get missing: %v", err)
	}
}

func TestSaveScheduleProblems(t *testing.T) {
	e := newSchedEnv(t)
	empty := e.svc.NewSchedule()
	res, err := e.svc.SaveSchedule(empty)
	if err != nil || res.Schedule != nil {
		t.Fatalf("%+v %v", res, err)
	}
	if got := fields(res.Problems); got != "name=schedule.name_required recipients=recipient.required files=schedule.no_files" {
		t.Fatalf("empty: %s", got)
	}
	if len(e.changed) != 0 || e.saved != 0 {
		t.Fatal("nothing must be saved")
	}

	sc := e.valid(t)
	sc.Cadence.Interval = config.Interval{}
	sc.Caption = "{date"
	r1 := sc.Recipients[0]
	dup := r1
	dup.ID = "r2"
	dup.Target = json.RawMessage(`{ "to": "a" }`)
	lost := r1
	lost.ID, lost.ChannelID = "r3", "deleted"
	sc.Recipients = append(sc.Recipients, dup, lost)
	f := sc.Files[0]
	rel, glob, xlsPDF, badSheet, badCaption, badName, unknown := f, f, f, f, f, f, f
	rel.ID, rel.PathPattern = "rel", "report.pdf"
	glob.ID, glob.PathPattern = "glob", `C:\*\a.pdf`
	xlsPDF.ID, xlsPDF.Processor = "xls", xlsxsnapshot.Type
	badSheet.ID, badSheet.Processor, badSheet.PathPattern = "sheet", xlsxsnapshot.Type, `C:\r.xlsx`
	badSheet.ProcessorOptions = json.RawMessage(`{"sheets": [""]}`)
	caption := "{date:QQ}"
	badCaption.ID, badCaption.Caption = "cap", &caption
	badName.ID, badName.SendName = "name", "a:b"
	unknown.ID, unknown.Processor = "unk", "magic"
	sc.Files = append(sc.Files, rel, glob, xlsPDF, badSheet, badCaption, badName, unknown)
	res, err = e.svc.SaveSchedule(sc)
	if err != nil {
		t.Fatal(err)
	}
	want := "cadence.interval=cadence.interval_invalid recipients.r2=recipient.duplicate recipients.r3=recipient.channel_missing " +
		"caption=token.unclosed files.rel.path_pattern=filespec.path_not_absolute files.glob.path_pattern=filespec.glob_in_dir " +
		"files.xls.processor=processor.extension_unsupported files.sheet.processor_options=processor.options_invalid " +
		"files.cap.caption=token.format_invalid files.name.send_name=filespec.send_name_invalid files.unk.processor=processor.unknown"
	if got := fields(res.Problems); got != want {
		t.Fatalf("problems:\n got %s\nwant %s", got, want)
	}
	for _, p := range res.Problems {
		if p.Field == "files.xls.processor" && p.Params["ext"] != ".pdf" {
			t.Errorf("ext param: %+v", p)
		}
		if p.Field == "recipients.r3" && (p.Params["recipient"] != "Anna" || p.Params["schedule"] != "Sales") {
			t.Errorf("channel missing params: %+v", p)
		}
	}

	// Cadence codes land under their field.
	for _, c := range []struct {
		change func(*config.Cadence)
		want   string
	}{
		{func(c *config.Cadence) { c.Anchor = "25:00" }, "cadence.anchor=cadence.anchor_invalid"},
		{func(c *config.Cadence) { c.Window = &config.Window{Start: "08:00", End: "08:00"} }, "cadence.window=cadence.window_invalid"},
		{func(c *config.Cadence) { c.Days = nil }, "cadence.days=schedule.no_days"},
		{func(c *config.Cadence) { c.Mode, c.Times = config.CadenceFixed, nil }, "cadence.times=cadence.no_times"},
		{func(c *config.Cadence) { c.Mode, c.Times = config.CadenceFixed, []string{"8:00"} }, "cadence.times=cadence.time_invalid"},
	} {
		sc := e.valid(t)
		c.change(&sc.Cadence)
		res, _ := e.svc.SaveSchedule(sc)
		if got := fields(res.Problems); got != c.want {
			t.Errorf("got %s, want %s", got, c.want)
		}
	}
	// A wildcard or a token in the extension is checked only at the slot.
	sc = e.valid(t)
	sc.Files[0].Processor = xlsxsnapshot.Type
	for _, p := range []string{`C:\r\report.*`, `C:\r\report.{date:YYYY}`, `C:\r\report.xlsm`} {
		sc.Files[0].PathPattern = p
		if res, _ := e.svc.SaveSchedule(sc); len(res.Problems) != 0 {
			t.Errorf("%s: %s", p, fields(res.Problems))
		}
	}
}

func TestDuplicateDeleteEnable(t *testing.T) {
	e := newSchedEnv(t)
	a, _ := e.svc.SaveSchedule(e.valid(t))
	other := e.valid(t)
	other.Name = "Other"
	b, _ := e.svc.SaveSchedule(other)
	id := a.Schedule.ID
	e.st.UpdateScheduleState(id, func(s *state.ScheduleState) { s.LastResult = state.ResultFailed })

	cp, err := e.svc.DuplicateSchedule(id)
	if err != nil || cp.ID == id || cp.Name != "Sales (copia)" || cp.Enabled || len(cp.Files) != 1 || len(cp.Recipients) != 1 {
		t.Fatalf("copy %+v %v", cp, err)
	}
	names := []string{}
	for _, s := range e.svc.ListSchedules() {
		names = append(names, s.Schedule.Name)
	}
	if strings.Join(names, ",") != "Sales,Sales (copia),Other" {
		t.Fatalf("order %v", names)
	}
	if _, err := e.svc.DuplicateSchedule("x"); apperr.As(err).Code != apperr.ScheduleNotFound {
		t.Fatalf("dup missing: %v", err)
	}

	if err := e.svc.DeleteSchedule(id); err != nil {
		t.Fatal(err)
	}
	if _, ok := e.st.st.Schedules[id]; ok {
		t.Fatal("state entry must go with the schedule")
	}
	if len(e.svc.ListSchedules()) != 2 || e.changed[len(e.changed)-1] != id {
		t.Fatalf("after delete: %v", e.changed)
	}
	if err := e.svc.DeleteSchedule(id); apperr.As(err).Code != apperr.ScheduleNotFound {
		t.Fatalf("delete twice: %v", err)
	}

	if sc, err := e.svc.SetScheduleEnabled(b.Schedule.ID, false); err != nil || sc.Enabled {
		t.Fatalf("disable %+v %v", sc, err)
	}
	if sc, err := e.svc.SetScheduleEnabled(b.Schedule.ID, true); err != nil || !sc.Enabled {
		t.Fatalf("enable %+v %v", sc, err)
	}
	// A schedule left without recipients (its channel was deleted) can't be enabled.
	if err := e.st.UpdateConfig(func(c *config.Config) error {
		c.Schedules[1].Recipients = []config.Recipient{}
		c.Schedules[1].Enabled = false
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.SetScheduleEnabled(b.Schedule.ID, true); apperr.As(err).Code != apperr.RecipientRequired {
		t.Fatalf("enable without recipients: %v", err)
	}
}

func TestProcessorSchemas(t *testing.T) {
	e := newSchedEnv(t)
	got := e.svc.ProcessorSchemas()
	if len(got) != 2 || got[0].Type != "passthrough" || got[1].Type != xlsxsnapshot.Type ||
		!slices.Contains(got[1].Extensions, ".xlsx") || len(got[1].Options) != 1 || got[1].Options[0].Key != "sheets" {
		t.Fatalf("%+v", got)
	}
}

func TestPreviewFilesAndSendName(t *testing.T) {
	e := newSchedEnv(t)
	e.file(t, "b 26-10-2026.xlsm")
	e.file(t, "a 26-10-2026.xlsm")
	pattern := filepath.Join(e.dir, "* {date:DD-MM-YYYY}.xlsm")

	p := e.svc.PreviewFiles(pattern, config.MatchAll)
	if p.Error != nil || p.Path != filepath.Join(e.dir, "* 26-10-2026.xlsm") || len(p.Files) != 2 || p.Files[0].Name != "a 26-10-2026.xlsm" {
		t.Fatalf("all %+v", p)
	}
	if p = e.svc.PreviewFiles(pattern, config.MatchError); p.Error == nil || p.Error.Code != apperr.FileMultipleMatches {
		t.Fatalf("error mode %+v", p)
	}
	if p = e.svc.PreviewFiles(pattern, config.MatchNewest); p.Error != nil || len(p.Files) != 1 {
		t.Fatalf("newest %+v", p)
	}
	if p = e.svc.PreviewFiles(filepath.Join(e.dir, "none.pdf"), config.MatchError); p.Error == nil || p.Error.Code != apperr.FileNotFound || p.Path == "" {
		t.Fatalf("missing %+v", p)
	}
	if p = e.svc.PreviewFiles("rel.pdf", config.MatchError); p.Error == nil || p.Error.Code != apperr.FilespecPathNotAbsolute {
		t.Fatalf("relative %+v", p)
	}

	names, err := e.svc.PreviewSendName("Report {date:DD.MM}", pattern, config.MatchAll, xlsxsnapshot.Type)
	if err != nil || strings.Join(names, "|") != "Report 26.10 (1).xlsx|Report 26.10 (2).xlsx" {
		t.Fatalf("numbered %v %v", names, err)
	}
	names, err = e.svc.PreviewSendName("", pattern, config.MatchAll, "passthrough")
	if err != nil || strings.Join(names, "|") != "a 26-10-2026.xlsm|b 26-10-2026.xlsm" {
		t.Fatalf("original names %v %v", names, err)
	}
	names, err = e.svc.PreviewSendName("Report", filepath.Join(e.dir, "missing.pdf"), config.MatchError, "passthrough")
	if err != nil || strings.Join(names, "|") != "Report.pdf" {
		t.Fatalf("no match %v %v", names, err)
	}
	if _, err := e.svc.PreviewSendName("a/b", pattern, config.MatchNewest, "passthrough"); apperr.As(err).Code != apperr.FilespecSendNameInvalid {
		t.Fatalf("invalid %v", err)
	}
	if _, err := e.svc.PreviewSendName("", pattern, config.MatchNewest, "magic"); apperr.As(err).Code != apperr.ProcessorUnknown {
		t.Fatalf("unknown processor %v", err)
	}
}

func TestErrorRecipientsAndFolders(t *testing.T) {
	e := newSchedEnv(t)
	r := config.Recipient{ID: "e1", ChannelID: "ch", Target: json.RawMessage(`{"to":"me"}`), DisplayName: "Me"}
	ps, err := e.svc.SaveErrorRecipients([]config.Recipient{r})
	if err != nil || len(ps) != 0 {
		t.Fatalf("save %v %v", ps, err)
	}
	got := e.svc.ErrorRecipients()
	if len(got) != 1 || got[0].Verified != config.VerifiedUnknown {
		t.Fatalf("got %+v", got)
	}
	bad := r
	bad.ID, bad.ChannelID = "e2", "gone"
	dup := r
	dup.ID = "e3"
	ps, _ = e.svc.SaveErrorRecipients([]config.Recipient{r, bad, dup})
	if fields(ps) != "error_recipients.e2=recipient.channel_missing error_recipients.e3=recipient.duplicate" || len(e.svc.ErrorRecipients()) != 1 {
		t.Fatalf("problems %s", fields(ps))
	}

	if err := e.svc.OpenDataDir(); apperr.As(err).Code != apperr.FolderOpenFailed {
		t.Fatalf("no opener: %v", err)
	}
	var opened []string
	backups := t.TempDir()
	Configure(e.svc, Extras{DataDir: `C:\data`, BackupsDir: backups, OpenFolder: func(p string) error {
		opened = append(opened, p)
		if p == `C:\data` {
			return nil
		}
		return errors.New("explorer missing")
	}})
	if err := e.svc.OpenDataDir(); err != nil || e.svc.DataDir() != `C:\data` {
		t.Fatal(err)
	}
	if err := e.svc.OpenLogsDir(); apperr.As(err).Code != apperr.FolderOpenFailed || apperr.As(err).Params["path"] != e.svc.logsDir {
		t.Fatalf("logs: %v", err)
	}
	if len(opened) != 2 {
		t.Fatalf("opened %v", opened)
	}

	if e.svc.LastBackup() != nil {
		t.Fatal("no backup yet")
	}
	if err := os.Mkdir(filepath.Join(backups, "20261025-083000-v0.7.0"), 0o700); err != nil {
		t.Fatal(err)
	}
	if b := e.svc.LastBackup(); b == nil || b.Format("2006-01-02 15:04") != "2026-10-25 08:30" {
		t.Fatalf("backup %v", b)
	}
}

func TestRunHistory(t *testing.T) {
	e := newSchedEnv(t)
	res, _ := e.svc.SaveSchedule(e.valid(t))
	id := res.Schedule.ID
	lines := []map[string]any{
		{"time": "2026-10-26T08:00:05+01:00", "level": "INFO", "msg": "scheduler: slot skipped", "schedule": id, "code": "slot.missed",
			"params": map[string]string{"schedule": "Sales", "count": "2", "from": "06:00", "to": "07:00"}},
		{"time": "2026-10-26T08:30:00+01:00", "level": "WARN", "msg": "pipeline: problem", "schedule": id, "code": "file.not_found"},
		// Before 0.8.0 the line had no name: the current one is used.
		{"time": "2026-10-26T08:31:00+01:00", "level": "INFO", "msg": "pipeline: run finished", "schedule": id, "manual": false,
			"status": "failed", "sent": 0, "planned": 1},
		{"time": "2026-10-26T09:00:30+01:00", "level": "INFO", "msg": "pipeline: run finished", "schedule": id, "name": "Old name",
			"manual": true, "status": "partial", "sent": 1, "planned": 2, "error_code": "send.failed",
			"error_params": map[string]string{"file": "a.pdf", "recipient": "***567"}},
		{"time": "2026-10-26T09:10:00+01:00", "level": "INFO", "msg": "pipeline: run skipped, too late for the first send",
			"schedule": "gone", "name": "Deleted", "code": "slot.late", "params": map[string]string{"slot": "09:00"}},
	}
	var b strings.Builder
	for _, l := range lines {
		j, _ := json.Marshal(l)
		b.Write(j)
		b.WriteString("\n")
	}
	b.WriteString("not json\n")
	if err := os.WriteFile(filepath.Join(e.svc.logsDir, logging.FileName), []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.RunHistory(0)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range got {
		code := ""
		if r.Error != nil {
			code = string(r.Error.Code)
		}
		out = append(out, strings.Join([]string{r.ScheduleName, r.Status, code}, "/"))
	}
	if strings.Join(out, " ") != "Deleted/skipped/slot.late Old name/partial/send.failed Sales/failed/ Sales/skipped/slot.missed" {
		t.Fatalf("history %v", out)
	}
	if !got[1].Manual || got[1].Sent != 1 || got[1].Planned != 2 || got[1].Error.Params["file"] != "a.pdf" ||
		got[3].Error.Params["count"] != "2" {
		t.Fatalf("details %+v", got)
	}
	if got, _ := e.svc.RunHistory(2); len(got) != 2 {
		t.Fatalf("limit %d", len(got))
	}
}
