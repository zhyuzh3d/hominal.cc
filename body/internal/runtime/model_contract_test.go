package runtime

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestEveryModelUsesTheSamePortableCognitiveContract(t *testing.T) {
	for _, stage := range []int{4, 8, 9, 10} {
		source := cognitiveCommitTool(stage, []Event{{ID: "event-one", Kind: "action_result"}}, true, true, true)
		if stage == 10 {
			addLearningSchema(source)
		}
		before, _ := json.Marshal(source)
		baseline, choice := cognitiveModelTools(source)
		if choice != "cognitive_commit" || len(baseline) != 1 {
			t.Fatal("unified cognition did not produce one compact function")
		}
		for _, id := range []string{"codex-luna", "codex-terra", "codex-sol", "deepseek-v4-flash", "deepseek-v4-pro"} {
			tools, gotChoice := cognitiveModelTools(source)
			if gotChoice != choice || !reflect.DeepEqual(tools, baseline) {
				t.Fatalf("model %s selected a different wire contract", id)
			}
		}
		original := source["parameters"].(map[string]any)["properties"].(map[string]any)
		bindings := cognitiveMetadataBindings(source)
		for _, tool := range baseline {
			fields := tool["parameters"].(map[string]any)["properties"].(map[string]any)
			if !reflect.DeepEqual(fields["action"], original["action"]) {
				t.Fatal("action semantics changed during portable projection")
			}
			for key, value := range original {
				if _, bound := bindings[key]; bound {
					if _, declared := fields[key]; declared {
						t.Fatalf("fixed metadata still requires generation: %s", key)
					}
				} else if key != "action" && !reflect.DeepEqual(value, fields[key]) {
					t.Fatalf("shared cognitive field changed: %s", key)
				}
			}
			encoded, _ := json.Marshal(tool)
			if tool["strict"] != false || !strings.Contains(string(encoded), `"anyOf"`) {
				t.Fatal("portable function lost the canonical action alternatives")
			}
		}
		after, _ := json.Marshal(source)
		if string(before) != string(after) {
			t.Fatal("portable projection mutated the canonical cognitive contract")
		}
	}
}

func TestUnifiedFunctionsRespectAvailableActionsAndBoundIdentity(t *testing.T) {
	source := map[string]any{
		"type": "function", "name": "cognitive_commit", "strict": true, "description": "test",
		"parameters": map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{
				"focus_id":       map[string]any{"type": "string", "enum": []string{"event-one"}},
				"thought_thread": map[string]any{"type": "string", "pattern": `\S`},
				"action": map[string]any{"anyOf": []any{
					map[string]any{"type": "object", "properties": map[string]any{"kind": map[string]any{"type": "string", "enum": []string{"none"}}}, "required": []string{"kind"}, "additionalProperties": false},
					map[string]any{"type": "object", "properties": map[string]any{"kind": map[string]any{"type": "string", "enum": []string{"mentor_send"}}, "text": map[string]any{"type": "string", "pattern": `\S`}}, "required": []string{"kind", "text"}, "additionalProperties": false},
				}},
			},
			"required": []string{"focus_id", "thought_thread", "action"},
		},
	}
	tools, _ := cognitiveModelTools(source)
	if len(tools) != 1 {
		t.Fatal("unified projection did not produce one function")
	}
	for _, tc := range []struct {
		name, args string
		valid      bool
	}{
		{"cognitive_commit", `{"thought_thread":"clear","action":{"kind":"none"}}`, true},
		{"cognitive_commit", `{"focus_id":"invented","thought_thread":"clear","action":{"kind":"none"}}`, false},
		{"cognitive_commit", `{"thought_thread":" ","action":{"kind":"none"}}`, false},
		{"cognitive_commit", `{"thought_thread":"clear","action":{"kind":"mentor_send","text":"hi"}}`, true},
		{"cognitive_commit", `{"thought_thread":"clear","action":{"kind":"mentor_send","text":""}}`, false},
		{"cognitive_commit", `{"thought_thread":"clear","action":{"kind":"mentor_send"}}`, false},
		{"cognitive_commit", `{"thought_thread":"clear","action":{"kind":"none","text":"unused"}}`, false},
		{"cognitive_commit", `{"thought_thread":"clear","action":{"kind":"none"},"invented":1}`, false},
		{"cognitive_commit", `{"thought_thread":"clear","action":{"kind":"none"}} {}`, false},
	} {
		commit, err := decodeModelCommit(tc.name, tc.args, tools, source)
		if (err == nil) != tc.valid || tc.valid && commit.FocusID != "event-one" {
			t.Fatalf("invalid local decode for %s: %#v, %v", tc.name, commit, err)
		}
	}
}

func TestLocalFunctionValidationCoversHominalSchemaVocabulary(t *testing.T) {
	tool := map[string]any{
		"type": "function", "name": "probe", "strict": false,
		"parameters": map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{
				"kind":  map[string]any{"type": "string", "enum": []string{"probe"}},
				"text":  map[string]any{"type": "string", "pattern": `\S`, "maxLength": 8},
				"score": map[string]any{"type": "number", "minimum": -1, "maximum": 1},
				"count": map[string]any{"type": "integer", "minimum": 1, "maximum": 2},
				"items": map[string]any{"type": "array", "minItems": 1, "maxItems": 1, "items": map[string]any{"type": "boolean"}},
			},
			"required": []string{"kind", "text", "score", "count", "items"},
		},
	}
	valid := &functionCall{Name: "probe", Arguments: `{"kind":"probe","text":"ok","score":0.5,"count":2,"items":[true]}`}
	if err := validateFunctionCall(valid, []map[string]any{tool}); err != nil {
		t.Fatal(err)
	}
	for _, args := range []string{
		`{"kind":"wrong","text":"ok","score":0,"count":1,"items":[true]}`,
		`{"kind":"probe","text":" ","score":0,"count":1,"items":[true]}`,
		`{"kind":"probe","text":"ok","score":2,"count":1,"items":[true]}`,
		`{"kind":"probe","text":"ok","score":0,"count":1.5,"items":[true]}`,
		`{"kind":"probe","text":"ok","score":0,"count":1,"items":[]}`,
		`{"kind":"probe","text":"ok","score":0,"count":1,"items":[true],"extra":1}`,
	} {
		if err := validateFunctionCall(&functionCall{Name: "probe", Arguments: args}, []map[string]any{tool}); err == nil {
			t.Fatalf("local validator accepted %s", args)
		}
	}
}

func TestMetadataBindingNeverChoosesMeaningOrAction(t *testing.T) {
	for _, candidates := range [][]Event{{{ID: "one"}}, {{ID: "one"}, {ID: "two"}}} {
		tool := cognitiveCommitTool(10, candidates, false, true, true)
		addLearningSchema(tool)
		bound := cognitiveMetadataBindings(tool)
		_, focusBound := bound["focus_id"]
		if focusBound != (len(candidates) == 1) {
			t.Fatal("kernel made a multi-candidate focus choice")
		}
		for _, key := range []string{"action", "appraisals", "thought_thread", "resource_choice", "memory_updates", "experience_updates"} {
			if _, exists := bound[key]; exists {
				t.Fatalf("kernel took over model-owned content: %s", key)
			}
		}
	}
}

func TestActualModelIDOnlyChangesTheRequestedResource(t *testing.T) {
	var baseline string
	for _, id := range []string{"codex-terra", "deepseek-v4-flash"} {
		request := CognitiveRequest{Stage: 10, Config: testConfig(10), Profile: CognitiveProfile{Model: "terra", ReasoningEffort: "none"},
			Focus: Event{ID: "e", Kind: "value_signal"}, Candidates: []Event{{ID: "e", Kind: "value_signal"}}, Lease: Lease{ID: "test"}}
		model := request.Config.CognitiveResource.Models["terra"]
		model.ID = id
		request.Config.CognitiveResource.Models["terra"] = model
		isolatedModelInput(t, request, func(body map[string]any) {
			choice, _ := body["tool_choice"].(map[string]any)
			if body["model"] != id || body["parallel_tool_calls"] != false || choice["name"] != "cognitive_commit" {
				t.Error("model resource changed the unified request mechanism")
			}
			copy := map[string]any{"tools": body["tools"], "tool_choice": body["tool_choice"], "parallel_tool_calls": body["parallel_tool_calls"]}
			encoded, _ := json.Marshal(copy)
			if baseline == "" {
				baseline = string(encoded)
			} else if baseline != string(encoded) {
				t.Error("model ID changed the declared function contract")
			}
		})
	}
}

func TestPortableEmptyArraysRetainTheirExactValueSet(t *testing.T) {
	for _, count := range []int{0, 1} {
		ids := []string{}
		if count == 1 {
			ids = []string{"commitment-one"}
		}
		source := map[string]any{"type": "array", "minItems": count, "maxItems": count,
			"items": map[string]any{"type": "object", "properties": map[string]any{"id": map[string]any{"type": "string", "enum": ids}}, "required": []string{"id"}, "additionalProperties": false}}
		before, _ := json.Marshal(source)
		portable := portableFunctionParameters(source)
		if portable["minItems"] != count || portable["maxItems"] != count {
			t.Fatal("empty-array normalization widened accepted values")
		}
		if count == 1 && !reflect.DeepEqual(source, portable) {
			t.Fatal("a live item lost its causal reference constraint")
		}
		if count == 0 && len(portable["items"].(map[string]any)["properties"].(map[string]any)) != 0 {
			t.Fatal("unreachable empty enum reached the provider")
		}
		after, _ := json.Marshal(source)
		if string(before) != string(after) {
			t.Fatal("canonical schema mutated")
		}
	}
}
