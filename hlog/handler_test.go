//go:build go1.21
// +build go1.21

package hlog

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"testing/slogtest"
	"time"
)

func TestHandlerOutput(t *testing.T) {
	ts := time.Date(2024, 1, 15, 10, 30, 45, 123456000, time.UTC)

	fmtLine := func(level, msg, attrs string) string {
		return fmt.Sprintf("%s | %15s | %-40s %s\n", level, "10:30:45.123456", msg, attrs)
	}

	tests := []struct {
		name  string
		setup func() *Handler
		level slog.Level
		msg   string
		attrs []slog.Attr
		want  string
	}{
		{
			name:  "info message no attrs",
			setup: func() *Handler { return new(Handler).WithoutColor() },
			level: slog.LevelInfo,
			msg:   "test message",
			want:  fmtLine("info ", "test message", ""),
		},
		{
			name:  "debug message no attrs",
			setup: func() *Handler { return new(Handler).WithoutColor() },
			level: slog.LevelDebug,
			msg:   "debug msg",
			want:  fmtLine("debug", "debug msg", ""),
		},
		{
			name:  "warn message no attrs",
			setup: func() *Handler { return new(Handler).WithoutColor() },
			level: slog.LevelWarn,
			msg:   "warning",
			want:  fmtLine("warn ", "warning", ""),
		},
		{
			name:  "error message no attrs",
			setup: func() *Handler { return new(Handler).WithoutColor() },
			level: slog.LevelError,
			msg:   "something failed",
			want:  fmtLine("error", "something failed", ""),
		},
		{
			name:  "message with string attr",
			setup: func() *Handler { return new(Handler).WithoutColor() },
			level: slog.LevelInfo,
			msg:   "hello",
			attrs: []slog.Attr{slog.String("key", "value")},
			want:  fmtLine("info ", "hello", " key=value"),
		},
		{
			name:  "message with multiple attrs",
			setup: func() *Handler { return new(Handler).WithoutColor() },
			level: slog.LevelInfo,
			msg:   "multi",
			attrs: []slog.Attr{slog.String("a", "1"), slog.Int("b", 2)},
			want:  fmtLine("info ", "multi", " a=1 b=2"),
		},
		{
			name: "message with preexisting attrs",
			setup: func() *Handler {
				return new(Handler).WithoutColor().WithAttrs([]slog.Attr{slog.String("pre", "set")}).(*Handler)
			},
			level: slog.LevelInfo,
			msg:   "msg",
			attrs: []slog.Attr{slog.String("k", "v")},
			want:  fmtLine("info ", "msg", " pre=set k=v"),
		},
		{
			name:  "message with duration attr",
			setup: func() *Handler { return new(Handler).WithoutColor() },
			level: slog.LevelInfo,
			msg:   "timing",
			attrs: []slog.Attr{slog.Duration("elapsed", 5*time.Second)},
			want:  fmtLine("info ", "timing", " elapsed=5s"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			h := tt.setup().WithWriter(&buf)

			r := slog.NewRecord(ts, tt.level, tt.msg, 0)
			r.AddAttrs(tt.attrs...)
			if err := h.Handle(context.Background(), r); err != nil {
				t.Fatal(err)
			}

			got := buf.String()
			if got != tt.want {
				t.Errorf("output mismatch\ngot:  %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func TestHandler(t *testing.T) {
	var buf bytes.Buffer
	results := func() []map[string]any {
		var ms []map[string]any
		for _, line := range bytes.Split(buf.Bytes(), []byte{'\n'}) {
			if len(line) == 0 {
				continue
			}
			m, err := parseLogLine(string(line))
			if err != nil {
				t.Errorf("%q: %v", string(line), err)
				continue
			}
			ms = append(ms, m)
		}
		return ms
	}

	h := new(Handler).WithoutColor().WithWriter(&buf)

	err := slogtest.TestHandler(h, results)
	if err != nil {
		t.Errorf("handler failed test: %+v", err)
	}
}

func parseLogLine(line string) (map[string]any, error) {
	slvl, sline, ok := strings.Cut(line, "|")
	if !ok {
		return nil, fmt.Errorf("failed to find level segment of log line")
	}
	m := map[string]any{}

	slvl = strings.TrimSpace(slvl)
	switch slvl {
	case "error":
		m[slog.LevelKey] = slog.LevelError
	case "warn":
		m[slog.LevelKey] = slog.LevelWarn
	case "info":
		m[slog.LevelKey] = slog.LevelInfo
	case "debug":
		m[slog.LevelKey] = slog.LevelDebug
	}

	var stime string
	stime, sline, ok = strings.Cut(sline, "|")
	if !ok {
		return nil, fmt.Errorf("failed to find time segment of log line")
	}

	stime = strings.TrimSpace(stime)
	if stime != "" {
		ptime, err := time.Parse("15:04:05.000000", stime)
		if err != nil {
			return nil, fmt.Errorf("failed to parse time segment %s: %v", stime, err)
		}
		now := time.Now()
		m[slog.TimeKey] = ptime.AddDate(now.Year(), int(now.Month())-1, now.Day())
	}

	// After the time pipe, the remainder is: %-40s%s
	// where the first part is the message (left-padded to 40 chars)
	// and the second part is space-separated key=value attrs.
	// Split into message and attrs by finding the first key=value token.
	sline = strings.TrimLeft(sline, " ")
	fields := strings.Fields(sline)
	attrStart := len(fields)
	for i := len(fields) - 1; i >= 0; i-- {
		if strings.Contains(fields[i], "=") {
			attrStart = i
		} else {
			break
		}
	}
	m[slog.MessageKey] = strings.Join(fields[:attrStart], " ")

	if attrStart < len(fields) {
		parseAttrs(m, strings.Join(fields[attrStart:], " "))
	}

	return m, nil
}

func parseAttrs(m map[string]any, s string) {
	for s != "" {
		// Find key
		eqIdx := strings.IndexByte(s, '=')
		if eqIdx < 0 {
			break
		}
		key := s[:eqIdx]
		s = s[eqIdx+1:]

		var value string
		if strings.HasPrefix(s, `"`) {
			// Quoted value
			end := 1
			for end < len(s) {
				if s[end] == '\\' {
					end += 2
					continue
				}
				if s[end] == '"' {
					end++
					break
				}
				end++
			}
			value, _ = strconv.Unquote(s[:end])
			s = s[end:]
		} else {
			spIdx := strings.IndexByte(s, ' ')
			if spIdx < 0 {
				value = s
				s = ""
			} else {
				value = s[:spIdx]
				s = s[spIdx:]
			}
		}

		s = strings.TrimSpace(s)
		setNestedValue(m, key, value)
	}
}

func setNestedValue(m map[string]any, key string, value any) {
	parts := strings.Split(key, ".")
	for i := 0; i < len(parts)-1; i++ {
		sub, ok := m[parts[i]]
		if !ok {
			sub = map[string]any{}
			m[parts[i]] = sub
		}
		m = sub.(map[string]any)
	}
	m[parts[len(parts)-1]] = value
}
