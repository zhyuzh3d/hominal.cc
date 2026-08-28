package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Runtime struct {
	instanceRoot     string
	config           Config
	store            *Store
	cognizer         Cognizer
	state            State
	commands         chan RuntimeCommand
	notices          chan WorkerNotice
	results          chan CognitiveResult
	actionResults    chan ActionResultNotice
	workerCancel     context.CancelFunc
	lastSlowScan     time.Time
	activeCandidates map[string]Event
}

const cognitionRetryDelay = time.Minute

func New(instanceRoot, instanceID string, config Config, cognizer Cognizer) (*Runtime, error) {
	if instanceID == "" {
		return nil, errors.New("instance id is required")
	}
	if config.Stage != 3 && config.Stage != 4 && config.Stage != 5 && config.Stage != 8 {
		return nil, fmt.Errorf("unsupported runtime stage %d", config.Stage)
	}
	if config.GenerationKind == "" {
		config.GenerationKind = "engineering"
	}
	switch config.GenerationKind {
	case "engineering":
	case "rehearsal", "formal":
		if config.Stage != 5 && config.Stage != 8 {
			return nil, errors.New("rehearsal and formal generations require the stage-five or stage-eight cognition core")
		}
		if config.GenerationWindowSeconds <= 0 {
			return nil, errors.New("generation window is required")
		}
		if strings.TrimSpace(config.BirthBrief) == "" {
			return nil, errors.New("birth brief is required")
		}
	default:
		return nil, fmt.Errorf("unsupported generation kind %q", config.GenerationKind)
	}
	if config.Pulse.IntervalSeconds <= 0 {
		config.Pulse.IntervalSeconds = 5
	}
	if config.Pulse.SlowScanSeconds <= 0 {
		config.Pulse.SlowScanSeconds = 60
	}
	if err := normalizeResourceConfig(&config); err != nil {
		return nil, err
	}
	if config.Stage >= 4 {
		if config.Dynamics.AttentionCandidateLimit <= 0 {
			config.Dynamics.AttentionCandidateLimit = defaultAttentionCandidateLimit
		}
		if config.Dynamics.AttentionRevisitSeconds <= 0 {
			config.Dynamics.AttentionRevisitSeconds = defaultAttentionRevisitSeconds
		}
	}
	if config.Stage >= 5 {
		integrityValues := []float64{
			config.Dynamics.IntegrityPersistence,
			config.Dynamics.IntegrityGapGain,
			config.Dynamics.IntegrityRepairGain,
			config.Dynamics.IntegrityMirrorThreshold,
		}
		for _, value := range integrityValues {
			if value < 0 || value > 1 {
				return nil, errors.New("stage-five integrity dynamics must remain within 0..1")
			}
		}
		if config.Dynamics.IntegrityMirrorThreshold == 0 {
			return nil, errors.New("stage-five integrity mirror threshold is required")
		}
	}
	store, err := NewStore(instanceRoot)
	if err != nil {
		return nil, err
	}
	runtime := &Runtime{
		instanceRoot:     instanceRoot,
		config:           config,
		store:            store,
		cognizer:         cognizer,
		commands:         make(chan RuntimeCommand),
		notices:          make(chan WorkerNotice),
		results:          make(chan CognitiveResult, 1),
		actionResults:    make(chan ActionResultNotice, 4),
		activeCandidates: make(map[string]Event),
	}
	loaded, err := store.Load()
	if err != nil {
		return nil, err
	}
	if loaded == nil {
		runtime.state = State{
			Schema:         stateSchema,
			InstanceID:     instanceID,
			Stage:          config.Stage,
			GenerationKind: config.GenerationKind,
			ReadyAt:        nowUTC(),
			Mentor: MentorState{
				Received: make(map[string]uint64),
			},
			CognitiveResource: CognitiveResourceState{
				DefaultProfile:  config.CognitiveResource.InitialDefaultProfile,
				ProtectedModels: make(map[string]ProtectedModel),
			},
		}
		if config.Stage >= 4 {
			runtime.state.AffectiveState.Control = 0.5
			runtime.state.AffectiveState.Certainty = 0.5
			runtime.state.ExplorationPressure = clamp01(0.10 + 0.20*config.Seed.ExplorationBias)
		}
	} else {
		if loaded.InstanceID != instanceID {
			return nil, fmt.Errorf("state belongs to instance %q", loaded.InstanceID)
		}
		if loaded.Stage != config.Stage {
			return nil, fmt.Errorf("state stage %d does not match runtime stage %d", loaded.Stage, config.Stage)
		}
		if loaded.GenerationKind == "" {
			loaded.GenerationKind = "engineering"
		}
		if loaded.GenerationKind != config.GenerationKind {
			return nil, fmt.Errorf("state generation kind %q does not match runtime kind %q", loaded.GenerationKind, config.GenerationKind)
		}
		runtime.state = *loaded
		if runtime.state.Mentor.Received == nil {
			runtime.state.Mentor.Received = make(map[string]uint64)
		}
		if runtime.state.CognitiveResource.DefaultProfile.Model == "" {
			runtime.state.CognitiveResource.DefaultProfile = config.CognitiveResource.InitialDefaultProfile
		}
		if runtime.state.CognitiveResource.ProtectedModels == nil {
			runtime.state.CognitiveResource.ProtectedModels = make(map[string]ProtectedModel)
		}
		if runtime.state.T0 == "" && config.GenerationKind != "engineering" {
			if at, ok, journalErr := store.FirstJournalTime("generation_t0"); journalErr != nil {
				return nil, journalErr
			} else if ok {
				runtime.setGenerationIdentity(at)
			}
		}
	}
	ledgerUsage, err := store.LoadUsage(time.Now().UTC().Add(-resourceDayWindow))
	if err != nil {
		return nil, err
	}
	runtime.state.Usage = mergeUsageRecords(runtime.state.Usage, ledgerUsage)
	if runtime.state.TotalCommitments < uint64(len(runtime.state.Commitments)) {
		runtime.state.TotalCommitments = uint64(len(runtime.state.Commitments))
	}
	if runtime.state.TotalExperiences < uint64(len(runtime.state.Experiences)) {
		runtime.state.TotalExperiences = uint64(len(runtime.state.Experiences))
	}
	return runtime, nil
}

func (r *Runtime) Run(ctx context.Context) error {
	if r.cognizer == nil {
		return errors.New("cognizer is required")
	}
	if err := r.recoverInterrupted(); err != nil {
		return err
	}
	if err := r.initialSnapshot(); err != nil {
		return err
	}
	if err := r.establishGenerationT0(); err != nil {
		return err
	}
	server, err := StartMentorServer(r.commands)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Close(closeCtx)
	}()
	if err := r.journal("ready", "", map[string]any{"pid": os.Getpid(), "stage": r.state.Stage}); err != nil {
		return err
	}
	if err := r.persist(); err != nil {
		return err
	}
	if _, err := r.activateBirthOrientation(); err != nil {
		return err
	}
	r.maybeStartCognition(ctx)

	ticker := time.NewTicker(time.Duration(r.config.Pulse.IntervalSeconds) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			if r.workerCancel != nil {
				r.workerCancel()
			}
			_ = r.journal("stopped", "", map[string]any{"reason": ctx.Err().Error()})
			return r.persist()
		case command := <-r.commands:
			if err := r.handleCommand(ctx, command); err != nil {
				return err
			}
		case notice := <-r.notices:
			if err := r.handleNotice(notice); err != nil {
				notice.Ack <- NoticeAck{Accepted: false}
				return err
			}
		case result := <-r.results:
			if err := r.handleCognitiveResult(ctx, result); err != nil {
				return err
			}
		case result := <-r.actionResults:
			if err := r.handleStage4ActionResult(ctx, result); err != nil {
				return err
			}
		case <-ticker.C:
			if err := r.pulse(ctx); err != nil {
				return err
			}
		}
	}
}

func (r *Runtime) recoverInterrupted() error {
	hadStartedAction := r.state.PendingAction != nil && r.state.PendingAction.Status == "started"
	if r.state.Lease != nil {
		oldLease := *r.state.Lease
		if oldLease.ReservedMicrousd > 0 {
			model := r.config.CognitiveResource.Models[oldLease.Profile.Model]
			usage := UsageRecord{
				Time:             nowUTC(),
				LeaseID:          oldLease.ID,
				AttentionPulseID: oldLease.PulseID,
				FocusID:          oldLease.FocusID,
				RequestedModel:   oldLease.Profile.Model,
				EffectiveModel:   model.ID,
				ReasoningEffort:  oldLease.Profile.ReasoningEffort,
				ProfileSource:    oldLease.ProfileSource,
				ProfilePurpose:   oldLease.ProfilePurpose,
				ReservedMicrousd: oldLease.ReservedMicrousd,
				ActualMicrousd:   oldLease.ReservedMicrousd,
				Status:           "interrupted_unknown",
			}
			if err := r.store.AppendUsage(usage); err != nil {
				return err
			}
			r.state.Usage = append(r.state.Usage, usage)
			r.state.CognitiveResource.LastSpend = &usage
			if err := r.journal("cognition_spend", oldLease.ID, usage); err != nil {
				return err
			}
		}
		if err := r.journal("cognition_interrupted", oldLease.ID, map[string]any{"focus_id": oldLease.FocusID}); err != nil {
			return err
		}
		if hadStartedAction {
			markEvent(&r.state, oldLease.FocusID, "interrupted")
		} else {
			markEvent(&r.state, oldLease.FocusID, "pending")
		}
		r.state.Lease = nil
		r.state.CurrentFocus = ""
		r.state.Revision++
	}
	if hadStartedAction {
		interrupted := *r.state.PendingAction
		interrupted.Status = "unknown"
		interrupted.EndedAt = nowUTC()
		if commitment := r.commitmentByID(interrupted.CommitmentID); commitment != nil {
			commitment.Status = "reality_unknown"
		}
		if err := r.journal("action_unknown", interrupted.ID, map[string]any{"kind": interrupted.Kind, "request": interrupted.Request}); err != nil {
			return err
		}
		payload, _ := json.Marshal(interrupted)
		if err := r.addEvent(
			"action_result",
			"recovered",
			"一项外部行动在完成确认前中断；先观察现实，再形成新的决定。",
			interrupted.ID,
			payload,
			r.config.Stage >= 4,
		); err != nil {
			return err
		}
		r.state.PendingAction = nil
	}
	return r.persist()
}

func (r *Runtime) initialSnapshot() error {
	if r.config.Stage >= 5 {
		if err := r.syncSelfFromFiles(); err != nil {
			return err
		}
		if err := r.store.SaveSelf(r.state.Self); err != nil {
			return err
		}
	}
	current := collectSnapshot(r.config, r.state, true)
	initial := r.state.Body.ObservedAt == ""
	differences := bodyDifferences(r.state.Body, current, initial)
	r.state.Body = current
	r.lastSlowScan = time.Now()
	if len(differences) > 0 {
		payload, _ := json.Marshal(current)
		return r.addEvent("body_delta", "observed", strings.Join(differences, "; "), "", payload, r.config.Stage >= 4)
	}
	return nil
}

func (r *Runtime) activateBirthOrientation() (bool, error) {
	if r.config.GenerationKind == "engineering" || r.state.BirthBriefEnteredAt != "" {
		return true, nil
	}
	sealed := filepath.Join(r.instanceRoot, "birth", "sealed")
	if _, err := os.Stat(sealed); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	payload, _ := json.Marshal(map[string]any{
		"generation_kind": r.config.GenerationKind,
		"birth_manifest":  filepath.Join(r.instanceRoot, "birth", "birth.yaml"),
	})
	if err := r.addEvent("birth_orientation", "birth", strings.TrimSpace(r.config.BirthBrief), "", payload, true); err != nil {
		return false, err
	}
	r.state.BirthBriefEnteredAt = nowUTC()
	if err := r.journal("birth_activated", r.state.SampleID, map[string]any{"t0": r.state.T0}); err != nil {
		return false, err
	}
	return true, r.persist()
}

func (r *Runtime) pulse(ctx context.Context) error {
	r.state.PulseID++
	r.state.LastPulseAt = nowUTC()
	r.pruneUsage()
	slow := r.lastSlowScan.IsZero() || time.Since(r.lastSlowScan) >= time.Duration(r.config.Pulse.SlowScanSeconds)*time.Second
	current := collectSnapshot(r.config, r.state, slow)
	if !slow {
		current = mergeFastSnapshot(r.state.Body, current)
	} else {
		r.lastSlowScan = time.Now()
		if r.config.Stage >= 5 {
			if err := r.syncSelfFromFiles(); err != nil {
				return err
			}
		}
	}
	differences := bodyDifferences(r.state.Body, current, false)
	r.state.Body = current
	if len(differences) > 0 {
		payload, _ := json.Marshal(current)
		if err := r.addEvent("body_delta", "observed", strings.Join(differences, "; "), "", payload, r.config.Stage >= 4); err != nil {
			return err
		}
	}
	pendingPerception := len(r.state.Perception[browserPerceptionSurface].Pending) > 0
	if (slow || pendingPerception) && r.state.Stage >= 8 &&
		r.state.ExplorationPressure >= r.config.Dynamics.AttentionThreshold &&
		!r.attentionCandidateActive() && !r.explorationCandidateActive() &&
		r.state.Body.ChromeAvailable && r.state.Body.PlaywrightReady {
		if pendingPerception {
			if err := r.emitBrowserPerception(); err != nil {
				return err
			}
		} else if err := r.observeBrowserPerception(); err != nil {
			return err
		}
	}
	if r.config.Stage >= 4 {
		if err := r.advanceDynamics(time.Duration(r.config.Pulse.IntervalSeconds) * time.Second); err != nil {
			return err
		}
	}
	r.releaseRetryableEvents()
	if err := r.releaseCognitiveResourceWaits(); err != nil {
		return err
	}
	if _, err := r.activateBirthOrientation(); err != nil {
		return err
	}
	if err := r.persist(); err != nil {
		return err
	}
	if r.config.Engineering {
		crashRequest := filepath.Join(r.instanceRoot, "state", "crash-request")
		if _, err := os.Stat(crashRequest); err == nil {
			_ = os.Remove(crashRequest)
			_ = r.journal("test_crash", "", nil)
			os.Exit(42)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	r.maybeStartCognition(ctx)
	return nil
}

func (r *Runtime) observeBrowserPerception() error {
	observation, err := collectBrowserPerception(r.instanceRoot)
	if err != nil {
		// Perception is an available bodily route, not a reason to stop life when
		// one scan cannot return content. The ordinary body probes will expose a
		// persistent browser or Playwright availability change.
		return nil
	}
	trace := queuePerceptualNovelty(r.state.Perception[observation.Surface], observation)
	// A nested surface that has already exhausted its realised yield has lost
	// the right to keep reopening itself merely because an endless feed can
	// supply another unseen fragment.  The object-level judgement remains
	// Alice's; the body converts the accumulated scene-level result into an
	// immediate retreat to the prior concrete surface.  Root surfaces have no
	// return path and retain the slower resampling behaviour below.
	if perceptualReturnDue(trace) {
		trace = discardPendingPerception(trace)
		returned, renewErr := renewBrowserPerception(r.instanceRoot, trace.ReturnPath)
		if renewErr == nil && returned {
			if err := r.journal("perceptual_orientation", observation.Surface, map[string]any{
				"surface": observation.Surface,
				"motion":  "return to prior surface after realised yield exhaustion",
			}); err != nil {
				return err
			}
			if renewed, err := collectBrowserPerception(r.instanceRoot); err == nil {
				trace = queuePerceptualNovelty(trace, renewed)
				observation = renewed
			}
		}
	}
	if len(trace.Pending) == 0 {
		// A stable visual field activates one small orienting movement before the
		// next comparison. This is the digital analogue of shifting gaze: it only
		// changes the local viewport and cannot publish, follow, message or choose
		// a semantic object for Alice.
		if err := orientBrowserPerception(r.instanceRoot); err != nil {
			return nil
		}
		if err := r.journal("perceptual_orientation", observation.Surface, map[string]any{
			"surface": observation.Surface,
			"motion":  "one bounded viewport forward",
		}); err != nil {
			return err
		}
		observation, err = collectBrowserPerception(r.instanceRoot)
		if err != nil {
			return nil
		}
		trace = queuePerceptualNovelty(trace, observation)
	}
	if r.state.Perception == nil {
		r.state.Perception = make(map[string]PerceptualTrace)
	}
	r.state.Perception[observation.Surface] = trace
	if len(trace.Pending) == 0 {
		now := time.Now().UTC()
		if !perceptualResampleDue(trace, now, r.perceptualReorientationSeconds()) {
			return nil
		}
		contextKey := perceptualContextKey(trace.Context)
		if trace.ExhaustedContext == contextKey {
			// A persistent drive first lets its sensory organ renew the same
			// external surface. This can reveal genuinely new content without
			// making the model invent an object or letting the kernel choose a new
			// site, topic or social action. Stable, already-seen content remains
			// suppressed by the same semantic novelty gate.
			returned, renewErr := renewBrowserPerception(r.instanceRoot, trace.ReturnPath)
			if renewErr == nil {
				motion := "renew current surface after quiet exhaustion"
				if returned {
					motion = "return to prior surface after quiet exhaustion"
				}
				if err := r.journal("perceptual_orientation", observation.Surface, map[string]any{
					"surface": observation.Surface,
					"motion":  motion,
				}); err != nil {
					return err
				}
				if renewed, err := collectBrowserPerception(r.instanceRoot); err == nil {
					trace = queuePerceptualNovelty(trace, renewed)
					if len(trace.Pending) > 0 {
						trace = reopenPerceptualSampling(trace)
					}
					r.state.Perception[renewed.Surface] = trace
					if len(trace.Pending) > 0 {
						return r.emitBrowserPerception()
					}
				}
			}
		}
		return r.recordBrowserPerceptualExhaustion(trace, "no unseen object remained after bounded sensory orientation")
	}
	return r.emitBrowserPerception()
}

func (r *Runtime) emitBrowserPerception() error {
	const surface = browserPerceptionSurface
	trace, exists := r.state.Perception[surface]
	if !exists {
		return nil
	}
	if trace.Saturation >= r.config.Dynamics.AttentionThreshold {
		contextKey := perceptualContextKey(trace.Context)
		if trace.ExhaustedContext == contextKey &&
			perceptualSaturationDue(trace, r.config.Dynamics.AttentionThreshold, time.Now().UTC(), r.perceptualReorientationSeconds()) {
			// The quiet interval restores one concrete sample, not another thought
			// about the lack of a sample.  Alice still appraises the real object;
			// the body only decides that this sensory surface may be sampled again.
			trace = reopenPerceptualSampling(trace)
			r.state.Perception[surface] = trace
			return r.emitBrowserPerception()
		}
		if trace.ExhaustedContext != contextKey {
			return r.recordBrowserPerceptualExhaustion(trace, "recent concrete objects produced low realised perceptual yield")
		}
		trace = discardPendingPerception(trace)
		r.state.Perception[surface] = trace
		return nil
	}
	trace, novelContent := takePerceptualNovelty(trace)
	r.state.Perception[surface] = trace
	if novelContent == "" {
		return nil
	}
	observedAt := nowUTC()
	payload, _ := json.Marshal(map[string]any{
		"surface":     surface,
		"digest":      trace.Digest,
		"observed_at": observedAt,
		"content":     truncate(novelContent, perceptualContentLimit),
	})
	return r.addEvent(
		"perceptual_change",
		"observed",
		"Chrome 当前页面呈现了一个此前尚未进入感知的新对象。",
		perceptualObjectDigest(novelContent),
		payload,
		true,
	)
}

func (r *Runtime) recordBrowserPerceptualExhaustion(trace PerceptualTrace, reason string) error {
	trace = discardPendingPerception(trace)
	trace.ExhaustedContext = perceptualContextKey(trace.Context)
	trace.ExhaustedAt = nowUTC()
	if r.state.Perception == nil {
		r.state.Perception = make(map[string]PerceptualTrace)
	}
	r.state.Perception[browserPerceptionSurface] = trace
	payload, _ := json.Marshal(map[string]any{
		"surface":     browserPerceptionSurface,
		"digest":      trace.Digest,
		"observed_at": trace.ObservedAt,
		"context":     trace.Context,
		"saturation":  trace.Saturation,
		"reason":      reason,
	})
	// Exhaustion is a sensory control fact. It never enters the attention
	// candidate set: absence of a new referent cannot become the referent of a
	// paid thought, Concern or action. Exploration pressure remains alive while
	// the organ habituates and later resamples reality.
	return r.journal("perceptual_exhaustion", trace.ExhaustedContext, payload)
}

func (r *Runtime) perceptualReorientationSeconds() int {
	seconds := r.config.Pulse.SlowScanSeconds * 5
	if seconds < defaultAttentionRevisitSeconds {
		return defaultAttentionRevisitSeconds
	}
	return seconds
}

func (r *Runtime) handleCommand(ctx context.Context, command RuntimeCommand) error {
	switch command.Kind {
	case "mentor_receive":
		if seq, exists := r.state.Mentor.Received[command.Mentor.MessageID]; exists {
			command.Reply <- CommandReply{Status: 200, Body: map[string]any{"status": "duplicate", "seq": seq}}
			return nil
		}
		repliedConcernID := ""
		repliedCommitmentID := ""
		if command.Mentor.ReplyTo != "" {
			for index := range r.state.Mentor.Outbox {
				message := &r.state.Mentor.Outbox[index]
				if message.MessageID != command.Mentor.ReplyTo {
					continue
				}
				message.RepliedAt = nowUTC()
				if commitment := r.commitmentByID(message.CommitmentID); commitment != nil {
					repliedConcernID = commitment.ConcernID
					repliedCommitmentID = commitment.ID
				}
			}
		}
		payload, _ := json.Marshal(struct {
			MentorInput
			CommitmentID string `json:"commitment_id,omitempty"`
		}{MentorInput: command.Mentor, CommitmentID: repliedCommitmentID})
		if err := r.addEvent("mentor_received", "observed", command.Mentor.Body, command.Mentor.MessageID, payload, true, repliedConcernID); err != nil {
			return err
		}
		r.state.Mentor.Received[command.Mentor.MessageID] = r.state.EventSeq
		if err := r.persist(); err != nil {
			return err
		}
		command.Reply <- CommandReply{Status: 202, Body: map[string]any{"status": "queued", "seq": r.state.EventSeq}}
		r.maybeStartCognition(ctx)
		return nil
	case "environment_receive":
		if r.state.Stage < 5 {
			command.Reply <- CommandReply{Status: 409, Body: map[string]string{"error": "environment events become available in stage five"}}
			return nil
		}
		payload := command.Environment.Payload
		if len(payload) == 0 {
			payload = json.RawMessage("{}")
		}
		if err := r.addEvent("environment_change", "observed", command.Environment.Summary, command.Environment.EventID, payload, true); err != nil {
			return err
		}
		if err := r.persist(); err != nil {
			return err
		}
		command.Reply <- CommandReply{Status: 202, Body: map[string]any{"status": "observed", "seq": r.state.EventSeq}}
		r.maybeStartCognition(ctx)
		return nil
	case "mentor_outbox":
		messages := append([]MentorMessage(nil), r.state.Mentor.Outbox...)
		command.Reply <- CommandReply{Status: 200, Body: map[string]any{"messages": messages}}
		return nil
	case "mentor_ack":
		for index := range r.state.Mentor.Outbox {
			message := &r.state.Mentor.Outbox[index]
			if message.MessageID != command.MessageID {
				continue
			}
			if message.Status != "delivered" {
				message.Status = "delivered"
				message.DeliveredAt = nowUTC()
				if err := r.journal("mentor_delivered", message.MessageID, map[string]any{"reply_to": message.ReplyTo}); err != nil {
					return err
				}
				if err := r.persist(); err != nil {
					return err
				}
			}
			command.Reply <- CommandReply{Status: 200, Body: map[string]string{"status": "delivered"}}
			return nil
		}
		command.Reply <- CommandReply{Status: 404, Body: map[string]string{"error": "message not found"}}
		return nil
	default:
		command.Reply <- CommandReply{Status: 400, Body: map[string]string{"error": "unknown command"}}
		return nil
	}
}

func (r *Runtime) handleNotice(notice WorkerNotice) error {
	if r.state.Lease == nil || r.state.Lease.ID != notice.LeaseID {
		notice.Ack <- NoticeAck{Accepted: false}
		return nil
	}
	ack := NoticeAck{Accepted: true}
	switch notice.Kind {
	case "model_reserve":
		reservation, ok := notice.Payload.(ModelReservation)
		if !ok || reservation.ReservedMicrousd <= 0 || reservation.Profile != r.state.Lease.Profile {
			ack.Accepted = false
			break
		}
		now := time.Now().UTC()
		if protected, until := modelProtected(r.state, reservation.Profile.Model, now); protected {
			ack.Accepted = false
			ack.Output = fmt.Sprintf("model %s is protected until %s", reservation.Profile.Model, until.Format(time.RFC3339Nano))
			break
		}
		if !canReserve(r.state, r.config.CognitiveResource, reservation.ReservedMicrousd, now) {
			validationFallback := r.state.Lease.ProfileSource == "validation_fallback"
			if !validationFallback {
				if fallback, fallbackCost, available := r.affordableResourceFallback(reservation, now); available && fallback != reservation.Profile {
					purpose := "当前首选认知档位超出可用额度；身体临时使用仍可承受的最低成本档位保持一次认知，让你根据真实资源状态继续选择"
					r.state.CognitiveResource.NextProfile = &NextCognitiveProfile{
						FocusID: r.state.Lease.FocusID, Purpose: purpose, Profile: fallback, Source: "resource_fallback",
					}
					if err := r.journal("cognitive_resource_fallback_planned", notice.LeaseID, map[string]any{
						"focus_id": r.state.Lease.FocusID, "preferred_profile": reservation.Profile,
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
					FocusID: r.state.Lease.FocusID, Purpose: r.state.Lease.ProfilePurpose,
					Profile: reservation.Profile, Source: "validation_fallback",
				}
			}
			r.state.CognitiveResource.Limited = &CognitiveResourceLimit{
				FocusID:          r.state.Lease.FocusID,
				Profile:          reservation.Profile,
				RequiredMicrousd: reservation.ReservedMicrousd,
				ObservedAt:       nowUTC(),
			}
			markEvent(&r.state, r.state.Lease.FocusID, "resource_wait")
			if err := r.journal("cognitive_resource_limited", notice.LeaseID, r.state.CognitiveResource.Limited); err != nil {
				return err
			}
			ack.Accepted = false
			ack.Output = fmt.Sprintf("cognitive resource cannot reserve %d microUSD within the rolling hour and day limits", reservation.ReservedMicrousd)
			break
		}
		r.state.Lease.ReservedMicrousd = reservation.ReservedMicrousd
	case "model_usage":
		usage, ok := notice.Payload.(UsageRecord)
		if !ok {
			return errors.New("invalid model usage notice")
		}
		if usage.ReservedMicrousd != r.state.Lease.ReservedMicrousd {
			return errors.New("model usage does not match the active reservation")
		}
		if err := r.store.AppendUsage(usage); err != nil {
			return err
		}
		r.state.Usage = append(r.state.Usage, usage)
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
		r.state.Lease.ReservedMicrousd = 0
		updateResourceSnapshot(&r.state.Body, r.state, r.config.CognitiveResource, time.Now().UTC())
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
	case "action_start":
		request, ok := notice.Payload.(ShellActionRequest)
		if !ok || r.state.PendingAction != nil {
			ack.Accepted = false
			break
		}
		r.state.PendingAction = &ActionState{ID: request.ActionID, LeaseID: notice.LeaseID, Kind: "body_shell", Request: request.Command, Status: "started", StartedAt: nowUTC()}
		if err := r.journal("action_started", request.ActionID, map[string]any{"kind": "body_shell", "command": request.Command, "timeout_seconds": request.TimeoutSeconds}); err != nil {
			return err
		}
	case "action_result":
		result, ok := notice.Payload.(ActionResultNotice)
		if !ok || r.state.PendingAction == nil || r.state.PendingAction.ID != result.ActionID {
			ack.Accepted = false
			break
		}
		r.state.PendingAction.Status = "completed"
		r.state.PendingAction.EndedAt = nowUTC()
		r.state.PendingAction.Result = truncate(result.Result, 64*1024)
		if err := r.journal("action_completed", result.ActionID, map[string]any{"kind": r.state.PendingAction.Kind, "result": r.state.PendingAction.Result}); err != nil {
			return err
		}
	case "mentor_send":
		request, ok := notice.Payload.(MentorActionRequest)
		if !ok || r.state.PendingAction != nil {
			ack.Accepted = false
			break
		}
		messageID := "alice-" + randomID()
		r.state.PendingAction = &ActionState{ID: request.ActionID, LeaseID: notice.LeaseID, Kind: "mentor_send", Request: request.Text, Status: "completed", StartedAt: nowUTC(), EndedAt: nowUTC(), Result: messageID}
		r.state.Mentor.Outbox = append(r.state.Mentor.Outbox, MentorMessage{MessageID: messageID, Body: request.Text, ReplyTo: request.ReplyTo, Status: "queued", QueuedAt: nowUTC()})
		if err := r.journal("mentor_queued", messageID, map[string]any{"body": request.Text, "reply_to": request.ReplyTo, "action_id": request.ActionID}); err != nil {
			return err
		}
		ack.Output = fmt.Sprintf(`{"message_id":%q,"status":"queued"}`, messageID)
	default:
		ack.Accepted = false
	}
	if err := r.persist(); err != nil {
		return err
	}
	notice.Ack <- ack
	return nil
}

func (r *Runtime) handleCognitiveResult(ctx context.Context, result CognitiveResult) error {
	if r.state.Lease == nil || r.state.Lease.ID != result.LeaseID {
		return r.journal("late_cognition_result", result.LeaseID, map[string]any{"focus_id": result.FocusID, "error": errorString(result.Error)})
	}
	var stage4Action *CognitiveAction
	if result.Error != nil {
		var unavailable *CognitiveResourceUnavailableError
		if errors.As(result.Error, &unavailable) {
			fallbackReady := r.state.CognitiveResource.NextProfile != nil &&
				r.state.CognitiveResource.NextProfile.FocusID == r.state.Lease.FocusID &&
				r.state.CognitiveResource.NextProfile.Source == "resource_fallback"
			if fallbackReady {
				markEvent(&r.state, r.state.Lease.FocusID, "pending")
			} else {
				markEvent(&r.state, r.state.Lease.FocusID, "resource_wait")
			}
			if err := r.journal("cognition_deferred", result.LeaseID, map[string]any{"focus_id": result.FocusID, "reason": result.Error.Error()}); err != nil {
				return err
			}
		} else {
			var callFailure *ModelCallError
			isCallFailure := errors.As(result.Error, &callFailure)
			paidUnusable := false
			if !isCallFailure {
				paidUnusable = r.markLeaseUsageUnusable(result.LeaseID)
			}
			attempts := markEventForRetry(&r.state, r.state.Lease.FocusID, result.Error.Error())
			protected := false
			if isCallFailure {
				var err error
				protected, err = r.protectModelAfterFailures(r.state.Lease.Profile.Model)
				if err != nil {
					return err
				}
			}
			if protected {
				markEventModelWait(&r.state, r.state.Lease.FocusID, r.state.Lease.Profile.Model)
			} else if paidUnusable && attempts > r.config.CognitiveResource.ValidationRetryPerFocus && !r.focusIsActionResult(r.state.Lease.FocusID) {
				recovered, err := r.planValidationRecovery(r.state.Lease.FocusID, r.state.Lease.Profile)
				if err != nil {
					return err
				}
				if !recovered {
					if err := r.exhaustCognition(r.state.Lease.FocusID); err != nil {
						return err
					}
				}
			} else if attempts > r.config.CognitiveResource.ValidationRetryPerFocus && !r.focusIsActionResult(r.state.Lease.FocusID) {
				markEvent(&r.state, r.state.Lease.FocusID, "failed")
			}
			failurePayload := map[string]any{"focus_id": result.FocusID, "error": result.Error.Error(), "attempt": attempts}
			if callFailure != nil {
				failurePayload["model_failure"] = callFailure.Fact
			}
			if err := r.journal("cognition_failed", result.LeaseID, failurePayload); err != nil {
				return err
			}
		}
	} else if r.state.Stage >= 4 && result.Stage4 != nil {
		commit, withheldActionKind := normalizeUnendorsedAction(*result.Stage4, r.config.Dynamics.AttentionThreshold)
		if err := r.applyPreparedCognitiveCommit(commit, withheldActionKind); err != nil {
			paidUnusable := r.markLeaseUsageUnusable(result.LeaseID)
			attempts := markEventForRetry(&r.state, r.state.Lease.FocusID, err.Error())
			if paidUnusable && attempts > r.config.CognitiveResource.ValidationRetryPerFocus && !r.focusIsActionResult(r.state.Lease.FocusID) {
				recovered, recoveryErr := r.planValidationRecovery(r.state.Lease.FocusID, r.state.Lease.Profile)
				if recoveryErr != nil {
					return recoveryErr
				}
				if !recovered {
					if recoveryErr := r.exhaustCognition(r.state.Lease.FocusID); recoveryErr != nil {
						return recoveryErr
					}
				}
			} else if attempts > r.config.CognitiveResource.ValidationRetryPerFocus && !r.focusIsActionResult(r.state.Lease.FocusID) {
				markEvent(&r.state, r.state.Lease.FocusID, "failed")
			}
			if journalErr := r.journal("cognition_failed", result.LeaseID, map[string]any{
				"focus_id":                result.FocusID,
				"error":                   err.Error(),
				"action_kind":             result.Stage4.Action.Kind,
				"resource_choice":         result.Stage4.ResourceChoice,
				"experience_update_count": len(result.Stage4.ExperienceUpdates),
			}); journalErr != nil {
				return journalErr
			}
		} else {
			markEvent(&r.state, commit.FocusID, "processed")
			action := commit.Action
			if err := r.formActionCommitment(result.LeaseID, r.state.Lease.Profile, commit, &action); err != nil {
				return err
			}
			stage4Action = &action
		}
	} else {
		markEvent(&r.state, r.state.Lease.FocusID, "processed")
		if err := r.journal("cognition_completed", result.LeaseID, map[string]any{"focus_id": result.FocusID, "text": truncate(result.Text, 16*1024)}); err != nil {
			return err
		}
	}
	r.state.Revision++
	leaseID := r.state.Lease.ID
	r.state.Lease = nil
	if r.state.Stage == 3 {
		r.state.PendingAction = nil
	}
	r.state.CurrentFocus = ""
	r.workerCancel = nil
	r.activeCandidates = make(map[string]Event)
	r.pruneBackground()
	if err := r.persist(); err != nil {
		return err
	}
	if stage4Action != nil {
		if err := r.startStage4Action(ctx, leaseID, *stage4Action); err != nil {
			return err
		}
	}
	r.maybeStartCognition(ctx)
	return nil
}

func (r *Runtime) focusIsActionResult(focusID string) bool {
	for _, event := range r.state.Background {
		if event.ID == focusID {
			return event.Kind == "action_result"
		}
	}
	return false
}

func (r *Runtime) releaseRetryableEvents() {
	now := time.Now().UTC()
	for index := range r.state.Background {
		event := &r.state.Background[index]
		if event.Status != "retry_wait" || event.LastFocusedAt == "" {
			continue
		}
		last, err := time.Parse(time.RFC3339Nano, event.LastFocusedAt)
		if err == nil && now.Sub(last) < cognitionRetryDelay {
			continue
		}
		event.Status = "pending"
	}
}

func (r *Runtime) releaseCognitiveResourceWaits() error {
	now := time.Now().UTC()
	for model, protection := range r.state.CognitiveResource.ProtectedModels {
		until, err := time.Parse(time.RFC3339Nano, protection.Until)
		if err == nil && now.Before(until) {
			continue
		}
		delete(r.state.CognitiveResource.ProtectedModels, model)
		r.releaseModelWaits(model)
		if err := r.journal("cognitive_model_restored", model, map[string]any{"model": model}); err != nil {
			return err
		}
	}
	limited := r.state.CognitiveResource.Limited
	if limited == nil || !canReserve(r.state, r.config.CognitiveResource, limited.RequiredMicrousd, now) {
		return nil
	}
	if protected, _ := modelProtected(r.state, limited.Profile.Model, now); protected {
		return nil
	}
	markEvent(&r.state, limited.FocusID, "pending")
	r.state.CognitiveResource.Limited = nil
	return r.journal("cognitive_resource_restored", limited.FocusID, map[string]any{"focus_id": limited.FocusID})
}

func (r *Runtime) releaseModelWaits(model string) {
	for index := range r.state.Background {
		if r.state.Background[index].Status == "model_wait" && r.state.Background[index].WaitModel == model {
			r.state.Background[index].Status = "pending"
			r.state.Background[index].WaitModel = ""
		}
	}
	for index := range r.state.Concerns {
		if r.state.Concerns[index].WaitModel == model {
			r.state.Concerns[index].WaitModel = ""
		}
	}
}

func (r *Runtime) markLeaseUsageUnusable(leaseID string) bool {
	for index := len(r.state.Usage) - 1; index >= 0; index-- {
		if r.state.Usage[index].LeaseID != leaseID {
			continue
		}
		if r.state.Usage[index].FailureCategory != "" {
			return false
		}
		r.state.Usage[index].Status = "unusable"
		copy := r.state.Usage[index]
		r.state.CognitiveResource.LastSpend = &copy
		return copy.CostConfirmed && copy.ActualMicrousd > 0
	}
	return false
}

func (r *Runtime) protectModelAfterFailures(model string) (bool, error) {
	if model == "" {
		return false, nil
	}
	now := time.Now().UTC()
	cutoff := now.Add(-time.Duration(r.config.CognitiveResource.PaidFailureWindowMinutes) * time.Minute)
	failures := 0
	latest := UsageRecord{}
	for _, usage := range r.state.Usage {
		at, err := time.Parse(time.RFC3339Nano, usage.Time)
		failed := usage.FailureCategory != "" || (usage.Status == "unusable" && usage.CostConfirmed && usage.ActualMicrousd > 0)
		if err == nil && !at.Before(cutoff) && usage.RequestedModel == model && failed {
			failures++
			if latest.Time == "" || usage.Time > latest.Time {
				latest = usage
			}
		}
	}
	if failures < r.config.CognitiveResource.PaidFailureThreshold {
		return false, nil
	}
	if active, _ := modelProtected(r.state, model, now); active {
		return true, nil
	}
	maximum := time.Duration(r.config.CognitiveResource.ModelProtectionMinutes) * time.Minute
	backoff := time.Duration(failures) * 30 * time.Second
	if backoff > maximum {
		backoff = maximum
	}
	if retry := retryAfterDuration(latest.RetryAfter, latest.Time, now); retry > 0 {
		backoff = retry
		if backoff > maximum {
			backoff = maximum
		}
	}
	until := now.Add(backoff)
	reason := "repeated model failures"
	protected := ProtectedModel{Until: until.Format(time.RFC3339Nano), Reason: reason}
	r.state.CognitiveResource.ProtectedModels[model] = protected
	payload, _ := json.Marshal(map[string]any{"model": model, "failures": failures, "until": until.Format(time.RFC3339Nano), "last_failure": latest.FailureCategory})
	if err := r.addEvent("cognitive_resource_change", "observed", "一个认知模型暂时不可用；你仍可以理解资源状态并决定等待或使用其他模型。", model, payload, true); err != nil {
		return false, err
	}
	if err := r.journal("cognitive_model_protected", model, map[string]any{"model": model, "failures": failures, "until": until.Format(time.RFC3339Nano), "last_failure": latest.FailureCategory}); err != nil {
		return false, err
	}
	return true, nil
}

func retryAfterDuration(value, observedAt string, now time.Time) time.Duration {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := time.ParseDuration(value + "s"); err == nil && seconds > 0 {
		return seconds
	}
	if at, err := http.ParseTime(value); err == nil && at.After(now) {
		return at.Sub(now)
	}
	if observed, err := time.Parse(time.RFC3339Nano, observedAt); err == nil {
		_ = observed
	}
	return 0
}

func (r *Runtime) maybeStartCognition(parent context.Context) {
	if r.state.Lease != nil {
		return
	}
	if r.config.GenerationKind != "engineering" && r.state.BirthBriefEnteredAt == "" {
		return
	}
	if r.state.Stage >= 5 && !r.state.Body.NetworkAvailable {
		return
	}
	request, ok := r.nextCognitiveRequest()
	if !ok {
		return
	}
	if !r.cognitiveRequestAllowedAt(request, time.Now().UTC()) {
		return
	}
	if limited := r.state.CognitiveResource.Limited; limited != nil && limited.FocusID == request.Focus.ID {
		return
	}
	profile, profileSource, profilePurpose := activeProfileDecision(r.state, r.config.CognitiveResource, request.Focus.ID)
	requestedModel := profile.Model
	if protected, _ := modelProtected(r.state, requestedModel, time.Now().UTC()); protected {
		protectedState := r.state.CognitiveResource.ProtectedModels[requestedModel]
		if !protectedState.RecoveryOffered {
			if recovery, ok := r.recoveryProfile(requestedModel); ok {
				profile = recovery
				profileSource = "resource_recovery"
				profilePurpose = "理解当前模型不可用的身体事实，并自主选择等待、切换或转向"
				protectedState.RecoveryOffered = true
				r.state.CognitiveResource.ProtectedModels[requestedModel] = protectedState
			} else {
				markEventModelWait(&r.state, request.Focus.ID, requestedModel)
				_ = r.persist()
				return
			}
		} else {
			markEventModelWait(&r.state, request.Focus.ID, requestedModel)
			_ = r.persist()
			return
		}
	}
	r.state.Revision++
	lease := Lease{ID: "lease-" + randomID(), Revision: r.state.Revision, PulseID: r.state.PulseID, FocusID: request.Focus.ID, StartedAt: nowUTC(), Profile: profile, ProfileSource: profileSource, ProfilePurpose: profilePurpose, VariationBias: request.VariationBias, VariationSeed: request.VariationSeed}
	r.state.Lease = &lease
	if r.state.CognitiveResource.NextProfile != nil && r.state.CognitiveResource.NextProfile.FocusID == request.Focus.ID {
		r.state.CognitiveResource.NextProfile = nil
	}
	r.state.CurrentFocus = request.Focus.ID
	markEvent(&r.state, request.Focus.ID, "in_focus")
	_ = r.journal("cognition_started", lease.ID, map[string]any{"focus_id": request.Focus.ID, "candidate_count": len(request.Candidates), "profile": profile, "profile_source": profileSource, "profile_purpose": profilePurpose, "variation_bias": request.VariationBias, "variation_seed": request.VariationSeed})
	_ = r.persist()
	request.Lease = lease
	request.State = cloneState(r.state)
	request.Config = r.config
	request.Profile = profile
	r.activeCandidates = make(map[string]Event, len(request.Candidates))
	for _, candidate := range request.Candidates {
		r.activeCandidates[candidate.ID] = candidate
	}
	ctx, cancel := context.WithCancel(parent)
	r.workerCancel = cancel
	go func() {
		result := r.cognizer.Run(ctx, request, r.notices)
		select {
		case r.results <- result:
		case <-ctx.Done():
		}
	}()
}

// cognitiveRequestAllowedAt keeps a planned generation boundary from opening a
// new cognitive subject while still allowing enacted reality to be assimilated.
// The external Lab owns the bounded drain timeout and stops the runtime after
// the open causal chain settles.
func (r *Runtime) cognitiveRequestAllowedAt(request CognitiveRequest, now time.Time) bool {
	if r.config.GenerationKind == "engineering" || strings.TrimSpace(r.state.PlannedEnd) == "" {
		return true
	}
	plannedEnd, err := time.Parse(time.RFC3339Nano, r.state.PlannedEnd)
	if err != nil || now.Before(plannedEnd) {
		return true
	}
	return request.Focus.Kind == "action_result" ||
		(request.Focus.Kind == "mentor_received" && commitmentIDFromEvent(request.Focus) != "")
}

func (r *Runtime) recoveryProfile(failedModel string) (CognitiveProfile, bool) {
	for _, model := range []string{"luna", "terra", "sol"} {
		if model == failedModel {
			continue
		}
		if protected, _ := modelProtected(r.state, model, time.Now().UTC()); protected {
			continue
		}
		for _, effort := range []string{"low", "medium", "none"} {
			profile := CognitiveProfile{Model: model, ReasoningEffort: effort}
			if validateProfile(r.config.CognitiveResource, profile) == nil {
				return profile, true
			}
		}
	}
	return CognitiveProfile{}, false
}

func (r *Runtime) nextCognitiveRequest() (CognitiveRequest, bool) {
	if r.state.Stage == 3 {
		for _, event := range r.state.Background {
			if event.Status == "pending" && event.Kind == "mentor_received" {
				return CognitiveRequest{Stage: 3, Focus: event, Candidates: []Event{event}}, true
			}
		}
		return CognitiveRequest{}, false
	}
	request, ok := r.nextStage4Request()
	request.Stage = r.state.Stage
	return request, ok
}

func (r *Runtime) addEvent(kind, source, summary, correlationID string, payload json.RawMessage, candidate bool, concernIDs ...string) error {
	r.state.EventSeq++
	event := Event{
		ID:            fmt.Sprintf("event-%012d", r.state.EventSeq),
		Seq:           r.state.EventSeq,
		Kind:          kind,
		Source:        source,
		ObservedAt:    nowUTC(),
		Summary:       summary,
		CorrelationID: correlationID,
		Payload:       payload,
		Status:        "observed",
	}
	if candidate {
		event.Status = "pending"
		if len(concernIDs) > 0 {
			event.ConcernID = concernIDs[0]
		}
		r.state.Background = append(r.state.Background, event)
	}
	record := JournalRecord{Seq: event.Seq, Time: event.ObservedAt, Kind: kind, InstanceID: r.state.InstanceID, Revision: r.state.Revision, CorrelationID: correlationID, Payload: map[string]any{"event_id": event.ID, "source": source, "summary": summary, "payload": json.RawMessage(payload)}}
	return r.store.Append(record)
}

func (r *Runtime) journal(kind, correlationID string, payload any) error {
	r.state.EventSeq++
	return r.store.Append(JournalRecord{Seq: r.state.EventSeq, Time: nowUTC(), Kind: kind, InstanceID: r.state.InstanceID, Revision: r.state.Revision, CorrelationID: correlationID, Payload: payload})
}

func (r *Runtime) establishGenerationT0() error {
	if r.config.GenerationKind == "engineering" || r.state.T0 != "" {
		return nil
	}
	at := time.Now().UTC()
	r.setGenerationIdentity(at)
	return r.journal("generation_t0", r.state.SampleID, map[string]any{
		"t0":          r.state.T0,
		"sample_id":   r.state.SampleID,
		"planned_end": r.state.PlannedEnd,
	})
}

func (r *Runtime) setGenerationIdentity(at time.Time) {
	if r.state.T0 != "" {
		return
	}
	r.state.T0 = at.UTC().Format(time.RFC3339Nano)
	shanghai := at.In(time.FixedZone("Asia/Shanghai", 8*60*60))
	r.state.SampleID = fmt.Sprintf("alice%02d%02d%c", int(shanghai.Month()), shanghai.Day(), rune('a'+shanghai.Hour()))
	r.state.PlannedEnd = at.UTC().Add(time.Duration(r.config.GenerationWindowSeconds) * time.Second).Format(time.RFC3339Nano)
}

func (r *Runtime) persist() error {
	if r.state.LastPulseAt == "" {
		r.state.LastPulseAt = nowUTC()
	}
	if err := r.store.Save(&r.state); err != nil {
		return err
	}
	return r.store.Heartbeat(&r.state)
}

func (r *Runtime) pruneUsage() {
	cutoff := time.Now().UTC().Add(-resourceDayWindow)
	kept := r.state.Usage[:0]
	for _, record := range r.state.Usage {
		at, err := time.Parse(time.RFC3339Nano, record.Time)
		if err == nil && !at.Before(cutoff) {
			kept = append(kept, record)
		}
	}
	r.state.Usage = kept
}

func (r *Runtime) pruneBackground() {
	if len(r.state.Background) <= 128 {
		return
	}
	kept := make([]Event, 0, 128)
	for _, event := range r.state.Background {
		if event.Status == "pending" || event.Status == "in_focus" {
			kept = append(kept, event)
		}
	}
	for index := len(r.state.Background) - 1; index >= 0 && len(kept) < 128; index-- {
		event := r.state.Background[index]
		if event.Status != "pending" && event.Status != "in_focus" {
			kept = append(kept, event)
		}
	}
	r.state.Background = kept
}

func markEvent(state *State, id, status string) {
	for index := range state.Background {
		if state.Background[index].ID == id {
			state.Background[index].Status = status
			state.Background[index].LastFocusedAt = nowUTC()
			if status == "processed" {
				state.Background[index].LastCommitErr = ""
				state.Background[index].CognitionAttempts = 0
			}
			return
		}
	}
	for index := range state.Concerns {
		if state.Concerns[index].ID == id {
			state.Concerns[index].LastFocusedAt = nowUTC()
			if status == "processed" {
				state.Concerns[index].LastCommitErr = ""
				state.Concerns[index].CognitionAttempts = 0
			}
			return
		}
	}
}

func markEventModelWait(state *State, id, model string) {
	for index := range state.Background {
		if state.Background[index].ID == id {
			state.Background[index].Status = "model_wait"
			state.Background[index].WaitModel = model
			state.Background[index].LastFocusedAt = nowUTC()
			return
		}
	}
	for index := range state.Concerns {
		if state.Concerns[index].ID == id {
			state.Concerns[index].WaitModel = model
			state.Concerns[index].LastFocusedAt = nowUTC()
			return
		}
	}
}

func markEventForRetry(state *State, id, commitError string) int {
	for index := range state.Background {
		if state.Background[index].ID == id {
			state.Background[index].Status = "retry_wait"
			state.Background[index].LastFocusedAt = nowUTC()
			state.Background[index].LastCommitErr = truncate(commitError, 1024)
			state.Background[index].CognitionAttempts++
			return state.Background[index].CognitionAttempts
		}
	}
	for index := range state.Concerns {
		if state.Concerns[index].ID == id {
			state.Concerns[index].LastFocusedAt = nowUTC()
			state.Concerns[index].LastCommitErr = truncate(commitError, 1024)
			state.Concerns[index].CognitionAttempts++
			return state.Concerns[index].CognitionAttempts
		}
	}
	return 0
}

func cloneState(state State) State {
	data, _ := json.Marshal(state)
	var clone State
	_ = json.Unmarshal(data, &clone)
	return clone
}

func randomID() string {
	buffer := make([]byte, 8)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func clampSigned(value float64) float64 {
	if value < -1 {
		return -1
	}
	if value > 1 {
		return 1
	}
	return value
}
