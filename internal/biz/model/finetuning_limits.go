package model

import "fmt"

// FinetuningLimitViolation reports measured source bytes or a parser structure
// bound without disclosing a storage location or provider error text.
type FinetuningLimitViolation struct {
	Reason, LimitName string
	Actual, Limit     int64
}

func (e *FinetuningLimitViolation) Error() string {
	return fmt.Sprintf("%s: %s actual=%d limit=%d", e.Reason, e.LimitName, e.Actual, e.Limit)
}
