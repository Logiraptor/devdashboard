package pty

import (
	"context"
	"io"
	"os"
	"os/exec"

	"github.com/creack/pty"
)

// Size represents terminal dimensions in rows and columns.
type Size struct {
	Rows uint16
	Cols uint16
}

// Runner is the interface for spawning and controlling a PTY.
// Implementations can be swapped (e.g. creack/pty, or a mock for tests).
type Runner interface {
	Start(ctx context.Context, cmd *exec.Cmd, size Size) (io.ReadWriteCloser, error)
	Resize(rwc io.ReadWriteCloser, size Size) error
}

// CreackPTY implements Runner using github.com/creack/pty.
type CreackPTY struct{}

// Ensure CreackPTY implements Runner.
var _ Runner = (*CreackPTY)(nil)

// Start implements Runner. Spawns cmd in a PTY with the given size.
func (c *CreackPTY) Start(ctx context.Context, cmd *exec.Cmd, size Size) (io.ReadWriteCloser, error) {
	ws := &pty.Winsize{Rows: size.Rows, Cols: size.Cols}
	f, err := pty.StartWithSize(cmd, ws)
	if err != nil {
		return nil, err
	}
	// Context cancellation is handled by the caller (e.g. closing the returned ReadWriteCloser).
	return f, nil
}

// Resize implements Runner. Resizes the PTY to the given dimensions.
// The rwc must be the *os.File returned by Start; other types are no-op.
func (c *CreackPTY) Resize(rwc io.ReadWriteCloser, size Size) error {
	f, ok := rwc.(*os.File)
	if !ok {
		return nil // No-op if we can't get the underlying file
	}
	return pty.Setsize(f, &pty.Winsize{Rows: size.Rows, Cols: size.Cols})
}
