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
