package hlog

import (
	"context"
	"fmt"
	"strings"

	"golang.org/x/exp/slog"
)

const (
	colorReset  = "\x1b[0m"
	colorRed    = "\x1b[1;31m"
	colorGreen  = "\x1b[1;32m"
	colorYellow = "\x1b[1;33m"
	colorBlue   = "\x1b[1;34m"
)

var _ slog.Handler = (*Handler)(nil)

// Handler is a slog logging handler that provides human friendly log output. It's not intended to be used in high
// throughput situations, but is more suited to logs that a human might want to watch, such during as the development
// phase of a service.
type Handler struct {
	level      slog.Level
	nocolor    bool
	flatattrs  string
	prefix     string
	prefixName *string
}

func (ih *Handler) WithLevel(level slog.Level) *Handler {
	ih.level = level
	return ih
}

// WithoutColor configures the handler to emit logs without using ANSI color directives.
func (ih *Handler) WithoutColor() *Handler {
	ih.nocolor = true
	return ih
}

// WithPrefix designates an attribute that should be formatted as a prefix to the log message instead of being shown
// as a standard attribute.
func (ih *Handler) WithPrefix(name string) *Handler {
	ih.prefixName = &name
	return ih
}

func (ih *Handler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= ih.level
}

func (ih *Handler) Handle(_ context.Context, r slog.Record) error {
	kind := "???"
	switch r.Level {
	case slog.LevelError:
		kind = "error"
	case slog.LevelWarn:
		kind = "warn"
	case slog.LevelInfo:
		kind = "info"
	case slog.LevelDebug:
		kind = "debug"
	default:
		kind = fmt.Sprintf("%02d", r.Level)
	}

	if !ih.nocolor {
		if r.Level >= slog.LevelError {
			kind = fmt.Sprintf("%s%-5s%s", colorRed, kind, colorReset)
		} else if r.Level >= slog.LevelWarn {
			kind = fmt.Sprintf("%s%-5s%s", colorYellow, kind, colorReset)
		} else if r.Level >= slog.LevelInfo {
			kind = fmt.Sprintf("%s%-5s%s", colorGreen, kind, colorReset)
		}
	} else {
		kind = fmt.Sprintf("%-5s", kind)
	}

	prefix := ih.prefix

	var b strings.Builder
	r.Attrs(func(a slog.Attr) {
		if ih.prefixName != nil && a.Key == *ih.prefixName {
			prefix = a.Value.String()
			return
		}
		b.WriteString(" ")
		if !ih.nocolor {
			b.WriteString(colorBlue)
		}
		b.WriteString(a.Key)
		if !ih.nocolor {
			b.WriteString(colorReset)
		}
		b.WriteString("=")
		b.WriteString(quote(a.Value.String()))
	})

	flatattrs := b.String()
	msg := r.Message
	if prefix != "" {
		msg = prefix + ": " + msg
	}

	fmt.Printf("%s | %15s | %-40s %s\n", kind, r.Time.Format("15:04:05.000000"), msg, flatattrs)

	return nil
}

func (ih *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	ih2 := &Handler{
		nocolor:    ih.nocolor,
		level:      ih.level,
		prefix:     ih.prefix,
		prefixName: ih.prefixName,
	}

	b := new(strings.Builder)
	b.WriteString(ih.flatattrs)
	for _, a := range attrs {
		if ih.prefixName != nil && a.Key == *ih.prefixName {
			ih2.prefix = a.Value.String()
			continue
		}
		b.WriteString(" ")
		if !ih.nocolor {
			b.WriteString(colorYellow)
		}
		b.WriteString(a.Key)
		if !ih.nocolor {
			b.WriteString(colorReset)
		}
		b.WriteString("=")
		b.WriteString(quote(a.Value.String()))
	}

	ih2.flatattrs = b.String()

	return ih2
}

func (ih *Handler) WithGroup(name string) slog.Handler {
	return ih
}

func quote(s string) string {
	if strings.ContainsAny(s, " ") {
		return fmt.Sprintf("%q", s)
	}
	return s
}
