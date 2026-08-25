package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	if config.Stage != 3 && config.Stage != 4 {
		return nil, fmt.Errorf("unsupported runtime stage %d", config.Stage)
	}
	if config.Pulse.IntervalSeconds <= 0 {
		config.Pulse.IntervalSeconds = 5
	}
	if config.Pulse.SlowScanSeconds <= 0 {
		config.Pulse.SlowScanSeconds = 60
	}
	if config.Quota.WindowMins <= 0 {
		config.Quota.WindowMins = 60
	}
	if config.Model.MaxOutputTokens <= 0 {
		config.Model.MaxOutputTokens = 1200
	}
	if config.Stage == 4 {
		if config.Dynamics.AttentionCandidateLimit <= 0 {
			config.Dynamics.AttentionCandidateLimit = defaultAttentionCandidateLimit
		}
		if config.Dynamics.AttentionRevisitSeconds <= 0 {
			config.Dynamics.AttentionRevisitSeconds = defaultAttentionRevisitSeconds
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
			Schema:     stateSchema,
			InstanceID: instanceID,
			Stage:      config.Stage,
			ReadyAt:    nowUTC(),
			Mentor: MentorState{
				Received: make(map[string]uint64),
			},
		}
		if config.Stage == 4 {
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
		runtime.state = *loaded
		if runtime.state.Mentor.Received == nil {
			runtime.state.Mentor.Received = make(map[string]uint64)
		}
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
			r.config.Stage == 4,
		); err != nil {
			return err
		}
		r.state.PendingAction = nil
	}
	return r.persist()
}

func (r *Runtime) initialSnapshot() error {
	used := usageInWindow(r.state.Usage, r.config.Quota.WindowMins)
	current := collectSnapshot(r.config, used, true)
	initial := r.state.Body.ObservedAt == ""
	differences := bodyDifferences(r.state.Body, current, initial)
	r.state.Body = current
	r.lastSlowScan = time.Now()
	if len(differences) > 0 {
		payload, _ := json.Marshal(current)
		return r.addEvent("body_delta", "observed", strings.Join(differences, "; "), "", payload, r.config.Stage == 4)
	}
	return nil
}

func (r *Runtime) pulse(ctx context.Context) error {
	r.state.PulseID++
	r.state.LastPulseAt = nowUTC()
	r.pruneUsage()
	used := usageInWindow(r.state.Usage, r.config.Quota.WindowMins)
	slow := r.lastSlowScan.IsZero() || time.Since(r.lastSlowScan) >= time.Duration(r.config.Pulse.SlowScanSeconds)*time.Second
	current := collectSnapshot(r.config, used, slow)
	if !slow {
		current = mergeFastSnapshot(r.state.Body, current)
	} else {
		r.lastSlowScan = time.Now()
	}
	differences := bodyDifferences(r.state.Body, current, false)
	r.state.Body = current
	if len(differences) > 0 {
		payload, _ := json.Marshal(current)
		if err := r.addEvent("body_delta", "observed", strings.Join(differences, "; "), "", payload, r.config.Stage == 4); err != nil {
			return err
		}
	}
	if r.config.Stage == 4 {
		if err := r.advanceDynamics(time.Duration(r.config.Pulse.IntervalSeconds) * time.Second); err != nil {
			return err
		}
	}
	r.releaseRetryableEvents()
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

func (r *Runtime) handleCommand(ctx context.Context, command RuntimeCommand) error {
	switch command.Kind {
	case "mentor_receive":
		if seq, exists := r.state.Mentor.Received[command.Mentor.MessageID]; exists {
			command.Reply <- CommandReply{Status: 200, Body: map[string]any{"status": "duplicate", "seq": seq}}
			return nil
		}
		if command.Mentor.ReplyTo != "" {
			for index := range r.state.Mentor.Outbox {
				if r.state.Mentor.Outbox[index].MessageID == command.Mentor.ReplyTo {
					r.state.Mentor.Outbox[index].RepliedAt = nowUTC()
				}
			}
		}
		payload, _ := json.Marshal(command.Mentor)
		if err := r.addEvent("mentor_received", "observed", command.Mentor.Body, command.Mentor.MessageID, payload, true); err != nil {
			return err
		}
		r.state.Mentor.Received[command.Mentor.MessageID] = r.state.EventSeq
		if err := r.persist(); err != nil {
			return err
		}
		command.Reply <- CommandReply{Status: 202, Body: map[string]any{"status": "queued", "seq": r.state.EventSeq}}
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
	case "model_usage":
		usage, ok := notice.Payload.(UsageRecord)
		if !ok {
			return errors.New("invalid model usage notice")
		}
		r.state.Usage = append(r.state.Usage, usage)
		if err := r.journal("model_usage", notice.LeaseID, usage); err != nil {
			return err
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
		markEvent(&r.state, r.state.Lease.FocusID, "retry_wait")
		if err := r.journal("cognition_failed", result.LeaseID, map[string]any{"focus_id": result.FocusID, "error": result.Error.Error()}); err != nil {
			return err
		}
	} else if r.state.Stage == 4 && result.Stage4 != nil {
		if err := r.applyCognitiveCommit(*result.Stage4); err != nil {
			markEventForRetry(&r.state, r.state.Lease.FocusID, err.Error())
			if journalErr := r.journal("cognition_failed", result.LeaseID, map[string]any{"focus_id": result.FocusID, "error": err.Error()}); journalErr != nil {
				return journalErr
			}
		} else {
			markEvent(&r.state, result.Stage4.FocusID, "processed")
			action := result.Stage4.Action
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

func (r *Runtime) maybeStartCognition(parent context.Context) {
	if r.state.Lease != nil {
		return
	}
	request, ok := r.nextCognitiveRequest()
	if !ok {
		return
	}
	r.state.Revision++
	lease := Lease{ID: "lease-" + randomID(), Revision: r.state.Revision, PulseID: r.state.PulseID, FocusID: request.Focus.ID, StartedAt: nowUTC()}
	r.state.Lease = &lease
	r.state.CurrentFocus = request.Focus.ID
	markEvent(&r.state, request.Focus.ID, "in_focus")
	_ = r.journal("cognition_started", lease.ID, map[string]any{"focus_id": request.Focus.ID, "candidate_count": len(request.Candidates)})
	_ = r.persist()
	request.Lease = lease
	request.State = cloneState(r.state)
	request.Config = r.config
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

func (r *Runtime) nextCognitiveRequest() (CognitiveRequest, bool) {
	if r.state.Stage == 3 {
		for _, event := range r.state.Background {
			if event.Status == "pending" && event.Kind == "mentor_received" {
				return CognitiveRequest{Stage: 3, Focus: event, Candidates: []Event{event}}, true
			}
		}
		return CognitiveRequest{}, false
	}
	return r.nextStage4Request()
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
	cutoff := time.Now().UTC().Add(-time.Duration(r.config.Quota.WindowMins) * time.Minute)
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
			}
			return
		}
	}
}

func markEventForRetry(state *State, id, commitError string) {
	for index := range state.Background {
		if state.Background[index].ID == id {
			state.Background[index].Status = "retry_wait"
			state.Background[index].LastFocusedAt = nowUTC()
			state.Background[index].LastCommitErr = truncate(commitError, 1024)
			return
		}
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
