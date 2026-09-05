package runtime

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// The journal is authoritative. This compact index is rebuilt once at wake-up,
// then updated with committed learning; it is never a second editable truth.
type learningIndex struct {
	memories    map[string]Memory
	experiences map[string]Experience
	terms       map[string]map[string]bool
	corrections map[string]string
	order       []string
}

type learningBatch struct {
	Memories    []Memory     `json:"memories,omitempty"`
	Experiences []Experience `json:"experiences,omitempty"`
}

func newLearningIndex() *learningIndex {
	return &learningIndex{memories: map[string]Memory{}, experiences: map[string]Experience{}, terms: map[string]map[string]bool{}, corrections: map[string]string{}}
}

func (index *learningIndex) apply(batch learningBatch) {
	for _, m := range batch.Memories {
		if m.ID == "" {
			continue
		}
		if _, exists := index.memories[m.ID]; !exists {
			index.order = append(index.order, m.ID)
		}
		index.memories[m.ID] = m
		if m.Corrects != "" {
			index.corrections[m.Corrects] = m.ID
		}
		index.indexTerms(m.ID, m.Meaning+" "+strings.Join(m.Keywords, " "), m.Keywords...)
	}
	for _, e := range batch.Experiences {
		if e.ID == "" {
			continue
		}
		if previous, exists := index.experiences[e.ID]; exists {
			for term := range recallTerms(previous.Judgment + " " + previous.Context) {
				delete(index.terms[term], e.ID)
			}
		}
		index.experiences[e.ID] = e
		index.indexTerms(e.ID, e.Judgment+" "+e.Context)
	}
}

func (index *learningIndex) indexTerms(id, content string, keywords ...string) {
	terms := recallTerms(content)
	for _, keyword := range keywords {
		if key := normalizeRecallCue(keyword); key != "" {
			terms["cue:"+key] = true
		}
	}
	for term := range terms {
		if index.terms[term] == nil {
			index.terms[term] = map[string]bool{}
		}
		index.terms[term][id] = true
	}
}

func (s *Store) loadLearning(state State) (*learningIndex, error) {
	index := newLearningIndex()
	index.apply(learningBatch{Memories: state.Memories, Experiences: state.Experiences})
	file, err := os.Open(s.journalPath)
	if errors.Is(err, os.ErrNotExist) {
		index.importHistoricalLessons()
		return index, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		var row struct {
			Kind    string          `json:"kind"`
			Payload json.RawMessage `json:"payload"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
			return nil, err
		}
		switch row.Kind {
		case "learning_committed":
			var batch learningBatch
			if err := json.Unmarshal(row.Payload, &batch); err != nil {
				return nil, err
			}
			index.apply(batch)
		case "memory_assimilated", "experience_assimilated":
			var payload struct {
				Memory Memory `json:"memory"`
				Legacy Memory `json:"experience"`
			}
			if err := json.Unmarshal(row.Payload, &payload); err != nil {
				return nil, err
			}
			m := payload.Memory
			if m.ID == "" {
				m = payload.Legacy
			}
			if m.ID != "" {
				index.apply(learningBatch{Memories: []Memory{m}})
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	index.importHistoricalLessons()
	sort.SliceStable(index.order, func(i, j int) bool {
		a, b := index.memories[index.order[i]], index.memories[index.order[j]]
		if a.ObservedAt == b.ObservedAt {
			return a.ID < b.ID
		}
		return a.ObservedAt < b.ObservedAt
	})
	return index, nil
}

func (index *learningIndex) importHistoricalLessons() {
	// Imported historical lessons remain conditional judgments with their real
	// episode as evidence; contemporary updates in the journal take precedence.
	for _, m := range index.memories {
		if strings.TrimSpace(m.Lesson) == "" {
			continue
		}
		id := "experience-" + m.ID
		if _, exists := index.experiences[id]; !exists {
			index.apply(learningBatch{Experiences: []Experience{{ID: id, Judgment: m.Lesson, Context: m.SourceKind, Evidence: []string{m.ID}, UpdatedAt: m.ObservedAt}}})
		}
	}
}

func (r *Runtime) commitLearning(batch learningBatch) error {
	if len(batch.Memories)+len(batch.Experiences) == 0 {
		return nil
	}
	if err := r.journal("learning_committed", r.state.CurrentFocus, batch); err != nil {
		return err
	}
	if r.learning == nil {
		r.learning = newLearningIndex()
	}
	for _, m := range r.state.Memories {
		if _, exists := r.learning.memories[m.ID]; !exists {
			r.learning.apply(learningBatch{Memories: []Memory{m}})
		}
	}
	r.learning.apply(batch)
	r.refreshLearningWindow()
	return nil
}

func (r *Runtime) refreshLearningWindow() {
	if r.learning == nil {
		return
	}
	start := len(r.learning.order) - maxMemories
	if start < 0 {
		start = 0
	}
	r.state.Memories = nil
	for _, id := range r.learning.order[start:] {
		r.state.Memories = append(r.state.Memories, r.learning.memories[id])
	}
	r.state.TotalMemories = uint64(len(r.learning.memories))
	r.state.Experiences = nil
	for _, e := range r.learning.experiences {
		r.state.Experiences = append(r.state.Experiences, e)
	}
	sort.Slice(r.state.Experiences, func(i, j int) bool {
		a, b := r.state.Experiences[i], r.state.Experiences[j]
		if a.UpdatedAt == b.UpdatedAt {
			return a.ID < b.ID
		}
		return a.UpdatedAt < b.UpdatedAt
	})
	if len(r.state.Experiences) > maxMemories {
		r.state.Experiences = r.state.Experiences[len(r.state.Experiences)-maxMemories:]
	}
	r.state.LearningVersion = 2
}

func (r *Runtime) validateLearningUpdates(commit CognitiveCommit) error {
	if len(commit.MemoryUpdates) > 3 || len(commit.ExperienceUpdates) > 2 || len([]rune(commit.RecallQuery)) > 160 {
		return errors.New("learning update exceeds the compact boundary")
	}
	known := func(id string) bool {
		if _, ok := r.activeCandidates[id]; ok {
			return true
		}
		if r.learning != nil {
			_, ok := r.learning.memories[id]
			return ok
		}
		return false
	}
	for _, m := range commit.MemoryUpdates {
		if strings.TrimSpace(m.Content) == "" || len([]rune(m.Content)) > 500 || len(m.Keywords) > 12 || len(m.SourceRefs) > 8 {
			return errors.New("memory needs compact content and source references")
		}
		switch m.Origin {
		case "observed", "remembered", "predicted", "imagined", "reflected":
		default:
			return errors.New("unknown memory origin")
		}
		for _, ref := range m.SourceRefs {
			if !known(ref) {
				return fmt.Errorf("memory source %q is unavailable", ref)
			}
		}
		if m.Corrects != "" {
			if r.learning == nil {
				return errors.New("correction has no earlier memory")
			}
			if _, ok := r.learning.memories[m.Corrects]; !ok {
				return fmt.Errorf("memory %q to correct is unavailable", m.Corrects)
			}
		}
	}
	for _, e := range commit.ExperienceUpdates {
		if strings.TrimSpace(e.Judgment) == "" || len([]rune(e.Judgment)) > 500 || len([]rune(e.Context)) > 300 || len(e.Evidence) == 0 || len(e.Evidence) > 8 {
			return errors.New("experience needs a compact judgment, context and memory evidence")
		}
		if e.ID != "" {
			if r.learning == nil {
				return errors.New("experience unavailable")
			}
			if _, ok := r.learning.experiences[e.ID]; !ok {
				return fmt.Errorf("experience %q unavailable", e.ID)
			}
		}
		for _, ref := range e.Evidence {
			if strings.HasPrefix(ref, "new:") {
				n, err := strconv.Atoi(strings.TrimPrefix(ref, "new:"))
				if err == nil && n >= 0 && n < len(commit.MemoryUpdates) {
					continue
				}
			}
			if r.learning != nil {
				if _, ok := r.learning.memories[ref]; ok {
					continue
				}
			}
			return fmt.Errorf("experience evidence %q must name a memory", ref)
		}
	}
	return nil
}

// Keep independently valid projections of the same cognitive result. Memory
// remains one ordered block: filtering individual entries would silently change
// what an Experience's new:n evidence refers to. Experiences depend on that
// block or existing Memory; recall has no write dependency at all.
func (r *Runtime) retainValidLearningUpdates(commit *CognitiveCommit) string {
	if r.validateLearningUpdates(*commit) == nil {
		return ""
	}
	var rejected []string
	if err := r.validateLearningUpdates(CognitiveCommit{MemoryUpdates: commit.MemoryUpdates}); err != nil {
		rejected = append(rejected, "memory_updates: "+err.Error())
		commit.MemoryUpdates = nil
	}
	if len(commit.ExperienceUpdates) > 2 {
		rejected = append(rejected, "experience_updates: at most two updates fit one attention pulse")
		commit.ExperienceUpdates = nil
	} else {
		var valid []ExperienceUpdate
		for i, update := range commit.ExperienceUpdates {
			if err := r.validateLearningUpdates(CognitiveCommit{MemoryUpdates: commit.MemoryUpdates, ExperienceUpdates: []ExperienceUpdate{update}}); err != nil {
				rejected = append(rejected, fmt.Sprintf("experience_updates[%d]: %s", i, err))
			} else {
				valid = append(valid, update)
			}
		}
		commit.ExperienceUpdates = valid
	}
	if len([]rune(commit.RecallQuery)) > 160 {
		rejected = append(rejected, "recall_query: at most 160 characters fit one recall cue")
		commit.RecallQuery = ""
	}
	return strings.Join(rejected, "; ") + "; independently valid learning and recall were retained"
}

func (r *Runtime) applyLearningUpdates(commit CognitiveCommit) error {
	batch := learningBatch{}
	owner := commit.FocusID
	if r.state.Lease != nil {
		owner = r.state.Lease.ID
	}
	now := nowUTC()
	for i, update := range commit.MemoryUpdates {
		refs := append([]string(nil), update.SourceRefs...)
		if len(refs) == 0 {
			refs = []string{commit.FocusID}
		}
		batch.Memories = append(batch.Memories, Memory{ID: fmt.Sprintf("memory-%s-%d", owner, i), FocusID: commit.FocusID, SourceKind: r.activeCandidates[commit.FocusID].Kind, ObservedAt: now, Meaning: strings.TrimSpace(update.Content), Origin: update.Origin, SourceRefs: refs, Keywords: update.Keywords, Corrects: update.Corrects, Significance: "ordinary"})
	}
	for i, update := range commit.ExperienceUpdates {
		id := update.ID
		if id == "" {
			id = fmt.Sprintf("experience-%s-%d", owner, i)
		}
		evidence := append([]string(nil), update.Evidence...)
		for n, ref := range evidence {
			if strings.HasPrefix(ref, "new:") {
				i, _ := strconv.Atoi(strings.TrimPrefix(ref, "new:"))
				evidence[n] = batch.Memories[i].ID
			}
		}
		batch.Experiences = append(batch.Experiences, Experience{ID: id, Judgment: strings.TrimSpace(update.Judgment), Context: strings.TrimSpace(update.Context), Evidence: evidence, UpdatedAt: now})
	}
	if err := r.commitLearning(batch); err != nil {
		return err
	}
	if query := strings.TrimSpace(commit.RecallQuery); query != "" {
		payload, _ := json.Marshal(map[string]any{"recall_query": query, "previous_focus": commit.FocusID})
		return r.addEvent("memory_recall", "remembered", query, commit.FocusID, payload, true)
	}
	return nil
}

func (index *learningIndex) roots(id string, visited map[string]bool) []string {
	if visited[id] {
		return nil
	}
	visited[id] = true
	m, ok := index.memories[id]
	if !ok {
		return []string{id}
	}
	refs := m.SourceRefs
	if len(refs) == 0 {
		refs = []string{m.FocusID}
		if m.FocusID == "" {
			refs = []string{m.ID}
		}
	}
	var roots []string
	for _, ref := range refs {
		if ref == id {
			roots = append(roots, ref)
		} else {
			roots = append(roots, index.roots(ref, visited)...)
		}
	}
	sort.Strings(roots)
	return roots
}

func (index *learningIndex) recall(query, seed string) RecallBundle {
	bundle := RecallBundle{Query: query, Seed: seed}
	if index == nil {
		return bundle
	}
	// Candidate facts contain compact JSON, while deliberate recall may quote
	// an ID in prose. Resolve complete tokens by identity in both cases; an ID
	// is not a lexical hint. Keep request order and exact matching.
	references := []string{}
	referenced := map[string]bool{}
	for _, id := range strings.FieldsFunc(query, func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("\"'`\\，。、,:：;；()（）[]{}<>“”‘’", r)
	}) {
		_, memory := index.memories[id]
		_, experience := index.experiences[id]
		if (memory || experience) && !referenced[id] {
			references = append(references, id)
			referenced[id] = true
		}
	}
	seen := map[string]bool{}
	rootsSeen := map[string]bool{}
	experienceRootsSeen := map[string]bool{}
	memoryLimit := 5
	if len(references) > 0 {
		// Direct evidence may use all six existing total slots instead of
		// reserving a slot for an unrequested Experience.
		memoryLimit = 6
	}
	resolvedMemoryID := func(id string) string {
		for n := 0; n < 8 && index.corrections[id] != ""; n++ {
			id = index.corrections[id]
		}
		return id
	}
	appendMemory := func(id string, causal bool) bool {
		id = resolvedMemoryID(id)
		m, ok := index.memories[id]
		if !ok || seen[id] || len(bundle.Memories) >= memoryLimit || len(bundle.Memories)+len(bundle.Experiences) >= 6 {
			return false
		}
		roots := strings.Join(index.roots(id, map[string]bool{}), "|")
		if !causal && rootsSeen[roots] {
			return false
		}
		rootsSeen[roots] = true
		seen[id] = true
		bundle.Memories = append(bundle.Memories, m)
		return true
	}
	timelineIncluded := false
	appendTimeline := func(anchor Memory) {
		if timelineIncluded {
			return
		}
		// A relevant old interpretation is not the entire current situation.
		// Spend up to two existing slots on later related material, retaining
		// its original time/origin. This is comparison, not supersession or a
		// kernel judgment that the newer account is true.
		added := 0
		for _, m := range index.laterRelatedMemories(anchor) {
			if appendMemory(m.ID, false) {
				added++
			}
			if added == 2 {
				break
			}
		}
		timelineIncluded = added > 0
	}
	addMemory := func(id string, causal bool) {
		if appendMemory(id, causal) {
			appendTimeline(bundle.Memories[len(bundle.Memories)-1])
		}
	}
	if len(references) > 0 {
		// First supply explicitly selected records. Associations and experience
		// evidence use the remaining slots, never displacing requested outcomes.
		for _, id := range references {
			if e, ok := index.experiences[id]; ok {
				if len(bundle.Experiences) < 3 && len(bundle.Memories)+len(bundle.Experiences) < 6 {
					bundle.Experiences = append(bundle.Experiences, e)
					seen[id] = true
				}
			} else {
				appendMemory(id, true)
			}
		}
		for _, e := range bundle.Experiences {
			for _, id := range e.Evidence {
				appendMemory(id, true)
			}
		}
		for _, m := range bundle.Memories {
			appendTimeline(m)
		}
		for _, id := range references {
			if !seen[id] && !seen[resolvedMemoryID(id)] {
				bundle.DeferredReferences = append(bundle.DeferredReferences, id)
			}
		}
		return bundle
	}

	hits := map[string]bool{}
	for term := range recallTerms(query) {
		for id := range index.terms[term] {
			hits[id] = true
		}
	}
	type scored struct {
		id    string
		score float64
		cue   float64
	}
	queryCue := normalizeRecallCue(query)
	// The author's own tags are explicit association cues, not extra prose.
	// The most discriminating matching cue anchors ordinary recall; broad
	// lexical similarity fills out the association. No topic names are supplied
	// by the kernel, and an Experience inherits only its own evidence's cues.
	cueScore := func(keywords []string) float64 {
		best := 0.0
		for _, raw := range keywords {
			key := normalizeRecallCue(raw)
			if key != "" && containsRecallCue(queryCue, key) {
				weight := math.Log1p(float64(len(index.memories)) / float64(1+len(index.terms["cue:"+key])))
				best = maxFloat(best, weight)
			}
		}
		return best
	}
	var items []scored
	for id := range hits {
		text := ""
		cue := 0.0
		if m, ok := index.memories[id]; ok {
			text = m.Meaning + " " + strings.Join(m.Keywords, " ")
			cue = cueScore(m.Keywords)
		}
		if e, ok := index.experiences[id]; ok {
			text = e.Judgment + " " + e.Context
			for _, evidence := range e.Evidence {
				cue = maxFloat(cue, cueScore(index.memories[evidence].Keywords))
			}
		}
		if relevance := index.relevance(query, text); relevance > 0 {
			items = append(items, scored{id, relevance, cue})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].cue != items[j].cue {
			return items[i].cue > items[j].cue
		}
		if items[i].score == items[j].score {
			return items[i].id < items[j].id
		}
		return items[i].score > items[j].score
	})
	// Random variation belongs to association, not to addressing evidence.
	if len(items) > 4 && seed != "" {
		n := 4 + int(sha256.Sum256([]byte(seed + "|recall"))[0])%minInt(len(items)-4, 8)
		items[4], items[n] = items[n], items[4]
	}
	for _, item := range items {
		if e, ok := index.experiences[item.id]; ok {
			roots := []string{}
			visited := map[string]bool{}
			for _, id := range e.Evidence {
				roots = append(roots, index.roots(id, visited)...)
			}
			sort.Strings(roots)
			rootKey := strings.Join(roots, "|")
			if experienceRootsSeen[rootKey] {
				continue
			}
			if len(bundle.Experiences) < 3 {
				experienceRootsSeen[rootKey] = true
				bundle.Experiences = append(bundle.Experiences, e)
				for _, id := range e.Evidence {
					addMemory(id, true)
				}
			}
		} else {
			addMemory(item.id, false)
		}
		if len(bundle.Memories)+len(bundle.Experiences) >= 6 {
			break
		}
	}
	return bundle
}

// Exact causal identities are strongest. Shared keywords are only an
// association cue: it does not assert that two events have the same cause.
// Reuse the existing index/history; no editable "current truth" is created.
func (index *learningIndex) laterRelatedMemories(anchor Memory) []Memory {
	anchorTime, err := time.Parse(time.RFC3339Nano, anchor.ObservedAt)
	if err != nil {
		return nil
	}
	weights := map[string]float64{}
	keys := []string{}
	containsKeyword := func(m Memory, key string) bool {
		return strings.Contains(strings.ToLower(m.Meaning+" "+strings.Join(m.Keywords, " ")), key)
	}
	for _, raw := range anchor.Keywords {
		key := strings.ToLower(strings.TrimSpace(raw))
		if len([]rune(key)) < 2 {
			continue
		}
		if _, exists := weights[key]; exists {
			continue
		}
		n := 0
		for _, m := range index.memories {
			if containsKeyword(m, key) {
				n++
			}
		}
		weights[key] = math.Log1p(float64(len(index.memories)) / float64(1+n))
		keys = append(keys, key)
	}
	sort.Strings(keys)
	type related struct {
		memory Memory
		at     time.Time
		causal bool
		score  float64
	}
	var candidates []related
	for _, m := range index.memories {
		at, err := time.Parse(time.RFC3339Nano, m.ObservedAt)
		if err != nil || !at.After(anchorTime) || index.corrections[m.ID] != "" {
			continue
		}
		causal := anchor.CommitmentID != "" && m.CommitmentID == anchor.CommitmentID
		for _, ref := range m.SourceRefs {
			causal = causal || ref == anchor.ID
		}
		score := 0.0
		for _, key := range keys {
			if containsKeyword(m, key) {
				score += weights[key]
			}
		}
		if causal || score > 0 {
			candidates = append(candidates, related{m, at, causal, score})
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]
		if a.causal != b.causal {
			return a.causal
		}
		if a.score != b.score {
			return a.score > b.score
		}
		// Later observed accounts help compare reality with reflection, while
		// their labels remain explicit and do not certify their truth.
		if (a.memory.Origin == "observed") != (b.memory.Origin == "observed") {
			return a.memory.Origin == "observed"
		}
		if !a.at.Equal(b.at) {
			return a.at.After(b.at)
		}
		return a.memory.ID < b.memory.ID
	})
	result := make([]Memory, 0, len(candidates))
	for _, item := range candidates {
		result = append(result, item.memory)
	}
	return result
}

// Local lexical relevance discounts language shared by almost every memory.
// It is a retrieval prior, not a judgment of personal meaning or truth.
func (index *learningIndex) relevance(query, content string) float64 {
	queryTerms := recallTerms(query)
	documents := float64(len(index.memories) + len(index.experiences))
	match, norm := 0.0, 0.0
	for term := range recallTerms(content) {
		weight := math.Log1p(documents / float64(1+len(index.terms[term])))
		norm += weight * weight
		if queryTerms[term] {
			match += weight * weight
		}
	}
	if norm == 0 {
		return 0
	}
	return match / math.Sqrt(norm)
}

// Preserve word boundaries in spaced scripts. Character pairs across English
// words turn long unrelated prose into a match for nearly every personal
// subject. Unsegmented CJK text keeps local pairs; neither punctuation nor a
// script boundary invents a new lexical association. This representation is
// shared by indexing, scoring and index updates, without changing general
// dynamics similarity or adding a semantic model call.
func recallTerms(value string) map[string]bool {
	terms := map[string]bool{}
	run := []rune{}
	cjk := false
	flush := func() {
		if len(run) == 0 {
			return
		}
		if cjk && len(run) > 1 {
			for i := 1; i < len(run); i++ {
				terms[string(run[i-1:i+1])] = true
			}
		} else {
			terms[string(run)] = true
		}
		run = run[:0]
	}
	for _, r := range strings.ToLower(value) {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			flush()
			continue
		}
		isCJK := unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
		if len(run) > 0 && cjk != isCJK {
			flush()
		}
		cjk = isCJK
		run = append(run, r)
	}
	flush()
	return terms
}

func normalizeRecallCue(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(value, "_", " ")), " "))
}

func containsRecallCue(query, cue string) bool {
	if cue == "" {
		return false
	}
	// CJK labels may naturally occur inside an unsegmented sentence. Spaced
	// scripts must match complete words: X is not a cue in "example".
	runes := []rune(cue)
	wordRune := func(r rune) bool {
		return (unicode.IsLetter(r) || unicode.IsDigit(r)) && !unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
	}
	for offset := 0; offset < len(query); {
		at := strings.Index(query[offset:], cue)
		if at < 0 {
			return false
		}
		at += offset
		left, right := []rune(query[:at]), []rune(query[at+len(cue):])
		if (len(left) == 0 || !wordRune(runes[0]) || !wordRune(left[len(left)-1])) &&
			(len(right) == 0 || !wordRune(runes[len(runes)-1]) || !wordRune(right[0])) {
			return true
		}
		offset = at + len(cue)
	}
	return false
}

func recallContext(bundle RecallBundle) map[string]any {
	memories := make([]map[string]any, 0, len(bundle.Memories))
	for _, m := range bundle.Memories {
		memories = append(memories, map[string]any{"id": m.ID, "time": m.ObservedAt, "content": m.Meaning, "origin": m.Origin, "sources": m.SourceRefs, "keywords": m.Keywords, "corrects": m.Corrects})
	}
	view := map[string]any{"query": bundle.Query, "memories": memories, "experiences": bundle.Experiences}
	if len(bundle.DeferredReferences) > 0 {
		view["deferred_references"] = bundle.DeferredReferences
	}
	return view
}

func addLearningSchema(tool map[string]any) {
	parameters := tool["parameters"].(map[string]any)
	properties := parameters["properties"].(map[string]any)
	// Reality settles consequences. Personal judgments have one write path:
	// Experience, rather than a parallel legacy lesson/method projection.
	if reality, ok := properties["reality_updates"].(map[string]any); ok {
		item := reality["items"].(map[string]any)
		fields := item["properties"].(map[string]any)
		removed := map[string]bool{"lesson": true, "method_update": true, "method_slot": true}
		for field := range removed {
			delete(fields, field)
		}
		required := []string{}
		for _, field := range item["required"].([]string) {
			if !removed[field] {
				required = append(required, field)
			}
		}
		item["required"] = required
	}
	text := map[string]any{"type": "string"}
	stringsSchema := map[string]any{"type": "array", "items": text, "maxItems": 8}
	properties["memory_updates"] = map[string]any{
		"type": "array", "maxItems": 3, "description": "本轮愿意记住的具体片段；没有需要保存的内容时使用空数组。",
		"items": map[string]any{"type": "object", "properties": map[string]any{
			"content":  map[string]any{"type": "string", "maxLength": 500},
			"origin":   map[string]any{"type": "string", "enum": []string{"observed", "remembered", "predicted", "imagined", "reflected"}},
			"keywords": stringsSchema, "source_refs": stringsSchema, "corrects": text,
		}, "required": []string{"content", "origin", "keywords", "source_refs", "corrects"}, "additionalProperties": false},
	}
	properties["experience_updates"] = map[string]any{
		"type": "array", "maxItems": 2, "description": "以具体记忆为依据形成或修订当前判断；没有更新时使用空数组。",
		"items": map[string]any{"type": "object", "properties": map[string]any{
			"id": text, "judgment": map[string]any{"type": "string", "maxLength": 500}, "context": map[string]any{"type": "string", "maxLength": 300}, "evidence": stringsSchema,
		}, "required": []string{"id", "judgment", "context", "evidence"}, "additionalProperties": false},
	}
	properties["recall_query"] = map[string]any{"type": "string", "maxLength": 160, "description": "想进一步回忆的具体线索；无需展开时留空。"}
	parameters["required"] = append(parameters["required"].([]string), "memory_updates", "experience_updates", "recall_query")
}

// Use the actual small context to offer valid references at generation time,
// rather than paying for invented IDs and rejecting them after generation.
// Validation against the authoritative index remains unchanged.
func addLearningReferences(tool map[string]any, request CognitiveRequest) {
	properties := tool["parameters"].(map[string]any)["properties"].(map[string]any)
	memories, experiences, sources := []string{}, []string{""}, []string{}
	for _, m := range request.Recall.Memories {
		memories = append(memories, m.ID)
	}
	for _, e := range request.Recall.Experiences {
		experiences = append(experiences, e.ID)
	}
	for _, c := range request.Candidates {
		sources = append(sources, c.ID)
	}
	sources = append(sources, memories...)
	refs := func(ids []string) map[string]any { return map[string]any{"type": "string", "enum": ids} }
	fields := properties["memory_updates"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	if len(sources) > 0 {
		fields["source_refs"] = map[string]any{"type": "array", "maxItems": 8, "items": refs(sources)}
	}
	fields["corrects"] = refs(append([]string{""}, memories...))
	efields := properties["experience_updates"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	efields["id"] = refs(experiences)
	efields["evidence"] = map[string]any{"type": "array", "maxItems": 8, "minItems": 1, "items": refs(append(append([]string{}, memories...), "new:0", "new:1", "new:2")), "description": "已有Memory ID或本轮实际新片段new:n；new:n须对应存在的memory_updates条目。"}
}
