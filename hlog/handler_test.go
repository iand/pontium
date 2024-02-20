//go:build go1.21
// +build go1.21

package hlog

import (
	"bytes"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"testing/slogtest"
	"time"
)

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
	slvl, sline, ok := strings.Cut(string(line), "|")
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
	stime, sline, ok = strings.Cut(string(sline), "|")
	if !ok {
		return nil, fmt.Errorf("failed to find time segment of log line")
	}

	stime = strings.TrimSpace(stime)
	ptime, err := time.Parse("15:04:05.000000", stime)
	if err != nil {
		return nil, fmt.Errorf("failed to parse time segment %s: %v", stime, err)
	}
	now := time.Now()
	m[slog.TimeKey] = ptime.AddDate(now.Year(), int(now.Month())-1, now.Day())

	var msg string
	msg, sline, ok = strings.Cut(string(sline), "|")

	m[slog.MessageKey] = strings.TrimSpace(msg)

	_ = ptime
	_ = sline

	return m, nil
}
