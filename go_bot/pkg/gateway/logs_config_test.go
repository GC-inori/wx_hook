package gateway

import "testing"

func TestLogBufferUsesConfiguredCapacity(t *testing.T) {
	buffer := newLogBuffer(2)
	for _, message := range []string{"first", "second", "third"} {
		buffer.add(LogEntry{Msg: message})
	}
	entries := buffer.snapshot()
	if len(entries) != 2 || entries[0].Msg != "second" || entries[1].Msg != "third" {
		t.Fatalf("configured log buffer contents = %+v", entries)
	}

	buffer.setLimit(1)
	entries = buffer.snapshot()
	if len(entries) != 1 || entries[0].Msg != "third" {
		t.Fatalf("shrinking log buffer did not retain newest entry: %+v", entries)
	}
}

func TestLogBufferUsesDefaultForNonpositiveCapacity(t *testing.T) {
	buffer := newLogBuffer(0)
	if buffer.limit != maxLogEntries || cap(buffer.entries) != maxLogEntries {
		t.Fatalf("nonpositive capacity was not defaulted: limit=%d cap=%d", buffer.limit, cap(buffer.entries))
	}
}
