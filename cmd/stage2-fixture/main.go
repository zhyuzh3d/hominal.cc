package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

type record struct {
	Type       string `json:"type"`
	Time       string `json:"time"`
	InstanceID string `json:"instance_id"`
	PID        int    `json:"pid"`
}

func main() {
	instanceRoot := os.Getenv("HOMINAL_INSTANCE_ROOT")
	instanceID := os.Getenv("HOMINAL_INSTANCE_ID")
	if instanceRoot == "" || instanceID == "" {
		fatal(errors.New("HOMINAL_INSTANCE_ROOT and HOMINAL_INSTANCE_ID are required"))
	}

	for _, name := range []string{"state", "journal", "life", "logs"} {
		if err := os.MkdirAll(filepath.Join(instanceRoot, name), 0o755); err != nil {
			fatal(err)
		}
	}

	journalPath := filepath.Join(instanceRoot, "journal", "events.jsonl")
	journal, err := os.OpenFile(journalPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fatal(err)
	}
	defer journal.Close()
	writer := bufio.NewWriter(journal)
	defer writer.Flush()

	writeEvent(writer, record{Type: "ready", Time: now(), InstanceID: instanceID, PID: os.Getpid()})
	if err := writeState(instanceRoot, instanceID); err != nil {
		fatal(err)
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case sig := <-signals:
			writeEvent(writer, record{Type: "stopped:" + sig.String(), Time: now(), InstanceID: instanceID, PID: os.Getpid()})
			return
		case <-ticker.C:
			if err := writeState(instanceRoot, instanceID); err != nil {
				fatal(err)
			}
			crashRequest := filepath.Join(instanceRoot, "state", "crash-request")
			if _, err := os.Stat(crashRequest); err == nil {
				_ = os.Remove(crashRequest)
				writeEvent(writer, record{Type: "test-crash", Time: now(), InstanceID: instanceID, PID: os.Getpid()})
				_ = writer.Flush()
				os.Exit(42)
			} else if !errors.Is(err, os.ErrNotExist) {
				fatal(err)
			}
		}
	}
}

func writeState(instanceRoot, instanceID string) error {
	state := record{Type: "heartbeat", Time: now(), InstanceID: instanceID, PID: os.Getpid()}
	contents, err := json.Marshal(state)
	if err != nil {
		return err
	}
	path := filepath.Join(instanceRoot, "state", "heartbeat.json")
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, append(contents, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func writeEvent(writer *bufio.Writer, event record) {
	contents, err := json.Marshal(event)
	if err != nil {
		fatal(err)
	}
	if _, err := writer.Write(append(contents, '\n')); err != nil {
		fatal(err)
	}
	if err := writer.Flush(); err != nil {
		fatal(err)
	}
}

func now() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
