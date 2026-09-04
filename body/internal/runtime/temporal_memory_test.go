package runtime

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRecallKeepsHistoricalStateWithLaterRelatedExperience(t *testing.T) {
	for _, laterOrigin := range []string{"observed", "reflected", ""} {
		t.Run("later_"+laterOrigin, func(t *testing.T) {
			index := newLearningIndex()
			old := Memory{ID: "memory-old-object", Meaning: "paper-42 has been sent; currently waiting for a concrete response", Keywords: []string{"paper-42", "waiting"}, Origin: "observed", ObservedAt: "2026-09-04T01:00:00Z", SourceRefs: []string{"sent"}}
			index.apply(learningBatch{Memories: []Memory{old}})
			for i := 0; i < 8; i++ {
				index.apply(learningBatch{Memories: []Memory{{ID: fmt.Sprintf("similar-%d", i), Meaning: "has been sent; currently waiting for a concrete response", ObservedAt: "2026-09-04T01:01:00Z", SourceRefs: []string{fmt.Sprintf("old-event-%d", i)}}}})
			}
			if laterOrigin != "" {
				index.apply(learningBatch{Memories: []Memory{{ID: "later", Meaning: "paper-42: the reviewer discussed the layout and proposed a different interpretation", Keywords: []string{"paper-42", "layout"}, Origin: laterOrigin, ObservedAt: "2026-09-04T01:05:00Z", SourceRefs: []string{"review"}}}})
			}
			// Newness alone does not make an unrelated event part of this history.
			index.apply(learningBatch{Memories: []Memory{{ID: "unrelated", Meaning: "rain stopped in another city", Origin: "observed", ObservedAt: "2026-09-04T01:06:00Z", SourceRefs: []string{"weather"}}}})
			found := index.recall(old.Meaning, "fixed")
			seen := map[string]Memory{}
			for _, m := range found.Memories {
				seen[m.ID] = m
			}
			if seen[old.ID].Meaning != old.Meaning {
				t.Fatal("retrieval erased what was true or believed at the earlier time")
			}
			if laterOrigin != "" && seen["later"].Origin != laterOrigin {
				t.Fatalf("later related material or its epistemic origin was lost: %#v", found)
			}
			if _, exists := seen["unrelated"]; exists {
				t.Fatal("unrelated recency displaced relevant history")
			}
			if laterOrigin == "" && len(seen) == 0 {
				t.Fatal("genuine unresolved history disappeared")
			}
			if len(found.Memories) > 5 || len(found.Memories)+len(found.Experiences) > 6 {
				t.Fatal("temporal context expanded the recall budget")
			}
			if index.memories[old.ID].Meaning != old.Meaning || index.memories[old.ID].Corrects != "" {
				t.Fatal("retrieval silently rewrote personal memory")
			}
		})
	}
}

func TestRecallKeepsCausalFollowupWithoutSharedWording(t *testing.T) {
	index := newLearningIndex()
	old := Memory{ID: "old-action-state", CommitmentID: "action-42", Meaning: "等待图片写入", ObservedAt: "2026-09-04T01:00:00Z", Origin: "predicted", SourceRefs: []string{"request"}}
	newer := Memory{ID: "new-action-state", CommitmentID: "action-42", Meaning: "exit code 0; /life/art.png; 1080 x 1080", ObservedAt: "2026-09-04T01:05:00Z", Origin: "observed", SourceRefs: []string{"result"}}
	index.apply(learningBatch{Memories: []Memory{old, newer}})
	found := index.recall(old.ID, "fixed")
	if len(found.Memories) != 2 || found.Memories[0].ID != old.ID || found.Memories[1].ID != newer.ID {
		t.Fatalf("exact action continuity depended on lexical resemblance: %#v", found)
	}
}

// Optional offline replay of the frozen eighth sample, never a live API call.
// Keep the private full archive outside the repository; normal unit coverage
// above uses small synthetic paired cases.
func TestStage103ArchivedRecallTimeline(t *testing.T) {
	path := os.Getenv("HOMINAL_RECALL_REPLAY_ARCHIVE")
	if path == "" {
		t.Skip("set HOMINAL_RECALL_REPLAY_ARCHIVE for frozen sample replay")
	}
	index, original := archivedRecallAt(t, path, 874)
	found := index.recall(original.Query, "fixed-replay")
	old, later := false, false
	for _, m := range found.Memories {
		t.Log(m.ID, m.ObservedAt, m.Origin)
		old = old || m.ID == "memory-lease-a904067f52fc5248-0"
		// Independently inspected records retaining content-specific feedback.
		later = later || m.ID == "memory-event-000000000778" || m.ID == "memory-lease-c7201a826ad1fda9-0" || m.ID == "memory-event-000000000826" || m.ID == "memory-lease-090b122b97fd9ffe-0"
	}
	if !old || !later {
		t.Fatalf("frozen recall still lacks earlier waiting alongside already absorbed feedback: old=%v later=%v", old, later)
	}
}

// Rebuild only what had been committed when the selected recall occurred.
// Later records cannot leak into a frozen causal replay.
func archivedRecallAt(t *testing.T, path string, targetSeq int) (*learningIndex, RecallBundle) {
	t.Helper()
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
	tarReader := tar.NewReader(gz)
	for {
		h, err := tarReader.Next()
		if err == io.EOF {
			t.Fatal("journal not found")
		}
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(h.Name, "/journal/events.jsonl") {
			continue
		}
		index := newLearningIndex()
		scanner := bufio.NewScanner(tarReader)
		scanner.Buffer(make([]byte, 65536), 8*1024*1024)
		for scanner.Scan() {
			var row struct {
				Seq     int             `json:"seq"`
				Kind    string          `json:"kind"`
				Payload json.RawMessage `json:"payload"`
			}
			if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
				t.Fatal(err)
			}
			if row.Kind == "learning_committed" {
				var batch learningBatch
				if err := json.Unmarshal(row.Payload, &batch); err != nil {
					t.Fatal(err)
				}
				index.apply(batch)
			}
			if row.Seq != targetSeq || row.Kind != "memory_recalled" {
				continue
			}
			var original RecallBundle
			if err := json.Unmarshal(row.Payload, &original); err != nil {
				t.Fatal(err)
			}
			return index, original
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		t.Fatal("target recall not found")
	}
}

func TestValueSignalQuotesMemoryWithItsOriginalTimeAndOrigin(t *testing.T) {
	r, err := New(t.TempDir(), "dated-association", testConfig(10), nil)
	if err != nil {
		t.Fatal(err)
	}
	r.state.ValueField.Activation = LifeValueVector{Relatedness: 0.95}
	r.state.LastAttentionAt = time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	m := Memory{ID: "old-view", Meaning: "现在页面还没有正文", Origin: "observed", ObservedAt: "2026-09-04T01:00:00Z", ActionKind: "organ_action", EnactedRequest: `{"organ_id":"browser"}`}
	r.state.Memories = []Memory{m}
	if emitted, err := r.maybeEmitLifeValueSignal(); err != nil || !emitted {
		t.Fatalf("signal: %v %v", emitted, err)
	}
	e := r.state.Background[len(r.state.Background)-1]
	var payload map[string]any
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["context_observed_at"] != m.ObservedAt || payload["context_origin"] != m.Origin || !strings.Contains(e.Summary, m.ObservedAt) {
		t.Fatalf("historical association was presented without its original temporal/source position: %s %s", e.Summary, e.Payload)
	}
}
