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
	"sort"
	"strings"
	"time"

	"hominal.cc/hominal/body/internal/organ"
)

type Runtime struct {
	instanceRoot       string
	config             Config
	store              *Store
	learning           *learningIndex
	peripheralLeases   map[string]Lease
	instinctResults    chan instinctResult
	instinctScenes     map[string]string
	organs             *organ.Registry
	cognizer           Cognizer
	state              State
	commands           chan RuntimeCommand
	notices            chan WorkerNotice
	results            chan CognitiveResult
	actionResults      chan ActionResultNotice
	bodyResults        chan BodySnapshot
	perceptionResults  chan perceptionResult
	bodyScanPending    bool
	perceptionPending  string
	perceptionCancel   context.CancelFunc
	perceptionOrients  bool
	actionEpoch        uint64
	lastDynamicsAt     time.Time
	workerCancel       context.CancelFunc
	lastSlowScan       time.Time
	lastPerceptualScan time.Time
	organCursor        int
	activeCandidates   map[string]Event
}

const (
	cognitionRetryDelay       = 10 * time.Second
	maximumGenerationLifetime = 2 * time.Hour
)

func New(instanceRoot, instanceID string, config Config, cognizer Cognizer) (*Runtime, error) {
	if instanceID == "" {
		return nil, errors.New("instance id is required")
	}
	if config.Stage != 20 && config.Stage != 3 && config.Stage != 4 && config.Stage != 5 && config.Stage != 8 && config.Stage != 9 && config.Stage != 10 {
		return nil, fmt.Errorf("unsupported runtime stage %d", config.Stage)
	}
	if config.Stage == 20 && config.CognitiveCore != "continuous-v1" {
		return nil, errors.New("stage20 requires continuous-v1 cognitive core")
	}
	if config.GenerationKind == "" {
		config.GenerationKind = "engineering"
	}
	switch config.GenerationKind {
	case "engineering":
	case "rehearsal", "formal":
		if config.Stage != 20 && config.Stage != 5 && config.Stage != 8 && config.Stage != 9 && config.Stage != 10 {
			return nil, errors.New("rehearsal and formal generations require the stage-five, stage-eight, stage-nine, or stage-ten cognition core")
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
		if config.Dynamics.DifferenceDecayRate <= 0 {
			config.Dynamics.DifferenceDecayRate = 0.35
		}
		if config.Dynamics.DifferenceLearningRate <= 0 {
			config.Dynamics.DifferenceLearningRate = 0.12
		}
		if config.Dynamics.DifferenceDecayRate > 1 || config.Dynamics.DifferenceLearningRate > 1 {
			return nil, errors.New("difference dynamics must remain within 0..1")
		}
		if config.Dynamics.AttentionCandidateLimit <= 0 {
			config.Dynamics.AttentionCandidateLimit = defaultAttentionCandidateLimit
		}
		if config.Dynamics.AttentionRevisitSeconds <= 0 {
			config.Dynamics.AttentionRevisitSeconds = defaultAttentionRevisitSeconds
		}
		if config.Dynamics.AttentionMaximumIdleSeconds <= 0 {
			config.Dynamics.AttentionMaximumIdleSeconds = 10
		}
		if err := validateLifeValueVector(config.Seed.ValueOrientation, false); err != nil {
			return nil, fmt.Errorf("invalid genesis value orientation: %w", err)
		}
		valueDynamics := []float64{
			config.Dynamics.AttentionValueWeight,
			config.Dynamics.ValueActivationGain,
			config.Dynamics.ValueActivationReturnRate,
			config.Dynamics.ValueSatiationGain,
			config.Dynamics.ValueSatiationReturnRate,
			config.Dynamics.ValueOrientationGain,
		}
		for _, value := range valueDynamics {
			if value < 0 || value > 1 {
				return nil, errors.New("life value dynamics must remain within 0..1")
			}
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
	organRegistry, err := organ.Load(instanceRoot)
	if err != nil {
		return nil, err
	}
	organRegistry.SetEnvironment(map[string]string{
		"HOMINAL_NETWORK_PROBE_URL": config.ModelGateway.BaseURL,
		"HOMINAL_DATA_ROOT":         config.Platform.DataRoot,
		"HOMINAL_PLATFORM_NAME":     config.Platform.OS,
		"HOMINAL_DESKTOP_SERVICE":   config.Platform.DesktopService,
	})
	runtime := &Runtime{
		instanceRoot:      instanceRoot,
		config:            config,
		store:             store,
		organs:            organRegistry,
		cognizer:          cognizer,
		commands:          make(chan RuntimeCommand, 16),
		notices:           make(chan WorkerNotice),
		results:           make(chan CognitiveResult, 1),
		actionResults:     make(chan ActionResultNotice, 4),
		bodyResults:       make(chan BodySnapshot, 1),
		perceptionResults: make(chan perceptionResult, 1),
		instinctResults:   make(chan instinctResult, 1),
		peripheralLeases:  make(map[string]Lease),
		instinctScenes:    make(map[string]string),
		activeCandidates:  make(map[string]Event),
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
			runtime.state.ValueField = LifeValueField{
				Orientation: config.Seed.ValueOrientation,
				Activation: mapLifeValueVector(config.Seed.ValueOrientation, func(orientation float64) float64 {
					return clamp01(0.10 + 0.20*orientation)
				}),
				UpdatedAt: nowUTC(),
			}
			runtime.state.DifferenceField = make(map[string]DifferenceTrace)
			runtime.state.ValueAffordances = make(map[string]ValueAffordanceTrace)
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
		if config.Stage >= 4 && lifeValueVectorEmpty(runtime.state.ValueField.Orientation) {
			runtime.state.ValueField.Orientation = config.Seed.ValueOrientation
			runtime.state.ValueField.UpdatedAt = nowUTC()
		}
		if config.Stage >= 4 && runtime.state.DifferenceField == nil {
			runtime.state.DifferenceField = make(map[string]DifferenceTrace)
		}
		if config.Stage >= 4 && runtime.state.ValueAffordances == nil {
			runtime.state.ValueAffordances = make(map[string]ValueAffordanceTrace)
		}
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
	if runtime.state.TotalMemories < uint64(len(runtime.state.Memories)) {
		runtime.state.TotalMemories = uint64(len(runtime.state.Memories))
	}
	runtime.learning, err = store.loadLearning(runtime.state)
	if err != nil {
		return nil, fmt.Errorf("load personal history: %w", err)
	}
	runtime.refreshLearningWindow()
	return runtime, nil
}

func (r *Runtime) Run(ctx context.Context) error {
	ctx, cancelRuntime := context.WithCancel(ctx)
	defer cancelRuntime()
	if r.cognizer == nil {
		return errors.New("cognizer is required")
	}
	if err := r.organs.Start(ctx); err != nil {
		r.organs.Stop()
		return err
	}
	defer r.organs.Stop()
	// Cancel workers on every exit path before stopping their organ processes.
	defer cancelRuntime()
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
	r.lastDynamicsAt = time.Now()

	ticker := time.NewTicker(time.Duration(r.config.Pulse.IntervalSeconds) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			if r.perceptionCancel != nil {
				r.perceptionCancel()
			}
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
		case snapshot := <-r.bodyResults:
			r.bodyScanPending = false
			if err := r.acceptBodySnapshot(snapshot); err != nil {
				return err
			}
			r.maybeStartCognition(ctx)
		case observation := <-r.perceptionResults:
			if err := r.acceptPerception(ctx, observation); err != nil {
				return err
			}
			r.maybeStartCognition(ctx)
		case result := <-r.instinctResults:
			if err := r.acceptInstinct(result); err != nil {
				return err
			}
			r.maybeStartCognition(ctx)
		case <-ticker.C:
			if err := r.pulse(ctx); err != nil {
				return err
			}
		}
	}
}

func (r *Runtime) recoverInterrupted() error {
	for key, pending := range r.state.ModelReservations {
		owner := pending.Owner
		usage := UsageRecord{CallID: key, Time: nowUTC(), LeaseID: owner.ID, FocusID: owner.FocusID, RequestedModel: owner.Profile.Model, ReasoningEffort: owner.Profile.ReasoningEffort, ProfileSource: owner.ProfileSource, ReservedMicrousd: pending.Reservation.ReservedMicrousd, ActualMicrousd: pending.Reservation.ReservedMicrousd, Status: "interrupted_unknown"}
		settled := false
		for _, old := range r.state.Usage {
			if usageKey(old) == key {
				settled = true
				break
			}
		}
		if !settled {
			if err := r.store.AppendUsage(usage); err != nil {
				return err
			}
			r.state.Usage = mergeUsageRecords(r.state.Usage, []UsageRecord{usage})
			if err := r.journal("cognition_spend", owner.ID, usage); err != nil {
				return err
			}
		}
		delete(r.state.ModelReservations, key)
		if r.state.Lease != nil && r.state.Lease.ID == owner.ID {
			r.state.Lease.ReservedMicrousd = 0
		}
	}
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
	current := collectSnapshot(r.config, r.state, true, r.organs)
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
	// Birth orientation is a constitutive fact already available to every
	// cognitive request, not a second environmental occurrence.  Keep it in the
	// factual background without recruiting attention; the mentor's waking
	// message is the single conversational event through which Alice first meets
	// those facts.  Treating both copies as pending events made the environment
	// itself elicit two nearly identical replies.
	r.state.EventSeq++
	orientation := Event{
		ID:         fmt.Sprintf("event-%012d", r.state.EventSeq),
		Seq:        r.state.EventSeq,
		Kind:       "birth_orientation",
		Source:     "birth",
		ObservedAt: nowUTC(),
		Summary:    strings.TrimSpace(r.config.BirthBrief),
		Payload:    payload,
		Status:     "processed",
	}
	r.state.Background = append(r.state.Background, orientation)
	if err := r.store.Append(JournalRecord{
		Seq: orientation.Seq, Time: orientation.ObservedAt, Kind: orientation.Kind,
		InstanceID: r.state.InstanceID, Revision: r.state.Revision,
		Payload: map[string]any{
			"event_id": orientation.ID, "source": orientation.Source,
			"summary": orientation.Summary, "payload": json.RawMessage(payload),
			"attention_admitted": false,
		},
	}); err != nil {
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
	if err := r.refreshResourceBody(time.Now().UTC()); err != nil {
		return err
	}
	slow := r.lastSlowScan.IsZero() || time.Since(r.lastSlowScan) >= time.Duration(r.config.Pulse.SlowScanSeconds)*time.Second
	current := mergeFastSnapshot(r.state.Body, collectSnapshot(r.config, r.state, false, r.organs))
	if slow && !r.bodyScanPending {
		r.lastSlowScan = time.Now()
		r.bodyScanPending = true
		config, state, organs := r.config, cloneState(r.state), r.organs
		go func() {
			snapshot := collectSnapshotContext(ctx, config, state, true, organs)
			select {
			case r.bodyResults <- snapshot:
			case <-ctx.Done():
			}
		}()
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
	pendingSurface, pendingPerception := r.pendingPerceptionSurface()
	explorationOrientation := lifeValuePressure(r.state.ValueField).Exploration >= r.config.Dynamics.AttentionThreshold &&
		explorationDominatesValuePressure(r.state.ValueField) && !r.explorationCandidateActive()
	idleOrientation := r.activePerceptionDue(time.Now().UTC())
	sensesOpen := r.config.GenerationKind == "engineering" || r.state.BirthBriefEnteredAt != ""
	if sensesOpen && r.state.Stage >= 8 && !r.attentionCandidateActive() {
		if pendingPerception {
			if err := r.emitPerception(pendingSurface); err != nil {
				return err
			}
		}
	}
	// Read-only senses remain available while the main consciousness reasons.
	// Orientation, unlike observation, can change the scene and stays exclusive.
	if sensesOpen && r.state.Stage >= 8 && r.passivePerceptionAllowed() &&
		(idleOrientation || explorationOrientation || r.state.Lease != nil || r.state.PendingAction != nil) &&
		(r.lastPerceptualScan.IsZero() || time.Since(r.lastPerceptualScan) >= 10*time.Second) {
		if id := r.passiveObservableOrgan(); id != "" {
			r.startPerception(ctx, id, false)
		}
	}
	if r.config.Stage >= 4 {
		now := time.Now()
		elapsed := time.Duration(r.config.Pulse.IntervalSeconds) * time.Second
		if !r.lastDynamicsAt.IsZero() {
			elapsed = now.Sub(r.lastDynamicsAt)
		}
		r.lastDynamicsAt = now
		if err := r.advanceDynamics(elapsed); err != nil {
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

// activePerceptionDue keeps an awake digital body from becoming inert merely
// because every abstract affordance was recently considered. The kernel may
// move an existing sense organ after the common idle boundary; only a concrete
// novel object can then enter cognition, and Alice still decides its meaning.
func (r *Runtime) activePerceptionDue(now time.Time) bool {
	if r.state.Stage < 10 || !r.passivePerceptionAllowed() || r.attentionCandidateActive() {
		return false
	}
	seconds := r.config.Dynamics.AttentionMaximumIdleSeconds
	if seconds <= 0 {
		seconds = 10
	}
	minimum := time.Duration(maxInt(r.config.Pulse.IntervalSeconds*2, 10)) * time.Second
	if !r.lastPerceptualScan.IsZero() && now.Sub(r.lastPerceptualScan) < minimum {
		return false
	}
	return attentionDue(r.state.LastAttentionAt, now, seconds)
}

// Observation does not own the conscious thread. Scene-changing orientation
// has a separate, narrower exclusion boundary.
func (r *Runtime) passivePerceptionAllowed() bool {
	return r.perceptionPending == ""
}

func (r *Runtime) pendingPerceptionSurface() (string, bool) {
	surfaces := make([]string, 0, len(r.state.Perception))
	for surface, trace := range r.state.Perception {
		if len(trace.Pending) > 0 {
			surfaces = append(surfaces, surface)
		}
	}
	sort.Strings(surfaces)
	if len(surfaces) == 0 {
		return "", false
	}
	return surfaces[0], true
}

// supersedePerceptualBatchForAction drops peripheral objects sampled before an
// intentional organ action starts.  An action may change that organ's surface,
// so replaying the remainder of the older observation afterwards would present
// stale scene inventory as current perception.  The objects are habituated,
// not deleted: a later observation can still expose genuinely new objects or a
// changed context through the ordinary sensing path.
func (r *Runtime) supersedePerceptualBatchForAction(organID string) int {
	discarded := 0
	for surface, trace := range r.state.Perception {
		if trace.OrganID != organID || len(trace.Pending) == 0 {
			continue
		}
		discarded += len(trace.Pending)
		trace = discardPendingPerception(trace)
		trace.ExhaustedContext = ""
		trace.ExhaustedAt = ""
		trace.SettledByAttention = false
		r.state.Perception[surface] = trace
	}
	return discarded
}

func (r *Runtime) passiveObservableOrgan() string {
	ids := r.organs.ObservableIDs()
	for offset := 0; offset < len(ids); offset++ {
		index := (r.organCursor + offset) % len(ids)
		id := ids[index]
		body, exists := r.state.Body.Organs[id]
		if exists && body.Accepting && (body.Status == "ready" || body.Status == "recovering") {
			r.organCursor = (index + 1) % len(ids)
			return id
		}
	}
	return ""
}

func (r *Runtime) emitPerception(surface string) error {
	trace, exists := r.state.Perception[surface]
	if !exists {
		return nil
	}
	trace, object, novelContent := takePerceptualNovelty(trace)
	r.state.Perception[surface] = trace
	if novelContent == "" {
		return nil
	}
	observedAt := nowUTC()
	payload, _ := json.Marshal(map[string]any{
		"organ_id":    trace.OrganID,
		"surface_id":  trace.SurfaceID,
		"object_id":   object.ID,
		"digest":      trace.Digest,
		"observed_at": observedAt,
		"content":     truncate(novelContent, perceptualContentLimit),
	})
	return r.addEvent(
		"perceptual_change",
		"observed",
		fmt.Sprintf("%s 器官观察到当前内容记录发生变化；内容是否熟悉、变化是否有意义由你结合经历判断。", trace.OrganID),
		object.ID,
		payload,
		true,
	)
}

func (r *Runtime) recordPerceptualExhaustion(surface string, trace PerceptualTrace, reason string) error {
	trace = discardPendingPerception(trace)
	trace.ExhaustedContext = perceptualContextKey(trace.Context)
	trace.ExhaustedAt = nowUTC()
	trace.SettledByAttention = false
	if r.state.Perception == nil {
		r.state.Perception = make(map[string]PerceptualTrace)
	}
	r.state.Perception[surface] = trace
	payload, _ := json.Marshal(map[string]any{
		"organ_id":    trace.OrganID,
		"surface_id":  trace.SurfaceID,
		"digest":      trace.Digest,
		"observed_at": trace.ObservedAt,
		"context":     trace.Context,
		"reason":      reason,
	})
	// Exhaustion is a sensory control fact. It never enters the attention
	// candidate set: absence of a new referent cannot become the referent of a
	// paid thought, Concern or action. Exploration pressure remains alive while
	// the organ habituates and later resamples reality.
	return r.journal("perceptual_exhaustion", trace.ExhaustedContext, payload)
}

func (r *Runtime) perceptualReorientationSeconds() int {
	idle := r.config.Dynamics.AttentionMaximumIdleSeconds
	if idle <= 0 {
		idle = 10
	}
	// A mature digital body does not need three empty windows before moving its
	// sensory pose.  At the configured maximum-idle boundary it may perform one
	// bounded, deterministic orientation.  This preserves continuous embodied
	// activity without purchasing a main-model thought or reopening a recently
	// settled abstract doorway merely to prove that Alice is still running.
	return maxInt(idle, r.config.Pulse.IntervalSeconds*2)
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
		var repliedMessage *MentorMessage
		if command.Mentor.ReplyTo != "" {
			for index := range r.state.Mentor.Outbox {
				message := &r.state.Mentor.Outbox[index]
				if message.MessageID != command.Mentor.ReplyTo {
					continue
				}
				message.RepliedAt = nowUTC()
				copy := *message
				repliedMessage = &copy
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
		if r.state.Stage >= 10 {
			// Sending was settled by enqueue Reality. A reply is one new
			// utterance, with its earlier causal context available for adoption.
			payload, _ = json.Marshal(struct {
				MentorInput
				ReplyToMessage   *MentorMessage `json:"reply_to_message,omitempty"`
				RelatedConcernID string         `json:"related_concern_id,omitempty"`
			}{command.Mentor, repliedMessage, repliedConcernID})
			repliedConcernID = ""
		}
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
	case "generation_extend":
		if r.config.GenerationKind == "engineering" || r.state.T0 == "" || r.state.PlannedEnd == "" {
			command.Reply <- CommandReply{Status: 409, Body: map[string]string{"error": "generation deadline is unavailable"}}
			return nil
		}
		currentEnd, currentErr := time.Parse(time.RFC3339Nano, r.state.PlannedEnd)
		requestedEnd, requestedErr := time.Parse(time.RFC3339Nano, command.Deadline.PlannedEnd)
		t0, t0Err := time.Parse(time.RFC3339Nano, r.state.T0)
		if currentErr != nil || requestedErr != nil || t0Err != nil {
			command.Reply <- CommandReply{Status: 409, Body: map[string]string{"error": "generation deadline state is invalid"}}
			return nil
		}
		maximumEnd := t0.Add(maximumGenerationLifetime)
		if !requestedEnd.After(currentEnd) {
			command.Reply <- CommandReply{Status: 409, Body: map[string]string{"error": "planned_end must extend the current deadline"}}
			return nil
		}
		if requestedEnd.After(maximumEnd) {
			command.Reply <- CommandReply{Status: 409, Body: map[string]string{"error": "planned_end exceeds the two-hour generation limit"}}
			return nil
		}
		previous := r.state.PlannedEnd
		r.state.PlannedEnd = requestedEnd.UTC().Format(time.RFC3339Nano)
		r.state.Revision++
		if err := r.journal("generation_deadline_extended", r.state.InstanceID, map[string]any{
			"previous_planned_end": previous,
			"planned_end":          r.state.PlannedEnd,
			"maximum_planned_end":  maximumEnd.UTC().Format(time.RFC3339Nano),
		}); err != nil {
			return err
		}
		if err := r.persist(); err != nil {
			return err
		}
		command.Reply <- CommandReply{Status: 200, Body: map[string]string{"status": "extended", "planned_end": r.state.PlannedEnd}}
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
	if notice.Kind == "model_reserve" || notice.Kind == "model_usage" {
		return r.handleModelNotice(notice)
	}
	if r.state.Lease == nil || r.state.Lease.ID != notice.LeaseID {
		notice.Ack <- NoticeAck{Accepted: false}
		return nil
	}
	ack := NoticeAck{Accepted: true}
	switch notice.Kind {
	case "action_start":
		request, ok := notice.Payload.(OrganActionRequest)
		if !ok || r.state.PendingAction != nil {
			ack.Accepted = false
			break
		}
		description, exists := r.organs.Description(request.OrganID)
		if !exists || !stringSliceContains(description.Capabilities, "perform") ||
			!stringSliceContains(description.Operations, request.Operation) || strings.TrimSpace(request.Input) == "" {
			ack.Accepted = false
			break
		}
		if discarded := r.supersedePerceptualBatchForAction(request.OrganID); discarded > 0 {
			if err := r.journal("perceptual_batch_superseded", request.ActionID, map[string]any{
				"organ_id": request.OrganID, "discarded_objects": discarded,
				"reason": "intentional organ action started",
			}); err != nil {
				return err
			}
		}
		r.state.PendingAction = &ActionState{ID: request.ActionID, LeaseID: notice.LeaseID, Kind: "organ_action", OrganID: request.OrganID, Operation: request.Operation, Request: request.Input, Status: "started", StartedAt: nowUTC()}
		if err := r.journal("action_started", request.ActionID, map[string]any{"kind": "organ_action", "organ_id": request.OrganID, "operation": request.Operation, "input": request.Input, "timeout_seconds": request.TimeoutSeconds}); err != nil {
			return err
		}
		timeout := time.Duration(request.TimeoutSeconds) * time.Second
		if timeout <= 0 || timeout > 2*time.Minute {
			timeout = 30 * time.Second
		}
		callCtx, cancel := context.WithTimeout(context.Background(), timeout)
		performed, performErr := r.organs.Perform(callCtx, request.OrganID, organ.ActionRequest{
			ActionID: request.ActionID, Operation: request.Operation, Input: request.Input, TimeoutMilliseconds: int(timeout.Milliseconds()),
		})
		cancel()
		status := performed.Status
		effect := performed.Effect
		output := performed.Output
		if performErr != nil {
			status = "failed"
			effect = "unknown"
			if errors.Is(callCtx.Err(), context.DeadlineExceeded) || errors.Is(callCtx.Err(), context.Canceled) {
				status = "unknown"
			}
			output = fmt.Sprintf(`{"error":%q}`, truncate(performErr.Error(), 2048))
		}
		r.state.PendingAction.Status = status
		r.state.PendingAction.Effect = effect
		r.state.PendingAction.EndedAt = nowUTC()
		r.state.PendingAction.Result = truncate(redactRuntimeSecret(output, r.config.ModelGateway.APIKey), 64*1024)
		if err := r.journal("action_"+status, request.ActionID, map[string]any{"kind": r.state.PendingAction.Kind, "organ_id": request.OrganID, "operation": request.Operation, "result": r.state.PendingAction.Result}); err != nil {
			return err
		}
		response := map[string]any{"status": status, "output": r.state.PendingAction.Result}
		encoded, _ := json.Marshal(response)
		ack.Output = string(encoded)
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
	if result.Error == nil && r.state.Stage >= 10 {
		if r.state.Lease.ProfileSource == "next" && (result.Assistance == nil || result.Stage4 != nil || strings.TrimSpace(result.Assistance.Answer) == "") {
			result.Error = errors.New("local assistance must return a bounded answer, not a cognitive commit")
		} else if result.Assistance != nil && r.state.Lease.ProfileSource != "next" {
			result.Error = errors.New("main cognition requires its own cognitive commit")
		}
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
			infrastructureFailure := isCallFailure && modelInfrastructureFailure(callFailure.Fact.Category)
			contractFailure := isCallFailure && modelOutputContractFailure(callFailure.Fact.Category)
			paidUnusable := false
			if !isCallFailure {
				paidUnusable = r.markLeaseUsageUnusable(result.LeaseID)
			}
			attempts := 0
			if infrastructureFailure {
				markEventForInfrastructureRetry(&r.state, r.state.Lease.FocusID, result.Error.Error())
			} else {
				attempts = markEventForRetry(&r.state, r.state.Lease.FocusID, result.Error.Error())
			}
			protected := false
			waitModel := ""
			if isCallFailure && !infrastructureFailure && !contractFailure {
				var err error
				protected, err = r.protectModelAfterFailures(r.state.Lease.Profile.Model)
				if err != nil {
					return err
				}
				if r.state.Lease.ProfileSource == "resource_recovery" && r.state.Lease.RecoveryForModel != "" {
					// Two distinct routes for the same cognition have now failed.
					// Preserve the Reality, but stop rapid gateway probing for the
					// full configured protection window. This is a bodily backoff,
					// not a judgment about the focus or a persistent model choice.
					if err := r.extendModelProtectionAfterRecoveryFailure(r.state.Lease.RecoveryForModel); err != nil {
						return err
					}
					waitModel = r.state.Lease.RecoveryForModel
				} else if protected {
					waitModel = r.state.Lease.Profile.Model
				}
			}
			if waitModel != "" {
				// Keep the same causal foreground eligible for the one bounded
				// alternate-model recovery. Sending it straight to model_wait here
				// made the action Reality block every other focus until the failed
				// model's timer expired; expiry then removed the protection record,
				// so the runtime retried the same model and could never reach the
				// recovery path in maybeStartCognition.
				if waitModel == r.state.Lease.Profile.Model && r.protectedModelRecoveryAvailable(r.state.Lease.Profile.Model) {
					markEvent(&r.state, r.state.Lease.FocusID, "pending")
				} else {
					markEventModelWait(&r.state, r.state.Lease.FocusID, waitModel)
				}
			} else if !infrastructureFailure && paidUnusable && attempts > r.config.CognitiveResource.ValidationRetryPerFocus && !r.focusIsActionResult(r.state.Lease.FocusID) {
				recovered, err := r.planValidationRecovery(r.state.Lease.FocusID, r.state.Lease.Profile)
				if err != nil {
					return err
				}
				if !recovered {
					if err := r.exhaustCognition(r.state.Lease.FocusID); err != nil {
						return err
					}
				}
			} else if !infrastructureFailure && attempts > r.config.CognitiveResource.ValidationRetryPerFocus && !r.focusIsActionResult(r.state.Lease.FocusID) {
				markEvent(&r.state, r.state.Lease.FocusID, "failed")
			}
			failurePayload := map[string]any{"focus_id": result.FocusID, "error": result.Error.Error(), "attempt": attempts}
			if callFailure != nil {
				failurePayload["model_failure"] = callFailure.Fact
			}
			if infrastructureFailure {
				failurePayload["retry_class"] = "infrastructure"
			}
			if err := r.journal("cognition_failed", result.LeaseID, failurePayload); err != nil {
				return err
			}
		}
	} else if result.Assistance != nil {
		r.releaseSuccessfulRecovery(r.state.Lease)
		if err := r.acceptAssistance(result); err != nil {
			return err
		}
	} else if r.state.Stage >= 4 && result.Stage4 != nil {
		r.releaseSuccessfulRecovery(r.state.Lease)
		commit, withheldActionKind := normalizeUnendorsedAction(*result.Stage4, r.config.Dynamics.AttentionThreshold)
		if err := r.applyPreparedCognitiveCommit(commit, withheldActionKind); err != nil {
			var progressBoundary *actionProgressBoundary
			if errors.As(err, &progressBoundary) {
				// A settled embodied request is a real limit, not a broken model
				// response. Settle an ordinary attention episode, but keep original
				// action Reality available for a later valid interpretation. Return a
				// compact boundary fact; every organ receives the same loop.
				if r.state.Stage >= 10 && r.focusIsActionResult(r.state.Lease.FocusID) {
					markEventForRetry(&r.state, r.state.Lease.FocusID, err.Error())
				} else {
					markEvent(&r.state, r.state.Lease.FocusID, "processed")
				}
				if candidate, exists := r.activeCandidates[r.state.Lease.FocusID]; exists {
					if key := valueAffordanceKey(candidate); key != "" {
						r.settleValueAffordance(key, false, nowUTC())
					}
				}
				if boundaryErr := r.returnActionProgressBoundary(result, commit, progressBoundary); boundaryErr != nil {
					return boundaryErr
				}
			} else {
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
					"focus_id":             result.FocusID,
					"error":                err.Error(),
					"action_kind":          result.Stage4.Action.Kind,
					"resource_choice":      result.Stage4.ResourceChoice,
					"reality_update_count": len(result.Stage4.RealityUpdates),
				}); journalErr != nil {
					return journalErr
				}
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
		r.releaseSuccessfulRecovery(r.state.Lease)
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

func (r *Runtime) returnActionProgressBoundary(result CognitiveResult, commit CognitiveCommit, boundary *actionProgressBoundary) error {
	kind, request := cognitiveActionRequest(commit.Action)
	payload, _ := json.Marshal(map[string]any{
		"focus_id":          result.FocusID,
		"action_kind":       kind,
		"organ_id":          commit.Action.OrganID,
		"operation":         commit.Action.Operation,
		"attempted_request": request,
		"reason":            boundary.Error(),
	})
	concernID := r.focusConcernID(result.FocusID)
	return r.addEvent(
		"action_boundary",
		"interoception",
		"此次行动未执行。具体身体边界见 reason，原始事实与既有结果继续保留，可据此决定下一步。",
		result.LeaseID,
		payload,
		true,
		concernID,
	)
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
		if err == nil && now.Sub(last) < r.realityRetryDelay(*event) {
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
		failed := (usage.FailureCategory != "" && !modelInfrastructureFailure(usage.FailureCategory) && !modelOutputContractFailure(usage.FailureCategory)) ||
			(usage.Status == "unusable" && usage.CostConfirmed && usage.ActualMicrousd > 0)
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
	if !gatewayRetry(r.state, r.config.CognitiveResource).allows(time.Now().UTC(), true) {
		return
	}
	// Cancelling a movement cannot undo a tab switch or scroll already made.
	// Let this bounded orientation and its observation settle before copying
	// the scene into consciousness. Ordinary read-only sensing stays concurrent.
	if r.perceptionOrients {
		return
	}
	if r.config.GenerationKind != "engineering" && r.state.BirthBriefEnteredAt == "" {
		return
	}
	// A body HTTP probe describes one route, not the availability of the model
	// gateway. Actual model failures already have a shared bounded backoff.
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
		if r.protectedModelRecoveryAvailable(requestedModel) {
			recovery, _ := r.recoveryProfile(requestedModel)
			profile = recovery
			profileSource = "resource_recovery"
			profilePurpose = "理解当前模型不可用的身体事实，并自主选择等待、切换或转向"
			protectedState.RecoveryBlocked = true
			r.state.CognitiveResource.ProtectedModels[requestedModel] = protectedState
		} else {
			markEventModelWait(&r.state, request.Focus.ID, requestedModel)
			_ = r.persist()
			return
		}
	}
	r.state.Revision++
	lease := Lease{ID: "lease-" + randomID(), Revision: r.state.Revision, PulseID: r.state.PulseID, FocusID: request.Focus.ID, StartedAt: nowUTC(), Profile: profile, ProfileSource: profileSource, ProfilePurpose: profilePurpose, VariationBias: request.VariationBias, VariationSeed: request.VariationSeed}
	if profileSource == "resource_recovery" {
		lease.RecoveryForModel = requestedModel
	}
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
	if r.state.Stage >= 10 && profileSource != "next" {
		request.Recall = r.learning.recall(memoryQuery(request.Candidates), request.VariationSeed)
		request.VariationBias = ""
		_ = r.journal("memory_recalled", lease.ID, recallContext(request.Recall))
	}
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
		request.Focus.Kind == "cognition_assistance_result" ||
		(r.state.Stage >= 10 && request.Focus.Kind == "mentor_received" && !timeAfter(request.Focus.ObservedAt, r.state.PlannedEnd)) ||
		(request.Focus.Kind == "mentor_received" && commitmentIDFromEvent(request.Focus) != "")
}

func (r *Runtime) recoveryProfile(failedModel string) (CognitiveProfile, bool) {
	// Stage ten assigns consciousness and organ assistance distinct roles.
	// A failed assistant can hand back to the main profile; a main-model failure
	// waits for that model instead of automatically promoting the high role or an instinct.
	if r.state.Stage >= 10 {
		main := r.state.CognitiveResource.DefaultProfile
		if main.Model == "" {
			main = r.config.CognitiveResource.InitialDefaultProfile
		}
		protected, _ := modelProtected(r.state, main.Model, time.Now().UTC())
		return main, main.Model != failedModel && !protected && validateProfile(r.config.CognitiveResource, main) == nil
	}
	// Preserve cognition with the action-capable support model before falling to
	// a lower-capability alternate. The recovery source and purpose remain
	// visible to Alice; this is continuity under a failed organ, not a silent
	// second stream of thought.
	for _, model := range []string{"high", "main", "fast"} {
		if model == failedModel {
			continue
		}
		if protected, _ := modelProtected(r.state, model, time.Now().UTC()); protected {
			continue
		}
		for _, effort := range []string{"low", "none", "medium"} {
			profile := CognitiveProfile{Model: model, ReasoningEffort: effort}
			if validateProfile(r.config.CognitiveResource, profile) == nil {
				return profile, true
			}
		}
	}
	return CognitiveProfile{}, false
}

func (r *Runtime) protectedModelRecoveryAvailable(model string) bool {
	protected, active := r.state.CognitiveResource.ProtectedModels[model]
	if !active || protected.RecoveryBlocked {
		return false
	}
	if stillProtected, _ := modelProtected(r.state, model, time.Now().UTC()); !stillProtected {
		return false
	}
	_, available := r.recoveryProfile(model)
	return available
}

// A protected primary model may borrow an alternate for as many successive
// causal steps as are needed to keep one life thread moving. Starting a
// recovery blocks another attempt until its result is known. Success reopens
// the route for the next Reality or focus; failure leaves it blocked until the
// primary protection expires. This preserves continuity without running a
// concurrent or hidden second consciousness.
func (r *Runtime) releaseSuccessfulRecovery(lease *Lease) {
	if lease == nil || lease.ProfileSource != "resource_recovery" || lease.RecoveryForModel == "" {
		return
	}
	protected, exists := r.state.CognitiveResource.ProtectedModels[lease.RecoveryForModel]
	if !exists {
		return
	}
	protected.RecoveryBlocked = false
	r.state.CognitiveResource.ProtectedModels[lease.RecoveryForModel] = protected
}

func (r *Runtime) extendModelProtectionAfterRecoveryFailure(model string) error {
	protected, ok := r.state.CognitiveResource.ProtectedModels[model]
	if !ok {
		return nil
	}
	now := time.Now().UTC()
	until := now.Add(time.Duration(r.config.CognitiveResource.ModelProtectionMinutes) * time.Minute)
	if current, err := time.Parse(time.RFC3339Nano, protected.Until); err == nil && current.After(until) {
		until = current
	}
	protected.Until = until.Format(time.RFC3339Nano)
	protected.RecoveryBlocked = true
	r.state.CognitiveResource.ProtectedModels[model] = protected
	return r.journal("cognitive_recovery_failed", model, map[string]any{
		"model": model, "until": protected.Until,
	})
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
	_, err := r.addEventWithAdmission(kind, source, summary, correlationID, payload, candidate, concernIDs...)
	return err
}

// addEventWithAdmission exposes only the factual outcome of the common
// pre-conscious admission gate.  Most producers merely record a signal and use
// addEvent; dynamics that change because Alice consciously encountered a
// signal must distinguish accumulation below attention from actual admission.
func (r *Runtime) addEventWithAdmission(kind, source, summary, correlationID string, payload json.RawMessage, candidate bool, concernIDs ...string) (bool, error) {
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
	if candidate && r.state.Stage >= 4 {
		var admitted bool
		event, admitted = r.admitDifference(event)
		candidate = admitted
	}
	if candidate {
		event.Status = "pending"
		if len(concernIDs) > 0 {
			event.ConcernID = concernIDs[0]
		}
		r.state.Background = append(r.state.Background, event)
	}
	record := JournalRecord{Seq: event.Seq, Time: event.ObservedAt, Kind: kind, InstanceID: r.state.InstanceID, Revision: r.state.Revision, CorrelationID: correlationID, Payload: map[string]any{
		"event_id": event.ID, "source": source, "summary": summary, "payload": json.RawMessage(payload),
		"attention_admitted": candidate, "difference_key": event.DifferenceKey,
		"prediction_gap": event.PredictionGap, "attention_pressure": event.AttentionPressure,
	}}
	return candidate, r.store.Append(record)
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
	r.state.SampleID = fmt.Sprintf("alice%02d%02d-%02d%02d%02d", int(shanghai.Month()), shanghai.Day(), shanghai.Hour(), shanghai.Minute(), shanghai.Second())
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
		if eventChainActive(event.Status) {
			kept = append(kept, event)
		}
	}
	for index := len(r.state.Background) - 1; index >= 0 && len(kept) < 128; index-- {
		event := r.state.Background[index]
		if !eventChainActive(event.Status) {
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

// A gateway or transport interruption changes neither Alice's appraisal nor a
// model's demonstrated cognitive ability. Preserve the same factual focus and
// retry it after the ordinary causal delay without consuming its semantic
// validation attempts or switching personalities under a shared outage.
func markEventForInfrastructureRetry(state *State, id, failure string) {
	for index := range state.Background {
		if state.Background[index].ID == id {
			state.Background[index].Status = "retry_wait"
			state.Background[index].LastFocusedAt = nowUTC()
			state.Background[index].LastCommitErr = truncate(failure, 1024)
			return
		}
	}
	for index := range state.Concerns {
		if state.Concerns[index].ID == id {
			state.Concerns[index].LastFocusedAt = nowUTC()
			state.Concerns[index].LastCommitErr = truncate(failure, 1024)
			return
		}
	}
}

func modelInfrastructureFailure(category string) bool {
	switch category {
	case "transport_error", "transport_timeout", "response_read_error", "upstream_unavailable", "idempotency_in_progress", "rate_limited", "gateway_quota", "gateway_backoff":
		return true
	default:
		return false
	}
}

// The provider answered, but this requested output did not satisfy its
// contract. Repair/backoff belongs to that cognition, like local validation;
// it does not establish that other cognitive requests cannot use the model.
// Billing uncertainty is retained independently in the usage ledger.
func modelOutputContractFailure(category string) bool {
	switch category {
	case "invalid_provider_tool_call", "invalid_function_output", "invalid_response_status", "response_failed":
		return true
	default:
		return false
	}
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
