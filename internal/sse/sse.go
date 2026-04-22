package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type EventType string

const (
	EventProgress EventType = "progress"
	EventResult   EventType = "result"
	EventError    EventType = "error"
)

type Event struct {
	Type    EventType `json:"type"`
	Message string    `json:"message,omitempty"`
	Data    any       `json:"data,omitempty"`
}

// Writer wraps ResponseWriter for SSE streaming
type Writer struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// New sets SSE headers and returns a Writer. Returns false if streaming not supported.
func New(w http.ResponseWriter) (*Writer, bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, false
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	return &Writer{w: w, flusher: flusher}, true
}

// Send sends one SSE event and flushes immediately.
func (sw *Writer) Send(event Event) error {
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(sw.w, "data: %s\n\n", b)
	sw.flusher.Flush()
	return err
}

func (sw *Writer) Progress(message string) error {
	return sw.Send(Event{Type: EventProgress, Message: message})
}

func (sw *Writer) Result(data any) error {
	return sw.Send(Event{Type: EventResult, Data: data})
}

func (sw *Writer) Error(message string) error {
	return sw.Send(Event{Type: EventError, Message: message})
}

// Stream reads from ch and writes SSE events until ch is closed or ctx is done.
func Stream(ctx context.Context, sw *Writer, ch <-chan Event) {
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			_ = sw.Send(event)
			if event.Type == EventResult || event.Type == EventError {
				return
			}
		case <-ctx.Done():
			return
		}
	}
}
