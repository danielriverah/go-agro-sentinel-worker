package storage

import (
	"os"
	"testing"
)

func TestJobDir(t *testing.T) {
	base := t.TempDir()
	jd := New(base, "test-job-123")

	if err := jd.Create(); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	for _, dir := range []string{jd.Input(), jd.Work(), jd.Output()} {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("directory %s should exist", dir)
		}
	}

	if err := jd.Cleanup(); err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}

	if _, err := os.Stat(jd.Input()); !os.IsNotExist(err) {
		t.Error("job directory should be removed after cleanup")
	}
}
