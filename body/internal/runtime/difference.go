package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

const (
	initialDifferenceAttentionValue = 0.50
	maximumDifferenceTraces         = 128
)

// admitDifference is the single pre-conscious admission gate for factual and
// endogenous signals. It compresses repeated observations into a stable trace
// and only opens a global-attention candidate after unresolved pressure crosses
// the ordinary attention threshold. Causal facts use the same equation with a
// factual relationship gain; they do not travel through a second cognition
// channel.
func (r *Runtime) admitDifference(event Event) (Event, bool) {
	if r.state.DifferenceField == nil {
		r.state.DifferenceField = make(map[string]DifferenceTrace)
	}
	key := differenceFamilyKey(event)
	trace, exists := r.state.DifferenceField[key]
	if !exists {
		for len(r.state.DifferenceField) >= maximumDifferenceTraces {
			r.evictDifferenceTrace()
		}
		trace = DifferenceTrace{
			Key: key, ExpectedChangeRate: 0.50,
			AttentionValue: initialDifferenceAttentionValue,
		}
	}

	digest := differenceDigest(event)
	changed := 1.0
	predictionGap := 1.0
	if trace.Observations > 0 {
		if digest == trace.LastDigest {
			changed = 0
		}
		predictionGap = absFloat(changed - trace.ExpectedChangeRate)
	}
	learningRate := r.config.Dynamics.DifferenceLearningRate
	trace.ExpectedChangeRate = clamp01(
		trace.ExpectedChangeRate + learningRate*(changed-trace.ExpectedChangeRate),
	)
	trace.Observations++
	trace.LastDigest = digest
	trace.LastObservedAt = event.ObservedAt
	trace.LastPredictionGap = predictionGap

	// A non-zero base keeps unfamiliar sources open. Learned attention value
	// then makes previously consequential signal families easier to ignite and
	// repeatedly low-yield families slower, without making either permanent.
	contextGain := 0.25 + 0.50*trace.AttentionValue
	// Surprise opens the unknown. A small epistemic sampling floor prevents a
	// learned-low, regularly changing source from becoming a permanent blind
	// spot; it accumulates slowly instead of waking cognition per observation.
	// Learned value then shortens that sampling interval for sources whose past
	// appearances changed Alice's understanding or action.
	openSamplingEvidence := 0.03 * changed
	expectedValueEvidence := 0.12 * trace.AttentionValue * changed
	evidence := predictionGap*contextGain + openSamplingEvidence + expectedValueEvidence + differenceCausalPressure(event.Kind)
	pressure := clamp01(trace.Accumulated + evidence)
	event.DifferenceKey = key
	event.PredictionGap = predictionGap
	event.AttentionPressure = pressure
	if pressure >= r.config.Dynamics.AttentionThreshold {
		trace.Accumulated = 0
		trace.LastIgnitedAt = event.ObservedAt
		r.state.DifferenceField[key] = trace
		return event, true
	}
	trace.Accumulated = pressure
	r.state.DifferenceField[key] = trace
	return event, false
}

// evictDifferenceTrace keeps the pre-conscious field physically bounded even
// when an organ reports malformed or unbounded family names. Retention depends
// only on accumulated pressure and learned attention access; it does not infer
// meaning. Deterministic ties make replay behavior stable.
func (r *Runtime) evictDifferenceTrace() {
	victim := ""
	victimRetention := 2.0
	victimObservedAt := ""
	for key, trace := range r.state.DifferenceField {
		retention := 0.55*trace.AttentionValue + 0.45*trace.Accumulated
		if victim == "" || retention < victimRetention ||
			(retention == victimRetention && (trace.LastObservedAt < victimObservedAt ||
				(trace.LastObservedAt == victimObservedAt && key < victim))) {
			victim = key
			victimRetention = retention
			victimObservedAt = trace.LastObservedAt
		}
	}
	if victim != "" {
		delete(r.state.DifferenceField, victim)
	}
}

// differenceCausalPressure describes facts whose relationship to Alice's
// present causal line is already mechanically known. It is not importance or
// meaning: an action result answers an outstanding action whether the result is
// good or bad; birth is the active genesis fact; a reply is addressed input.
func differenceCausalPressure(kind string) float64 {
	switch kind {
	case "action_result", "birth_orientation":
		return 1
	case "cognition_continuation":
		return 0.75
	case "mentor_received", "mentor_content", "concern_contribution":
		return 0.55
	case "cognitive_resource_change", "integrity_mirror", "self_model_difference":
		return 0.40
	case "body_delta", "environment_change", "reality_consequence":
		return 0.25
	default:
		return 0
	}
}

func differenceFamilyKey(event Event) string {
	type signalPayload struct {
		OrganID        string `json:"organ_id"`
		SurfaceID      string `json:"surface_id"`
		Operation      string `json:"operation"`
		Direction      string `json:"value_direction"`
		AffordanceKey  string `json:"affordance_key"`
		DifferenceKind string `json:"difference_kind"`
	}
	var payload signalPayload
	_ = json.Unmarshal(event.Payload, &payload)
	parts := []string{strings.TrimSpace(event.Source), strings.TrimSpace(event.Kind)}
	switch event.Kind {
	case "perceptual_change":
		parts = append(parts, payload.OrganID, payload.SurfaceID)
	case "action_result":
		parts = append(parts, payload.OrganID, payload.Operation)
	case "value_signal":
		parts = append(parts, payload.Direction, payload.AffordanceKey)
	case "self_model_difference":
		parts = append(parts, payload.DifferenceKind)
	}
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	return strings.Join(parts, "/")
}

func differenceDigest(event Event) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(event.Summary) + "\n" + strings.TrimSpace(string(event.Payload))))
	return hex.EncodeToString(digest[:])
}

func (r *Runtime) decayDifferenceField(elapsedMinutes float64) {
	if elapsedMinutes <= 0 || len(r.state.DifferenceField) == 0 {
		return
	}
	factor := clamp01(1 - r.config.Dynamics.DifferenceDecayRate*elapsedMinutes)
	for key, trace := range r.state.DifferenceField {
		trace.Accumulated = clamp01(trace.Accumulated * factor)
		r.state.DifferenceField[key] = trace
	}
}

func candidateDifferencePressure(candidate Event) float64 {
	if candidate.AttentionPressure > 0 {
		return clamp01(candidate.AttentionPressure)
	}
	if candidate.Kind != "concern" {
		return 1
	}
	return 0
}

// learnDifferenceFromAppraisal lets Alice's own AIP tune future access to
// attention. A negative or difficult fact can remain valuable; magnitude,
// ownership and answerability matter, not whether the appraisal is pleasant.
func (r *Runtime) learnDifferenceFromAppraisal(candidate Event, appraisal CandidateAppraisal, acted bool) {
	if candidate.DifferenceKey == "" {
		return
	}
	trace, exists := r.state.DifferenceField[candidate.DifferenceKey]
	if !exists {
		return
	}
	target := clamp01(appraisal.Ownership * (0.40*absFloat(appraisal.Value) +
		0.35*appraisal.Answerability + 0.25*appraisal.Certainty))
	if acted {
		target = maxFloat(target, 0.60*appraisal.Ownership)
	}
	r.learnDifferenceAttentionValue(candidate.DifferenceKey, trace, target)
}

func (r *Runtime) learnDifferenceFromExperience(commitment ActionCommitment, experience Experience) {
	candidate, exists := r.backgroundEvent(commitment.FocusID)
	if !exists || candidate.DifferenceKey == "" {
		return
	}
	trace, exists := r.state.DifferenceField[candidate.DifferenceKey]
	if !exists {
		return
	}
	consequence := maxLifeValueMagnitude(lifeValuesVector(experience.Values))
	consequence = maxFloat(consequence, absFloat(experience.Values.SelfEndorsed))
	memoryGain := 0.0
	if experience.Significance == "reusable" {
		memoryGain = 0.5
	} else if experience.Significance == "self_defining" {
		memoryGain = 1
	}
	target := clamp01(
		0.35*experience.PredictionDifference +
			0.35*consequence +
			0.15*(1-experience.ExperiencedCost) +
			0.15*memoryGain,
	)
	r.learnDifferenceAttentionValue(candidate.DifferenceKey, trace, target)
}

func (r *Runtime) learnDifferenceAttentionValue(key string, trace DifferenceTrace, target float64) {
	rate := r.config.Dynamics.DifferenceLearningRate
	trace.AttentionValue = clamp01(trace.AttentionValue + rate*(target-trace.AttentionValue))
	r.state.DifferenceField[key] = trace
}

func (r *Runtime) backgroundEvent(id string) (Event, bool) {
	for _, event := range r.state.Background {
		if event.ID == id {
			return event, true
		}
	}
	return Event{}, false
}
