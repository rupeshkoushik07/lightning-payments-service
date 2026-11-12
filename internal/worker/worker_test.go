package worker

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/rupeshkoushik07/lightning-payments-service/internal/store"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

// fakeNode answers the way we tell it to. No Lightning node is needed.
type fakeNode struct {
	payResult   Result
	payErr      error
	payCalls    int
	trackResult Result
	trackErr    error
}

func (n *fakeNode) Pay(context.Context, store.Job) (Result, error) {
	n.payCalls++
	return n.payResult, n.payErr
}
func (n *fakeNode) Track(context.Context, []byte) (Result, error) {
	return n.trackResult, n.trackErr
}

// fakeDB remembers what the workers wrote. No database is needed.
type fakeDB struct {
	paused    bool
	pauseErr  error
	job       *store.Job
	open      []store.Open
	succeeded map[int64][]byte
	failed    map[int64]string
}

func newDB() *fakeDB { return &fakeDB{succeeded: map[int64][]byte{}, failed: map[int64]string{}} }

func (d *fakeDB) PayoutsPaused(context.Context) (bool, error) { return d.paused, d.pauseErr }
func (d *fakeDB) ClaimPending(context.Context) (store.Job, error) {
	if d.job == nil {
		return store.Job{}, store.ErrNothingToDo
	}
	j := *d.job
	d.job = nil
	return j, nil
}
func (d *fakeDB) ListInFlight(context.Context) ([]store.Open, error) { return d.open, nil }
func (d *fakeDB) MarkSucceeded(_ context.Context, id int64, preimage []byte) error {
	d.succeeded[id] = preimage
	return nil
}
func (d *fakeDB) MarkFailed(_ context.Context, id int64, reason string) error {
	d.failed[id] = reason
	return nil
}

func (d *fakeDB) untouched() bool { return len(d.succeeded) == 0 && len(d.failed) == 0 }

func TestSender(t *testing.T) {
	job := store.Job{ID: 7}
	preimage := make([]byte, 32)

	t.Run("an error from the node leaves the payment open", func(t *testing.T) {
		db, node := newDB(), &fakeNode{payErr: errors.New("connection dropped")}
		db.job = &job
		if err := (&Sender{Node: node, Payments: db, Log: quiet}).PayOne(context.Background()); err != nil {
			t.Fatal(err)
		}
		if !db.untouched() {
			t.Errorf("an error must not close the payment; got succeeded=%v failed=%v", db.succeeded, db.failed)
		}
	})

	t.Run("no final answer leaves the payment open", func(t *testing.T) {
		db, node := newDB(), &fakeNode{payResult: Result{Outcome: Unknown}}
		db.job = &job
		_ = (&Sender{Node: node, Payments: db, Log: quiet}).PayOne(context.Background())
		if !db.untouched() {
			t.Error("unknown must not close the payment")
		}
	})

	t.Run("success is saved with its receipt", func(t *testing.T) {
		db, node := newDB(), &fakeNode{payResult: Result{Outcome: Succeeded, Preimage: preimage}}
		db.job = &job
		_ = (&Sender{Node: node, Payments: db, Log: quiet}).PayOne(context.Background())
		if _, ok := db.succeeded[7]; !ok || len(db.failed) != 0 {
			t.Errorf("want payment 7 succeeded; got succeeded=%v failed=%v", db.succeeded, db.failed)
		}
	})

	t.Run("the node's failure is saved with its reason", func(t *testing.T) {
		db, node := newDB(), &fakeNode{payResult: Result{Outcome: Failed, Reason: "NO_ROUTE"}}
		db.job = &job
		_ = (&Sender{Node: node, Payments: db, Log: quiet}).PayOne(context.Background())
		if db.failed[7] != "NO_ROUTE" {
			t.Errorf("want failed with NO_ROUTE; got %v", db.failed)
		}
	})

	t.Run("a result is saved even while shutting down", func(t *testing.T) {
		db, node := newDB(), &fakeNode{payResult: Result{Outcome: Succeeded, Preimage: preimage}}
		db.job = &job
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // the program has been told to stop
		_ = (&Sender{Node: node, Payments: db, Log: quiet}).PayOne(ctx)
		if _, ok := db.succeeded[7]; !ok {
			t.Error("a result we already hold must still be written")
		}
	})

	for name, db := range map[string]*fakeDB{
		"paused":                {paused: true},
		"switch cannot be read": {pauseErr: errors.New("database down")},
	} {
		t.Run("nothing is paid when "+name, func(t *testing.T) {
			db.succeeded, db.failed = map[int64][]byte{}, map[int64]string{}
			db.job = &job
			node := &fakeNode{payResult: Result{Outcome: Succeeded, Preimage: preimage}}
			err := (&Sender{Node: node, Payments: db, Log: quiet}).PayOne(context.Background())
			if !errors.Is(err, store.ErrNothingToDo) || node.payCalls != 0 || db.job == nil {
				t.Errorf("must not claim or pay; err=%v payCalls=%d", err, node.payCalls)
			}
		})
	}
}

func TestTracker(t *testing.T) {
	preimage := make([]byte, 32)
	old := store.Open{ID: 1, Age: 5 * time.Minute}
	fresh := store.Open{ID: 2, Age: 5 * time.Second}

	cases := []struct {
		name          string
		open          store.Open
		node          fakeNode
		wantSucceeded bool
		wantFailed    bool
	}{
		{"node says succeeded", old, fakeNode{trackResult: Result{Outcome: Succeeded, Preimage: preimage}}, true, false},
		{"node says failed", old, fakeNode{trackResult: Result{Outcome: Failed, Reason: "x"}}, false, true},
		{"node never heard of it, row is old", old, fakeNode{trackResult: Result{Outcome: NotFound}}, false, true},
		{"node never heard of it, row is fresh: wait", fresh, fakeNode{trackResult: Result{Outcome: NotFound}}, false, false},
		{"node is still trying: wait", old, fakeNode{trackResult: Result{Outcome: Unknown}}, false, false},
		{"node cannot be reached: change nothing", old, fakeNode{trackErr: errors.New("unreachable")}, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			db := newDB()
			db.open = []store.Open{c.open}
			if err := (&Tracker{Node: &c.node, Payments: db, Log: quiet}).ResolveAll(context.Background()); err != nil {
				t.Fatal(err)
			}
			_, gotS := db.succeeded[c.open.ID]
			_, gotF := db.failed[c.open.ID]
			if gotS != c.wantSucceeded || gotF != c.wantFailed {
				t.Errorf("succeeded=%v failed=%v, want succeeded=%v failed=%v", gotS, gotF, c.wantSucceeded, c.wantFailed)
			}
		})
	}
}
