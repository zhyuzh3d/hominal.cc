package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

// Replay the unchanged event and all learning available at that instant. A
// proper subject already in personal memory must survive incidental overlap
// from English prose, transport metadata and a page's footer.
func TestStage103ArchivedSubjectRecall(t *testing.T) {
	path := os.Getenv("HOMINAL_SUBJECT_RECALL_ARCHIVE")
	if path == "" {
		t.Skip("set HOMINAL_SUBJECT_RECALL_ARCHIVE for the eleventh frozen sample")
	}
	for _, tc := range []struct {
		seq     int
		subject string
	}{{656, "Kirkby Thore"}, {666, "Kirkby Thore"}, {676, "Marion Meadows"}} {
		t.Run(fmt.Sprintf("%s/%d", tc.subject, tc.seq), func(t *testing.T) {
			index, original := archivedRecallAt(t, path, tc.seq)
			for _, seed := range []string{original.Seed, "contrast-one", "contrast-two"} {
				found := index.recall(original.Query, seed)
				if !recallContainsSubject(found, tc.subject) {
					type ranked struct {
						id    string
						score float64
					}
					var scores []ranked
					for id, m := range index.memories {
						scores = append(scores, ranked{id, index.relevance(original.Query, m.Meaning+" "+strings.Join(m.Keywords, " "))})
					}
					sort.Slice(scores, func(i, j int) bool { return scores[i].score > scores[j].score })
					for i, s := range scores {
						if i < 5 || strings.Contains(index.memories[s.id].Meaning, tc.subject) {
							t.Logf("rank=%d score=%.3f %s %s", i+1, s.score, s.id, index.memories[s.id].Meaning)
						}
					}
					t.Fatalf("existing subject %s missing", tc.subject)
				}
				request := CognitiveRequest{Stage: 10, Config: testConfig(10), Profile: CognitiveProfile{Model: "main", ReasoningEffort: "none"}, Lease: Lease{ID: "subject-replay"}, Recall: found}
				var view struct {
					Personal struct {
						Memories []struct {
							Content string `json:"content"`
						} `json:"memories"`
					} `json:"personal_recall"`
				}
				input := isolatedModelInput(t, request)
				if err := json.Unmarshal([]byte(strings.TrimPrefix(input, "当前注意场：\n")), &view); err != nil {
					t.Fatal(err)
				}
				seen := false
				for _, m := range view.Personal.Memories {
					seen = seen || strings.Contains(m.Content, tc.subject)
				}
				if !seen {
					t.Fatal("subject lost between retrieval and model input")
				}
			}
		})
	}
}

func recallContainsSubject(bundle RecallBundle, subject string) bool {
	for _, m := range bundle.Memories {
		if strings.Contains(m.Meaning, subject) {
			return true
		}
	}
	return false
}

func TestRecallRespectsLexicalBoundaries(t *testing.T) {
	index := newLearningIndex()
	index.apply(learningBatch{Memories: []Memory{
		{ID: "target", Meaning: "我记得 Kirkby Thore 的村庄生活", SourceRefs: []string{"place"}},
		{ID: "unrelated", Meaning: "the theory of work by Thor exists", SourceRefs: []string{"other"}},
	}})
	if score := index.relevance("Kirkby Thore", index.memories["unrelated"].Meaning); score != 0 {
		t.Fatalf("sub-word resemblance is not a recalled subject: %f", score)
	}
	for _, query := range []string{"KIRKBY_THORE", "Kirkby Thore", "村庄生活"} {
		found := index.recall(query, "fixed")
		if !recallContainsSubject(found, "Kirkby Thore") {
			t.Fatalf("lost subject for %q", query)
		}
	}
}

func TestPersonalCuesAnchorRecallWithoutInventingIdentity(t *testing.T) {
	for _, subject := range []string{"Marion Meadows", "旧城改造", "X"} {
		t.Run(subject, func(t *testing.T) {
			index := newLearningIndex()
			index.apply(learningBatch{Memories: []Memory{
				{ID: "topic", Meaning: "那次接触让我提出一个具体问题", Keywords: []string{subject}, SourceRefs: []string{"reading"}},
				{ID: "noise", Meaning: "Page URL https information browser current home active body", Keywords: []string{"browser"}, SourceRefs: []string{"other"}},
			}, Experiences: []Experience{{ID: "topic-method", Judgment: "先比较这件事的不同材料", Evidence: []string{"topic"}}}})
			found := index.recall("Page URL https information browser current home active body "+subject, "fixed")
			seen := false
			for _, m := range found.Memories {
				seen = seen || m.ID == "topic"
			}
			if !seen {
				t.Fatal("personally tagged history lost to incidental prose")
			}
			// Tags cue association, not exact addressing: an explicit reference
			// still chooses the requested other record before all associations.
			found = index.recall("noise "+subject, "fixed")
			if len(found.Memories) != 1 || found.Memories[0].ID != "noise" {
				t.Fatal("association displaced explicit identity")
			}
		})
	}
	for _, tc := range []struct {
		query, cue string
		want       bool
	}{
		{"example", "X", false}, {"X 页面", "x", true}, {"Marion_Meadows 的经历", "marion meadows", true},
		{"Marion Meadowson", "Marion Meadows", false}, {"这次旧城改造让我好奇", "旧城改造", true},
	} {
		if got := containsRecallCue(normalizeRecallCue(tc.query), normalizeRecallCue(tc.cue)); got != tc.want {
			t.Fatalf("cue %q in %q: %v", tc.cue, tc.query, got)
		}
	}
}

func TestDifferenceSchemaStatesDirectionAtNumericBoundary(t *testing.T) {
	tool := cognitiveCommitTool(10, []Event{{ID: "result", Kind: "action_result"}}, false, true, true)
	props := tool["parameters"].(map[string]any)["properties"].(map[string]any)
	appraisals := props["appraisals"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	d := appraisals["d"].(map[string]any)
	reality := props["reality_updates"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	prediction := reality["prediction_difference"].(map[string]any)
	for _, field := range []map[string]any{d, prediction} {
		description, _ := field["description"].(string)
		if !strings.Contains(description, "0 表示") || !strings.Contains(description, "1 表示") {
			t.Fatal("numeric scale has no local orientation")
		}
	}
	if strings.Contains(d["description"].(string), "完全符合预测") {
		t.Fatal("remaining concern confused with action prediction")
	}
}
