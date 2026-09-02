package organ

import "encoding/json"

const (
	ManifestSchema     = "hominal.organ-manifest/v1"
	DescriptionSchema  = "hominal.organ-description/v1"
	HealthSchema       = "hominal.organ-health/v1"
	ObservationSchema  = "hominal.organ-observation/v1"
	OrientationSchema  = "hominal.organ-orientation/v1"
	ActionSchema       = "hominal.organ-action/v1"
	ActionResultSchema = "hominal.organ-action-result/v1"
)

type Manifest struct {
	Schema  string `json:"schema"`
	ID      string `json:"id"`
	Command string `json:"command"`
	Daemon  bool   `json:"daemon"`
}

type Description struct {
	Schema       string   `json:"schema"`
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Command      string   `json:"command"`
	Capabilities []string `json:"capabilities"`
	Operations   []string `json:"operations,omitempty"`
	Guidance     string   `json:"guidance"`
}

type Health struct {
	Schema    string `json:"schema"`
	ID        string `json:"id"`
	Status    string `json:"status"`
	Accepting bool   `json:"accepting"`
	InFlight  int    `json:"in_flight"`
	Queued    int    `json:"queued"`
}

type Object struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

type Observation struct {
	Schema     string                     `json:"schema"`
	OrganID    string                     `json:"organ_id"`
	SurfaceID  string                     `json:"surface_id"`
	ObservedAt string                     `json:"observed_at"`
	Context    []string                   `json:"context,omitempty"`
	Objects    []Object                   `json:"objects,omitempty"`
	Facts      map[string]json.RawMessage `json:"facts,omitempty"`
}

type Orientation struct {
	Schema     string `json:"schema"`
	OrganID    string `json:"organ_id"`
	Status     string `json:"status"`
	ObservedAt string `json:"observed_at"`
	Detail     string `json:"detail,omitempty"`
}

type ActionRequest struct {
	Schema              string `json:"schema"`
	ActionID            string `json:"action_id"`
	Operation           string `json:"operation"`
	Input               string `json:"input"`
	TimeoutMilliseconds int    `json:"timeout_milliseconds"`
}

type ActionResult struct {
	Schema     string `json:"schema"`
	OrganID    string `json:"organ_id"`
	ActionID   string `json:"action_id"`
	Status     string `json:"status"`
	ObservedAt string `json:"observed_at"`
	Summary    string `json:"summary"`
	Output     string `json:"output,omitempty"`
}

type Snapshot struct {
	Name         string   `json:"name"`
	Command      string   `json:"command"`
	Capabilities []string `json:"capabilities,omitempty"`
	Operations   []string `json:"operations,omitempty"`
	Guidance     string   `json:"guidance,omitempty"`
	Status       string   `json:"status"`
	Accepting    bool     `json:"accepting"`
}
