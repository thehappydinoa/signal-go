// Package qrterminal renders QR codes as compact ANSI half-block art for TTYs.
// It wraps [github.com/skip2/go-qrcode] (see ADR 0035).
package qrterminal

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/skip2/go-qrcode"
	"golang.org/x/term"
)

// ErrDisabled indicates QR output was skipped (non-TTY, NO_COLOR, or OptOut).
var ErrDisabled = errors.New("qrterminal: disabled")

// Options configures [Write].
type Options struct {
	// Level is the QR error-correction level. Zero defaults to [qrcode.Medium].
	Level qrcode.RecoveryLevel
	// Writer is the destination. When nil, [Write] uses [os.Stdout].
	Writer io.Writer
	// OptOut skips rendering even on a TTY (CLI -no-qr).
	OptOut bool
}

// Enabled reports whether [Write] would render for w (terminal and not OptOut).
func Enabled(w io.Writer, optOut bool) bool {
	if optOut || os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// Write encodes content as a QR code and prints half-block ANSI art to opts.Writer.
// Returns [ErrDisabled] when output is skipped; other errors come from the encoder.
func Write(content string, opts Options) error {
	w := opts.Writer
	if w == nil {
		w = os.Stdout
	}
	if !Enabled(w, opts.OptOut) {
		return ErrDisabled
	}
	level := opts.Level
	if level == 0 {
		level = qrcode.Medium
	}
	qr, err := qrcode.New(content, level)
	if err != nil {
		return fmt.Errorf("qrterminal: encode: %w", err)
	}
	art := strings.TrimRight(qr.ToSmallString(false), "\n")
	if art == "" {
		return errors.New("qrterminal: empty render")
	}
	if _, err := fmt.Fprintln(w, art); err != nil {
		return fmt.Errorf("qrterminal: write: %w", err)
	}
	return nil
}
