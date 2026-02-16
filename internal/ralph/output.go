package ralph

import (
	"fmt"
	"io"
)

// writef writes formatted output, ignoring errors.
// Use for non-critical output where write failures are acceptable.
func writef(w io.Writer, format string, args ...interface{}) {
	_, _ = fmt.Fprintf(w, format, args...)
}
