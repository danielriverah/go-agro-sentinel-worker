package gdal

import (
	"context"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestExecutorRun(t *testing.T) {
	if _, err := exec.LookPath("gdalinfo"); err != nil {
		t.Skip("gdalinfo not found in PATH")
	}
	e := NewExecutor(300)
	stdout, stderr, err := e.Run(context.Background(), "gdalinfo", []string{"--version"})
	if err != nil {
		t.Fatalf("gdalinfo --version failed: %v (stderr: %s)", err, stderr)
	}
	if !strings.Contains(stdout, "GDAL") {
		t.Errorf("expected GDAL in output, got: %s", stdout)
	}
}

func TestExecutorRunNonZeroExit(t *testing.T) {
	if _, err := exec.LookPath("gdalinfo"); err != nil {
		t.Skip("gdalinfo not found in PATH")
	}
	e := NewExecutor(300)
	_, _, err := e.Run(context.Background(), "gdalinfo", []string{"/nonexistent/path/does-not-exist.tif"})
	if err == nil {
		t.Fatal("expected error for nonexistent input file")
	}
}

func TestExecutorRunTimeout(t *testing.T) {
	sleepCmd, sleepArgs := sleepCommand(2)
	if _, err := exec.LookPath(sleepCmd); err != nil {
		t.Skipf("%s not found in PATH", sleepCmd)
	}

	e := NewExecutor(1) // 1 second default timeout
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	_, _, err := e.Run(ctx, sleepCmd, sleepArgs)
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestExecutorRunDefaultTimeoutAppliedWithoutDeadline(t *testing.T) {
	sleepCmd, sleepArgs := sleepCommand(5)
	if _, err := exec.LookPath(sleepCmd); err != nil {
		t.Skipf("%s not found in PATH", sleepCmd)
	}

	e := NewExecutor(1) // 1 second default timeout, no deadline on ctx
	start := time.Now()
	_, _, err := e.Run(context.Background(), sleepCmd, sleepArgs)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("expected timeout error from default timeout")
	}
	if elapsed > 4*time.Second {
		t.Errorf("expected Run to be cut off near 1s default timeout, took %v", elapsed)
	}
}

// sleepCommand returns a platform-appropriate command+args that sleeps for
// roughly n seconds.
func sleepCommand(n int) (string, []string) {
	if runtime.GOOS == "windows" {
		return "ping", []string{"-n", strconv.Itoa(n + 1), "127.0.0.1"}
	}
	return "sleep", []string{strconv.Itoa(n)}
}
