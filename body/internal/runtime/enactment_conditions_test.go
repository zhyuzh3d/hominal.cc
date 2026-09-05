package runtime

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func TestOrientationChangesReadConditionsWithoutClaimingWorldProgress(t *testing.T) {
	for _, effect := range []string{"observed", "oriented", "changed", "unknown", ""} {
		t.Run(effect, func(t *testing.T) {
			r, err := New(t.TempDir(), "conditions", testConfig(10), nil)
			if err != nil {
				t.Fatal(err)
			}
			r.state.Commitments = []ActionCommitment{{ID: "read-old", ConcernID: "old", ActionKind: "organ_action", Status: "assimilated"}}
			r.state.Memories = []Memory{{CommitmentID: "read-old", RemainingDifference: 0.05}}
			old, _ := json.Marshal(ActionState{CommitmentID: "read-old", Kind: "organ_action", OrganID: "camera", Operation: "read", Request: `{}`, Effect: "observed", Status: "completed"})
			later, _ := json.Marshal(ActionState{Kind: "organ_action", OrganID: "camera", Operation: "point", Request: `{"position":"other"}`, Effect: effect, Status: "completed"})
			r.state.Background = []Event{{Seq: 1, Kind: "action_result", Payload: old}, {Seq: 2, Kind: "action_result", Payload: later}}
			err = r.validateActionProgress("new", CognitiveAction{Kind: "organ_action", OrganID: "camera", Operation: "read", Input: `{}`})
			if (err == nil) != (effect != "observed") {
				t.Fatalf("effect %q misclassified changed/unknown reading conditions: %v", effect, err)
			}
			if actionEffectIsContactOnly(effect) != (effect != "changed") {
				t.Fatal("permission to read was confused with verified world progress")
			}
		})
	}
}

func TestPendingRealityAllowsThinkingWithoutAnotherBodyAction(t *testing.T) {
	for _, status := range []string{"acting", "reality_available", "reality_unknown", "assimilated"} {
		t.Run(status, func(t *testing.T) {
			r, err := New(t.TempDir(), "waiting", testConfig(10), nil)
			if err != nil {
				t.Fatal(err)
			}
			r.state.Concerns = []Concern{{ID: "held", OriginKind: "environment_change", Subject: "理解一个具体对象", ClosureCondition: "获得具体解释", Meaning: "继续理解", Ownership: 0.9, Difference: 0.8, Resolution: "hold"}}
			focus := Event{ID: "held", Kind: "concern", ConcernID: "held", Status: "in_focus"}
			r.activeCandidates = map[string]Event{focus.ID: focus}
			r.state.Commitments = []ActionCommitment{{ID: "other-action", ActionKind: "organ_action", Status: status}}
			if status == "acting" {
				r.state.PendingAction = &ActionState{ID: "body-action", Kind: "organ_action", Status: "running"}
			}
			commit := CognitiveCommit{FocusID: focus.ID, Appraisals: []CandidateAppraisal{{CandidateID: focus.ID, Meaning: "身体正在接触另一段材料，先保留这个判断", Difference: 0.8, Ownership: 0.9, Value: 0.8, Answerability: 0.9, Certainty: 0.9, Resolution: "hold"}}, Action: CognitiveAction{Kind: "none"}, ThoughtThread: "等待当前身体行动结果进入理解。", ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"}}
			err = r.applyCognitiveCommit(commit)
			if (err == nil) != (status != "assimilated") {
				t.Fatalf("waiting state %s: %v", status, err)
			}
			if len(r.state.Commitments) != 1 {
				t.Fatal("thinking duplicated an ongoing action")
			}
		})
	}
}

func TestStage103ArchivedOrientationReopensSnapshot(t *testing.T) {
	path := os.Getenv("HOMINAL_ORIENTATION_REPLAY_ARCHIVE")
	if path == "" {
		t.Skip("set HOMINAL_ORIENTATION_REPLAY_ARCHIVE for twelfth frozen sample")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	r, err := New(t.TempDir(), "frozen-conditions", testConfig(10), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for {
		h, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(h.Name, "/journal/events.jsonl") {
			continue
		}
		scanner := bufio.NewScanner(reader)
		scanner.Buffer(make([]byte, 65536), 8*1024*1024)
		for scanner.Scan() {
			var row struct {
				Seq     uint64          `json:"seq"`
				Kind    string          `json:"kind"`
				Payload json.RawMessage `json:"payload"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
				t.Fatal(err)
			}
			if row.Seq >= 120 {
				found = row.Seq == 120
				break
			}
			switch row.Kind {
			case "action_committed":
				var c ActionCommitment
				if err := json.Unmarshal(row.Payload, &c); err != nil {
					t.Fatal(err)
				}
				r.state.Commitments = append(r.state.Commitments, c)
			case "memory_assimilated":
				var p struct {
					Memory Memory `json:"memory"`
				}
				if err := json.Unmarshal(row.Payload, &p); err != nil {
					t.Fatal(err)
				}
				r.state.Memories = append(r.state.Memories, p.Memory)
				for i := range r.state.Commitments {
					if r.state.Commitments[i].ID == p.Memory.CommitmentID {
						r.state.Commitments[i].Status = "assimilated"
					}
				}
			default:
				var p struct {
					EventID  string          `json:"event_id"`
					Payload  json.RawMessage `json:"payload"`
					Admitted *bool           `json:"attention_admitted"`
				}
				if err := json.Unmarshal(row.Payload, &p); err != nil {
					continue
				}
				if p.EventID != "" && (p.Admitted == nil || *p.Admitted) {
					r.state.Background = append(r.state.Background, Event{ID: p.EventID, Seq: row.Seq, Kind: row.Kind, Payload: p.Payload})
				}
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		break
	}
	if !found {
		t.Fatal("frozen rejection boundary not found")
	}
	r.activeCandidates = map[string]Event{"concern-57f57c2c0e07a86f": {ID: "concern-57f57c2c0e07a86f", Kind: "concern", ConcernID: "concern-57f57c2c0e07a86f"}}
	if err := r.validateActionProgress("concern-57f57c2c0e07a86f", CognitiveAction{Kind: "organ_action", OrganID: "browser", Operation: "browser_snapshot", Input: `{}`}); err != nil {
		t.Fatalf("new page is still mistaken for old snapshot: %v", err)
	}
}
