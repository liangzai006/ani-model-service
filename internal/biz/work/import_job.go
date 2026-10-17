package work

import (
	"fmt"
	"strings"
)

// ImportJobName returns the deterministic Kubernetes Job name for an attempt.
func ImportJobName(taskID string, attempt int) string {
	clean := strings.ToLower(strings.ReplaceAll(taskID, "-", ""))
	if attempt < 0 {
		attempt = 0
	}
	suffix := fmt.Sprintf("-a%d", attempt)
	maxTaskIDLength := 63 - len("ani-import-") - len(suffix)
	if len(clean) > maxTaskIDLength {
		clean = clean[:maxTaskIDLength]
	}
	return "ani-import-" + clean + suffix
}
