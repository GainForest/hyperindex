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

type cleanupActivityStore struct {
	calls chan int
}

func (s *cleanupActivityStore) CleanupOldActivity(_ context.Context, hours int) error {
	s.calls <- hours
	return nil
}

func TestActivityCleanupWorker_StartRunsCleanupPeriodically(t *testing.T) {
	activity := &cleanupActivityStore{calls: make(chan int, 4)}
	w := &ActivityCleanupWorker{
		activity:     activity,
		interval:     10 * time.Millisecond,
		retentionHrs: 168,
		stop:         make(chan struct{}),
		done:         make(chan struct{}),
	}

	w.Start(context.Background())
	defer w.Stop()

	for call := 1; call <= 2; call++ {
		select {
		case hours := <-activity.calls:
			if hours != 168 {
				t.Fatalf("cleanup call %d used retention hours %d, want 168", call, hours)
			}
		case <-time.After(250 * time.Millisecond):
			t.Fatalf("cleanup call %d did not run", call)
		}
	}
}
