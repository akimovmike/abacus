package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// bdMigrateGateMarkers are substrings bd 1.1.x prints to stdout/stderr when
// it refuses a write because running it would auto-apply a Dolt schema
// migration to a remote-tracked database (design spec fold F17). Matched by
// containment, not exact string — "parse defensively" per the spec, since
// bd's exact wording around these markers is not itself part of the
// contract. bd's CLI output reaches here via CLIError.Output (see
// internal/beads/errors.go), which formatCommandError embeds directly into
// the error text these markers are matched against.
//
// A bare "Warning:" marker was deliberately dropped (round-1 review): bd can
// print unrelated warnings on a failed write, and matching on that alone
// would show the actionable-but-wrong migrate-gate toast instead of the real
// error, burying useful information. Both remaining markers are specific
// enough to bd's migrate-gate wording to be unambiguous on their own.
var bdMigrateGateMarkers = []string{
	"refusing to auto-apply",
	"BD_ALLOW_REMOTE_MIGRATE",
}

// migrateGateToastMessage is the actionable message shown instead of the
// raw bd CLI failure when a migrate-gate marker is detected.
const migrateGateToastMessage = "writes blocked — run `BD_ALLOW_REMOTE_MIGRATE=1 bd migrate` then `bd dolt push`"

// isMigrateGateError reports whether err's text contains one of bd 1.1.x's
// migrate-gate markers.
func isMigrateGateError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, marker := range bdMigrateGateMarkers {
		if strings.Contains(msg, marker) {
			return true
		}
	}
	return false
}

// operationErrorMessage returns the text to show for a failed write: the bd
// migrate-gate's actionable message when a marker is detected, otherwise
// the raw error text.
func operationErrorMessage(err error) string {
	if isMigrateGateError(err) {
		return migrateGateToastMessage
	}
	return err.Error()
}

// showOperationError sets the error-toast state for a failed write-command
// result and returns the cmd that animates it, centralizing the bd
// migrate-gate detection so every write-result handler benefits (ab-6irx).
func (m *App) showOperationError(err error) tea.Cmd {
	m.lastError = operationErrorMessage(err)
	m.lastErrorSource = errorSourceOperation
	m.showErrorToast = true
	m.errorToastStart = time.Now()
	return scheduleErrorToastTick()
}
