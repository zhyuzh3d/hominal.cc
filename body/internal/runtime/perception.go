package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"hominal.cc/hominal/body/internal/organ"
)

// Workers own immutable inputs; only the runtime accepts their observations.
type perceptionResult struct {
	ID          string
	OrganID     string
	Operation   string
	Epoch       uint64
	Observation organ.Observation
	Orientation *organ.Orientation
	Error       error
}

func (r *Runtime) startPerception(parent context.Context, id string, orient bool) {
	if r.perceptionPending != "" || r.organs == nil || (orient && r.organHasUnabsorbedReality(id)) {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 20*time.Second)
	job, epoch := "sense-"+randomID(), r.actionEpoch
	r.perceptionPending, r.perceptionCancel, r.perceptionOrients = job, cancel, orient
	r.lastPerceptualScan = time.Now()
	go func() {
		defer cancel()
		result := perceptionResult{ID: job, OrganID: id, Epoch: epoch, Operation: "observe"}
		if orient {
			result.Operation = "orient"
			orientation, err := r.organs.Orient(ctx, id)
			result.Orientation, result.Error = &orientation, err
		}
		if result.Error == nil {
			result.Operation = "observe"
			result.Observation, result.Error = r.organs.Observe(ctx, id)
		}
		select {
		case r.perceptionResults <- result:
		case <-parent.Done():
		}
	}()
}

func (r *Runtime) acceptPerception(ctx context.Context, result perceptionResult) error {
	if result.ID != r.perceptionPending {
		return nil
	}
	oriented := r.perceptionOrients
	r.perceptionPending, r.perceptionCancel, r.perceptionOrients = "", nil, false
	if result.Epoch != r.actionEpoch {
		return r.journal("perception_superseded", result.ID, nil)
	}
	operation := result.Operation
	if operation == "" {
		operation = "observe"
	}
	if result.Orientation != nil && operation == "observe" {
		if err := r.recordOperationOutcome(result.OrganID, "orient", "completed", "", true); err != nil {
			return err
		}
	}
	if result.Error != nil {
		if oriented {
			// A failed movement may already have changed position. Keep learned
			// content, but require a fresh observation to recover its location.
			for key, trace := range r.state.Perception {
				if trace.OrganID == result.OrganID {
					trace.Context = nil
					trace.Pending = nil
					r.state.Perception[key] = trace
				}
			}
		}
		if errors.Is(result.Error, context.Canceled) {
			return r.journal("perception_cancelled", result.ID, nil)
		}
		if err := r.journal("perception_unavailable", result.ID, map[string]any{"organ_id": result.OrganID, "operation": operation, "error": truncate(result.Error.Error(), 400)}); err != nil {
			return err
		}
		return r.recordOperationOutcome(result.OrganID, operation, "failed", result.Error.Error(), true)
	}
	if err := r.recordOperationOutcome(result.OrganID, operation, "completed", "", true); err != nil {
		return err
	}
	observation := observationFromOrgan(result.Observation)
	surface := perceptualSurfaceKey(observation.OrganID, observation.SurfaceID)
	previous := r.state.Perception[surface]
	if previous.ObservedAt != "" && timeAfter(previous.ObservedAt, observation.ObservedAt) {
		return nil
	}
	if r.state.Perception == nil {
		r.state.Perception = make(map[string]PerceptualTrace)
	}
	trace := queuePerceptualNovelty(previous, observation)
	r.state.Perception[surface] = trace
	r.startInstinct(ctx, result.Observation, result.Epoch)
	if result.Orientation != nil {
		if err := r.journal("perceptual_orientation", surface, result.Orientation); err != nil {
			return err
		}
	}
	if len(trace.Pending) > 0 {
		if result.Orientation != nil {
			r.state.Perception[surface] = reopenPerceptualSampling(trace)
		}
		return r.emitPerception(surface)
	}
	if result.Orientation != nil {
		return r.recordPerceptualExhaustion(surface, trace, "one bounded sensory orientation produced no unseen object")
	}
	if r.state.Lease == nil && r.state.PendingAction == nil && !r.hasCommitmentAwaitingAssimilation() &&
		!r.attentionCandidateActive() && perceptualResampleDue(trace, time.Now(), r.perceptualReorientationSeconds()) {
		r.startPerception(ctx, observation.OrganID, true)
	}
	return nil
}

func (r *Runtime) acceptBodySnapshot(snapshot BodySnapshot) error {
	now := time.Now().UTC()
	if err := r.refreshResourceBody(now); err != nil {
		return err
	}
	// Resource balances belong to the core, not to the older sensor snapshot.
	updateResourceSnapshot(&snapshot, r.state, r.config.CognitiveResource, now)
	differences := bodyDifferences(r.state.Body, snapshot, false)
	r.state.Body = snapshot
	if len(differences) > 0 {
		payload, _ := json.Marshal(snapshot)
		if err := r.addEvent("body_delta", "observed", strings.Join(differences, "; "), "", payload, r.config.Stage >= 4); err != nil {
			return err
		}
	}
	return r.persist()
}
