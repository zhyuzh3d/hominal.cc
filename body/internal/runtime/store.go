package runtime

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Store struct {
	root        string
	statePath   string
	heartbeat   string
	journalPath string
}

func NewStore(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("instance root is required")
	}
	for _, name := range []string{"state", "journal", "life", "logs"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			return nil, err
		}
	}
	return &Store{
		root:        root,
		statePath:   filepath.Join(root, "state", "current.json"),
		heartbeat:   filepath.Join(root, "state", "heartbeat.json"),
		journalPath: filepath.Join(root, "journal", "events.jsonl"),
	}, nil
}

func (s *Store) Load() (*State, error) {
	data, err := os.ReadFile(s.statePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("decode current state: %w", err)
	}
	if state.Schema != stateSchema {
		return nil, fmt.Errorf("unsupported state schema %q", state.Schema)
	}
	journalSeq, err := s.lastJournalSeq()
	if err != nil {
		return nil, err
	}
	if journalSeq > state.EventSeq {
		state.EventSeq = journalSeq
	}
	return &state, nil
}

func (s *Store) lastJournalSeq() (uint64, error) {
	file, err := os.Open(s.journalPath)
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var maximum uint64
	for scanner.Scan() {
		var record JournalRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return 0, fmt.Errorf("decode journal sequence: %w", err)
		}
		if record.Seq > maximum {
			maximum = record.Seq
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, err
	}
	return maximum, nil
}

func (s *Store) Save(state *State) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary := s.statePath + ".tmp"
	if err := os.WriteFile(temporary, data, 0o600); err != nil {
		return err
	}
	file, err := os.OpenFile(temporary, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, s.statePath); err != nil {
		return err
	}
	directory, err := os.Open(filepath.Dir(s.statePath))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func (s *Store) Heartbeat(state *State) error {
	record := map[string]any{
		"type":        "heartbeat",
		"time":        state.LastPulseAt,
		"instance_id": state.InstanceID,
		"pid":         os.Getpid(),
		"pulse_id":    state.PulseID,
		"stage":       state.Stage,
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	temporary := s.heartbeat + ".tmp"
	if err := os.WriteFile(temporary, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, s.heartbeat)
}

func (s *Store) Append(record JournalRecord) error {
	file, err := os.OpenFile(s.journalPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	writer := bufio.NewWriter(file)
	encoder := json.NewEncoder(writer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(record); err != nil {
		file.Close()
		return err
	}
	if err := writer.Flush(); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
