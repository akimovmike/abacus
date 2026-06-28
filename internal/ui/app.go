package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"abacus/internal/beads"
	"abacus/internal/config"
	"abacus/internal/graph"
	"abacus/internal/update"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

const (
	minViewportWidth  = 20
	minViewportHeight = 5
	minTreeWidth      = 18
	// minTreeWidthForColumns is the minimum treeWidth (after subtracting column area)
	// before a column is dropped during progressive hiding. Set to guarantee ~30 chars
	// of title space (30 title + ~16 max prefix = 46).
	minTreeWidthForColumns = 46
	minListHeight          = 5
	refreshDisplayDuration = 3 * time.Second // How long delta metrics stay visible in footer
)

// OverlayType represents which overlay is currently active.
type OverlayType int

const (
	OverlayNone OverlayType = iota
	OverlayStatus
	OverlayLabels
	OverlayCreate
	OverlayDelete
	OverlayComment
	OverlayPriority
	OverlayColumns
)

// Layout describes how the tree and detail panes are arranged.
type Layout int

const (
	LayoutWide Layout = iota // default: tree left, detail right
	LayoutTall               // tree top, detail below
)

// ViewMode represents the current view filter mode.
type ViewMode int

// View modes in cycle order (broad -> narrow). Names and predicates live in
// viewModeDefs (state_filter.go), keyed by these consts so they can't drift.
const (
	ViewModeAll       ViewMode = iota // Show all issues (default)
	ViewModeNotClosed                 // Hide only closed issues
	ViewModeActive                    // Hide closed, blocked, and deferred
	ViewModeReady                     // Show ready issues (open + not blocked)
	viewModeCount                     // sentinel: keep last; must equal len(viewModeDefs)
)

// valid reports whether v indexes a defined mode in viewModeDefs.
func (v ViewMode) valid() bool { return int(v) >= 0 && int(v) < len(viewModeDefs) }

// String returns the display name of the view mode.
func (v ViewMode) String() string {
	if !v.valid() {
		return viewModeDefs[ViewModeAll].name // guard: unknown mode reads as All
	}
	return viewModeDefs[v].name
}

// Next returns the next view mode in the cycle.
func (v ViewMode) Next() ViewMode {
	return ViewMode((int(v) + 1) % len(viewModeDefs))
}

// Prev returns the previous view mode in the cycle.
func (v ViewMode) Prev() ViewMode {
	return ViewMode((int(v) + len(viewModeDefs) - 1) % len(viewModeDefs))
}

// Config configures the UI application.
type Config struct {
	RefreshInterval time.Duration
	AutoRefresh     bool
	OutputFormat    string
	StartupReporter StartupReporter
	Client          beads.Client
	Version         string // Version string to display in header
	UpdateChan      <-chan *update.UpdateInfo
	Backend         string // Backend type: "bd" or "br"
	StorePath       string // Path watched for refresh (db file or dolt store dir)
}

// errorSource tracks where the last error originated so refresh success can
// decide whether to clear it.
type errorSource int

const (
	errorSourceNone errorSource = iota
	errorSourceRefresh
	errorSourceOperation
)

// App implements the Bubble Tea model for Abacus.
type App struct {
	roots             []*graph.Node
	visibleRows       []graph.TreeRow
	cursor            int
	treeTopLine       int
	treeMouseScrolled bool
	repoName          string

	viewport      viewport.Model
	ShowDetails   bool
	focus         FocusArea
	ready         bool
	detailIssueID string

	textInput  textinput.Model
	searching  bool
	filterText string
	viewMode   ViewMode // Current view filter mode (see viewModeDefs)
	// filterCollapsed tracks nodes explicitly collapsed while a search filter is active.
	filterCollapsed map[string]bool
	// filterForcedExpanded tracks nodes temporarily expanded to surface filter matches.
	filterForcedExpanded map[string]bool
	filterEval           map[string]filterEvaluation
	// expandedInstances tracks expanded state per TreeRow instance for multi-parent nodes.
	// Key format: "parentID:nodeID" where parentID is empty for root nodes.
	expandedInstances map[string]bool

	width            int
	height           int
	resizePending    bool      // Resize debounce: tick is scheduled
	resizeLastEvent  time.Time // Resize debounce: time of last WindowSizeMsg
	refreshInterval  time.Duration
	autoRefresh      bool
	dbPath           string
	lastDBModTime    time.Time
	lastRefreshStats string
	refreshInFlight  bool
	lastRefreshTime  time.Time
	spinner          spinner.Model
	outputFormat     string
	version          string
	backend          string // Backend type: "bd" or "br"

	client beads.Client

	// Error toast state
	lastError       string // Full error message (separate from stats)
	lastErrorSource errorSource
	errorShownOnce  bool      // True after first toast display
	showErrorToast  bool      // Currently showing toast
	errorToastStart time.Time // When toast was shown (for countdown)

	// Copy toast state
	showCopyToast  bool
	copyToastStart time.Time
	copiedBeadID   string

	// Status toast state
	statusToastVisible   bool
	statusToastStart     time.Time
	statusToastNewStatus string
	statusToastBeadID    string

	// Help overlay state
	showHelp bool
	keys     KeyMap

	// Overlay state
	activeOverlay   OverlayType
	statusOverlay   *StatusOverlay
	labelsOverlay   *LabelsOverlay
	createOverlay   *CreateOverlay
	deleteOverlay   *DeleteOverlay
	commentOverlay  *CommentOverlay
	priorityOverlay *PriorityOverlay
	columnsOverlay  *ColumnsOverlay

	// Labels toast state
	labelsToastVisible bool
	labelsToastStart   time.Time
	labelsToastAdded   []string
	labelsToastRemoved []string
	labelsToastBeadID  string

	// Create toast state
	createToastVisible  bool
	createToastStart    time.Time
	createToastBeadID   string
	createToastTitle    string
	createToastIsUpdate bool

	// New label toast state (shown during create overlay when new label added)
	newLabelToastVisible bool
	newLabelToastStart   time.Time
	newLabelToastLabel   string

	// New assignee toast state (shown during create overlay when new assignee added)
	newAssigneeToastVisible  bool
	newAssigneeToastStart    time.Time
	newAssigneeToastAssignee string

	// Delete toast state
	deleteToastVisible    bool
	deleteToastStart      time.Time
	deleteToastBeadID     string
	deleteToastCascade    bool
	deleteToastChildCount int

	// Theme toast state
	themeToastVisible bool
	themeToastStart   time.Time
	themeToastName    string

	// Comment toast state
	commentToastVisible bool
	commentToastStart   time.Time
	commentToastBeadID  string

	// Priority toast state
	priorityToastVisible     bool
	priorityToastStart       time.Time
	priorityToastBeadID      string
	priorityToastNewPriority int

	// Layout state
	layout             Layout
	layoutToastVisible bool
	layoutToastStart   time.Time
	layoutToastName    string

	// Update notification state
	updateToastVisible bool
	updateToastStart   time.Time
	updateInfo         *update.UpdateInfo
	updateInProgress   bool
	updateError        string
	updateChan         <-chan *update.UpdateInfo

	// Update success/failure toast state (ab-w1wp)
	updateSuccessToastVisible bool
	updateSuccessToastStart   time.Time
	updateSuccessVersion      string

	updateFailureToastVisible bool
	updateFailureToastStart   time.Time
	updateFailureError        string
	updateFailureCommand      string

	// Session tracking for exit summary
	sessionStart time.Time
	initialStats Stats
}

// errorHotkeyAvailable returns true when the global error hotkey should
// be allowed to toggle the toast (i.e., not while typing into a text input).
func (m *App) errorHotkeyAvailable() bool {
	if m.searching {
		return false
	}
	if m.activeOverlay == OverlayCreate && m.createOverlay != nil && m.createOverlay.IsTextInputActive() {
		return false
	}
	return true
}

// NewApp creates a new UI app instance based on configuration and current working directory.
func NewApp(cfg Config) (*App, error) {
	if cfg.RefreshInterval <= 0 {
		cfg.RefreshInterval = time.Duration(config.GetInt(config.KeyAutoRefreshSeconds)) * time.Second
	}

	reporter := cfg.StartupReporter
	if reporter != nil {
		reporter.Stage(StartupStageFindingDatabase, "Finding Beads workspace...")
	}

	client := cfg.Client
	storePath := cfg.StorePath
	if client == nil {
		var err error
		client, storePath, err = resolveClientAndStorePath(cfg.Backend, reporter)
		if err != nil {
			return nil, err
		}
	}

	roots, err := loadData(context.Background(), client, reporter)
	if err != nil && !errors.Is(err, ErrNoIssues) {
		return nil, err
	}

	var modTime time.Time
	if storePath != "" {
		if latest, err := latestModTimeForDB(storePath); err == nil {
			modTime = latest
		}
	}

	ti := textinput.New()
	ti.Placeholder = "Search..."
	ti.Prompt = "/"

	s := spinner.New()
	s.Spinner = spinner.Spinner{
		Frames: []string{"◴", "◷", "◶", "◵"},
		FPS:    time.Second / 4,
	}

	repo := "abacus"
	if wd, err := os.Getwd(); err == nil && wd != "" {
		repo = filepath.Base(wd)
	}

	app := &App{
		roots:           roots,
		textInput:       ti,
		repoName:        repo,
		focus:           FocusTree,
		refreshInterval: cfg.RefreshInterval,
		autoRefresh:     cfg.AutoRefresh && storePath != "",
		outputFormat:    cfg.OutputFormat,
		version:         cfg.Version,
		backend:         cfg.Backend,
		client:          client,
		dbPath:          storePath,
		lastDBModTime:   modTime,
		spinner:         s,
		keys:            DefaultKeyMap(),
		sessionStart:    time.Now(),
		updateChan:      cfg.UpdateChan,
	}
	if storePath == "" {
		app.lastRefreshStats = "refresh unavailable: no store path"
	}
	if config.GetString(config.KeyLayoutMode) == "tall" {
		app.layout = LayoutTall
	}
	app.recalcVisibleRows()
	// Capture initial stats for session summary
	app.initialStats = app.getStats()
	app.applyViewportTheme()
	if reporter != nil {
		reporter.Stage(StartupStageReady, "Ready!")
	}
	return app, nil
}

// resolveClientAndStorePath is a fallback for tests and standalone use when no
// client is injected. Production callers should resolve the store in main.go.
func resolveClientAndStorePath(backend string, reporter StartupReporter) (beads.Client, string, error) {
	workDir, beadsDir, err := FindBeadsWorkdir()
	if err != nil {
		return nil, "", err
	}
	if reporter != nil {
		reporter.Stage(StartupStageFindingDatabase, fmt.Sprintf("Using workspace at %s", beadsDir))
	}
	if backend == "" {
		backend = beads.BackendBd
	}
	ctx, _ := beads.ProbeContext(context.Background(), backend, workDir)

	switch ctx.Kind {
	case beads.StoreKindDolt:
		desc := beads.StoreDescriptor{Kind: beads.StoreKindDolt, WorkDir: workDir}
		client, err := beads.NewClientForBackend(backend, desc, ctx)
		if err != nil {
			return nil, "", fmt.Errorf("create client for backend %q: %w", backend, err)
		}
		return client, doltStorePath(beadsDir), nil
	case beads.StoreKindSQLite:
		dbPath := filepath.Join(beadsDir, "beads.db")
		desc := beads.StoreDescriptor{Kind: beads.StoreKindSQLite, DBPath: dbPath}
		client, err := beads.NewClientForBackend(backend, desc, ctx)
		if err != nil {
			return nil, "", fmt.Errorf("create client for backend %q: %w", backend, err)
		}
		return client, dbPath, nil
	}
	return nil, "", fmt.Errorf("could not determine store kind for %s", beadsDir)
}

// doltStorePath returns the directory to watch for Dolt store changes.
func doltStorePath(beadsDir string) string {
	for _, name := range []string{"embeddeddolt", "dolt"} {
		candidate := filepath.Join(beadsDir, name)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
	}
	return beadsDir
}

func (m *App) Init() tea.Cmd {
	cmds := []tea.Cmd{textinput.Blink}
	if m.autoRefresh && m.refreshInterval > 0 {
		cmds = append(cmds, scheduleTick(m.refreshInterval))
	}
	// Start background comment loading after TUI is displayed (ab-fkyz)
	cmds = append(cmds, scheduleBackgroundCommentLoad())
	// Start waiting for update check result (ab-a4qc)
	if m.updateChan != nil {
		cmds = append(cmds, m.waitForUpdateCheck())
	}
	return tea.Batch(cmds...)
}

func (m *App) applyViewportTheme() {
	m.viewport.Style = baseStyle()
}
