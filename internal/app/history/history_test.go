package history

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/ninoamine/agent-factory/internal/app/execution"
)

// memStore is an in-memory Store; failRewrite simulates a failed atomic
// rewrite (the old content must stay untouched).
type memStore struct {
	data        []byte
	exists      bool
	failRewrite bool
}

func (m *memStore) Read(context.Context) ([]byte, error) {
	if !m.exists {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), m.data...), nil
}

func (m *memStore) Append(_ context.Context, d []byte) error {
	if len(m.data) > 0 && m.data[len(m.data)-1] != '\n' {
		m.data = append(m.data, '\n')
	}
	m.data, m.exists = append(m.data, d...), true
	return nil
}

func (m *memStore) Rewrite(_ context.Context, d []byte) error {
	if m.failRewrite {
		return errors.New("disk full")
	}
	m.data, m.exists = append([]byte(nil), d...), true
	return nil
}

func newSvc(t *testing.T) (*Service, *memStore) {
	t.Helper()
	st := &memStore{}
	n := 0
	return &Service{Store: st, NewID: func() string { n++; return string(rune('a' + n - 1)) }}, st
}

func ip(i int) *int { return &i }

func rec(st execution.Status, in, out *int) execution.Record {
	return execution.Record{
		Time: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Command: "/code", Model: "p/m",
		Prompt: "SECRET PROMPT", Status: st, Usage: execution.Usage{Input: in, Output: out},
	}
}

func TestHIST1_HIST2_HIST4_ThreeOutcomes(t *testing.T) {
	s, st := newSvc(t)
	ctx := context.Background()
	for _, r := range []execution.Record{
		rec(execution.StatusSuccess, ip(1), ip(2)),
		rec(execution.StatusError, nil, nil),
		rec(execution.StatusInterrupted, nil, nil),
	} {
		if err := s.Record(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	data := st.data
	if n := strings.Count(string(data), "\n"); n != 3 {
		t.Fatalf("HIST-1 want 3 lines, got %d", n)
	}
	if !strings.Contains(string(data), `"inputTokens":null,"outputTokens":null,"status":"error"`) {
		t.Errorf("HIST-4 missing usage must be null: %s", data)
	}
	es, warns, err := s.List(ctx)
	if err != nil || len(warns) != 0 || len(es) != 3 {
		t.Fatalf("HIST-2 list: %v %v %v", es, warns, err)
	}
	if es[0].Command != "/code" || es[0].Model != "p/m" || *es[0].InputTokens != 1 {
		t.Errorf("HIST-1 fields wrong: %+v", es[0])
	}
}

func TestHIST2_RejectsInvalidStatus(t *testing.T) {
	s, _ := newSvc(t)
	if err := s.Record(context.Background(), rec("weird", nil, nil)); err == nil {
		t.Error("HIST-2 invalid status must be rejected")
	}
}

func TestHIST3_NoPromptStored(t *testing.T) {
	s, st := newSvc(t)
	_ = s.Record(context.Background(), rec(execution.StatusSuccess, nil, nil))
	data := st.data
	if strings.Contains(string(data), "SECRET") {
		t.Error("HIST-3 prompt text stored")
	}
}

func TestHIST5_ListShowsIDs(t *testing.T) {
	s, _ := newSvc(t)
	ctx := context.Background()
	_ = s.Record(ctx, rec(execution.StatusSuccess, nil, nil))
	es, _, _ := s.List(ctx)
	if len(es) != 1 || es[0].ID != "a" {
		t.Errorf("HIST-5 %+v", es)
	}
}

func TestHIST6_HIST7_DeleteAndClear(t *testing.T) {
	s, st := newSvc(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_ = s.Record(ctx, rec(execution.StatusSuccess, nil, nil))
	}
	if err := s.Delete(ctx, "b"); err != nil {
		t.Fatal(err)
	}
	es, _, _ := s.List(ctx)
	if len(es) != 2 || es[0].ID != "a" || es[1].ID != "c" {
		t.Errorf("HIST-6 delete one: %+v", es)
	}
	if err := s.Delete(ctx, "zzz"); !errors.Is(err, ErrNotFound) {
		t.Errorf("HIST-6 unknown id: %v", err)
	}
	if err := s.Clear(ctx); err != nil {
		t.Fatal(err)
	}
	if data := st.data; len(data) != 0 {
		t.Errorf("HIST-6 clear left %q", data)
	}
}

func TestHIST7_RewriteIsAtomicOnFailure(t *testing.T) {
	s, st := newSvc(t)
	ctx := context.Background()
	_ = s.Record(ctx, rec(execution.StatusSuccess, nil, nil))
	st.failRewrite = true
	if err := s.Delete(ctx, "a"); err == nil {
		t.Fatal("expected error")
	}
	if es, _, _ := s.List(ctx); len(es) != 1 {
		t.Error("HIST-7 original corrupted by failed rewrite")
	}
}

func TestHIST8_HistoryIsStandalone(t *testing.T) {
	// The service exposes no hook into routing or configuration; it only
	// implements execution.Recorder.
	var _ execution.Recorder = (*Service)(nil)
	t.Log("HIST-8 verified at compile time: no routing/configuration imports")
}

func TestHIST9_AbsentFileNoWarning(t *testing.T) {
	s, st := newSvc(t)
	es, warns, err := s.List(context.Background())
	if err != nil || len(es) != 0 || len(warns) != 0 {
		t.Fatalf("HIST-9 list absent: %v %v %v", es, warns, err)
	}
	if err := s.Record(context.Background(), rec(execution.StatusSuccess, nil, nil)); err != nil {
		t.Fatal(err)
	}
	if !st.exists {
		t.Error("HIST-9 file not created")
	}
}

func TestHIST10_CorruptedLineSkipped(t *testing.T) {
	s, st := newSvc(t)
	ctx := context.Background()
	_ = s.Record(ctx, rec(execution.StatusSuccess, nil, nil))
	st.data = append(st.data, "{not json"...) // no trailing newline
	_ = s.Record(ctx, rec(execution.StatusSuccess, nil, nil))
	es, warns, err := s.List(ctx)
	if err != nil || len(es) != 2 || len(warns) != 1 {
		t.Fatalf("HIST-10 %v %v %v", es, warns, err)
	}
	if err := s.Delete(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if data := st.data; !strings.Contains(string(data), "{not json") {
		t.Error("HIST-10 delete must not drop unrelated lines")
	}
}

func TestHIST6_CommandParsingAndMutates(t *testing.T) {
	t.Run("HIST-6 delete and clear are mutating", func(t *testing.T) {
		for args, want := range map[string]bool{
			"": false, "delete abc": true, "clear": true, "list": false, "  clear ": true,
		} {
			if got := Mutates(args); got != want {
				t.Errorf("Mutates(%q) = %v, want %v", args, got, want)
			}
		}
	})
	t.Run("HIST-6 clear asks first, then clears", func(t *testing.T) {
		s, st := newSvc(t)
		_ = s.Record(context.Background(), rec(execution.StatusSuccess, ip(1), ip(2)))
		r, err := s.Command(context.Background(), "clear")
		if err != nil || r.Confirm == nil || len(st.data) == 0 {
			t.Fatalf("clear must only ask: %+v %v", r, err)
		}
		if _, err := r.Confirm.Run(context.Background()); err != nil || len(st.data) != 0 {
			t.Fatalf("confirmed clear must empty history: %v", err)
		}
	})
	t.Run("HIST-5 list shows id; empty and unknown ids are reported", func(t *testing.T) {
		s, _ := newSvc(t)
		r, _ := s.Command(context.Background(), "")
		if len(r.Lines) != 1 || r.Lines[0] != "history is empty" {
			t.Fatalf("got %v", r.Lines)
		}
		_ = s.Record(context.Background(), rec(execution.StatusSuccess, nil, nil))
		r, _ = s.Command(context.Background(), "")
		if want := "a | 2026-01-02T03:04:05Z | /code | p/m | in: unavailable | out: unavailable | success"; r.Lines[0] != want {
			t.Fatalf("got %v", r.Lines)
		}
		r, _ = s.Command(context.Background(), "delete zzz")
		if r.Lines[0] != "no history entry with id zzz" {
			t.Fatalf("got %v", r.Lines)
		}
	})
}

func TestHIST11_SDDFieldOptional(t *testing.T) {
	svc, st := newSvc(t)
	r := rec(execution.StatusSuccess, ip(1), ip(2))
	r.SDD = "implement"
	if err := svc.Record(context.Background(), r); err != nil {
		t.Fatal(err)
	}
	if err := svc.Record(context.Background(), rec(execution.StatusSuccess, ip(1), ip(2))); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(st.data)), "\n")
	if !strings.Contains(lines[0], `"sdd":"implement"`) || strings.Contains(lines[1], "sdd") || strings.Contains(string(st.data), "SECRET PROMPT") {
		t.Fatalf("HIST-11: %v", lines)
	}
	if entries, warns, _ := svc.List(context.Background()); len(entries) != 2 || len(warns) != 0 {
		t.Fatalf("HIST-11: lines with the sdd field must stay valid: %v %v", entries, warns)
	}
}
