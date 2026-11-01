package protocol

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestParseAndFormatCQCode(t *testing.T) {
	input := `hello [CQ:at,qq=wxid_1] [CQ:image,file=https://example.test/a&#44;b.png]`
	segments := ParseCQCode(input)
	if len(segments) != 4 {
		t.Fatalf("got %d segments: %#v", len(segments), segments)
	}
	if segments[0].Type != "text" || segments[0].Data["text"] != "hello " {
		t.Fatalf("unexpected leading text: %#v", segments[0])
	}
	if segments[1].Type != "at" || segments[1].Data["qq"] != "wxid_1" {
		t.Fatalf("unexpected at segment: %#v", segments[1])
	}
	if got := FormatCQCode(segments); got != input {
		t.Fatalf("CQ roundtrip got %q, want %q", got, input)
	}
}

func TestParseMessageAndSegmentSerialization(t *testing.T) {
	segments, err := ParseMessage(json.RawMessage(`[ {"type":"text","data":{"text":"hello"}}, {"type":"image","data":{"file":"http://example.test/x.png"}} ]`))
	if err != nil {
		t.Fatal(err)
	}
	want := []Segment{
		{Type: "text", Data: map[string]any{"text": "hello"}},
		{Type: "image", Data: map[string]any{"file": "http://example.test/x.png"}},
	}
	if !reflect.DeepEqual(segments, want) {
		t.Fatalf("segments = %#v, want %#v", segments, want)
	}
	encoded, err := json.Marshal(segments)
	if err != nil {
		t.Fatal(err)
	}
	var decoded []Segment
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("serialized segments = %#v, want %#v", decoded, want)
	}
}

func TestOneBotResponseEnvelope(t *testing.T) {
	echo := json.RawMessage(`"request-1"`)
	response := NewResponse(1404, map[string]any{}, "unknown action", echo)
	encoded, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"status", "retcode", "data", "message", "wording", "echo"} {
		if len(fields[field]) == 0 {
			t.Errorf("response omitted %s", field)
		}
	}
	if response.Status != "failed" || response.Retcode != 1404 || string(fields["echo"]) != `"request-1"` {
		t.Fatalf("unexpected response: %s", encoded)
	}
}
