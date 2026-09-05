package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"hominal.cc/hominal/body/internal/organ"
)

func TestCurrentSituationRetainsSceneAfterNoveltyIsConsumed(t *testing.T) {
	state := State{Perception: map[string]PerceptualTrace{
		"browser/page":   {OrganID: "browser", SurfaceID: "page", ObservedAt: "2026-09-04T00:00:02Z", Context: []string{"Page URL: https://example.test/current#scene", "Page Title: Current"}, SettledByAttention: true},
		"system/session": {OrganID: "system", SurfaceID: "session", ObservedAt: "2026-09-04T00:00:01Z", Context: []string{"Working directory: /life"}},
	}}
	view := currentSituation(CognitiveRequest{State: state, Config: testConfig(10)})
	scenes, present := view["organ_scenes"]
	if !present {
		t.Fatal("current organ position vanished after its content left attention")
	}
	encoded, _ := json.Marshal(scenes)
	for _, fact := range []string{"https://example.test/current#scene", "2026-09-04T00:00:02Z", "Working directory: /life"} {
		if !strings.Contains(string(encoded), fact) {
			t.Fatalf("missing current scene fact %q: %s", fact, encoded)
		}
	}
	if strings.Contains(string(encoded), "settled_by_attention") || strings.Contains(string(encoded), "pending") {
		t.Fatal("scene projection leaked attention machinery")
	}
}

func TestCurrentSituationDoesNotRepeatCurrentActionReality(t *testing.T) {
	current := Event{ID: "result-current", Kind: "action_result", Status: "in_focus", Summary: "current result", Payload: json.RawMessage(`{"result":"current"}`)}
	previous := Event{ID: "result-previous", Kind: "action_result", Status: "settled", Summary: "previous result", Payload: json.RawMessage(`{"result":"previous"}`)}
	request := CognitiveRequest{
		State:      State{Background: []Event{previous, current}},
		Candidates: []Event{current},
		Config:     testConfig(10),
	}
	encoded, _ := json.Marshal(currentSituation(request)["recent_action_reality"])
	if strings.Contains(string(encoded), "current result") || !strings.Contains(string(encoded), "previous result") {
		t.Fatalf("current candidate was duplicated or prior reality was lost: %s", encoded)
	}
}

func TestOrientationSettlesBeforeCognitionButReadOnlySenseDoesNotBlock(t *testing.T) {
	for _, orient := range []bool{true, false} {
		name := "read_only"
		if orient {
			name = "scene_change"
		}
		t.Run(name, func(t *testing.T) {
			model := &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})}
			r, err := New(t.TempDir(), name, testConfig(10), model)
			if err != nil {
				t.Fatal(err)
			}
			r.state.Body.NetworkAvailable = true
			if err := r.addEvent("mentor_received", "observed", "a new message", "", nil, true); err != nil {
				t.Fatal(err)
			}
			r.perceptionPending, r.perceptionOrients = "sense-1", orient
			cancelled := false
			r.perceptionCancel = func() { cancelled = true }
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			r.maybeStartCognition(ctx)
			if orient {
				if r.state.Lease != nil || cancelled {
					t.Fatal("consciousness raced an unfinished scene change")
				}
				observation := organ.Observation{OrganID: "browser", SurfaceID: "page", ObservedAt: nowUTC(), Context: []string{"Page URL: https://example.test/settled"}}
				if err := r.acceptPerception(ctx, perceptionResult{ID: "sense-1", Epoch: r.actionEpoch, Observation: observation, Orientation: &organ.Orientation{OrganID: "browser", Status: "moved"}}); err != nil {
					t.Fatal(err)
				}
				r.maybeStartCognition(ctx)
			}
			select {
			case request := <-model.started:
				if orient && len(request.State.Perception["browser/page"].Context) == 0 {
					t.Fatal("consciousness did not receive the settled scene")
				}
			case <-time.After(time.Second):
				t.Fatal("cognition did not resume, or read-only sensing blocked it")
			}
		})
	}
}

func TestFailedOrientationInvalidatesPositionNotLearnedObjects(t *testing.T) {
	r, err := New(t.TempDir(), "failed-movement", testConfig(10), nil)
	if err != nil {
		t.Fatal(err)
	}
	r.state.Perception = map[string]PerceptualTrace{
		"browser/page":   {OrganID: "browser", SurfaceID: "page", Context: []string{"old page"}, Seen: []string{"remembered-object"}},
		"system/session": {OrganID: "system", SurfaceID: "session", Context: []string{"/life"}},
	}
	r.perceptionPending, r.perceptionOrients = "movement", true
	if err := r.acceptPerception(context.Background(), perceptionResult{ID: "movement", OrganID: "browser", Error: errors.New("observation timed out after moving")}); err != nil {
		t.Fatal(err)
	}
	if len(r.state.Perception["browser/page"].Context) != 0 || len(r.state.Perception["browser/page"].Seen) != 1 || len(r.state.Perception["system/session"].Context) != 1 {
		t.Fatal("failed movement retained a false location or erased unrelated knowledge")
	}
}
