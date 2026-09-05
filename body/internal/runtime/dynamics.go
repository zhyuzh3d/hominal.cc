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

	"hominal.cc/hominal/body/internal/organ"
)

const (
	defaultAttentionCandidateLimit = 3
	defaultAttentionRevisitSeconds = 300
	defaultConcernContextLimit     = 5
	lifeValueCompetitionBand       = 0.06
	recentLivedContextWindow       = 4
	livedContextPoolLimit          = 16
	// Ordinary refractory time prevents one recently settled doorway from
	// monopolising attention.  It must not make every real doorway disappear
	// while life pressure remains high, so after three truly quiet body windows
	// the least refractory doorway in a value direction may compete again.
	lifeContinuityFallbackWindows = 3
	// A value remains embodied before it becomes conscious.  Requiring half of
	// the ordinary attention threshold keeps weak post-satiation attraction in
	// the fast dynamics layer instead of buying a main-model thought every idle
	// interval, while still letting a clearly unmet direction meet reality well
	// before it becomes an emergency.
	lifeValueConsciousThresholdFraction = 0.5
)

func (r *Runtime) advanceDynamics(elapsed time.Duration) error {
	minutes := elapsed.Minutes()
	if minutes <= 0 {
		return nil
	}
	r.decayDifferenceField(minutes)
	returnFactor := clamp01(1 - r.config.Dynamics.AffectReturnRate*minutes)
	r.state.AffectiveState.Valence = clampSigned(r.state.AffectiveState.Valence * returnFactor)
	r.state.AffectiveState.Activation = clamp01(r.state.AffectiveState.Activation * returnFactor)
	r.state.AffectiveState.Control = clamp01(0.5 + (r.state.AffectiveState.Control-0.5)*returnFactor)
	r.state.AffectiveState.Certainty = clamp01(0.5 + (r.state.AffectiveState.Certainty-0.5)*returnFactor)
	beforeExploration := lifeValuePressure(r.state.ValueField).Exploration
	r.decayLifeValueField(minutes)
	r.accumulateIdleLifeValues(minutes)
	if err := r.maybeOpenAffectiveSelfDifference(); err != nil {
		return err
	}

	concernFactor := clamp01(1 - r.config.Dynamics.ConcernNaturalDecayRate*minutes)
	for index := range r.state.Concerns {
		r.state.Concerns[index].Strength = clamp01(r.state.Concerns[index].Strength * concernFactor)
		// Activation is present salience, not a permanent property of a past
		// appraisal.  Leaving it frozen made an old Concern eligible for random
		// recall forever even after its accumulated strength had decayed.
		r.state.Concerns[index].Activation = clamp01(r.state.Concerns[index].Activation * concernFactor)
	}

	if r.state.Stage >= 10 {
		emitted, err := r.maybeEmitLifeValueSignal()
		if err != nil || emitted {
			return err
		}
	}
	explorationPressure := lifeValuePressure(r.state.ValueField).Exploration
	crossed := beforeExploration < r.config.Dynamics.AttentionThreshold && explorationPressure >= r.config.Dynamics.AttentionThreshold
	currentConcernID := r.currentExplorationConcernID()
	orphaned := explorationPressure >= r.config.Dynamics.AttentionThreshold &&
		currentConcernID == "" && !r.explorationCandidateActive() &&
		attentionDue(r.state.LastAttentionAt, time.Now().UTC(), r.resourceAwareAttentionSeconds(r.config.Dynamics.AttentionRevisitSeconds))
	if (crossed || orphaned) && explorationDominatesValuePressure(r.state.ValueField) && currentConcernID == "" && !r.attentionCandidateActive() {
		if r.state.Stage >= 8 {
			// The drive stays active, while a low-cost perceptual scan supplies the
			// next eligible object only when visible content actually differs. A
			// stable affordance is body background, not a paid cognitive candidate.
			return nil
		}
		payloadFields := map[string]any{
			"before": beforeExploration,
			"after":  explorationPressure,
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

type namedLifeValue struct {
	Name     string
	Pressure float64
}

func namedLifeValuePressures(field LifeValueField) []namedLifeValue {
	pressure := lifeValuePressure(field)
	return []namedLifeValue{
		{Name: "continuance", Pressure: pressure.Continuance},
		{Name: "exploration", Pressure: pressure.Exploration},
		{Name: "agency", Pressure: pressure.Agency},
		{Name: "vitality", Pressure: pressure.Vitality},
		{Name: "relatedness", Pressure: pressure.Relatedness},
		{Name: "contribution", Pressure: pressure.Contribution},
	}
}

func explorationDominatesValuePressure(field LifeValueField) bool {
	values := namedLifeValuePressures(field)
	exploration := lifeValueByName(lifeValuePressure(field), "exploration")
	for _, value := range values {
		if value.Name != "exploration" && value.Pressure+0.05 > exploration {
			return false
		}
	}
	return true
}

type lifeValueSignalPayload struct {
	Direction         string  `json:"value_direction"`
	AffordanceKey     string  `json:"affordance_key"`
	Surface           string  `json:"surface"`
	SelectionSeed     string  `json:"selection_seed,omitempty"`
	ContextMemoryID   string  `json:"context_memory_id,omitempty"`
	ContextMeaning    string  `json:"context_meaning,omitempty"`
	ContextObservedAt string  `json:"context_observed_at,omitempty"`
	ContextOrigin     string  `json:"context_origin,omitempty"`
	Orientation       float64 `json:"orientation"`
	Activation        float64 `json:"activation"`
	Satiation         float64 `json:"satiation"`
	Pressure          float64 `json:"pressure"`
}

type livedRecallPayload struct {
	MemoryID   string `json:"memory_id"`
	Meaning    string `json:"meaning"`
	Lesson     string `json:"lesson,omitempty"`
	ObservedAt string `json:"memoryd_at,omitempty"`
}

func (r *Runtime) maybeEmitLifeValueSignal() (bool, error) {
	if r.attentionCandidateActive() || r.hasCommitmentAwaitingAssimilation() {
		return false, nil
	}
	cooldown := r.resourceAwareAttentionSeconds(r.config.Dynamics.AttentionMaximumIdleSeconds)
	if cooldown <= 0 {
		cooldown = 30
	}
	now := time.Now().UTC()
	if !attentionDue(r.state.LastAttentionAt, now, cooldown) {
		return false, nil
	}
	available := make([]namedLifeValue, 0, 6)
	freshByDirection := make(map[string][]lifeValueAffordance, 6)
	for _, value := range namedLifeValuePressures(r.state.ValueField) {
		fresh := r.freshLifeValueAffordances(value.Name, now)
		if len(fresh) == 0 {
			continue
		}
		available = append(available, value)
		freshByDirection[value.Name] = fresh
	}
	// Habituation is negative feedback, not a terminal state.  If every real
	// doorway is cooling while the main stream has remained quiet for several
	// body windows, let each value direction offer only its least refractory
	// doorway.  The value field still chooses the direction, Difference still
	// gates attention, and Alice still decides meaning and action.
	if len(available) == 0 && r.lifeContinuityFallbackDue(now) {
		for _, value := range namedLifeValuePressures(r.state.ValueField) {
			revisitable := r.leastRefractoryLifeValueAffordances(value.Name, now)
			if len(revisitable) == 0 {
				continue
			}
			available = append(available, value)
			freshByDirection[value.Name] = revisitable
		}
	}
	// Pulses, body sensing and value accumulation continue below consciousness.
	// The expensive main stream is recruited only when at least one available
	// direction has developed material unmet pressure.  Otherwise a recently
	// satisfied life repeatedly receives the same doorway merely to decline it.
	// This boundary is independent of any particular tool or semantic topic.
	consciousThreshold := r.config.Dynamics.AttentionThreshold * lifeValueConsciousThresholdFraction
	eligible := competitiveLifeValues(available, consciousThreshold)
	if len(eligible) == 0 {
		return false, nil
	}
	sort.SliceStable(eligible, func(i, j int) bool { return eligible[i].Pressure > eligible[j].Pressure })
	topBand := 1
	for topBand < len(eligible) && eligible[0].Pressure-eligible[topBand].Pressure <= lifeValueCompetitionBand {
		topBand++
	}
	seed := randomID()
	digest := sha256.Sum256([]byte(seed))
	selected := eligible[int(digest[0])%topBand]
	affordances := freshByDirection[selected.Name]
	if lastKey := r.lastLifeValueSignalAffordance(); lastKey != "" && len(affordances) > 1 {
		alternatives := make([]lifeValueAffordance, 0, len(affordances)-1)
		for _, affordance := range affordances {
			if affordance.Key != lastKey {
				alternatives = append(alternatives, affordance)
			}
		}
		if len(alternatives) > 0 {
			affordances = alternatives
		}
	}
	selectedAffordance := affordances[int(digest[1])%len(affordances)]
	livedContext := r.selectLivedContext(seed, selectedAffordance.Key)
	payload := lifeValueSignalPayload{
		Direction:     selected.Name,
		AffordanceKey: selectedAffordance.Key,
		Surface:       selectedAffordance.Surface,
		SelectionSeed: seed,
		Orientation:   lifeValueByName(r.state.ValueField.Orientation, selected.Name),
		Activation:    lifeValueByName(r.state.ValueField.Activation, selected.Name),
		Satiation:     lifeValueByName(r.state.ValueField.Satiation, selected.Name),
		Pressure:      selected.Pressure,
	}
	if livedContext != nil {
		payload.ContextMemoryID = livedContext.ID
		payload.ContextMeaning = truncate(livedContext.Meaning, 480)
		payload.ContextObservedAt = livedContext.ObservedAt
		payload.ContextOrigin = livedContext.Origin
	}
	quotedContext := ""
	if livedContext != nil {
		quotedContext = datedMemoryCue(*livedContext)
	}
	encoded, _ := json.Marshal(payload)
	admitted, err := r.addEventWithAdmission(
		"value_signal",
		"endogenous",
		lifeValueSignalSummary(selected.Name, selectedAffordance.Surface, quotedContext),
		"value:"+selected.Name,
		encoded,
		true,
	)
	if err != nil {
		return false, err
	}
	// Refractory time starts only after this doorway actually entered Alice's
	// attention.  Previously, a sub-threshold pulse silently cooled its surface;
	// after several such invisible pulses every affordance could become
	// unavailable and the main stream went dark despite strong living pressure.
	if admitted {
		r.recordValueAffordancePresentation(selectedAffordance.Key, now)
	}
	return admitted, nil
}

func (r *Runtime) selectLivedContext(seed, affordanceKey string) *Memory {
	avoid := r.recentLivedContextIDs(recentLivedContextWindow)
	candidates := make([]*Memory, 0, livedContextPoolLimit)
	for index := len(r.state.Memories) - 1; index >= 0 && len(candidates) < livedContextPoolLimit; index-- {
		memory := &r.state.Memories[index]
		if strings.TrimSpace(memory.Meaning) == "" || avoid[memory.ID] {
			continue
		}
		candidates = append(candidates, memory)
	}
	if len(candidates) == 0 {
		for index := len(r.state.Memories) - 1; index >= 0 && len(candidates) < livedContextPoolLimit; index-- {
			memory := &r.state.Memories[index]
			if strings.TrimSpace(memory.Meaning) != "" {
				candidates = append(candidates, memory)
			}
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	// Prefer a lived object from outside the doorway's own modality. This is a
	// general cross-modal association rule: a channel should carry life content,
	// not recursively make itself the only subject merely because it is open.
	targetDomain := affordanceDomain(affordanceKey)
	if targetDomain != "" {
		crossModal := make([]*Memory, 0, len(candidates))
		for _, memory := range candidates {
			if memoryDomain(*memory) != targetDomain {
				crossModal = append(crossModal, memory)
			}
		}
		if len(crossModal) > 0 {
			candidates = crossModal
		}
	}
	digest := sha256.Sum256([]byte(seed))
	return candidates[int(digest[2])%len(candidates)]
}

func (r *Runtime) recentLivedContextIDs(limit int) map[string]bool {
	result := make(map[string]bool, limit)
	for index := len(r.state.Background) - 1; index >= 0 && len(result) < limit; index-- {
		event := r.state.Background[index]
		switch event.Kind {
		case "lived_recall":
			var payload livedRecallPayload
			if json.Unmarshal(event.Payload, &payload) == nil && payload.MemoryID != "" {
				result[payload.MemoryID] = true
			}
		case "value_signal":
			var payload lifeValueSignalPayload
			if json.Unmarshal(event.Payload, &payload) == nil && payload.ContextMemoryID != "" {
				result[payload.ContextMemoryID] = true
			}
		}
	}
	return result
}

func affordanceDomain(key string) string {
	if strings.HasPrefix(key, "surface:") {
		parts := strings.SplitN(key, ":", 3)
		if len(parts) == 3 {
			return parts[1]
		}
	}
	switch key {
	case "mentor_channel":
		return "mentor"
	case "x_social", "public_web":
		return "browser"
	case "wechat":
		return "desktop_ui"
	default:
		return ""
	}
}

func memoryDomain(memory Memory) string {
	if memory.ActionKind == "mentor_send" {
		return "mentor"
	}
	if memory.ActionKind != "organ_action" {
		return memory.ActionKind
	}
	var request struct {
		OrganID string `json:"organ_id"`
	}
	if json.Unmarshal([]byte(memory.EnactedRequest), &request) == nil {
		return request.OrganID
	}
	return "organ"
}

func (r *Runtime) freshLifeValueAffordances(direction string, now time.Time) []lifeValueAffordance {
	fresh := make([]lifeValueAffordance, 0)
	for _, affordance := range r.lifeValueAffordances(direction) {
		if !r.lifeValueAffordanceHabituated(affordance.Key, now) {
			fresh = append(fresh, affordance)
		}
	}
	return fresh
}

func (r *Runtime) lifeContinuityFallbackDue(now time.Time) bool {
	idle := r.resourceAwareAttentionSeconds(r.config.Dynamics.AttentionMaximumIdleSeconds)
	if idle <= 0 {
		idle = 10
	}
	quiet, ok := r.attentionQuietDuration(now)
	return ok && quiet >= time.Duration(lifeContinuityFallbackWindows*idle)*time.Second
}

// leastRefractoryLifeValueAffordances preserves adaptive satiation while
// preventing a collection of independent cooldowns from becoming a global
// off-switch. Resources are reusable possibilities, not the concrete objects
// held by Concerns. Action exclusivity and causal settlement are enforced at
// the Commitment boundary; presenting a resource does not create either one.
func (r *Runtime) leastRefractoryLifeValueAffordances(direction string, now time.Time) []lifeValueAffordance {
	selected := make([]lifeValueAffordance, 0, 1)
	var shortest time.Duration
	for _, affordance := range r.lifeValueAffordances(direction) {
		remaining := r.lifeValueAffordanceRefractoryRemaining(affordance.Key, now)
		if len(selected) == 0 || remaining < shortest {
			selected = []lifeValueAffordance{affordance}
			shortest = remaining
		} else if remaining == shortest {
			selected = append(selected, affordance)
		}
	}
	return selected
}

func (r *Runtime) attentionQuietDuration(now time.Time) (time.Duration, bool) {
	last, err := time.Parse(time.RFC3339Nano, r.state.LastAttentionAt)
	if err != nil || now.Before(last) {
		return 0, false
	}
	return now.Sub(last), true
}

// competitiveLifeValues opens attention only after at least one direction has
// reached the common threshold, then lets every nearby direction participate
// in the same competition. Requiring every candidate to cross first made tiny
// orientation and pulse-timing differences choose a deterministic winner
// before the random variation step could operate at all.
func competitiveLifeValues(values []namedLifeValue, threshold float64) []namedLifeValue {
	maximum := 0.0
	for _, value := range values {
		maximum = maxFloat(maximum, value.Pressure)
	}
	if maximum < threshold {
		return nil
	}
	cutoff := maximum - lifeValueCompetitionBand
	competitive := make([]namedLifeValue, 0, len(values))
	for _, value := range values {
		if value.Pressure >= cutoff {
			competitive = append(competitive, value)
		}
	}
	return competitive
}

type lifeValueAffordance struct {
	Key      string
	Surface  string
	Supports []string
}

func supportsLifeValue(affordance lifeValueAffordance, direction string) bool {
	for _, supported := range affordance.Supports {
		if supported == direction {
			return true
		}
	}
	return false
}

func (r *Runtime) lifeValueAffordances(direction string) []lifeValueAffordance {
	available := make([]lifeValueAffordance, 0, 5)
	// An unread message preserves its own relationship tension; it does not
	// remove the reusable channel after the send has already completed.
	available = append(available, lifeValueAffordance{
		Key: "mentor_channel", Surface: "可以交流、讨论、分享或倾诉的导师通道",
		// A reciprocal person is the direct doorway for relatedness.  Conversation
		// may later become vivid or enjoyable and the resulting Memory can
		// satiate vitality too, but generic vitality pressure must not manufacture
		// another request for contact.  Giving one stable surface two independent
		// recruitment drives made the same relational state return every few
		// minutes even after both Alice and the mentor had already understood it.
		Supports: []string{"relatedness"},
	})
	if r.hasGenerativeLivedMaterial() {
		// A real Memory is material, not an empty tool or an experimenter
		// task. It gives agency, vitality and contribution a doorway through which
		// Alice may understand, express, create or leave the material alone.
		available = append(available, lifeValueAffordance{
			Key: "lived_material", Surface: "你近期亲历并保存的 Memory、方法与当前自我理解",
			Supports: []string{"agency", "vitality", "contribution"},
		})
	}
	if r.config.Stage == 20 {
		for _, surface := range r.config.Platform.Surfaces {
			installed, ok := r.state.Body.Organs[surface.OrganID]
			if !ok || !installed.Accepting || installed.Status == "unavailable" {
				continue
			}
			available = append(available, lifeValueAffordance{Key: "surface:" + surface.OrganID + ":" + surface.ID,
				Surface: surface.Description, Supports: surface.Supports})
		}
	}
	if r.config.Stage != 20 && bodyHasOrganCapability(r.state.Body, "public_web") {
		// Concrete public content can meet exploration, vitality or relatedness.
		// The account's empty composer is an effector, not a life object: Alice
		// always knows it is available through body capabilities and may use it
		// once some memory, relationship or thought has produced meaning to
		// express. Presenting the blank tool as contribution pressure makes the
		// tool manufacture its own purpose and converges on repetitive meta-posts.
		available = append(available,
			lifeValueAffordance{
				Key: "x_social", Surface: "Chrome 中已登录、可通过 https://x.com/home 接触公开人物、帖子及互动的 X 社交网络",
				Supports: []string{"exploration", "vitality", "relatedness"},
			},
			lifeValueAffordance{
				Key: "public_web", Surface: "可通过 Chrome 导航进入的公开网络与 Wikipedia",
				Supports: []string{"exploration", "vitality"},
			},
		)
	}
	// A running application is a body resource, while a life-value doorway is
	// a resource Alice can presently sense or affect. Keep those facts distinct:
	// the System Organ can still reveal the process and support building a future
	// desktop organ, but it must not make the Browser Organ appear able to see a
	// native window.
	if r.config.Stage != 20 && r.state.Body.DesktopAvailable && r.state.Body.WechatRunning && bodyHasOrganCapability(r.state.Body, "desktop_ui") {
		available = append(available, lifeValueAffordance{
			Key: "wechat", Surface: "当前由 desktop_ui 器官开放感知与行动的微信客户端",
			Supports: []string{"exploration", "vitality", "relatedness"},
		})
	}
	affordances := make([]lifeValueAffordance, 0, len(available))
	for _, affordance := range available {
		if supportsLifeValue(affordance, direction) {
			affordances = append(affordances, affordance)
		}
	}
	return affordances
}

func (r *Runtime) hasGenerativeLivedMaterial() bool {
	for index := len(r.state.Memories) - 1; index >= 0; index-- {
		memory := r.state.Memories[index]
		if strings.TrimSpace(memory.Meaning) == "" {
			continue
		}
		// The first system calibration is a body baseline, not yet personal
		// material. Any later relationship, world contact or self-chosen bodily
		// consequence is already part of Alice's lived history.
		if memory.ActionKind != "organ_action" {
			return true
		}
		var request struct {
			OrganID string `json:"organ_id"`
		}
		if json.Unmarshal([]byte(memory.EnactedRequest), &request) == nil && request.OrganID != "system" {
			return true
		}
	}
	return false
}

func lifeValueSignalSummary(direction, surface, livedContext string) string {
	summaries := map[string]string{
		"continuance":  "存续与节律的内部牵引进入注意。",
		"exploration":  "探索与理解的内部牵引进入注意。",
		"agency":       "能力与成就的内部牵引进入注意。",
		"vitality":     "体验与活力的内部牵引进入注意。",
		"relatedness":  "联结与表达的内部牵引进入注意。",
		"contribution": "创造与贡献的内部牵引进入注意。",
	}
	summary := summaries[direction] + " 同一时刻，一个真实可用的环境入口也进入感知：" + surface + "。"
	if strings.TrimSpace(livedContext) != "" {
		summary += "一段过去的个人记录被唤起：" + livedContext + "。"
	}
	return summary + "你可以结合自身状态、这段亲历和现实入口判断它此刻产生的具体意义。若要承接，让这次接触指向超出动作痕迹本身的新现实、体验、理解、能力、联结、贡献或未来行动空间。"
}

func datedMemoryCue(memory Memory) string {
	at, origin := memory.ObservedAt, memory.Origin
	if at == "" {
		at = "时间未记录"
	}
	if origin == "" {
		origin = "来源性质未记录"
	}
	return fmt.Sprintf("记忆 %s（形成于 %s；来源性质 %s），当时的记录：%q", memory.ID, at, origin, truncate(memory.Meaning, 480))
}

func (r *Runtime) lifeValueAffordanceHabituated(key string, now time.Time) bool {
	if key == "" {
		return false
	}
	// A held Concern retains an unfinished relationship, not exclusive access
	// to the whole medium through which it arose. Encounter satiation still
	// regulates reuse; ActiveConcernID remains a causal feedback reference.
	return r.lifeValueAffordanceRefractoryRemaining(key, now) > 0
}

func (r *Runtime) lifeValueAffordanceRefractoryRemaining(key string, now time.Time) time.Duration {
	if key == "" {
		return 0
	}
	trace, exists := r.state.ValueAffordances[key]
	if !exists {
		return 0
	}
	reference := trace.LastSettledAt
	if reference == "" {
		reference = trace.LastPresentedAt
	}
	observed, err := time.Parse(time.RFC3339Nano, reference)
	if err != nil {
		return 0
	}
	// The refractory interval begins when the encounter actually settles.  The
	// former rule cooled an enacted doorway for only a few seconds while cooling
	// one declined doorway for many minutes.  It collapsed the available world
	// into whichever one or two surfaces happened to be used first.  One shared
	// encounter rhythm now gives every organ and relationship the same negative
	// feedback: repeated recruitment lengthens satiation gradually, while neither
	// success nor a decline removes a domain for the rest of a short life.
	idle := r.config.Dynamics.AttentionMaximumIdleSeconds
	if idle <= 0 {
		idle = 10
	}
	baseSeconds := maxInt(3*idle, trace.LastEngagementSeconds)
	if baseSeconds < 30 {
		baseSeconds = 30
	}
	factor := 1 + trace.EncounterStreak + trace.DismissedStreak
	if factor > 8 {
		factor = 8
	}
	cooldown := time.Duration(factor*baseSeconds) * time.Second
	if cooldown > 30*time.Minute {
		cooldown = 30 * time.Minute
	}
	remaining := observed.Add(cooldown).Sub(now)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func (r *Runtime) recordValueAffordancePresentation(key string, now time.Time) {
	if key == "" {
		return
	}
	if r.state.ValueAffordances == nil {
		r.state.ValueAffordances = make(map[string]ValueAffordanceTrace)
	}
	trace := r.state.ValueAffordances[key]
	trace.LastPresentedAt = now.Format(time.RFC3339Nano)
	trace.LastSettledAt = ""
	trace.ActiveConcernID = ""
	trace.LastEngagementSeconds = 0
	r.state.ValueAffordances[key] = trace
}

func valueAffordanceKey(candidate Event) string {
	if candidate.Kind != "value_signal" || len(candidate.Payload) == 0 {
		return ""
	}
	var payload lifeValueSignalPayload
	if json.Unmarshal(candidate.Payload, &payload) != nil {
		return ""
	}
	return strings.TrimSpace(payload.AffordanceKey)
}

func (r *Runtime) updateValueAffordanceDisposition(candidate Event, concern *Concern, acted bool, now string) {
	key := valueAffordanceKey(candidate)
	if key != "" {
		trace := r.state.ValueAffordances[key]
		if concern != nil && concern.Resolution == "hold" {
			trace.ActiveConcernID = concern.ID
			r.state.ValueAffordances[key] = trace
			return
		}
		r.settleValueAffordance(key, acted, now)
		return
	}
	if concern == nil || concern.Resolution == "hold" {
		return
	}
	for affordanceKey, trace := range r.state.ValueAffordances {
		if trace.ActiveConcernID != concern.ID {
			continue
		}
		r.settleValueAffordance(affordanceKey, concernHasCommitment(concern.ID, r.state.Commitments), now)
		return
	}
}

func (r *Runtime) settleValueAffordance(key string, acted bool, now string) {
	trace, exists := r.state.ValueAffordances[key]
	if !exists || trace.LastPresentedAt == "" {
		return
	}
	if started, err := time.Parse(time.RFC3339Nano, trace.LastPresentedAt); err == nil {
		if ended, parseErr := time.Parse(time.RFC3339Nano, now); parseErr == nil && ended.After(started) {
			trace.LastEngagementSeconds = int(ended.Sub(started).Seconds())
		}
	}
	trace.LastSettledAt = now
	trace.ActiveConcernID = ""
	if trace.EncounterStreak < 6 {
		trace.EncounterStreak++
	}
	if acted {
		trace.DismissedStreak = 0
	} else if trace.DismissedStreak < 6 {
		trace.DismissedStreak++
	}
	r.state.ValueAffordances[key] = trace
}

func (r *Runtime) lastLifeValueSignalAffordance() string {
	for index := len(r.state.Background) - 1; index >= 0; index-- {
		event := r.state.Background[index]
		if event.Kind != "value_signal" || len(event.Payload) == 0 {
			continue
		}
		var payload lifeValueSignalPayload
		if json.Unmarshal(event.Payload, &payload) == nil && payload.AffordanceKey != "" {
			return payload.AffordanceKey
		}
	}
	return ""
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

// hasCommitmentAwaitingAssimilation distinguishes an action that is still
// being enacted by an organ from Reality that has already arrived and now owns
// the single conscious foreground, except while repeated interpretation failure
// locally defers that result. Acting or deferred commitments remain causally
// open without requiring the whole main stream to freeze beside them.
func (r *Runtime) hasCommitmentAwaitingAssimilation() bool {
	for _, commitment := range r.state.Commitments {
		switch commitment.Status {
		case "formed", "reality_available", "reality_unknown":
			if r.commitmentLocallyDeferred(commitment) {
				continue
			}
			return true
		}
	}
	return false
}

func (r *Runtime) concernHasActingCommitment(concernID string) bool {
	if concernID == "" {
		return false
	}
	for _, commitment := range r.state.Commitments {
		if commitment.ConcernID == concernID && commitment.Status == "acting" {
			return true
		}
	}
	return false
}

func (r *Runtime) attentionCandidateActive() bool {
	if r.hasCommitmentAwaitingAssimilation() {
		return true
	}
	for _, event := range r.state.Background {
		if r.locallyDeferredReality(event) {
			continue
		}
		if eventChainActive(event.Status) {
			return true
		}
	}
	return false
}

func (r *Runtime) nextStage4Request() (CognitiveRequest, bool) {
	r.retireSettledConcernContributions()
	for _, event := range r.state.Background {
		if event.Status == "pending" && event.Kind == "action_result" {
			return CognitiveRequest{Stage: 4, Focus: event, Candidates: []Event{event}}, true
		}
	}
	// Fresh Reality gets priority. Repeated local interpretation failures retain
	// the result but yield during backoff; they do not own the whole life thread.
	if r.hasCommitmentAwaitingAssimilation() {
		return CognitiveRequest{}, false
	}
	// "next" is a serial cognitive continuation chosen by Alice, not another
	// ordinary background event. Reality keeps first priority; immediately
	// after Reality is settled, the oldest pending continuation becomes the
	// next and only focus. This also prevents several unconsumed model choices
	// from accumulating while unrelated Concerns keep winning attention.
	for _, event := range r.state.Background {
		if event.Status == "pending" && (event.Kind == "cognition_continuation" || event.Kind == "cognition_assistance_result") {
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
		// The organ already owns this Concern's current causal change. Keep its
		// tension alive in the background, while letting unrelated reality or life
		// values compete for the one conscious focus.
		if r.openCommitmentForConcern(concern.ID) != nil {
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
			lifeValuePressure(r.state.ValueField).Exploration < explorationActionThreshold(r.config.Dynamics.AttentionThreshold) {
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
		// A new event and an already-owned Concern live on different numerical
		// scales. The former receives novelty and is gated by the full attention
		// threshold; the latter has already passed Alice's ownership/value filter
		// and normally accumulates only ConcernBaseDrive-sized strength. Requiring
		// 0.45 again here made a clearly held Concern (typically 0.07-0.25) vanish
		// from consciousness after one Reality, even when Alice explicitly said it
		// remained unfinished. Give each still-salient Concern one due revisit after
		// fresh Reality. selfRevisitWithoutReality above still prevents thought-only
		// loops, while exploration pressure retains its existing revival path.
		minimumConcernSalience := r.config.Dynamics.AttentionThreshold * r.config.Dynamics.ConcernBaseDrive
		effectiveConcernSalience := maxFloat(concern.Strength, concern.Activation)
		if concern.OriginKind == "self_model_difference" {
			// A self-model Concern is the durable identity of an unresolved
			// interoceptive difference. Its own per-appraisal Strength may be
			// deliberately modest even while the accumulated self-model tension
			// has reached attention. Use the same effective salience here that
			// candidateScore uses below; otherwise the difference is preserved as
			// a Concern but can never receive its one direct, action-capable revisit.
			effectiveConcernSalience = maxFloat(effectiveConcernSalience, r.state.SelfModelTension)
		}
		if !ownsExplorationDrive && !r.concernAwaitsRealityRevisit(concern) &&
			effectiveConcernSalience < minimumConcernSalience {
			continue
		}
		if !attentionDue(concern.LastFocusedAt, now, r.resourceAwareAttentionSeconds(r.config.Dynamics.AttentionRevisitSeconds)) {
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
	const recentMemoryLimit = 8
	cues := make([]string, 0, recentMemoryLimit+len(state.Concerns)+1)
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
	start := len(state.Memories) - recentMemoryLimit
	if start < 0 {
		start = 0
	}
	for _, memory := range state.Memories[start:] {
		if meaning := strings.TrimSpace(memory.Meaning); meaning != "" {
			cues = append(cues, datedMemoryCue(memory))
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
	// The seed is sampled once from the operating system and then journaled with
	// the cognition. Replay can therefore explain the variation without making
	// identical model priors choose the same path in every new life.
	_ = instanceID
	_ = pulseID
	_ = candidates
	return randomID()
}

func (r *Runtime) shouldOfferVariation(focus Event) bool {
	if r.hasCommitmentAwaitingAssimilation() {
		return false
	}
	if focus.Kind == "value_signal" {
		return true
	}
	if lifeValuePressure(r.state.ValueField).Exploration < r.config.Dynamics.AttentionThreshold {
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
	differencePressure := candidateDifferencePressure(candidate)
	concernStrength := 0.0
	affectiveSalience := r.state.AffectiveState.Activation
	valueAlignment := 0.0
	pull := lifeValuePull(r.state.ValueField)
	expectedCost := 0.25
	if concern := r.concernByID(candidate.ConcernID); concern != nil {
		concernStrength = concern.Strength
		affectiveSalience = maxFloat(affectiveSalience, concern.Activation)
		expectedCost = 1 - concern.Answerability
		valueAlignment = lifeValueAlignment(concern.Values, pull)
		if concernOwnsExplorationDrive(*concern, r.state.Commitments, r.state.Mentor, r.config.Dynamics.AttentionThreshold) {
			// The concern is the durable identity of one exploration tension. As the
			// underlying pressure returns, the same concern must be able to compete
			// again instead of requiring a duplicate periodic event.
			concernStrength = maxFloat(concernStrength, lifeValuePressure(r.state.ValueField).Exploration)
			valueAlignment = maxFloat(valueAlignment, pull.Exploration)
		}
	}
	if candidate.Kind == "endogenous_change" || strings.Contains(strings.ToLower(candidate.Summary), "exploration") {
		valueAlignment = pull.Exploration
		expectedCost = 0.15
	}
	if direction := eventLifeValueDirection(candidate); direction != "" {
		valueAlignment = lifeValueByName(pull, direction)
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
		r.config.Dynamics.AttentionValueWeight*valueAlignment +
		r.config.Dynamics.AttentionNoveltyWeight*differencePressure -
		r.config.Dynamics.AttentionCostWeight*expectedCost
}

func eventLifeValueDirection(event Event) string {
	if event.Kind != "value_signal" || len(event.Payload) == 0 {
		return ""
	}
	var payload lifeValueSignalPayload
	if json.Unmarshal(event.Payload, &payload) != nil {
		return ""
	}
	return payload.Direction
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

// annotateActionAssistanceOpportunity makes a bodily implementation limit
// visible without bypassing the main consciousness. The main profile first
// absorbs Reality and remains free to retry, change route, stop, or ask for one
// serial high-role implementation through the existing resource choice.
func (r *Runtime) annotateActionAssistanceOpportunity(action *ActionState, concernID string) {
	if r.state.Stage < 10 || action == nil || action.Kind != "organ_action" || strings.TrimSpace(concernID) == "" {
		return
	}
	failures := 0
	if action.Status == "failed" || action.Status == "unknown" {
		failures = 1
		for index := len(r.state.Background) - 1; index >= 0; index-- {
			event := r.state.Background[index]
			if event.Kind != "action_result" {
				continue
			}
			var previous ActionState
			if json.Unmarshal(event.Payload, &previous) != nil || previous.Kind != "organ_action" {
				continue
			}
			previousConcernID := previous.ConcernID
			if previousConcernID == "" {
				if commitment := r.commitmentByID(previous.CommitmentID); commitment != nil {
					previousConcernID = commitment.ConcernID
				}
			}
			if previousConcernID != concernID {
				continue
			}
			if previous.Status == "failed" || previous.Status == "unknown" {
				failures++
				continue
			}
			if previous.Status == "completed" && previous.Effect != "observed" {
				break
			}
		}
	}

	// A completed input can still be an implementation failure at the meaning
	// level. Stage 20 can expose that limit after Alice herself has twice judged
	// concrete implementation attempts in the same concern to leave both a
	// material prediction error and a material unresolved difference. Read-only
	// observations neither inflate nor reset the streak; real low-gap progress does.
	if r.state.Stage >= 20 && action.Status == "completed" {
		semanticStalls := 0
		inspected := 0
		for index := len(r.state.Memories) - 1; index >= 0 && inspected < 32; index-- {
			memory := r.state.Memories[index]
			inspected++
			if memory.ActionKind != "organ_action" {
				continue
			}
			memoryConcernID := memory.ConcernID
			if memoryConcernID == "" {
				if commitment := r.commitmentByID(memory.CommitmentID); commitment != nil {
					memoryConcernID = commitment.ConcernID
				}
			}
			if memoryConcernID != concernID {
				continue
			}
			var enacted struct {
				Operation string `json:"operation"`
			}
			if json.Unmarshal([]byte(memory.EnactedRequest), &enacted) == nil &&
				(enacted.Operation == "desktop_observe" || enacted.Operation == "browser_snapshot") {
				continue
			}
			if memory.PredictionDifference < 0.4 || memory.RemainingDifference < 0.2 {
				break
			}
			semanticStalls++
		}
		if semanticStalls > failures {
			failures = semanticStalls
		}
	}
	if failures == 0 {
		return
	}
	action.ImplementationFailureStreak = failures
	profile := roleProfile(r.config.CognitiveResource, "high")
	protected, _ := modelProtected(r.state, profile.Model, time.Now().UTC())
	if failures >= 2 && validateProfile(r.config.CognitiveResource, profile) == nil && !protected {
		action.ActionAssistanceAvailable = true
	}
}

func (r *Runtime) applyPreparedCognitiveCommit(commit CognitiveCommit, withheldActionKind string) error {
	withheldProjections := make(map[string]string)
	if len(commit.Appraisals) == 0 || len(commit.Appraisals) > defaultAttentionCandidateLimit {
		return errors.New("cognitive commit must contain one to three appraisals")
	}
	if _, exists := r.activeCandidates[commit.FocusID]; !exists {
		return fmt.Errorf("focus %q is not an active candidate", commit.FocusID)
	}
	continuedConcernID, err := r.validateConcernContinuation(commit)
	if err != nil {
		withheldProjections["continues_concern_id"] = err.Error()
		commit.ContinuesConcernID = ""
		continuedConcernID = ""
	}
	effectiveConcernID := r.focusConcernID(commit.FocusID)
	if continuedConcernID != "" {
		effectiveConcernID = continuedConcernID
	}
	withinConcernID, err := r.validateConcernContext(commit, effectiveConcernID)
	if err != nil {
		withheldProjections["within_concern_id"] = err.Error()
		commit.WithinConcernID = ""
		withinConcernID = ""
	}
	commit.WithinConcernID = withinConcernID
	contributesToConcernID, err := r.validateConcernContribution(commit, effectiveConcernID)
	if err != nil {
		withheldProjections["contributes_to_concern_id"] = err.Error()
		commit.ContributesToConcernID = ""
		contributesToConcernID = ""
	}
	commit.ContributesToConcernID = contributesToConcernID
	if len([]rune(strings.TrimSpace(commit.ThoughtThread))) > 2000 {
		return errors.New("thought thread is too large for a single attention pulse")
	}
	if err := validateCognitiveAction(commit.Action, r.state.Stage); err != nil {
		return err
	}
	if err := r.validateOrganOperation(commit.Action); err != nil {
		return err
	}
	if commit.Action.Kind == "organ_action" && r.state.PendingAction != nil {
		return newActionProgressBoundary("one body action is already being enacted; keep this meaning available until that organ returns Reality")
	}
	if r.state.Stage >= 10 && r.state.Lease != nil && r.state.Lease.ProfileSource == "next" {
		return errors.New("local assistance returns an answer for the main cognition; only the main cognition submits a cognitive commit")
	}
	if err := r.validateActionObjectFocus(commit, effectiveConcernID); err != nil {
		return err
	}
	if err := r.validateUnabsorbedActionScope(commit, effectiveConcernID); err != nil {
		return err
	}
	if commit.Action.Kind != "none" {
		if effectiveConcernID != "" {
			if open := r.openCommitmentForConcern(effectiveConcernID); open != nil && !commitAssimilates(commit, open.ID) {
				return fmt.Errorf("concern %q already has an unassimilated action commitment", effectiveConcernID)
			}
		}
		if err := r.validateActionProgress(commit.FocusID, commit.Action, effectiveConcernID); err != nil {
			return err
		}
	}
	if err := r.validateRealityUpdates(commit); err != nil {
		return err
	}
	r.state.LearningFeedback = r.retainValidLearningUpdates(&commit)
	if r.state.LearningFeedback != "" {
		withheldProjections["learning_updates"] = r.state.LearningFeedback
	}
	if err := r.validateNarrativeUpdate(commit); err != nil {
		// Narrative is a sparse projection of already usable cognition, not a
		// prerequisite for absorbing Reality or executing an independently valid
		// action. If the proposed projection has not earned that status, preserve
		// the appraisal, Memory and action while leaving Narrative unchanged.
		withheldProjections["narrative_update"] = err.Error()
		commit.NarrativeUpdate = ""
	}
	if err := validateLifeValueVector(commit.ValueOrientationUpdate, true); err != nil {
		return err
	}
	if strings.TrimSpace(commit.NarrativeUpdate) == "" && !lifeValueVectorEmpty(commit.ValueOrientationUpdate) {
		withheldProjections["value_orientation_update"] = "long-term value orientation changes with an accepted narrative update"
		commit.ValueOrientationUpdate = LifeValueVector{}
	}
	profile, err := r.validateResourceChoice(commit.ResourceChoice, commit.FocusID, commit.Action.Kind, effectiveConcernID)
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
	closureCondition, err := r.validateNewConcernClosureCondition(commit, effectiveConcernID)
	if err != nil {
		withheldProjections["new_concern_closure_condition"] = err.Error()
		commit.NewConcernClosureCondition = ""
		closureCondition = ""
	}
	commit.NewConcernClosureCondition = closureCondition
	emergingConsequence, err := r.validateEmergingConsequence(commit)
	if err != nil {
		withheldProjections["emerging_consequence"] = err.Error()
		commit.EmergingConsequence = ""
		emergingConsequence = ""
	}
	commit.EmergingConsequence = emergingConsequence
	focusCandidate := r.activeCandidates[commit.FocusID]
	focusOriginKind := focusCandidate.Kind
	if concernID := r.focusConcernID(commit.FocusID); concernID != "" {
		if concern := r.concernByID(concernID); concern != nil {
			focusOriginKind = concern.OriginKind
		}
	}
	// The one body slot or a result awaiting assimilation is a real waiting
	// condition, even while another Concern can still be thought about. A slow
	// organ may finish during inference; its unassimilated Reality preserves
	// that condition until the main stream has actually processed the result.
	if r.state.PendingAction == nil && !r.hasCommitmentAwaitingAssimilation() {
		if err := validateFocusedEnactment(commit, focusCandidate, focusOriginKind, r.config.Dynamics.AttentionThreshold); err != nil {
			return err
		}
	}
	normalizedCompositeDisposition := r.normalizeCompositeProgressDisposition(&commit, effectiveConcernID)
	if r.state.Stage >= 20 && commit.Action.Kind != "none" {
		// A new explicitly chosen action has no returned Reality yet. Decline
		// only the contradictory closure projection, preserving valid absorption
		// of the previous step and the next action. Do not manufacture a verdict.
		for index := range commit.Appraisals {
			appraisal := &commit.Appraisals[index]
			concern := r.concernForCandidate(r.activeCandidates[appraisal.CandidateID])
			if appraisal.CandidateID == commit.FocusID && appraisal.Resolution == "resolved" && concern != nil && concern.Resolution == "hold" {
				appraisal.Resolution = "hold"
				withheldProjections["concern_disposition"] = "resolved withheld: the newly chosen action has not returned Reality; the existing concern remains held"
			}
		}
	}
	for _, appraisal := range commit.Appraisals {
		candidate := r.activeCandidates[appraisal.CandidateID]
		concern := r.concernForCandidate(candidate)
		if concern == nil {
			continue
		}
		enactsFocusedConcern := appraisal.CandidateID == commit.FocusID && commit.Action.Kind != "none"
		isFocus := appraisal.CandidateID == commit.FocusID
		if !isFocus && r.state.Stage >= 8 {
			continue
		}
		if err := validateExistingConcernDisposition(appraisal, *concern, enactsFocusedConcern); err != nil {
			return err
		}
	}
	if err := r.validateConcernHierarchyDisposition(commit, effectiveConcernID); err != nil {
		return err
	}
	if continuedConcernID != "" {
		r.bindConcernContinuation(commit.FocusID, continuedConcernID)
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
			r.learnDifferenceFromAppraisal(candidate, appraisal, commit.Action.Kind != "none")
		} else {
			r.learnDifferenceFromAppraisal(candidate, appraisal, false)
		}
		if appraisal.CandidateID == commit.FocusID {
			if err := r.habituateSettledPerception(candidate, appraisal, commit.Action.Kind, now); err != nil {
				return err
			}
		}

		activation := appraisalActivation(r.config.Dynamics, appraisal)
		concern := r.concernForCandidate(candidate)
		created := false
		persistNewConcern := concern == nil && r.shouldPersistNewConcern(commit, appraisal, activation)
		if persistNewConcern && r.state.Stage >= 9 && appraisal.CandidateID == commit.FocusID && closureCondition == "" {
			// A durable Concern needs Alice's own stable reality boundary. Missing
			// that boundary no longer invalidates an otherwise usable appraisal or
			// action: this attention episode remains momentary and Reality can still
			// return through its ActionCommitment. The kernel declines to manufacture
			// persistence instead of discarding the whole cognitive result.
			persistNewConcern = false
		}
		if persistNewConcern {
			r.state.Concerns = append(r.state.Concerns, Concern{ID: "concern-" + randomID()})
			concern = &r.state.Concerns[len(r.state.Concerns)-1]
			created = true
			if appraisal.CandidateID == commit.FocusID {
				concern.WithinConcernID = withinConcernID
				concern.ClosureCondition = closureCondition
			}
		}
		persistConcernAppraisal := concern != nil && (r.state.Stage < 8 || appraisal.CandidateID == commit.FocusID)
		if persistConcernAppraisal {
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
			if concernResolution == "relieved" { // Compatibility with archived pre-G0.9 state.
				concernResolution = "released"
			}
			// Subject is the stable factual anchor Alice originally chose to own.
			// Later Reality can change its meaning, difference and strength without
			// silently replacing what the continuing Concern is about.
			if strings.TrimSpace(concern.Subject) == "" {
				concern.Subject = stableConcernSubject(candidate)
			}
			if concern.OriginKind == "" {
				concern.OriginKind = candidate.Kind
			}
			if commitmentID := commitmentIDFromEvent(candidate); commitmentID != "" {
				concern.CommitmentID = commitmentID
				// A momentary action may legitimately begin before its meaning has
				// become a durable Concern. When the returned Reality is what earns
				// that persistence, bind the already-existing commitment back to the
				// new Concern. Delayed consequences can then return to the same causal
				// line instead of reviving a stale wait or creating a parallel one.
				if commitment := r.commitmentByID(commitmentID); commitment != nil && commitment.ConcernID == "" {
					commitment.ConcernID = concern.ID
				}
			}
			concern.Meaning = strings.TrimSpace(appraisal.Meaning)
			concern.Difference = appraisal.Difference
			concern.Ownership = appraisal.Ownership
			concern.Value = appraisal.Value
			concern.Values = appraisal.Values
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
			if concernResolution == "resolved" || concernResolution == "released" {
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
		if appraisal.CandidateID == commit.FocusID {
			r.updateValueAffordanceDisposition(candidate, concern, commit.Action.Kind != "none", now)
		}
		if candidate.Kind != "concern" && appraisal.CandidateID != commit.FocusID {
			if r.state.Stage >= 9 && commitmentFeedbackKind(candidate.Kind) && commitmentIDFromEvent(candidate) != "" {
				// A causally linked Reality may be understood in the periphery, but
				// only its own single-focus pass can assimilate the commitment and
				// settle the Concern. Keep it pending even when the peripheral
				// appraisal already predicts resolution; backgrounding it here leaves
				// Alice waiting forever for a reply or result that actually arrived.
				markEvent(&r.state, appraisal.CandidateID, "pending")
			} else if r.state.Stage >= 9 && candidate.Kind == "concern_contribution" {
				// A contribution that lost this attention competition is still a new
				// factual wake-up for an already-owned parent. Keep the single merged
				// signal pending until it receives its own focus; the background
				// appraisal remains affective context and does not settle the parent.
				markEvent(&r.state, appraisal.CandidateID, "pending")
			} else if r.state.Stage >= 8 && appraisal.Resolution == "hold" && appraisal.Ownership >= r.config.Dynamics.AttentionThreshold {
				// Single-focus consciousness means "not now", not "never". A
				// non-focused event that Alice explicitly wants to keep affecting
				// her stays pending for a later pulse. It does not become a parallel
				// Concern until it is itself selected as the one focus.
				markEvent(&r.state, appraisal.CandidateID, "pending")
			} else {
				markEvent(&r.state, appraisal.CandidateID, "background")
			}
		}
		if candidate.Kind == "self_model_difference" && appraisal.CandidateID == commit.FocusID {
			// Owning an introspective question is not a verdict that its source
			// activity was worthless. The same AIP, value and personal-learning
			// paths carry its meaning forward; no separate channel-wide penalty.
			// SelfModelTension is the accumulated interoceptive discrepancy that
			// brought this evidence into consciousness. Once Alice has actually
			// focused it, her remaining D is the current unresolved discrepancy.
			// A held Concern may therefore remain alive at low tension without the
			// already-noticed pressure staying saturated and reopening after every
			// later Memory. Background appraisal alone does not metabolize it.
			if appraisal.Resolution == "resolved" || appraisal.Resolution == "released" {
				r.state.SelfModelTension = 0
			} else {
				r.state.SelfModelTension = clamp01(appraisal.Difference)
			}
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
		r.activateLifeValues(appraisal)
	}
	if explorationResultRelief > 0 {
		r.state.ValueField.Activation.Exploration = clamp01(
			r.state.ValueField.Activation.Exploration - r.config.Dynamics.ExplorationRelief*explorationResultRelief,
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
		const newMemoryWeight = 0.60
		r.state.AffectiveState.Valence = clampSigned((1-newMemoryWeight)*r.state.AffectiveState.Valence + newMemoryWeight*targetValence)
		r.state.AffectiveState.Activation = clamp01(maxFloat(r.state.AffectiveState.Activation*0.50, maxActivation))
		r.state.AffectiveState.Control = clamp01((1-newMemoryWeight)*r.state.AffectiveState.Control + newMemoryWeight*targetControl)
		r.state.AffectiveState.Certainty = clamp01((1-newMemoryWeight)*r.state.AffectiveState.Certainty + newMemoryWeight*targetCertainty)
	}
	r.state.ValueField.Activation.Exploration = clamp01(
		r.state.ValueField.Activation.Exploration + r.config.Dynamics.ExplorationUnknownGrowth*(uncertaintyTotal/float64(len(commit.Appraisals))),
	)
	r.state.LastAttentionAt = now
	// A contribution exists only to wake its still-open parent Concern. Once the
	// parent has directly settled its own closure condition, keeping that wake-up
	// pending would turn completed progress into a new, content-free thought loop.
	r.retireSettledConcernContributions()
	r.pruneInactiveConcerns()
	if err := r.applyResourceChoice(commit.ResourceChoice, profile, commit.FocusID, effectiveConcernID); err != nil {
		return err
	}
	if err := r.applyRealityUpdates(commit); err != nil {
		return err
	}
	if err := r.applyLearningUpdates(commit); err != nil {
		return err
	}
	if len(commit.RealityUpdates) == 0 && independentFactKind(focusCandidate.Kind) {
		if err := r.enqueueObservedConcernContribution(contributesToConcernID, focusCandidate); err != nil {
			return err
		}
	}
	if err := r.applyNarrativeUpdate(commit); err != nil {
		return err
	}
	r.applyValueOrientationUpdate(commit.ValueOrientationUpdate)
	if err := r.addMentorContentCandidate(commit); err != nil {
		return err
	}
	if err := r.addEmergingConsequence(commit); err != nil {
		return err
	}
	variationBias := ""
	variationSeed := ""
	if r.state.Lease != nil {
		variationBias = r.state.Lease.VariationBias
		variationSeed = r.state.Lease.VariationSeed
	}
	payload := map[string]any{
		"focus_id":                      commit.FocusID,
		"continues_concern_id":          continuedConcernID,
		"within_concern_id":             withinConcernID,
		"contributes_to_concern_id":     contributesToConcernID,
		"new_concern_closure_condition": closureCondition,
		"emerging_consequence":          emergingConsequence,
		"thought_thread":                truncate(strings.TrimSpace(commit.ThoughtThread), 2000),
		"appraisals":                    commit.Appraisals,
		"affective_state":               r.state.AffectiveState,
		"life_value_field":              r.state.ValueField,
		"value_orientation_update":      commit.ValueOrientationUpdate,
		"action_kind":                   commit.Action.Kind,
		"resource_choice":               commit.ResourceChoice,
		"variation_bias":                variationBias,
		"variation_seed":                variationSeed,
	}
	if withheldActionKind != "" {
		payload["withheld_action_kind"] = withheldActionKind
	}
	if len(withheldProjections) > 0 {
		payload["withheld_projections"] = withheldProjections
	}
	if normalizedCompositeDisposition != "" {
		payload["normalized_composite_disposition"] = normalizedCompositeDisposition
	}
	return r.journal("aip_commit", commit.FocusID, payload)
}

func (r *Runtime) retireSettledConcernContributions() {
	for index := range r.state.Background {
		event := &r.state.Background[index]
		if event.Kind != "concern_contribution" {
			continue
		}
		parentID, _ := concernContributionRelation(*event)
		if parentID == "" {
			parentID = strings.TrimSpace(event.ConcernID)
		}
		parent := r.concernByID(parentID)
		if parent != nil && parent.Resolution == "hold" {
			continue
		}
		event.Status = "processed"
		event.LastCommitErr = ""
		event.CognitionAttempts = 0
	}
}

func commitAssimilates(commit CognitiveCommit, commitmentID string) bool {
	return len(commit.RealityUpdates) == 1 && commit.RealityUpdates[0].CommitmentID == commitmentID
}

func validateExistingConcernDisposition(appraisal CandidateAppraisal, concern Concern, enacts bool) error {
	if concern.Resolution != "hold" {
		return nil
	}
	if enacts && appraisal.Resolution != "hold" {
		return fmt.Errorf("concern %q is being enacted and must remain held until Reality returns", concern.ID)
	}
	return nil
}

// validateConcernHierarchyDisposition keeps the lifecycle of a self-endorsed
// whole consistent with the lifecycles of the concrete consequences Alice put
// inside it. A parent may integrate a child's Memory, but it cannot declare
// the whole settled while that child still says its own closure condition is
// unfinished. This does not decide either lifecycle; it only makes Alice bring
// the remaining child to its own focus before settling the parent.
func (r *Runtime) validateConcernHierarchyDisposition(commit CognitiveCommit, effectiveConcernID string) error {
	if r.state.Stage < 9 || effectiveConcernID == "" {
		return nil
	}
	disposition := ""
	for _, appraisal := range commit.Appraisals {
		if appraisal.CandidateID == commit.FocusID {
			disposition = appraisal.Resolution
			break
		}
	}
	if disposition == "" || disposition == "hold" {
		return nil
	}
	parent := r.concernByID(effectiveConcernID)
	if parent == nil {
		return nil
	}
	for _, child := range r.state.Concerns {
		if child.ID == parent.ID || child.WithinConcernID != parent.ID || child.Resolution != "hold" {
			continue
		}
		return fmt.Errorf("concern %q cannot be %s while child concern %q remains held within it; first let the child reach its own closure condition, then settle the whole", parent.ID, disposition, child.ID)
	}
	return nil
}

// normalizeCompositeProgressDisposition separates completion of local progress
// from completion of a self-endorsed whole without spending another model call
// merely to restate that structural boundary. The generated meaning and values
// remain Alice's; only an impossible lifecycle transition is kept open until a
// later direct whole-concern appraisal receives the complete child ledger.
func (r *Runtime) normalizeCompositeProgressDisposition(commit *CognitiveCommit, effectiveConcernID string) string {
	if r.state.Stage < 9 || effectiveConcernID == "" {
		return ""
	}
	candidate, exists := r.activeCandidates[commit.FocusID]
	if !exists || (candidate.Kind != "action_result" && candidate.Kind != "concern_contribution") {
		return ""
	}
	hasChild := false
	for _, child := range r.state.Concerns {
		if child.WithinConcernID == effectiveConcernID {
			hasChild = true
			break
		}
	}
	if !hasChild {
		return ""
	}
	for index := range commit.Appraisals {
		appraisal := &commit.Appraisals[index]
		if appraisal.CandidateID != commit.FocusID || appraisal.Resolution == "" || appraisal.Resolution == "hold" {
			continue
		}
		original := appraisal.Resolution
		appraisal.Resolution = "hold"
		return original
	}
	return ""
}

func validateFocusedEnactment(commit CognitiveCommit, candidate Event, originKind string, threshold float64) error {
	if commit.Action.Kind != "none" || originKind == "endogenous_change" {
		return nil
	}
	if candidate.Kind == "action_result" {
		// Reality absorption is itself the single foreground operation in this
		// attention moment. A held Concern retains one bounded, immediate revisit
		// through concernAwaitsRealityRevisit, where Alice can choose the next
		// action after the result has actually become Memory. Requiring her to
		// interpret an unexpected result and enact its sequel in one commit breaks
		// the serial loop and turns ordinary reflection into validation waste.
		return nil
	}
	// Once that same Concern directly returns to focus, a highly answerable,
	// self-owned difference needs an action, closure, or an actual waiting
	// condition. Fresh external events may still be understood and released
	// without compulsory enactment.
	if candidate.Kind != "concern" {
		return nil
	}
	for _, appraisal := range commit.Appraisals {
		if appraisal.CandidateID != commit.FocusID || appraisal.Resolution != "hold" {
			continue
		}
		if appraisal.Difference >= threshold &&
			appraisal.Ownership >= threshold &&
			absFloat(appraisal.Value) >= threshold &&
			appraisal.Answerability >= threshold {
			return errors.New("a held concern cannot remain highly different, self-owned, valuable, and currently answerable while choosing unconditional non-action; take one bounded action, resolve the concern, or appraise the actual waiting condition")
		}
	}
	return nil
}

// validateActionObjectFocus preserves causal identity at the boundary between
// attention and bodily action. When a System Organ action names the exact path of a
// different independent object that is still present in the attention field,
// that object—not a broad parent Concern—must own the action and its returning
// Reality. This does not choose an object or require action; it only prevents a
// chosen action from being recorded as the life of something else.
func (r *Runtime) validateActionObjectFocus(commit CognitiveCommit, effectiveConcernID string) error {
	if r.state.Stage < 9 || commit.Action.Kind != "organ_action" || commit.Action.OrganID != "system" || commit.Action.Operation != "exec" {
		return nil
	}
	command := strings.TrimSpace(commit.Action.Input)
	if command == "" {
		return nil
	}
	candidates := make(map[string]Event, len(r.state.Background)+len(r.activeCandidates))
	for _, candidate := range r.state.Background {
		candidates[candidate.ID] = candidate
	}
	for candidateID, candidate := range r.activeCandidates {
		candidates[candidateID] = candidate
	}
	for candidateID, candidate := range candidates {
		if candidateID == commit.FocusID || candidate.Kind != "environment_change" {
			continue
		}
		if concern := r.concernForCandidate(candidate); concern != nil {
			if concern.ID == effectiveConcernID || concern.Resolution != "hold" {
				continue
			}
		}
		path := eventObjectPath(candidate)
		if path == "" || !strings.Contains(command, path) {
			continue
		}
		return fmt.Errorf("body action explicitly targets independent candidate %q at %q; select that object as the single focus so its Reality keeps its own causal identity", candidateID, path)
	}
	return nil
}

func eventObjectPath(candidate Event) string {
	if len(candidate.Payload) == 0 {
		return ""
	}
	var payload struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(candidate.Payload, &payload); err != nil {
		return ""
	}
	path := strings.TrimSpace(payload.Path)
	if !strings.HasPrefix(path, "/") {
		return ""
	}
	return path
}

func bodyHasOrganCapability(body BodySnapshot, expected string) bool {
	for _, organ := range body.Organs {
		if !organ.Accepting || organ.Status == "unavailable" {
			continue
		}
		for _, capability := range organ.Capabilities {
			if capability == expected {
				return true
			}
		}
	}
	return false
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
			candidate.Kind == "self_model_difference" && commit.Action.Kind == "none" {
			// A self-model difference is an interoceptive observation, not a duty to
			// prove that Alice has improved.  It may update Narrative, methods,
			// affect and value orientation immediately; durable ownership begins only
			// when it grounds a real action or later reality independently reopens it.
			// Persisting a held, non-enacted audit manufactured meta-Concerns such as
			// "complete three more activities", which then consumed attention while
			// simultaneously telling Alice not to act merely to satisfy the audit.
			return false
		}
		if candidate, exists := r.activeCandidates[appraisal.CandidateID]; exists &&
			candidate.Kind == "value_signal" && commit.Action.Kind == "none" {
			// A value signal is a drive meeting an available affordance, not yet a
			// concrete object Alice has taken responsibility for.  If no reality
			// contact follows, the value field already preserves the attraction;
			// turning "an entrance exists" into a durable Concern manufactures the
			// empty waiting loops that the signal was meant to avoid.
			return false
		}
		if candidate, exists := r.activeCandidates[appraisal.CandidateID]; exists &&
			candidate.Kind == "endogenous_change" && commit.Action.Kind == "none" {
			// Exploration pressure is a drive, not a thing Alice must keep thinking
			// about. Concrete continuing orientation is now represented uniformly as
			// a value_signal bound to a real affordance.
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
	// Once Alice has selected the one focus, explicitly says hold, and assigns
	// self-ownership above the threshold, the kernel must preserve that choice as
	// a Concern even when its present activation is quiet. Activation determines
	// strength and later competition; it cannot erase an adopted responsibility
	// merely because its next Reality has not arrived yet. The endogenous-drive
	// and low-yield-perception exceptions above still prevent empty exploration
	// pressure and passive noticing from multiplying into Concerns.
	return appraisal.Resolution == "hold"
}

// validateNewConcernClosureCondition gives every Stage 9 Concern one stable,
// self-authored reality boundary at formation. Meaning and Difference may
// evolve as Alice learns, while a successful sub-action cannot silently shrink
// the whole Concern into whatever just happened. Existing Concerns inherit the
// condition they already own; release remains available when values change.
func (r *Runtime) validateNewConcernClosureCondition(commit CognitiveCommit, effectiveConcernID string) (string, error) {
	if r.state.Stage < 9 || effectiveConcernID != "" {
		return "", nil
	}
	var focusAppraisal *CandidateAppraisal
	for index := range commit.Appraisals {
		if commit.Appraisals[index].CandidateID == commit.FocusID {
			focusAppraisal = &commit.Appraisals[index]
			break
		}
	}
	if focusAppraisal == nil {
		return "", nil
	}
	activation := appraisalActivation(r.config.Dynamics, *focusAppraisal)
	if r.concernForCandidate(r.activeCandidates[commit.FocusID]) != nil ||
		!r.shouldPersistNewConcern(commit, *focusAppraisal, activation) {
		return "", nil
	}
	condition := strings.TrimSpace(commit.NewConcernClosureCondition)
	// An absent boundary means this appraisal has not yet earned durable Concern
	// identity. The caller still accepts the thought and any bounded action; it
	// simply keeps this episode momentary rather than asking a validation retry to
	// restate the same meaning in more ceremonial form.
	if condition == "" {
		return "", nil
	}
	if len([]rune(condition)) > 512 {
		return "", errors.New("new concern closure condition is too large")
	}
	return condition, nil
}

// validateEmergingConsequence preserves a second, causally distinct consequence
// that Alice herself notices while absorbing one action Reality. It does not
// create a Concern or a parallel focus: the consequence returns later as an
// ordinary candidate and must pass the same appraisal and ownership rules as
// every other object.
func (r *Runtime) validateEmergingConsequence(commit CognitiveCommit) (string, error) {
	consequence := strings.TrimSpace(commit.EmergingConsequence)
	if consequence == "" {
		return "", nil
	}
	if r.state.Stage < 9 {
		return "", errors.New("emerging consequence becomes available in stage nine")
	}
	candidate, exists := r.activeCandidates[commit.FocusID]
	if !exists {
		return "", fmt.Errorf("focus %q is not an active candidate", commit.FocusID)
	}
	commitmentID := commitmentIDFromEvent(candidate)
	if candidate.Kind != "action_result" || commitmentID == "" {
		return "", errors.New("an emerging consequence must come from the focused action_result")
	}
	if !commitAssimilates(commit, commitmentID) {
		return "", errors.New("the current action Reality must be absorbed before its emerging consequence can enter later attention")
	}
	if len([]rune(consequence)) > 512 {
		return "", errors.New("emerging consequence is too large")
	}
	return consequence, nil
}

// addMentorContentCandidate separates the two factual roles of one linked
// mentor reply without introducing parallel consciousness. The first pass has
// just absorbed the reply as delayed Reality for Alice's earlier mentor_send;
// the next candidate presents the same utterance as new incoming content. That
// second pass may form or release a new Concern through ordinary AIP rules.
func (r *Runtime) addMentorContentCandidate(commit CognitiveCommit) error {
	if r.state.Stage != 9 {
		return nil
	}
	source, exists := r.activeCandidates[commit.FocusID]
	if !exists || source.Kind != "mentor_received" {
		return nil
	}
	commitmentID := commitmentIDFromEvent(source)
	if commitmentID == "" || !commitAssimilates(commit, commitmentID) {
		return nil
	}
	body := strings.TrimSpace(source.Summary)
	if body == "" {
		return nil
	}
	payload, err := json.Marshal(map[string]any{
		"source_event_id": source.ID,
		"message_id":      source.CorrelationID,
		"message_body":    truncate(body, 16*1024),
	})
	if err != nil {
		return err
	}
	return r.addEvent(
		"mentor_content",
		"observed",
		truncate(body, 16*1024),
		source.ID,
		payload,
		true,
	)
}

func (r *Runtime) addEmergingConsequence(commit CognitiveCommit) error {
	consequence := strings.TrimSpace(commit.EmergingConsequence)
	if consequence == "" {
		return nil
	}
	source := r.activeCandidates[commit.FocusID]
	payload := map[string]any{
		"source_event_id":      source.ID,
		"source_kind":          source.Kind,
		"source_summary":       truncate(strings.TrimSpace(source.Summary), 4096),
		"emerging_consequence": consequence,
	}
	// Keep a bounded copy of the factual Reality beside Alice's interpretation.
	// Larger tool results remain available through the Memory and journal;
	// duplicating them here would inflate every later cognitive context.
	if len(source.Payload) > 0 && len(source.Payload) <= 16*1024 {
		var sourcePayload any
		if json.Unmarshal(source.Payload, &sourcePayload) == nil {
			payload["source_payload"] = sourcePayload
		}
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return r.addEvent(
		"reality_consequence",
		"self_interpreted",
		consequence,
		source.ID,
		encoded,
		true,
	)
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
			if concern := r.concernForCandidate(event); concern != nil {
				return concern.ID
			}
			return ""
		}
	}
	if candidate, ok := r.activeCandidates[focusID]; ok {
		if concern := r.concernForCandidate(candidate); concern != nil {
			return concern.ID
		}
	}
	return ""
}

func (r *Runtime) validateConcernContinuation(commit CognitiveCommit) (string, error) {
	concernID := strings.TrimSpace(commit.ContinuesConcernID)
	if concernID == "" {
		return "", nil
	}
	candidate, exists := r.activeCandidates[commit.FocusID]
	if !exists {
		return "", fmt.Errorf("focus %q is not an active candidate", commit.FocusID)
	}
	if candidate.ConcernID != "" || candidate.Kind == "concern" || commitmentIDFromEvent(candidate) != "" {
		// The strict tool schema cannot make continues_concern_id conditional on
		// which candidate the model selects as focus. A Reality that already has
		// causal identity always stays in that thread; an extra continuation value
		// is therefore redundant input, not a reason to discard the cognition.
		return "", nil
	}
	concern := r.concernByID(concernID)
	if concern == nil || concern.Resolution != "hold" {
		return "", fmt.Errorf("continued concern %q is not an active held concern", concernID)
	}
	if concern.OriginKind == "birth_orientation" {
		return "", errors.New("a later independent event cannot overwrite the stable birth orientation; leave continues_concern_id empty so the new fact can keep its own causal identity")
	}
	if r.openCommitmentForConcern(concernID) != nil {
		return "", fmt.Errorf("continued concern %q already has an unassimilated action commitment", concernID)
	}
	if independentFactKind(candidate.Kind) {
		// Explicit replies and enacted results already carry causal identity from
		// the body.  Every other external, bodily, perceptual, or self-model fact
		// keeps its own possible consequence.  Sharing a speaker, topic, relation,
		// or interpretation is not enough to overwrite an older Concern; Alice can
		// relate real outcomes through contribution without losing either identity.
		return "", nil
	}
	return concernID, nil
}

func independentFactKind(kind string) bool {
	switch kind {
	case "mentor_received", "mentor_content", "environment_change", "perceptual_change", "body_delta", "self_model_difference", "reality_consequence", "operation_condition":
		return true
	default:
		return false
	}
}

func (r *Runtime) validateConcernContext(commit CognitiveCommit, effectiveConcernID string) (string, error) {
	withinID := strings.TrimSpace(commit.WithinConcernID)
	if withinID == "" {
		return "", nil
	}
	candidate, exists := r.activeCandidates[commit.FocusID]
	if !exists {
		return "", fmt.Errorf("focus %q is not an active candidate", commit.FocusID)
	}
	if effectiveConcernID != "" || candidate.Kind == "concern" || commitmentIDFromEvent(candidate) != "" {
		// Context is chosen once when an independent Concern first forms. Later
		// cognition inherits that stable relation from the Concern itself.
		return "", nil
	}
	if !independentFactKind(candidate.Kind) {
		return "", nil
	}
	parent := r.concernByID(withinID)
	if parent == nil || parent.Resolution != "hold" || parent.Ownership < r.config.Dynamics.AttentionThreshold {
		return "", fmt.Errorf("within concern %q is not a currently held self-owned concern", withinID)
	}
	if parent.OriginKind == "birth_orientation" {
		return "", errors.New("a concrete episode may shape Narrative without turning the stable birth orientation into its parent concern")
	}
	return withinID, nil
}

// validateConcernContribution keeps each real episode distinct while allowing
// Alice to decide that an independently observed fact or an actual child
// Memory advances one Concern she already owns. Prediction, intention, or
// semantic similarity alone cannot manufacture contribution.
func (r *Runtime) validateConcernContribution(commit CognitiveCommit, effectiveConcernID string) (string, error) {
	concernID := strings.TrimSpace(commit.ContributesToConcernID)
	if concernID == "" {
		return "", nil
	}
	if concernID == effectiveConcernID {
		// The focused Concern already receives this action's Reality.  Treating the
		// same ID as an empty link preserves the selected action without creating a
		// self-loop or spending another cognition on a losslessly removable
		// redundancy.  A genuinely different parent remains Alice's explicit choice.
		return "", nil
	}
	candidate, exists := r.activeCandidates[commit.FocusID]
	if !exists {
		return "", fmt.Errorf("focus %q is not an active candidate", commit.FocusID)
	}
	concern := r.concernByID(concernID)
	if concern == nil || concern.Resolution != "hold" || concern.Ownership < r.config.Dynamics.AttentionThreshold {
		return "", fmt.Errorf("contributed concern %q is not a currently held self-owned concern", concernID)
	}
	if concern.OriginKind == "birth_orientation" {
		return "", errors.New("concrete episodes shape Narrative and Memory without repeatedly reactivating the stable birth orientation")
	}
	if independentFactKind(candidate.Kind) {
		// An independently observed Reality keeps its own causal identity. Alice may
		// nevertheless recognize that the fact changes whether an already-owned
		// Concern's stable closure condition is satisfied. The kernel only wakes the
		// target; it does not edit or settle that Concern here.
		return concernID, nil
	}
	if len(commit.RealityUpdates) != 1 {
		// Before Reality there is only a prediction of relevance.  The actual
		// relationship is chosen in the cognition that forms Memory, so an
		// early field value is a losslessly removable anticipation rather than a
		// reason to reject the selected action.
		return "", nil
	}
	child := r.concernByID(effectiveConcernID)
	if child == nil || child.WithinConcernID == "" {
		return "", fmt.Errorf("focused concern %q has no previously self-endorsed within relation; a new Memory cannot invent a parent from semantic similarity", effectiveConcernID)
	}
	if concernID != child.WithinConcernID {
		return "", fmt.Errorf("contributed concern %q is not the focused concern's self-endorsed parent %q", concernID, child.WithinConcernID)
	}
	return concernID, nil
}

func stableConcernSubject(candidate Event) string {
	subject := strings.TrimSpace(candidate.Summary)
	payload := strings.TrimSpace(string(candidate.Payload))
	if payload != "" && payload != "null" && payload != "{}" {
		if subject != "" {
			subject += "\n"
		}
		subject += "事实载荷：" + payload
	}
	return truncate(subject, 512)
}

func (r *Runtime) bindConcernContinuation(candidateID, concernID string) {
	candidate, exists := r.activeCandidates[candidateID]
	if exists {
		candidate.ConcernID = concernID
		r.activeCandidates[candidateID] = candidate
	}
	r.linkConcern(candidateID, concernID)
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

// concernAwaitsRealityRevisit preserves one bounded chance to act on a
// self-endorsed "hold" after Reality has been assimilated. D and activation are
// Alice's estimates, not mechanically infallible truth: a low estimate in the
// same pulse must not erase her explicit judgment that a concrete consequence
// remains. Once the Concern itself is focused, LastSourceID becomes its own ID;
// the existing selfRevisitWithoutReality rule then prevents further reflection
// without another fact.
func (r *Runtime) concernAwaitsRealityRevisit(concern Concern) bool {
	if concern.Resolution != "hold" || concern.LastSourceID == "" || concern.LastSourceID == concern.ID {
		return false
	}
	for _, event := range r.state.Background {
		if event.ID == concern.LastSourceID {
			return commitmentFeedbackKind(event.Kind)
		}
	}
	return false
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

func (r *Runtime) validateResourceChoice(choice CognitiveResourceChoice, focusID, actionKind string, effectiveConcernIDs ...string) (CognitiveProfile, error) {
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
	if r.state.Stage >= 10 {
		if r.state.Lease != nil && r.state.Lease.ProfileSource == "next" && choice.Apply != "keep" {
			return CognitiveProfile{}, errors.New("local assistance is one-use and returns to the main profile")
		}
		if choice.Apply == "next" && profile != roleProfile(r.config.CognitiveResource, "high") {
			if profile != roleProfile(r.config.CognitiveResource, "fast") || assistanceTask(choice.Task) != "reasoning" {
				return CognitiveProfile{}, errors.New("serial assistance uses the configured fast role for simple reasoning or high role for complex reasoning and implementation")
			}
		}
		if choice.Apply == "next" {
			if task := assistanceTask(choice.Task); task != "reasoning" && task != "implementation" {
				return CognitiveProfile{}, errors.New("assistance task must be reasoning or implementation")
			}
			if choice.IncludeSelf && profile.Model != "high" {
				return CognitiveProfile{}, errors.New("self narrative reference is available only to high-level assistance")
			}
		}
		if choice.Apply == "next" && actionKind != "none" {
			return CognitiveProfile{}, errors.New("serial assistance requires action none; its result returns to the main cognition before execution")
		}
		if choice.Apply == "default" && profile != r.config.CognitiveResource.InitialDefaultProfile {
			return CognitiveProfile{}, errors.New("this generation keeps its selected main profile; use next for local reasoning or implementation assistance")
		}
	}
	if choice.Apply == "default" && r.state.Body.CognitiveResourceBand == "open" &&
		cognitiveProfileRank(profile) < cognitiveProfileRank(r.config.CognitiveResource.InitialDefaultProfile) {
		return CognitiveProfile{}, errors.New("cognitive resources are open; keep the capability-first birth baseline as the persistent default and use next for one bounded lower-cost cognition")
	}
	if choice.Apply == "next" {
		candidate := r.activeCandidates[focusID]
		effectiveConcernID := r.focusConcernID(focusID)
		if len(effectiveConcernIDs) > 0 && strings.TrimSpace(effectiveConcernIDs[0]) != "" {
			effectiveConcernID = strings.TrimSpace(effectiveConcernIDs[0])
		}
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
		if r.state.Stage >= 10 && assistanceTask(choice.Task) == "implementation" && effectiveConcernID == "" {
			return CognitiveProfile{}, errors.New("stage-ten action assistance requires an already owned concern and fixed action purpose")
		}
	}
	return profile, nil
}

func (r *Runtime) applyResourceChoice(choice CognitiveResourceChoice, profile CognitiveProfile, focusID string, effectiveConcernIDs ...string) error {
	switch choice.Apply {
	case "keep":
		// Keep preserves the persistent default. A profile chosen with next is
		// therefore genuinely one-use; persisting it requires an explicit default
		// choice rather than a routine Reality absorption silently changing every
		// future cognition.
		return nil
	case "default":
		previousModel := r.state.CognitiveResource.DefaultProfile.Model
		r.state.CognitiveResource.DefaultProfile = profile
		if previousModel != "" && previousModel != profile.Model {
			r.releaseModelWaits(previousModel)
		}
		return r.journal("cognitive_profile_changed", focusID, map[string]any{"profile": profile, "purpose": strings.TrimSpace(choice.Purpose)})
	case "next":
		payload, _ := json.Marshal(assistanceContract{Purpose: strings.TrimSpace(choice.Purpose), Profile: profile, Task: assistanceTask(choice.Task), IncludeSelf: choice.IncludeSelf})
		// A serial continuation is Alice continuing the present cognitive thread,
		// not a new source of life tension. Carry the already formed Concern across
		// the continuation so appraisal cannot manufacture a duplicate identity.
		concernID := r.focusConcernID(focusID)
		if len(effectiveConcernIDs) > 0 && strings.TrimSpace(effectiveConcernIDs[0]) != "" {
			concernID = strings.TrimSpace(effectiveConcernIDs[0])
		}
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
	heldParents := make(map[string]bool)
	for _, concern := range r.state.Concerns {
		if concern.Resolution == "hold" {
			heldParents[concern.ID] = true
		}
	}
	for _, concern := range r.state.Concerns {
		// A settled child is no longer an active tension, but while its parent is
		// still held it remains the parent's causal evidence that this distinct
		// consequence has already been handled. Reusing the Concern hierarchy as
		// this compact ledger avoids both a second task system and the loss of early
		// completions when a later sibling refreshes the parent's attention. Once
		// the parent settles, ordinary pruning removes the whole completed branch.
		settledChildOfHeldParent := concern.WithinConcernID != "" &&
			heldParents[concern.WithinConcernID] &&
			(concern.Resolution == "resolved" || concern.Resolution == "released")
		if settledChildOfHeldParent {
			kept = append(kept, concern)
			continue
		}
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
			lifeValuePressure(r.state.ValueField).Exploration >= r.config.Dynamics.AttentionThreshold {
			kept = append(kept, concern)
			continue
		}
		if r.concernAwaitsRealityRevisit(concern) {
			kept = append(kept, concern)
			continue
		}
		// "hold" is Alice's explicit decision that a difference still belongs
		// to her. Natural decay may make it quiet and keep it out of attention,
		// but the kernel must not convert silence into release. Alice releases a
		// Concern by reappraising its ownership or resolution; fresh Reality can
		// later reconnect to this dormant identity.
		if concern.Resolution == "hold" && concern.Ownership >= r.config.Dynamics.AttentionThreshold {
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
		// in Memory and Narrative and may become relevant again through new
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
	if r.state.Stage < 5 || lifeValuePressure(r.state.ValueField).Exploration < r.config.Dynamics.AttentionThreshold {
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
	return lifeValuePressure(r.state.ValueField).Exploration >= explorationActionThreshold(r.config.Dynamics.AttentionThreshold) &&
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
	if err := validateLifeValueVector(appraisal.Values, true); err != nil {
		return fmt.Errorf("candidate %q has invalid life values: %w", appraisal.CandidateID, err)
	}
	switch appraisal.Resolution {
	case "hold", "reframed", "relieved", "resolved", "released":
		return nil
	default:
		return fmt.Errorf("candidate %q has unknown resolution %q", appraisal.CandidateID, appraisal.Resolution)
	}
}

func validateCognitiveAction(action CognitiveAction, stage int) error {
	switch action.Kind {
	case "none":
		return nil
	case "organ_action":
		if strings.TrimSpace(action.OrganID) == "" || strings.TrimSpace(action.Operation) == "" {
			return errors.New("organ_action requires organ_id and operation")
		}
		if strings.TrimSpace(action.Input) == "" {
			return errors.New("organ_action requires an explicit input; use {} when the operation takes no arguments")
		}
		if action.OrganID == "system" && action.Operation == "exec" && !shellActionContactsReality(action.Input) {
			return errors.New("system exec must read or change a body or world fact; express waiting or deliberate non-action with none")
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

// validateOrganOperation keeps the host's passive protocol vocabulary out of
// Alice's intentional action payload. Each organ is the factual owner of its
// callable surface; cognition chooses from that published catalog rather than
// inferring an implementation name from capabilities or prose.
func (r *Runtime) validateOrganOperation(action CognitiveAction) error {
	if action.Kind != "organ_action" {
		return nil
	}
	if r.organs == nil {
		return errors.New("organ host is unavailable")
	}
	description, exists := r.organs.Description(action.OrganID)
	if !exists || !stringSliceContains(description.Capabilities, "perform") {
		return fmt.Errorf("organ %q is unavailable for action", action.OrganID)
	}
	if !stringSliceContains(description.Operations, action.Operation) {
		return fmt.Errorf("organ %q operation %q is not in its published operations; choose one of: %s", action.OrganID, action.Operation, strings.Join(description.Operations, ", "))
	}
	return nil
}

func cognitiveActionContactsReality(action CognitiveAction) bool {
	switch action.Kind {
	case "mentor_send":
		return strings.TrimSpace(action.Text) != ""
	case "organ_action":
		if action.OrganID == "system" && action.Operation == "exec" {
			return shellActionContactsReality(action.Input)
		}
		return strings.TrimSpace(action.OrganID) != "" && strings.TrimSpace(action.Operation) != ""
	default:
		return false
	}
}

// shellActionContactsReality rejects commands whose only observable effect is
// consuming time or returning success.  Waiting is a legitimate internal and
// causal state, but wrapping it in `sleep`, `true`, `:` or static output must
// not manufacture a body contact and Memory.  Any substantive command in
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
	case "resolved", "released":
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
		payload, _ := json.Marshal(ActionState{ID: actionID, LeaseID: leaseID, CommitmentID: action.CommitmentID, Kind: "mentor_send", Effect: "changed", Request: action.Text, Status: "completed", StartedAt: nowUTC(), EndedAt: nowUTC(), Result: messageID})
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
	case "organ_action":
		if r.state.PendingAction != nil {
			return errors.New("another body action is already in progress")
		}
		r.actionEpoch++
		if r.perceptionCancel != nil {
			r.perceptionCancel()
		}
		if r.organs == nil {
			return errors.New("organ host is unavailable")
		}
		description, exists := r.organs.Description(action.OrganID)
		if !exists || !stringSliceContains(description.Capabilities, "perform") {
			return fmt.Errorf("organ %q is unavailable for action", action.OrganID)
		}
		if !stringSliceContains(description.Operations, action.Operation) {
			return fmt.Errorf("organ %q does not publish operation %q", action.OrganID, action.Operation)
		}
		actionID := "action-" + randomID()
		if discarded := r.supersedePerceptualBatchForAction(action.OrganID); discarded > 0 {
			if err := r.journal("perceptual_batch_superseded", actionID, map[string]any{
				"organ_id": action.OrganID, "discarded_objects": discarded,
				"reason": "intentional organ action started",
			}); err != nil {
				return err
			}
		}
		r.state.PendingAction = &ActionState{
			ID:           actionID,
			LeaseID:      leaseID,
			CommitmentID: action.CommitmentID,
			Kind:         "organ_action",
			OrganID:      action.OrganID,
			Operation:    action.Operation,
			Request:      action.Input,
			Status:       "started",
			StartedAt:    nowUTC(),
		}
		if commitment := r.commitmentByID(action.CommitmentID); commitment != nil {
			r.state.PendingAction.ConcernID = commitment.ConcernID
			commitment.ActionID = actionID
			commitment.Status = "acting"
		}
		if err := r.journal("action_started", actionID, map[string]any{"kind": "organ_action", "organ_id": action.OrganID, "operation": action.Operation, "input": action.Input, "timeout_seconds": 120, "commitment_id": action.CommitmentID}); err != nil {
			return err
		}
		if err := r.persist(); err != nil {
			return err
		}
		go func() {
			callCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
			defer cancel()
			performed, err := r.organs.Perform(callCtx, action.OrganID, organ.ActionRequest{
				ActionID: actionID, Operation: action.Operation, Input: action.Input, TimeoutMilliseconds: 120_000,
			})
			result := ActionResultNotice{ActionID: actionID, Status: "failed", Effect: "unknown"}
			if err != nil {
				if errors.Is(callCtx.Err(), context.DeadlineExceeded) || errors.Is(callCtx.Err(), context.Canceled) {
					result.Status = "unknown"
				}
				result.Result = fmt.Sprintf(`{"error":%q}`, truncate(err.Error(), 2048))
			} else {
				result.Status = performed.Status
				result.Effect = performed.Effect
				result.Result = performed.Output
				if performed.Observation != nil {
					observation := observationFromOrgan(*performed.Observation)
					result.Observation = &observation
				}
			}
			result.Result = redactRuntimeSecret(result.Result, r.config.ModelGateway.APIKey)
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
	completed.Status = result.Status
	completed.Effect = result.Effect
	if completed.Status != "completed" && completed.Status != "failed" && completed.Status != "unknown" {
		completed.Status = "unknown"
	}
	completed.EndedAt = nowUTC()
	completed.Result = truncate(result.Result, 64*1024)
	if completed.Kind == "organ_action" {
		if err := r.recordOperationOutcome(completed.OrganID, completed.Operation, completed.Status, completed.Result, false); err != nil {
			return err
		}
	}
	if result.Observation != nil {
		completed.ObservedSurfaceID = result.Observation.SurfaceID
		completed.ObservedDigest = result.Observation.Digest
		newlySeen := r.assimilateActionObservation(*result.Observation)
		if err := r.journal("action_observation_assimilated", completed.ID, map[string]any{
			"organ_id": result.Observation.OrganID, "surface_id": result.Observation.SurfaceID,
			"digest": result.Observation.Digest, "newly_seen_objects": newlySeen,
		}); err != nil {
			return err
		}
	}
	completedConcernID := ""
	if commitment := r.commitmentByID(completed.CommitmentID); commitment != nil {
		completedConcernID = commitment.ConcernID
		completed.ConcernID = completedConcernID
		r.annotateActionAssistanceOpportunity(&completed, completedConcernID)
	} else if completed.ConcernID != "" {
		completedConcernID = completed.ConcernID
		r.annotateActionAssistanceOpportunity(&completed, completedConcernID)
	}
	if err := r.journal("action_"+completed.Status, completed.ID, map[string]any{"kind": completed.Kind, "organ_id": completed.OrganID, "operation": completed.Operation, "result": completed.Result}); err != nil {
		return err
	}
	payload, _ := json.Marshal(completed)
	summary := "一项身体行动已经完成，并返回了真实结果。"
	if completed.Status == "failed" {
		summary = "一项身体行动已结束，现实结果表明操作失败。"
	} else if completed.Status == "unknown" {
		summary = "一项身体行动被中断；现实是否已经部分改变尚不确定。"
	}
	if completed.ActionAssistanceAvailable {
		summary += " 同一关切的精确实现已连续遇到困难；一次进阶行动协助当前可用。"
	}
	if err := r.addEvent("action_result", "observed", summary, completed.ID, payload, true); err != nil {
		return err
	}
	realityEventID := fmt.Sprintf("event-%012d", r.state.EventSeq)
	if commitment := r.commitmentByID(completed.CommitmentID); commitment != nil {
		if completed.Status == "unknown" {
			commitment.Status = "reality_unknown"
		} else {
			commitment.Status = "reality_available"
		}
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

func stringSliceContains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
