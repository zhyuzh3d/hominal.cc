package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func TestRecallResolvesReferencesInsideStructuredAndQuotedText(t *testing.T) {
	for _, query := range []string{
		`重复入口 {"evidence_memory_ids":["memory-a","memory-b"]}`,
		"重复入口：‘memory-a’、（memory-b）。",
		"重复入口 `memory-a`,\n\"memory-b\"",
	} {
		index := newLearningIndex()
		index.apply(learningBatch{Memories: []Memory{
			{ID: "memory-a", Meaning: "读到了螃蟹的具体分类", SourceRefs: []string{"crab"}},
			{ID: "memory-b", Meaning: "导师讨论了我的新发现", SourceRefs: []string{"reply"}},
			{ID: "old", Meaning: "重复入口，重复入口，没有内容", SourceRefs: []string{"old"}},
			{ID: "memory-a-extra", Meaning: "重复入口", SourceRefs: []string{"other"}},
		}})
		found := index.recall(query, "fixed")
		if len(found.Memories) != 2 || found.Memories[0].ID != "memory-a" || found.Memories[1].ID != "memory-b" {
			t.Errorf("explicit references became similarity search for %q: %#v", query, found)
		}
	}
}

func TestDirectRecallPrecedesAssociationAndReportsCapacity(t *testing.T) {
	index := newLearningIndex()
	ids := []string{}
	for i := 0; i < 7; i++ {
		id := fmt.Sprintf("memory-action-%d", i)
		ids = append(ids, id)
		index.apply(learningBatch{Memories: []Memory{{ID: id, Meaning: fmt.Sprintf("不同结果 %d", i), Keywords: []string{"共同主题"}, SourceRefs: []string{id}, ObservedAt: fmt.Sprintf("2026-09-04T01:0%d:00Z", i)}}})
	}
	// A later, strongly related interpretation must not displace an expressly
	// selected outcome before the bounded set has been read.
	index.apply(learningBatch{Memories: []Memory{{ID: "later-related", Meaning: "共同主题的新解释", Keywords: []string{"共同主题"}, ObservedAt: "2026-09-04T02:00:00Z"}}})
	payload, _ := json.Marshal(map[string]any{"evidence_memory_ids": ids})
	for _, seed := range []string{"one", "two", "three"} {
		found := index.recall(string(payload), seed)
		if len(found.Memories) != 6 {
			t.Fatalf("explicit outcomes lost bounded capacity: %#v", found)
		}
		for i, m := range found.Memories {
			if m.ID != ids[i] {
				t.Fatalf("reference order/content changed with association or random seed: %#v", found)
			}
		}
		view := recallContext(found)
		deferred, _ := view["deferred_references"].([]string)
		if len(deferred) != 1 || deferred[0] != ids[6] {
			t.Fatalf("partial recall masquerades as complete review: %#v", view)
		}
	}
}

func TestQuotedExperienceStillBringsItsEvidenceAndCorrection(t *testing.T) {
	index := newLearningIndex()
	index.apply(learningBatch{Memories: []Memory{
		{ID: "memory-old", Meaning: "按钮被误认", SourceRefs: []string{"event-old"}},
		{ID: "memory-new", Meaning: "照片纠正了按钮颜色", Corrects: "memory-old", SourceRefs: []string{"memory-old", "photo"}},
	}, Experiences: []Experience{{ID: "experience-rule", Judgment: "检查照片中的按钮", Evidence: []string{"memory-old"}}}})
	for _, query := range []string{`{"evidence":["experience-rule"]}`, `{"evidence":["memory-old"]}`} {
		found := index.recall(query, "fixed")
		if len(found.Memories) != 1 || found.Memories[0].ID != "memory-new" {
			t.Fatalf("direct reference lost accepted correction: %#v", found)
		}
		if _, exists := recallContext(found)["deferred_references"]; exists {
			t.Fatal("a supplied correction was reported as an omitted reference")
		}
	}
}

// Exercise the final HTTP model input, not only index selection. These are
// local fake responses; no external model, life sample or budget is involved.
func TestOperationalRecallEvidenceReachesModelInput(t *testing.T) {
	for _, result := range []string{"读到新的具体材料并形成问题", "再次刷新仍然只有同一加载外壳"} {
		t.Run(result, func(t *testing.T) {
			index := newLearningIndex()
			ids := []string{}
			for i := 0; i < 4; i++ {
				id := fmt.Sprintf("outcome-%d", i)
				ids = append(ids, id)
				index.apply(learningBatch{Memories: []Memory{{ID: id, Meaning: fmt.Sprintf("%s %d", result, i), SourceRefs: []string{id}}}})
			}
			index.apply(learningBatch{Memories: []Memory{{ID: "old-method", Meaning: "近期多次使用相同动作形式，重复核验入口"}}})
			payload, _ := json.Marshal(map[string]any{"evidence_memory_ids": ids, "repeated_action_forms": map[string]int{"browser_snapshot": 4}})
			focus := Event{ID: "review", Kind: "self_model_difference", Summary: "近期多次使用相同动作形式", Payload: payload}
			request := CognitiveRequest{Stage: 10, Focus: focus, Candidates: []Event{focus}, Config: testConfig(10), Profile: CognitiveProfile{Model: "main", ReasoningEffort: "none"}, Lease: Lease{ID: "isolated"}}
			request.Recall = index.recall(memoryQuery(request.Candidates), "fixed")
			input := isolatedModelInput(t, request)
			var view struct {
				Personal map[string]any `json:"personal_recall"`
			}
			if err := json.Unmarshal([]byte(strings.TrimPrefix(input, "当前注意场：\n")), &view); err != nil {
				t.Fatal(err)
			}
			memories, _ := view.Personal["memories"].([]any)
			if len(memories) != 4 {
				t.Fatalf("real HTTP input omitted designated action results: %s", input)
			}
			for i, raw := range memories {
				m := raw.(map[string]any)
				if m["id"] != ids[i] || !strings.Contains(m["content"].(string), result) {
					t.Fatalf("action form replaced its actual consequence: %#v", memories)
				}
			}
		})
	}
}

func TestStage103ArchivedOperationalReferencesReachModel(t *testing.T) {
	path := os.Getenv("HOMINAL_CAUSAL_RECALL_ARCHIVE")
	if path == "" {
		t.Skip("set HOMINAL_CAUSAL_RECALL_ARCHIVE for the ninth frozen sample")
	}
	index, original := archivedRecallAt(t, path, 1046)
	start := strings.Index(original.Query, "{")
	if start < 0 {
		t.Fatal("frozen query has no structured facts")
	}
	focus := Event{ID: "event-000000001042", Kind: "self_model_difference", Summary: strings.TrimSpace(original.Query[:start]), Payload: json.RawMessage(original.Query[start:])}
	var facts struct {
		Evidence []string `json:"evidence_memory_ids"`
	}
	if err := json.Unmarshal(focus.Payload, &facts); err != nil {
		t.Fatal(err)
	}
	if len(facts.Evidence) != 6 {
		t.Fatalf("unexpected frozen case: %#v", facts)
	}
	for _, m := range original.Memories {
		for _, id := range facts.Evidence {
			if m.ID == id {
				t.Fatal("the archived baseline no longer represents the diagnosed omission")
			}
		}
	}
	request := CognitiveRequest{Stage: 10, Focus: focus, Candidates: []Event{focus}, Config: testConfig(10), Profile: CognitiveProfile{Model: "main", ReasoningEffort: "none"}, Lease: Lease{ID: "archive-replay"}}
	request.Recall = index.recall(memoryQuery(request.Candidates), "frozen-replay")
	if len(request.Recall.Memories) != 6 {
		t.Fatalf("causal evidence still displaced: %#v", request.Recall)
	}
	for i, id := range facts.Evidence {
		if request.Recall.Memories[i].ID != id {
			t.Fatalf("wanted %s, got %s", id, request.Recall.Memories[i].ID)
		}
		t.Log(id, request.Recall.Memories[i].Meaning)
	}
	input := isolatedModelInput(t, request)
	var view struct {
		Personal struct {
			Memories []map[string]any `json:"memories"`
		} `json:"personal_recall"`
	}
	if err := json.Unmarshal([]byte(strings.TrimPrefix(input, "当前注意场：\n")), &view); err != nil {
		t.Fatal(err)
	}
	if len(view.Personal.Memories) != 6 {
		t.Fatal("full model input lost selected evidence")
	}
	for i, id := range facts.Evidence {
		if view.Personal.Memories[i]["id"] != id || view.Personal.Memories[i]["content"] != index.memories[id].Meaning {
			t.Fatalf("full model input replaced frozen memory %s", id)
		}
	}
}

func isolatedModelInput(t *testing.T, request CognitiveRequest, inspect ...func(map[string]any)) string {
	t.Helper()
	inputs := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		input, _ := body["input"].(string)
		for _, inspectBody := range inspect {
			inspectBody(body)
		}
		inputs <- input
		arguments, _ := json.Marshal(CognitiveCommit{FocusID: request.Focus.ID, Action: CognitiveAction{Kind: "none"}})
		_ = json.NewEncoder(w).Encode(map[string]any{"model": "gpt-5.6-terra", "output": []map[string]any{{"type": "function_call", "name": "cognitive_commit", "call_id": "test-call", "arguments": string(arguments)}}, "usage": map[string]int{"input_tokens": 1, "output_tokens": 1, "total_tokens": 2}})
	}))
	defer server.Close()
	request.Config.ModelGateway.BaseURL = server.URL
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	notices := make(chan WorkerNotice)
	go func() {
		for {
			select {
			case n := <-notices:
				if n.Ack != nil {
					n.Ack <- NoticeAck{Accepted: true}
				}
			case <-ctx.Done():
				return
			}
		}
	}()
	if result := NewModelClient().Run(ctx, request, notices); result.Error != nil {
		t.Fatal(result.Error)
	}
	select {
	case input := <-inputs:
		return input
	case <-ctx.Done():
		t.Fatal("model input missing")
		return ""
	}
}
