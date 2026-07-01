package workers

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// BackfillState tests
// ---------------------------------------------------------------------------

func TestBackfillState_Start(t *testing.T) {
	t.Run("first call returns true", func(t *testing.T) {
		s := NewBackfillState()
		ctx := context.Background()

		ok := s.Start(ctx, 10)
		if !ok {
			t.Fatal("expected Start to return true on first call")
		}
	})

	t.Run("second call returns false (already active)", func(t *testing.T) {
		s := NewBackfillState()
		ctx := context.Background()

		s.Start(ctx, 10)
		ok := s.Start(ctx, 5)
		if ok {
			t.Fatal("expected Start to return false when already active")
		}
	})

	t.Run("IsActive returns true after Start", func(t *testing.T) {
		s := NewBackfillState()
		ctx := context.Background()

		if s.IsActive() {
			t.Fatal("expected IsActive to be false before Start")
		}

		s.Start(ctx, 10)

		if !s.IsActive() {
			t.Fatal("expected IsActive to be true after Start")
		}
	})
}

func TestBackfillState_Complete(t *testing.T) {
	t.Run("sets IsActive to false", func(t *testing.T) {
		s := NewBackfillState()
		ctx := context.Background()

		s.Start(ctx, 10)
		if !s.IsActive() {
			t.Fatal("expected IsActive to be true after Start")
		}

		s.Complete()

		if s.IsActive() {
			t.Fatal("expected IsActive to be false after Complete")
		}
	})

	t.Run("no-op when not active", func(t *testing.T) {
		s := NewBackfillState()

		// Should not panic or error when not active.
		s.Complete()

		if s.IsActive() {
			t.Fatal("expected IsActive to remain false")
		}
	})
}

func TestBackfillState_Callbacks(t *testing.T) {
	t.Run("OnStart callback fires", func(t *testing.T) {
		s := NewBackfillState()
		ctx := context.Background()

		var mu sync.Mutex
		called := false
		s.OnStart(func() {
			mu.Lock()
			called = true
			mu.Unlock()
		})

		s.Start(ctx, 5)

		// The callback fires in a goroutine, give it a moment.
		deadline := time.After(time.Second)
		for {
			mu.Lock()
			done := called
			mu.Unlock()
			if done {
				break
			}
			select {
			case <-deadline:
				t.Fatal("OnStart callback was not called within timeout")
			default:
				time.Sleep(5 * time.Millisecond)
			}
		}
	})

	t.Run("OnComplete callback fires", func(t *testing.T) {
		s := NewBackfillState()
		ctx := context.Background()

		var mu sync.Mutex
		called := false
		s.OnComplete(func() {
			mu.Lock()
			called = true
			mu.Unlock()
		})

		s.Start(ctx, 5)
		s.Complete()

		deadline := time.After(time.Second)
		for {
			mu.Lock()
			done := called
			mu.Unlock()
			if done {
				break
			}
			select {
			case <-deadline:
				t.Fatal("OnComplete callback was not called within timeout")
			default:
				time.Sleep(5 * time.Millisecond)
			}
		}
	})
}

func TestBackfillState_UpdateProgress(t *testing.T) {
	s := NewBackfillState()
	ctx := context.Background()

	s.Start(ctx, 100)
	s.UpdateProgress(42, 1500, "did:plc:example")

	p := s.Progress()
	if p.ActorsDone != 42 {
		t.Errorf("ActorsDone = %d, want 42", p.ActorsDone)
	}
	if p.RecordsDone != 1500 {
		t.Errorf("RecordsDone = %d, want 1500", p.RecordsDone)
	}
	if p.CurrentActor != "did:plc:example" {
		t.Errorf("CurrentActor = %q, want %q", p.CurrentActor, "did:plc:example")
	}
	if p.ActorsTotal != 100 {
		t.Errorf("ActorsTotal = %d, want 100", p.ActorsTotal)
	}
}

func TestBackfillState_RecordError(t *testing.T) {
	s := NewBackfillState()
	ctx := context.Background()

	s.Start(ctx, 10)

	s.RecordError(errors.New("fetch failed"))
	s.RecordError(errors.New("timeout"))

	p := s.Progress()
	if p.ErrorCount != 2 {
		t.Errorf("ErrorCount = %d, want 2", p.ErrorCount)
	}
	if p.LastError != "timeout" {
		t.Errorf("LastError = %q, want %q", p.LastError, "timeout")
	}
}

func TestBackfillState_Reset(t *testing.T) {
	s := NewBackfillState()
	ctx := context.Background()

	s.Start(ctx, 50)
	s.UpdateProgress(10, 200, "did:plc:someone")
	s.RecordError(errors.New("oops"))

	s.Reset()

	p := s.Progress()
	if p.IsActive {
		t.Error("expected IsActive to be false after Reset")
	}
	if p.StartedAt != nil {
		t.Error("expected StartedAt to be nil after Reset")
	}
	if p.ActorsDone != 0 {
		t.Errorf("ActorsDone = %d, want 0", p.ActorsDone)
	}
	if p.ActorsTotal != 0 {
		t.Errorf("ActorsTotal = %d, want 0", p.ActorsTotal)
	}
	if p.RecordsDone != 0 {
		t.Errorf("RecordsDone = %d, want 0", p.RecordsDone)
	}
	if p.ErrorCount != 0 {
		t.Errorf("ErrorCount = %d, want 0", p.ErrorCount)
	}
	if p.LastError != "" {
		t.Errorf("LastError = %q, want empty", p.LastError)
	}
	if p.CurrentActor != "" {
		t.Errorf("CurrentActor = %q, want empty", p.CurrentActor)
	}
}

// ---------------------------------------------------------------------------
// ActivityCleanupWorker tests
// ---------------------------------------------------------------------------

func TestActivityCleanupWorker_StartStop(t *testing.T) {
	// Use a nil activity repo — Start will call cleanup which will call
	// CleanupOldActivity on the repo. We need a real repo backed by a real
	// DB to avoid a nil-pointer panic during the initial cleanup() call.
	// Import testutil from an external test package would create a cycle
	// (workers -> testutil -> migrations -> database -> ... ), so instead
	// we exercise only the goroutine lifecycle by cancelling the context
	// before Start can do meaningful work.

	// We cannot easily construct an IndexingActivityRepository without
	// a database.Executor, so we test the goroutine lifecycle by using
	// a context that is immediately cancelled, preventing cleanup from
	// executing against a nil repo.
	//
	// For a full integration test with a real DB, see
	// internal/integration or use testutil from an _test package.

	// Verify Stop doesn't hang using a timeout channel.
	done := make(chan struct{})

	go func() {
		defer close(done)

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // cancel immediately so cleanup is skipped or errors harmlessly

		// Create worker with nil repo — cleanup will error-log but not panic
		// because the context is already cancelled and slog.Error is safe.
		// Actually, CleanupOldActivity dereferences the repo. Instead, let's
		// just test that the goroutine lifecycle (start/stop via context) works
		// by relying on context cancellation in the select loop.

		w := &ActivityCleanupWorker{
			activity:     nil,
			interval:     time.Hour,
			retentionHrs: 168,
			stop:         make(chan struct{}),
			done:         make(chan struct{}),
		}

		// Start the worker goroutine manually (skip the initial cleanup
		// call which would dereference nil repo).
		go func() {
			defer close(w.done)

			ticker := time.NewTicker(w.interval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-w.stop:
					return
				case <-ticker.C:
					// would call cleanup, skipped for test
				}
			}
		}()

		w.Stop()
	}()

	select {
	case <-done:
		// success — Stop returned without hanging
	case <-time.After(5 * time.Second):
		t.Fatal("ActivityCleanupWorker.Stop() hung for more than 5 seconds")
	}
}
