// Package tui is an interactive terminal front-end for the client: fetch
// the account's exit nodes from the control plane, pick one or many, and
// start/stop routing traffic through them, all without restarting the
// process or editing a config file.
package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/bethrou/bethrou/internal/client"
	"github.com/bethrou/bethrou/internal/config"
	"github.com/bethrou/bethrou/pkg/control"
	"github.com/bethrou/bethrou/pkg/logging"
)

// Run validates cfg, opens a session against the control plane, and hands
// control of the terminal to the interactive picker until the user quits.
// Background logs are redirected to a file (printed on exit) since they'd
// otherwise corrupt the full-screen UI.
func Run(ctx context.Context, cfg *config.ClientConfig) error {
	// Fail fast, before opening a log file, on an obviously-bad config.
	// client.NewSession (below) enforces the same invariants — including
	// defaulting IdentityKey the same way — so this is a redundant but
	// harmless early check, not a second source of truth for what "valid"
	// means; see client/client/client.go's prepareConfig.
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("invalid configuration: %w", err)
	}

	logPath := filepath.Join(os.TempDir(), "bethrou.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer logFile.Close()
	logging.SetupOutput(cfg.Log, logFile)

	sess, err := client.NewSession(ctx, cfg)
	if err != nil {
		return err
	}

	listenAddr := cfg.Server.ListenAddr
	if listenAddr == "" {
		listenAddr = "127.0.0.1:1080"
	}

	m := newModel(ctx, sess, listenAddr)
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui error: %w", err)
	}

	// Belt and suspenders: Update already stops the session on quit, but
	// make sure nothing's left running (e.g. the program exited via an
	// unexpected path).
	if sess.Running() {
		_ = sess.Stop()
	}

	fmt.Printf("Logs were written to %s\n", logPath)
	return nil
}

type state int

const (
	stateLoading state = iota
	statePicking
	stateStarting
	stateRunning
	stateStopping
)

type model struct {
	ctx        context.Context
	session    *client.Session
	listenAddr string

	state    state
	nodes    []control.NodeSummary
	cursor   int
	selected map[string]bool
	err      error
}

func newModel(ctx context.Context, session *client.Session, listenAddr string) model {
	return model{
		ctx:        ctx,
		session:    session,
		listenAddr: listenAddr,
		selected:   map[string]bool{},
		state:      stateLoading,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.fetchNodesCmd, waitCtxDoneCmd(m.ctx))
}

type (
	nodesMsg   struct{ nodes []control.NodeSummary }
	errMsg     struct{ err error }
	startedMsg struct{}
	stoppedMsg struct{}
	ctxDoneMsg struct{}
)

func (m model) fetchNodesCmd() tea.Msg {
	nodes, err := m.session.Nodes(m.ctx)
	if err != nil {
		return errMsg{err}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Label < nodes[j].Label })
	return nodesMsg{nodes}
}

func waitCtxDoneCmd(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		<-ctx.Done()
		return ctxDoneMsg{}
	}
}

func (m model) selectedIDs() []string {
	var ids []string
	for _, n := range m.nodes {
		if m.selected[n.ID] {
			ids = append(ids, n.ID)
		}
	}
	return ids
}

func (m model) startCmd() tea.Cmd {
	ids := m.selectedIDs()
	return func() tea.Msg {
		if err := m.session.Start(m.ctx, ids); err != nil {
			return errMsg{err}
		}
		return startedMsg{}
	}
}

func (m model) stopCmd() tea.Cmd {
	return func() tea.Msg {
		if err := m.session.Stop(); err != nil {
			return errMsg{err}
		}
		return stoppedMsg{}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ctxDoneMsg:
		if m.session.Running() {
			_ = m.session.Stop()
		}
		return m, tea.Quit

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			if m.session.Running() {
				_ = m.session.Stop()
			}
			return m, tea.Quit
		}

		switch m.state {
		case statePicking:
			switch msg.String() {
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "j":
				if m.cursor < len(m.nodes)-1 {
					m.cursor++
				}
			case " ", "x":
				if len(m.nodes) > 0 {
					id := m.nodes[m.cursor].ID
					m.selected[id] = !m.selected[id]
				}
			case "a":
				allSelected := len(m.nodes) > 0
				for _, n := range m.nodes {
					if !m.selected[n.ID] {
						allSelected = false
						break
					}
				}
				for _, n := range m.nodes {
					m.selected[n.ID] = !allSelected
				}
			case "r":
				m.state = stateLoading
				m.err = nil
				return m, m.fetchNodesCmd
			case "enter":
				if len(m.selectedIDs()) == 0 {
					m.err = fmt.Errorf("select at least one node first (space to toggle)")
					return m, nil
				}
				m.err = nil
				m.state = stateStarting
				return m, m.startCmd()
			}

		case stateRunning:
			if msg.String() == "s" {
				m.state = stateStopping
				return m, m.stopCmd()
			}
		}

	case nodesMsg:
		m.nodes = msg.nodes
		m.state = statePicking
		if m.cursor >= len(m.nodes) {
			m.cursor = 0
		}

	case errMsg:
		m.err = msg.err
		if m.state == stateLoading || m.state == stateStarting || m.state == stateStopping {
			m.state = statePicking
		}

	case startedMsg:
		m.state = stateRunning
		m.err = nil

	case stoppedMsg:
		m.state = statePicking
		m.err = nil
	}

	return m, nil
}

var (
	titleStyle  = lipgloss.NewStyle().Bold(true)
	errStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	dimStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cursorStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	onlineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	helpStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)

func (m model) View() string {
	var b strings.Builder

	b.WriteString(titleStyle.Render("Bethrou client") + "\n\n")

	if m.err != nil {
		b.WriteString(errStyle.Render("error: "+m.err.Error()) + "\n\n")
	}

	switch m.state {
	case stateLoading:
		b.WriteString("Loading nodes from the control plane...\n")

	case statePicking, stateStarting:
		if len(m.nodes) == 0 {
			b.WriteString("No exit nodes enrolled on this account yet. Enroll one, then press r to refresh.\n")
		} else {
			b.WriteString("Select node(s) to connect through:\n\n")
			for i, n := range m.nodes {
				cursor := "  "
				if i == m.cursor {
					cursor = "> "
				}
				check := "[ ]"
				if m.selected[n.ID] {
					check = "[x]"
				}
				label := n.Label
				if label == "" {
					label = "(unlabeled)"
				}
				line := fmt.Sprintf("%s%s %-20s %s", cursor, check, label, dimStyle.Render(n.ID))
				if i == m.cursor {
					line = cursorStyle.Render(fmt.Sprintf("%s%s %-20s %s", cursor, check, label, n.ID))
				}
				b.WriteString(line + "\n")
			}
		}
		if m.state == stateStarting {
			b.WriteString("\nstarting...\n")
		}
		b.WriteString("\n" + helpStyle.Render("↑/↓ move · space toggle · a toggle all · r refresh · enter start · q quit") + "\n")

	case stateRunning, stateStopping:
		var names []string
		for _, n := range m.nodes {
			if m.selected[n.ID] {
				label := n.Label
				if label == "" {
					label = n.ID
				}
				names = append(names, label)
			}
		}
		b.WriteString(onlineStyle.Render("● running") + "\n\n")
		b.WriteString(fmt.Sprintf("Nodes:       %s\n", strings.Join(names, ", ")))
		b.WriteString(fmt.Sprintf("In pool:     %d\n", m.session.PoolSize()))
		b.WriteString(fmt.Sprintf("SOCKS5:      socks5://%s\n", m.listenAddr))
		if m.state == stateStopping {
			b.WriteString("\nstopping...\n")
		}
		b.WriteString("\n" + helpStyle.Render("s stop · q quit") + "\n")
	}

	return b.String()
}
