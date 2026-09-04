package runtime

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"hominal.cc/hominal/body/internal/organ"
)

func TestGatewayFailureCannotBeBypassedByAnotherFocusOrOrgan(t *testing.T) {
	r, err := New(t.TempDir(), "gateway", testConfig(10), nil)
	if err != nil {
		t.Fatal(err)
	}
	r.state.Usage = []UsageRecord{{Time: nowUTC(), LeaseID: "failed", RequestedModel: "terra", FailureCategory: "upstream_unavailable", HTTPStatus: 503}}
	r.state.Lease = &Lease{ID: "other-focus", FocusID: "other", Profile: CognitiveProfile{Model: "terra", ReasoningEffort: "none"}}
	local := Lease{ID: "local", Profile: CognitiveProfile{Model: "luna", ReasoningEffort: "none"}}
	r.peripheralLeases[local.ID] = local
	for _, owner := range []Lease{*r.state.Lease, local} {
		ack := make(chan NoticeAck, 1)
		if err := r.handleModelNotice(WorkerNotice{CallID: owner.ID, LeaseID: owner.ID, Kind: "model_reserve", Payload: ModelReservation{Profile: owner.Profile, ReservedMicrousd: 1000}, Ack: ack}); err != nil {
			t.Fatal(err)
		}
		if result := <-ack; result.Accepted || !strings.Contains(result.Output, "gateway") {
			t.Fatalf("shared failure was bypassed by %s: %#v", owner.ID, result)
		}
	}
	if len(r.state.ModelReservations) != 0 || r.state.CognitiveResource.Limited != nil || r.state.CognitiveResource.NextProfile != nil {
		t.Fatal("gateway wait became a bill, budget limit or personality switch")
	}
}

func TestGatewayWaitDoesNotConsumeLocalScene(t *testing.T) {
	local := &blockingLocalInterpreter{started: make(chan CognitiveRequest, 1), release: make(chan struct{})}
	r, err := New(t.TempDir(), "gateway", testConfig(10), local)
	if err != nil {
		t.Fatal(err)
	}
	r.config.GenerationKind = "engineering"
	r.state.Usage = []UsageRecord{{Time: nowUTC(), LeaseID: "failed", FailureCategory: "transport_error"}}
	observation := organ.Observation{OrganID: "browser", SurfaceID: "page", Interpret: &organ.InterpretationRequest{Question: "解释反馈", Material: "暂时无法加载"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.startInstinct(ctx, observation, 0)
	if len(r.peripheralLeases) != 0 || len(r.instinctScenes) != 0 {
		t.Fatal("gateway wait dispatched or consumed the local scene")
	}
	r.state.Usage = append(r.state.Usage, UsageRecord{Time: nowUTC(), LeaseID: "recovered", Status: "completed"})
	r.startInstinct(ctx, observation, 0)
	select {
	case <-local.started:
	case <-time.After(time.Second):
		t.Fatal("recovered gateway could not interpret the same scene")
	}
}

func TestGatewayWaitDoesNotAcquireConsciousFocus(t *testing.T) {
	r, err := New(t.TempDir(), "gateway", testConfig(10), &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})})
	if err != nil {
		t.Fatal(err)
	}
	r.config.GenerationKind = "engineering"
	r.state.Body.NetworkAvailable = true
	r.state.Background = []Event{{ID: "new-focus", Kind: "mentor_message", Status: "pending", Summary: "你好"}}
	r.state.Usage = []UsageRecord{{Time: nowUTC(), LeaseID: "failed-other-focus", FailureCategory: "upstream_unavailable"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.maybeStartCognition(ctx)
	if r.state.Lease != nil || r.state.CurrentFocus != "" || r.state.Background[0].Status != "pending" {
		t.Fatal("infrastructure wait acquired or consumed the next conscious focus")
	}
}

func TestGatewayBackoffIsAnchoredBoundedAndReleasedByReachableResponse(t *testing.T) {
	now := time.Now().UTC()
	config := testConfig(10).CognitiveResource
	state := State{}
	for index := 0; index < 10; index++ {
		state.Usage = append(state.Usage, UsageRecord{Time: now.Format(time.RFC3339Nano), FailureCategory: "upstream_unavailable"})
		gate := gatewayRetry(state, config)
		want := min(time.Duration(1<<index)*10*time.Second, 10*time.Minute)
		if gate.Until.Sub(now) != want || gate.allows(now, true) || !gate.allows(now.Add(want), true) || gate.allows(now.Add(want), false) {
			t.Fatalf("unbounded, sliding or bypassed retry: %#v, want %s", gate, want)
		}
	}
	for _, retryAfter := range []string{"90", now.Add(90 * time.Second).Format(http.TimeFormat)} {
		state.Usage = []UsageRecord{{Time: now.Format(time.RFC3339Nano), FailureCategory: "transport_error", RetryAfter: retryAfter}}
		gate := gatewayRetry(state, config)
		if gap := gate.Until.Sub(now); gap < 89*time.Second || gap > 90*time.Second {
			t.Fatalf("Retry-After was not anchored: %s", gap)
		}
	}
	state.Usage = append(state.Usage, UsageRecord{Time: nowUTC(), FailureCategory: "billing_unconfirmed"})
	if gatewayRetry(state, config).Failures == 0 {
		t.Fatal("unknown bill was mistaken for recovery")
	}
	state.Usage = append(state.Usage, UsageRecord{Time: nowUTC(), Status: "unusable"})
	if !gatewayRetry(state, config).allows(now, false) {
		t.Fatal("reachable but semantically invalid response did not restore transport")
	}
}

func TestGatewayRecoveryAdmitsOneRequestAndAcceptsLateBill(t *testing.T) {
	r, err := New(t.TempDir(), "gateway", testConfig(10), nil)
	if err != nil {
		t.Fatal(err)
	}
	r.state.Usage = []UsageRecord{{LeaseID: "failed", Time: time.Now().Add(-11 * time.Second).UTC().Format(time.RFC3339Nano), FailureCategory: "transport_error"}}
	owner := Lease{ID: "recover", Profile: CognitiveProfile{Model: "terra", ReasoningEffort: "none"}}
	r.state.Lease = &owner
	reserve := func(key string) NoticeAck {
		ack := make(chan NoticeAck, 1)
		if err := r.handleModelNotice(WorkerNotice{CallID: key, LeaseID: owner.ID, Kind: "model_reserve", Payload: ModelReservation{Profile: owner.Profile, ReservedMicrousd: 1000}, Ack: ack}); err != nil {
			t.Fatal(err)
		}
		return <-ack
	}
	if !reserve("probe").Accepted || reserve("concurrent-probe").Accepted {
		t.Fatal("recovery did not admit exactly one physical request")
	}
	r.state.Lease = nil
	for iteration := 0; iteration < 2; iteration++ {
		ack := make(chan NoticeAck, 1)
		usage := UsageRecord{Time: nowUTC(), LeaseID: owner.ID, RequestedModel: "terra", ReservedMicrousd: 1000, ActualMicrousd: 100, CostConfirmed: true, Status: "completed"}
		if err := r.handleModelNotice(WorkerNotice{CallID: "probe", LeaseID: owner.ID, Kind: "model_usage", Payload: usage, Ack: ack}); err != nil {
			t.Fatal(err)
		}
		if !(<-ack).Accepted {
			t.Fatal("late recovery bill rejected")
		}
	}
	spent, _, pending := resourceSpend(r.state, r.config.CognitiveResource, time.Now())
	if spent != 100 || pending != 0 || !gatewayRetry(r.state, r.config.CognitiveResource).allows(time.Now(), false) {
		t.Fatal("late recovery did not release both shared gate and reservation exactly once")
	}
}

func TestGatewayDenialRetainsInfrastructureMeaningAcrossModelBoundary(t *testing.T) {
	config := testConfig(10)
	profile := CognitiveProfile{Model: "terra", ReasoningEffort: "none"}
	request := CognitiveRequest{Config: config, Profile: profile, Lease: Lease{ID: "main", Profile: profile}}
	notices := make(chan WorkerNotice)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		select {
		case notice := <-notices:
			notice.Ack <- NoticeAck{Output: "shared gateway recovery", Failure: &ModelFailureFact{Category: "gateway_backoff"}}
		case <-ctx.Done():
		}
	}()
	_, err := NewModelClient().call(ctx, request, notices, "test", "test", nil, "")
	var failure *ModelCallError
	if !errors.As(err, &failure) || failure.Fact.Category != "gateway_backoff" {
		t.Fatalf("gateway denial became a budget/semantic failure: %v", err)
	}
	r, err := New(t.TempDir(), "retry", config, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.state.Lease = &request.Lease
	r.state.Lease.FocusID = "focus"
	r.state.Background = []Event{{ID: "focus", Status: "in_focus"}}
	if err := r.handleCognitiveResult(ctx, CognitiveResult{LeaseID: "main", FocusID: "focus", Error: failure}); err != nil {
		t.Fatal(err)
	}
	if event := r.state.Background[0]; event.Status != "retry_wait" || event.CognitionAttempts != 0 || len(r.state.Usage) != 0 {
		t.Fatalf("local denial spent money or consumed cognition: %#v", event)
	}
}
