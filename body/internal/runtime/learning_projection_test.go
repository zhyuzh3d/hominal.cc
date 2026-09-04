package runtime

import (
	"strings"
	"testing"
)

func TestLearningProjectionPreservesIndependentValidMaterial(t *testing.T) {
	memory := MemoryUpdate{Content: "读到了一段可继续理解的具体材料", Origin: "observed", SourceRefs: []string{"reading"}}
	oldEvidence := ExperienceUpdate{Judgment: "旧的观察仍可作为有条件的判断材料", Evidence: []string{"memory-old"}}
	newEvidence := ExperienceUpdate{Judgment: "新的观察支持这个局部判断", Evidence: []string{"new:1"}}
	badEvidence := ExperienceUpdate{Judgment: "事件编号不等于记忆编号", Evidence: []string{"reading"}}
	badMemory := MemoryUpdate{Content: "来源尚未取得", Origin: "observed", SourceRefs: []string{"unavailable"}}
	for _, tc := range []struct {
		name            string
		memories        []MemoryUpdate
		experiences     []ExperienceUpdate
		query           string
		wantMemories    int
		wantExperiences int
		wantQuery       bool
		wantFeedback    bool
		wantNewEvidence bool
	}{
		{"bad_experience_keeps_memory_and_recall", []MemoryUpdate{memory}, []ExperienceUpdate{badEvidence}, "memory-old", 1, 0, true, true, false},
		{"one_bad_experience_keeps_other", []MemoryUpdate{memory}, []ExperienceUpdate{badEvidence, oldEvidence}, "memory-old", 1, 1, true, true, false},
		// Dropping/reindexing just the first Memory would bind new:0 to a
		// different observation. Preserve the block's original meaning instead.
		{"bad_memory_keeps_independent_old_evidence", []MemoryUpdate{badMemory, memory}, []ExperienceUpdate{{Judgment: "必须绑定原第一条", Evidence: []string{"new:0"}}, oldEvidence}, "memory-old", 0, 1, true, true, false},
		{"valid_new_binding_unchanged", []MemoryUpdate{memory, {Content: "第二条观察", Origin: "observed"}}, []ExperienceUpdate{newEvidence}, "memory-old", 2, 1, true, false, true},
		{"long_recall_keeps_learning", []MemoryUpdate{memory}, []ExperienceUpdate{oldEvidence}, strings.Repeat("长", 161), 1, 1, false, true, false},
		{"experience_cap_keeps_memory", []MemoryUpdate{memory}, []ExperienceUpdate{oldEvidence, oldEvidence, oldEvidence}, "memory-old", 1, 0, true, true, false},
		{"memory_cap_keeps_old_evidence", []MemoryUpdate{memory, memory, memory, memory}, []ExperienceUpdate{oldEvidence}, "memory-old", 0, 1, true, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			r, err := New(root, tc.name, testConfig(10), nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := r.commitLearning(learningBatch{Memories: []Memory{{ID: "memory-old", Meaning: "已经形成的旧观察", Origin: "observed"}}}); err != nil {
				t.Fatal(err)
			}
			event := Event{ID: "reading", Kind: "perceptual_change", Source: "observed", Summary: "一段实际读到的材料", Status: "in_focus"}
			r.state.Background = []Event{event}
			r.activeCandidates = map[string]Event{event.ID: event}
			commit := CognitiveCommit{
				FocusID: event.ID, ThoughtThread: "理解这次阅读，并保留适用的材料。",
				Appraisals: []CandidateAppraisal{{CandidateID: event.ID, Meaning: "这段阅读已经形成理解", Difference: 0.2, Ownership: 0.8, Value: 0.6, Urgency: 0.2, Answerability: 0.8, Certainty: 0.8, Resolution: "resolved"}},
				Action:     CognitiveAction{Kind: "none"}, ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
				MemoryUpdates: tc.memories, ExperienceUpdates: tc.experiences, RecallQuery: tc.query,
			}
			// This remains a strict validator, not a best-effort parser or an
			// automatic event-to-memory evidence converter.
			if invalid := r.validateLearningUpdates(commit) != nil; invalid != tc.wantFeedback {
				t.Fatalf("strict validation changed: invalid=%v", invalid)
			}
			if err := r.applyCognitiveCommit(commit); err != nil {
				t.Fatal(err)
			}
			if got := r.state.LearningFeedback != ""; got != tc.wantFeedback {
				t.Fatalf("rejected projection feedback missing or spurious: %q", r.state.LearningFeedback)
			}
			if err := r.persist(); err != nil {
				t.Fatal(err)
			}
			reloaded, err := New(root, tc.name, testConfig(10), nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(reloaded.learning.memories) != 1+tc.wantMemories || len(reloaded.learning.experiences) != tc.wantExperiences {
				t.Fatalf("independent learning lost or invalid learning persisted: memories=%#v experiences=%#v", reloaded.learning.memories, reloaded.learning.experiences)
			}
			for _, e := range reloaded.learning.experiences {
				if tc.wantNewEvidence {
					if len(e.Evidence) != 1 || reloaded.learning.memories[e.Evidence[0]].Meaning != "第二条观察" {
						t.Fatalf("new:1 rebound to different material: %#v", e)
					}
				} else if len(e.Evidence) != 1 || e.Evidence[0] != "memory-old" {
					t.Fatalf("unavailable evidence was invented or rebound: %#v", e)
				}
			}
			queryFound := false
			for _, e := range reloaded.state.Background {
				queryFound = queryFound || (e.Kind == "memory_recall" && e.Summary == tc.query)
			}
			if queryFound != tc.wantQuery {
				t.Fatalf("independent recall intent lost or invalid query accepted: %v", queryFound)
			}
		})
	}
}
