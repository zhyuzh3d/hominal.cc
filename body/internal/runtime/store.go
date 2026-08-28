package runtime

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Store struct {
	root        string
	statePath   string
	heartbeat   string
	journalPath string
	selfRoot    string
	usageLedger string
}

func NewStore(root string) (*Store, error) {
	if root == "" {
		return nil, errors.New("instance root is required")
	}
	for _, name := range []string{"state", "journal", "life", "life/self", "logs"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			return nil, err
		}
	}
	usageLedger := strings.TrimSpace(os.Getenv("HOMINAL_RESOURCE_LEDGER"))
	if usageLedger == "" {
		usageLedger = filepath.Join(root, "state", "cognitive-usage.jsonl")
	}
	if err := os.MkdirAll(filepath.Dir(usageLedger), 0o755); err != nil {
		return nil, err
	}
	return &Store{
		root:        root,
		statePath:   filepath.Join(root, "state", "current.json"),
		heartbeat:   filepath.Join(root, "state", "heartbeat.json"),
		journalPath: filepath.Join(root, "journal", "events.jsonl"),
		selfRoot:    filepath.Join(root, "life", "self"),
		usageLedger: usageLedger,
	}, nil
}

func (s *Store) AppendUsage(record UsageRecord) error {
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(s.usageLedger, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o640)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

func (s *Store) LoadUsage(since time.Time) ([]UsageRecord, error) {
	file, err := os.Open(s.usageLedger)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	latest := make(map[string]UsageRecord)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var record UsageRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return nil, fmt.Errorf("decode cognitive usage ledger: %w", err)
		}
		at, err := time.Parse(time.RFC3339Nano, record.Time)
		if err != nil || at.Before(since) || record.LeaseID == "" {
			continue
		}
		latest[record.LeaseID] = record
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	records := make([]UsageRecord, 0, len(latest))
	for _, record := range latest {
		records = append(records, record)
	}
	sort.Slice(records, func(left, right int) bool { return records[left].Time < records[right].Time })
	return records, nil
}

func (s *Store) LoadSelf() (SelfState, error) {
	var self SelfState
	methodsData, methodsErr := os.ReadFile(filepath.Join(s.selfRoot, "methods.md"))
	if methodsErr != nil && !errors.Is(methodsErr, os.ErrNotExist) {
		return self, methodsErr
	}
	if methodsErr == nil {
		for _, line := range strings.Split(string(methodsData), "\n") {
			line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-"))
			if line != "" && len(self.Methods) < maxSelfMethods {
				self.Methods = append(self.Methods, truncate(line, maxSelfMethodBytes))
			}
		}
	}
	narrativeData, narrativeErr := os.ReadFile(filepath.Join(s.selfRoot, "narrative.md"))
	if narrativeErr != nil && !errors.Is(narrativeErr, os.ErrNotExist) {
		return self, narrativeErr
	}
	if narrativeErr == nil {
		self.Narrative = strings.TrimSpace(truncate(string(narrativeData), maxSelfNarrativeBytes))
	}
	return self, nil
}

func (s *Store) SaveSelf(self SelfState) error {
	methodLines := make([]string, 0, len(self.Methods))
	for _, method := range self.Methods {
		method = strings.TrimSpace(truncate(method, maxSelfMethodBytes))
		if method != "" {
			methodLines = append(methodLines, "- "+method)
		}
	}
	methods := strings.Join(methodLines, "\n")
	if methods != "" {
		methods += "\n"
	}
	if err := atomicWrite(filepath.Join(s.selfRoot, "methods.md"), []byte(methods), 0o644); err != nil {
		return err
	}
	narrative := strings.TrimSpace(truncate(self.Narrative, maxSelfNarrativeBytes))
	if narrative != "" {
		narrative += "\n"
	}
	return atomicWrite(filepath.Join(s.selfRoot, "narrative.md"), []byte(narrative), 0o644)
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".hominal-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
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

func (s *Store) FirstJournalTime(kinds ...string) (time.Time, bool, error) {
	wanted := make(map[string]bool, len(kinds))
	for _, kind := range kinds {
		wanted[kind] = true
	}
	file, err := os.Open(s.journalPath)
	if errors.Is(err, os.ErrNotExist) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		var record JournalRecord
		if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
			return time.Time{}, false, fmt.Errorf("decode journal time: %w", err)
		}
		if !wanted[record.Kind] {
			continue
		}
		at, err := time.Parse(time.RFC3339Nano, record.Time)
		if err != nil {
			return time.Time{}, false, fmt.Errorf("decode journal timestamp: %w", err)
		}
		return at, true, nil
	}
	if err := scanner.Err(); err != nil {
		return time.Time{}, false, err
	}
	return time.Time{}, false, nil
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
