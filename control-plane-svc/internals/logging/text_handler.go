// Package logging provides a plain-text slog handler that writes lines as:
//
//	2026-11-10 14:03:22.123 [service] reconciling realm users realm=dc-1
//
// The module is the name of the Go package that emitted the log call.
package logging

import (
	"context"
	"io"
	"log"
	"log/slog"
	"runtime"
	"strings"
	"sync"
)

const timeFormat = "2006-01-02 15:04:05.000"

type TextHandler struct {
	mu    *sync.Mutex
	w     io.Writer
	level slog.Leveler
	attrs []slog.Attr
}

func NewTextHandler(w io.Writer, level slog.Leveler) *TextHandler {
	return &TextHandler{mu: &sync.Mutex{}, w: w, level: level}
}

// Setup installs the handler as the process-wide default, for slog and for the
// standard "log" package.
func Setup(w io.Writer, level slog.Leveler) {
	// slog.SetDefault only captures the caller of log.Printf & co. when a file
	// flag is set; it resets the flags to 0 afterwards.
	log.SetFlags(log.Lshortfile)
	slog.SetDefault(slog.New(NewTextHandler(w, level)))
}

func (h *TextHandler) Enabled(_ context.Context, l slog.Level) bool {
	return l >= h.level.Level()
}

func (h *TextHandler) Handle(_ context.Context, r slog.Record) error {
	var b strings.Builder
	b.WriteString(r.Time.Format(timeFormat))
	b.WriteString(" [")
	b.WriteString(module(r.PC))
	b.WriteString("] ")
	b.WriteString(r.Message)

	writeAttr := func(a slog.Attr) bool {
		b.WriteByte(' ')
		b.WriteString(a.Key)
		b.WriteByte('=')
		b.WriteString(a.Value.Resolve().String())
		return true
	}
	for _, a := range h.attrs {
		writeAttr(a)
	}
	r.Attrs(writeAttr)
	b.WriteByte('\n')

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, b.String())
	return err
}

func (h *TextHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h2 := *h
	h2.attrs = append(append([]slog.Attr{}, h.attrs...), attrs...)
	return &h2
}

// WithGroup is a no-op: groups aren't used and would clutter plain-text output.
func (h *TextHandler) WithGroup(string) slog.Handler { return h }

// module returns the last element of the package path of the function at pc,
// e.g. "control-plane-svc/internals/service.(*X).Run" -> "service".
func module(pc uintptr) string {
	if pc == 0 {
		return "unknown"
	}
	fn, _ := runtime.CallersFrames([]uintptr{pc}).Next()
	name := fn.Function
	if i := strings.LastIndexByte(name, '/'); i >= 0 {
		name = name[i+1:]
	}
	if i := strings.IndexByte(name, '.'); i >= 0 {
		name = name[:i]
	}
	return name
}
