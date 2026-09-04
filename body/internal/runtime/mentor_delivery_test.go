package runtime

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func mentorWireHasAction(t *testing.T, request CognitiveRequest, kind string) bool {
	t.Helper()
	wire := make(chan map[string]any, 1)
	isolatedModelInput(t, request, func(body map[string]any) { wire <- body })
	body := <-wire
	tool := body["tools"].([]any)[0].(map[string]any)
	properties := tool["parameters"].(map[string]any)["properties"].(map[string]any)
	for _, raw := range properties["action"].(map[string]any)["anyOf"].([]any) {
		branch := raw.(map[string]any)["properties"].(map[string]any)
		if branch["kind"].(map[string]any)["enum"].([]any)[0] == kind {
			return true
		}
	}
	return false
}

func TestUnreadMentorDeliveryIsNotAnExecutingAction(t *testing.T) {
	for _, status := range []string{"queued", "delivered", "replied"} {
		for _, kind := range []string{"mentor_received", "perceptual_change"} {
			t.Run(status+"/"+kind, func(t *testing.T) {
				r, err := New(t.TempDir(), "delivery", testConfig(10), nil)
				if err != nil {
					t.Fatal(err)
				}
				old := MentorMessage{MessageID: "old", CommitmentID: "old-action", Body: "早先发送的另一件事", Status: status, QueuedAt: nowUTC()}
				if status == "replied" {
					old.Status, old.RepliedAt = "queued", nowUTC()
				}
				r.state.Mentor.Outbox = []MentorMessage{old}
				r.state.Commitments = []ActionCommitment{{ID: "old-action", ConcernID: "old-concern", ActionKind: "mentor_send", Status: "assimilated"}}
				focus := Event{ID: "new-event", Kind: kind, Summary: "一个新的具体问题", Status: "in_focus"}
				if kind == "mentor_received" {
					focus.CorrelationID = "new-question"
				}
				r.state.Background = []Event{focus}
				r.activeCandidates = map[string]Event{focus.ID: focus}
				request := CognitiveRequest{Stage: 10, Config: r.config, State: r.state, Focus: focus, Candidates: []Event{focus}, Profile: CognitiveProfile{Model: "terra", ReasoningEffort: "none"}, Lease: Lease{ID: "delivery-request"}}
				if !mentorWireHasAction(t, request, "mentor_send") {
					t.Error("unread delivery removed an independent expression from the actual model tool")
				}
				available := false
				for _, a := range r.lifeValueAffordances("relatedness") {
					available = available || a.Key == "mentor_channel"
				}
				if !available {
					t.Error("unread delivery removed the whole relationship affordance")
				}
				commit := CognitiveCommit{
					FocusID: focus.ID, NewConcernClosureCondition: "把这个新的具体想法送入导师通道",
					Appraisals:     []CandidateAppraisal{{CandidateID: focus.ID, Meaning: "我愿意表达新想法", Difference: .4, Ownership: .9, Value: .7, Answerability: .8, Certainty: .9, Resolution: "hold"}},
					ThoughtThread:  "这与先前正在等候的事项不同。",
					Action:         CognitiveAction{Kind: "mentor_send", Text: "我想把今天读到的材料写成一则短故事。", ReplyTo: focus.CorrelationID, Intent: "表达具体想法", Prediction: "消息入队", RealityCheck: "获得消息标识", StopCondition: "发送一次"},
					ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
				}
				if err := r.applyCognitiveCommit(commit); err != nil {
					t.Fatalf("valid independent expression rejected: %v", err)
				}
				formed := r.state.Commitments[len(r.state.Commitments)-1]
				commit.Action.CommitmentID = formed.ID
				if err := r.startStage4Action(context.Background(), "delivery-request", commit.Action); err != nil {
					t.Fatal(err)
				}
				if len(r.state.Mentor.Outbox) != 2 || r.state.Mentor.Outbox[0] != old {
					t.Fatal("new send changed prior delivery facts or failed to enqueue")
				}
				if r.state.Mentor.Outbox[1].Status != "queued" || r.state.PendingAction != nil {
					t.Fatal("synchronous enqueue did not finish independently of reader acknowledgement")
				}
				if err := r.validateMentorCausalNovelty(CognitiveAction{Kind: "mentor_send", Text: old.Body}, time.Now()); err == nil {
					t.Fatal("unread messages lost ordinary duplicate protection")
				}
			})
		}
	}
}

func TestMentorDeliveryChangePreservesRealityAndAssistantBoundaries(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{"commitment_id": "prior-send"})
	focus := Event{ID: "reply", Kind: "mentor_received", Payload: payload}
	request := CognitiveRequest{Stage: 10, Config: testConfig(10), Focus: focus, Candidates: []Event{focus}, Profile: CognitiveProfile{Model: "terra", ReasoningEffort: "none"}, Lease: Lease{ID: "reply-only"}}
	if mentorWireHasAction(t, request, "mentor_send") || mentorWireHasAction(t, request, "organ_action") {
		t.Fatal("linked delayed Reality reopened effectors before content assimilation")
	}
	request.Focus = Event{ID: "technical", Kind: "concern"}
	request.Candidates = []Event{request.Focus}
	request.Lease.ProfileSource = "next"
	// Local requests now use assistance_result rather than the main commit
	// grammar; the native-wire boundary is covered by assistance_test.go.
	if requestAllowsMentorSend(request) {
		t.Fatal("technical assistance became a relationship author")
	}
}

func TestStage103ArchivedCrossingMentorMessage(t *testing.T) {
	path := os.Getenv("HOMINAL_MENTOR_REPLAY_ARCHIVE")
	if path == "" {
		t.Skip("set HOMINAL_MENTOR_REPLAY_ARCHIVE for the thirteenth frozen sample")
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
	tr := tar.NewReader(gz)
	request := CognitiveRequest{Stage: 10, Config: testConfig(10), Profile: CognitiveProfile{Model: "terra", ReasoningEffort: "none"}, Lease: Lease{ID: "crossing-replay"}}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			t.Fatal("frozen crossing request not found")
		}
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasSuffix(h.Name, "/journal/events.jsonl") {
			continue
		}
		scanner := bufio.NewScanner(tr)
		scanner.Buffer(make([]byte, 65536), 8*1024*1024)
		for scanner.Scan() {
			var row struct {
				Seq           int
				Kind          string
				CorrelationID string `json:"correlation_id"`
				Payload       json.RawMessage
			}
			if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
				t.Fatal(err)
			}
			if row.Seq == 731 {
				var p struct {
					Body         string
					CommitmentID string `json:"commitment_id"`
				}
				if err := json.Unmarshal(row.Payload, &p); err != nil {
					t.Fatal(err)
				}
				request.State.Mentor.Outbox = []MentorMessage{{MessageID: row.CorrelationID, CommitmentID: p.CommitmentID, Body: p.Body, Status: "queued"}}
			}
			if row.Seq == 735 {
				var p struct {
					EventID string `json:"event_id"`
					Summary string
					Payload json.RawMessage
				}
				if err := json.Unmarshal(row.Payload, &p); err != nil {
					t.Fatal(err)
				}
				var input MentorInput
				if err := json.Unmarshal(p.Payload, &input); err != nil {
					t.Fatal(err)
				}
				if input.ReplyTo != "" {
					t.Fatal("fixture is not an independent crossing message")
				}
				request.Focus = Event{ID: p.EventID, Kind: row.Kind, Summary: p.Summary, Payload: p.Payload, CorrelationID: input.MessageID}
				request.Candidates = []Event{request.Focus}
			}
			if row.Seq == 742 {
				if len(request.State.Mentor.Outbox) != 1 || request.Focus.ID == "" {
					t.Fatal("crossing fixture incomplete")
				}
				if !mentorWireHasAction(t, request, "mentor_send") {
					t.Fatal("actual frozen question cannot be answered until an unrelated old message is acknowledged")
				}
				return
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
	}
}
