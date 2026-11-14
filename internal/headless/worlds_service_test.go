package headless

import (
	"fmt"
	"testing"
)

type refusedFocusTx struct {
	focusOutput []string
	status      []string
}

func (t refusedFocusTx) Exec(cmd string, _ ...ExecOption) ([]string, error) {
	switch cmd {
	case "worlds":
		return []string{"[1] Target\tUsers: 1\tPresent: 0\tAccessLevel: Private\tMaxUsers: 4"}, nil
	case "focus 1":
		return t.focusOutput, nil
	case "status":
		return t.status, nil
	default:
		return nil, fmt.Errorf("unexpected command: %s", cmd)
	}
}

func TestFocusWorldRejectsUnconfirmedTarget(t *testing.T) {
	for _, tc := range []struct {
		name string
		tx   refusedFocusTx
	}{
		{"focus error", refusedFocusTx{focusOutput: []string{"Unknown command"}, status: []string{"Name: Target", "SessionID: S-old"}}},
		{"previous world still focused", refusedFocusTx{status: []string{"Name: Previous", "SessionID: S-old"}}},
		{"missing status", refusedFocusTx{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := FocusWorld(tc.tx, 1); err == nil {
				t.Fatal("切り替え未確認の対象を操作可能と判断した")
			}
		})
	}
}
