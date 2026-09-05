package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

func usageKey(record UsageRecord) string {
	if record.CallID != "" {
		return record.CallID
	}
	return record.LeaseID
}

// All inference roles reserve and settle through the single runtime owner.
// A late physical bill is still accountable after its cognitive lease ended.
func (r *Runtime) handleModelNotice(notice WorkerNotice) error {
	key := notice.CallID
	if key == "" {
		key = notice.LeaseID
	}
	lease := r.state.Lease
	main := lease != nil && lease.ID == notice.LeaseID
	if !main {
		lease = nil
		if local, ok := r.peripheralLeases[notice.LeaseID]; ok {
			copy := local
			lease = &copy
		}
	}
	pending, hasPending := r.state.ModelReservations[key]
	if notice.Kind == "model_usage" && hasPending && pending.Owner.ID == notice.LeaseID {
		if !main {
			copy := pending.Owner
			lease = &copy
		}
		lease.ReservedMicrousd = pending.Reservation.ReservedMicrousd
	}
	if notice.Kind == "model_usage" && !hasPending {
		for _, previous := range r.state.Usage {
			if usageKey(previous) == key {
				notice.Ack <- NoticeAck{Accepted: true}
				return nil
			}
		}
	}
	if lease == nil {
		notice.Ack <- NoticeAck{Accepted: false}
		return nil
	}
	if notice.Kind == "model_reserve" && hasPending {
		notice.Ack <- NoticeAck{Accepted: false, Output: "request already reserved"}
		return nil
	}
	ack := NoticeAck{Accepted: true}
	switch notice.Kind {
	case "model_reserve":
		reservation, ok := notice.Payload.(ModelReservation)
		if !ok || reservation.ReservedMicrousd <= 0 || reservation.Profile != lease.Profile {
			ack.Accepted = false
			break
		}
		now := time.Now().UTC()
		gate := gatewayRetry(r.state, r.config.CognitiveResource)
		if !gate.allows(now, main) {
			ack.Accepted = false
			ack.Output = fmt.Sprintf("shared model gateway recovery (%s): retry at %s, probe in flight: %t", gate.Cause, gate.Until.Format(time.RFC3339Nano), gate.ProbeInFlight)
			ack.Failure = &ModelFailureFact{ObservedAt: nowUTC(), Model: reservation.Profile.Model, Category: "gateway_backoff"}
			break
		}
		if protected, until := modelProtected(r.state, reservation.Profile.Model, now); protected {
			ack.Accepted = false
			ack.Output = fmt.Sprintf("model %s is protected until %s", reservation.Profile.Model, until.Format(time.RFC3339Nano))
			break
		}
		if !canReserve(r.state, r.config.CognitiveResource, reservation.ReservedMicrousd, now) {
			if !main {
				ack.Accepted = false
				ack.Output = "shared body resource is unavailable"
				break
			}
			validationFallback := lease.ProfileSource == "validation_fallback"
			if !validationFallback {
				if fallback, fallbackCost, available := r.affordableResourceFallback(reservation, now); available && fallback != reservation.Profile {
					purpose := "当前首选认知档位超出可用额度；身体临时使用仍可承受的最低成本档位保持一次认知，让你根据真实资源状态继续选择"
					r.state.CognitiveResource.NextProfile = &NextCognitiveProfile{
						FocusID: lease.FocusID, Purpose: purpose, Profile: fallback, Source: "resource_fallback",
					}
					if err := r.journal("cognitive_resource_fallback_planned", notice.LeaseID, map[string]any{
						"focus_id": lease.FocusID, "preferred_profile": reservation.Profile,
						"preferred_required_microusd": reservation.ReservedMicrousd,
						"fallback_profile":            fallback, "fallback_required_microusd": fallbackCost,
					}); err != nil {
						return err
					}
					ack.Accepted = false
					ack.Output = "preferred cognitive profile is beyond the current resource balance; an affordable resource fallback is ready"
					break
				}
			} else {
				// A capability recovery must not silently fall back to the profile
				// that already failed this focus. Preserve it until the rolling
				// resource balance can afford the stronger one.
				r.state.CognitiveResource.NextProfile = &NextCognitiveProfile{
					FocusID: lease.FocusID, Purpose: lease.ProfilePurpose,
					Profile: reservation.Profile, Source: "validation_fallback",
				}
			}
			r.state.CognitiveResource.Limited = &CognitiveResourceLimit{
				FocusID:          lease.FocusID,
				Profile:          reservation.Profile,
				RequiredMicrousd: reservation.ReservedMicrousd,
				ObservedAt:       nowUTC(),
			}
			markEvent(&r.state, lease.FocusID, "resource_wait")
			if err := r.journal("cognitive_resource_limited", notice.LeaseID, r.state.CognitiveResource.Limited); err != nil {
				return err
			}
			ack.Accepted = false
			ack.Output = fmt.Sprintf("cognitive resource cannot reserve %d microUSD within the rolling hour and day limits", reservation.ReservedMicrousd)
			break
		}
		lease.ReservedMicrousd = reservation.ReservedMicrousd
		if r.state.ModelReservations == nil {
			r.state.ModelReservations = map[string]PendingModelCall{}
		}
		r.state.ModelReservations[key] = PendingModelCall{Owner: *lease, Reservation: reservation}
	case "model_usage":
		usage, ok := notice.Payload.(UsageRecord)
		if !ok {
			return errors.New("invalid model usage notice")
		}
		usage.CallID = notice.CallID
		if usage.ReservedMicrousd != lease.ReservedMicrousd {
			return errors.New("model usage does not match the active reservation")
		}
		if !usage.CostConfirmed && (usage.FailureCategory == "billing_unconfirmed" || usage.HTTPStatus == 0 || usage.HTTPStatus >= 500) {
			usage.ActualMicrousd = usage.ReservedMicrousd
		}
		if err := r.store.AppendUsage(usage); err != nil {
			return err
		}
		r.state.Usage = mergeUsageRecords(r.state.Usage, []UsageRecord{usage})
		lastSpend := usage
		r.state.CognitiveResource.LastSpend = &lastSpend
		if usage.FailureCategory != "" {
			failure := ModelFailureFact{
				ObservedAt:  usage.Time,
				Model:       usage.RequestedModel,
				Category:    usage.FailureCategory,
				HTTPStatus:  usage.HTTPStatus,
				RetryAfter:  usage.RetryAfter,
				RequestID:   usage.RequestID,
				GatewayDate: usage.GatewayDate,
				CostStatus:  "unconfirmed",
			}
			r.state.CognitiveResource.LastFailure = &failure
		}
		lease.ReservedMicrousd = 0
		delete(r.state.ModelReservations, key)
		if err := r.journal("cognition_spend", notice.LeaseID, usage); err != nil {
			return err
		}
		model := r.config.CognitiveResource.Models[usage.RequestedModel]
		if usage.EffectiveModel != "" && model.ID != "" && usage.EffectiveModel != model.ID {
			payload, _ := json.Marshal(map[string]any{"requested_model": model.ID, "effective_model": usage.EffectiveModel})
			if err := r.addEvent("body_delta", "observed", "模型服务实际返回了不同于请求的模型。", notice.LeaseID, payload, r.config.Stage >= 4); err != nil {
				return err
			}
		}

	default:
		ack.Accepted = false
	}
	if err := r.refreshResourceBody(time.Now().UTC()); err != nil {
		return err
	}
	if err := r.persist(); err != nil {
		return err
	}
	notice.Ack <- ack
	return nil
}
