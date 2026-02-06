package ui

import (
	"bytes"
	"io"
	"os/exec"

	"devdeploy/internal/pty"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// ShellOutputMsg carries bytes read from the PTY for display.
type ShellOutputMsg struct {
	Data []byte
}

// ShellView is a PTY-backed overlay that spawns a shell and passes through stdin/stdout.
// The user can type and run commands; output is displayed in a viewport.
// Esc dismisses (does not pass through to the shell).
type ShellView struct {
	ptyRunner pty.Runner
	ptmx      io.ReadWriteCloser
	content   *bytes.Buffer
	viewport  viewport.Model
	width     int
	height    int
	workDir   string
	outputCh  chan []byte
}

// Ensure ShellView implements View.
var _ View = (*ShellView)(nil)

const defaultShellWidth = 70
const defaultShellHeight = 18

// NewShellView creates a shell view that will spawn a PTY in workDir.
// The ptyRunner is injected so implementations can be swapped.
func NewShellView(ptyRunner pty.Runner, workDir string) *ShellView {
	vp := viewport.New(defaultShellWidth, defaultShellHeight)
	vp.Style = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("205")).
		Padding(0, 1)
	return &ShellView{
		ptyRunner: ptyRunner,
		content:   &bytes.Buffer{},
		viewport:  vp,
		width:     defaultShellWidth,
		height:    defaultShellHeight,
		workDir:   workDir,
		outputCh:  make(chan []byte, 64),
	}
}

// Init implements View. Spawns the shell and starts reading from PTY.
func (s *ShellView) Init() tea.Cmd {
	shell := "sh"
	if path, err := exec.LookPath("bash"); err == nil {
		shell = path
	}
	cmd := exec.Command(shell)
	cmd.Dir = s.workDir
	if cmd.Dir == "" {
		cmd.Dir = "."
	}

	sz := pty.Size{Rows: uint16(defaultShellHeight), Cols: uint16(defaultShellWidth)}
	ptmx, err := s.ptyRunner.Start(nil, cmd, sz)
	if err != nil {
		// Fallback: show error in view
		s.content.WriteString("Failed to spawn shell: " + err.Error() + "\r\n")
		s.refreshViewport()
		return nil
	}
	s.ptmx = ptmx

	// Goroutine: read from PTY, send to channel
	go func() {
		buf := make([]byte, 256)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				cp := make([]byte, n)
				copy(cp, buf[:n])
				select {
				case s.outputCh <- cp:
				default:
					// Channel full, drop (avoid blocking)
				}
			}
			if err != nil {
				close(s.outputCh)
				return
			}
		}
	}()

	return s.waitForOutput()
}

func (s *ShellView) waitForOutput() tea.Cmd {
	return func() tea.Msg {
		data, ok := <-s.outputCh
		if !ok {
			return nil
		}
		return ShellOutputMsg{Data: data}
	}
}

// Update implements View.
func (s *ShellView) Update(msg tea.Msg) (View, tea.Cmd) {
	switch msg := msg.(type) {
	case ShellOutputMsg:
		if s.ptmx != nil {
			s.content.Write(msg.Data)
			s.refreshViewport()
			s.viewport.GotoBottom()
		}
		return s, s.waitForOutput()
	case tea.KeyMsg:
		if msg.String() == "esc" {
			return s, func() tea.Msg { return DismissModalMsg{} }
		}
		if s.ptmx != nil {
			b := keyToPTYBytes(msg)
			if len(b) > 0 {
				s.ptmx.Write(b)
			}
		}
		return s, s.waitForOutput()
	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height
		w := msg.Width - 4
		h := msg.Height/2 + 4
		if w < 40 {
			w = 40
		}
		if h < 12 {
			h = 12
		}
		s.viewport.Width = w
		s.viewport.Height = h
		if s.ptmx != nil && s.ptyRunner != nil {
			s.ptyRunner.Resize(s.ptmx, pty.Size{Rows: uint16(h), Cols: uint16(w)})
		}
		s.refreshViewport()
		return s, s.waitForOutput()
	}

	var cmd tea.Cmd
	s.viewport, cmd = s.viewport.Update(msg)
	return s, tea.Batch(cmd, s.waitForOutput())
}

// View implements View.
func (s *ShellView) View() string {
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("86"))
	header := titleStyle.Render("Agent shell") + lipgloss.NewStyle().Foreground(lipgloss.Color("241")).Render("  Esc: exit")
	return header + "\n" + s.viewport.View()
}

func (s *ShellView) refreshViewport() {
	s.viewport.SetContent(s.content.String())
}

// keyToPTYBytes converts a Bubble Tea KeyMsg to bytes the PTY expects.
func keyToPTYBytes(msg tea.KeyMsg) []byte {
	switch msg.Type {
	case tea.KeyEnter:
		return []byte{'\r'}
	case tea.KeyBackspace:
		return []byte{0x7f}
	case tea.KeyTab:
		return []byte{'\t'}
	case tea.KeySpace:
		return []byte{' '}
	case tea.KeyUp:
		return []byte{0x1b, '[', 'A'}
	case tea.KeyDown:
		return []byte{0x1b, '[', 'B'}
	case tea.KeyRight:
		return []byte{0x1b, '[', 'C'}
	case tea.KeyLeft:
		return []byte{0x1b, '[', 'D'}
	case tea.KeyCtrlC:
		return []byte{0x03}
	case tea.KeyCtrlD:
		return []byte{0x04}
	case tea.KeyEsc:
		return []byte{0x1b}
	case tea.KeyRunes:
		return []byte(string(msg.Runes))
	default:
		// Try runes for unknown types
		if len(msg.Runes) > 0 {
			return []byte(string(msg.Runes))
		}
		return nil
	}
}

// Close releases PTY resources. Call when dismissing the overlay.
func (s *ShellView) Close() error {
	if s.ptmx != nil {
		if c, ok := s.ptmx.(io.Closer); ok {
			return c.Close()
		}
	}
	return nil
}
