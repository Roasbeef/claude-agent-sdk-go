package claudeagent

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewClientAcceptsPermissionModeConstants(t *testing.T) {
	tests := []struct {
		name string
		mode PermissionMode
	}{
		{name: "default", mode: PermissionModeDefault},
		{name: "plan", mode: PermissionModePlan},
		{name: "accept edits", mode: PermissionModeAcceptEdits},
		{name: "bypass all", mode: PermissionModeBypassAll},
		{name: "auto", mode: PermissionModeAuto},
		{name: "dont ask", mode: PermissionModeDontAsk},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(WithPermissionMode(tt.mode))
			require.NoError(t, err)
			require.Equal(t, tt.mode, client.options.PermissionMode)
		})
	}
}
