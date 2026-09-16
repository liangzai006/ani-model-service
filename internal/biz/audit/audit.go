package audit

import "context"

import "errors"

var ErrInvalidEvent = errors.New("invalid audit event")

type Store interface {
	Insert(context.Context, Event) error
}

type Event struct {
	TenantID, Actor, Workload, RequestID                string
	TaskID, Action, BeforeState, AfterState, ErrorClass string
}

func (e Event) Validate() error {
	if e.TenantID == "" || e.Actor == "" || e.Workload == "" || e.RequestID == "" || e.Action == "" {
		return ErrInvalidEvent
	}
	if e.BeforeState == "" && e.AfterState == "" && e.ErrorClass == "" {
		return ErrInvalidEvent
	}
	return nil
}
