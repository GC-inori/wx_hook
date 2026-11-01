package main

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wxbot/pkg/config"
)

func TestLoggerAppliesRotationSettings(t *testing.T) {
	previousOutput, previousFlags := log.Writer(), log.Flags()
	defer func() {
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
	}()

	logDir := t.TempDir()
	writer, err := initLogger(logDir, config.LoggingConfig{MaxSizeMB: 1, MaxBackups: 1, Compress: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	payload := strings.Repeat("x", 700*1024)
	for i := 0; i < 4; i++ {
		if n, err := io.WriteString(writer, payload); err != nil || n != len(payload) {
			t.Fatalf("write log payload: n=%d err=%v", n, err)
		}
	}

	files, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatal(err)
	}
	compressedBackups := 0
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".gz") {
			compressedBackups++
		}
	}
	if compressedBackups != 1 {
		t.Fatalf("compressed backups = %d, want 1 (files: %v)", compressedBackups, files)
	}
	if _, err := os.Stat(filepath.Join(logDir, "gateway.log")); err != nil {
		t.Fatalf("active log file missing: %v", err)
	}
}
