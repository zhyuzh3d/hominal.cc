package runtime

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestProbeFailureDoesNotRevokeCognition(t *testing.T) {
	c := &blockingCognizer{started: make(chan CognitiveRequest, 2), release: make(chan struct{})}
	r, err := New(t.TempDir(), "probe-independent", testConfig(10), c)
	if err != nil {
		t.Fatal(err)
	}
	r.state.Body.NetworkAvailable = false
	r.config.GenerationKind = "engineering"
	focus := Event{ID: "current", Kind: "mentor_received", Status: "pending", Summary: "一个真实问题"}
	r.state.Background = []Event{focus}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.maybeStartCognition(ctx)
	select {
	case <-c.started:
	case <-time.After(time.Second):
		t.Fatal("an unrelated network probe silenced the main brain")
	}
	r.maybeStartCognition(ctx)
	if len(c.started) != 0 {
		t.Fatal("duplicate foreground lease")
	}
}

func TestResourceReservationsDoNotManufactureRepeatedBandChanges(t *testing.T) {
	r, err := New(t.TempDir(), "resource-baseline", testConfig(10), nil)
	if err != nil {
		t.Fatal(err)
	}
	r.config.CognitiveResource.RollingHourLimitMicrousd = 1000
	r.config.CognitiveResource.RollingDayLimitMicrousd = 10000
	now := time.Now().UTC()
	r.state.Usage = []UsageRecord{{CallID: "paid", Time: now.Format(time.RFC3339Nano), ActualMicrousd: 240, CostConfirmed: true}}
	if err = r.refreshResourceBody(now); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		r.state.ModelReservations = map[string]PendingModelCall{"temporary": {Reservation: ModelReservation{ReservedMicrousd: 100}}}
		if err = r.refreshResourceBody(now); err != nil {
			t.Fatal(err)
		}
		if r.state.Body.CognitiveResourceBand != "open" || r.state.Body.CognitiveHourRemainingMicrousd != 660 {
			t.Fatalf("reservation truth lost: %#v", r.state.Body)
		}
		r.state.ModelReservations = nil
		if err = r.refreshResourceBody(now); err != nil {
			t.Fatal(err)
		}
	}
	if len(r.state.Background) != 0 {
		t.Fatal("reservation churn created attention")
	}
	staleSensor := r.state.Body
	r.state.Usage[0].ActualMicrousd = 260
	if err = r.refreshResourceBody(now); err != nil {
		t.Fatal(err)
	}
	seq := r.state.EventSeq
	if seq != 1 || r.state.Body.CognitiveResourceBand != "comfortable" {
		t.Fatal("real settled crossing was not reported once")
	}
	if err = r.acceptBodySnapshot(staleSensor); err != nil {
		t.Fatal(err)
	}
	if r.state.EventSeq != seq {
		t.Fatal("a later sensor repeated the settled crossing")
	}
	if err = r.refreshResourceBody(now.Add(61 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if r.state.Body.CognitiveResourceBand != "open" || r.state.EventSeq != seq+1 {
		t.Fatal("rolling recovery was not reported")
	}
}

func TestMentorReplyIsOneIndependentUtterance(t *testing.T) {
	r, err := New(t.TempDir(), "single-message", testConfig(10), nil)
	if err != nil {
		t.Fatal(err)
	}
	r.state.Lease = &Lease{ID: "busy"} // Keep this test entirely synchronous.
	r.state.Mentor.Outbox = []MentorMessage{{MessageID: "sent", CommitmentID: "old-action", Body: "以前的问题", Status: "delivered"}}
	r.state.Commitments = []ActionCommitment{{ID: "old-action", ConcernID: "old-concern", Status: "assimilated"}}
	reply := make(chan CommandReply, 1)
	command := RuntimeCommand{Kind: "mentor_receive", Mentor: MentorInput{MessageID: "new", ReplyTo: "sent", Body: "已收到，也有一个新问题。"}, Reply: reply}
	if err = r.handleCommand(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	<-reply
	if len(r.state.Background) != 1 {
		t.Fatal("message was duplicated")
	}
	e := r.state.Background[0]
	if commitmentIDFromEvent(e) != "" || e.ConcernID != "" || !strings.Contains(string(e.Payload), "old-concern") || !strings.Contains(string(e.Payload), "以前的问题") {
		t.Fatalf("reply lost relationship or acquired forced old identity: %s", e.Payload)
	}
	request := CognitiveRequest{Stage: 10, Focus: e, Candidates: []Event{e}, Config: r.config, Profile: CognitiveProfile{Model: "main", ReasoningEffort: "none"}}
	if !requestAllowsMentorSend(request) || !mentorWireHasAction(t, request, "mentor_send") {
		t.Fatal("reply still needs a second paid content pass")
	}
	if err = r.addMentorContentCandidate(CognitiveCommit{FocusID: e.ID}); err != nil {
		t.Fatal(err)
	}
	if len(r.state.Background) != 1 {
		t.Fatal("reply body duplicated")
	}
	if err = r.handleCommand(context.Background(), command); err != nil {
		t.Fatal(err)
	}
	<-reply
	if len(r.state.Background) != 1 {
		t.Fatal("message ID deduplication lost")
	}
}

func TestDeadlineDrainsArrivedFactsAndLateAssistanceNotNewConversation(t *testing.T) {
	r, err := New(t.TempDir(), "deadline-facts", testConfig(10), nil)
	if err != nil {
		t.Fatal(err)
	}
	r.config.GenerationKind = "rehearsal"
	now := time.Now().UTC()
	r.state.PlannedEnd = now.Format(time.RFC3339Nano)
	for _, tc := range []struct {
		kind string
		at   time.Time
		want bool
	}{
		{"mentor_received", now.Add(-time.Second), true},
		{"mentor_received", now.Add(time.Second), false},
		{"cognition_assistance_result", now.Add(time.Second), true},
		{"action_result", now.Add(time.Second), true},
		{"value_signal", now.Add(-time.Second), false},
	} {
		request := CognitiveRequest{Focus: Event{Kind: tc.kind, ObservedAt: tc.at.Format(time.RFC3339Nano)}}
		if got := r.cognitiveRequestAllowedAt(request, now.Add(2*time.Second)); got != tc.want {
			t.Fatalf("%s at %s allowed=%v want=%v", tc.kind, tc.at, got, tc.want)
		}
	}
}

func TestPerceptualMagnitudePreservesTinyChangesWithoutCallingThemNewMeaning(t *testing.T) {
	text := strings.Repeat("a concrete observed object with details and controls ", 20) + "clock 12"
	changed := strings.Replace(text, "clock 12", "clock 13", 1)
	tiny := perceptualChangeMagnitude(text, changed)
	large := perceptualChangeMagnitude(text, "Another scene: mountain river bright flowers")
	if tiny <= 0 || tiny >= large || perceptualChangeMagnitude(text, text) != 0 {
		t.Fatalf("tiny=%f large=%f", tiny, large)
	}
	r := newDifferenceTestRuntime(t)
	for i := 0; i < 30; i++ {
		p, _ := json.Marshal(map[string]string{"organ_id": "any-organ", "surface_id": "local", "object_id": "text", "content": text})
		r.admitDifference(Event{Kind: "perceptual_change", Source: "observed", Payload: p})
	}
	p, _ := json.Marshal(map[string]string{"organ_id": "any-organ", "surface_id": "local", "object_id": "changed", "content": changed})
	e, _ := r.admitDifference(Event{Kind: "perceptual_change", Source: "observed", Payload: p})
	if e.PredictionGap >= .1 {
		t.Fatal("a small content change became an entire unexpected world")
	}
}

func TestRecallUsesMaterialNotTransportAndRandomHintIsNotAnAddress(t *testing.T) {
	result := `{"content":[{"type":"text","text":"a substantive object"}],"metadata":"transport-only-noise"}`
	p, _ := json.Marshal(ActionState{Operation: "read", Result: result})
	q := memoryQuery([]Event{{Kind: "action_result", Payload: p}})
	if !strings.Contains(q, "substantive object") || strings.Contains(q, "transport-only-noise") {
		t.Fatalf("wrong material: %s", q)
	}
	p, _ = json.Marshal(map[string]string{"surface": "current material", "context_memory_id": "random-old-id", "context_meaning": "a possible association"})
	q = memoryQuery([]Event{{Kind: "value_signal", Summary: "random-old-id", Payload: p}})
	if strings.Contains(q, "random-old-id") || !strings.Contains(q, "possible association") {
		t.Fatal("random cue became a forced record lookup")
	}
}

func TestLearningSchemaReferencesOnlyAvailableFactsAndMemories(t *testing.T) {
	tool := map[string]any{"parameters": map[string]any{"properties": map[string]any{}, "required": []string{}}}
	addLearningSchema(tool)
	request := CognitiveRequest{
		Candidates: []Event{{ID: "current-fact"}},
		Recall:     RecallBundle{Memories: []Memory{{ID: "old-memory"}}, Experiences: []Experience{{ID: "old-judgment"}}},
	}
	addLearningReferences(tool, request)
	properties := tool["parameters"].(map[string]any)["properties"].(map[string]any)
	mem := properties["memory_updates"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	exp := properties["experience_updates"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	for name, check := range map[string]struct {
		field map[string]any
		want  []string
	}{
		"source":     {mem["source_refs"].(map[string]any)["items"].(map[string]any), []string{"current-fact", "old-memory"}},
		"corrects":   {mem["corrects"].(map[string]any), []string{"", "old-memory"}},
		"experience": {exp["id"].(map[string]any), []string{"", "old-judgment"}},
		"evidence":   {exp["evidence"].(map[string]any)["items"].(map[string]any), []string{"old-memory", "new:0", "new:1", "new:2"}},
	} {
		got, _ := json.Marshal(check.field["enum"])
		want, _ := json.Marshal(check.want)
		if string(got) != string(want) {
			t.Fatalf("%s references = %s, want %s", name, got, want)
		}
	}
	// Generation-time hints complement rather than replace runtime evidence
	// validation: a new:n reference must still correspond to an actual fragment.
	r, err := New(t.TempDir(), "valid-evidence", testConfig(10), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = r.validateLearningUpdates(CognitiveCommit{ExperienceUpdates: []ExperienceUpdate{{Judgment: "a judgment", Evidence: []string{"new:2"}}}}); err == nil {
		t.Fatal("a schema-listed but nonexistent memory was accepted")
	}
}

func TestStage103FinalArchiveResultRecall(t *testing.T) {
	path := os.Getenv("HOMINAL_FINAL_REPLAY_ARCHIVE")
	if path == "" {
		t.Skip("set HOMINAL_FINAL_REPLAY_ARCHIVE for frozen final-hour replay")
	}
	index, original := archivedRecallAt(t, path, 996)
	if recallContainsSubject(original, "Cybercab") {
		t.Fatal("the selected frozen counterexample unexpectedly already recalled the subject")
	}
	start := strings.Index(original.Query, "{")
	if start < 0 {
		t.Fatal("recorded action payload missing")
	}
	payload := json.RawMessage(original.Query[start:])
	for _, seed := range []string{original.Seed, "variation-one", "variation-two"} {
		found := index.recall(memoryQuery([]Event{{Kind: "action_result", Payload: payload}}), seed)
		if !recallContainsSubject(found, "Cybercab") {
			for _, m := range found.Memories {
				t.Log(m.ID, m.Meaning)
			}
			t.Fatal("previous encounter with the same content missing")
		}
	}
	// Preserve the same sample's genuine positive learning. A content-oriented
	// query must not throw away technical experience when the current issue is
	// actually a control's role/locator rather than the post's subject.
	index, _ = archivedRecallAt(t, path, 173)
	for _, seed := range []string{"technical-one", "technical-two", "technical-three"} {
		found := index.recall("Following role=tab aria-selected 寻址 控件 祖先语义", seed)
		seen := false
		for _, e := range found.Experiences {
			seen = seen || (e.ID == "experience-lease-f4dd2c4ef2d536ed-0" && strings.Contains(e.Context, "role=tab"))
		}
		if !seen {
			t.Fatal("learned Following locator missing from technical recall")
		}
	}
}
