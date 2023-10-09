//go:build !go1.21
// +build !go1.21

package hlog

import (
	"context"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

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
	minLevel   slog.Level
	nocolor    bool
	group      string
	attrs      []slog.Attr
	writer     io.Writer
	prefixName *string
	attrLevels map[string][]attrValueLevel
}

func (ih *Handler) clone() *Handler {
	ih2 := &Handler{
		minLevel:   ih.minLevel,
		nocolor:    ih.nocolor,
		group:      ih.group,
		prefixName: ih.prefixName,
		attrLevels: make(map[string][]attrValueLevel),
	}
	ih2.attrs = append(ih2.attrs, ih.attrs...)
	for k, v := range ih.attrLevels {
		ih2.attrLevels[k] = append(ih2.attrLevels[k], v...)
	}

	return ih2
}

type attrValueLevel struct {
	value slog.Value
	level slog.Level
}

// WithLevel returns a handler with a minimum log level
func (ih *Handler) WithLevel(level slog.Level) *Handler {
	ih2 := ih.clone()
	ih2.minLevel = level
	return ih2
}

// WithoutColor configures the handler to emit logs without using ANSI color directives.
func (ih *Handler) WithoutColor() *Handler {
	ih2 := ih.clone()
	ih2.nocolor = true
	return ih2
}

// WithPrefix designates an attribute that should be formatted as a prefix to the log message instead of being shown
// as a standard attribute.
func (ih *Handler) WithPrefix(name string) *Handler {
	ih2 := ih.clone()
	ih2.prefixName = &name
	return ih2
}

func (ih *Handler) WithWriter(w io.Writer) *Handler {
	ih2 := ih.clone()
	ih2.writer = w
	return ih2
}

// WithAttrLevel associates a log level with an attribute key and value. Any log record with a matching attribute will
// only be emitted if the record's level is greater or equal to the the given level
func (ih *Handler) WithAttrLevel(a slog.Attr, level slog.Level) *Handler {
	ih2 := ih.clone()
	if ih2.attrLevels == nil {
		ih2.attrLevels = make(map[string][]attrValueLevel)
	}
	// TODO: make sure unique?
	ih2.attrLevels[a.Key] = append(ih2.attrLevels[a.Key], attrValueLevel{value: a.Value, level: level})
	return ih2
}

func (ih *Handler) Enabled(_ context.Context, level slog.Level) bool {
	if len(ih.attrLevels) == 0 {
		return level >= ih.minLevel
	}
	return true
}

func (ih *Handler) enabledForRecord(_ context.Context, r slog.Record) bool {
	if r.Level >= ih.minLevel {
		return true
	}
	enabled := false
	for _, a := range ih.attrs {
		if ih.attrHasMinLevel(a, r.Level) {
			return true
		}
	}
	r.Attrs(func(a slog.Attr) bool {
		if enabled {
			return false
		}
		enabled = ih.attrHasMinLevel(a, r.Level)
		return enabled
	})
	return enabled
}

func (ih *Handler) attrHasMinLevel(a slog.Attr, level slog.Level) bool {
	if vs, ok := ih.attrLevels[a.Key]; ok {
		for _, v := range vs {
			if v.value.Equal(a.Value) {
				if level >= v.level {
					return true
				}
			}
		}
	}
	return false
}

func (ih *Handler) Handle(ctx context.Context, r slog.Record) error {
	// Check whether we should log this record
	if len(ih.attrLevels) > 0 {
		if !ih.enabledForRecord(ctx, r) {
			return nil
		}
	}

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

	prefix := ""

	var b strings.Builder
	for _, a := range ih.attrs {
		if ih.prefixName != nil && a.Key == *ih.prefixName {
			prefix = a.Value.String()
		}
		ih.writeAttr(&b, a)
	}
	r.Attrs(func(a slog.Attr) bool {
		if ih.prefixName != nil && a.Key == *ih.prefixName {
			prefix = a.Value.String()
			return true
		}
		ih.writeAttr(&b, a)
		return true
	})

	flatattrs := b.String()
	msg := r.Message
	if prefix != "" {
		msg = prefix + ": " + msg
	}

	w := ih.writer
	if w == nil {
		w = os.Stdout
	}
	fmt.Fprintf(w, "%s | %15s | %-40s %s\n", kind, r.Time.Format("15:04:05.000000"), msg, flatattrs)

	return nil
}

func (ih *Handler) writeAttr(b *strings.Builder, a slog.Attr) {
	b.WriteString(" ")
	if !ih.nocolor {
		b.WriteString(colorBlue)
	}
	b.WriteString(a.Key)
	if !ih.nocolor {
		b.WriteString(colorReset)
	}
	b.WriteString("=")

	switch a.Value.Kind() {
	case slog.KindFloat64:
		v := a.Value.Float64()
		abs := math.Abs(v)
		if abs == 0 || 1e-6 <= v && v < 1e21 {
			b.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
		} else {
			b.WriteString(strconv.FormatFloat(v, 'g', -1, 64))
		}
	case slog.KindDuration:
		v := a.Value.Duration()
		s := v.String()
		if strings.HasSuffix(s, "m0s") {
			s = s[:len(s)-2]
		}
		if strings.HasSuffix(s, "h0m") {
			s = s[:len(s)-2]
		}
		b.WriteString(s)
	case slog.KindTime:
		v := a.Value.Time()
		b.WriteString(v.Format(time.RFC3339Nano))
	default:
		b.WriteString(quote(a.Value.String()))
	}
}

func (ih *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	ih2 := ih.clone()
	ih2.attrs = append(ih2.attrs, attrs...)
	return ih2
}

// WithGroup not supported
func (ih *Handler) WithGroup(name string) slog.Handler {
	ih2 := ih.clone()
	ih2.group = name
	return ih2
}

func quote(s string) string {
	if strings.ContainsAny(s, " ") {
		return fmt.Sprintf("%q", s)
	}
	return s
}
