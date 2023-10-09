//go:build go1.21
// +build go1.21

package hlog

import (
	"bytes"
	"encoding/json"
	"testing"
	"testing/slogtest"
)

func TestHandler(t *testing.T) {
	var buf bytes.Buffer
	results := func() []map[string]any {
		var ms []map[string]any
		for _, line := range bytes.Split(buf.Bytes(), []byte{'\n'}) {
			if len(line) == 0 {
				continue
			}
			var m map[string]any
			if err := json.Unmarshal(line, &m); err != nil {
				t.Fatal(err)
			}
			ms = append(ms, m)
		}
		return ms
	}

	h := new(Handler).WithWriter(&buf)

	err := slogtest.TestHandler(h, results)
	if err != nil {
		t.Errorf("handler failed test: %v", err)
	}
}
