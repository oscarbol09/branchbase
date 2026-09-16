package tui

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/branchbase/branchbase/internal/config"
	"github.com/branchbase/branchbase/internal/driver"
	"github.com/branchbase/branchbase/internal/git"
)

// ANSI Styling Constants
const (
	Reset       = "\033[0m"
	Bold        = "\033[1m"
	Dim         = "\033[2m"
	Green       = "\033[32m"
	Cyan        = "\033[36m"
	Yellow      = "\033[33m"
	Red         = "\033[31m"
	SelectedBg  = "\033[44;97;1m" // Blue background, bright white bold text
	ClearScreen = "\033[2J\033[H"
	CursorHide  = "\033[?25l"
	CursorShow  = "\033[?25h"
)

// DashboardModel holds the state and data for the interactive Terminal UI.
type DashboardModel struct {
	RepoPath        string
	Config          *config.Config
	Driver          driver.Driver
	ActiveBranch    string
	SanitizedActive string
	Branches        []driver.BranchInfo
	SelectedIndex   int
	FlashMessage    string
	FlashType       string // "info", "success", "error"
	TotalBytes      int64
	IsQuitting      bool
}

// NewModel initializes a new DashboardModel.
func NewModel(repoPath string, cfg *config.Config, drv driver.Driver) *DashboardModel {
	return &DashboardModel{
		RepoPath:      repoPath,
		Config:        cfg,
		Driver:        drv,
		SelectedIndex: 0,
		FlashMessage:  "Use ↑/↓ or j/k to navigate • Enter to switch • r to refresh • q to quit",
		FlashType:     "info",
	}
}

// Refresh reloads the active Git branch and queries the driver for managed databases.
func (m *DashboardModel) Refresh(ctx context.Context) error {
	branch, err := git.ResolveCurrentBranch(m.RepoPath)
	if err != nil {
		m.ActiveBranch = "unknown"
		m.SanitizedActive = "unknown"
	} else {
		m.ActiveBranch = branch
		m.SanitizedActive = git.SanitizeBranchName(branch)
	}

	if m.Driver == nil {
		m.Branches = nil
		m.TotalBytes = 0
		return nil
	}

	branches, err := m.Driver.ListBranches(ctx)
	if err != nil {
		return fmt.Errorf("failed to list databases: %w", err)
	}

	var total int64
	for i := range branches {
		if git.SanitizeBranchName(branches[i].Name) == m.SanitizedActive {
			branches[i].IsActive = true
		}
		total += branches[i].SizeBytes
	}

	m.Branches = branches
	m.TotalBytes = total

	// Clamp selected index
	if m.SelectedIndex >= len(m.Branches) {
		m.SelectedIndex = len(m.Branches) - 1
	}
	if m.SelectedIndex < 0 {
		m.SelectedIndex = 0
	}

	return nil
}

// MoveUp decrements the selected row index.
func (m *DashboardModel) MoveUp() {
	if m.SelectedIndex > 0 {
		m.SelectedIndex--
	}
}

// MoveDown increments the selected row index.
func (m *DashboardModel) MoveDown() {
	if m.SelectedIndex < len(m.Branches)-1 {
		m.SelectedIndex++
	}
}

// SelectedBranch returns the currently selected BranchInfo, or nil if empty.
func (m *DashboardModel) SelectedBranch() *driver.BranchInfo {
	if len(m.Branches) == 0 || m.SelectedIndex < 0 || m.SelectedIndex >= len(m.Branches) {
		return nil
	}
	return &m.Branches[m.SelectedIndex]
}

// SetFlash sets a temporary feedback message on the status bar.
func (m *DashboardModel) SetFlash(msg, msgType string) {
	m.FlashMessage = msg
	m.FlashType = msgType
}

// FormatBytes formats byte counts into human-readable representations.
func FormatBytes(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case bytes >= gb:
		return fmt.Sprintf("%.1f GB", float64(bytes)/float64(gb))
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// RenderView generates the complete ASCII / ANSI dashboard string.
func (m *DashboardModel) RenderView() string {
	var b strings.Builder

	driverName := "postgres"
	if m.Config != nil && m.Config.Driver != "" {
		driverName = m.Config.Driver
	}

	proxyPort := 5432
	if m.Config != nil && m.Config.Proxy.ListenPort > 0 {
		proxyPort = m.Config.Proxy.ListenPort
	}

	backendHost := "127.0.0.1:5433"
	if m.Config != nil {
		backendHost = fmt.Sprintf("%s:%d", m.Config.Connection.Host, m.Config.Connection.Port)
	}

	// 1. Header Card
	b.WriteString(Bold + Green + "🌿 BranchBase Dashboard" + Reset + " " + Dim + "v0.3.0" + Reset + "\n")
	b.WriteString(Dim + "─────────────────────────────────────────────────────────────────────────────" + Reset + "\n")
	b.WriteString(fmt.Sprintf("  • %-16s %s%s%s (sanitized: %s)\n", "Active Branch:", Bold+Cyan, m.ActiveBranch, Reset, m.SanitizedActive))
	b.WriteString(fmt.Sprintf("  • %-16s %s (%s)\n", "Database Engine:", Bold+driverName+Reset, backendHost))
	b.WriteString(fmt.Sprintf("  • %-16s Port %d -> Backend %s\n", "Proxy Routing:", proxyPort, backendHost))
	b.WriteString(fmt.Sprintf("  • %-16s %d database(s) (%s total)\n", "Managed Storage:", len(m.Branches), FormatBytes(m.TotalBytes)))
	b.WriteString(Dim + "─────────────────────────────────────────────────────────────────────────────" + Reset + "\n\n")

	// 2. Table Header
	b.WriteString(Bold + fmt.Sprintf("  %-3s %-24s %-28s %-10s %-12s", "", "BRANCH", "DATABASE", "SIZE", "STATUS") + Reset + "\n")
	b.WriteString(Dim + fmt.Sprintf("  %-3s %-24s %-28s %-10s %-12s", "", "------", "--------", "----", "------") + Reset + "\n")

	// 3. Table Rows
	if len(m.Branches) == 0 {
		b.WriteString(Dim + "  (No databases managed by BranchBase yet. Switch or query a branch to create one)\n" + Reset)
	} else {
		for i, item := range m.Branches {
			cursor := "  "
			rowPrefix := " "
			isSel := (i == m.SelectedIndex)

			if isSel {
				cursor = Bold + Cyan + "▶ " + Reset
			}

			statusText := "Idle"
			statusColor := Dim
			if item.IsProtected {
				statusText = "Protected"
				statusColor = Yellow
			}
			if item.IsActive {
				rowPrefix = "*"
				statusText = "Active"
				statusColor = Bold + Green
			}

			rowContent := fmt.Sprintf("%-2s %-24s %-28s %-10s %s%-12s%s",
				rowPrefix,
				item.Name,
				item.Database,
				FormatBytes(item.SizeBytes),
				statusColor,
				statusText,
				Reset,
			)

			if isSel {
				b.WriteString(cursor + SelectedBg + " " + rowContent + " " + Reset + "\n")
			} else {
				b.WriteString(cursor + " " + rowContent + "\n")
			}
		}
	}

	b.WriteString("\n" + Dim + "─────────────────────────────────────────────────────────────────────────────" + Reset + "\n")

	// 4. Status Bar / Flash
	flashColor := Cyan
	switch m.FlashType {
	case "success":
		flashColor = Green
	case "error":
		flashColor = Red
	}
	b.WriteString(Bold + flashColor + "  ℹ " + m.FlashMessage + Reset + "\n")
	b.WriteString(Dim + "  [↑/k] Up  [↓/j] Down  [Enter/s] Switch  [r] Refresh  [q] Quit" + Reset + "\n")
	b.WriteString(Dim + "  BranchBase • Built with 💚 by @oscarbol09 (github.com/oscarbol09/branchbase)" + Reset + "\n")

	return b.String()
}

// HandleKey processes a single key input event. Returns true if the dashboard should exit.
func (m *DashboardModel) HandleKey(key string, ctx context.Context) (bool, error) {
	key = strings.ToLower(strings.TrimSpace(key))

	switch key {
	case "q", "esc", "ctrl+c", "\x03", "\x1b":
		m.IsQuitting = true
		return true, nil

	case "up", "k", "\x1b[a", "arrow_up":
		m.MoveUp()
		return false, nil

	case "down", "j", "\x1b[b", "arrow_down":
		m.MoveDown()
		return false, nil

	case "r":
		if err := m.Refresh(ctx); err != nil {
			m.SetFlash(fmt.Sprintf("Refresh failed: %v", err), "error")
			return false, err
		}
		m.SetFlash("Refreshed databases and metrics successfully", "success")
		return false, nil

	case "enter", "s", "\r", "\n":
		sel := m.SelectedBranch()
		if sel == nil {
			m.SetFlash("No branch database selected", "info")
			return false, nil
		}
		if m.Driver == nil {
			m.SetFlash("No database driver initialized", "error")
			return false, nil
		}

		defaultBranch := "main"
		if m.Config != nil && m.Config.Proxy.DefaultBranch != "" {
			defaultBranch = m.Config.Proxy.DefaultBranch
		}

		sanitized := git.SanitizeBranchName(sel.Name)
		exists, err := m.Driver.BranchExists(ctx, sanitized)
		if err != nil {
			m.SetFlash(fmt.Sprintf("Failed to check branch: %v", err), "error")
			return false, err
		}

		if !exists {
			if err := m.Driver.CreateBranch(ctx, defaultBranch, sanitized); err != nil {
				m.SetFlash(fmt.Sprintf("Failed to create branch database: %v", err), "error")
				return false, err
			}
			m.SetFlash(fmt.Sprintf("Provisioned & switched to database %q", sel.Database), "success")
		} else {
			m.SetFlash(fmt.Sprintf("Target database %q is ready and active", sel.Database), "success")
		}
		_ = m.Refresh(ctx)
		return false, nil

	default:
		return false, nil
	}
}

// Run executes the interactive TUI event loop reading keys from reader and rendering to writer.
func Run(ctx context.Context, repoPath string, cfg *config.Config, drv driver.Driver, in io.Reader, out io.Writer) error {
	model := NewModel(repoPath, cfg, drv)
	if err := model.Refresh(ctx); err != nil {
		model.SetFlash(fmt.Sprintf("Initial load error: %v", err), "error")
	}

	reader := bufio.NewReader(in)

	// Hide cursor and clear screen
	_, _ = fmt.Fprint(out, CursorHide+ClearScreen)
	defer func() {
		_, _ = fmt.Fprint(out, CursorShow+"\n")
	}()

	// Render initial state
	_, _ = fmt.Fprint(out, ClearScreen+model.RenderView())

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		b, err := reader.ReadByte()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		var key string
		if b == 27 { // Escape character
			// Check if part of arrow sequence
			if reader.Buffered() >= 2 {
				next1, _ := reader.ReadByte()
				next2, _ := reader.ReadByte()
				if next1 == '[' {
					switch next2 {
					case 'A':
						key = "up"
					case 'B':
						key = "down"
					case 'C':
						key = "right"
					case 'D':
						key = "left"
					}
				}
			} else {
				key = "esc"
			}
		} else if b == '\r' || b == '\n' {
			key = "enter"
		} else if b == 3 { // Ctrl+C
			key = "ctrl+c"
		} else {
			key = string(b)
		}

		quit, _ := model.HandleKey(key, ctx)
		if quit {
			return nil
		}

		// Re-render dashboard
		_, _ = fmt.Fprint(out, ClearScreen+model.RenderView())
	}
}
