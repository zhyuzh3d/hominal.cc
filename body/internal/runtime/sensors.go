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
	current := BodySnapshot{ObservedAt: nowUTC()}
	updateResourceSnapshot(&current, state, config.CognitiveResource, time.Now().UTC())
	if !slow {
		return current
	}
	current.Organs = make(map[string]OrganSnapshot)
	if organs != nil {
		for id, snapshot := range organs.Snapshot() {
			current.Organs[id] = OrganSnapshot{
				Name: snapshot.Name, Command: snapshot.Command,
				Capabilities: append([]string{}, snapshot.Capabilities...),
				Operations:   append([]string{}, snapshot.Operations...), Guidance: snapshot.Guidance,
				Status: snapshot.Status, Accepting: snapshot.Accepting,
			}
		}
		for _, id := range organs.BodyStateIDs() {
			ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
			observation, err := organs.Observe(ctx, id)
			cancel()
			if err == nil {
				applyBodyFacts(&current, observation.Facts)
			}
		}
	}
	return current
}

func mergeFastSnapshot(previous, fast BodySnapshot) BodySnapshot {
	fast.UptimeSeconds = previous.UptimeSeconds
	fast.RootFreeBytes = previous.RootFreeBytes
	fast.AgentFreeBytes = previous.AgentFreeBytes
	fast.NetworkAvailable = previous.NetworkAvailable
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
	if contextChanged || previous.OrganID != observation.OrganID || previous.SurfaceID != observation.SurfaceID {
		previous.Pending = nil
		previous.ExhaustedContext = ""
		previous.ExhaustedAt = ""
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
	booleanDifference("network availability", previous.NetworkAvailable, current.NetworkAvailable)
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
