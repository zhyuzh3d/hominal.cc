package runtime

import (
	"context"
	"encoding/json"
	"hominal.cc/hominal/body/internal/organ"
	"testing"
	"time"
)

func TestSharedReservationsIncludeMainAndLocalModelsAndSurviveLateResults(t *testing.T) {
	r, err := New(t.TempDir(), "parallel", testConfig(10), nil)
	if err != nil {
		t.Fatal(err)
	}
	r.config.CognitiveResource.RollingHourLimitMicrousd = 1000
	main := Lease{ID: "main", Profile: CognitiveProfile{Model: "terra", ReasoningEffort: "none"}}
	local := Lease{ID: "local", Profile: CognitiveProfile{Model: "luna", ReasoningEffort: "none"}}
	r.state.Lease = &main
	r.peripheralLeases[local.ID] = local
	reserve := func(key string, owner Lease, amount int64) bool {
		ack := make(chan NoticeAck, 1)
		err := r.handleModelNotice(WorkerNotice{CallID: key, LeaseID: owner.ID, Kind: "model_reserve", Payload: ModelReservation{Profile: owner.Profile, ReservedMicrousd: amount}, Ack: ack})
		if err != nil {
			t.Fatal(err)
		}
		return (<-ack).Accepted
	}
	if !reserve("main-1", main, 700) || reserve("local-too-large", local, 400) || !reserve("local-1", local, 200) {
		t.Fatal("shared reservation gate is incorrect")
	}
	_, _, pending := resourceSpend(r.state, r.config.CognitiveResource, time.Now())
	if pending != 900 {
		t.Fatalf("in-flight = %d", pending)
	}
	r.state.Lease = nil // cognition ended; a real bill retains its original owner
	settle := func(key string, owner Lease, reserved, amount int64) {
		ack := make(chan NoticeAck, 1)
		usage := UsageRecord{CallID: key, LeaseID: owner.ID, Time: nowUTC(), RequestedModel: owner.Profile.Model, ReservedMicrousd: reserved, ActualMicrousd: amount, CostConfirmed: true, Status: "completed"}
		if err := r.handleModelNotice(WorkerNotice{CallID: key, LeaseID: owner.ID, Kind: "model_usage", Payload: usage, Ack: ack}); err != nil {
			t.Fatal(err)
		}
		if !(<-ack).Accepted {
			t.Fatal("late physical bill rejected")
		}
	}
	settle("main-1", main, 700, 100)
	settle("main-1", main, 700, 100)
	settle("local-1", local, 200, 20)
	main.ReservedMicrousd = 0 // a new lease is not the retired owner's stale snapshot
	r.state.Lease = &main
	if !reserve("main-2", main, 700) {
		t.Fatal("settled reservation was not released")
	}
	settle("main-2", main, 700, 80)
	records, err := r.store.LoadUsage(time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Fatalf("multiple requests in one lease collapsed or duplicated: %d", len(records))
	}
	spent, _, pending := resourceSpend(r.state, r.config.CognitiveResource, time.Now())
	if spent != 200 || pending != 0 {
		t.Fatalf("spend=%d pending=%d", spent, pending)
	}
}

func TestLatePerceptionCannotReplaceSceneAndOldScanCannotReplaceBill(t *testing.T) {
	r, err := New(t.TempDir(), "ordering", testConfig(10), nil)
	if err != nil {
		t.Fatal(err)
	}
	r.perceptionPending = "old"
	r.actionEpoch = 2
	observation := organ.Observation{OrganID: "browser", SurfaceID: "page", ObservedAt: nowUTC(), Objects: []organ.Object{{ID: "object", Content: "old scene"}}}
	if err := r.acceptPerception(context.Background(), perceptionResult{ID: "old", Epoch: 1, Observation: observation}); err != nil {
		t.Fatal(err)
	}
	if len(r.state.Perception) != 0 {
		t.Fatal("pre-action scene overwritten current state")
	}
	r.state.Usage = []UsageRecord{{CallID: "paid", LeaseID: "main", Time: nowUTC(), ActualMicrousd: 1234, CostConfirmed: true}}
	if err := r.acceptBodySnapshot(BodySnapshot{CognitiveHourRemainingMicrousd: 5_000_000}); err != nil {
		t.Fatal(err)
	}
	if r.state.Body.CognitiveHourRemainingMicrousd != 5_000_000-1234 {
		t.Fatal("slow scan erased a newer bill")
	}
}

type blockingLocalInterpreter struct {
	started chan CognitiveRequest
	release chan struct{}
}

func (b *blockingLocalInterpreter) Run(context.Context, CognitiveRequest, chan<- WorkerNotice) CognitiveResult {
	return CognitiveResult{}
}
func (b *blockingLocalInterpreter) Interpret(ctx context.Context, r CognitiveRequest, _ chan<- WorkerNotice, _, _ string) (string, error) {
	b.started <- r
	select {
	case <-b.release:
		return "局部反馈仍不确定", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func TestLocalInstinctDoesNotOwnConsciousnessOrPersonalHistory(t *testing.T) {
	local := &blockingLocalInterpreter{started: make(chan CognitiveRequest, 1), release: make(chan struct{})}
	r, err := New(t.TempDir(), "roles", testConfig(10), local)
	if err != nil {
		t.Fatal(err)
	}
	r.state.Lease = &Lease{ID: "main"}
	r.state.Self.Narrative = "PRIVATE SELF"
	observation := organ.Observation{OrganID: "browser", SurfaceID: "page", ObservedAt: nowUTC(), Interpret: &organ.InterpretationRequest{Question: "页面反馈含义？", Material: "暂时无法加载"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.config.GenerationKind = "rehearsal"
	r.startInstinct(ctx, observation, 0)
	if len(r.peripheralLeases) != 0 {
		t.Fatal("birth preflight triggered personal resource consumption")
	}
	r.config.GenerationKind = "engineering"
	r.startInstinct(ctx, observation, 0)
	select {
	case request := <-local.started:
		encoded, _ := json.Marshal(request.State)
		if string(encoded) == "" || request.State.Self.Narrative != "" {
			t.Fatal("local model received personal history")
		}
		if request.Profile.Model != "luna" || request.Profile.ReasoningEffort != "none" {
			t.Fatal("local role changed")
		}
	case <-time.After(time.Second):
		t.Fatal("local worker did not start")
	}
	if r.state.Lease.ID != "main" || !r.passivePerceptionAllowed() {
		t.Fatal("local inference blocked main or senses")
	}
	r.startInstinct(ctx, observation, 0)
	if len(r.peripheralLeases) != 1 {
		t.Fatal("duplicate local work")
	}
	r.actionEpoch++
	close(local.release)
	result := <-r.instinctResults
	if err := r.acceptInstinct(result); err != nil {
		t.Fatal(err)
	}
	if len(r.state.Background) != 0 {
		t.Fatal("stale local inference became current fact")
	}
	r.startInstinct(ctx, observation, 1)
	if len(r.peripheralLeases) != 0 {
		t.Fatal("same material called the local model again")
	}
}
