package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"hominal.cc/hominal/body/internal/organ"
)

const diskEventThreshold = 64 * 1024 * 1024
const perceptualContentLimit = 12 * 1024

type perceptualObservation struct {
	OrganID    string
	SurfaceID  string
	ObservedAt string
	Digest     string
	Context    []string
	Objects    []PerceptualObject
}

func collectSnapshot(config Config, state State, slow bool, organs *organ.Registry) BodySnapshot {
	return collectSnapshotContext(context.Background(), config, state, slow, organs)
}

func collectSnapshotContext(parent context.Context, config Config, state State, slow bool, organs *organ.Registry) BodySnapshot {
	current := mergeFastSnapshot(state.Body, BodySnapshot{ObservedAt: nowUTC()})
	updateResourceSnapshot(&current, state, config.CognitiveResource, time.Now().UTC())
	if !slow {
		return current
	}
	current.Organs = make(map[string]OrganSnapshot)
	if organs != nil {
		for id, snapshot := range organs.SnapshotContext(parent) {
			current.Organs[id] = OrganSnapshot{
				Name: snapshot.Name, Command: snapshot.Command,
				Capabilities:    append([]string{}, snapshot.Capabilities...),
				Operations:      append([]string{}, snapshot.Operations...),
				OperationInputs: cloneRuntimeStringMap(snapshot.OperationInputs), Guidance: snapshot.Guidance,
				Status: snapshot.Status, Accepting: snapshot.Accepting,
			}
		}
		for _, id := range organs.BodyStateIDs() {
			ctx, cancel := context.WithTimeout(parent, 12*time.Second)
			observation, err := organs.Observe(ctx, id)
			cancel()
			if err == nil {
				applyBodyFacts(&current, observation.Facts)
			}
		}
	}
	return current
}

func cloneRuntimeStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func mergeFastSnapshot(previous, fast BodySnapshot) BodySnapshot {
	fast.UptimeSeconds = previous.UptimeSeconds
	fast.RootFreeBytes = previous.RootFreeBytes
	fast.AgentFreeBytes = previous.AgentFreeBytes
	fast.NetworkAvailable = previous.NetworkAvailable
	fast.NetworkProbe = previous.NetworkProbe
	fast.DesktopAvailable = previous.DesktopAvailable
	fast.Organs = previous.Organs
	fast.WechatRunning = previous.WechatRunning
	fast.ClashVergeRunning = previous.ClashVergeRunning
	return fast
}

func applyBodyFacts(snapshot *BodySnapshot, facts map[string]json.RawMessage) {
	decodeBodyFact(facts, "uptime_seconds", &snapshot.UptimeSeconds)
	decodeBodyFact(facts, "root_free_bytes", &snapshot.RootFreeBytes)
	decodeBodyFact(facts, "agent_free_bytes", &snapshot.AgentFreeBytes)
	decodeBodyFact(facts, "network_available", &snapshot.NetworkAvailable)
	decodeBodyFact(facts, "network_probe", &snapshot.NetworkProbe)
	decodeBodyFact(facts, "desktop_available", &snapshot.DesktopAvailable)
	decodeBodyFact(facts, "wechat_running", &snapshot.WechatRunning)
	decodeBodyFact(facts, "clash_verge_running", &snapshot.ClashVergeRunning)
}

func decodeBodyFact[T any](facts map[string]json.RawMessage, key string, target *T) {
	if encoded, exists := facts[key]; exists {
		_ = json.Unmarshal(encoded, target)
	}
}

func observationFromOrgan(value organ.Observation) perceptualObservation {
	objects := make([]PerceptualObject, 0, len(value.Objects))
	for _, object := range value.Objects {
		id := strings.TrimSpace(object.ID)
		content := strings.TrimSpace(object.Content)
		if id == "" || content == "" {
			continue
		}
		objects = append(objects, PerceptualObject{ID: id, Content: truncate(content, 4000)})
	}
	digestInput, _ := json.Marshal(struct {
		Context []string           `json:"context"`
		Objects []PerceptualObject `json:"objects"`
	}{value.Context, objects})
	digest := sha256.Sum256(digestInput)
	return perceptualObservation{
		OrganID: value.OrganID, SurfaceID: value.SurfaceID, ObservedAt: value.ObservedAt,
		Digest: hex.EncodeToString(digest[:]), Context: append([]string{}, value.Context...), Objects: objects,
	}
}

func perceptualSurfaceKey(organID, surfaceID string) string {
	return strings.TrimSpace(organID) + "/" + strings.TrimSpace(surfaceID)
}

func queuePerceptualNovelty(previous PerceptualTrace, observation perceptualObservation) PerceptualTrace {
	contextChanged := len(previous.Context) > 0 && perceptualContextKey(previous.Context) != perceptualContextKey(observation.Context)
	surfaceChanged := previous.OrganID != observation.OrganID || previous.SurfaceID != observation.SurfaceID
	realityChanged := previous.Digest != "" && previous.Digest != observation.Digest
	if contextChanged || surfaceChanged {
		previous.Pending = nil
		previous.ExhaustedContext = ""
		previous.ExhaustedAt = ""
		previous.SettledByAttention = false
	} else if realityChanged {
		previous.SettledByAttention = false
	}
	known := make(map[string]bool, len(previous.Seen)+len(previous.Pending)+len(observation.Objects))
	for _, id := range previous.Seen {
		known[id] = true
	}
	for _, object := range previous.Pending {
		known[object.ID] = true
	}
	pending := append([]PerceptualObject{}, previous.Pending...)
	for _, object := range observation.Objects {
		if known[object.ID] {
			continue
		}
		known[object.ID] = true
		pending = append(pending, object)
	}
	observedAt := observation.ObservedAt
	if strings.TrimSpace(observedAt) == "" {
		observedAt = nowUTC()
	}
	return PerceptualTrace{
		OrganID: observation.OrganID, SurfaceID: observation.SurfaceID,
		Digest: observation.Digest, ObservedAt: observedAt,
		Context: append([]string{}, observation.Context...), Pending: pending,
		Seen:             append([]string{}, previous.Seen...),
		ExhaustedContext: previous.ExhaustedContext, ExhaustedAt: previous.ExhaustedAt,
		SettledByAttention: previous.SettledByAttention,
	}
}

func takePerceptualNovelty(trace PerceptualTrace) (PerceptualTrace, PerceptualObject, string) {
	if len(trace.Pending) == 0 {
		return trace, PerceptualObject{}, ""
	}
	object := trace.Pending[0]
	trace.Pending = trace.Pending[1:]
	trace.Seen = append(trace.Seen, object.ID)
	const maximumSeenObjects = 512
	if len(trace.Seen) > maximumSeenObjects {
		trace.Seen = append([]string{}, trace.Seen[len(trace.Seen)-maximumSeenObjects:]...)
	}
	parts := append([]string{}, trace.Context...)
	parts = append(parts, "Visible object:", "- "+object.Content)
	return trace, object, strings.TrimSpace(strings.Join(parts, "\n"))
}

func perceptualResampleDue(trace PerceptualTrace, now time.Time, revisitSeconds int) bool {
	if trace.SettledByAttention {
		// Conscious settlement closes the current object, not the living sensory
		// surface forever.  Keep the scene quiet for the same embodied refractory
		// interval, then permit one bounded reorientation.  A genuinely changed
		// digest still reopens immediately through queuePerceptualNovelty.
		if revisitSeconds <= 0 || trace.ExhaustedAt == "" {
			return false
		}
		settledAt, err := time.Parse(time.RFC3339Nano, trace.ExhaustedAt)
		return err != nil || now.Sub(settledAt) >= time.Duration(revisitSeconds)*time.Second
	}
	contextKey := perceptualContextKey(trace.Context)
	return contextKey != "" && len(trace.Pending) == 0 && perceptualExhaustionDue(trace, contextKey, now, revisitSeconds)
}

func perceptualExhaustionDue(trace PerceptualTrace, contextKey string, now time.Time, revisitSeconds int) bool {
	if trace.ExhaustedContext != contextKey {
		return true
	}
	if revisitSeconds <= 0 || trace.ExhaustedAt == "" {
		return false
	}
	exhaustedAt, err := time.Parse(time.RFC3339Nano, trace.ExhaustedAt)
	return err != nil || now.Sub(exhaustedAt) >= time.Duration(revisitSeconds)*time.Second
}

func perceptualContextKey(contextLines []string) string {
	if len(contextLines) == 0 {
		return ""
	}
	digest := sha256.Sum256([]byte(strings.Join(contextLines, "\n")))
	return hex.EncodeToString(digest[:])
}

func reopenPerceptualSampling(trace PerceptualTrace) PerceptualTrace {
	trace.ExhaustedContext = ""
	trace.ExhaustedAt = ""
	trace.SettledByAttention = false
	return trace
}

func discardPendingPerception(trace PerceptualTrace) PerceptualTrace {
	for _, object := range trace.Pending {
		trace.Seen = append(trace.Seen, object.ID)
	}
	trace.Pending = nil
	const maximumSeenObjects = 512
	if len(trace.Seen) > maximumSeenObjects {
		trace.Seen = append([]string{}, trace.Seen[len(trace.Seen)-maximumSeenObjects:]...)
	}
	return trace
}

// habituateSettledPerception closes one consciously focused sensory object.
// An action_result settles the preceding Action Commitment; any concrete
// objects newly revealed by that action remain distinct environmental facts
// and receive their own later opportunity to enter attention. Treating the
// whole post-action surface as settled here made a read-only navigation's stop
// condition silently suppress every person, post or document it exposed.
func (r *Runtime) habituateSettledPerception(candidate Event, appraisal CandidateAppraisal, actionKind, now string) error {
	if actionKind != "none" || (appraisal.Resolution != "released" && appraisal.Resolution != "resolved") {
		return nil
	}
	var observed struct {
		OrganID   string `json:"organ_id"`
		SurfaceID string `json:"surface_id"`
		Digest    string `json:"digest"`
	}
	switch candidate.Kind {
	case "perceptual_change":
		if json.Unmarshal(candidate.Payload, &observed) != nil {
			return nil
		}
	case "action_result":
		var action ActionState
		if json.Unmarshal(candidate.Payload, &action) != nil || action.Status != "completed" {
			return nil
		}
		observed.OrganID = action.OrganID
		observed.SurfaceID = action.ObservedSurfaceID
		observed.Digest = action.ObservedDigest
		if observed.SurfaceID == "" && action.Effect == "observed" {
			// A targeted read may return a fact rather than a full Observation.
			// Its latest known unchanged surface can settle only when no object on
			// that surface is still waiting for its own attention.
			latestObservedAt := ""
			for _, trace := range r.state.Perception {
				if trace.OrganID != action.OrganID || trace.SurfaceID == "" || trace.Digest == "" ||
					(latestObservedAt != "" && !timeAfter(trace.ObservedAt, latestObservedAt)) {
					continue
				}
				observed.SurfaceID = trace.SurfaceID
				observed.Digest = trace.Digest
				latestObservedAt = trace.ObservedAt
			}
		}
	default:
		return nil
	}
	if observed.OrganID == "" || observed.SurfaceID == "" {
		return nil
	}
	surface := perceptualSurfaceKey(observed.OrganID, observed.SurfaceID)
	trace, exists := r.state.Perception[surface]
	if !exists || observed.Digest == "" || trace.Digest != observed.Digest {
		return nil
	}
	if candidate.Kind == "action_result" && len(trace.Pending) > 0 {
		return nil
	}
	trace = discardPendingPerception(trace)
	trace.ExhaustedContext = perceptualContextKey(trace.Context)
	trace.ExhaustedAt = now
	trace.SettledByAttention = true
	r.state.Perception[surface] = trace
	return r.journal("perceptual_habituation", candidate.ID, map[string]any{
		"organ_id": observed.OrganID, "surface_id": observed.SurfaceID,
		"digest": observed.Digest, "resolution": appraisal.Resolution, "source_kind": candidate.Kind,
	})
}

// assimilateActionObservation joins intentional action feedback and passive
// sensing at one factual boundary without merging their causal roles. The
// action_result lets Alice settle what the organ did; newly exposed concrete
// objects stay pending so each may independently compete for attention. The
// objects become Seen only when one actually enters attention, or when a later
// intentional action supersedes the old surface.
func (r *Runtime) assimilateActionObservation(observation perceptualObservation) int {
	surface := perceptualSurfaceKey(observation.OrganID, observation.SurfaceID)
	trace := queuePerceptualNovelty(r.state.Perception[surface], observation)
	newlyAvailable := len(trace.Pending)
	// This surface has just changed or been sampled through a real action. It is
	// immediately eligible for object-level attention; no hidden orientation is
	// needed first.
	trace.ExhaustedContext = ""
	trace.ExhaustedAt = ""
	trace.SettledByAttention = false
	if r.state.Perception == nil {
		r.state.Perception = make(map[string]PerceptualTrace)
	}
	r.state.Perception[surface] = trace
	return newlyAvailable
}

func bodyDifferences(previous, current BodySnapshot, initial bool) []string {
	if initial {
		return []string{"initial body snapshot"}
	}
	var differences []string
	if current.UptimeSeconds < previous.UptimeSeconds {
		differences = append(differences, "system uptime restarted")
	}
	if absUint(current.RootFreeBytes, previous.RootFreeBytes) >= diskEventThreshold {
		differences = append(differences, fmt.Sprintf("root free bytes changed from %d to %d", previous.RootFreeBytes, current.RootFreeBytes))
	}
	if absUint(current.AgentFreeBytes, previous.AgentFreeBytes) >= diskEventThreshold {
		differences = append(differences, fmt.Sprintf("agent free bytes changed from %d to %d", previous.AgentFreeBytes, current.AgentFreeBytes))
	}
	if previous.CognitiveResourceBand != "" && current.CognitiveResourceBand != "" && previous.CognitiveResourceBand != current.CognitiveResourceBand {
		differences = append(differences, fmt.Sprintf(
			"cognitive resource band changed from %s to %s; %.6f USD remains in the rolling hour and %.6f USD remains in the rolling day",
			previous.CognitiveResourceBand, current.CognitiveResourceBand,
			float64(current.CognitiveHourRemainingMicrousd)/float64(microusdPerUSD),
			float64(current.CognitiveDayRemainingMicrousd)/float64(microusdPerUSD),
		))
	}
	if previous.CognitivePriceTableVersion != "" && previous.CognitivePriceTableVersion != current.CognitivePriceTableVersion {
		differences = append(differences, fmt.Sprintf("cognitive price table changed from %s to %s", previous.CognitivePriceTableVersion, current.CognitivePriceTableVersion))
	}
	booleanDifference := func(label string, before, after bool) {
		if before != after {
			differences = append(differences, fmt.Sprintf("%s changed from %t to %t", label, before, after))
		}
	}
	booleanDifference("network probe reachability", previous.NetworkAvailable, current.NetworkAvailable)
	booleanDifference("desktop availability", previous.DesktopAvailable, current.DesktopAvailable)
	booleanDifference("wechat running", previous.WechatRunning, current.WechatRunning)
	booleanDifference("clash verge running", previous.ClashVergeRunning, current.ClashVergeRunning)
	ids := make(map[string]bool)
	for id := range previous.Organs {
		ids[id] = true
	}
	for id := range current.Organs {
		ids[id] = true
	}
	ordered := make([]string, 0, len(ids))
	for id := range ids {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	for _, id := range ordered {
		before, beforeExists := previous.Organs[id]
		after, afterExists := current.Organs[id]
		beforeUsable := beforeExists && before.Accepting && before.Status != "unavailable"
		afterUsable := afterExists && after.Accepting && after.Status != "unavailable"
		if beforeUsable != afterUsable {
			differences = append(differences, fmt.Sprintf("organ %s availability changed from %t to %t", id, beforeUsable, afterUsable))
		}
	}
	return differences
}

func absUint(a, b uint64) uint64 {
	if a > b {
		return a - b
	}
	return b - a
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
