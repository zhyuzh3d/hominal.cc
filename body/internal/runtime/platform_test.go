package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStage20UsesExplicitCoreAndRealPlatform(t *testing.T) {
	cfg := testConfig(20)
	if _, err := New(t.TempDir(), "s20-test", cfg, nil); err == nil {
		t.Fatal("implicit cognitive core accepted")
	}
	cfg.CognitiveCore = "continuous-v1"
	cfg.Platform = PlatformConfig{Hostname: "a1x-test", OS: "Bazzite", LifeRoot: "/independent/life", Service: "hominal20-life"}
	r, err := New(t.TempDir(), "s20-test", cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(currentSituation(CognitiveRequest{Stage: 20, Config: cfg, State: r.state}))
	for _, legacy := range []string{"hominal-ThinkCentre", "@hominal_cc", "/agent/lives", "root 身份"} {
		if strings.Contains(string(raw), legacy) {
			t.Fatalf("legacy body claim leaked: %s", legacy)
		}
	}
	if !strings.Contains(string(raw), "/independent/life") {
		t.Fatal("actual life directory missing")
	}
}

func TestStage20SharesBudgetAcrossNewIndividuals(t *testing.T) {
	ledger := filepath.Join(t.TempDir(), "rolling.jsonl")
	t.Setenv("HOMINAL_RESOURCE_LEDGER", ledger)
	one, _ := NewStore(t.TempDir())
	at := time.Now().UTC().Format(time.RFC3339Nano)
	if err := one.AppendUsage(UsageRecord{CallID: "paid-1", LeaseID: "preflight-1", Time: at, ActualMicrousd: 1234, CostConfirmed: true}); err != nil {
		t.Fatal(err)
	}
	cfg := testConfig(20)
	cfg.CognitiveCore = "continuous-v1"
	two, err := New(t.TempDir(), "second-individual", cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if spendInWindow(two.state.Usage, time.Hour, time.Now()) != 1234 {
		t.Fatal("new individual bypassed rolling ledger")
	}
}

func TestStage20UnsettledRequestSurvivesIndividualReplacement(t *testing.T) {
	t.Setenv("HOMINAL_RESOURCE_LEDGER", filepath.Join(t.TempDir(), "shared.jsonl"))
	cfg := testConfig(20)
	cfg.CognitiveCore = "continuous-v1"
	first, err := New(t.TempDir(), "first", cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	profile := cfg.CognitiveResource.InitialDefaultProfile
	first.state.Lease = &Lease{ID: "lease-one", Profile: profile}
	ack := make(chan NoticeAck, 1)
	err = first.handleModelNotice(WorkerNotice{Kind: "model_reserve", CallID: "physical-one", LeaseID: "lease-one", Ack: ack,
		Payload: ModelReservation{Profile: profile, ReservedMicrousd: 100000}})
	if err != nil || !(<-ack).Accepted {
		t.Fatalf("reservation failed: %v", err)
	}
	if spendInWindow(first.state.Usage, time.Hour, time.Now()) != 0 {
		t.Fatal("live reservation double counted as spend")
	}
	second, err := New(t.TempDir(), "second", cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if spendInWindow(second.state.Usage, time.Hour, time.Now()) != 100000 {
		t.Fatal("new sample bypassed an unconfirmed request")
	}
	err = first.handleModelNotice(WorkerNotice{Kind: "model_usage", CallID: "physical-one", LeaseID: "lease-one", Ack: ack,
		Payload: UsageRecord{CallID: "physical-one", LeaseID: "lease-one", Time: nowUTC(), ReservedMicrousd: 100000, ActualMicrousd: 321, CostConfirmed: true, Status: "completed"}})
	if err != nil || !(<-ack).Accepted {
		t.Fatalf("settlement failed: %v", err)
	}
	third, err := New(t.TempDir(), "third", cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if spendInWindow(third.state.Usage, time.Hour, time.Now()) != 321 {
		t.Fatal("settlement did not replace conservative reservation")
	}
}

func TestDesktopUserMentorAccessNeedsNoHominalAccount(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("ordinary-user contract")
	}
	dir := filepath.Join(t.TempDir(), "mentor")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := setRuntimeDirectoryAccess(dir); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "socket-fixture")
	os.WriteFile(p, []byte{}, 0644)
	if err := setSocketAccess(p); err != nil {
		t.Fatal(err)
	}
	for _, v := range []struct {
		p    string
		mode os.FileMode
	}{{dir, 0700}, {p, 0600}} {
		info, _ := os.Stat(v.p)
		if info.Mode().Perm() != v.mode {
			t.Fatal("cross-user mentor access exposed")
		}
	}
}

func TestStage20AffordancesRequireAnExplicitAccessibleSurface(t *testing.T) {
	cfg := testConfig(20)
	cfg.CognitiveCore = "continuous-v1"
	cfg.Platform.Surfaces = []PlatformSurface{{ID: "workbench", OrganID: "desktop", Description: "local visible material", Supports: []string{"exploration", "vitality"}}}
	r, err := New(t.TempDir(), "scoped", cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.state.Body = BodySnapshot{DesktopAvailable: true, WechatRunning: true, Organs: map[string]OrganSnapshot{
		"desktop": {Accepting: true, Status: "ready", Capabilities: []string{"desktop_ui", "public_web"}},
	}}
	got := r.lifeValueAffordances("exploration")
	if len(got) != 1 || got[0].Key != "surface:desktop:workbench" {
		t.Fatalf("host process became an ungranted surface: %#v", got)
	}
	r.config.Platform.Surfaces = nil
	if len(r.lifeValueAffordances("exploration")) != 0 {
		t.Fatal("generic desktop capability invented an accessible app")
	}
}

func TestStage20AbsorbsPriorStepWithoutClosingItsNextChosenAction(t *testing.T) {
	root := t.TempDir()
	installTestSystemOrgan(t, root)
	cfg := testConfig(20)
	cfg.CognitiveCore = "continuous-v1"
	r, err := New(root, "serial-visual", cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	r.state.Concerns = []Concern{{ID: "read", Resolution: "hold", ClosureCondition: "资料已实际读到", Ownership: .9, Difference: .6}}
	prior := ActionCommitment{ID: "locate", ConcernID: "read", ActionKind: "organ_action", Status: "reality_available"}
	r.state.Commitments = []ActionCommitment{prior}
	payload, _ := json.Marshal(ActionState{ID: "located", CommitmentID: prior.ID, Kind: "organ_action", Status: "completed", Effect: "observed", Result: "target found"})
	e := Event{ID: "returned", Kind: "action_result", ConcernID: "read", Source: "observed", Payload: payload, Status: "in_focus"}
	r.state.Background = []Event{e}
	r.activeCandidates = map[string]Event{e.ID: e}
	c := CognitiveCommit{FocusID: e.ID, ThoughtThread: "位置已找到，继续读取内容。",
		Appraisals:     []CandidateAppraisal{{CandidateID: e.ID, Meaning: "定位完成，材料尚未读到", Difference: .4, Ownership: .9, Value: .8, Urgency: .5, Answerability: .9, Certainty: .9, Resolution: "resolved"}},
		Action:         CognitiveAction{Kind: "organ_action", OrganID: "system", Operation: "exec", Input: "cat /proc/version", Intent: "读取内容", Prediction: "内容返回", RealityCheck: "核对输出", StopCondition: "返回后停止"},
		ResourceChoice: CognitiveResourceChoice{Apply: "keep", Model: "current", ReasoningEffort: "current"},
		RealityUpdates: []RealityUpdate{{CommitmentID: prior.ID, PredictionDifference: .01, Meaning: "位置已实际取得", Significance: "ordinary"}}}
	if err := r.applyCognitiveCommit(c); err != nil {
		t.Fatal(err)
	}
	if r.state.Commitments[0].Status != "assimilated" || len(r.state.Memories) == 0 {
		t.Fatal("valid previous Reality was discarded")
	}
	if err := r.formActionCommitment("next", cfg.CognitiveResource.InitialDefaultProfile, c, &c.Action); err != nil {
		t.Fatal(err)
	}
	if r.state.Concerns[0].Resolution != "hold" || len(r.state.Commitments) != 2 {
		t.Fatalf("next chosen action lost its open concern: %#v", r.state.Commitments)
	}
	// Closure without another action remains the individual's decision.
	e = Event{ID: "read", Kind: "concern", ConcernID: "read", Status: "in_focus"}
	r.activeCandidates = map[string]Event{e.ID: e}
	r.state.Commitments[1].Status = "assimilated"
	c.FocusID = e.ID
	c.Appraisals[0].CandidateID = e.ID
	c.Appraisals[0].Difference = 0
	c.Appraisals[0].Resolution = "resolved"
	c.Appraisals[0].Meaning = "资料已读到"
	c.Action = CognitiveAction{Kind: "none"}
	c.RealityUpdates = nil
	if err := r.applyCognitiveCommit(c); err != nil {
		t.Fatal(err)
	}
	if remaining := r.concernByID("read"); remaining != nil && remaining.Resolution != "resolved" {
		t.Fatal("direct closure was overridden")
	}
}
