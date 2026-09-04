package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
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
	content := ""
	if event.Kind == "perceptual_change" {
		var p struct {
			Content string `json:"content"`
		}
		_ = json.Unmarshal(event.Payload, &p)
		content = p.Content
	}
	changed := 1.0
	predictionGap := 1.0
	if trace.Observations > 0 {
		if digest == trace.LastDigest {
			changed = 0
		} else if content != "" && trace.LastContent != "" {
			changed = perceptualChangeMagnitude(trace.LastContent, content)
		}
		predictionGap = absFloat(changed - trace.ExpectedChangeRate)
		if content != "" && trace.LastContent != "" {
			// A tiny ticker/control change is a real change, but not an entire
			// unfamiliar world. It accumulates without a semantic veto.
			predictionGap *= changed
		}
	}
	if event.Kind == "operation_condition" && trace.Operation != nil {
		if trace.Operation.Status != "recovered" {
			// A failed passive operation begins as a small bodily discrepancy.
			// Persistence accumulates via the existing pressure/decay mechanism;
			// this is attention eligibility, never an estimate of actual harm.
			predictionGap = 1 / (1 + float64(trace.Operation.ConsecutiveFailures))
		} else {
			predictionGap = 1 // A real recovery changes the available route.
		}
	}
	learningRate := r.config.Dynamics.DifferenceLearningRate
	trace.ExpectedChangeRate = clamp01(
		trace.ExpectedChangeRate + learningRate*(changed-trace.ExpectedChangeRate),
	)
	trace.Observations++
	trace.LastDigest = digest
	trace.LastContent = content
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
	// Below-threshold sampling must not oscillate forever while the conscious
	// stream is genuinely quiet.  Wall-clock continuity contributes a bounded
	// gain only when a real current perceptual object or value doorway is present;
	// it can make that referent noticeable, never invent a subject or an action.
	continuityEvidence := r.lifeContinuityAttentionEvidence(event)
	evidence := predictionGap*contextGain + openSamplingEvidence + expectedValueEvidence +
		differenceCausalPressure(event.Kind) + continuityEvidence
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

func perceptualChangeMagnitude(previous, current string) float64 {
	if previous == current {
		return 0
	}
	left, right := recallTerms(previous), recallTerms(current)
	common := 0
	for term := range left {
		if right[term] {
			common++
		}
	}
	union := len(left) + len(right) - common
	if union == 0 {
		return 1
	}
	return 1 - float64(common)/float64(union)
}

func (r *Runtime) lifeContinuityAttentionEvidence(event Event) float64 {
	if r.state.Stage < 10 || !eventCarriesRealAttentionReferent(event) {
		return 0
	}
	now := time.Now().UTC()
	if observed, err := time.Parse(time.RFC3339Nano, event.ObservedAt); err == nil {
		now = observed
	}
	quiet, ok := r.attentionQuietDuration(now)
	if !ok {
		return 0
	}
	idle := r.config.Dynamics.AttentionMaximumIdleSeconds
	if idle <= 0 {
		idle = 10
	}
	boundary := time.Duration(idle) * time.Second
	if quiet <= boundary {
		return 0
	}
	// The gain rises from zero after the ordinary idle boundary to one complete
	// attention threshold after two additional windows, then stays bounded.
	progress := clamp01(float64(quiet-boundary) / float64(2*boundary))
	return r.config.Dynamics.AttentionThreshold * progress
}

func eventCarriesRealAttentionReferent(event Event) bool {
	switch event.Kind {
	case "perceptual_change":
		var payload struct {
			ObjectID string `json:"object_id"`
			Content  string `json:"content"`
		}
		return json.Unmarshal(event.Payload, &payload) == nil &&
			strings.TrimSpace(payload.ObjectID) != "" && strings.TrimSpace(payload.Content) != ""
	case "value_signal":
		var payload struct {
			AffordanceKey string `json:"affordance_key"`
			Surface       string `json:"surface"`
		}
		return json.Unmarshal(event.Payload, &payload) == nil &&
			strings.TrimSpace(payload.AffordanceKey) != "" && strings.TrimSpace(payload.Surface) != ""
	default:
		return false
	}
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
	case "action_result", "action_boundary", "birth_orientation":
		return 1
	case "cognition_continuation", "cognition_assistance_result":
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
		MemoryID       string `json:"memory_id"`
	}
	var payload signalPayload
	_ = json.Unmarshal(event.Payload, &payload)
	parts := []string{strings.TrimSpace(event.Source), strings.TrimSpace(event.Kind)}
	switch event.Kind {
	case "perceptual_change":
		parts = append(parts, payload.OrganID, payload.SurfaceID)
	case "action_result", "operation_condition":
		parts = append(parts, payload.OrganID, payload.Operation)
	case "value_signal":
		parts = append(parts, payload.Direction, payload.AffordanceKey)
	case "self_model_difference":
		parts = append(parts, payload.DifferenceKind)
	case "lived_recall":
		parts = append(parts, payload.MemoryID)
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

func (r *Runtime) learnDifferenceFromMemory(commitment ActionCommitment, memory Memory) {
	candidate, exists := r.backgroundEvent(commitment.FocusID)
	if !exists || candidate.DifferenceKey == "" {
		return
	}
	trace, exists := r.state.DifferenceField[candidate.DifferenceKey]
	if !exists {
		return
	}
	consequence := maxLifeValueMagnitude(lifeValuesVector(memory.Values))
	consequence = maxFloat(consequence, absFloat(memory.Values.SelfEndorsed))
	memoryGain := 0.0
	if memory.Significance == "reusable" {
		memoryGain = 0.5
	} else if memory.Significance == "self_defining" {
		memoryGain = 1
	}
	target := clamp01(
		0.35*memory.PredictionDifference +
			0.35*consequence +
			0.15*(1-memory.ExperiencedCost) +
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
