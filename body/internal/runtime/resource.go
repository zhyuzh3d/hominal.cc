package runtime

import (
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
)

func normalizeResourceConfig(config *Config) error {
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
	for _, name := range []string{"luna", "terra", "sol"} {
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
			latest[record.LeaseID] = record
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
	if state.Lease != nil {
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
	hourSpent, daySpent, _ := resourceSpend(state, config, now)
	snapshot.CognitiveHourSpentMicrousd = hourSpent
	snapshot.CognitiveHourRemainingMicrousd = maxInt64(config.RollingHourLimitMicrousd-hourSpent, 0)
	snapshot.CognitiveDaySpentMicrousd = daySpent
	snapshot.CognitiveDayRemainingMicrousd = maxInt64(config.RollingDayLimitMicrousd-daySpent, 0)
	snapshot.CognitiveResourceBand = resourceBand(
		snapshot.CognitiveHourRemainingMicrousd,
		config.RollingHourLimitMicrousd,
		snapshot.CognitiveDayRemainingMicrousd,
		config.RollingDayLimitMicrousd,
	)
	snapshot.CognitivePriceTableVersion = config.PriceTableVersion
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
	order := []string{"luna", "terra", "sol"}
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
		for _, candidate := range []string{"low", "none", "medium"} {
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

func resourceView(request CognitiveRequest, inputTokenEstimate int) map[string]any {
	now := time.Now().UTC()
	hourSpent, daySpent, inflight := resourceSpend(request.State, request.Config.CognitiveResource, now)
	models := make([]map[string]any, 0, len(request.Config.CognitiveResource.Models))
	keys := make([]string, 0, len(request.Config.CognitiveResource.Models))
	for key := range request.Config.CognitiveResource.Models {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		model := request.Config.CognitiveResource.Models[key]
		models = append(models, map[string]any{
			"name":                         key,
			"model_id":                     model.ID,
			"supported_reasoning_efforts":  model.SupportedReasoningEfforts,
			"estimated_upper_microusd":     reservationCost(model, inputTokenEstimate, request.Config.ModelGateway.MaxOutputTokens),
			"input_usd_per_million":        float64(model.InputPerMillionMicrousd) / float64(microusdPerUSD),
			"cached_input_usd_per_million": float64(model.CachedInputPerMillionMicrousd) / float64(microusdPerUSD),
			"output_usd_per_million":       float64(model.OutputPerMillionMicrousd) / float64(microusdPerUSD),
		})
	}
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
		"models":           models,
		"last_spend":       request.State.CognitiveResource.LastSpend,
		"protected_models": request.State.CognitiveResource.ProtectedModels,
	}
	return view
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
