package protocol

import (
	"bufio"
	"encoding/json"
	"fmt"
)

// ReadMessage reads a single JSON-line message from the buffered reader.
// Returns the message type and raw JSON bytes.
func ReadMessage(r *bufio.Reader) (msgType string, data []byte, err error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return "", nil, err
	}
	var base BaseMessage
	if err := json.Unmarshal(line, &base); err != nil {
		return "", nil, fmt.Errorf("invalid message: %w", err)
	}
	return base.Type, line, nil
}

// WriteMessage writes a JSON-line message to the buffered writer and flushes.
func WriteMessage(w *bufio.Writer, msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return err
	}
	if err := w.WriteByte('\n'); err != nil {
		return err
	}
	return w.Flush()
}

// DecodeMessage decodes raw JSON bytes into the target struct.
func DecodeMessage(data []byte, target any) error {
	return json.Unmarshal(data, target)
}
