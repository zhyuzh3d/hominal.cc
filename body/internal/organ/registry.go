package organ

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"
)

var validOrganID = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,31}$`)
var validOperation = regexp.MustCompile(`^[a-z][a-z0-9_.:-]{0,63}$`)

type installedOrgan struct {
	manifest     Manifest
	commandPath  string
	description  Description
	process      *exec.Cmd
	log          *os.File
	startupError string
}

type Registry struct {
	instanceRoot string
	runtimeDir   string
	organs       map[string]*installedOrgan
	faults       map[string]Snapshot
	extraEnv     []string
}

func Load(instanceRoot string) (*Registry, error) {
	root, err := filepath.Abs(instanceRoot)
	if err != nil {
		return nil, err
	}
	runtimeDir := strings.TrimSpace(os.Getenv("HOMINAL_ORGAN_RUNTIME_DIR"))
	if runtimeDir == "" {
		runtimeDir = "/run/hominal/organs"
	}
	registry := &Registry{
		instanceRoot: root, runtimeDir: runtimeDir,
		organs: make(map[string]*installedOrgan), faults: make(map[string]Snapshot),
	}
	manifestPaths, err := filepath.Glob(filepath.Join(root, "body", "organs", "*.json"))
	if err != nil {
		return nil, err
	}
	sort.Strings(manifestPaths)
	for index, path := range manifestPaths {
		faultID := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if !validOrganID.MatchString(faultID) {
			faultID = fmt.Sprintf("invalid-%d", index+1)
		}
		recordFault := func(reason error) {
			registry.faults[faultID] = Snapshot{Name: faultID, Status: "unavailable", Guidance: reason.Error()}
		}
		encoded, err := os.ReadFile(path)
		if err != nil {
			recordFault(err)
			continue
		}
		var manifest Manifest
		if err := json.Unmarshal(encoded, &manifest); err != nil {
			recordFault(fmt.Errorf("decode organ manifest: %w", err))
			continue
		}
		if manifest.Schema != ManifestSchema || !validOrganID.MatchString(manifest.ID) {
			recordFault(errors.New("invalid organ manifest"))
			continue
		}
		faultID = manifest.ID
		if _, exists := registry.organs[manifest.ID]; exists {
			faultID = fmt.Sprintf("%s-duplicate-%d", manifest.ID, index+1)
			recordFault(fmt.Errorf("duplicate organ id %q", manifest.ID))
			continue
		}
		if filepath.IsAbs(manifest.Command) || strings.TrimSpace(manifest.Command) == "" {
			recordFault(errors.New("organ command must be relative to the instance"))
			continue
		}
		commandPath := filepath.Clean(filepath.Join(root, manifest.Command))
		if commandPath == root || !strings.HasPrefix(commandPath, root+string(os.PathSeparator)) {
			recordFault(errors.New("organ command escapes the instance"))
			continue
		}
		if info, err := os.Stat(commandPath); err != nil || info.IsDir() || info.Mode()&0o111 == 0 {
			recordFault(fmt.Errorf("organ command is not executable: %s", commandPath))
			continue
		}
		installed := &installedOrgan{manifest: manifest, commandPath: commandPath}
		description, err := registry.describe(context.Background(), installed)
		if err != nil {
			recordFault(fmt.Errorf("describe failed: %w", err))
			continue
		}
		if description.Schema != DescriptionSchema || description.ID != manifest.ID || strings.TrimSpace(description.Name) == "" {
			recordFault(errors.New("organ returned an invalid description"))
			continue
		}
		if hasCapability(description.Capabilities, "perform") {
			seen := make(map[string]bool, len(description.Operations))
			for _, operation := range description.Operations {
				if !validOperation.MatchString(operation) || seen[operation] {
					recordFault(fmt.Errorf("organ returned invalid action operation %q", operation))
					continue
				}
				seen[operation] = true
			}
			if len(description.Operations) == 0 || len(seen) != len(description.Operations) {
				recordFault(errors.New("action organ must publish a valid operation catalog"))
				continue
			}
			inputContractsValid := true
			for operation, input := range description.OperationInputs {
				if !seen[operation] || strings.TrimSpace(input) == "" {
					recordFault(fmt.Errorf("organ returned invalid input contract for operation %q", operation))
					inputContractsValid = false
					break
				}
			}
			if !inputContractsValid {
				continue
			}
		}
		installed.description = description
		registry.organs[manifest.ID] = installed
	}
	return registry, nil
}

func (r *Registry) Start(ctx context.Context) error {
	if len(r.organs) == 0 {
		return nil
	}
	if err := os.MkdirAll(r.runtimeDir, 0o755); err != nil {
		for _, organ := range r.organs {
			organ.startupError = err.Error()
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Join(r.instanceRoot, "logs"), 0o755); err != nil {
		for _, organ := range r.organs {
			organ.startupError = err.Error()
		}
		return nil
	}
	for _, id := range r.IDs() {
		organ := r.organs[id]
		if !organ.manifest.Daemon {
			continue
		}
		_ = os.Remove(r.socketPath(id))
		log, err := os.OpenFile(filepath.Join(r.instanceRoot, "logs", "organ-"+id+".log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
		if err != nil {
			organ.startupError = err.Error()
			continue
		}
		command := exec.CommandContext(ctx, organ.commandPath, "serve")
		command.Dir = r.instanceRoot
		command.Env = r.environment(id, "organ-host")
		command.Stdout = log
		command.Stderr = log
		command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		if err := command.Start(); err != nil {
			_ = log.Close()
			organ.startupError = err.Error()
			continue
		}
		organ.process = command
		organ.log = log
		deadline := time.Now().Add(2 * time.Second)
		for {
			if info, statErr := os.Stat(r.socketPath(id)); statErr == nil && info.Mode()&os.ModeSocket != 0 {
				break
			}
			if time.Now().After(deadline) {
				organ.startupError = "organ did not publish its socket"
				_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
				_ = command.Wait()
				organ.process = nil
				_ = log.Close()
				organ.log = nil
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	return nil
}

func (r *Registry) Stop() {
	for _, id := range r.IDs() {
		organ := r.organs[id]
		if organ.process != nil && organ.process.Process != nil {
			_ = syscall.Kill(-organ.process.Process.Pid, syscall.SIGTERM)
			done := make(chan struct{})
			go func(command *exec.Cmd) {
				_ = command.Wait()
				close(done)
			}(organ.process)
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				_ = syscall.Kill(-organ.process.Process.Pid, syscall.SIGKILL)
				<-done
			}
		}
		if organ.log != nil {
			_ = organ.log.Close()
		}
		_ = os.Remove(r.socketPath(id))
	}
}

func (r *Registry) IDs() []string {
	ids := make([]string, 0, len(r.organs))
	for id := range r.organs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func (r *Registry) ObservableIDs() []string {
	ids := make([]string, 0, len(r.organs))
	for _, id := range r.IDs() {
		if hasCapability(r.organs[id].description.Capabilities, "observe") &&
			!hasCapability(r.organs[id].description.Capabilities, "body_state") {
			ids = append(ids, id)
		}
	}
	return ids
}

func (r *Registry) BodyStateIDs() []string {
	ids := make([]string, 0, len(r.organs))
	for _, id := range r.IDs() {
		if hasCapability(r.organs[id].description.Capabilities, "observe") &&
			hasCapability(r.organs[id].description.Capabilities, "body_state") {
			ids = append(ids, id)
		}
	}
	return ids
}

func (r *Registry) SetEnvironment(values map[string]string) {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	r.extraEnv = r.extraEnv[:0]
	for _, key := range keys {
		if strings.TrimSpace(key) != "" && !strings.Contains(key, "=") {
			r.extraEnv = append(r.extraEnv, key+"="+values[key])
		}
	}
}

func (r *Registry) Snapshot() map[string]Snapshot {
	return r.SnapshotContext(context.Background())
}

func (r *Registry) SnapshotContext(parent context.Context) map[string]Snapshot {
	result := make(map[string]Snapshot, len(r.organs)+len(r.faults))
	for id, snapshot := range r.faults {
		result[id] = snapshot
	}
	for _, id := range r.IDs() {
		organ := r.organs[id]
		health := Health{Status: "unavailable"}
		if organ.startupError == "" {
			ctx, cancel := context.WithTimeout(parent, 2*time.Second)
			observed, err := r.Health(ctx, id)
			cancel()
			if err == nil {
				health = observed
			}
		}
		guidance := organ.description.Guidance
		if organ.startupError != "" {
			guidance = strings.TrimSpace(guidance + " 当前器官入口不可用：" + organ.startupError)
		}
		result[id] = Snapshot{
			Name: organ.description.Name, Command: organ.description.Command,
			Capabilities:    append([]string{}, organ.description.Capabilities...),
			Operations:      append([]string{}, organ.description.Operations...),
			OperationInputs: cloneStringMap(organ.description.OperationInputs),
			Guidance:        guidance, Status: health.Status, Accepting: health.Accepting,
		}
	}
	return result
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func (r *Registry) Health(ctx context.Context, id string) (Health, error) {
	organ, err := r.organ(id)
	if err != nil {
		return Health{}, err
	}
	var health Health
	if err := r.invoke(ctx, organ, "health", &health); err != nil {
		return Health{}, err
	}
	if health.Schema != HealthSchema || health.ID != id || !validHealthStatus(health.Status) {
		return Health{}, errors.New("organ returned an invalid health envelope")
	}
	return health, nil
}

func (r *Registry) Observe(ctx context.Context, id string) (Observation, error) {
	organ, err := r.organ(id)
	if err != nil {
		return Observation{}, err
	}
	var observation Observation
	if err := r.invoke(ctx, organ, "observe", &observation); err != nil {
		return Observation{}, err
	}
	if observation.Schema != ObservationSchema || observation.OrganID != id || strings.TrimSpace(observation.SurfaceID) == "" {
		return Observation{}, errors.New("organ returned an invalid observation envelope")
	}
	return observation, nil
}

func (r *Registry) Orient(ctx context.Context, id string) (Orientation, error) {
	organ, err := r.organ(id)
	if err != nil {
		return Orientation{}, err
	}
	var orientation Orientation
	if err := r.invoke(ctx, organ, "orient", &orientation); err != nil {
		return Orientation{}, err
	}
	if orientation.Schema != OrientationSchema || orientation.OrganID != id || strings.TrimSpace(orientation.Status) == "" {
		return Orientation{}, errors.New("organ returned an invalid orientation envelope")
	}
	return orientation, nil
}

func (r *Registry) Perform(ctx context.Context, id string, request ActionRequest) (ActionResult, error) {
	organ, err := r.organ(id)
	if err != nil {
		return ActionResult{}, err
	}
	if !hasCapability(organ.description.Capabilities, "perform") {
		return ActionResult{}, fmt.Errorf("organ %q does not accept actions", id)
	}
	if strings.TrimSpace(request.ActionID) == "" || strings.TrimSpace(request.Operation) == "" {
		return ActionResult{}, errors.New("organ action requires action_id and operation")
	}
	if !hasCapability(organ.description.Operations, request.Operation) {
		return ActionResult{}, fmt.Errorf("organ %q does not publish operation %q; available operations: %s", id, request.Operation, strings.Join(organ.description.Operations, ", "))
	}
	if request.TimeoutMilliseconds <= 0 {
		request.TimeoutMilliseconds = 30_000
	}
	request.Schema = ActionSchema
	encoded, err := json.Marshal(request)
	if err != nil {
		return ActionResult{}, err
	}
	var result ActionResult
	if err := r.invokeArgs(ctx, organ, "intentional-action", &result, "perform", string(encoded)); err != nil {
		return ActionResult{}, err
	}
	if result.Schema != ActionResultSchema || result.OrganID != id || result.ActionID != request.ActionID ||
		!validActionStatus(result.Status) || !validActionEffect(result.Effect) ||
		strings.TrimSpace(result.ObservedAt) == "" || strings.TrimSpace(result.Summary) == "" {
		return ActionResult{}, errors.New("organ returned an invalid action result envelope")
	}
	if result.Observation != nil && (result.Observation.Schema != ObservationSchema ||
		result.Observation.OrganID != id || strings.TrimSpace(result.Observation.SurfaceID) == "") {
		return ActionResult{}, errors.New("organ returned an invalid action result observation")
	}
	return result, nil
}

func validActionEffect(effect string) bool {
	switch effect {
	case "observed", "oriented", "changed", "unknown":
		return true
	default:
		return false
	}
}

func (r *Registry) Description(id string) (Description, bool) {
	organ, ok := r.organs[id]
	if !ok {
		return Description{}, false
	}
	return organ.description, true
}

func (r *Registry) describe(ctx context.Context, organ *installedOrgan) (Description, error) {
	var description Description
	describeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := r.invoke(describeCtx, organ, "describe", &description); err != nil {
		return Description{}, err
	}
	return description, nil
}

func (r *Registry) invoke(ctx context.Context, organ *installedOrgan, operation string, output any) error {
	return r.invokeArgs(ctx, organ, "passive-perception", output, operation)
}

func (r *Registry) invokeArgs(ctx context.Context, organ *installedOrgan, caller string, output any, arguments ...string) error {
	command := exec.CommandContext(ctx, organ.commandPath, arguments...)
	command.Dir = r.instanceRoot
	command.Env = r.environment(organ.manifest.ID, caller)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error {
		if command.Process == nil {
			return os.ErrProcessDone
		}
		err := syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	command.WaitDelay = 2 * time.Second
	encoded, err := command.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return err
	}
	decoder := json.NewDecoder(strings.NewReader(string(encoded)))
	if err := decoder.Decode(output); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return errors.New("organ returned more than one JSON value")
	}
	return nil
}

func (r *Registry) environment(id, caller string) []string {
	environment := append(os.Environ(),
		"HOMINAL_INSTANCE_ROOT="+r.instanceRoot,
		"HOMINAL_ORGAN_ID="+id,
		"HOMINAL_ORGAN_SOCKET="+r.socketPath(id),
		"HOMINAL_ORGAN_CALLER="+caller,
	)
	return append(environment, r.extraEnv...)
}

func (r *Registry) socketPath(id string) string { return filepath.Join(r.runtimeDir, id+".sock") }

func (r *Registry) organ(id string) (*installedOrgan, error) {
	organ, ok := r.organs[id]
	if !ok {
		return nil, fmt.Errorf("organ %q is not installed", id)
	}
	return organ, nil
}

func hasCapability(capabilities []string, expected string) bool {
	for _, capability := range capabilities {
		if capability == expected {
			return true
		}
	}
	return false
}

func validHealthStatus(status string) bool {
	switch status {
	case "ready", "busy", "recovering", "unavailable":
		return true
	default:
		return false
	}
}

func validActionStatus(status string) bool {
	switch status {
	case "completed", "failed", "unknown":
		return true
	default:
		return false
	}
}
