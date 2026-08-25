package uid

import (
	"regexp"
	"strings"
	"testing"
)

// 8-4-4-4-12 lowercase hex, with the version nibble pinned to 4 and the
// variant nibble to one of 8, 9, a or b.
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

var workflowIDPattern = regexp.MustCompile(`^wf-[0-9a-f]{16}$`)

func TestNewUUIDLayout(t *testing.T) {
	for i := 0; i < 200; i++ {
		got := NewUUID()
		if len(got) != 36 {
			t.Fatalf("NewUUID() = %q, want 36 characters, got %d", got, len(got))
		}
		if !uuidPattern.MatchString(got) {
			t.Fatalf("NewUUID() = %q, want a lowercase version 4 uuid", got)
		}
	}
}

func TestNewUUIDIsUnique(t *testing.T) {
	const draws = 2000

	seen := make(map[string]struct{}, draws)
	for i := 0; i < draws; i++ {
		id := NewUUID()
		if _, dup := seen[id]; dup {
			t.Fatalf("NewUUID() repeated %q after %d draws", id, i)
		}
		seen[id] = struct{}{}
	}
}

func TestNewWorkflowIDLayout(t *testing.T) {
	for i := 0; i < 200; i++ {
		got := NewWorkflowID()
		if !strings.HasPrefix(got, "wf-") {
			t.Fatalf("NewWorkflowID() = %q, want a wf- prefix", got)
		}
		if !workflowIDPattern.MatchString(got) {
			t.Fatalf("NewWorkflowID() = %q, want wf- followed by 16 hex characters", got)
		}
	}
}

func TestNewWorkflowIDIsUnique(t *testing.T) {
	const draws = 2000

	seen := make(map[string]struct{}, draws)
	for i := 0; i < draws; i++ {
		id := NewWorkflowID()
		if _, dup := seen[id]; dup {
			t.Fatalf("NewWorkflowID() repeated %q after %d draws", id, i)
		}
		seen[id] = struct{}{}
	}
}
