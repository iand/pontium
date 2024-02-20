//go:build go1.21
// +build go1.21

package hlog

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
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
	attrLevels map[string][]attrValueLevel // associates an attribute key with a value and a log level
}

func (h *Handler) clone() *Handler {
	h2 := &Handler{
		minLevel:   h.minLevel,
		nocolor:    h.nocolor,
		group:      h.group,
		prefixName: h.prefixName,
		attrLevels: make(map[string][]attrValueLevel),
		writer:     h.writer,
	}
	h2.attrs = append(h2.attrs, h.attrs...)
	for k, v := range h.attrLevels {
		h2.attrLevels[k] = append(h2.attrLevels[k], v...)
	}

	return h2
}

type attrValueLevel struct {
	value slog.Value
	level slog.Level
}

// WithLevel returns a new Handler with a minimum log level set to level. The new
// Handler is otherwise identical to the receiver.
func (h *Handler) WithLevel(level slog.Level) *Handler {
	h2 := h.clone()
	h2.minLevel = level
	return h2
}

// WithoutColor returns a new Handler that is configured to emit logs without using ANSI
// color directives. The new Handler is otherwise identical to the receiver.
func (h *Handler) WithoutColor() *Handler {
	h2 := h.clone()
	h2.nocolor = true
	return h2
}

// WithPrefix returns a new Handler that designates the named attribute to be written as
// a prefix to the log message instead of being shown alongide other attributes. The
// new Handler is otherwise identical to the receiver. A Handler may only have one
// attribute designated as a prefix.
func (h *Handler) WithPrefix(name string) *Handler {
	h2 := h.clone()
	h2.prefixName = &name
	return h2
}

// WithWriter returns a new Handler that writes output to w. The new Handler is
// otherwise identical to the receiver.
func (h *Handler) WithWriter(w io.Writer) *Handler {
	h2 := h.clone()
	h2.writer = w
	return h2
}

// WithAttrLevel returns a new Handler that associates a log level with an attribute key
// and value. Any log record with a matching attribute will only be emitted if the
// record's level is greater or equal to the the given level. For example this could be
// used for controlling log levels by package name if the package is provided as an
// attribute in each log record. The new Handler is otherwise identical to the
// receiver.
func (h *Handler) WithAttrLevel(a slog.Attr, level slog.Level) *Handler {
	h2 := h.clone()
	if h2.attrLevels == nil {
		h2.attrLevels = make(map[string][]attrValueLevel)
	}
	// TODO: make sure unique?
	h2.attrLevels[a.Key] = append(h2.attrLevels[a.Key], attrValueLevel{value: a.Value, level: level})
	return h2
}

// nabled reports whether the handler handles records at the given level.
func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	if len(h.attrLevels) == 0 {
		return level >= h.minLevel
	}
	return true
}

func (h *Handler) enabledForRecord(_ context.Context, r slog.Record) bool {
	if r.Level >= h.minLevel {
		return true
	}
	enabled := false
	for _, a := range h.attrs {
		if h.attrHasMinLevel(a, r.Level) {
			return true
		}
	}
	r.Attrs(func(a slog.Attr) bool {
		if enabled {
			return false
		}
		enabled = h.attrHasMinLevel(a, r.Level)
		return enabled
	})
	return enabled
}

func (h *Handler) attrHasMinLevel(a slog.Attr, level slog.Level) bool {
	if vs, ok := h.attrLevels[a.Key]; ok {
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

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	// Check whether we should log this record
	if len(h.attrLevels) > 0 {
		if !h.enabledForRecord(ctx, r) {
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

	if !h.nocolor {
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
	for _, a := range h.attrs {
		// Ignore empty attrs
		if a.Equal(slog.Attr{}) {
			continue
		}

		if h.prefixName != nil && a.Key == *h.prefixName {
			prefix = a.Value.String()
		}
		h.writeAttr(&b, a)
	}
	r.Attrs(func(a slog.Attr) bool {
		// Ignore empty attrs
		if a.Equal(slog.Attr{}) {
			return true
		}
		if h.prefixName != nil && a.Key == *h.prefixName {
			prefix = a.Value.String()
			return true
		}
		h.writeAttr(&b, a)
		return true
	})

	flatattrs := b.String()
	msg := r.Message
	if prefix != "" {
		msg = prefix + ": " + msg
	}

	w := h.writer
	if w == nil {
		w = os.Stdout
	}
	fmt.Fprintf(w, "%s | %15s | %-40s %s\n", kind, r.Time.Format("15:04:05.000000"), msg, flatattrs)

	return nil
}

func (h *Handler) writeAttr(b *strings.Builder, a slog.Attr) {
	b.WriteString(" ")
	if !h.nocolor {
		b.WriteString(colorBlue)
	}
	b.WriteString(a.Key)
	if !h.nocolor {
		b.WriteString(colorReset)
	}
	b.WriteString("=")

	rv := a.Value.Resolve()
	switch rv.Kind() {
	case slog.KindFloat64:
		v := rv.Float64()
		abs := math.Abs(v)
		if abs == 0 || 1e-6 <= v && v < 1e21 {
			b.WriteString(strconv.FormatFloat(v, 'f', -1, 64))
		} else {
			b.WriteString(strconv.FormatFloat(v, 'g', -1, 64))
		}
	case slog.KindDuration:
		v := rv.Duration()
		s := v.String()
		if strings.HasSuffix(s, "m0s") {
			s = s[:len(s)-2]
		}
		if strings.HasSuffix(s, "h0m") {
			s = s[:len(s)-2]
		}
		b.WriteString(s)
	case slog.KindTime:
		v := rv.Time()
		b.WriteString(v.Format(time.RFC3339Nano))
	default:
		b.WriteString(quote(rv.String()))
	}
}

func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	h2 := h.clone()
	h2.attrs = append(h2.attrs, attrs...)
	return h2
}

func (h *Handler) WithGroup(name string) slog.Handler {
	h2 := h.clone()
	h2.group = name
	return h2
}

func quote(s string) string {
	if strings.ContainsAny(s, " ") {
		return fmt.Sprintf("%q", s)
	}
	return s
}
