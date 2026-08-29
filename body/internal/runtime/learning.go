package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

const (
	maxCommitments        = 32
	maxExperiences        = 128
	maxExperienceContext  = 8
	maxSelfMethods        = 8
	maxSelfMethodBytes    = 512
	maxSelfNarrativeBytes = 4096
	settledDifference     = 0.25
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
	if strings.TrimSpace(update.MethodUpdate) != "" && len(r.state.Self.Methods) >= maxSelfMethods {
		duplicate := false
		for _, method := range r.state.Self.Methods {
			if method == strings.TrimSpace(update.MethodUpdate) {
				duplicate = true
				break
			}
		}
		if !duplicate && (update.MethodSlot < 0 || update.MethodSlot >= len(r.state.Self.Methods)) {
			return fmt.Errorf("durable methods are full; method_slot must select 0..%d", len(r.state.Self.Methods)-1)
		}
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
	values := []float64{update.Values.Continuance, update.Values.Relatedness, update.Values.Expansion, update.Values.SelfEndorsed}
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
		} else {
			if experience.MethodSlot < 0 || experience.MethodSlot >= len(methods) {
				return fmt.Errorf("durable methods are full; method_slot must select 0..%d", len(methods)-1)
			}
			methods[experience.MethodSlot] = updated
		}
		r.state.Self.Methods = methods
	}
	if experience.MethodUpdate != "" {
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
	return r.enqueueConcernContribution(strings.TrimSpace(commit.ContributesToConcernID), *commitment, experience)
}

// enqueueConcernContribution gives a real child Experience one bounded route
// back into a broader Concern that Alice selected while assimilating the
// result. It creates no
// meaning, reward, priority, or prescribed response: the new candidate merely
// makes the factual relationship available to the next single attention pulse.
func (r *Runtime) enqueueConcernContribution(parentID string, commitment ActionCommitment, experience Experience) error {
	parentID = strings.TrimSpace(parentID)
	if parentID == "" {
		return nil
	}
	parent := r.concernByID(parentID)
	if parent == nil || parent.Resolution != "hold" || parent.Ownership < r.config.Dynamics.AttentionThreshold {
		return nil
	}
	payload, _ := json.Marshal(map[string]any{
		"experience_id":     experience.ID,
		"commitment_id":     commitment.ID,
		"child_concern_id":  commitment.ConcernID,
		"parent_concern_id": parentID,
	})
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
		existing.CorrelationID = experience.ID
		existing.Payload = payload
		existing.ObservedAt = nowUTC()
		if existing.Status != "in_focus" && existing.Status != "model_wait" {
			existing.Status = "pending"
			existing.LastCommitErr = ""
			existing.CognitionAttempts = 0
		}
		return r.journal("concern_contribution_refreshed", existing.ID, map[string]any{
			"experience_id":     experience.ID,
			"commitment_id":     commitment.ID,
			"child_concern_id":  commitment.ConcernID,
			"parent_concern_id": parentID,
		})
	}
	return r.addEvent(
		"concern_contribution",
		"experience",
		"一项或多项由你自主关联的独立行动已经产生真实 Experience；最新后果现在可以重新参与同一上位 Concern 的判断。",
		experience.ID,
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
	if threshold <= 0 || r.state.SelfModelTension < threshold || r.selfModelDifferenceActive() {
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
	return r.addEvent(
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

func (r *Runtime) selfModelDifferenceActive() bool {
	for _, event := range r.state.Background {
		if event.Kind == "self_model_difference" && eventChainActive(event.Status) {
			return true
		}
	}
	for _, concern := range r.state.Concerns {
		if concern.OriginKind == "self_model_difference" && concern.Resolution == "hold" {
			return true
		}
	}
	return false
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
// an unchanged concern, and an exactly repeated deterministic shell failure is
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
		if kind == "body_shell" && shellRequestFailed(previousAction) {
			return errors.New("the exact shell request already returned a non-zero or timed-out result; use the assimilated reality to form a genuinely changed request")
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
	case "body_shell":
		return action.Kind, normalizeEnactedRequest(action.Kind, action.Command)
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
		return event, &action, action.Kind, normalizeEnactedRequest(action.Kind, action.Request)
	}
	return nil, nil, "", ""
}

func shellRequestFailed(action *ActionState) bool {
	if action == nil || action.Kind != "body_shell" {
		return false
	}
	var result struct {
		ExitCode int  `json:"exit_code"`
		TimedOut bool `json:"timed_out"`
	}
	if json.Unmarshal([]byte(action.Result), &result) != nil {
		return false
	}
	return result.ExitCode != 0 || result.TimedOut
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
		if json.Unmarshal(event.Payload, &action) != nil || action.Kind != repeatedKind || normalizeEnactedRequest(action.Kind, action.Request) != repeatedRequest {
			return true
		}
	}
	return false
}

// genericExplorationMentorContactAvailable keeps two motivational sources
// distinct. General exploration may use one first outbound message to make the
// available mentor relationship real. Later conversation is still fully
// available when a mentor message, a situated experience, a feeling, or a
// concrete relationship concern is the actual focus. What cannot happen is for
// objectless exploration pressure to turn every reciprocal mentor reply into
// another generic status send. That would make reliable social acknowledgement
// the easiest endlessly renewable substitute for contact with the world.
func genericExplorationMentorContactAvailable(commitments []ActionCommitment) bool {
	for _, commitment := range commitments {
		if commitment.ActionKind == "mentor_send" {
			return false
		}
	}
	return true
}

// normalizeEnactedRequest identifies the embodied operation rather than inert
// shell policy wrappers. Options such as `set -euo pipefail` change failure
// handling but do not create a new contact with reality, so they cannot turn a
// settled request into apparent progress.
func normalizeEnactedRequest(kind, request string) string {
	request = strings.ReplaceAll(request, "\r\n", "\n")
	if kind != "body_shell" {
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
		text := strings.Join([]string{experience.Meaning, experience.Lesson, experience.MethodUpdate}, " ")
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
