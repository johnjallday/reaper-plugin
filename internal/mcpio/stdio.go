package mcpio

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// FramingMode tracks how stdio messages are framed.
type FramingMode int

const (
	FramingContentLength FramingMode = iota
	FramingLineDelimited
)

// ReadMessage reads a single JSON message from stdio and detects framing mode.
// It supports:
// 1) Content-Length framed payloads
// 2) Line-delimited JSON-RPC payloads
func ReadMessage(r *bufio.Reader) (json.RawMessage, FramingMode, error) {
	for {
		first, err := r.ReadString('\n')
		if err != nil {
			if err == io.EOF && strings.TrimSpace(first) == "" {
				return nil, FramingContentLength, io.EOF
			}

			// Last line without trailing newline.
			if err == io.EOF && strings.TrimSpace(first) != "" {
				raw := []byte(strings.TrimSpace(first))
				if !json.Valid(raw) {
					return nil, FramingLineDelimited, fmt.Errorf("invalid JSON payload")
				}
				return json.RawMessage(raw), FramingLineDelimited, nil
			}

			return nil, FramingContentLength, fmt.Errorf("read message: %w", err)
		}

		first = strings.TrimRight(first, "\r\n")
		if strings.TrimSpace(first) == "" {
			// Skip leading empty lines.
			continue
		}

		firstTrimmed := strings.TrimSpace(first)
		if strings.HasPrefix(firstTrimmed, "{") || strings.HasPrefix(firstTrimmed, "[") {
			raw := []byte(firstTrimmed)
			if !json.Valid(raw) {
				return nil, FramingLineDelimited, fmt.Errorf("invalid line-delimited JSON payload")
			}
			return json.RawMessage(raw), FramingLineDelimited, nil
		}

		// Content-Length path.
		length := -1
		if err := maybeParseContentLengthHeader(first, &length); err != nil {
			return nil, FramingContentLength, err
		}

		for {
			line, err := r.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					return nil, FramingContentLength, io.EOF
				}
				return nil, FramingContentLength, fmt.Errorf("read header: %w", err)
			}

			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				break
			}

			if err := maybeParseContentLengthHeader(line, &length); err != nil {
				return nil, FramingContentLength, err
			}
		}

		if length < 0 {
			return nil, FramingContentLength, fmt.Errorf("missing content-length header")
		}

		payload := make([]byte, length)
		if _, err := io.ReadFull(r, payload); err != nil {
			return nil, FramingContentLength, fmt.Errorf("read payload: %w", err)
		}

		payload = bytes.TrimSpace(payload)
		if len(payload) == 0 {
			return nil, FramingContentLength, fmt.Errorf("empty payload")
		}
		if !json.Valid(payload) {
			return nil, FramingContentLength, fmt.Errorf("invalid JSON payload")
		}

		return json.RawMessage(payload), FramingContentLength, nil
	}
}

func maybeParseContentLengthHeader(line string, length *int) error {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return nil
	}
	key := strings.ToLower(strings.TrimSpace(parts[0]))
	val := strings.TrimSpace(parts[1])

	if key == "content-length" {
		n, err := strconv.Atoi(val)
		if err != nil || n < 0 {
			return fmt.Errorf("invalid content-length: %q", val)
		}
		*length = n
	}
	return nil
}

// WriteMessage writes a single JSON message using the specified framing mode.
func WriteMessage(w io.Writer, message any, framing FramingMode) error {
	data, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}

	if framing == FramingLineDelimited {
		if _, err := w.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("write payload: %w", err)
		}
		return nil
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(data))
	if _, err := io.WriteString(w, header); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("write payload: %w", err)
	}
	return nil
}
