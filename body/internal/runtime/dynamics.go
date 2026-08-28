package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	defaultAttentionCandidateLimit = 3
	defaultAttentionRevisitSeconds = 300
	defaultConcernContextLimit     = 8
)

func (r *Runtime) advanceDynamics(elapsed time.Duration) error {
	minutes := elapsed.Minutes()
	if minutes <= 0 {
		return nil
	}
	returnFactor := clamp01(1 - r.config.Dynamics.AffectReturnRate*minutes)
	r.state.AffectiveState.Valence = clampSigned(r.state.AffectiveState.Valence * returnFactor)
	r.state.AffectiveState.Activation = clamp01(r.state.AffectiveState.Activation * returnFactor)
	r.state.AffectiveState.Control = clamp01(0.5 + (r.state.AffectiveState.Control-0.5)*returnFactor)
	r.state.AffectiveState.Certainty = clamp01(0.5 + (r.state.AffectiveState.Certainty-0.5)*returnFactor)

	concernFactor := clamp01(1 - r.config.Dynamics.ConcernNaturalDecayRate*minutes)
	for index := range r.state.Concerns {
		r.state.Concerns[index].Strength = clamp01(r.state.Concerns[index].Strength * concernFactor)
		// Activation is present salience, not a permanent property of a past
		// appraisal.  Leaving it frozen made an old Concern eligible for random
		// recall forever even after its accumulated strength had decayed.
		r.state.Concerns[index].Activation = clamp01(r.state.Concerns[index].Activation * concernFactor)
	}

	before := r.state.ExplorationPressure
	if r.state.Lease == nil && r.state.PendingAction == nil {
		r.state.ExplorationPressure = clamp01(before + r.config.Dynamics.ExplorationIdleGrowth*minutes)
	}
	crossed := before < r.config.Dynamics.AttentionThreshold && r.state.ExplorationPressure >= r.config.Dynamics.AttentionThreshold
	currentConcernID := r.currentExplorationConcernID()
	orphaned := r.state.ExplorationPressure >= r.config.Dynamics.AttentionThreshold &&
		currentConcernID == "" && !r.explorationCandidateActive() &&
		attentionDue(r.state.LastAttentionAt, time.Now().UTC(), r.config.Dynamics.AttentionRevisitSeconds)
	if (crossed || orphaned) && currentConcernID == "" && !r.attentionCandidateActive() {
		if r.state.Stage >= 8 {
			// The drive stays active, while a low-cost perceptual scan supplies the
			// next eligible object only when visible content actually differs. A
			// stable affordance is body background, not a paid cognitive candidate.
			return nil
		}
		payloadFields := map[string]any{
			"before": before,
			"after":  r.state.ExplorationPressure,
		}
		summary := "探索张力已经积蓄到值得接触现实。"
		payload, _ := json.Marshal(payloadFields)
		return r.addEvent(
			"endogenous_change",
			"endogenous",
			summary,
			"",
			payload,
			true,
		)
	}
	return nil
}

func (r *Runtime) currentExplorationConcernID() string {
	for index := len(r.state.Concerns) - 1; index >= 0; index-- {
		concern := r.state.Concerns[index]
		if cognitionValidationExhausted(concern, r.config.CognitiveResource) {
			continue
		}
		if concernOwnsExplorationDrive(concern, r.state.Commitments, r.state.Mentor, r.config.Dynamics.AttentionThreshold) {
			return concern.ID
		}
	}
	for index := len(r.state.Background) - 1; index >= 0; index-- {
		event := r.state.Background[index]
		if event.Kind != "endogenous_change" || event.ConcernID == "" {
			continue
		}
		if concern := r.concernByID(event.ConcernID); concern != nil {
			if cognitionValidationExhausted(*concern, r.config.CognitiveResource) {
				continue
			}
			if concernOwnsExplorationDrive(*concern, r.state.Commitments, r.state.Mentor, r.config.Dynamics.AttentionThreshold) {
				return event.ConcernID
			}
		}
	}
	return ""
}

func (r *Runtime) explorationCandidateActive() bool {
	for _, event := range r.state.Background {
		if event.Kind == "endogenous_change" && eventChainActive(event.Status) {
			return true
		}
	}
	return r.currentExplorationConcernID() != ""
}

func eventChainActive(status string) bool {
	switch status {
	case "pending", "in_focus", "retry_wait", "resource_wait", "model_wait":
		return true
	default:
		return false
	}
}

func (r *Runtime) hasUnassimilatedCommitment() bool {
	for _, commitment := range r.state.Commitments {
		switch commitment.Status {
		case "formed", "acting", "reality_available", "reality_unknown":
			return true
		}
	}
	return false
}

func (r *Runtime) attentionCandidateActive() bool {
	if r.state.PendingAction != nil || r.hasUnassimilatedCommitment() {
		return true
	}
	for _, event := range r.state.Background {
		if eventChainActive(event.Status) {
			return true
		}
	}
	return false
}

func (r *Runtime) nextStage4Request() (CognitiveRequest, bool) {
	if r.state.PendingAction != nil {
		return CognitiveRequest{}, false
	}
	for _, event := range r.state.Background {
		if event.Status == "pending" && event.Kind == "action_result" {
			return CognitiveRequest{Stage: 4, Focus: event, Candidates: []Event{event}}, true
		}
	}
	// A chosen action remains the single causal foreground until its reality has
	// been assimilated. If that reality is waiting for a validation retry, other
	// concerns and external events stay in the background instead of stealing
	// the attention thread.
	if r.hasUnassimilatedCommitment() {
		return CognitiveRequest{}, false
	}
	// "next" is a serial cognitive continuation chosen by Alice, not another
	// ordinary background event. Reality keeps first priority; immediately
	// after Reality is settled, the oldest pending continuation becomes the
	// next and only focus. This also prevents several unconsumed model choices
	// from accumulating while unrelated Concerns keep winning attention.
	for _, event := range r.state.Background {
		if event.Status == "pending" && event.Kind == "cognition_continuation" {
			return CognitiveRequest{Stage: 4, Focus: event, Candidates: []Event{event}}, true
		}
	}
	candidates := make([]Event, 0, defaultAttentionCandidateLimit*2)
	representedConcerns := make(map[string]bool)
	for _, event := range r.state.Background {
		if event.Status != "pending" {
			continue
		}
		candidates = append(candidates, event)
		if event.ConcernID != "" {
			representedConcerns[event.ConcernID] = true
		}
	}
	now := time.Now().UTC()
	for _, concern := range r.state.Concerns {
		if representedConcerns[concern.ID] {
			continue
		}
		if concern.WaitModel != "" {
			continue
		}
		if cognitionValidationExhausted(concern, r.config.CognitiveResource) {
			continue
		}
		// Once an unacted exploration drive has one Concern identity, repeating
		// the same model reflection cannot add reality. Let the same
		// pressure accumulate quietly until either a fresh event enters attention
		// or the derived action threshold is reached.  This preserves object
		// formation without paying for an empty semantic loop.
		selfRevisitWithoutReality := concern.LastSourceID == concern.ID
		ownsExplorationDrive := concernOwnsExplorationDrive(
			concern,
			r.state.Commitments,
			r.state.Mentor,
			r.config.Dynamics.AttentionThreshold,
		)
		if selfRevisitWithoutReality && !ownsExplorationDrive {
			// A direct Concern reflection followed by no action has already done
			// all the causal work available from the present state.  Rephrasing the
			// same held difference is not another object and cannot manufacture new
			// evidence, urgency or progress. Keep the Concern as lived background;
			// a later event explicitly linked to it can make it foreground again.
			continue
		}
		if len(candidates) == 0 &&
			ownsExplorationDrive &&
			(!concernHasCommitment(concern.ID, r.state.Commitments) || selfRevisitWithoutReality) &&
			r.state.ExplorationPressure < explorationActionThreshold(r.config.Dynamics.AttentionThreshold) {
			continue
		}
		candidate := Event{
			ID:                concern.ID,
			Kind:              "concern",
			Source:            "endogenous",
			ObservedAt:        concern.UpdatedAt,
			Summary:           concern.Meaning,
			Status:            "pending",
			ConcernID:         concern.ID,
			LastFocusedAt:     concern.LastFocusedAt,
			LastCommitErr:     concern.LastCommitErr,
			CognitionAttempts: concern.CognitionAttempts,
		}
		if r.candidateScore(candidate) < r.config.Dynamics.AttentionThreshold {
			continue
		}
		if !attentionDue(concern.LastFocusedAt, now, r.config.Dynamics.AttentionRevisitSeconds) {
			continue
		}
		candidates = append(candidates, candidate)
	}
	if len(candidates) == 0 {
		return CognitiveRequest{}, false
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left := r.candidateScore(candidates[i])
		right := r.candidateScore(candidates[j])
		if left == right {
			return candidates[i].Seq < candidates[j].Seq
		}
		return left > right
	})
	variationSeed := attentionVariationSeed(r.state.InstanceID, r.state.PulseID, candidates)
	topScore := r.candidateScore(candidates[0])
	topBand := 1
	for topBand < len(candidates) && topScore-r.candidateScore(candidates[topBand]) <= 0.05 {
		topBand++
	}
	if topBand > 1 {
		choice := int(sha256.Sum256([]byte(variationSeed))[0]) % topBand
		candidates[0], candidates[choice] = candidates[choice], candidates[0]
	}
	limit := r.config.Dynamics.AttentionCandidateLimit
	if limit <= 0 || limit > defaultAttentionCandidateLimit {
		limit = defaultAttentionCandidateLimit
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	request := CognitiveRequest{Stage: 4, Focus: candidates[0], Candidates: candidates, VariationSeed: variationSeed}
	if r.shouldOfferVariation(request.Focus) {
		request.VariationBias = associativeRecall(r.state, r.config.Dynamics, variationSeed)
	}
	return request, true
}

// associativeRecall gives program randomness a narrow and non-authoritative
// role. It varies which lived material becomes salient and the cognitive way
// Alice may approach the concrete surface already carried by the exploration
// event. It never manufactures another object, goal or reward.
func associativeRecall(state State, dynamics Dynamics, seed string) string {
	const recentExperienceLimit = 8
	cues := make([]string, 0, recentExperienceLimit+len(state.Concerns)+1)
	minimumConcernSalience := dynamics.AttentionThreshold * dynamics.ConcernBaseDrive
	for _, concern := range state.Concerns {
		if concern.Resolution == "resolved" {
			continue
		}
		// Random recall may vary what is salient, but it must not resurrect a
		// decayed Concern that no longer has enough present dynamics to compete.
		// External reality can make that Concern salient again through the normal
		// event path; randomness alone cannot do so.
		if maxFloat(concern.Strength, concern.Activation) < minimumConcernSalience {
			continue
		}
		if meaning := strings.TrimSpace(concern.Meaning); meaning != "" {
			cues = append(cues, "仍在生活中的关切："+truncate(meaning, 600))
		}
	}
	start := len(state.Experiences) - recentExperienceLimit
	if start < 0 {
		start = 0
	}
	for _, experience := range state.Experiences[start:] {
		if meaning := strings.TrimSpace(experience.Meaning); meaning != "" {
			cues = append(cues, "自己形成的近期经验："+truncate(meaning, 600))
		}
	}
	if narrative := strings.TrimSpace(state.Self.Narrative); narrative != "" {
		cues = append(cues, "当前自我叙事："+truncate(narrative, 600))
	}
	digest := sha256.Sum256([]byte(seed + "|associative-recall"))
	parts := []string{"随机变化视角（可采用，也可离开）：" + explorationApproachLens(digest[1])}
	if len(cues) > 0 {
		parts = append(parts, "联想材料："+cues[int(digest[0])%len(cues)])
	}
	return strings.Join(parts, "\n")
}

func explorationApproachLens(choice byte) string {
	lenses := [...]string{
		"从眼前已经出现的一个具体细节继续深入，让接触本身揭示它是否值得成为关切。",
		"让一个具体对象获得你的回应，或让现实因你发生一个小而可检验的变化。",
		"把两段已经经历过的事实重新组合成一个表达、问题或作品，再观察它带来的现实后果。",
		"寻找当前现实与自我叙事之间的一处具体反差，用一次接触让差异变得更清楚。",
	}
	return lenses[int(choice)%len(lenses)]
}

func attentionVariationSeed(instanceID string, pulseID uint64, candidates []Event) string {
	parts := []string{instanceID, fmt.Sprintf("%d", pulseID)}
	for _, candidate := range candidates {
		parts = append(parts, candidate.ID)
	}
	digest := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return fmt.Sprintf("%x", digest[:8])
}

func (r *Runtime) shouldOfferVariation(focus Event) bool {
	if r.state.ExplorationPressure < r.config.Dynamics.AttentionThreshold || r.hasUnassimilatedCommitment() {
		return false
	}
	if focus.Kind == "perceptual_change" {
		// A fresh reality object is already a valid referent. Mature exploration
		// may vary how Alice approaches it before it has acquired a persistent
		// Concern identity; otherwise program variation arrives only after the
		// model has already repeated its default observe-and-wait pattern. The cue
		// supplies no topic, value or action and Alice may still release the object.
		return true
	}
	if focus.Kind == "endogenous_change" {
		return true
	}
	if concern := r.concernByID(focus.ConcernID); concern != nil {
		return concernOwnsExplorationDrive(*concern, r.state.Commitments, r.state.Mentor, r.config.Dynamics.AttentionThreshold)
	}
	concern := r.concernByID(focus.ID)
	return focus.Kind == "concern" && concern != nil && concernOwnsExplorationDrive(*concern, r.state.Commitments, r.state.Mentor, r.config.Dynamics.AttentionThreshold)
}

func explorationActionThreshold(attentionThreshold float64) float64 {
	return clamp01(attentionThreshold + (1-attentionThreshold)*0.5)
}

// concernOwnsExplorationDrive binds free exploration energy to one concrete
// object after Alice has actually endorsed it. The general drive itself never
// becomes an object. A perceived concrete concern can keep
// receiving later attention while new Reality leaves it held and answerable;
// deliberate non-action on a direct Concern return unbinds the drive because
// LastSourceID then becomes the Concern's own ID.
// This uses Alice's existing O/V/A appraisal and causal identity instead of a
// developer topic whitelist, boredom counter or semantic text classifier.
func concernOwnsExplorationDrive(concern Concern, commitments []ActionCommitment, mentor MentorState, threshold float64) bool {
	if concern.Resolution != "hold" {
		return false
	}
	if concernAwaitsMentorReply(concern.ID, commitments, mentor) {
		return false
	}
	hasCommitment := concernHasCommitment(concern.ID, commitments)
	switch concern.OriginKind {
	case "endogenous_change":
		if concern.Answerability >= threshold {
			return true
		}
		return !hasCommitment
	case "perceptual_change":
		return concern.LastSourceID != concern.ID &&
			concern.Ownership >= threshold && absFloat(concern.Value) >= threshold &&
			concern.Answerability >= threshold
	default:
		return false
	}
}

func concernHasCommitment(concernID string, commitments []ActionCommitment) bool {
	for _, commitment := range commitments {
		if commitment.ConcernID == concernID {
			return true
		}
	}
	return false
}

func concernAwaitsMentorReply(concernID string, commitments []ActionCommitment, mentor MentorState) bool {
	if concernID == "" {
		return false
	}
	commitmentConcern := make(map[string]string, len(commitments))
	for _, commitment := range commitments {
		commitmentConcern[commitment.ID] = commitment.ConcernID
	}
	for _, message := range mentor.Outbox {
		if message.CommitmentID == "" || message.RepliedAt != "" {
			continue
		}
		if message.Status != "queued" && message.Status != "delivered" {
			continue
		}
		if commitmentConcern[message.CommitmentID] == concernID {
			return true
		}
	}
	return false
}

func attentionDue(last string, now time.Time, revisitSeconds int) bool {
	if last == "" {
		return true
	}
	if revisitSeconds <= 0 {
		revisitSeconds = defaultAttentionRevisitSeconds
	}
	parsed, err := time.Parse(time.RFC3339Nano, last)
	return err != nil || now.Sub(parsed) >= time.Duration(revisitSeconds)*time.Second
}

func (r *Runtime) candidateScore(candidate Event) float64 {
	novelty := 0.0
	if candidate.Kind != "concern" {
		novelty = 1
	}
	concernStrength := 0.0
	affectiveSalience := r.state.AffectiveState.Activation
	explorationValue := 0.0
	expectedCost := 0.25
	if concern := r.concernByID(candidate.ConcernID); concern != nil {
		concernStrength = concern.Strength
		affectiveSalience = maxFloat(affectiveSalience, concern.Activation)
		expectedCost = 1 - concern.Answerability
		if concernOwnsExplorationDrive(*concern, r.state.Commitments, r.state.Mentor, r.config.Dynamics.AttentionThreshold) {
			// The concern is the durable identity of one exploration tension. As the
			// underlying pressure returns, the same concern must be able to compete
			// again instead of requiring a duplicate periodic event.
			concernStrength = maxFloat(concernStrength, r.state.ExplorationPressure)
			explorationValue = r.state.ExplorationPressure
		}
	}
	if candidate.Kind == "endogenous_change" || strings.Contains(strings.ToLower(candidate.Summary), "exploration") {
		explorationValue = r.state.ExplorationPressure
		expectedCost = 0.15
	}
	if candidate.Kind == "self_model_difference" {
		concernStrength = maxFloat(concernStrength, r.state.SelfModelTension)
		expectedCost = 0.15
	}
	if concern := r.concernByID(candidate.ConcernID); concern != nil && concern.OriginKind == "self_model_difference" {
		concernStrength = maxFloat(concernStrength, r.state.SelfModelTension)
	}
	return concernStrength +
		r.config.Dynamics.AttentionAffectWeight*affectiveSalience +
		r.config.Dynamics.AttentionExplorationWeight*explorationValue +
		r.config.Dynamics.AttentionNoveltyWeight*novelty -
		r.config.Dynamics.AttentionCostWeight*expectedCost
}

func normalizeUnendorsedAction(commit CognitiveCommit, threshold float64) (CognitiveCommit, string) {
	if commit.Action.Kind == "none" {
		return commit, ""
	}
	for _, appraisal := range commit.Appraisals {
		if appraisal.CandidateID != commit.FocusID || appraisal.Ownership >= threshold {
			continue
		}
		withheld := commit.Action.Kind
		commit.Action = CognitiveAction{Kind: "none"}
		// A next profile attached to an action is ordinarily waiting for that
		// action's Reality.  Once the same appraisal withholds enactment, there is
		// no new fact for a serial continuation to absorb.
		if commit.ResourceChoice.Apply == "next" {
			commit.ResourceChoice = CognitiveResourceChoice{
				Apply:           "keep",
				Model:           "current",
				ReasoningEffort: "current",
			}
		}
		return commit, withheld
	}
	return commit, ""
}

func (r *Runtime) applyCognitiveCommit(commit CognitiveCommit) error {
	commit, withheldActionKind := normalizeUnendorsedAction(commit, r.config.Dynamics.AttentionThreshold)
	return r.applyPreparedCognitiveCommit(commit, withheldActionKind)
}

func (r *Runtime) applyPreparedCognitiveCommit(commit CognitiveCommit, withheldActionKind string) error {
	if len(commit.Appraisals) == 0 || len(commit.Appraisals) > defaultAttentionCandidateLimit {
		return errors.New("cognitive commit must contain one to three appraisals")
	}
	if _, exists := r.activeCandidates[commit.FocusID]; !exists {
		return fmt.Errorf("focus %q is not an active candidate", commit.FocusID)
	}
	if len([]rune(strings.TrimSpace(commit.ThoughtThread))) > 2000 {
		return errors.New("thought thread is too large for a single attention pulse")
	}
	if err := validateCognitiveAction(commit.Action, r.state.Stage); err != nil {
		return err
	}
	matureExplorationDrive := r.explorationHasMatureDrive(commit.FocusID)
	if matureExplorationDrive && commit.Action.Kind == "mentor_send" && !genericExplorationMentorContactAvailable(r.state.Commitments) {
		return errors.New("general exploration has already made mentor contact; continue the relationship from a mentor message or a concrete experience, or let another part of reality introduce new difference")
	}
	if commit.Action.Kind != "none" {
		if concernID := r.focusConcernID(commit.FocusID); concernID != "" && r.openCommitmentForConcern(concernID) != nil {
			return fmt.Errorf("concern %q already has an unassimilated action commitment", concernID)
		}
		if err := r.validateActionProgress(commit.FocusID, commit.Action); err != nil {
			return err
		}
	}
	if err := r.validateExperienceUpdates(commit); err != nil {
		return err
	}
	if err := r.validateNarrativeUpdate(commit); err != nil {
		return err
	}
	profile, err := r.validateResourceChoice(commit.ResourceChoice, commit.FocusID, commit.Action.Kind)
	if err != nil {
		return err
	}
	seen := make(map[string]bool)
	for _, appraisal := range commit.Appraisals {
		if _, exists := r.activeCandidates[appraisal.CandidateID]; !exists {
			return fmt.Errorf("appraisal candidate %q is not active", appraisal.CandidateID)
		}
		if seen[appraisal.CandidateID] {
			return fmt.Errorf("candidate %q was appraised twice", appraisal.CandidateID)
		}
		seen[appraisal.CandidateID] = true
		if err := validateAppraisal(appraisal); err != nil {
			return err
		}
	}
	if len(seen) != len(r.activeCandidates) {
		return errors.New("every active candidate must receive one appraisal")
	}
	if !seen[commit.FocusID] {
		return errors.New("the selected focus must also be appraised")
	}
	now := nowUTC()
	weightedValence := 0.0
	weightedControl := 0.0
	weightedCertainty := 0.0
	weightTotal := 0.0
	maxActivation := 0.0
	uncertaintyTotal := 0.0
	explorationResultRelief := 0.0
	for _, appraisal := range commit.Appraisals {
		candidate := r.activeCandidates[appraisal.CandidateID]
		if appraisal.CandidateID == commit.FocusID {
			r.applyPerceptualSaturation(candidate, appraisal, commit)
		}

		activation := appraisalActivation(r.config.Dynamics, appraisal)
		concern := r.concernForCandidate(candidate)
		created := false
		if concern == nil && r.shouldPersistNewConcern(commit, appraisal, activation) {
			r.state.Concerns = append(r.state.Concerns, Concern{ID: "concern-" + randomID()})
			concern = &r.state.Concerns[len(r.state.Concerns)-1]
			created = true
		}
		if concern != nil {
			previousMeaning := concern.Meaning
			previousResolution := concern.Resolution
			previousDifference := concern.Difference
			concernResolution := strings.TrimSpace(appraisal.Resolution)
			if concernResolution == "reframed" {
				// Reframing changes what the concern means while retaining its
				// causal identity. Store the changed concern as held; relieved and
				// resolved remain the two ways Alice expresses release.
				concernResolution = "hold"
			}
			if appraisal.CandidateID == commit.FocusID && commit.Action.Kind != "none" {
				// The enacted commitment is the embodied fact that this difference
				// remains owned until Reality returns, regardless of a looser verbal
				// resolution label in the same cognitive commit.
				concernResolution = "hold"
			}
			releasedByOwnership := appraisal.Ownership < r.config.Dynamics.AttentionThreshold &&
				!(appraisal.CandidateID == commit.FocusID && commit.Action.Kind != "none") &&
				!concernAwaitsMentorReply(concern.ID, r.state.Commitments, r.state.Mentor)
			if releasedByOwnership {
				// Ownership is the boundary between noticing a fact and agreeing to
				// let it remain part of one's active life.  A low-O appraisal may be
				// remembered as Experience, but it cannot remain a held Concern just
				// because the language model also emitted "hold".  This is a semantic
				// consistency rule, not an experimenter choice about which object
				// Alice ought to care about.
				concernResolution = "relieved"
			}
			concern.Subject = truncate(candidate.Summary, 512)
			if concern.OriginKind == "" {
				concern.OriginKind = candidate.Kind
			}
			if commitmentID := commitmentIDFromEvent(candidate); commitmentID != "" {
				concern.CommitmentID = commitmentID
			}
			concern.Meaning = strings.TrimSpace(appraisal.Meaning)
			concern.Difference = appraisal.Difference
			concern.Ownership = appraisal.Ownership
			concern.Value = appraisal.Value
			concern.Urgency = appraisal.Urgency
			concern.Answerability = appraisal.Answerability
			concern.Activation = activation
			concern.Certainty = appraisal.Certainty
			concern.LastSourceID = candidate.ID
			concern.UpdatedAt = now
			concern.Resolution = concernResolution
			realityProgress := 0.0
			if !created && concernResolution == "hold" && commitmentFeedbackKind(candidate.Kind) && commitmentIDFromEvent(candidate) != "" {
				// Holding a Concern means that its larger difference still belongs to
				// Alice; it does not mean that real progress had no relieving effect.
				// Only a commitment-linked Reality can supply this numerical relief.
				// Reflection alone cannot lower tension by describing it differently.
				realityProgress = maxFloat(0, previousDifference-appraisal.Difference)
			}
			concern.Strength = updateConcernStrength(
				r.config.Dynamics,
				concern.Strength,
				activation,
				concernResolution,
				realityProgress,
			)
			if releasedByOwnership {
				concern.Strength = 0
			}
			if appraisal.CandidateID == commit.FocusID {
				concern.LastFocusedAt = now
			}
			r.linkConcern(candidate.ID, concern.ID)
			transition := ""
			switch {
			case created:
				transition = "formed"
			case concern.Strength == 0 && concern.Resolution != "hold":
				transition = "released"
			case previousMeaning != concern.Meaning || previousResolution != concern.Resolution:
				transition = "restructured"
			}
			if transition != "" {
				if err := r.journal("concern_transition", concern.ID, map[string]any{
					"transition": transition,
					"source_id":  candidate.ID,
					"concern":    *concern,
				}); err != nil {
					return err
				}
			}
		}
		if candidate.Kind != "concern" && appraisal.CandidateID != commit.FocusID {
			markEvent(&r.state, appraisal.CandidateID, "background")
		}
		if candidate.Kind == "self_model_difference" {
			r.state.SelfModelTension = clamp01(
				r.state.SelfModelTension * (1 - resolutionRelief(appraisal.Resolution)),
			)
		}
		if commitmentFeedbackKind(candidate.Kind) && (r.state.Stage < 5 || r.commitmentFeedbackAnswersExploration(candidate)) {
			// Contact with Reality relieves the general urge to seek a fact even
			// when that fact opens a new, still-unresolved concern. Difference and
			// certainty express how much actual contact was obtained; the concern's
			// own resolution remains entirely under alice's appraisal.
			contactRelief := clamp01((1 - appraisal.Difference) * appraisal.Certainty)
			if contactRelief > explorationResultRelief {
				explorationResultRelief = contactRelief
			}
		}

		weight := 0.05 + activation
		weightedValence += appraisal.Value * weight
		weightedControl += appraisal.Answerability * weight
		weightedCertainty += appraisal.Certainty * weight
		weightTotal += weight
		if activation > maxActivation {
			maxActivation = activation
		}
		uncertaintyTotal += 1 - appraisal.Certainty
	}
	if explorationResultRelief > 0 {
		r.state.ExplorationPressure = clamp01(
			r.state.ExplorationPressure - r.config.Dynamics.ExplorationRelief*explorationResultRelief,
		)
		if r.state.Stage < 5 {
			for index := range r.state.Concerns {
				concern := &r.state.Concerns[index]
				if concern.OriginKind != "endogenous_change" {
					continue
				}
				concern.Strength = clamp01(concern.Strength - r.config.Dynamics.ConcernResolutionGain*explorationResultRelief)
				concern.Resolution = "resolved"
			}
		}
	}
	if weightTotal > 0 {
		targetValence := clampSigned(weightedValence / weightTotal)
		targetControl := clamp01(weightedControl / weightTotal)
		targetCertainty := clamp01(weightedCertainty / weightTotal)
		const newExperienceWeight = 0.60
		r.state.AffectiveState.Valence = clampSigned((1-newExperienceWeight)*r.state.AffectiveState.Valence + newExperienceWeight*targetValence)
		r.state.AffectiveState.Activation = clamp01(maxFloat(r.state.AffectiveState.Activation*0.50, maxActivation))
		r.state.AffectiveState.Control = clamp01((1-newExperienceWeight)*r.state.AffectiveState.Control + newExperienceWeight*targetControl)
		r.state.AffectiveState.Certainty = clamp01((1-newExperienceWeight)*r.state.AffectiveState.Certainty + newExperienceWeight*targetCertainty)
	}
	r.state.ExplorationPressure = clamp01(
		r.state.ExplorationPressure + r.config.Dynamics.ExplorationUnknownGrowth*(uncertaintyTotal/float64(len(commit.Appraisals))),
	)
	r.state.LastAttentionAt = now
	r.pruneInactiveConcerns()
	if err := r.applyResourceChoice(commit.ResourceChoice, profile, commit.FocusID); err != nil {
		return err
	}
	if err := r.applyExperienceUpdates(commit); err != nil {
		return err
	}
	if err := r.applyNarrativeUpdate(commit); err != nil {
		return err
	}
	variationBias := ""
	variationSeed := ""
	if r.state.Lease != nil {
		variationBias = r.state.Lease.VariationBias
		variationSeed = r.state.Lease.VariationSeed
	}
	payload := map[string]any{
		"focus_id":        commit.FocusID,
		"thought_thread":  truncate(strings.TrimSpace(commit.ThoughtThread), 2000),
		"appraisals":      commit.Appraisals,
		"affective_state": r.state.AffectiveState,
		"action_kind":     commit.Action.Kind,
		"resource_choice": commit.ResourceChoice,
		"variation_bias":  variationBias,
		"variation_seed":  variationSeed,
	}
	if withheldActionKind != "" {
		payload["withheld_action_kind"] = withheldActionKind
	}
	return r.journal("aip_commit", commit.FocusID, payload)
}

func (r *Runtime) applyPerceptualSaturation(candidate Event, appraisal CandidateAppraisal, commit CognitiveCommit) {
	trace, exists := r.state.Perception[browserPerceptionSurface]
	if !exists {
		return
	}
	concern := r.concernForCandidate(candidate)
	perceptualOrigin := candidate.Kind == "perceptual_change" ||
		(concern != nil && concern.OriginKind == "perceptual_change")
	if !perceptualOrigin {
		return
	}
	if commit.Action.Kind != "none" {
		// Acting is an attempt to obtain yield, not evidence that yield was
		// obtained. Clearing saturation here let a stream of unrelated objects
		// remain attractive forever as long as Alice repeated a low-yield action
		// on each one. Keep the accumulated scene history until Reality arrives.
		trace.Pending = nil
		trace.ExhaustedContext = ""
		trace.ExhaustedAt = ""
		r.state.Perception[browserPerceptionSurface] = trace
		return
	}
	if candidate.Kind == "action_result" {
		if len(commit.ExperienceUpdates) != 1 {
			return
		}
		commitment := r.commitmentByID(commit.ExperienceUpdates[0].CommitmentID)
		if commitment == nil || commitment.InitialDifference <= 0 {
			return
		}
		update := commit.ExperienceUpdates[0]
		progress := clamp01((commitment.InitialDifference - appraisal.Difference) / commitment.InitialDifference)
		endogenousValue := maxFloat(
			absFloat(update.Values.Continuance),
			maxFloat(absFloat(update.Values.Relatedness), absFloat(update.Values.Expansion)),
		)
		realisedYield := clamp01(
			progress * endogenousValue * absFloat(update.Values.SelfEndorsed) * (1 - update.ExperiencedCost),
		)
		threshold := r.config.Dynamics.AttentionThreshold
		if realisedYield >= threshold {
			trace.Saturation = clamp01(
				trace.Saturation * (1 - r.config.Dynamics.ConcernResolutionGain*realisedYield),
			)
		} else if threshold > 0 {
			trace.Saturation = clamp01(
				trace.Saturation + r.config.Dynamics.ConcernGrowthGain*(threshold-realisedYield)/threshold,
			)
		}
		r.state.Perception[browserPerceptionSurface] = trace
		return
	}
	if candidate.Kind != "perceptual_change" && candidate.Kind != "concern" {
		return
	}
	// A visible object may be novel and thematically important while the current
	// surface still cannot help Alice answer it.  Counting only low ownership and
	// low value made an endless feed of valued-but-unanswerable fragments look
	// productive forever.  Realised perceptual yield needs all three existing
	// AIP conditions: Alice owns the object, values it, and can currently respond
	// through this surface.  The complement is habituation pressure.  Urgency is
	// deliberately absent: an urgent but unanswerable concern should remain a
	// concern, while this particular surface still loses the right to keep
	// supplying near-identical fragments.
	realisedYield := clamp01(appraisal.Ownership * absFloat(appraisal.Value) * appraisal.Answerability)
	lowYield := clamp01((1 - realisedYield) * appraisal.Certainty)
	trace.Saturation = clamp01(trace.Saturation + r.config.Dynamics.ConcernGrowthGain*lowYield)
	r.state.Perception[browserPerceptionSurface] = trace
}

func (r *Runtime) shouldPersistNewConcern(commit CognitiveCommit, appraisal CandidateAppraisal, activation float64) bool {
	if appraisal.Ownership < r.config.Dynamics.AttentionThreshold {
		return false
	}
	if r.state.Stage >= 8 && appraisal.CandidateID != commit.FocusID {
		// One attention pulse can understand and be affected by several
		// candidates, but it actively adopts only its one selected focus.  Letting
		// every high-O background appraisal create a Concern turns simultaneous
		// noticing into simultaneous commitments and defeats single-threaded
		// consciousness. Existing Concerns may still be reappraised in the
		// background; this boundary governs only creation of a new identity.
		return false
	}
	if r.state.Stage >= 8 {
		if candidate, exists := r.activeCandidates[appraisal.CandidateID]; exists &&
			candidate.Kind == "endogenous_change" && commit.Action.Kind == "none" {
			// Exploration pressure is a drive, not a thing Alice must keep thinking
			// about. A non-enacted orienting surface has completed one value filter
			// and becomes temporarily familiar; concrete self-generated wishes can
			// still form from Narrative, Reality, relationship and other candidates.
			return false
		}
		if candidate, exists := r.activeCandidates[appraisal.CandidateID]; exists &&
			candidate.Kind == "perceptual_change" && commit.Action.Kind == "none" &&
			!perceptualAppraisalAssumesConcern(appraisal, r.config.Dynamics.AttentionThreshold) {
			// Perception can affect Affective State and present understanding without
			// every noticed fragment becoming another live Concern.  A first-seen
			// object obtains independent causal identity only when Alice both owns and
			// values it, and when either a current response is possible or its urgency
			// makes continued tension materially present.  This implements the
			// difference between noticing and actively assuming, using Alice's own AIP
			// values rather than a developer content classifier.
			return false
		}
	}
	if candidate, exists := r.activeCandidates[appraisal.CandidateID]; exists &&
		candidate.Kind == "endogenous_change" && appraisal.Resolution == "hold" {
		// The global drive has already crossed the attention threshold. If Alice
		// owns it and chooses to keep forming its meaning, that choice needs one
		// stable causal identity even before semantic activation or an action is
		// large. Otherwise every pulse would manufacture another copy of the same
		// still-forming motivation.
		return true
	}
	if appraisal.CandidateID == commit.FocusID && commit.Action.Kind != "none" {
		return true
	}
	if r.state.Stage < 8 {
		return appraisal.Resolution == "hold" && activation > 0
	}
	minimumActivation := r.config.Dynamics.AttentionThreshold * r.config.Dynamics.ConcernBaseDrive
	return appraisal.Resolution == "hold" && activation >= minimumActivation
}

func perceptualAppraisalAssumesConcern(appraisal CandidateAppraisal, threshold float64) bool {
	return appraisal.Ownership >= threshold &&
		absFloat(appraisal.Value) >= threshold &&
		maxFloat(appraisal.Answerability, appraisal.Urgency) >= threshold
}

func (r *Runtime) focusConcernID(focusID string) string {
	if r.concernByID(focusID) != nil {
		return focusID
	}
	for _, event := range r.state.Background {
		if event.ID == focusID {
			return event.ConcernID
		}
	}
	if candidate, ok := r.activeCandidates[focusID]; ok {
		return candidate.ConcernID
	}
	return ""
}

func (r *Runtime) openCommitmentForConcern(concernID string) *ActionCommitment {
	for index := range r.state.Commitments {
		commitment := &r.state.Commitments[index]
		if commitment.ConcernID != concernID {
			continue
		}
		switch commitment.Status {
		case "formed", "acting", "reality_available", "reality_unknown":
			return commitment
		}
	}
	return nil
}

func commitmentFeedbackKind(kind string) bool {
	return kind == "action_result" || kind == "mentor_received"
}

func (r *Runtime) commitmentFeedbackAnswersExploration(candidate Event) bool {
	commitmentID := commitmentIDFromEvent(candidate)
	commitment := r.commitmentByID(commitmentID)
	if commitment == nil {
		return false
	}
	if concern := r.concernByID(commitment.FocusID); concern != nil {
		return concern.OriginKind == "endogenous_change"
	}
	for _, event := range r.state.Background {
		if event.ID != commitment.FocusID {
			continue
		}
		if event.Kind == "endogenous_change" {
			return true
		}
		if concern := r.concernByID(event.ConcernID); concern != nil {
			return concern.OriginKind == "endogenous_change"
		}
		return false
	}
	return false
}

func (r *Runtime) validateResourceChoice(choice CognitiveResourceChoice, focusID, actionKind string) (CognitiveProfile, error) {
	current := activeProfile(r.state, r.config.CognitiveResource, focusID)
	if r.state.Lease != nil {
		// The lease belongs to this single attention pulse. The commit may
		// legitimately select any candidate that participated in that pulse.
		current = r.state.Lease.Profile
	}
	model := choice.Model
	effort := choice.ReasoningEffort
	if model == "current" {
		model = current.Model
	}
	if effort == "current" {
		effort = current.ReasoningEffort
	}
	profile := CognitiveProfile{Model: model, ReasoningEffort: effort}
	if choice.Apply == "keep" {
		if profile == current {
			return current, nil
		}
		return CognitiveProfile{}, fmt.Errorf("keep resource choice %s/%s does not describe current %s/%s", choice.Model, choice.ReasoningEffort, current.Model, current.ReasoningEffort)
	}
	if choice.Apply != "next" && choice.Apply != "default" {
		return CognitiveProfile{}, fmt.Errorf("unknown cognitive resource choice %q", choice.Apply)
	}
	if err := validateProfile(r.config.CognitiveResource, profile); err != nil {
		return CognitiveProfile{}, err
	}
	if choice.Apply == "next" {
		candidate := r.activeCandidates[focusID]
		if strings.TrimSpace(choice.Purpose) == "" {
			return CognitiveProfile{}, errors.New("one serial continuation requires a purpose")
		}
		// A thought-only continuation cannot schedule another thought-only
		// continuation: without new Reality that would be an internal recursion.
		// If this cognition enacts a new body or relationship action, its result is
		// a new causal fact. In that case next legitimately names the one cognition
		// that will absorb that result, even when the present focus was itself a
		// continuation.
		if candidate.Kind == "cognition_continuation" && actionKind == "none" {
			return CognitiveProfile{}, errors.New("a thought-only serial continuation cannot continue again without new reality")
		}
	}
	return profile, nil
}

func (r *Runtime) applyResourceChoice(choice CognitiveResourceChoice, profile CognitiveProfile, focusID string) error {
	switch choice.Apply {
	case "keep":
		// "keep" follows the profile that performed this cognition. This matches
		// the lived meaning of keeping one's current cognitive mode: a one-use
		// next profile remains one-use only when Alice explicitly selects another
		// default afterward, rather than silently snapping back behind her back.
		previousModel := r.state.CognitiveResource.DefaultProfile.Model
		previousProfile := r.state.CognitiveResource.DefaultProfile
		r.state.CognitiveResource.DefaultProfile = profile
		if previousModel != "" && previousModel != profile.Model {
			r.releaseModelWaits(previousModel)
		}
		if previousProfile == profile {
			return nil
		}
		return r.journal("cognitive_profile_changed", focusID, map[string]any{
			"profile": profile, "purpose": strings.TrimSpace(choice.Purpose), "source": "keep_current",
		})
	case "default":
		previousModel := r.state.CognitiveResource.DefaultProfile.Model
		r.state.CognitiveResource.DefaultProfile = profile
		if previousModel != "" && previousModel != profile.Model {
			r.releaseModelWaits(previousModel)
		}
		return r.journal("cognitive_profile_changed", focusID, map[string]any{"profile": profile, "purpose": strings.TrimSpace(choice.Purpose)})
	case "next":
		payload, _ := json.Marshal(map[string]any{"purpose": strings.TrimSpace(choice.Purpose), "profile": profile})
		// A serial continuation is Alice continuing the present cognitive thread,
		// not a new source of life tension. Carry the already formed Concern across
		// the continuation so appraisal cannot manufacture a duplicate identity.
		concernID := r.focusConcernID(focusID)
		if err := r.addEvent("cognition_continuation", "endogenous", strings.TrimSpace(choice.Purpose), focusID, payload, true, concernID); err != nil {
			return err
		}
		continuationID := fmt.Sprintf("event-%012d", r.state.EventSeq)
		r.state.CognitiveResource.NextProfile = &NextCognitiveProfile{FocusID: continuationID, Purpose: strings.TrimSpace(choice.Purpose), Profile: profile, Source: "next"}
		return r.journal("cognitive_continuation_planned", focusID, map[string]any{"focus_id": continuationID, "profile": profile, "purpose": strings.TrimSpace(choice.Purpose)})
	default:
		return nil
	}
}

// bindNextProfileToReality turns Alice's "next" choice into the next actual
// cognition in the same causal thread. If the current focus produced an action,
// its Reality is that next cognition; a separate continuation would otherwise
// run only after Reality had already been assimilated with the default profile.
// When there is no action, the ordinary continuation event remains unchanged.
func (r *Runtime) bindNextProfileToReality(concernID, realityEventID string) error {
	next := r.state.CognitiveResource.NextProfile
	if next == nil || strings.TrimSpace(realityEventID) == "" {
		return nil
	}
	for index := range r.state.Background {
		continuation := &r.state.Background[index]
		if continuation.ID != next.FocusID || continuation.Kind != "cognition_continuation" || continuation.Status != "pending" {
			continue
		}
		if concernID != "" && continuation.ConcernID != concernID {
			return nil
		}
		continuationID := continuation.ID
		continuation.Status = "processed"
		next.FocusID = realityEventID
		return r.journal("cognition_continuation_bound", concernID, map[string]any{
			"continuation_id":  continuationID,
			"reality_event_id": realityEventID,
			"profile":          next.Profile,
			"purpose":          next.Purpose,
		})
	}
	return nil
}

func (r *Runtime) pruneInactiveConcerns() {
	kept := r.state.Concerns[:0]
	minimumConcernSalience := r.config.Dynamics.AttentionThreshold * r.config.Dynamics.ConcernBaseDrive
	for _, concern := range r.state.Concerns {
		// Sending and receiving are two different pieces of Reality.  AIP may
		// correctly say that the immediate send difference was relieved, while
		// the body still knows that the same causal thread has an unanswered
		// outbound message.  Keep that thread available as quiet background until
		// the reply arrives; it does not retain the general exploration drive.
		if concernAwaitsMentorReply(concern.ID, r.state.Commitments, r.state.Mentor) {
			kept = append(kept, concern)
			continue
		}
		if concern.Resolution == "resolved" {
			continue
		}
		if r.openCommitmentForConcern(concern.ID) != nil {
			kept = append(kept, concern)
			continue
		}
		if concernOwnsExplorationDrive(concern, r.state.Commitments, r.state.Mentor, r.config.Dynamics.AttentionThreshold) &&
			r.state.ExplorationPressure >= r.config.Dynamics.AttentionThreshold {
			kept = append(kept, concern)
			continue
		}
		if concern.Strength == 0 && concern.Resolution != "" && concern.Resolution != "hold" {
			continue
		}
		// Concern is the active part of life, not permanent semantic storage.
		// Once both accumulated strength and present activation fall below the
		// same salience floor already used by attention and associative recall,
		// a causally closed Concern becomes dormant. Its Reality and meaning stay
		// in Experience and Narrative and may become relevant again through new
		// facts; the inactive object no longer consumes the live attention field.
		if r.state.Stage >= 8 && maxFloat(concern.Strength, concern.Activation) < minimumConcernSalience {
			continue
		}
		kept = append(kept, concern)
	}
	r.state.Concerns = kept
}

// explorationHasMatureDrive marks a held exploration Concern whose accumulated
// pressure makes reality contact especially useful. It does not authorize the
// kernel to choose action over deliberate non-action; its mechanical role is
// to keep the exploration thread salient and prevent a generic drive from
// repeatedly borrowing an already-established mentor relationship.
func (r *Runtime) explorationHasMatureDrive(focusID string) bool {
	if r.state.Stage < 5 || r.state.ExplorationPressure < r.config.Dynamics.AttentionThreshold {
		return false
	}
	candidate, exists := r.activeCandidates[focusID]
	if !exists {
		return false
	}
	if candidate.Kind == "endogenous_change" {
		// A newly noticed drive is not yet a concrete concern.  Alice may act
		// immediately when the object is already clear, or first give the drive
		// a personally owned meaning.  Once that held concern returns to focus,
		// the same drive requires reality contact.  This keeps concern formation
		// and concern enactment in one thread without collapsing them into one
		// compulsory shell command.
		return false
	}
	if candidate.Kind != "concern" {
		return false
	}
	concernID := candidate.ConcernID
	if candidate.Kind == "concern" && concernID == "" {
		concernID = candidate.ID
	}
	concern := r.concernByID(concernID)
	if concern == nil || concernAwaitsMentorReply(concern.ID, r.state.Commitments, r.state.Mentor) {
		return false
	}
	return r.state.ExplorationPressure >= explorationActionThreshold(r.config.Dynamics.AttentionThreshold) &&
		concernOwnsExplorationDrive(*concern, r.state.Commitments, r.state.Mentor, r.config.Dynamics.AttentionThreshold)
}

func validateAppraisal(appraisal CandidateAppraisal) error {
	if strings.TrimSpace(appraisal.Meaning) == "" {
		return fmt.Errorf("candidate %q has no meaning", appraisal.CandidateID)
	}
	unit := []float64{appraisal.Difference, appraisal.Ownership, appraisal.Urgency, appraisal.Answerability, appraisal.Certainty}
	for _, value := range unit {
		if value < 0 || value > 1 {
			return fmt.Errorf("candidate %q has a unit value outside 0..1", appraisal.CandidateID)
		}
	}
	if appraisal.Value < -1 || appraisal.Value > 1 {
		return fmt.Errorf("candidate %q has value outside -1..1", appraisal.CandidateID)
	}
	switch appraisal.Resolution {
	case "hold", "reframed", "relieved", "resolved":
		return nil
	default:
		return fmt.Errorf("candidate %q has unknown resolution %q", appraisal.CandidateID, appraisal.Resolution)
	}
}

func validateCognitiveAction(action CognitiveAction, stage int) error {
	switch action.Kind {
	case "none":
		return nil
	case "body_shell":
		if strings.TrimSpace(action.Command) == "" {
			return errors.New("body_shell action requires a command")
		}
		if !shellActionContactsReality(action.Command) {
			return errors.New("body_shell action must read or change a body or world fact; express waiting or deliberate non-action with none")
		}
	case "mentor_send":
		if strings.TrimSpace(action.Text) == "" {
			return errors.New("mentor_send action requires text")
		}
	default:
		return fmt.Errorf("unknown cognitive action %q", action.Kind)
	}
	if stage >= 5 {
		if !meaningfulCommitmentText(action.Intent) || !meaningfulCommitmentText(action.Prediction) || !meaningfulCommitmentText(action.RealityCheck) {
			return errors.New("an enacted stage-five action requires intent, prediction, and reality_check")
		}
		if len([]rune(action.Intent)) > 1000 || len([]rune(action.Prediction)) > 1000 || len([]rune(action.RealityCheck)) > 1000 || len([]rune(action.StopCondition)) > 1000 {
			return errors.New("action commitment exceeds the compact boundary")
		}
	}
	return nil
}

func cognitiveActionContactsReality(action CognitiveAction) bool {
	switch action.Kind {
	case "mentor_send":
		return strings.TrimSpace(action.Text) != ""
	case "body_shell":
		return shellActionContactsReality(action.Command)
	default:
		return false
	}
}

// shellActionContactsReality rejects commands whose only observable effect is
// consuming time or returning success.  Waiting is a legitimate internal and
// causal state, but wrapping it in `sleep`, `true`, `:` or static output must
// not manufacture a body contact and Experience.  Any substantive command in
// the sequence remains available; this is a mechanical effect boundary, not a
// semantic classification of Alice's chosen object.
func shellActionContactsReality(request string) bool {
	request = strings.ReplaceAll(request, "\r\n", "\n")
	request = strings.TrimSpace(request)
	for request != "" {
		command, rest := leadingShellCommand(request)
		command = strings.TrimSpace(command)
		if !isShellPolicyLine(command) && !isStaticShellDecoration(command) && !isInertShellCommand(command) {
			return true
		}
		request = strings.TrimSpace(rest)
	}
	return false
}

func isInertShellCommand(command string) bool {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return true
	}
	switch fields[0] {
	case ":", "true":
		return true
	case "sleep":
		// A dynamically computed duration can itself execute or observe a
		// command, so only literal delay syntax is known to be inert.
		return len(fields) > 1 && !strings.ContainsAny(command, "$`><|&")
	default:
		return false
	}
}

func meaningfulCommitmentText(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "", "-", "none", "null", "n/a", "na", "无", "不适用":
		return false
	default:
		return true
	}
}

func appraisalActivation(dynamics Dynamics, appraisal CandidateAppraisal) float64 {
	return clamp01(
		appraisal.Difference * appraisal.Ownership * absFloat(appraisal.Value) *
			(dynamics.ConcernBaseDrive + dynamics.ConcernUrgencyWeight*appraisal.Urgency),
	)
}

func updateConcernStrength(dynamics Dynamics, previous, activation float64, resolution string, realityProgress float64) float64 {
	return clamp01(
		previous +
			dynamics.ConcernGrowthGain*activation -
			dynamics.ConcernResolutionGain*(resolutionRelief(resolution)+clamp01(realityProgress)),
	)
}

func resolutionRelief(resolution string) float64 {
	switch resolution {
	case "reframed":
		return 0.25
	case "relieved":
		return 0.60
	case "resolved":
		return 1
	default:
		return 0
	}
}

func (r *Runtime) concernForCandidate(candidate Event) *Concern {
	if concern := r.concernByID(candidate.ConcernID); concern != nil {
		return concern
	}
	if candidate.Kind == "action_result" {
		if commitment := r.commitmentByID(commitmentIDFromEvent(candidate)); commitment != nil {
			if concern := r.concernByID(commitment.ConcernID); concern != nil {
				return concern
			}
		}
	}
	for index := range r.state.Concerns {
		if r.state.Concerns[index].LastSourceID == candidate.ID {
			return &r.state.Concerns[index]
		}
	}
	return nil
}

func (r *Runtime) concernByID(id string) *Concern {
	if id == "" {
		return nil
	}
	for index := range r.state.Concerns {
		if r.state.Concerns[index].ID == id {
			return &r.state.Concerns[index]
		}
	}
	return nil
}

func (r *Runtime) linkConcern(candidateID, concernID string) {
	for index := range r.state.Background {
		if r.state.Background[index].ID == candidateID {
			r.state.Background[index].ConcernID = concernID
			return
		}
	}
}

func (r *Runtime) startStage4Action(ctx context.Context, leaseID string, action CognitiveAction) error {
	if r.state.Stage >= 5 && action.Kind != "none" {
		if action.CommitmentID == "" || r.commitmentByID(action.CommitmentID) == nil {
			return errors.New("stage-five action requires a persisted commitment")
		}
	}
	switch action.Kind {
	case "none":
		return nil
	case "mentor_send":
		actionID := "action-" + randomID()
		messageID := "alice-" + randomID()
		r.state.Mentor.Outbox = append(r.state.Mentor.Outbox, MentorMessage{
			MessageID:    messageID,
			CommitmentID: action.CommitmentID,
			Body:         strings.TrimSpace(action.Text),
			ReplyTo:      strings.TrimSpace(action.ReplyTo),
			Status:       "queued",
			QueuedAt:     nowUTC(),
		})
		if commitment := r.commitmentByID(action.CommitmentID); commitment != nil {
			commitment.ActionID = actionID
			commitment.Status = "reality_available"
		}
		if err := r.journal("mentor_queued", messageID, map[string]any{"body": action.Text, "reply_to": action.ReplyTo, "action_id": actionID, "commitment_id": action.CommitmentID}); err != nil {
			return err
		}
		payload, _ := json.Marshal(ActionState{ID: actionID, LeaseID: leaseID, CommitmentID: action.CommitmentID, Kind: "mentor_send", Request: action.Text, Status: "completed", StartedAt: nowUTC(), EndedAt: nowUTC(), Result: messageID})
		if err := r.addEvent("action_result", "observed", "一条导师消息已经进入可信通道的发送队列。", actionID, payload, true); err != nil {
			return err
		}
		realityEventID := fmt.Sprintf("event-%012d", r.state.EventSeq)
		if commitment := r.commitmentByID(action.CommitmentID); commitment != nil {
			commitment.RealityEventID = realityEventID
			if err := r.bindNextProfileToReality(commitment.ConcernID, realityEventID); err != nil {
				return err
			}
		}
		return r.persist()
	case "body_shell":
		if r.state.PendingAction != nil {
			return errors.New("another body action is already in progress")
		}
		actionID := "action-" + randomID()
		r.state.PendingAction = &ActionState{
			ID:           actionID,
			LeaseID:      leaseID,
			CommitmentID: action.CommitmentID,
			Kind:         "body_shell",
			Request:      action.Command,
			Status:       "started",
			StartedAt:    nowUTC(),
		}
		if commitment := r.commitmentByID(action.CommitmentID); commitment != nil {
			commitment.ActionID = actionID
			commitment.Status = "acting"
		}
		if err := r.journal("action_started", actionID, map[string]any{"kind": "body_shell", "command": action.Command, "timeout_seconds": 120, "commitment_id": action.CommitmentID}); err != nil {
			return err
		}
		if err := r.persist(); err != nil {
			return err
		}
		go func() {
			result := ActionResultNotice{
				ActionID: actionID,
				Result: redactRuntimeSecret(
					executeShell(ctx, action.Command, 120*time.Second),
					r.config.ModelGateway.APIKey,
				),
			}
			select {
			case r.actionResults <- result:
			case <-ctx.Done():
			}
		}()
		return nil
	default:
		return fmt.Errorf("unsupported stage-four action %q", action.Kind)
	}
}

func (r *Runtime) handleStage4ActionResult(ctx context.Context, result ActionResultNotice) error {
	if r.state.PendingAction == nil || r.state.PendingAction.ID != result.ActionID {
		return r.journal("late_action_result", result.ActionID, map[string]any{"result": truncate(result.Result, 2048)})
	}
	completed := *r.state.PendingAction
	completed.Status = "completed"
	completed.EndedAt = nowUTC()
	completed.Result = truncate(result.Result, 64*1024)
	if err := r.journal("action_completed", completed.ID, map[string]any{"kind": completed.Kind, "result": completed.Result}); err != nil {
		return err
	}
	payload, _ := json.Marshal(completed)
	if err := r.addEvent("action_result", "observed", "一项身体行动已经完成，并返回了真实结果。", completed.ID, payload, true); err != nil {
		return err
	}
	realityEventID := fmt.Sprintf("event-%012d", r.state.EventSeq)
	if commitment := r.commitmentByID(completed.CommitmentID); commitment != nil {
		commitment.Status = "reality_available"
		commitment.RealityEventID = realityEventID
		if err := r.bindNextProfileToReality(commitment.ConcernID, realityEventID); err != nil {
			return err
		}
	}
	r.state.PendingAction = nil
	r.state.Revision++
	if r.state.Stage >= 5 {
		if err := r.syncSelfFromFiles(); err != nil {
			return err
		}
	}
	if err := r.persist(); err != nil {
		return err
	}
	r.maybeStartCognition(ctx)
	return nil
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func maxFloat(left, right float64) float64 {
	if left > right {
		return left
	}
	return right
}
