package main

import (
	"bytes"
	"reflect"
	"testing"
)

func TestLogTeeWritesAndCapturesCompleteLines(t *testing.T) {
	var output bytes.Buffer
	var got []struct{ level, message string }
	tee := &logTee{
		output: &output,
		addLog: func(level, message string) {
			got = append(got, struct{ level, message string }{level, message})
		},
	}
	if _, err := tee.Write([]byte("warning: partial")); err != nil {
		t.Fatal(err)
	}
	if _, err := tee.Write([]byte(" warning\nlauncher failed\nready\n")); err != nil {
		t.Fatal(err)
	}

	want := []struct{ level, message string }{
		{"WARN", "warning: partial warning"},
		{"ERROR", "launcher failed"},
		{"INFO", "ready"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("captured log lines = %#v, want %#v", got, want)
	}
	if output.String() != "warning: partial warning\nlauncher failed\nready\n" {
		t.Fatalf("writer output = %q", output.String())
	}
}
