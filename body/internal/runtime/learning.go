package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"
)

// actionProgressBoundary is not a malformed cognitive response. It is a
// factual boundary between intention and embodiment: the proposed request has
// already produced usable Reality and therefore cannot honestly create a new
// causal step. The runtime returns this boundary to attention instead of
// buying repeated validation retries for the same intention.
type actionProgressBoundary struct {
	reason string
}

func (e *actionProgressBoundary) Error() string { return e.reason }

func newActionProgressBoundary(reason string) error {
	return &actionProgressBoundary{reason: reason}
}

const (
	maxCommitments              = 32
	maxMemories                 = 128
	maxMemoryContext            = 5
	maxOperationalMemoryContext = 8
	maxSelfMethods              = 8
	maxSelfMethodBytes          = 512
	maxSelfNarrativeBytes       = 4096
	mentorCausalReplayWindow    = 30 * time.Minute
	settledDifference           = 0.25
)

func (r *Runtime) syncSelfFromFiles() error {
	self, err := r.store.LoadSelf()
	if err != nil {
		return err
	}
	if equalStrings(self.Methods, r.state.Self.Methods) && self.Narrative == r.state.Self.Narrative {
		return nil
	}
	self.UpdatedAt = nowUTC()
	r.state.Self = self
	return r.journal("self_observed", "", map[string]any{
		"method_count":    len(self.Methods),
		"narrative_bytes": len(self.Narrative),
	})
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (r *Runtime) validateRealityUpdates(commit CognitiveCommit) error {
	if r.state.Stage < 5 {
		if len(commit.RealityUpdates) != 0 {
			return errors.New("memory updates become available in stage five")
		}
		return nil
	}
	expected := r.commitmentIDForFocus(commit.FocusID)
	if expected == "" {
		if len(commit.RealityUpdates) != 0 {
			return errors.New("this focus has no completed commitment to assimilate")
		}
		return nil
	}
	if len(commit.RealityUpdates) != 1 {
		return errors.New("a focused action result requires one memory update")
	}
	update := commit.RealityUpdates[0]
	if update.CommitmentID != expected {
		return fmt.Errorf("memory commitment %q does not match reality %q", update.CommitmentID, expected)
	}
	commitment := r.commitmentByID(expected)
	if commitment == nil {
		return fmt.Errorf("commitment %q is unavailable", expected)
	}
	candidate, exists := r.activeCandidates[commit.FocusID]
	if !exists {
		return fmt.Errorf("memory focus %q is unavailable", commit.FocusID)
	}
	switch candidate.Kind {
	case "action_result":
		if commitment.MemoryID != "" || (commitment.Status != "reality_available" && commitment.Status != "reality_unknown") {
			return fmt.Errorf("commitment %q no longer has an unassimilated reality", expected)
		}
	case "mentor_received":
		if commitment.ActionKind != "mentor_send" || commitment.MemoryID == "" || commitment.Status != "assimilated" {
			return fmt.Errorf("mentor feedback for commitment %q arrived before its enacted send reality was assimilated", expected)
		}
		if r.memoryForFocus(commit.FocusID) != nil {
			return fmt.Errorf("mentor feedback %q has already been assimilated", commit.FocusID)
		}
	default:
		return fmt.Errorf("focus kind %q is not an assimilable commitment reality", candidate.Kind)
	}
	if err := validateRealityUpdate(update); err != nil {
		return err
	}
	return nil
}

func validateRealityUpdate(update RealityUpdate) error {
	if strings.TrimSpace(update.Meaning) == "" {
		return errors.New("memory meaning is required")
	}
	if update.PredictionDifference < 0 || update.PredictionDifference > 1 ||
		update.ExperiencedCost < 0 || update.ExperiencedCost > 1 {
		return errors.New("memory unit values must remain within 0..1")
	}
	values := []float64{
		update.Values.Continuance,
		update.Values.Exploration,
		update.Values.Agency,
		update.Values.Vitality,
		update.Values.Relatedness,
		update.Values.Contribution,
		update.Values.SelfEndorsed,
	}
	for _, value := range values {
		if value < -1 || value > 1 {
			return errors.New("memory endogenous values must remain within -1..1")
		}
	}
	switch update.Significance {
	case "ordinary", "reusable", "self_defining":
	default:
		return fmt.Errorf("unknown memory significance %q", update.Significance)
	}
	if len([]rune(update.Meaning)) > 1000 || len([]rune(update.Lesson)) > 1000 ||
		len(update.MethodUpdate) > maxSelfMethodBytes {
		return errors.New("memory update exceeds the compact memory boundary")
	}
	return nil
}

func effectiveMemorySignificance(update RealityUpdate, narrativeUpdated bool) string {
	if narrativeUpdated {
		return "self_defining"
	}
	if strings.TrimSpace(update.MethodUpdate) != "" {
		return "reusable"
	}
	return "ordinary"
}

func (r *Runtime) applyRealityUpdates(commit CognitiveCommit) error {
	if len(commit.RealityUpdates) == 0 {
		return nil
	}
	update := commit.RealityUpdates[0]
	commitment := r.commitmentByID(update.CommitmentID)
	if commitment == nil {
		return fmt.Errorf("commitment %q disappeared before assimilation", update.CommitmentID)
	}
	remainingDifference := 0.0
	resolution := "hold"
	for _, appraisal := range commit.Appraisals {
		if appraisal.CandidateID == commit.FocusID {
			remainingDifference = appraisal.Difference
			resolution = appraisal.Resolution
			break
		}
	}
	memory := Memory{
		ID:                   "memory-" + commit.FocusID,
		CommitmentID:         commitment.ID,
		ConcernID:            commitment.ConcernID,
		FocusID:              commit.FocusID,
		SourceKind:           r.activeCandidates[commit.FocusID].Kind,
		ActionKind:           commitment.ActionKind,
		EnactedRequest:       r.enactedRequestForCommitment(commitment.ID),
		ObservedAt:           nowUTC(),
		PredictionDifference: update.PredictionDifference,
		RemainingDifference:  remainingDifference,
		Meaning:              strings.TrimSpace(update.Meaning),
		Values:               update.Values,
		ExperiencedCost:      update.ExperiencedCost,
		Lesson:               strings.TrimSpace(update.Lesson),
		Significance:         effectiveMemorySignificance(update, strings.TrimSpace(commit.NarrativeUpdate) != ""),
		MethodUpdate:         strings.TrimSpace(update.MethodUpdate),
		MethodSlot:           update.MethodSlot,
		Origin:               "observed",
		SourceRefs:           []string{commit.FocusID},
	}
	r.state.Memories = append(r.state.Memories, memory)
	r.state.TotalMemories++
	r.learnDifferenceFromMemory(*commitment, memory)
	r.satiateLifeValues(memory)
	if len(r.state.Memories) > maxMemories {
		r.state.Memories = append([]Memory(nil), r.state.Memories[len(r.state.Memories)-maxMemories:]...)
	}
	if memory.SourceKind == "action_result" {
		commitment.Status = "assimilated"
		commitment.MemoryID = memory.ID
	}
	for index := range r.state.Concerns {
		if r.state.Concerns[index].CommitmentID == commitment.ID {
			r.state.Concerns[index].CommitmentID = ""
		}
	}

	methodChanged := false
	if memory.MethodUpdate != "" {
		updated := truncate(memory.MethodUpdate, maxSelfMethodBytes)
		methods := append([]string(nil), r.state.Self.Methods...)
		slot := memory.MethodSlot
		if slot >= 0 && slot < len(methods) {
			if methods[slot] != updated {
				methods[slot] = updated
				methodChanged = true
			}
		} else if len(methods) < maxSelfMethods && !methodProposalRedundant(updated, methods) {
			methods = append(methods, updated)
			methodChanged = true
		}
		if methodChanged {
			r.state.Self.Methods = methods
		}
	}
	if methodChanged {
		r.state.Self.UpdatedAt = nowUTC()
		if err := r.store.SaveSelf(r.state.Self); err != nil {
			return err
		}
	}

	previousDebt := r.state.IntegrityDebt
	realityRepair := maxFloat(0, commitment.InitialDifference-memory.RemainingDifference)
	realityFit := 1 - memory.RemainingDifference
	gap := maxFloat(0, resolutionRelief(resolution)-realityFit)
	r.state.IntegrityDebt = clamp01(
		r.config.Dynamics.IntegrityPersistence*previousDebt +
			r.config.Dynamics.IntegrityGapGain*gap -
			r.config.Dynamics.IntegrityRepairGain*realityRepair,
	)
	threshold := r.config.Dynamics.IntegrityMirrorThreshold
	if threshold > 0 && previousDebt < threshold && r.state.IntegrityDebt >= threshold && !r.state.IntegrityMirrorOpen {
		payload, _ := json.Marshal(map[string]any{
			"integrity_debt": r.state.IntegrityDebt,
			"memory_id":      memory.ID,
		})
		if err := r.addEvent(
			"integrity_mirror",
			"endogenous",
			"现实完整性正在请求一次清醒回看：让解释重新贴近已经发生的事实。",
			memory.ID,
			payload,
			true,
		); err != nil {
			return err
		}
		r.state.IntegrityMirrorOpen = true
	} else if r.state.IntegrityDebt < threshold*0.75 {
		r.state.IntegrityMirrorOpen = false
	}
	if err := r.accrueSelfModelTension(memory, strings.TrimSpace(commit.NarrativeUpdate) != ""); err != nil {
		return err
	}
	if err := r.journal("memory_assimilated", memory.ID, map[string]any{
		"memory":           memory,
		"integrity_before": previousDebt,
		"integrity_after":  r.state.IntegrityDebt,
		"reality_repair":   realityRepair,
		"reality_fit":      realityFit,
		"interpretive_gap": gap,
	}); err != nil {
		return err
	}
	batch := learningBatch{Memories: []Memory{memory}}
	if memory.Lesson != "" {
		batch.Experiences = []Experience{{ID: "experience-" + memory.ID, Judgment: memory.Lesson, Context: memory.SourceKind, Evidence: []string{memory.ID}, UpdatedAt: memory.ObservedAt}}
	}
	if err := r.commitLearning(batch); err != nil {
		return err
	}
	if err := r.maybeOpenOperationalSelfDifference(); err != nil {
		return err
	}
	return r.enqueueConcernContribution(strings.TrimSpace(commit.ContributesToConcernID), *commitment, memory)
}

// maybeOpenOperationalSelfDifference lets the existing self-model loop notice
// a whole-life pattern that no single successful action can expose. Several
// ordinary micro-closures can share an action form while producing very
// different content and consequences. Repeated form and expenditure warrant
// a look, not a verdict about yield. AIP interprets the actual results and may
// preserve a useful method, learn a different one, or change its next choice.
func (r *Runtime) maybeOpenOperationalSelfDifference() error {
	if r.state.Stage < 10 || r.selfModelDifferenceCandidateActive() {
		return nil
	}
	baseline := r.selfRegulationBaseline()
	recent := r.settledActionMemoriesAfter(baseline)
	if len(recent) < 4 {
		return nil
	}

	concerns := make(map[string]bool)
	patterns := make(map[string]int)
	ordinary := 0
	contactOnly := 0
	for _, memory := range recent {
		if memory.Significance == "ordinary" {
			ordinary++
		}
		if commitment := r.commitmentByID(memory.CommitmentID); commitment != nil && commitment.ConcernID != "" {
			concerns[commitment.ConcernID] = true
		}
		patterns[memoryActionPattern(memory)]++
		if actionEffectIsContactOnly(r.memoryActionEffect(memory)) {
			contactOnly++
		}
	}
	repeatedPatterns := 0
	dominantPatternCount := 0
	for _, count := range patterns {
		if count >= 2 {
			repeatedPatterns++
		}
		if count > dominantPatternCount {
			dominantPatternCount = count
		}
	}
	spent, calls := r.cognitiveSpendAfter(baseline)
	// These are two observable patterns that can be reviewed. Read-only action
	// says that the organ did not write to the world; it says nothing about the
	// novelty, meaning or future value of what Alice learned from it.
	fragmentedChurn := len(recent) >= 6 && len(concerns) >= 3 &&
		(repeatedPatterns >= 2 || dominantPatternCount >= 3)
	contactChurn := contactOnly >= len(recent)-1 && len(concerns) <= 2 &&
		(repeatedPatterns >= 2 || dominantPatternCount >= 3)
	// Calls and causal outcomes express attention metabolism independently of
	// model price. A fixed USD gate made the same pattern visible after a few
	// main-role thoughts but nearly invisible under the fast role. Confirmed spend remains in
	// the factual payload for Alice to interpret; it does not decide whether her
	// own repeated operating pattern exists.
	if (!fragmentedChurn && !contactChurn) || calls < len(recent) {
		return nil
	}

	evidenceIDs := make([]string, 0, len(recent))
	for index := len(recent) - 1; index >= 0; index-- {
		evidenceIDs = append(evidenceIDs, recent[index].ID)
	}
	r.state.SelfModelTension = maxFloat(r.state.SelfModelTension, r.config.Dynamics.AttentionThreshold)
	payload, _ := json.Marshal(map[string]any{
		"difference_kind":           "attention_yield_balance",
		"current_narrative":         r.state.Self.Narrative,
		"memory_count":              len(recent),
		"ordinary_count":            ordinary,
		"distinct_concern_count":    len(concerns),
		"repeated_action_forms":     patterns,
		"dominant_action_count":     dominantPatternCount,
		"contact_only_action_count": contactOnly,
		"cognition_calls":           calls,
		"cognition_spend_microusd":  spent,
		"affective_state":           r.state.AffectiveState,
		"integrity_debt":            r.state.IntegrityDebt,
		"evidence_memory_ids":       evidenceIDs,
	})
	return r.enqueueSelfModelDifference(
		"self_model_difference",
		"endogenous",
		"近期多次使用了相同动作形式；这些活动的具体内容、现实结果与资源投入可供你判断这段经历的价值。",
		evidenceIDs[len(evidenceIDs)-1],
		payload,
		true,
	)
}

// One enacted commitment contributes one outcome to operational self-sensing.
// Personal recollections, predictions and reinterpretations remain cognitive
// material; multiplying them does not multiply completed body actions. Read
// through the learning index so a rich fragment stream cannot evict the real
// outcomes from this bounded operational window.
func (r *Runtime) settledActionMemoriesAfter(baseline string) []Memory {
	recent := make([]Memory, 0, maxOperationalMemoryContext)
	seen := make(map[string]bool)
	for _, commitment := range r.state.Commitments {
		if commitment.ID == "" || seen[commitment.ID] || commitment.Status != "assimilated" {
			continue
		}
		memory := r.memoryByID(commitment.MemoryID)
		if memory == nil && commitment.MemoryID == "" {
			// Historical snapshots predate the explicit canonical outcome link.
			memory = r.memoryForCommitment(commitment.ID)
		}
		if memory == nil || memory.CommitmentID != commitment.ID || memory.ActionKind == "" ||
			(memory.SourceKind != "" && memory.SourceKind != "action_result") ||
			(memory.Origin != "" && memory.Origin != "observed") || !timeAfter(memory.ObservedAt, baseline) {
			continue
		}
		seen[commitment.ID] = true
		recent = append(recent, *memory)
	}
	sort.SliceStable(recent, func(i, j int) bool { return timeAfter(recent[i].ObservedAt, recent[j].ObservedAt) })
	if len(recent) > maxOperationalMemoryContext {
		recent = recent[:maxOperationalMemoryContext]
	}
	return recent
}

func (r *Runtime) memoryByID(memoryID string) *Memory {
	if r.learning != nil {
		if memory, exists := r.learning.memories[memoryID]; exists {
			return &memory
		}
	}
	for index := len(r.state.Memories) - 1; index >= 0; index-- {
		if r.state.Memories[index].ID == memoryID {
			return &r.state.Memories[index]
		}
	}
	return nil
}

func actionEffectIsContactOnly(effect string) bool {
	// Only an organ-verified changed effect can prove a persistent causal
	// alteration. Observed, oriented, unknown, failed and legacy-empty effects
	// may still yield valuable Memory, but do not by themselves prove a run of
	// contact achieved a persistent change. This is not a repeat-read policy:
	// moving the sensory pose can change what the same read will observe.
	return effect != "changed"
}

func (r *Runtime) memoryActionEffect(memory Memory) string {
	_, action, _, _ := r.realityForCommitment(memory.CommitmentID)
	if action == nil {
		return ""
	}
	return action.Effect
}

// maybeOpenAffectiveSelfDifference exposes sustained high activation with low
// control as an interoceptive fact. Mere non-action is deliberately absent:
// understanding or releasing a real object can be a complete cognition, and a
// counter that demands a new body consequence rewards cheap artifact creation.
// Repetitive enacted behavior is handled separately from actual Memories.
func (r *Runtime) maybeOpenAffectiveSelfDifference() error {
	if r.state.Stage < 10 || r.selfModelDifferenceCandidateActive() || r.state.PendingAction != nil || r.hasCommitmentAwaitingAssimilation() {
		return nil
	}
	threshold := r.config.Dynamics.AttentionThreshold
	affectiveStrain := r.state.AffectiveState.Activation * (1 - r.state.AffectiveState.Control)
	if affectiveStrain >= threshold &&
		attentionDue(r.state.LastAttentionAt, time.Now().UTC(), 60) &&
		!r.selfModelDifferenceObservedWithin(5*time.Minute) {
		r.state.SelfModelTension = maxFloat(r.state.SelfModelTension, threshold)
		payload, _ := json.Marshal(map[string]any{
			"difference_kind":     "affective_control_balance",
			"current_narrative":   r.state.Self.Narrative,
			"affective_state":     r.state.AffectiveState,
			"affective_strain":    affectiveStrain,
			"integrity_debt":      r.state.IntegrityDebt,
			"evidence_memory_ids": []string{},
		})
		return r.enqueueSelfModelDifference(
			"self_model_difference",
			"endogenous",
			"持续的高激活与较低控制感已经形成一次值得自己理解的内在运行差异。",
			"affective-control",
			payload,
			true,
		)
	}
	return nil
}

func (r *Runtime) selfModelDifferenceObservedWithin(window time.Duration) bool {
	cutoff := time.Now().UTC().Add(-window)
	for index := len(r.state.Background) - 1; index >= 0; index-- {
		event := r.state.Background[index]
		if event.Kind != "self_model_difference" {
			continue
		}
		observed, err := time.Parse(time.RFC3339Nano, event.ObservedAt)
		return err == nil && observed.After(cutoff)
	}
	return false
}

func (r *Runtime) selfRegulationBaseline() string {
	baseline := r.state.T0
	for index := len(r.state.Background) - 1; index >= 0; index-- {
		event := r.state.Background[index]
		if event.Kind != "self_model_difference" {
			continue
		}
		var payload struct {
			DifferenceKind string `json:"difference_kind"`
		}
		if json.Unmarshal(event.Payload, &payload) != nil || payload.DifferenceKind != "attention_yield_balance" {
			continue
		}
		if timeAfter(event.ObservedAt, baseline) {
			baseline = event.ObservedAt
		}
		break
	}
	return baseline
}

func (r *Runtime) cognitiveSpendAfter(baseline string) (int64, int) {
	var spent int64
	calls := 0
	for _, usage := range r.state.Usage {
		// A paid response remains metabolic expenditure when its cognitive commit
		// is unusable.  Excluding schema-invalid work made precisely the most
		// wasteful kind of thought invisible to self-regulation.
		if !usage.CostConfirmed || usage.ActualMicrousd <= 0 || !timeAfter(usage.Time, baseline) {
			continue
		}
		spent += usage.ActualMicrousd
		calls++
	}
	return spent, calls
}

func memoryActionPattern(memory Memory) string {
	if memory.ActionKind != "organ_action" {
		return memory.ActionKind
	}
	var request struct {
		OrganID   string `json:"organ_id"`
		Operation string `json:"operation"`
		Input     string `json:"input"`
	}
	if json.Unmarshal([]byte(memory.EnactedRequest), &request) != nil {
		return memory.ActionKind
	}
	pattern := memory.ActionKind + ":" + request.OrganID + ":" + request.Operation
	if request.OrganID != "system" || request.Operation != "exec" {
		return pattern
	}
	leading, _ := leadingShellCommand(request.Input)
	fields := strings.Fields(leading)
	if len(fields) == 0 {
		return pattern
	}
	return pattern + ":" + fields[0]
}

func timeAfter(candidate, baseline string) bool {
	if strings.TrimSpace(candidate) == "" {
		return false
	}
	if strings.TrimSpace(baseline) == "" {
		return true
	}
	candidateTime, candidateErr := time.Parse(time.RFC3339Nano, candidate)
	baselineTime, baselineErr := time.Parse(time.RFC3339Nano, baseline)
	return candidateErr == nil && baselineErr == nil && candidateTime.After(baselineTime)
}

// enqueueConcernContribution gives a real child Memory one bounded route
// back into a broader Concern that Alice selected while assimilating the
// result. It creates no meaning, reward, priority, or prescribed response: the
// new candidate merely makes the factual relationship available to a later
// single attention pulse.
func (r *Runtime) enqueueConcernContribution(parentID string, commitment ActionCommitment, memory Memory) error {
	payload, _ := json.Marshal(map[string]any{
		"memory_id":         memory.ID,
		"commitment_id":     commitment.ID,
		"child_concern_id":  commitment.ConcernID,
		"parent_concern_id": parentID,
	})
	journalPayload := map[string]any{
		"memory_id":         memory.ID,
		"commitment_id":     commitment.ID,
		"child_concern_id":  commitment.ConcernID,
		"parent_concern_id": parentID,
	}
	return r.enqueueConcernContributionFact(
		parentID,
		memory.ID,
		"memory",
		"一项或多项由你自主关联的独立行动已经产生真实 Memory；最新后果现在可以重新参与同一上位 Concern 的判断。",
		payload,
		journalPayload,
	)
}

// enqueueObservedConcernContribution lets an independent Reality fact gain
// causal power over a Concern without losing its own identity or directly
// rewriting the Concern. Alice chooses the link; the kernel only preserves one
// bounded wake-up surface for the target.
func (r *Runtime) enqueueObservedConcernContribution(parentID string, source Event) error {
	payloadObject := map[string]any{
		"source_event_id":   source.ID,
		"source_kind":       source.Kind,
		"source_summary":    truncate(strings.TrimSpace(source.Summary), 4096),
		"parent_concern_id": strings.TrimSpace(parentID),
	}
	if len(source.Payload) > 0 && len(source.Payload) <= 16*1024 {
		var sourcePayload any
		if json.Unmarshal(source.Payload, &sourcePayload) == nil {
			payloadObject["source_payload"] = sourcePayload
		}
	}
	payload, _ := json.Marshal(payloadObject)
	return r.enqueueConcernContributionFact(
		parentID,
		source.ID,
		"observed",
		"一项由你自主关联的现实事实已经出现；这项事实现在可以重新参与原 Concern 对稳定闭合条件的判断。",
		payload,
		payloadObject,
	)
}

func (r *Runtime) enqueueConcernContributionFact(
	parentID string,
	correlationID string,
	source string,
	summary string,
	payload []byte,
	journalPayload map[string]any,
) error {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return nil
	}
	parent := r.concernByID(parentID)
	if parent == nil || parent.Resolution != "hold" || parent.Ownership < r.config.Dynamics.AttentionThreshold {
		return nil
	}
	for index := range r.state.Background {
		existing := &r.state.Background[index]
		if existing.Kind != "concern_contribution" {
			continue
		}
		existingParentID, _ := concernContributionRelation(*existing)
		if existingParentID != parentID {
			continue
		}
		// A contribution is only a wake-up surface for the parent Concern. The
		// Memories remain separate durable facts, while one parent needs only
		// one current signal regardless of how many children advanced. Reusing even
		// a previously processed signal prevents several candidates from mapping to
		// the same Concern in one single-focus pulse.
		existing.CorrelationID = correlationID
		existing.Payload = payload
		existing.ObservedAt = nowUTC()
		if existing.Status != "in_focus" && existing.Status != "model_wait" {
			existing.Status = "pending"
			existing.LastCommitErr = ""
			existing.CognitionAttempts = 0
		}
		return r.journal("concern_contribution_refreshed", existing.ID, journalPayload)
	}
	return r.addEvent(
		"concern_contribution",
		source,
		summary,
		correlationID,
		payload,
		true,
		parentID,
	)
}

func concernContributionRelation(candidate Event) (string, string) {
	if candidate.Kind != "concern_contribution" || len(candidate.Payload) == 0 {
		return "", ""
	}
	var payload struct {
		ParentConcernID string `json:"parent_concern_id"`
		ChildConcernID  string `json:"child_concern_id"`
	}
	_ = json.Unmarshal(candidate.Payload, &payload)
	return payload.ParentConcernID, payload.ChildConcernID
}

func (r *Runtime) validateNarrativeUpdate(commit CognitiveCommit) error {
	update := strings.TrimSpace(commit.NarrativeUpdate)
	if update == "" {
		return nil
	}
	if r.state.Stage < 8 {
		return errors.New("narrative updates become part of the common cognitive commit in stage eight")
	}
	if len([]byte(update)) > maxSelfNarrativeBytes {
		return errors.New("narrative update exceeds the compact self boundary")
	}
	if update == strings.TrimSpace(r.state.Self.Narrative) {
		return errors.New("an unchanged narrative is represented by an empty narrative_update")
	}
	candidate, exists := r.activeCandidates[commit.FocusID]
	if !exists {
		return fmt.Errorf("narrative focus %q is unavailable", commit.FocusID)
	}
	if candidate.Kind == "self_model_difference" {
		if r.state.Stage >= 10 {
			var payload struct {
				DifferenceKind    string   `json:"difference_kind"`
				EvidenceMemoryIDs []string `json:"evidence_memory_ids"`
			}
			if json.Unmarshal(candidate.Payload, &payload) == nil &&
				strings.TrimSpace(payload.DifferenceKind) != "" &&
				len(payload.EvidenceMemoryIDs) == 0 {
				// An operational interoceptive signal is real evidence that
				// something in the current regulation loop deserves attention, but
				// it is not yet lived evidence of how Alice actually regulates it.
				// Let the signal change appraisal, attention or action. Narrative
				// waits for subsequent Reality to become Memory, so paraphrasing
				// "I will wait/change" cannot clear the very gap being observed.
				return errors.New("an operational self-model signal without lived Memory may guide appraisal or action, while Narrative changes after its regulation produces Reality")
			}
		}
		return nil
	}
	if commitmentIDFromEvent(candidate) == "" || len(commit.RealityUpdates) != 1 {
		return errors.New("a narrative update is grounded in an important reality memory or a self-model difference")
	}
	if strings.TrimSpace(r.state.Self.Narrative) != "" {
		return errors.New("an established narrative changes through accumulated self-model difference")
	}
	if absFloat(commit.RealityUpdates[0].Values.SelfEndorsed) < r.config.Dynamics.AttentionThreshold {
		return errors.New("this reality has not been appraised as sufficiently self-relevant for a narrative update")
	}
	return nil
}

func (r *Runtime) enactedRequestForCommitment(commitmentID string) string {
	_, _, _, request := r.realityForCommitment(commitmentID)
	return request
}

func (r *Runtime) applyNarrativeUpdate(commit CognitiveCommit) error {
	update := strings.TrimSpace(commit.NarrativeUpdate)
	if update == "" {
		return nil
	}
	previous := r.state.Self.Narrative
	r.state.Self.Narrative = truncate(update, maxSelfNarrativeBytes)
	r.state.Self.UpdatedAt = nowUTC()
	r.state.SelfModelTension = 0
	if err := r.store.SaveSelf(r.state.Self); err != nil {
		return err
	}
	evidence := make([]string, 0, 4)
	for _, memoryUpdate := range commit.RealityUpdates {
		if memory := r.memoryForFocus(commit.FocusID); memory != nil {
			evidence = append(evidence, memory.ID)
		} else if memory := r.memoryForCommitment(memoryUpdate.CommitmentID); memory != nil {
			evidence = append(evidence, memory.ID)
		}
	}
	if candidate, exists := r.activeCandidates[commit.FocusID]; exists && candidate.Kind == "self_model_difference" {
		var payload struct {
			EvidenceMemoryIDs []string `json:"evidence_memory_ids"`
		}
		if json.Unmarshal(candidate.Payload, &payload) == nil {
			evidence = append(evidence, payload.EvidenceMemoryIDs...)
		}
	}
	return r.journal("self_updated", commit.FocusID, map[string]any{
		"focus_id":            commit.FocusID,
		"evidence_memory_ids": evidence,
		"previous_narrative":  previous,
		"current_narrative":   r.state.Self.Narrative,
	})
}

// accrueSelfModelTension closes the same Difference Field over the current
// self-model. A self-relevant memory contributes in proportion to its
// prediction/reality residue; repeated low-gap memories can accumulate,
// while one surprising memory contributes faster. No semantic conclusion
// is generated here—the threshold only returns grounded evidence to attention.
func (r *Runtime) accrueSelfModelTension(memory Memory, narrativeUpdated bool) error {
	if narrativeUpdated {
		return nil
	}
	selfRelevance := absFloat(memory.Values.SelfEndorsed)
	if selfRelevance == 0 {
		return nil
	}
	difference := maxFloat(memory.PredictionDifference, memory.RemainingDifference)
	contribution := selfRelevance * (r.config.Dynamics.ConcernBaseDrive + r.config.Dynamics.ConcernUrgencyWeight*difference)
	before := r.state.SelfModelTension
	r.state.SelfModelTension = clamp01(before + r.config.Dynamics.ConcernGrowthGain*contribution)
	threshold := r.config.Dynamics.AttentionThreshold
	if threshold <= 0 || r.state.SelfModelTension < threshold || r.selfModelDifferenceCandidateActive() {
		return nil
	}
	evidence := r.recentSelfModelEvidence(4)
	evidenceIDs := make([]string, 0, len(evidence))
	for _, item := range evidence {
		evidenceIDs = append(evidenceIDs, item.ID)
	}
	payload, _ := json.Marshal(map[string]any{
		"before":              before,
		"after":               r.state.SelfModelTension,
		"current_narrative":   r.state.Self.Narrative,
		"evidence":            evidence,
		"evidence_memory_ids": evidenceIDs,
	})
	return r.enqueueSelfModelDifference(
		"self_model_difference",
		"endogenous",
		"近期真实经历已经积累成一次值得理解的自我模型差异。",
		memory.ID,
		payload,
		true,
	)
}

func (r *Runtime) recentSelfModelEvidence(limit int) []Memory {
	if limit <= 0 {
		return nil
	}
	selected := make([]Memory, 0, limit)
	for index := len(r.state.Memories) - 1; index >= 0 && len(selected) < limit; index-- {
		memory := r.state.Memories[index]
		if absFloat(memory.Values.SelfEndorsed) == 0 {
			continue
		}
		selected = append(selected, memory)
	}
	for left, right := 0, len(selected)-1; left < right; left, right = left+1, right-1 {
		selected[left], selected[right] = selected[right], selected[left]
	}
	return selected
}

// selfModelDifferenceCandidateActive prevents duplicate simultaneous attention
// surfaces. A held self-model Concern is intentionally not included: long-lived
// questions about identity or life rhythm must remain able to receive new
// operational evidence. Otherwise the first durable self question would make
// later fatigue, low-yield attention, and contradictory Memory invisible.
func (r *Runtime) selfModelDifferenceCandidateActive() bool {
	for _, event := range r.state.Background {
		if event.Kind == "self_model_difference" && eventChainActive(event.Status) {
			return true
		}
	}
	return false
}

func (r *Runtime) heldSelfModelConcernID() string {
	for index := len(r.state.Concerns) - 1; index >= 0; index-- {
		concern := r.state.Concerns[index]
		if concern.OriginKind == "self_model_difference" && concern.Resolution == "hold" {
			return concern.ID
		}
	}
	return ""
}

// enqueueSelfModelDifference keeps one durable self question while allowing
// later evidence to wake it again. The kernel contributes only current facts;
// AIP still decides whether they confirm, change, or release the Concern.
func (r *Runtime) enqueueSelfModelDifference(kind, source, summary, correlationID string, payload json.RawMessage, candidate bool) error {
	if concernID := r.heldSelfModelConcernID(); concernID != "" {
		return r.addEvent(kind, source, summary, correlationID, payload, candidate, concernID)
	}
	return r.addEvent(kind, source, summary, correlationID, payload, candidate)
}

func (r *Runtime) formActionCommitment(leaseID string, profile CognitiveProfile, commit CognitiveCommit, action *CognitiveAction) error {
	if r.state.Stage < 5 || action == nil || action.Kind == "none" {
		return nil
	}
	concernID := r.focusConcernID(commit.FocusID)
	if open := r.openCommitmentForConcern(concernID); concernID != "" && open != nil {
		return fmt.Errorf("concern %q already has unassimilated commitment %q", concernID, open.ID)
	}
	commitment := ActionCommitment{
		ID:            "commitment-" + randomID(),
		FocusID:       commit.FocusID,
		ConcernID:     concernID,
		LeaseID:       leaseID,
		ActionKind:    action.Kind,
		Intent:        strings.TrimSpace(action.Intent),
		Prediction:    strings.TrimSpace(action.Prediction),
		RealityCheck:  strings.TrimSpace(action.RealityCheck),
		StopCondition: strings.TrimSpace(action.StopCondition),
		Profile:       profile,
		FormedAt:      nowUTC(),
		Status:        "formed",
	}
	for _, appraisal := range commit.Appraisals {
		if appraisal.CandidateID == commit.FocusID {
			commitment.InitialDifference = appraisal.Difference
			break
		}
	}
	action.CommitmentID = commitment.ID
	r.state.Commitments = append(r.state.Commitments, commitment)
	r.state.TotalCommitments++
	if concern := r.concernByID(concernID); concern != nil {
		concern.CommitmentID = commitment.ID
	}
	r.pruneCommitments()
	return r.journal("action_committed", commitment.ID, commitment)
}

func (r *Runtime) pruneCommitments() {
	excess := len(r.state.Commitments) - maxCommitments
	if excess <= 0 {
		return
	}
	kept := r.state.Commitments[:0]
	for _, commitment := range r.state.Commitments {
		// A deferred result can outlive many unrelated actions. Its causal
		// identity is live work, not an old settled item in the history window.
		if excess > 0 && commitment.Status == "assimilated" {
			excess--
			continue
		}
		kept = append(kept, commitment)
	}
	r.state.Commitments = kept
}

// validateActionProgress gives assimilated reality two small, factual
// consequences. A settled request cannot be replayed as apparent progress for
// an unchanged concern, and an exactly repeated failed organ action is
// not a new experiment merely because a different concern names it. Alice still
// chooses the next substantive action and may use any genuinely changed request.
func (r *Runtime) validateActionProgress(focusID string, action CognitiveAction, effectiveConcernIDs ...string) error {
	if err := r.validateMentorCausalNovelty(action, time.Now().UTC()); err != nil {
		return err
	}
	concernID := r.focusConcernID(focusID)
	if len(effectiveConcernIDs) > 0 && strings.TrimSpace(effectiveConcernIDs[0]) != "" {
		concernID = strings.TrimSpace(effectiveConcernIDs[0])
	}
	kind, request := cognitiveActionRequest(action)
	if request == "" {
		return nil
	}
	for index := len(r.state.Commitments) - 1; index >= 0; index-- {
		commitment := r.state.Commitments[index]
		if commitment.Status != "assimilated" || commitment.ActionKind != kind {
			continue
		}
		reality, previousAction, previousKind, previousRequest := r.realityForCommitment(commitment.ID)
		if reality == nil || previousKind != kind || previousRequest != request {
			continue
		}
		memory := r.memoryForCommitment(commitment.ID)
		if memory == nil {
			continue
		}
		// Concern identity cannot manufacture progress, but it also cannot erase
		// a real change that occurred after the earlier request. A later external
		// fact or a genuinely different embodied action may change the conditions
		// under which the same observation is meaningful. Check that causal fact
		// before comparing concern names or treating an earlier failure as final.
		if r.materialRealityAfter(reality.Seq, kind, request) {
			return nil
		}
		if actionRequestFailed(previousAction) {
			return newActionProgressBoundary("the exact organ request already failed or returned an unknown result; inspect reality and form a genuinely changed request")
		}
		if memory.RemainingDifference > settledDifference {
			return nil
		}
		if commitment.ConcernID != concernID {
			return newActionProgressBoundary("the same enacted request already returned a settled reality under another concern; a new concern does not reset embodied memory, so form a genuinely changed request")
		}
		return newActionProgressBoundary("the same enacted request already returned a low-difference reality for this concern, and no new body or world fact has changed its conditions")
	}
	return nil
}

// Only exact recent unsolicited retransmission is mechanically knowable.
// Lexical overlap cannot decide whether a new question or expression matters.
func (r *Runtime) validateMentorCausalNovelty(action CognitiveAction, now time.Time) error {
	if action.Kind != "mentor_send" || strings.TrimSpace(action.ReplyTo) != "" {
		return nil
	}
	for index := len(r.state.Mentor.Outbox) - 1; index >= 0; index-- {
		message := r.state.Mentor.Outbox[index]
		if strings.TrimSpace(message.ReplyTo) != "" {
			continue
		}
		queuedAt, err := time.Parse(time.RFC3339Nano, message.QueuedAt)
		if err == nil && now.Sub(queuedAt) > mentorCausalReplayWindow {
			break
		}
		if strings.TrimSpace(action.Text) == strings.TrimSpace(message.Body) {
			return newActionProgressBoundary("this exact unsolicited message was already sent recently; its delivery remains recorded")
		}
	}
	return nil
}

func cognitiveActionRequest(action CognitiveAction) (string, string) {
	switch action.Kind {
	case "organ_action":
		return action.Kind, normalizedOrganRequest(action.OrganID, action.Operation, action.Input)
	case "mentor_send":
		return action.Kind, normalizeEnactedRequest(action.Kind, action.Text)
	default:
		return action.Kind, ""
	}
}

func (r *Runtime) realityForCommitment(commitmentID string) (*Event, *ActionState, string, string) {
	for index := len(r.state.Background) - 1; index >= 0; index-- {
		event := &r.state.Background[index]
		if event.Kind != "action_result" {
			continue
		}
		var action ActionState
		if json.Unmarshal(event.Payload, &action) != nil || action.CommitmentID != commitmentID {
			continue
		}
		return event, &action, action.Kind, normalizedActionStateRequest(action)
	}
	return nil, nil, "", ""
}

func actionRequestFailed(action *ActionState) bool {
	return action != nil && action.Kind == "organ_action" && (action.Status == "failed" || action.Status == "unknown")
}

func (r *Runtime) memoryForCommitment(commitmentID string) *Memory {
	for index := len(r.state.Memories) - 1; index >= 0; index-- {
		if r.state.Memories[index].CommitmentID == commitmentID {
			return &r.state.Memories[index]
		}
	}
	return nil
}

func (r *Runtime) memoryForFocus(focusID string) *Memory {
	for index := len(r.state.Memories) - 1; index >= 0; index-- {
		if r.state.Memories[index].FocusID == focusID {
			return &r.state.Memories[index]
		}
	}
	return nil
}

func (r *Runtime) materialRealityAfter(seq uint64, repeatedKind, repeatedRequest string) bool {
	for index := range r.state.Background {
		event := r.state.Background[index]
		if event.Seq <= seq {
			continue
		}
		if event.Kind != "action_result" {
			// Internal pressure, reflection and resource bookkeeping can alter
			// attention without changing the reality an earlier observation read.
			// Only a newly observed body/world fact can reopen that settled request.
			switch event.Kind {
			case "mentor_received", "mentor_content", "environment_change", "body_delta", "perceptual_change", "reality_consequence":
				return true
			default:
				continue
			}
		}
		var action ActionState
		if json.Unmarshal(event.Payload, &action) != nil {
			// An undecodable observed result leaves its causal effect unknown.
			return true
		}
		if action.Kind == repeatedKind && normalizedActionStateRequest(action) == repeatedRequest {
			continue
		}
		// A pure read leaves its conditions unchanged. Orientation, a state
		// change or an unknown effect may instead change what the next identical
		// read observes. Permission to inspect these conditions is distinct from
		// claiming valuable or persistent world change; contact-only self-sensing
		// continues to use the stricter actionEffectIsContactOnly distinction.
		if action.Effect != "observed" {
			return true
		}
	}
	return false
}

func normalizedActionStateRequest(action ActionState) string {
	if action.Kind == "organ_action" {
		return normalizedOrganRequest(action.OrganID, action.Operation, action.Request)
	}
	return normalizeEnactedRequest(action.Kind, action.Request)
}

func normalizedOrganRequest(organID, operation, input string) string {
	input = strings.TrimSpace(input)
	if organID == "system" && operation == "exec" {
		input = normalizeEnactedRequest("system_exec", input)
	} else {
		var value any
		if json.Unmarshal([]byte(input), &value) == nil {
			if encoded, err := json.Marshal(value); err == nil {
				input = string(encoded)
			}
		}
	}
	encoded, _ := json.Marshal(struct {
		OrganID   string `json:"organ_id"`
		Operation string `json:"operation"`
		Input     string `json:"input"`
	}{strings.TrimSpace(organID), strings.TrimSpace(operation), input})
	return string(encoded)
}

// normalizeEnactedRequest identifies the embodied operation rather than inert
// shell policy wrappers. Options such as `set -euo pipefail` change failure
// handling but do not create a new contact with reality, so they cannot turn a
// settled request into apparent progress.
func normalizeEnactedRequest(kind, request string) string {
	request = strings.ReplaceAll(request, "\r\n", "\n")
	if kind != "system_exec" {
		return strings.TrimSpace(request)
	}
	request = strings.TrimSpace(request)
	for request != "" {
		leading, rest := leadingShellCommand(request)
		leading = strings.TrimSpace(leading)
		if !isShellPolicyLine(leading) && !isStaticShellDecoration(leading) {
			break
		}
		request = strings.TrimSpace(rest)
	}
	lines := strings.Split(request, "\n")
	for index := range lines {
		lines[index] = strings.TrimSpace(lines[index])
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

// leadingShellCommand isolates only the command prefix used by normalization.
// It respects quotes and escapes so punctuation printed as text is not treated
// as a shell boundary. Once a substantive command begins, the remainder is
// preserved verbatim apart from whitespace normalization.
func leadingShellCommand(request string) (string, string) {
	quote := byte(0)
	escaped := false
	for index := 0; index < len(request); index++ {
		current := request[index]
		if escaped {
			escaped = false
			continue
		}
		if current == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if current == quote {
				quote = 0
			}
			continue
		}
		if current == '\'' || current == '"' {
			quote = current
			continue
		}
		if current == ';' || current == '\n' {
			return request[:index], request[index+1:]
		}
	}
	return request, ""
}

func isStaticShellDecoration(command string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 || (fields[0] != "printf" && fields[0] != "echo") {
		return false
	}
	// Variable expansion, command substitution, redirection, pipelines and
	// control operators can observe or change reality; retain those commands.
	return !strings.ContainsAny(command, "$`><|&")
}

func isShellPolicyLine(line string) bool {
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != "set" || !strings.HasPrefix(fields[1], "-") {
		return false
	}
	if len(fields) == 2 {
		for _, option := range strings.TrimPrefix(fields[1], "-") {
			if !strings.ContainsRune("eux", option) {
				return false
			}
		}
		return true
	}
	return len(fields) == 3 && fields[2] == "pipefail" && (fields[1] == "-o" || strings.Contains(fields[1], "o"))
}

func (r *Runtime) commitmentByID(id string) *ActionCommitment {
	for index := range r.state.Commitments {
		if r.state.Commitments[index].ID == id {
			return &r.state.Commitments[index]
		}
	}
	return nil
}

func (r *Runtime) commitmentIDForFocus(focusID string) string {
	candidate, exists := r.activeCandidates[focusID]
	if !exists {
		return ""
	}
	return commitmentIDFromEvent(candidate)
}

func commitmentIDFromEvent(candidate Event) string {
	if len(candidate.Payload) == 0 {
		return ""
	}
	switch candidate.Kind {
	case "action_result":
		var action ActionState
		if err := json.Unmarshal(candidate.Payload, &action); err == nil {
			return action.CommitmentID
		}
	case "mentor_received":
		var payload struct {
			CommitmentID string `json:"commitment_id"`
		}
		_ = json.Unmarshal(candidate.Payload, &payload)
		return payload.CommitmentID
	}
	return ""
}

func contributedMemoryIDFromEvent(candidate Event) string {
	if candidate.Kind != "concern_contribution" || len(candidate.Payload) == 0 {
		return ""
	}
	var payload struct {
		MemoryID string `json:"memory_id"`
	}
	_ = json.Unmarshal(candidate.Payload, &payload)
	return payload.MemoryID
}

func selectContextMemories(memories []Memory, candidates []Event) []Memory {
	if len(memories) <= maxMemoryContext {
		return append([]Memory(nil), memories...)
	}
	wantedCommitments := make(map[string]bool)
	wantedMemories := make(map[string]bool)
	for _, candidate := range candidates {
		if commitmentID := commitmentIDFromEvent(candidate); commitmentID != "" {
			wantedCommitments[commitmentID] = true
		}
		if memoryID := contributedMemoryIDFromEvent(candidate); memoryID != "" {
			wantedMemories[memoryID] = true
		}
	}
	selected := make([]Memory, 0, maxMemoryContext)
	seen := make(map[string]bool)
	for index := len(memories) - 1; index >= 0; index-- {
		memory := memories[index]
		if (wantedCommitments[memory.CommitmentID] || wantedMemories[memory.ID]) && !seen[memory.ID] {
			selected = append(selected, memory)
			seen[memory.ID] = true
		}
	}
	query := memoryQuery(candidates)
	type scoredMemory struct {
		memory Memory
		score  float64
		index  int
	}
	scored := make([]scoredMemory, 0, len(memories))
	for index, memory := range memories {
		if seen[memory.ID] {
			continue
		}
		// Causal recall needs what the body actually did, not only the meaning
		// later written about it. A delayed regression often names the affected
		// path or setting more precisely than the Memory summary; retaining
		// the enacted request in similarity lets Alice reconnect a present body
		// change to her own earlier intervention without the kernel declaring a
		// cause or choosing a correction.
		text := strings.Join([]string{memory.EnactedRequest, memory.Meaning, memory.Lesson, memory.MethodUpdate}, " ")
		scored = append(scored, scoredMemory{memory: memory, score: memorySimilarity(query, text), index: index})
	}
	sort.SliceStable(scored, func(left, right int) bool {
		if scored[left].score == scored[right].score {
			return scored[left].index > scored[right].index
		}
		return scored[left].score > scored[right].score
	})
	for _, item := range scored {
		if len(selected) >= maxMemoryContext {
			break
		}
		selected = append(selected, item.memory)
		seen[item.memory.ID] = true
	}
	return selected
}

func memoryQuery(candidates []Event) string {
	parts := make([]string, 0, len(candidates)*2)
	for _, candidate := range candidates {
		if candidate.Kind == "value_signal" {
			var p struct {
				Surface string `json:"surface"`
				Meaning string `json:"context_meaning"`
			}
			if json.Unmarshal(candidate.Payload, &p) == nil {
				// Randomly associated history is a cue, not an explicit request
				// to fill the entire recall window with this record's lineage.
				parts = append(parts, p.Surface, p.Meaning)
				continue
			}
		}
		if candidate.Kind == "action_result" {
			var action ActionState
			if json.Unmarshal(candidate.Payload, &action) == nil {
				parts = append(parts, action.Operation, action.Status, recallMaterial(action.Result))
				continue
			}
		}
		parts = append(parts, candidate.Summary, string(candidate.Payload))
	}
	return strings.Join(parts, " ")
}

// Transport envelopes repeat controls and implementation metadata. Recall
// follows the returned material. The complete journal record and the existing
// bounded factual projection are unchanged. This works for any organ.
func recallMaterial(output string) string {
	var envelope struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal([]byte(output), &envelope) == nil && len(envelope.Content) > 0 {
		parts := []string{}
		for _, item := range envelope.Content {
			if item.Type == "text" {
				parts = append(parts, item.Text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	return output
}

func memorySimilarity(left, right string) float64 {
	leftGrams := runeBigrams(left)
	if len(leftGrams) == 0 {
		return 0
	}
	rightGrams := runeBigrams(right)
	common := 0
	for gram := range leftGrams {
		if rightGrams[gram] {
			common++
		}
	}
	return float64(common) / float64(len(leftGrams))
}

func runeBigrams(value string) map[string]bool {
	runes := make([]rune, 0, len(value))
	for _, current := range []rune(strings.ToLower(value)) {
		if unicode.IsLetter(current) || unicode.IsDigit(current) {
			runes = append(runes, current)
		}
	}
	grams := make(map[string]bool)
	if len(runes) == 1 {
		grams[string(runes)] = true
	}
	for index := 0; index+1 < len(runes); index++ {
		grams[string(runes[index:index+2])] = true
	}
	return grams
}

// methodProposalRedundant is a deterministic guard on the scarce durable
// method slots. The main cognition still decides whether an Memory carries
// a method and may explicitly revise one indexed slot. For an unindexed new
// proposal, this guard only rejects strong lexical restatements; the complete
// Memory remains available as evidence, so the kernel does not rewrite or
// reinterpret Alice's meaning.
func methodProposalRedundant(candidate string, methods []string) bool {
	candidateBigrams := runeBigrams(candidate)
	candidateRunes := normalizedRuneSet(candidate)
	for _, method := range methods {
		if strings.TrimSpace(candidate) == strings.TrimSpace(method) {
			return true
		}
		methodBigrams := runeBigrams(method)
		methodRunes := normalizedRuneSet(method)
		bigramCommon := setIntersectionSize(candidateBigrams, methodBigrams)
		bigramTotal := len(candidateBigrams) + len(methodBigrams)
		bigramDice := 0.0
		if bigramTotal > 0 {
			bigramDice = 2 * float64(bigramCommon) / float64(bigramTotal)
		}
		shorterRuneSet := minInt(len(candidateRunes), len(methodRunes))
		runeCoverage := 0.0
		if shorterRuneSet > 0 {
			runeCoverage = float64(setIntersectionSize(candidateRunes, methodRunes)) / float64(shorterRuneSet)
		}
		if bigramDice >= 0.34 || (bigramDice >= 0.20 && runeCoverage >= 0.45) {
			return true
		}
	}
	return false
}

func normalizedRuneSet(value string) map[string]bool {
	result := make(map[string]bool)
	for _, current := range []rune(strings.ToLower(value)) {
		if unicode.IsLetter(current) || unicode.IsDigit(current) {
			result[string(current)] = true
		}
	}
	return result
}

func setIntersectionSize(left, right map[string]bool) int {
	if len(left) > len(right) {
		left, right = right, left
	}
	common := 0
	for item := range left {
		if right[item] {
			common++
		}
	}
	return common
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}
