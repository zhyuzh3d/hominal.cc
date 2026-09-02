package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"

	"hominal.cc/hominal/body/internal/organ"
)

func TestPerformDistinguishesCompletedAndFailed(t *testing.T) {
	encode := func(actionID, source string) string {
		value, err := json.Marshal(organ.ActionRequest{
			Schema: organ.ActionSchema, ActionID: actionID, Operation: "exec", Input: source, TimeoutMilliseconds: 1_000,
		})
		if err != nil {
			t.Fatal(err)
		}
		return string(value)
	}
	completed := perform("system", encode("action-ok", "printf 'fact'"))
	if completed.Status != "completed" || completed.ActionID != "action-ok" || completed.OrganID != "system" {
		t.Fatalf("unexpected completed result: %#v", completed)
	}
	failed := perform("system", encode("action-failed", "printf 'problem' >&2; exit 7"))
	if failed.Status != "failed" || failed.ActionID != "action-failed" {
		t.Fatalf("unexpected failed result: %#v", failed)
	}
	var output struct {
		ExitCode int    `json:"exit_code"`
		Stderr   string `json:"stderr"`
	}
	if err := json.Unmarshal([]byte(failed.Output), &output); err != nil {
		t.Fatal(err)
	}
	if output.ExitCode != 7 || output.Stderr != "problem" {
		t.Fatalf("system organ hid the failure facts: %#v", output)
	}
}

func TestExecuteTimeoutStopsDescendantProcesses(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "child.pid")
	command := fmt.Sprintf("sleep 30 & child=$!; printf '%%s' \"$child\" > %s; wait", strconv.Quote(pidPath))
	status, resultText := execute(command, 2*time.Second)
	var result struct {
		TimedOut bool `json:"timed_out"`
	}
	if err := json.Unmarshal([]byte(resultText), &result); err != nil {
		t.Fatal(err)
	}
	if status != "unknown" || !result.TimedOut {
		t.Fatalf("system action did not report its deadline: status=%s result=%s", status, resultText)
	}
	rawPID, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatalf("%v: %s", err, resultText)
	}
	pid, err := strconv.Atoi(string(rawPID))
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		err = syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("descendant process %d survived the system organ deadline", pid)
}
