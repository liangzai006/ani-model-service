package work

import (
	"context"
	"errors"
	"time"
)

type Creator interface {
	Create(context.Context, Task) (Task, error)
}

var (
	ErrInvalidTransition = errors.New("invalid import task transition")
	ErrLeaseFenced       = errors.New("import task lease fenced")
)

const (
	Pending   = "pending"
	Importing = "importing"
	Completed = "completed"
	Failed    = "failed"
)

type Task struct {
	TenantID, ID, LeaseOwner                                               string
	ModelID, VersionID, TaskType, Source, RepoID, Revision, IdempotencyKey string
	LeaseEpoch                                                             int64
	Status                                                                 string
	LeaseUntil                                                             time.Time
	AttemptCount                                                           int
}

func (t Task) Claim(owner string, now time.Time, lease time.Duration) (Task, error) {
	if owner == "" || lease <= 0 || (t.Status != Pending && t.Status != Importing) || (!t.LeaseUntil.IsZero() && t.LeaseUntil.After(now)) {
		return Task{}, ErrInvalidTransition
	}
	t.Status, t.LeaseOwner, t.LeaseEpoch = Importing, owner, t.LeaseEpoch+1
	t.LeaseUntil, t.AttemptCount = now.Add(lease), t.AttemptCount+1
	return t, nil
}

func (t Task) Complete(owner string, epoch int64, now time.Time) (Task, error) {
	if t.Status != Importing || t.LeaseOwner != owner || t.LeaseEpoch != epoch || !t.LeaseUntil.After(now) {
		return Task{}, ErrLeaseFenced
	}
	t.Status, t.LeaseOwner, t.LeaseUntil = Completed, "", time.Time{}
	return t, nil
}

func (t Task) Fail(owner string, epoch int64, now time.Time) (Task, error) {
	if t.Status != Importing || t.LeaseOwner != owner || t.LeaseEpoch != epoch || !t.LeaseUntil.After(now) {
		return Task{}, ErrLeaseFenced
	}
	t.Status, t.LeaseOwner, t.LeaseUntil = Failed, "", time.Time{}
	return t, nil
}
