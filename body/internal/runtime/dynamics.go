package runtime

import (
	"context"
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
	}

	before := r.state.ExplorationPressure
	if r.state.Lease == nil && r.state.PendingAction == nil {
		r.state.ExplorationPressure = clamp01(before + r.config.Dynamics.ExplorationIdleGrowth*minutes)
	}
	crossed := before < r.config.Dynamics.AttentionThreshold && r.state.ExplorationPressure >= r.config.Dynamics.AttentionThreshold
	revisit := r.state.ExplorationPressure >= r.config.Dynamics.AttentionThreshold &&
		attentionDue(r.state.LastAttentionAt, time.Now().UTC(), r.config.Dynamics.AttentionRevisitSeconds)
	if (crossed || revisit) && !r.explorationCandidateActive() {
		summary := "探索张力已经积蓄到值得重新接触现实。"
		concernID := ""
		if revisit {
			summary = "探索张力持续存在，正在再次进入注意；它会在现实接触得到解释后变化。"
			concernID = r.currentExplorationConcernID()
		}
		payload, _ := json.Marshal(map[string]any{
			"before":     before,
			"after":      r.state.ExplorationPressure,
			"is_revisit": revisit,
		})
		return r.addEvent(
			"endogenous_change",
			"endogenous",
			summary,
			"",
			payload,
			true,
			concernID,
		)
	}
	return nil
}

func (r *Runtime) currentExplorationConcernID() string {
	for index := len(r.state.Concerns) - 1; index >= 0; index-- {
		concern := r.state.Concerns[index]
		if concern.OriginKind == "endogenous_change" && concern.Resolution != "resolved" {
			return concern.ID
		}
	}
	for index := len(r.state.Background) - 1; index >= 0; index-- {
		event := r.state.Background[index]
		if event.Kind == "endogenous_change" && event.ConcernID != "" && r.concernByID(event.ConcernID) != nil {
			return event.ConcernID
		}
	}
	return ""
}

func (r *Runtime) explorationCandidateActive() bool {
	for _, event := range r.state.Background {
		if event.Kind == "endogenous_change" && (event.Status == "pending" || event.Status == "in_focus" || event.Status == "retry_wait") {
			return true
		}
	}
	return false
}

func (r *Runtime) nextStage4Request() (CognitiveRequest, bool) {
	if r.state.PendingAction != nil {
		return CognitiveRequest{}, false
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
		candidate := Event{
			ID:            concern.ID,
			Kind:          "concern",
			Source:        "endogenous",
			ObservedAt:    concern.UpdatedAt,
			Summary:       concern.Meaning,
			Status:        "pending",
			ConcernID:     concern.ID,
			LastFocusedAt: concern.LastFocusedAt,
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
	limit := r.config.Dynamics.AttentionCandidateLimit
	if limit <= 0 || limit > defaultAttentionCandidateLimit {
		limit = defaultAttentionCandidateLimit
	}
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	return CognitiveRequest{Stage: 4, Focus: candidates[0], Candidates: candidates}, true
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
	}
	if candidate.Kind == "endogenous_change" || strings.Contains(strings.ToLower(candidate.Summary), "exploration") {
		explorationValue = r.state.ExplorationPressure
		expectedCost = 0.15
	}
	return concernStrength +
		r.config.Dynamics.AttentionAffectWeight*affectiveSalience +
		r.config.Dynamics.AttentionExplorationWeight*explorationValue +
		r.config.Dynamics.AttentionNoveltyWeight*novelty -
		r.config.Dynamics.AttentionCostWeight*expectedCost
}

func (r *Runtime) applyCognitiveCommit(commit CognitiveCommit) error {
	if len(commit.Appraisals) == 0 || len(commit.Appraisals) > defaultAttentionCandidateLimit {
		return errors.New("cognitive commit must contain one to three appraisals")
	}
	if _, exists := r.activeCandidates[commit.FocusID]; !exists {
		return fmt.Errorf("focus %q is not an active candidate", commit.FocusID)
	}
	if len([]rune(strings.TrimSpace(commit.ThoughtThread))) > 2000 {
		return errors.New("thought thread is too large for a single attention pulse")
	}
	if err := validateCognitiveAction(commit.Action); err != nil {
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
	explorationResultResolution := ""
	for _, appraisal := range commit.Appraisals {
		candidate := r.activeCandidates[appraisal.CandidateID]

		activation := appraisalActivation(r.config.Dynamics, appraisal)
		concern := r.concernForCandidate(candidate)
		if concern == nil {
			r.state.Concerns = append(r.state.Concerns, Concern{ID: "concern-" + randomID()})
			concern = &r.state.Concerns[len(r.state.Concerns)-1]
		}
		concern.Subject = truncate(candidate.Summary, 512)
		if candidate.Kind != "concern" || concern.OriginKind == "" {
			concern.OriginKind = candidate.Kind
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
		concern.Resolution = strings.TrimSpace(appraisal.Resolution)
		concern.Strength = updateConcernStrength(r.config.Dynamics, concern.Strength, activation, appraisal.Resolution)
		if appraisal.CandidateID == commit.FocusID {
			concern.LastFocusedAt = now
		}
		r.linkConcern(candidate.ID, concern.ID)
		if candidate.Kind != "concern" && appraisal.CandidateID != commit.FocusID {
			markEvent(&r.state, appraisal.CandidateID, "background")
		}
		if candidate.Kind == "action_result" {
			resolution := resolutionRelief(appraisal.Resolution)
			if resolution > explorationResultRelief {
				explorationResultRelief = resolution
				explorationResultResolution = appraisal.Resolution
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
		r.relieveExplorationConcerns(explorationResultResolution, explorationResultRelief)
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
	return r.journal("aip_commit", commit.FocusID, map[string]any{
		"focus_id":        commit.FocusID,
		"thought_thread":  truncate(strings.TrimSpace(commit.ThoughtThread), 2000),
		"appraisals":      commit.Appraisals,
		"affective_state": r.state.AffectiveState,
		"action_kind":     commit.Action.Kind,
	})
}

func (r *Runtime) relieveExplorationConcerns(resolution string, relief float64) {
	if relief <= 0 {
		return
	}
	for index := range r.state.Concerns {
		concern := &r.state.Concerns[index]
		if concern.OriginKind != "endogenous_change" {
			continue
		}
		concern.Strength = clamp01(concern.Strength - r.config.Dynamics.ConcernResolutionGain*relief)
		concern.Resolution = resolution
	}
}

func (r *Runtime) pruneInactiveConcerns() {
	kept := r.state.Concerns[:0]
	for _, concern := range r.state.Concerns {
		if concern.Strength == 0 && concern.Resolution != "" && concern.Resolution != "hold" {
			continue
		}
		kept = append(kept, concern)
	}
	r.state.Concerns = kept
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

func validateCognitiveAction(action CognitiveAction) error {
	switch action.Kind {
	case "none":
		return nil
	case "body_shell":
		if strings.TrimSpace(action.Command) == "" {
			return errors.New("body_shell action requires a command")
		}
		return nil
	case "mentor_send":
		if strings.TrimSpace(action.Text) == "" {
			return errors.New("mentor_send action requires text")
		}
		return nil
	default:
		return fmt.Errorf("unknown cognitive action %q", action.Kind)
	}
}

func appraisalActivation(dynamics Dynamics, appraisal CandidateAppraisal) float64 {
	return clamp01(
		appraisal.Difference * appraisal.Ownership * absFloat(appraisal.Value) *
			(dynamics.ConcernBaseDrive + dynamics.ConcernUrgencyWeight*appraisal.Urgency),
	)
}

func updateConcernStrength(dynamics Dynamics, previous, activation float64, resolution string) float64 {
	return clamp01(previous + dynamics.ConcernGrowthGain*activation - dynamics.ConcernResolutionGain*resolutionRelief(resolution))
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
	switch action.Kind {
	case "none":
		return nil
	case "mentor_send":
		actionID := "action-" + randomID()
		messageID := "alice-" + randomID()
		r.state.Mentor.Outbox = append(r.state.Mentor.Outbox, MentorMessage{
			MessageID: messageID,
			Body:      strings.TrimSpace(action.Text),
			ReplyTo:   strings.TrimSpace(action.ReplyTo),
			Status:    "queued",
			QueuedAt:  nowUTC(),
		})
		if err := r.journal("mentor_queued", messageID, map[string]any{"body": action.Text, "reply_to": action.ReplyTo, "action_id": actionID}); err != nil {
			return err
		}
		payload, _ := json.Marshal(map[string]any{"action_id": actionID, "message_id": messageID, "status": "queued"})
		if err := r.addEvent("action_result", "observed", "一条导师消息已经进入可信通道的发送队列。", actionID, payload, true); err != nil {
			return err
		}
		return r.persist()
	case "body_shell":
		if r.state.PendingAction != nil {
			return errors.New("another body action is already in progress")
		}
		actionID := "action-" + randomID()
		r.state.PendingAction = &ActionState{
			ID:        actionID,
			LeaseID:   leaseID,
			Kind:      "body_shell",
			Request:   action.Command,
			Status:    "started",
			StartedAt: nowUTC(),
		}
		if err := r.journal("action_started", actionID, map[string]any{"kind": "body_shell", "command": action.Command, "timeout_seconds": 120}); err != nil {
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
					r.config.Model.APIKey,
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
	r.state.PendingAction = nil
	r.state.Revision++
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
