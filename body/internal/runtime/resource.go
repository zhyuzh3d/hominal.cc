package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	microusdPerUSD     int64 = 1_000_000
	tokensPerMillion   int64 = 1_000_000
	resourceHourWindow       = time.Hour
	resourceDayWindow        = 24 * time.Hour
	// A settled spend can move a rolling balance across the same boundary that
	// an older spend is leaving.  Those crossings remain bodily facts, but they
	// must not buy a new thought on every alternation.
	stageTwentyResourceAttentionCooldown = 5 * time.Minute
)

func normalizeResourceConfig(config *Config) error {
	switch modelGatewayAdapter(config.ModelGateway) {
	case "openai", "llmserver":
	default:
		return fmt.Errorf("unsupported model gateway adapter %q", config.ModelGateway.Adapter)
	}
	if config.ModelGateway.MaxOutputTokens <= 0 {
		config.ModelGateway.MaxOutputTokens = 1200
	}
	resource := &config.CognitiveResource
	if resource.RollingHourLimitMicrousd <= 0 || resource.RollingDayLimitMicrousd <= 0 {
		return errors.New("cognitive resource hour and day limits are required")
	}
	if len(resource.Models) != 3 {
		return fmt.Errorf("cognitive resource requires exactly three models, got %d", len(resource.Models))
	}
	for _, name := range []string{"fast", "main", "high"} {
		model, ok := resource.Models[name]
		if !ok || strings.TrimSpace(model.ID) == "" {
			return fmt.Errorf("cognitive model %q is required", name)
		}
		if model.InputPerMillionMicrousd <= 0 || model.CachedInputPerMillionMicrousd <= 0 || model.OutputPerMillionMicrousd <= 0 {
			return fmt.Errorf("cognitive model %q has incomplete pricing", name)
		}
		if len(model.SupportedReasoningEfforts) == 0 {
			return fmt.Errorf("cognitive model %q has no reasoning efforts", name)
		}
	}
	if err := validateProfile(*resource, resource.InitialDefaultProfile); err != nil {
		return fmt.Errorf("initial cognitive profile: %w", err)
	}
	if resource.ValidationRetryPerFocus <= 0 {
		resource.ValidationRetryPerFocus = 1
	}
	if resource.ContinuationPerFocus <= 0 {
		resource.ContinuationPerFocus = 1
	}
	if resource.PaidFailureThreshold <= 0 {
		resource.PaidFailureThreshold = 3
	}
	if resource.PaidFailureWindowMinutes <= 0 {
		resource.PaidFailureWindowMinutes = 10
	}
	if resource.ModelProtectionMinutes <= 0 {
		resource.ModelProtectionMinutes = 10
	}
	return nil
}

func validateProfile(resource CognitiveResourceConfig, profile CognitiveProfile) error {
	model, ok := resource.Models[profile.Model]
	if !ok {
		return fmt.Errorf("unknown cognitive model %q", profile.Model)
	}
	for _, effort := range model.SupportedReasoningEfforts {
		if profile.ReasoningEffort == effort {
			return nil
		}
	}
	return fmt.Errorf("model %q does not support reasoning effort %q", profile.Model, profile.ReasoningEffort)
}

func cognitiveProfileRank(profile CognitiveProfile) int {
	modelRank := map[string]int{"fast": 0, "main": 10, "high": 20}[profile.Model]
	effortRank := map[string]int{
		"none": 0, "low": 1, "medium": 2, "high": 3, "xhigh": 4, "max": 5,
	}[profile.ReasoningEffort]
	return modelRank + effortRank
}

func resolveModel(resource CognitiveResourceConfig, profile CognitiveProfile) (CognitiveModelConfig, error) {
	if err := validateProfile(resource, profile); err != nil {
		return CognitiveModelConfig{}, err
	}
	return resource.Models[profile.Model], nil
}

func tokenCost(tokens int, pricePerMillionMicrousd int64) int64 {
	if tokens <= 0 || pricePerMillionMicrousd <= 0 {
		return 0
	}
	numerator := int64(tokens) * pricePerMillionMicrousd
	return (numerator + tokensPerMillion - 1) / tokensPerMillion
}

func usageCost(model CognitiveModelConfig, usage apiUsage) int64 {
	cached := usage.InputTokensDetails.CachedTokens
	if cached < 0 {
		cached = 0
	}
	if cached > usage.InputTokens {
		cached = usage.InputTokens
	}
	uncached := usage.InputTokens - cached
	return tokenCost(uncached, model.InputPerMillionMicrousd) +
		tokenCost(cached, model.CachedInputPerMillionMicrousd) +
		tokenCost(usage.OutputTokens, model.OutputPerMillionMicrousd)
}

func reservationCost(model CognitiveModelConfig, inputTokenUpperBound, maxOutputTokens int) int64 {
	return tokenCost(inputTokenUpperBound, model.InputPerMillionMicrousd) +
		tokenCost(maxOutputTokens, model.OutputPerMillionMicrousd)
}

func spendInWindow(records []UsageRecord, window time.Duration, now time.Time) int64 {
	cutoff := now.Add(-window)
	var total int64
	for _, record := range records {
		at, err := time.Parse(time.RFC3339Nano, record.Time)
		if err == nil && !at.Before(cutoff) {
			total += record.ActualMicrousd
		}
	}
	return total
}

func mergeUsageRecords(groups ...[]UsageRecord) []UsageRecord {
	latest := make(map[string]UsageRecord)
	for _, records := range groups {
		for _, record := range records {
			if record.LeaseID == "" {
				continue
			}
			latest[usageKey(record)] = record
		}
	}
	merged := make([]UsageRecord, 0, len(latest))
	for _, record := range latest {
		merged = append(merged, record)
	}
	sort.Slice(merged, func(left, right int) bool { return merged[left].Time < merged[right].Time })
	return merged
}

func resourceSpend(state State, config CognitiveResourceConfig, now time.Time) (hourSpent, daySpent, inflight int64) {
	hourSpent = spendInWindow(state.Usage, resourceHourWindow, now)
	daySpent = spendInWindow(state.Usage, resourceDayWindow, now)
	if len(state.ModelReservations) > 0 {
		for _, call := range state.ModelReservations {
			inflight += call.Reservation.ReservedMicrousd
		}
	} else if state.Lease != nil {
		inflight = state.Lease.ReservedMicrousd
	}
	return
}

func canReserve(state State, config CognitiveResourceConfig, amount int64, now time.Time) bool {
	hourSpent, daySpent, inflight := resourceSpend(state, config, now)
	return hourSpent+inflight+amount <= config.RollingHourLimitMicrousd &&
		daySpent+inflight+amount <= config.RollingDayLimitMicrousd
}

func resourceBand(hourRemaining, hourLimit, dayRemaining, dayLimit int64) string {
	if hourLimit <= 0 || dayLimit <= 0 {
		return ""
	}
	hourPercent := 100 * maxInt64(hourRemaining, 0) / hourLimit
	dayPercent := 100 * maxInt64(dayRemaining, 0) / dayLimit
	percent := minInt64(hourPercent, dayPercent)
	switch {
	case percent >= 75:
		return "open"
	case percent >= 50:
		return "comfortable"
	case percent >= 25:
		return "limited"
	case percent >= 10:
		return "scarce"
	default:
		return "critical"
	}
}

func updateResourceSnapshot(snapshot *BodySnapshot, state State, config CognitiveResourceConfig, now time.Time) {
	hourSpent, daySpent, inflight := resourceSpend(state, config, now)
	snapshot.CognitiveHourSpentMicrousd = hourSpent
	snapshot.CognitiveHourRemainingMicrousd = maxInt64(config.RollingHourLimitMicrousd-hourSpent-inflight, 0)
	snapshot.CognitiveDaySpentMicrousd = daySpent
	snapshot.CognitiveDayRemainingMicrousd = maxInt64(config.RollingDayLimitMicrousd-daySpent-inflight, 0)
	snapshot.CognitiveResourceBand = resourceBand(
		maxInt64(config.RollingHourLimitMicrousd-hourSpent, 0),
		config.RollingHourLimitMicrousd,
		maxInt64(config.RollingDayLimitMicrousd-daySpent, 0),
		config.RollingDayLimitMicrousd,
	)
	snapshot.CognitivePriceTableVersion = config.PriceTableVersion
}

func resourceBandRank(band string) int {
	switch band {
	case "open":
		return 4
	case "comfortable":
		return 3
	case "limited":
		return 2
	case "scarce":
		return 1
	case "critical":
		return 0
	default:
		return -1
	}
}

// resourceAwareAttentionSeconds changes only timers for cognition that has no
// new external fact. Fresh Reality, mentor input and action results stay
// immediately eligible. Stage 20 has an explicit rolling money budget, so a
// rapid idle/revisit cadence must slow before its own reflections consume the
// remaining ability to act.
func (r *Runtime) resourceAwareAttentionSeconds(configured int) int {
	if configured <= 0 {
		configured = 10
	}
	if r.state.Stage < 20 {
		return configured
	}
	floor := 0
	switch r.state.Body.CognitiveResourceBand {
	case "limited":
		floor = 30
	case "scarce":
		floor = 60
	case "critical":
		floor = 120
	}
	return maxInt(configured, floor)
}

func (r *Runtime) resourceBandChangeNeedsAttention(previous, current string, now time.Time) bool {
	if r.state.Stage < 20 {
		return true
	}
	// A recovery is already present in every later body view, while a focus that
	// was actually blocked on affordability is released by
	// releaseCognitiveResourceWaits. It does not need an independent paid focus.
	if resourceBandRank(current) >= resourceBandRank(previous) {
		return false
	}
	key := differenceFamilyKey(Event{Kind: "cognitive_resource_change", Source: "interoception"})
	trace, exists := r.state.DifferenceField[key]
	if !exists || trace.LastIgnitedAt == "" {
		return true
	}
	last, err := time.Parse(time.RFC3339Nano, trace.LastIgnitedAt)
	return err != nil || now.Sub(last) >= stageTwentyResourceAttentionCooldown
}

// Spend is a durable resource change; a reservation only temporarily occupies
// funds. Every writer refreshes through this owner so settling a request cannot
// erase the baseline before the next sensor compares it.
func (r *Runtime) refreshResourceBody(now time.Time) error {
	previous := r.state.Body.CognitiveResourceBand
	updateResourceSnapshot(&r.state.Body, r.state, r.config.CognitiveResource, now)
	current := r.state.Body.CognitiveResourceBand
	if previous == "" || previous == current {
		return nil
	}
	payload, _ := json.Marshal(map[string]any{"previous_band": previous, "current_band": current,
		"hour_spent_microusd": r.state.Body.CognitiveHourSpentMicrousd, "day_spent_microusd": r.state.Body.CognitiveDaySpentMicrousd})
	return r.addEvent("cognitive_resource_change", "interoception", "已结算认知资源档位由 "+previous+" 变为 "+current+"；可支配余额另计在途预留。", "", payload,
		r.state.Stage >= 4 && r.resourceBandChangeNeedsAttention(previous, current, now))
}

func activeProfile(state State, config CognitiveResourceConfig, focusID string) CognitiveProfile {
	profile, _, _ := activeProfileDecision(state, config, focusID)
	return profile
}

func activeProfileDecision(state State, config CognitiveResourceConfig, focusID string) (CognitiveProfile, string, string) {
	if state.CognitiveResource.NextProfile != nil && state.CognitiveResource.NextProfile.FocusID == focusID {
		source := state.CognitiveResource.NextProfile.Source
		if source == "" {
			source = "next"
		}
		return state.CognitiveResource.NextProfile.Profile, source, state.CognitiveResource.NextProfile.Purpose
	}
	// Consuming NextProfile starts a lease, not completion of the request.
	// Recover the same local contract on an infrastructure retry or restart.
	if state.Stage >= 10 {
		for _, event := range state.Background {
			if event.ID == focusID && event.Kind == "cognition_continuation" && eventChainActive(event.Status) {
				var contract assistanceContract
				if json.Unmarshal(event.Payload, &contract) == nil && contract.Task != "" && validateProfile(config, contract.Profile) == nil {
					return contract.Profile, "next", contract.Purpose
				}
			}
		}
	}
	if state.CognitiveResource.DefaultProfile.Model != "" {
		return state.CognitiveResource.DefaultProfile, "default", ""
	}
	return config.InitialDefaultProfile, "initial_default", ""
}

// validationRecoveryProfile selects the next strictly more capable model for
// one focus whose current profile repeatedly failed the executable cognitive
// contract. This is a temporary bodily accommodation, not a new default and
// not a decision about what Alice ought to care about. The next cognition sees
// the source and purpose and may keep or change the profile from lived result.
func (r *Runtime) validationRecoveryProfile(failed CognitiveProfile) (CognitiveProfile, bool) {
	order := []string{"fast", "main", "high"}
	failedRank := -1
	for index, model := range order {
		if model == failed.Model {
			failedRank = index
			break
		}
	}
	if failedRank < 0 {
		return CognitiveProfile{}, false
	}
	for _, model := range order[failedRank+1:] {
		if protected, _ := modelProtected(r.state, model, time.Now().UTC()); protected {
			continue
		}
		for _, effort := range []string{"medium", failed.ReasoningEffort, "low", "high", "none"} {
			profile := CognitiveProfile{Model: model, ReasoningEffort: effort}
			if validateProfile(r.config.CognitiveResource, profile) == nil {
				return profile, true
			}
		}
	}
	return CognitiveProfile{}, false
}

func (r *Runtime) planValidationRecovery(focusID string, failed CognitiveProfile) (bool, error) {
	if r.config.CognitiveResource.DisableValidationFallback {
		return false, nil
	}
	profile, ok := r.validationRecoveryProfile(failed)
	if !ok {
		return false, nil
	}
	purpose := "当前认知档位连续未能形成身体可执行的表达；身体临时提高一次认知能力以保持同一关切的连续性，结果仍由你判断"
	r.state.CognitiveResource.NextProfile = &NextCognitiveProfile{
		FocusID: focusID, Purpose: purpose, Profile: profile, Source: "validation_fallback",
	}
	markEvent(&r.state, focusID, "pending")
	return true, r.journal("cognitive_validation_fallback_planned", focusID, map[string]any{
		"focus_id": focusID, "failed_profile": failed, "fallback_profile": profile,
	})
}

func validationAttemptLimit(config CognitiveResourceConfig) int {
	return config.ValidationRetryPerFocus + len(config.Models)
}

func cognitionValidationExhausted(concern Concern, config CognitiveResourceConfig) bool {
	return concern.CognitionAttempts >= validationAttemptLimit(config)
}

// exhaustCognition ends only the rapid mechanical retry path. It deliberately
// leaves the Concern's meaning, ownership and resolution untouched; a later
// reality event can make the object relevant through a new causal entrance.
func (r *Runtime) exhaustCognition(focusID string) error {
	limit := validationAttemptLimit(r.config.CognitiveResource)
	for index := range r.state.Background {
		if r.state.Background[index].ID == focusID {
			r.state.Background[index].Status = "failed"
			r.state.Background[index].CognitionAttempts = limit
			r.state.Background[index].LastFocusedAt = nowUTC()
		}
	}
	for index := range r.state.Concerns {
		if r.state.Concerns[index].ID == focusID {
			r.state.Concerns[index].CognitionAttempts = limit
			r.state.Concerns[index].LastFocusedAt = nowUTC()
		}
	}
	return r.journal("cognitive_validation_exhausted", focusID, map[string]any{
		"focus_id": focusID, "attempt_limit": limit,
	})
}

func (r *Runtime) affordableResourceFallback(reservation ModelReservation, now time.Time) (CognitiveProfile, int64, bool) {
	var selected CognitiveProfile
	var selectedCost int64
	for modelName, model := range r.config.CognitiveResource.Models {
		if protected, _ := modelProtected(r.state, modelName, now); protected {
			continue
		}
		effort := ""
		for _, candidate := range []string{"none", "low", "medium"} {
			profile := CognitiveProfile{Model: modelName, ReasoningEffort: candidate}
			if validateProfile(r.config.CognitiveResource, profile) == nil {
				effort = candidate
				break
			}
		}
		if effort == "" {
			continue
		}
		cost := reservationCost(model, reservation.InputTokenUpperBound, r.config.ModelGateway.MaxOutputTokens)
		if !canReserve(r.state, r.config.CognitiveResource, cost, now) {
			continue
		}
		if selected.Model == "" || cost < selectedCost {
			selected = CognitiveProfile{Model: modelName, ReasoningEffort: effort}
			selectedCost = cost
		}
	}
	return selected, selectedCost, selected.Model != ""
}

func modelProtected(state State, model string, now time.Time) (bool, time.Time) {
	protected, ok := state.CognitiveResource.ProtectedModels[model]
	if !ok {
		return false, time.Time{}
	}
	until, err := time.Parse(time.RFC3339Nano, protected.Until)
	if err != nil || !now.Before(until) {
		return false, until
	}
	return true, until
}

type gatewayRetryState struct {
	Failures      int       `json:"consecutive_failures"`
	Cause         string    `json:"cause,omitempty"`
	Until         time.Time `json:"retry_at"`
	ProbeInFlight bool      `json:"probe_in_flight"`
}

// The shared physical request ledger is the source of recovery state, including
// after restart. Changing cognitive focus or inference role cannot reset it.
func gatewayRetry(state State, config CognitiveResourceConfig) gatewayRetryState {
	var result gatewayRetryState
	var latest UsageRecord
	var quotaUntil time.Time
	maximum := time.Duration(config.ModelProtectionMinutes) * time.Minute
	if maximum < cognitionRetryDelay {
		maximum = cognitionRetryDelay
	}
	for index := len(state.Usage) - 1; index >= 0; index-- {
		usage := state.Usage[index]
		if modelInfrastructureFailure(usage.FailureCategory) {
			at, err := time.Parse(time.RFC3339Nano, usage.Time)
			if err != nil {
				continue
			}
			if result.Failures == 0 {
				latest = usage
			}
			result.Failures++
			if usage.FailureCategory == "gateway_quota" {
				// A later in-flight transport failure cannot shorten a known
				// exhausted key's recovery interval. One successful response can.
				hold := max(maximum, retryAfterDuration(usage.RetryAfter, usage.Time, at))
				if until := at.Add(hold); until.After(quotaUntil) {
					quotaUntil = until
				}
			}
		} else if usage.FailureCategory == "" && (usage.Status == "completed" || usage.Status == "unusable") {
			// A semantic validation error is not a gateway outage. A cancelled
			// call with unknown billing, however, is not evidence of recovery.
			break
		}
	}
	if result.Failures == 0 {
		return result
	}
	delay := cognitionRetryDelay
	for index := 1; index < result.Failures && delay < maximum; index++ {
		delay *= 2
	}
	observed, _ := time.Parse(time.RFC3339Nano, latest.Time)
	if delay > maximum {
		delay = maximum
	}
	// Anchor Retry-After to the response and honor even a longer server window.
	delay = max(delay, retryAfterDuration(latest.RetryAfter, latest.Time, observed))
	result.Until = observed.Add(delay)
	result.Cause = latest.FailureCategory
	if !quotaUntil.IsZero() {
		result.Cause = "gateway_quota"
		if quotaUntil.After(result.Until) {
			result.Until = quotaUntil
		}
	}
	result.ProbeInFlight = len(state.ModelReservations) > 0
	return result
}

func (gate gatewayRetryState) allows(now time.Time, main bool) bool {
	// Recovery gives the next ordinary main-brain request one chance. Local
	// interpretation resumes with healthy service, preserving the shared budget.
	return gate.Failures == 0 || (main && !now.Before(gate.Until) && !gate.ProbeInFlight)
}

func resourceView(request CognitiveRequest, inputTokenEstimate int) map[string]any {
	now := time.Now().UTC()
	hourSpent, daySpent, inflight := resourceSpend(request.State, request.Config.CognitiveResource, now)
	mainProfile := request.State.CognitiveResource.DefaultProfile
	if mainProfile.Model == "" {
		mainProfile = request.Config.CognitiveResource.InitialDefaultProfile
	}
	main := cognitiveProfileResourceView(request.Config, mainProfile, inputTokenEstimate)
	main["role"] = "main"
	main["use"] = "绝大多数感知、意义判断、关切、生活决策与现实结果吸收"
	actionAssistProfile := CognitiveProfile{Model: "high", ReasoningEffort: "low"}
	actionAssist := cognitiveProfileResourceView(request.Config, actionAssistProfile, inputTokenEstimate)
	actionAssist["role"] = "action_assistance"
	actionAssist["use"] = "主脑对复杂逻辑、代码、命令或工具实现把握不足时的一次性协助；结果交还主脑采用"
	actionAssist["context"] = "默认仅问题和必要材料；implementation 增加操作契约；include_self:true 时增加当前叙事参考"
	local := cognitiveProfileResourceView(request.Config, CognitiveProfile{Model: "fast", ReasoningEffort: "none"}, 600)
	local["use"] = "极简单的局部逻辑判断；next/fast/none、task:reasoning，短问题与必要材料，最多200输出token"
	local["context"] = "局部材料，无自动自我叙事或个人回忆"
	view := map[string]any{
		"current_profile": map[string]any{
			"profile": request.Profile,
			"source":  request.Lease.ProfileSource,
			"purpose": request.Lease.ProfilePurpose,
		},
		"default_profile": request.State.CognitiveResource.DefaultProfile,
		"next_profile":    request.State.CognitiveResource.NextProfile,
		"hour": map[string]any{
			"spent_microusd":     hourSpent,
			"limit_microusd":     request.Config.CognitiveResource.RollingHourLimitMicrousd,
			"remaining_microusd": maxInt64(request.Config.CognitiveResource.RollingHourLimitMicrousd-hourSpent-inflight, 0),
		},
		"day": map[string]any{
			"spent_microusd":     daySpent,
			"limit_microusd":     request.Config.CognitiveResource.RollingDayLimitMicrousd,
			"remaining_microusd": maxInt64(request.Config.CognitiveResource.RollingDayLimitMicrousd-daySpent-inflight, 0),
		},
		"roles": map[string]any{
			"main":              main,
			"action_assistance": actionAssist,
			"local_reasoning":   local,
			"organ_instinct": map[string]any{
				"profile":               CognitiveProfile{Model: "fast", ReasoningEffort: "none"},
				"use":                   "器官按需解释局部含糊材料，共用身体额度，由器官自动请求",
				"model_choice_required": false,
			},
			"body_reflex": map[string]any{
				"implementation":        "deterministic_kernel",
				"use":                   "脉搏、计费、重试、排序、状态保存与其他可确定的机械工作",
				"model_choice_required": false,
			},
		},
		"reserved_microusd": inflight,
		"last_spend":        request.State.CognitiveResource.LastSpend,
		"protected_models":  request.State.CognitiveResource.ProtectedModels,
		"gateway_retry":     gatewayRetry(request.State, request.Config.CognitiveResource),
	}
	return view
}

func cognitiveProfileResourceView(config Config, profile CognitiveProfile, inputTokenEstimate int) map[string]any {
	model := config.CognitiveResource.Models[profile.Model]
	return map[string]any{
		"profile":                      profile,
		"model_id":                     model.ID,
		"estimated_upper_microusd":     reservationCost(model, inputTokenEstimate, config.ModelGateway.MaxOutputTokens),
		"input_usd_per_million":        float64(model.InputPerMillionMicrousd) / float64(microusdPerUSD),
		"cached_input_usd_per_million": float64(model.CachedInputPerMillionMicrousd) / float64(microusdPerUSD),
		"output_usd_per_million":       float64(model.OutputPerMillionMicrousd) / float64(microusdPerUSD),
	}
}

func maxInt64(left, right int64) int64 {
	if left > right {
		return left
	}
	return right
}

func minInt64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}
