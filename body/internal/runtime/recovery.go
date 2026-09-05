package runtime

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// A failed interpretation is not an executing body action. Keep its original
// Reality pending while a bounded local retry yields to unrelated cognition.
func (r *Runtime) locallyDeferredReality(event Event) bool {
	return r.state.Stage >= 10 && event.Kind == "action_result" &&
		event.CognitionAttempts > r.config.CognitiveResource.ValidationRetryPerFocus &&
		event.Status == "retry_wait"
}

// Failed passive reads need a path into the same attention field as successful
// observations. Enacted actions already have causal Reality, so they update
// this body context without emitting another attention candidate.
func (r *Runtime) recordOperationOutcome(id, operation, outcome, detail string, notify bool) error {
	if r.state.Stage < 10 {
		return nil
	}
	condition := OperationCondition{OrganID: id, Operation: operation, Status: "recovered", ObservedAt: nowUTC()}
	payload, _ := json.Marshal(condition)
	key := differenceFamilyKey(Event{Kind: "operation_condition", Source: "observed", Payload: payload})
	trace, exists := r.state.DifferenceField[key]
	if outcome == "completed" && (!exists || trace.Operation == nil || trace.Operation.Status == "recovered") {
		return nil
	}
	if trace.Operation != nil {
		condition.FailureSince = trace.Operation.FailureSince
		condition.LastError = trace.Operation.LastError
	}
	if outcome != "completed" {
		condition.Status = outcome
		condition.LastError = truncate(redactRuntimeSecret(detail, r.config.ModelGateway.APIKey), 600)
		condition.ConsecutiveFailures = 1
		if trace.Operation != nil && trace.Operation.Status != "recovered" {
			condition.ConsecutiveFailures += trace.Operation.ConsecutiveFailures
		} else {
			condition.FailureSince = condition.ObservedAt
		}
	}
	if r.state.DifferenceField == nil {
		r.state.DifferenceField = map[string]DifferenceTrace{}
	}
	if !exists {
		for len(r.state.DifferenceField) >= maximumDifferenceTraces {
			r.evictDifferenceTrace()
		}
		trace = DifferenceTrace{Key: key, ExpectedChangeRate: .5, AttentionValue: initialDifferenceAttentionValue}
	}
	trace.Operation = &condition
	r.state.DifferenceField[key] = trace

	// One outstanding candidate per operation; newer outcomes remain visible in
	// current_situation. Never mutate the event on which a running model reasons.
	for _, event := range r.state.Background {
		if event.DifferenceKey == key && eventChainActive(event.Status) {
			notify = false
		}
	}
	if outcome == "completed" && trace.LastIgnitedAt == "" {
		notify = false // A transient incident recovered before needing attention.
	}
	payload, _ = json.Marshal(condition)
	status := "已恢复"
	if outcome == "failed" {
		status = "遇到困难"
	} else if outcome == "unknown" {
		status = "结果尚未确认"
	}
	summary := fmt.Sprintf("器官 %s 的 %s 操作当前%s；实际错误、持续时间和次数见事实。", id, operation, status)
	return r.addEvent("operation_condition", "observed", summary, "", payload, notify)
}

func operationConditionViews(state State) []OperationCondition {
	var conditions []OperationCondition
	for _, trace := range state.DifferenceField {
		if trace.Operation != nil {
			conditions = append(conditions, *trace.Operation)
		}
	}
	sort.Slice(conditions, func(i, j int) bool {
		if conditions[i].ObservedAt != conditions[j].ObservedAt {
			return conditions[i].ObservedAt > conditions[j].ObservedAt
		}
		return strings.Compare(conditions[i].OrganID+"/"+conditions[i].Operation, conditions[j].OrganID+"/"+conditions[j].Operation) < 0
	})
	if len(conditions) > 6 {
		conditions = conditions[:6]
	}
	return conditions
}

func (r *Runtime) realityRetryDelay(event Event) time.Duration {
	if !r.locallyDeferredReality(event) {
		return cognitionRetryDelay
	}
	delay := 2 * cognitionRetryDelay
	limit := time.Duration(maxInt(r.config.CognitiveResource.ModelProtectionMinutes, 1)) * time.Minute
	// Repeating the same structural interpretation error has sharply
	// diminishing information value. Keep one short recovery chance, then
	// spread later attempts quickly while the captured Reality remains durable.
	for n := event.CognitionAttempts - r.config.CognitiveResource.ValidationRetryPerFocus - 1; n > 0 && delay < limit; n-- {
		delay *= 3
	}
	if delay > limit {
		delay = limit
	}
	return delay
}

func (r *Runtime) deferredSettledAction(event Event) (*ActionState, bool) {
	if !r.locallyDeferredReality(event) {
		return nil, false
	}
	var action ActionState
	if json.Unmarshal(event.Payload, &action) != nil {
		return nil, false
	}
	return &action, action.Status == "completed" || action.Status == "failed"
}

func (r *Runtime) commitmentLocallyDeferred(commitment ActionCommitment) bool {
	event, exists := r.backgroundEvent(commitment.RealityEventID)
	return exists && r.locallyDeferredReality(event)
}

// Scope is conservatively the existing organ and Concern, not an invented
// shell read/write dependency analysis. Unknown outcomes keep this boundary.
func (r *Runtime) validateUnabsorbedActionScope(commit CognitiveCommit, concernID string) error {
	if r.state.Stage < 10 || commit.Action.Kind == "none" {
		return nil
	}
	for _, c := range r.state.Commitments {
		if c.Status != "reality_available" && c.Status != "reality_unknown" || commitAssimilates(commit, c.ID) {
			continue
		}
		event, exists := r.backgroundEvent(c.RealityEventID)
		var action ActionState
		if !exists || json.Unmarshal(event.Payload, &action) != nil {
			return newActionProgressBoundary("an unabsorbed action has no usable result identity; recover that factual record first")
		}
		if settled, released := r.deferredSettledAction(event); released {
			kind, request := cognitiveActionRequest(commit.Action)
			sameRequest := kind == settled.Kind && request != "" && request == normalizedActionStateRequest(*settled)
			sameConcern := c.ConcernID != "" && c.ConcernID == concernID
			if !sameRequest && !sameConcern {
				// The immutable result remains available for later interpretation,
				// but a completed or failed action no longer occupies the physical
				// organ. A distinct causal thread can continue living.
				continue
			}
		}
		if c.ConcernID != "" && c.ConcernID == concernID ||
			(commit.Action.Kind == "organ_action" && action.Kind == "organ_action" && action.OrganID == commit.Action.OrganID) ||
			(commit.Action.Kind == "mentor_send" && action.Kind == "mentor_send" && commit.Action.Text == action.Request) {
			return newActionProgressBoundary(fmt.Sprintf("result %s still awaits interpretation; this action shares its Concern, organ or exact external request", event.ID))
		}
	}
	return nil
}

func (r *Runtime) organHasUnabsorbedReality(id string) bool {
	for _, c := range r.state.Commitments {
		if c.Status != "reality_available" && c.Status != "reality_unknown" {
			continue
		}
		event, exists := r.backgroundEvent(c.RealityEventID)
		if _, released := r.deferredSettledAction(event); exists && released {
			continue
		}
		var action ActionState
		if exists && json.Unmarshal(event.Payload, &action) == nil && action.OrganID == id {
			return true
		}
	}
	return false
}

func deferredRealityViews(request CognitiveRequest) []map[string]any {
	r := Runtime{state: request.State, config: request.Config}
	var views []map[string]any
	for _, event := range request.State.Background {
		if !r.locallyDeferredReality(event) {
			continue
		}
		var action ActionState
		_ = json.Unmarshal(event.Payload, &action)
		next, _ := time.Parse(time.RFC3339Nano, event.LastFocusedAt)
		views = append(views, map[string]any{
			"event_id": event.ID, "commitment_id": action.CommitmentID,
			"organ_id": action.OrganID, "operation": action.Operation,
			"result_status": action.Status, "interpretation_error": event.LastCommitErr,
			"attempts": event.CognitionAttempts, "next_attempt_at": next.Add(r.realityRetryDelay(event)).Format(time.RFC3339Nano),
		})
	}
	return views
}
