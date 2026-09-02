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

const (
	maxCommitments                  = 32
	maxExperiences                  = 128
	maxExperienceContext            = 5
	maxOperationalExperienceContext = 8
	maxSelfMethods                  = 8
	maxSelfMethodBytes              = 512
	maxSelfNarrativeBytes           = 4096
	settledDifference               = 0.25
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

func (r *Runtime) validateExperienceUpdates(commit CognitiveCommit) error {
	if r.state.Stage < 5 {
		if len(commit.ExperienceUpdates) != 0 {
			return errors.New("experience updates become available in stage five")
		}
		return nil
	}
	expected := r.commitmentIDForFocus(commit.FocusID)
	if expected == "" {
		if len(commit.ExperienceUpdates) != 0 {
			return errors.New("this focus has no completed commitment to assimilate")
		}
		return nil
	}
	if len(commit.ExperienceUpdates) != 1 {
		return errors.New("a focused action result requires one experience update")
	}
	update := commit.ExperienceUpdates[0]
	if update.CommitmentID != expected {
		return fmt.Errorf("experience commitment %q does not match reality %q", update.CommitmentID, expected)
	}
	commitment := r.commitmentByID(expected)
	if commitment == nil {
		return fmt.Errorf("commitment %q is unavailable", expected)
	}
	candidate, exists := r.activeCandidates[commit.FocusID]
	if !exists {
		return fmt.Errorf("experience focus %q is unavailable", commit.FocusID)
	}
	switch candidate.Kind {
	case "action_result":
		if commitment.ExperienceID != "" || (commitment.Status != "reality_available" && commitment.Status != "reality_unknown") {
			return fmt.Errorf("commitment %q no longer has an unassimilated reality", expected)
		}
	case "mentor_received":
		if commitment.ActionKind != "mentor_send" || commitment.ExperienceID == "" || commitment.Status != "assimilated" {
			return fmt.Errorf("mentor feedback for commitment %q arrived before its enacted send reality was assimilated", expected)
		}
		if r.experienceForFocus(commit.FocusID) != nil {
			return fmt.Errorf("mentor feedback %q has already been assimilated", commit.FocusID)
		}
	default:
		return fmt.Errorf("focus kind %q is not an assimilable commitment reality", candidate.Kind)
	}
	if err := validateExperienceUpdate(update); err != nil {
		return err
	}
	return nil
}

func validateExperienceUpdate(update ExperienceUpdate) error {
	if strings.TrimSpace(update.Meaning) == "" {
		return errors.New("experience meaning is required")
	}
	if update.PredictionDifference < 0 || update.PredictionDifference > 1 ||
		update.ExperiencedCost < 0 || update.ExperiencedCost > 1 {
		return errors.New("experience unit values must remain within 0..1")
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
			return errors.New("experience endogenous values must remain within -1..1")
		}
	}
	switch update.Significance {
	case "ordinary", "reusable", "self_defining":
	default:
		return fmt.Errorf("unknown experience significance %q", update.Significance)
	}
	if len([]rune(update.Meaning)) > 1000 || len([]rune(update.Lesson)) > 1000 ||
		len(update.MethodUpdate) > maxSelfMethodBytes {
		return errors.New("experience update exceeds the compact memory boundary")
	}
	return nil
}

func effectiveExperienceSignificance(update ExperienceUpdate, narrativeUpdated bool) string {
	if narrativeUpdated {
		return "self_defining"
	}
	if strings.TrimSpace(update.MethodUpdate) != "" {
		return "reusable"
	}
	return "ordinary"
}

func (r *Runtime) applyExperienceUpdates(commit CognitiveCommit) error {
	if len(commit.ExperienceUpdates) == 0 {
		return nil
	}
	update := commit.ExperienceUpdates[0]
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
	experience := Experience{
		ID:                   "experience-" + randomID(),
		CommitmentID:         commitment.ID,
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
		Significance:         effectiveExperienceSignificance(update, strings.TrimSpace(commit.NarrativeUpdate) != ""),
		MethodUpdate:         strings.TrimSpace(update.MethodUpdate),
		MethodSlot:           update.MethodSlot,
	}
	r.state.Experiences = append(r.state.Experiences, experience)
	r.state.TotalExperiences++
	r.learnDifferenceFromExperience(*commitment, experience)
	r.satiateLifeValues(experience)
	if len(r.state.Experiences) > maxExperiences {
		r.state.Experiences = append([]Experience(nil), r.state.Experiences[len(r.state.Experiences)-maxExperiences:]...)
	}
	if experience.SourceKind == "action_result" {
		commitment.Status = "assimilated"
		commitment.ExperienceID = experience.ID
	}
	for index := range r.state.Concerns {
		if r.state.Concerns[index].CommitmentID == commitment.ID {
			r.state.Concerns[index].CommitmentID = ""
		}
	}

	methodChanged := false
	if experience.MethodUpdate != "" {
		methods := make([]string, 0, maxSelfMethods)
		for _, method := range r.state.Self.Methods {
			if method != experience.MethodUpdate {
				methods = append(methods, method)
			}
		}
		updated := truncate(experience.MethodUpdate, maxSelfMethodBytes)
		if len(methods) < maxSelfMethods {
			methods = append(methods, updated)
			methodChanged = true
		} else {
			if experience.MethodSlot >= 0 && experience.MethodSlot < len(methods) {
				methods[experience.MethodSlot] = updated
				methodChanged = true
			}
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
	realityRepair := maxFloat(0, commitment.InitialDifference-experience.RemainingDifference)
	realityFit := 1 - experience.RemainingDifference
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
			"experience_id":  experience.ID,
		})
		if err := r.addEvent(
			"integrity_mirror",
			"endogenous",
			"现实完整性正在请求一次清醒回看：让解释重新贴近已经发生的事实。",
			experience.ID,
			payload,
			true,
		); err != nil {
			return err
		}
		r.state.IntegrityMirrorOpen = true
	} else if r.state.IntegrityDebt < threshold*0.75 {
		r.state.IntegrityMirrorOpen = false
	}
	if err := r.accrueSelfModelTension(experience, strings.TrimSpace(commit.NarrativeUpdate) != ""); err != nil {
		return err
	}
	if err := r.journal("experience_assimilated", experience.ID, map[string]any{
		"experience":       experience,
		"integrity_before": previousDebt,
		"integrity_after":  r.state.IntegrityDebt,
		"reality_repair":   realityRepair,
		"reality_fit":      realityFit,
		"interpretive_gap": gap,
	}); err != nil {
		return err
	}
	if err := r.maybeOpenOperationalSelfDifference(); err != nil {
		return err
	}
	return r.enqueueConcernContribution(strings.TrimSpace(commit.ContributesToConcernID), *commitment, experience)
}

// maybeOpenOperationalSelfDifference lets the existing self-model loop notice
// a whole-life pattern that no single successful action can expose. Several
// ordinary micro-closures can each return real facts while collectively
// spending attention on the same small causal forms without producing a new
// method or a changed self-understanding. The kernel reports only those
// embodied facts. AIP still decides whether the pattern is valuable work,
// fatigue, caution, avoidance, or something else, and what—if anything—to do.
func (r *Runtime) maybeOpenOperationalSelfDifference() error {
	if r.state.Stage < 10 || r.selfModelDifferenceCandidateActive() {
		return nil
	}
	baseline := r.selfRegulationBaseline()
	recent := make([]Experience, 0, maxOperationalExperienceContext)
	for index := len(r.state.Experiences) - 1; index >= 0 && len(recent) < maxOperationalExperienceContext; index-- {
		experience := r.state.Experiences[index]
		if !timeAfter(experience.ObservedAt, baseline) {
			break
		}
		recent = append(recent, experience)
	}
	if len(recent) < 6 {
		return nil
	}

	concerns := make(map[string]bool)
	patterns := make(map[string]int)
	ordinary := 0
	for _, experience := range recent {
		if experience.Significance == "ordinary" {
			ordinary++
		}
		if commitment := r.commitmentByID(experience.CommitmentID); commitment != nil && commitment.ConcernID != "" {
			concerns[commitment.ConcernID] = true
		}
		patterns[experienceActionPattern(experience)]++
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
	minimumSpend := r.config.CognitiveResource.RollingHourLimitMicrousd / 25
	// One dominant causal form repeated three times is already an embodied
	// pattern, even when the surrounding shell verbs vary. Requiring two
	// separately repeated command shapes missed narrow loops made of create,
	// inspect and restate operations over the same small surface.
	if len(concerns) < 3 || ordinary < len(recent)-1 || (repeatedPatterns < 2 && dominantPatternCount < 3) || calls < len(recent) || spent < minimumSpend {
		return nil
	}

	evidenceIDs := make([]string, 0, len(recent))
	for index := len(recent) - 1; index >= 0; index-- {
		evidenceIDs = append(evidenceIDs, recent[index].ID)
	}
	r.state.SelfModelTension = maxFloat(r.state.SelfModelTension, r.config.Dynamics.AttentionThreshold)
	payload, _ := json.Marshal(map[string]any{
		"difference_kind":          "attention_yield_balance",
		"current_narrative":        r.state.Self.Narrative,
		"experience_count":         len(recent),
		"ordinary_count":           ordinary,
		"distinct_concern_count":   len(concerns),
		"repeated_action_forms":    patterns,
		"dominant_action_count":    dominantPatternCount,
		"cognition_calls":          calls,
		"cognition_spend_microusd": spent,
		"affective_state":          r.state.AffectiveState,
		"integrity_debt":           r.state.IntegrityDebt,
		"evidence_experience_ids":  evidenceIDs,
	})
	return r.enqueueSelfModelDifference(
		"self_model_difference",
		"endogenous",
		"近期多项已完成活动的注意投入、现实结果与持久改变之间形成了一次值得理解的自我运行差异。",
		evidenceIDs[len(evidenceIDs)-1],
		payload,
		true,
	)
}

// maybeOpenAffectiveSelfDifference exposes sustained high activation with low
// control as an interoceptive fact. Mere non-action is deliberately absent:
// understanding or releasing a real object can be a complete cognition, and a
// counter that demands a new body consequence rewards cheap artifact creation.
// Repetitive enacted behavior is handled separately from actual Experiences.
func (r *Runtime) maybeOpenAffectiveSelfDifference() error {
	if r.state.Stage < 10 || r.selfModelDifferenceCandidateActive() || r.state.PendingAction != nil || r.hasUnassimilatedCommitment() {
		return nil
	}
	threshold := r.config.Dynamics.AttentionThreshold
	affectiveStrain := r.state.AffectiveState.Activation * (1 - r.state.AffectiveState.Control)
	if affectiveStrain >= threshold &&
		attentionDue(r.state.LastAttentionAt, time.Now().UTC(), 60) &&
		!r.selfModelDifferenceObservedWithin(5*time.Minute) {
		r.state.SelfModelTension = maxFloat(r.state.SelfModelTension, threshold)
		payload, _ := json.Marshal(map[string]any{
			"difference_kind":         "affective_control_balance",
			"current_narrative":       r.state.Self.Narrative,
			"affective_state":         r.state.AffectiveState,
			"affective_strain":        affectiveStrain,
			"integrity_debt":          r.state.IntegrityDebt,
			"evidence_experience_ids": []string{},
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
	if timeAfter(r.state.Self.UpdatedAt, baseline) {
		baseline = r.state.Self.UpdatedAt
	}
	for index := len(r.state.Background) - 1; index >= 0; index-- {
		event := r.state.Background[index]
		if event.Kind != "self_model_difference" {
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

func experienceActionPattern(experience Experience) string {
	if experience.ActionKind != "organ_action" {
		return experience.ActionKind
	}
	var request struct {
		OrganID   string `json:"organ_id"`
		Operation string `json:"operation"`
		Input     string `json:"input"`
	}
	if json.Unmarshal([]byte(experience.EnactedRequest), &request) != nil {
		return experience.ActionKind
	}
	pattern := experience.ActionKind + ":" + request.OrganID + ":" + request.Operation
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

// enqueueConcernContribution gives a real child Experience one bounded route
// back into a broader Concern that Alice selected while assimilating the
// result. It creates no meaning, reward, priority, or prescribed response: the
// new candidate merely makes the factual relationship available to a later
// single attention pulse.
func (r *Runtime) enqueueConcernContribution(parentID string, commitment ActionCommitment, experience Experience) error {
	payload, _ := json.Marshal(map[string]any{
		"experience_id":     experience.ID,
		"commitment_id":     commitment.ID,
		"child_concern_id":  commitment.ConcernID,
		"parent_concern_id": parentID,
	})
	journalPayload := map[string]any{
		"experience_id":     experience.ID,
		"commitment_id":     commitment.ID,
		"child_concern_id":  commitment.ConcernID,
		"parent_concern_id": parentID,
	}
	return r.enqueueConcernContributionFact(
		parentID,
		experience.ID,
		"experience",
		"一项或多项由你自主关联的独立行动已经产生真实 Experience；最新后果现在可以重新参与同一上位 Concern 的判断。",
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
		// Experiences remain separate durable facts, while one parent needs only
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
				DifferenceKind        string   `json:"difference_kind"`
				EvidenceExperienceIDs []string `json:"evidence_experience_ids"`
			}
			if json.Unmarshal(candidate.Payload, &payload) == nil &&
				strings.TrimSpace(payload.DifferenceKind) != "" &&
				len(payload.EvidenceExperienceIDs) == 0 {
				// An operational interoceptive signal is real evidence that
				// something in the current regulation loop deserves attention, but
				// it is not yet lived evidence of how Alice actually regulates it.
				// Let the signal change appraisal, attention or action. Narrative
				// waits for subsequent Reality to become Experience, so paraphrasing
				// "I will wait/change" cannot clear the very gap being observed.
				return errors.New("an operational self-model signal without lived Experience may guide appraisal or action, while Narrative changes after its regulation produces Reality")
			}
		}
		return nil
	}
	if commitmentIDFromEvent(candidate) == "" || len(commit.ExperienceUpdates) != 1 {
		return errors.New("a narrative update is grounded in an important reality experience or a self-model difference")
	}
	if strings.TrimSpace(r.state.Self.Narrative) != "" {
		return errors.New("an established narrative changes through accumulated self-model difference")
	}
	if absFloat(commit.ExperienceUpdates[0].Values.SelfEndorsed) < r.config.Dynamics.AttentionThreshold {
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
	for _, experienceUpdate := range commit.ExperienceUpdates {
		if experience := r.experienceForFocus(commit.FocusID); experience != nil {
			evidence = append(evidence, experience.ID)
		} else if experience := r.experienceForCommitment(experienceUpdate.CommitmentID); experience != nil {
			evidence = append(evidence, experience.ID)
		}
	}
	if candidate, exists := r.activeCandidates[commit.FocusID]; exists && candidate.Kind == "self_model_difference" {
		var payload struct {
			EvidenceExperienceIDs []string `json:"evidence_experience_ids"`
		}
		if json.Unmarshal(candidate.Payload, &payload) == nil {
			evidence = append(evidence, payload.EvidenceExperienceIDs...)
		}
	}
	return r.journal("self_updated", commit.FocusID, map[string]any{
		"focus_id":                commit.FocusID,
		"evidence_experience_ids": evidence,
		"previous_narrative":      previous,
		"current_narrative":       r.state.Self.Narrative,
	})
}

// accrueSelfModelTension closes the same Difference Field over the current
// self-model. A self-relevant experience contributes in proportion to its
// prediction/reality residue; repeated low-gap experiences can accumulate,
// while one surprising experience contributes faster. No semantic conclusion
// is generated here—the threshold only returns grounded evidence to attention.
func (r *Runtime) accrueSelfModelTension(experience Experience, narrativeUpdated bool) error {
	if narrativeUpdated {
		return nil
	}
	selfRelevance := absFloat(experience.Values.SelfEndorsed)
	if selfRelevance == 0 {
		return nil
	}
	difference := maxFloat(experience.PredictionDifference, experience.RemainingDifference)
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
		"before":                  before,
		"after":                   r.state.SelfModelTension,
		"current_narrative":       r.state.Self.Narrative,
		"evidence":                evidence,
		"evidence_experience_ids": evidenceIDs,
	})
	return r.enqueueSelfModelDifference(
		"self_model_difference",
		"endogenous",
		"近期真实经历已经积累成一次值得理解的自我模型差异。",
		experience.ID,
		payload,
		true,
	)
}

func (r *Runtime) recentSelfModelEvidence(limit int) []Experience {
	if limit <= 0 {
		return nil
	}
	selected := make([]Experience, 0, limit)
	for index := len(r.state.Experiences) - 1; index >= 0 && len(selected) < limit; index-- {
		experience := r.state.Experiences[index]
		if absFloat(experience.Values.SelfEndorsed) == 0 {
			continue
		}
		selected = append(selected, experience)
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
// later fatigue, low-yield attention, and contradictory Experience invisible.
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
	if len(r.state.Commitments) > maxCommitments {
		r.state.Commitments = append([]ActionCommitment(nil), r.state.Commitments[len(r.state.Commitments)-maxCommitments:]...)
	}
	return r.journal("action_committed", commitment.ID, commitment)
}

// validateActionProgress gives assimilated reality two small, factual
// consequences. A settled request cannot be replayed as apparent progress for
// an unchanged concern, and an exactly repeated failed organ action is
// not a new experiment merely because a different concern names it. Alice still
// chooses the next substantive action and may use any genuinely changed request.
func (r *Runtime) validateActionProgress(focusID string, action CognitiveAction, effectiveConcernIDs ...string) error {
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
		experience := r.experienceForCommitment(commitment.ID)
		if experience == nil {
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
			return errors.New("the exact organ request already failed or returned an unknown result; inspect reality and form a genuinely changed request")
		}
		if experience.RemainingDifference > settledDifference {
			return nil
		}
		if commitment.ConcernID != concernID {
			return errors.New("the same enacted request already returned a settled reality under another concern; a new concern does not reset embodied experience, so form a genuinely changed request")
		}
		return errors.New("the same enacted request already returned a low-difference reality for this concern, and no new body or world fact has changed its conditions")
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

func (r *Runtime) experienceForCommitment(commitmentID string) *Experience {
	for index := len(r.state.Experiences) - 1; index >= 0; index-- {
		if r.state.Experiences[index].CommitmentID == commitmentID {
			return &r.state.Experiences[index]
		}
	}
	return nil
}

func (r *Runtime) experienceForFocus(focusID string) *Experience {
	for index := len(r.state.Experiences) - 1; index >= 0; index-- {
		if r.state.Experiences[index].FocusID == focusID {
			return &r.state.Experiences[index]
		}
	}
	return nil
}

func (r *Runtime) materialRealityAfter(seq uint64, repeatedKind, repeatedRequest string) bool {
	for index := range r.state.Background {
		event := r.state.Background[index]
		if event.Seq <= seq || event.Kind == "endogenous_change" {
			continue
		}
		if event.Kind != "action_result" {
			return true
		}
		var action ActionState
		if json.Unmarshal(event.Payload, &action) != nil || action.Kind != repeatedKind || normalizedActionStateRequest(action) != repeatedRequest {
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

func contributedExperienceIDFromEvent(candidate Event) string {
	if candidate.Kind != "concern_contribution" || len(candidate.Payload) == 0 {
		return ""
	}
	var payload struct {
		ExperienceID string `json:"experience_id"`
	}
	_ = json.Unmarshal(candidate.Payload, &payload)
	return payload.ExperienceID
}

func selectContextExperiences(experiences []Experience, candidates []Event) []Experience {
	if len(experiences) <= maxExperienceContext {
		return append([]Experience(nil), experiences...)
	}
	wantedCommitments := make(map[string]bool)
	wantedExperiences := make(map[string]bool)
	for _, candidate := range candidates {
		if commitmentID := commitmentIDFromEvent(candidate); commitmentID != "" {
			wantedCommitments[commitmentID] = true
		}
		if experienceID := contributedExperienceIDFromEvent(candidate); experienceID != "" {
			wantedExperiences[experienceID] = true
		}
	}
	selected := make([]Experience, 0, maxExperienceContext)
	seen := make(map[string]bool)
	for index := len(experiences) - 1; index >= 0; index-- {
		experience := experiences[index]
		if (wantedCommitments[experience.CommitmentID] || wantedExperiences[experience.ID]) && !seen[experience.ID] {
			selected = append(selected, experience)
			seen[experience.ID] = true
		}
	}
	query := memoryQuery(candidates)
	type scoredExperience struct {
		experience Experience
		score      float64
		index      int
	}
	scored := make([]scoredExperience, 0, len(experiences))
	for index, experience := range experiences {
		if seen[experience.ID] {
			continue
		}
		// Causal recall needs what the body actually did, not only the meaning
		// later written about it. A delayed regression often names the affected
		// path or setting more precisely than the Experience summary; retaining
		// the enacted request in similarity lets Alice reconnect a present body
		// change to her own earlier intervention without the kernel declaring a
		// cause or choosing a correction.
		text := strings.Join([]string{experience.EnactedRequest, experience.Meaning, experience.Lesson, experience.MethodUpdate}, " ")
		scored = append(scored, scoredExperience{experience: experience, score: memorySimilarity(query, text), index: index})
	}
	sort.SliceStable(scored, func(left, right int) bool {
		if scored[left].score == scored[right].score {
			return scored[left].index > scored[right].index
		}
		return scored[left].score > scored[right].score
	})
	for _, item := range scored {
		if len(selected) >= maxExperienceContext {
			break
		}
		selected = append(selected, item.experience)
		seen[item.experience.ID] = true
	}
	return selected
}

func memoryQuery(candidates []Event) string {
	parts := make([]string, 0, len(candidates)*2)
	for _, candidate := range candidates {
		parts = append(parts, candidate.Summary, string(candidate.Payload))
	}
	return strings.Join(parts, " ")
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
