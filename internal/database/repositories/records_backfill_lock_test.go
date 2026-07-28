package repositories

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAcquireBackfillDIDProcessLockHonorsCanceledWait(t *testing.T) {
	release, err := acquireBackfillDIDProcessLock(context.Background(), "did:plc:locked")
	if err != nil {
		t.Fatalf("acquire first lock: %v", err)
	}
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	secondRelease, err := acquireBackfillDIDProcessLock(ctx, "did:plc:locked")
	if secondRelease != nil {
		secondRelease()
		t.Fatal("canceled waiter unexpectedly acquired process lock")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled waiter error = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("canceled process-lock wait took %s, want prompt return", elapsed)
	}
}
