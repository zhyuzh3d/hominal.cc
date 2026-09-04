package runtime

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

// Export a stopped sample's pending cognition to a private Lab artifact. This
// calls only the local fake gateway, never a model or an organ, and writes no
// personal state. It is an end-state reproduction, not an exact request trace.
func TestExportStoppedCognitiveContract(t *testing.T) {
	archive, output, focusID := os.Getenv("HOMINAL_CONTRACT_ARCHIVE"), os.Getenv("HOMINAL_CONTRACT_OUTPUT"), os.Getenv("HOMINAL_CONTRACT_FOCUS")
	if archive == "" || output == "" || focusID == "" {
		t.Skip("set archive, output and focus for an isolated contract reproduction")
	}
	f, err := os.Open(archive)
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
	var state State
	var config Config
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		var target any
		if strings.HasSuffix(h.Name, "/state/current.json") {
			target = &state
		}
		if strings.HasSuffix(h.Name, "/birth/runtime.public.json") {
			target = &config
		}
		if target != nil {
			if err := json.NewDecoder(tr).Decode(target); err != nil {
				t.Fatal(err)
			}
		}
	}
	var focus Event
	for _, event := range state.Background {
		if event.ID == focusID {
			focus = event
		}
	}
	if focus.ID == "" || config.Stage < 10 {
		t.Fatal("stopped focus/config unavailable")
	}
	profile := state.CognitiveResource.DefaultProfile
	config.ModelGateway.Adapter = "openai" // fake gateway does not settle real bills
	config.ModelGateway.APIKey = "isolated-probe"
	request := CognitiveRequest{Stage: state.Stage, Config: config, State: state, Focus: focus, Candidates: []Event{focus}, Profile: profile,
		Lease: Lease{ID: "isolated-contract-probe", FocusID: focus.ID, Profile: profile, ProfileSource: "default"}}
	index := newLearningIndex()
	index.apply(learningBatch{Memories: state.Memories, Experiences: state.Experiences})
	request.Recall = index.recall(memoryQuery(request.Candidates), "isolated-contract-probe")
	isolatedModelInput(t, request, func(body map[string]any) {
		data, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			t.Error(err)
			return
		}
		if err := os.WriteFile(output, data, 0600); err != nil {
			t.Error(err)
		}
	})
}
