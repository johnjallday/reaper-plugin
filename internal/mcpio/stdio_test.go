package mcpio

import (
	"bufio"
	"bytes"
	"encoding/json"
	"testing"
)

func TestReadWriteMessage(t *testing.T) {
	var out bytes.Buffer
	msg := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "ping"}

	if err := WriteMessage(&out, msg, FramingContentLength); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}

	reader := bufio.NewReader(bytes.NewReader(out.Bytes()))
	raw, framing, err := ReadMessage(reader)
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if framing != FramingContentLength {
		t.Fatalf("framing = %v, want %v", framing, FramingContentLength)
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}

	if parsed["method"] != "ping" {
		t.Fatalf("method = %v, want ping", parsed["method"])
	}
}

func TestReadWriteMessageLineDelimited(t *testing.T) {
	var out bytes.Buffer
	msg := map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"}

	if err := WriteMessage(&out, msg, FramingLineDelimited); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}

	reader := bufio.NewReader(bytes.NewReader(out.Bytes()))
	raw, framing, err := ReadMessage(reader)
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if framing != FramingLineDelimited {
		t.Fatalf("framing = %v, want %v", framing, FramingLineDelimited)
	}

	var parsed map[string]any
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}

	if parsed["method"] != "tools/list" {
		t.Fatalf("method = %v, want tools/list", parsed["method"])
	}
}
