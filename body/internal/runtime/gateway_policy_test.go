package runtime

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGatewayHTTPQuotaAndRateFactsRemainDistinct(t *testing.T) {
	for _, tc := range []struct {
		name, body, category string
		status               int
	}{
		{"mesh_key_quota", `{"error":{"message":"API key 额度已用完"}}`, "gateway_quota", 429},
		{"structured_quota", `{"error":{"code":"insufficient_quota","message":"exhausted"}}`, "gateway_quota", 429},
		{"typed_quota", `{"error":{"type":"insufficient_quota","message":"exhausted"}}`, "gateway_quota", 429},
		{"daily_quota", `{"error":{"code":"daily_quota_exceeded","message":"exhausted"}}`, "gateway_quota", 402},
		{"rate_not_money", `{"error":{"message":"request rate quota exceeded"}}`, "rate_limited", 429},
		{"model_contract", `{"error":{"code":"invalid_provider_tool_call"}}`, "invalid_provider_tool_call", 502},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				w.Header().Set("Retry-After", "37")
				w.Header().Set("X-Request-ID", "quota-test")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			config := testConfig(10)
			config.ModelGateway.BaseURL = server.URL
			profile := CognitiveProfile{Model: "terra", ReasoningEffort: "none"}
			request := CognitiveRequest{Config: config, Stage: 10, Profile: profile, Lease: Lease{ID: "test", Profile: profile}, Focus: Event{ID: "focus"}, Candidates: []Event{{ID: "focus"}}}
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			notices := make(chan WorkerNotice)
			usageSeen := make(chan UsageRecord, 1)
			go func() {
				for {
					select {
					case n := <-notices:
						if n.Kind == "model_usage" {
							usageSeen <- n.Payload.(UsageRecord)
						}
						n.Ack <- NoticeAck{Accepted: true}
					case <-ctx.Done():
						return
					}
				}
			}()
			result := NewModelClient().Run(ctx, request, notices)
			var failure *ModelCallError
			if !errors.As(result.Error, &failure) || failure.Fact.Category != tc.category || failure.Fact.RetryAfter != "37" || failure.Fact.RequestID != "quota-test" {
				t.Fatalf("gateway fact changed or lost: %#v error=%v", failure, result.Error)
			}
			select {
			case u := <-usageSeen:
				if u.FailureCategory != tc.category || u.CostConfirmed || u.ActualMicrousd != 0 || u.ReservedMicrousd == 0 {
					t.Fatalf("failure manufactured payment or lost its cause: %#v", u)
				}
			case <-ctx.Done():
				t.Fatal("missing request settlement")
			}
		})
	}
}

func TestGatewayRateAndQuotaPreserveCognitionWithoutModelPromotion(t *testing.T) {
	for _, category := range []string{"rate_limited", "gateway_quota"} {
		t.Run(category, func(t *testing.T) {
			cognizer := &blockingCognizer{started: make(chan CognitiveRequest, 1), release: make(chan struct{})}
			defer close(cognizer.release)
			r, err := New(t.TempDir(), "quota", testConfig(10), cognizer)
			if err != nil {
				t.Fatal(err)
			}
			r.config.GenerationKind = "engineering"
			r.state.Body.NetworkAvailable = true
			now := time.Now().UTC()
			r.state.Background = []Event{{ID: "reality", Kind: "action_result", Status: "in_focus", CognitionAttempts: 2}}
			r.state.Lease = &Lease{ID: "failed", FocusID: "reality", Profile: CognitiveProfile{Model: "terra", ReasoningEffort: "none"}}
			for i := 0; i < 3; i++ {
				r.state.Usage = append(r.state.Usage, UsageRecord{Time: now.Format(time.RFC3339Nano), RequestedModel: "terra", FailureCategory: category, HTTPStatus: 429, RetryAfter: "37"})
			}
			err = r.handleCognitiveResult(context.Background(), CognitiveResult{LeaseID: "failed", FocusID: "reality", Error: &ModelCallError{Fact: ModelFailureFact{Category: category, HTTPStatus: 429}}})
			if err != nil {
				t.Fatal(err)
			}
			gate := gatewayRetry(r.state, r.config.CognitiveResource)
			if gate.allows(now, true) || gate.allows(now, false) || r.state.Lease != nil || len(r.state.CognitiveResource.ProtectedModels) != 0 || r.state.Background[0].CognitionAttempts != 2 || r.state.IntegrityDebt != 0 {
				t.Fatalf("external resource failure became personal failure or another model call: gate=%#v state=%#v", gate, r.state.CognitiveResource)
			}
			if category == "gateway_quota" && gate.Until.Sub(now) != time.Duration(r.config.CognitiveResource.ModelProtectionMinutes)*time.Minute {
				t.Fatalf("confirmed quota exhaustion used rapid transport retries: %#v", gate)
			}
			r.state.Usage = append(r.state.Usage, UsageRecord{Time: nowUTC(), Status: "completed"})
			if !gatewayRetry(r.state, r.config.CognitiveResource).allows(now, false) {
				t.Fatal("successful recovery did not release shared gate")
			}
		})
	}
}

func TestStageTenRecoveryReturnsToMainRole(t *testing.T) {
	r, err := New(t.TempDir(), "roles", testConfig(10), nil)
	if err != nil {
		t.Fatal(err)
	}
	r.state.CognitiveResource.DefaultProfile = CognitiveProfile{Model: "terra", ReasoningEffort: "none"}
	if got, ok := r.recoveryProfile("terra"); ok {
		t.Fatalf("main failure promoted an organ model: %#v", got)
	}
	if got, ok := r.recoveryProfile("sol"); !ok || got != r.state.CognitiveResource.DefaultProfile {
		t.Fatalf("technical assistance did not return to the main role: %#v %v", got, ok)
	}
	r.state.CognitiveResource.ProtectedModels["terra"] = ProtectedModel{Until: time.Now().Add(time.Minute).UTC().Format(time.RFC3339Nano)}
	if _, ok := r.recoveryProfile("sol"); ok {
		t.Fatal("recovery ignored the main profile's protection")
	}
}

func TestGatewayQuotaHoldSurvivesOtherFailuresAndRestart(t *testing.T) {
	root := t.TempDir()
	r, err := New(root, "quota", testConfig(10), nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	r.state.Usage = []UsageRecord{
		{LeaseID: "quota-request", Time: now.Format(time.RFC3339Nano), FailureCategory: "gateway_quota", RetryAfter: "3600"},
		{LeaseID: "late-request", Time: now.Add(time.Second).Format(time.RFC3339Nano), FailureCategory: "transport_error"},
	}
	if err := r.persist(); err != nil {
		t.Fatal(err)
	}
	reloaded, err := New(root, "quota", testConfig(10), nil)
	if err != nil {
		t.Fatal(err)
	}
	gate := gatewayRetry(reloaded.state, reloaded.config.CognitiveResource)
	if gate.Cause != "gateway_quota" || !gate.Until.Equal(now.Add(time.Hour)) || gate.allows(now.Add(20*time.Minute), true) {
		t.Fatalf("late failure/restart forgot shared quota or shortened Retry-After: %#v", gate)
	}
	if !gate.allows(now.Add(time.Hour), true) || gate.allows(now.Add(time.Hour), false) {
		t.Fatal("recovery probe is not exclusive to ordinary main cognition")
	}
}
