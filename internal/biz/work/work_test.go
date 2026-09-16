package work

import (
	"errors"
	"testing"
	"time"
)

func TestClaimFencesStaleWorker(t *testing.T) {
	now := time.Unix(100, 0)
	task, err := (Task{TenantID: "t", ID: "id", Status: Pending}).Claim("worker-a", now, time.Minute)
	if err != nil || task.LeaseEpoch != 1 || task.Status != Importing {
		t.Fatalf("claim: %+v %v", task, err)
	}
	if _, err := task.Complete("worker-b", 1, now.Add(time.Second)); !errors.Is(err, ErrLeaseFenced) {
		t.Fatalf("expected owner fencing, got %v", err)
	}
	if _, err := task.Complete("worker-a", 1, now.Add(2*time.Minute)); !errors.Is(err, ErrLeaseFenced) {
		t.Fatalf("expected expiry fencing, got %v", err)
	}
}

func TestExpiredLeaseCanBeReclaimedWithNewEpoch(t *testing.T) {
	now := time.Unix(100, 0)
	task := Task{Status: Importing, LeaseOwner: "old", LeaseEpoch: 4, LeaseUntil: now.Add(-time.Second)}
	task, err := task.Claim("new", now, time.Minute)
	if err != nil || task.LeaseEpoch != 5 || task.LeaseOwner != "new" {
		t.Fatalf("reclaim: %+v %v", task, err)
	}
	if _, err := task.Complete("old", 4, now.Add(time.Second)); !errors.Is(err, ErrLeaseFenced) {
		t.Fatalf("expected stale result fencing, got %v", err)
	}
}
