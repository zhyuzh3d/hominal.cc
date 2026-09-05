package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPersonalMemoryPersistsBeyondWorkingWindow(t *testing.T) {
	root := t.TempDir()
	r, err := New(root, "memory-life", testConfig(10), nil)
	if err != nil {
		t.Fatal(err)
	}
	batch := learningBatch{}
	for i := 0; i < 160; i++ {
		batch.Memories = append(batch.Memories, Memory{ID: fmt.Sprintf("m%03d", i), ObservedAt: fmt.Sprintf("2026-09-04T00:%02d:%02dZ", i/60, i%60), Meaning: fmt.Sprintf("ordinary %d", i), SourceRefs: []string{fmt.Sprintf("event%d", i)}})
	}
	batch.Memories[0].Meaning = "Carlo Gimach 建筑师的迁徙与作品"
	batch.Memories[0].Keywords = []string{"Gimach", "建筑"}
	if err := r.commitLearning(batch); err != nil {
		t.Fatal(err)
	}
	if len(r.state.Memories) != maxMemories {
		t.Fatalf("working memory was not bounded: %d", len(r.state.Memories))
	}
	if err := r.persist(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := New(root, "memory-life", testConfig(10), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := reloaded.learning.recall("Gimach 建筑", "fixed")
	if len(found.Memories) == 0 || found.Memories[0].ID != "m000" {
		t.Fatalf("old meaningful memory was lost: %#v", found)
	}
	byID := reloaded.learning.recall("m000", "fixed")
	if len(byID.Memories) != 1 || byID.Memories[0].ID != "m000" {
		t.Fatal("explicit memory ID cannot be followed")
	}
}

func TestOperationalSelfSensingCountsActionsNotPersonalFragments(t *testing.T) {
	r, err := New(t.TempDir(), "operational-memory", testConfig(10), nil)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now().UTC().Add(-time.Hour)
	r.state.T0 = base.Format(time.RFC3339Nano)
	addOutcome := func(i int) {
		id, mid := fmt.Sprintf("action-%d", i), fmt.Sprintf("outcome-%d", i)
		at := base.Add(time.Duration(i+1) * time.Minute).Format(time.RFC3339Nano)
		r.state.Commitments = append(r.state.Commitments, ActionCommitment{ID: id, MemoryID: mid, ActionKind: "organ_action", ConcernID: "topic", Status: "assimilated"})
		if err := r.commitLearning(learningBatch{Memories: []Memory{{ID: mid, CommitmentID: id, SourceKind: "action_result", ActionKind: "organ_action", EnactedRequest: `{"organ_id":"browser","operation":"browser_snapshot","input":"{}"}`, Origin: "observed", ObservedAt: at}}}); err != nil {
			t.Fatal(err)
		}
	}
	addOutcome(0)
	fragments := learningBatch{}
	for i := 0; i < 160; i++ {
		origin := []string{"observed", "predicted", "reflected"}[i%3]
		fragments.Memories = append(fragments.Memories, Memory{ID: fmt.Sprintf("fragment-%d", i), Origin: origin, SourceKind: "value_signal", Meaning: "同一次接触引起的个人理解", SourceRefs: []string{"outcome-0"}, ObservedAt: base.Add(2 * time.Minute).Format(time.RFC3339Nano), Significance: "ordinary"})
	}
	if err := r.commitLearning(fragments); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 30; i++ {
		r.state.Usage = append(r.state.Usage, UsageRecord{Time: base.Add(10 * time.Minute).Format(time.RFC3339Nano), CostConfirmed: true, ActualMicrousd: 1})
	}
	if err := r.maybeOpenOperationalSelfDifference(); err != nil {
		t.Fatal(err)
	}
	if len(r.state.Background) != 0 || r.state.SelfModelTension != 0 {
		t.Fatal("one action plus rich personal memory falsely became repeated low-yield activity")
	}
	if got := r.settledActionMemoriesAfter(r.state.T0); len(got) != 1 || got[0].ID != "outcome-0" {
		t.Fatalf("personal fragments evicted the canonical action outcome: %#v", got)
	}
	for i := 1; i < 4; i++ {
		addOutcome(i)
	}
	// Duplicate references and an in-flight action do not add completed outcomes.
	r.state.Commitments = append(r.state.Commitments, r.state.Commitments[0], ActionCommitment{ID: "still-running", MemoryID: "outcome-0", Status: "acting"})
	if err := r.maybeOpenOperationalSelfDifference(); err != nil {
		t.Fatal(err)
	}
	if len(r.state.Background) != 1 || r.state.Background[0].Kind != "self_model_difference" {
		t.Fatal("four genuine repeated action outcomes no longer reach self-sensing")
	}
	var payload struct {
		MemoryCount int            `json:"memory_count"`
		Forms       map[string]int `json:"repeated_action_forms"`
		Evidence    []string       `json:"evidence_memory_ids"`
	}
	if err := json.Unmarshal(r.state.Background[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.MemoryCount != 4 || len(payload.Evidence) != 4 || payload.Forms[""] != 0 {
		t.Fatalf("operational signal counted interpretations as actions: %#v", payload)
	}
}

func TestMemoryCorrectionAndExperienceRevisionSurviveRestart(t *testing.T) {
	root := t.TempDir()
	r, err := New(root, "correction", testConfig(10), nil)
	if err != nil {
		t.Fatal(err)
	}
	first := Memory{ID: "old", Meaning: "照片是七月拍的", ObservedAt: "2026-09-04T00:00:00Z", SourceRefs: []string{"photo-old"}}
	e := Experience{ID: "date-rule", Judgment: "相信我的日期印象", Context: "回忆照片", Evidence: []string{"old"}}
	if err := r.commitLearning(learningBatch{Memories: []Memory{first}, Experiences: []Experience{e}}); err != nil {
		t.Fatal(err)
	}
	if err := r.persist(); err != nil {
		t.Fatal(err)
	}
	corrected := Memory{ID: "new", Meaning: "照片日期实际上是八月，之前记成七月", ObservedAt: "2026-09-04T00:01:00Z", Corrects: "old", SourceRefs: []string{"old", "photo-metadata"}}
	e.Judgment = "重要日期先查看照片元数据"
	e.Evidence = []string{"old", "new"}
	if err := r.commitLearning(learningBatch{Memories: []Memory{corrected}, Experiences: []Experience{e}}); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash after the durable learning entry, before the snapshot.
	reloaded, err := New(root, "correction", testConfig(10), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := reloaded.learning.recall("照片 日期 七月", "fixed")
	if reloaded.learning.memories["old"].Meaning != first.Meaning {
		t.Fatal("correction erased the earlier understanding")
	}
	if reloaded.learning.experiences["date-rule"].Judgment != e.Judgment {
		t.Fatal("experience revision was not recovered")
	}
	for _, m := range found.Memories {
		if m.ID == "old" {
			t.Fatal("normal recall preferred superseded memory")
		}
	}
	if len(found.Memories) == 0 || found.Memories[0].ID != "new" {
		t.Fatalf("correction unavailable: %#v", found)
	}
}

func TestIndependentPerceptionCanBecomeMemoryAndConditionalExperience(t *testing.T) {
	r, err := New(t.TempDir(), "perception", testConfig(10), nil)
	if err != nil {
		t.Fatal(err)
	}
	r.activeCandidates["reading"] = Event{ID: "reading", Kind: "perceptual_change"}
	commit := CognitiveCommit{FocusID: "reading", MemoryUpdates: []MemoryUpdate{{Content: "我对这位建筑师跨城市的生活产生好奇", Origin: "reflected", Keywords: []string{"建筑师", "迁徙"}}}, ExperienceUpdates: []ExperienceUpdate{{Judgment: "比较不同城市的作品或许能帮助理解他的变化", Context: "追踪这位建筑师的作品", Evidence: []string{"new:0"}}}}
	if err := r.validateLearningUpdates(commit); err != nil {
		t.Fatal(err)
	}
	if err := r.applyLearningUpdates(commit); err != nil {
		t.Fatal(err)
	}
	if len(r.state.Memories) != 1 || len(r.state.Experiences) != 1 {
		t.Fatalf("perception required an action: %#v", r.state)
	}
	if r.state.Experiences[0].Evidence[0] != r.state.Memories[0].ID {
		t.Fatal("experience did not point at its own memory")
	}
	commit.ExperienceUpdates[0].Evidence = []string{"unknown-memory"}
	if r.validateLearningUpdates(commit) == nil {
		t.Fatal("invented memory reference was accepted")
	}
}

func TestRecallDoesNotMultiplyOneSourceIntoIndependentEvidence(t *testing.T) {
	index := newLearningIndex()
	for i := 0; i < 10; i++ {
		index.apply(learningBatch{Memories: []Memory{{ID: fmt.Sprintf("m%d", i), Meaning: "上海的建筑值得继续探索", SourceRefs: []string{"same-reading"}}}})
	}
	found := index.recall("上海 建筑 探索", "seed")
	if len(found.Memories) != 1 {
		t.Fatalf("one event monopolised recall through paraphrases: %#v", found)
	}
	index.apply(learningBatch{Memories: []Memory{{ID: "different", Meaning: "上海另一处建筑实际与之前印象不同", SourceRefs: []string{"new-reading"}}}})
	found = index.recall("上海 建筑", "seed")
	if len(found.Memories) != 2 {
		t.Fatal("independent observation was lost")
	}
	for i := 0; i < 10; i++ {
		index.apply(learningBatch{Experiences: []Experience{{ID: fmt.Sprintf("e%d", i), Judgment: "上海建筑的一个近似判断", Evidence: []string{fmt.Sprintf("m%d", i)}}}})
	}
	found = index.recall("上海 建筑", "seed")
	if len(found.Experiences) > 1 {
		t.Fatal("one source occupied recall with multiple experience paraphrases")
	}
}

func TestLegacyEpisodesMigrateWithoutRewritingTheirMeaning(t *testing.T) {
	root := t.TempDir()
	s, err := NewStore(root)
	if err != nil {
		t.Fatal(err)
	}
	state := map[string]any{"schema": stateSchema, "instance_id": "legacy", "stage": 10, "generation_kind": "engineering", "experiences": []map[string]any{{"id": "old", "meaning": "过去的具体经历", "lesson": "以前形成的一个判断", "observed_at": "2026-09-03T00:00:00Z"}}, "commitments": []map[string]any{{"id": "c", "experience_id": "old"}}}
	encoded, _ := json.Marshal(state)
	if err := os.WriteFile(filepath.Join(root, "state", "current.json"), encoded, 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	index, err := s.loadLearning(*loaded)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Commitments[0].MemoryID != "old" || index.memories["old"].Meaning != "过去的具体经历" {
		t.Fatal("legacy identity or content was rewritten")
	}
	if !strings.Contains(index.experiences["experience-old"].Judgment, "以前") {
		t.Fatal("legacy lesson was not preserved")
	}
}

func TestStageTenHasOnePersonalJudgmentWritePath(t *testing.T) {
	tool := cognitiveCommitTool(10, []Event{{ID: "fact", Kind: "perceptual_change"}}, false, true, true)
	addLearningSchema(tool)
	properties := tool["parameters"].(map[string]any)["properties"].(map[string]any)
	item := properties["reality_updates"].(map[string]any)["items"].(map[string]any)
	fields := item["properties"].(map[string]any)
	for _, field := range []string{"lesson", "method_update", "method_slot"} {
		if _, ok := fields[field]; ok {
			t.Fatalf("duplicate legacy learning field %s", field)
		}
		for _, required := range item["required"].([]string) {
			if required == field {
				t.Fatalf("removed field still required: %s", field)
			}
		}
	}
	if properties["experience_updates"] == nil || fields["prediction_difference"] == nil {
		t.Fatal("personal learning or reality feedback was lost")
	}
}

func TestRecallDiscountsGenericMethodLanguageAndReindexesRevisions(t *testing.T) {
	index := newLearningIndex()
	for i := 0; i < 20; i++ {
		index.apply(learningBatch{Memories: []Memory{{ID: fmt.Sprintf("generic-%d", i), Meaning: "我已经核验现实边界，得到具体可判断的内容，理解身体行动与世界反馈，然后停止重复核验", SourceRefs: []string{fmt.Sprintf("event-%d", i)}}}})
	}
	index.apply(learningBatch{Memories: []Memory{{ID: "topic", Meaning: "协调机制中的共享资源通过中间件管理访问", Keywords: []string{"coordination", "共享资源"}, SourceRefs: []string{"reading"}}}})
	found := index.recall("我已经核验内容 coordination 共享资源", "")
	if len(found.Memories) == 0 || found.Memories[0].ID != "topic" {
		t.Fatalf("generic methods displaced concrete topic: %#v", found)
	}
	index.apply(learningBatch{Experiences: []Experience{{ID: "rule", Judgment: "uniqueobsoleteword", Evidence: []string{"topic"}}}})
	index.apply(learningBatch{Experiences: []Experience{{ID: "rule", Judgment: "其他理解", Evidence: []string{"topic"}}}})
	if index.terms["un"]["rule"] {
		t.Fatal("superseded experience terms survived in retrieval index")
	}
}
