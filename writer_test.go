package slog

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestWriterUsesRestrictedFilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are platform-specific on Windows")
	}

	path := filepath.Join(t.TempDir(), "logs", "app.log")
	w := NewWriter(path).SetCompress(false)
	if _, err := w.Write([]byte("secret\n")); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat log dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != logDirPerm {
		t.Fatalf("log dir perm = %#o, want %#o", got, os.FileMode(logDirPerm))
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat log file: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != logFilePerm {
		t.Fatalf("log file perm = %#o, want %#o", got, os.FileMode(logFilePerm))
	}
}

func TestWriterRotatesCompressesAndFlushesMaintenanceOnClose(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	w := NewWriter(path).SetMaxSize(1).SetMaxBackups(2).SetMaxAge(0).SetCompress(true)
	payload := bytes.Repeat([]byte("a"), 1024*1024)

	if n, err := w.Write(payload); err != nil || n != len(payload) {
		t.Fatalf("first Write() = %d, %v", n, err)
	}
	if n, err := w.Write([]byte("next")); err != nil || n != 4 {
		t.Fatalf("rotating Write() = %d, %v", n, err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read current log: %v", err)
	}
	if string(current) != "next" {
		t.Fatalf("current log = %q, want next", current)
	}
	compressed, err := filepath.Glob(filepath.Join(dir, "app-*.log.gz"))
	if err != nil {
		t.Fatalf("glob compressed backups: %v", err)
	}
	if len(compressed) != 1 {
		t.Fatalf("compressed backups = %v, want one", compressed)
	}
	if got := readGzipFile(t, compressed[0]); !bytes.Equal(got, payload) {
		t.Fatalf("compressed backup length = %d, want %d", len(got), len(payload))
	}
}

func TestWriterEnforcesBackupRetention(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	w := NewWriter(path).SetMaxSize(1).SetMaxBackups(1).SetMaxAge(0).SetCompress(false)
	full := bytes.Repeat([]byte("x"), 1024*1024)

	if _, err := w.Write(full); err != nil {
		t.Fatalf("Write(first file) error = %v", err)
	}
	if _, err := w.Write([]byte("1")); err != nil {
		t.Fatalf("Write(first rotation) error = %v", err)
	}
	if _, err := w.Write(full[:len(full)-1]); err != nil {
		t.Fatalf("Write(second file) error = %v", err)
	}
	if _, err := w.Write([]byte("2")); err != nil {
		t.Fatalf("Write(second rotation) error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	backups, err := filepath.Glob(filepath.Join(dir, "app-*.log"))
	if err != nil {
		t.Fatalf("glob backups: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("backups = %v, want newest backup only", backups)
	}
}

func TestWriterRemovesExpiredBackupsDuringRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	expired := filepath.Join(dir, "app-"+time.Now().Add(-48*time.Hour).Format(backupTimeFormat)+".000000000.log")
	if err := os.WriteFile(expired, []byte("expired"), logFilePerm); err != nil {
		t.Fatalf("write expired backup: %v", err)
	}

	w := NewWriter(path).SetMaxSize(1).SetMaxBackups(0).SetMaxAge(1).SetCompress(false)
	if _, err := w.Write(bytes.Repeat([]byte("x"), 1024*1024)); err != nil {
		t.Fatalf("Write(full file) error = %v", err)
	}
	if _, err := w.Write([]byte("rotate")); err != nil {
		t.Fatalf("Write(rotation) error = %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := os.Stat(expired); !os.IsNotExist(err) {
		t.Fatalf("expired backup still exists, stat error = %v", err)
	}
}

func TestWriterStripsANSIAndReportsOriginalWriteLength(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	w := NewWriter(path).SetCompress(false)
	input := []byte("\x1b[31merror\x1b[0m plain")
	if n, err := w.Write(input); err != nil || n != len(input) {
		t.Fatalf("Write() = %d, %v, want %d, nil", n, err, len(input))
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if string(got) != "error plain" {
		t.Fatalf("log contents = %q, want ANSI-free text", got)
	}
}

func TestWriterRejectsInvalidLimitsAndOversizedRecords(t *testing.T) {
	tests := []struct {
		name      string
		configure func(*writer)
	}{
		{name: "negative size", configure: func(w *writer) { w.SetMaxSize(-1) }},
		{name: "negative age", configure: func(w *writer) { w.SetMaxAge(-1) }},
		{name: "negative backups", configure: func(w *writer) { w.SetMaxBackups(-1) }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			w := NewWriter(filepath.Join(t.TempDir(), "app.log"))
			test.configure(w)
			if _, err := w.Write([]byte("record")); err == nil {
				t.Fatal("Write() error = nil")
			}
		})
	}

	path := filepath.Join(t.TempDir(), "oversized.log")
	w := NewWriter(path).SetMaxSize(1)
	if _, err := w.Write(bytes.Repeat([]byte("x"), 1024*1024+1)); err == nil || !strings.Contains(err.Error(), "exceeds maximum") {
		t.Fatalf("oversized Write() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("oversized write created a file, stat error = %v", err)
	}
}

func readGzipFile(t *testing.T, path string) []byte {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open gzip file: %v", err)
	}
	defer file.Close()

	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("open gzip stream: %v", err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read gzip stream: %v", err)
	}
	return data
}
