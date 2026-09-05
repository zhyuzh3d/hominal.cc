package organ

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRegistryDiscoversAndInvokesAProtocolOrgan(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "body", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "body", "organs"), 0o755); err != nil {
		t.Fatal(err)
	}
	adapter := filepath.Join(root, "body", "bin", "test-organ")
	script := `#!/bin/sh
case "$1" in
describe) printf '%s\n' '{"schema":"hominal.organ-description/v1","id":"test","name":"Test sense","command":"test-organ","capabilities":["observe","orient","perform"],"operations":["touch"],"operation_inputs":{"touch":"{\"target\":\"fact\"}"},"guidance":"test guidance"}' ;;
health) printf '%s\n' '{"schema":"hominal.organ-health/v1","id":"test","status":"ready","accepting":true,"in_flight":0,"queued":0}' ;;
observe) printf '%s\n' '{"schema":"hominal.organ-observation/v1","organ_id":"test","surface_id":"current","observed_at":"2026-09-01T00:00:00Z","context":["room"],"objects":[{"id":"one","content":"a fact"}]}' ;;
orient) printf '%s\n' '{"schema":"hominal.organ-orientation/v1","organ_id":"test","status":"moved","observed_at":"2026-09-01T00:00:01Z","detail":"one step"}' ;;
perform) printf '%s\n' '{"schema":"hominal.organ-action-result/v1","organ_id":"test","action_id":"action-test","status":"completed","effect":"changed","observed_at":"2026-09-01T00:00:02Z","summary":"done","output":"fact"}' ;;
*) exit 2 ;;
esac
`
	if err := os.WriteFile(adapter, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":"hominal.organ-manifest/v1","id":"test","command":"body/bin/test-organ","daemon":false}`
	if err := os.WriteFile(filepath.Join(root, "body", "organs", "test.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}

	registry, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := registry.ObservableIDs(); len(got) != 1 || got[0] != "test" {
		t.Fatalf("observable ids = %#v", got)
	}
	if snapshot := registry.Snapshot()["test"]; snapshot.Status != "ready" || snapshot.Command != "test-organ" {
		t.Fatalf("unexpected body snapshot: %#v", snapshot)
	} else if len(snapshot.Operations) != 1 || snapshot.Operations[0] != "touch" {
		t.Fatalf("body snapshot omitted the action catalog: %#v", snapshot)
	} else if snapshot.OperationInputs["touch"] != `{"target":"fact"}` {
		t.Fatalf("body snapshot omitted the action input contract: %#v", snapshot)
	}
	observation, err := registry.Observe(context.Background(), "test")
	if err != nil {
		t.Fatal(err)
	}
	if len(observation.Objects) != 1 || observation.Objects[0].ID != "one" {
		t.Fatalf("unexpected observation: %#v", observation)
	}
	orientation, err := registry.Orient(context.Background(), "test")
	if err != nil || orientation.Status != "moved" {
		t.Fatalf("unexpected orientation: %#v err=%v", orientation, err)
	}
	performed, err := registry.Perform(context.Background(), "test", ActionRequest{ActionID: "action-test", Operation: "touch", Input: `{}`})
	if err != nil || performed.Status != "completed" || performed.Output != "fact" {
		t.Fatalf("unexpected action result: %#v err=%v", performed, err)
	}
	if _, err := registry.Perform(context.Background(), "test", ActionRequest{ActionID: "invalid-action", Operation: "observe", Input: `{}`}); err == nil {
		t.Fatal("a passive protocol capability was accepted as an intentional action operation")
	}
}

func TestActionOrganWithoutAnOperationCatalogIsUnavailable(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "body", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "body", "organs"), 0o755); err != nil {
		t.Fatal(err)
	}
	adapter := filepath.Join(root, "body", "bin", "ambiguous-organ")
	script := `#!/bin/sh
case "$1" in
describe) printf '%s\n' '{"schema":"hominal.organ-description/v1","id":"ambiguous","name":"Ambiguous","command":"ambiguous-organ","capabilities":["observe","perform"],"guidance":"ambiguous"}' ;;
*) exit 2 ;;
esac
`
	if err := os.WriteFile(adapter, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schema":"hominal.organ-manifest/v1","id":"ambiguous","command":"body/bin/ambiguous-organ","daemon":false}`
	if err := os.WriteFile(filepath.Join(root, "body", "organs", "ambiguous.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := registry.Snapshot()["ambiguous"]
	if snapshot.Status != "unavailable" || snapshot.Accepting {
		t.Fatalf("ambiguous action organ remained callable: %#v", snapshot)
	}
}

func TestInvalidOrganBecomesUnavailableWithoutBreakingTheRegistry(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "body", "organs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "body", "organs", "broken.json"), []byte(`{"schema":`), 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := Load(root)
	if err != nil {
		t.Fatalf("one malformed organ stopped the body registry: %v", err)
	}
	snapshot, exists := registry.Snapshot()["broken"]
	if !exists || snapshot.Status != "unavailable" || snapshot.Accepting {
		t.Fatalf("broken organ was not exposed as an unavailable body fact: %#v", snapshot)
	}
}
