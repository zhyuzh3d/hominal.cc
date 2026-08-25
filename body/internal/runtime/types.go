package runtime

import (
	"encoding/json"
	"time"
)

const stateSchema = "hominal.runtime.state/v1"

type Config struct {
	Stage       int         `json:"stage"`
	Engineering bool        `json:"engineering"`
	Pulse       PulseConfig `json:"pulse"`
	Model       ModelConfig `json:"model"`
	Quota       QuotaConfig `json:"quota"`
	Dynamics    Dynamics    `json:"dynamics"`
	Seed        Seed        `json:"seed"`
}

type PulseConfig struct {
	IntervalSeconds int `json:"interval_seconds"`
	SlowScanSeconds int `json:"slow_scan_seconds"`
}

type ModelConfig struct {
	BaseURL         string `json:"base_url"`
	APIKey          string `json:"api_key"`
	Name            string `json:"name"`
	ReasoningEffort string `json:"reasoning_effort"`
	MaxOutputTokens int    `json:"max_output_tokens"`
}

type QuotaConfig struct {
	LimitTokens int `json:"limit_tokens"`
	WindowMins  int `json:"window_minutes"`
}

type Dynamics struct {
	AffectReturnRate           float64 `json:"affect_return_rate"`
	ConcernBaseDrive           float64 `json:"concern_base_drive"`
	ConcernUrgencyWeight       float64 `json:"concern_urgency_weight"`
	ConcernGrowthGain          float64 `json:"concern_growth_gain"`
	ConcernResolutionGain      float64 `json:"concern_resolution_gain"`
	ConcernNaturalDecayRate    float64 `json:"concern_natural_decay_rate"`
	AttentionAffectWeight      float64 `json:"attention_affect_weight"`
	AttentionExplorationWeight float64 `json:"attention_exploration_weight"`
	AttentionNoveltyWeight     float64 `json:"attention_novelty_weight"`
	AttentionCostWeight        float64 `json:"attention_cost_weight"`
	AttentionThreshold         float64 `json:"attention_threshold"`
	AttentionCandidateLimit    int     `json:"attention_candidate_limit"`
	AttentionRevisitSeconds    int     `json:"attention_revisit_seconds"`
	ExplorationIdleGrowth      float64 `json:"exploration_idle_growth"`
	ExplorationUnknownGrowth   float64 `json:"exploration_unknown_growth"`
	ExplorationRelief          float64 `json:"exploration_relief"`
}

type Seed struct {
	Name                        string  `json:"name"`
	Gender                      string  `json:"gender"`
	Age                         int     `json:"age"`
	LifeForm                    string  `json:"life_form"`
	SocialOpennessBias          float64 `json:"social_openness_bias"`
	ExplorationBias             float64 `json:"exploration_bias"`
	ConstructiveRecoveryBias    float64 `json:"constructive_recovery_bias"`
	ContinuanceSensitivity      float64 `json:"continuance_sensitivity"`
	RelatednessSensitivity      float64 `json:"relatedness_sensitivity"`
	ExpansionSensitivity        float64 `json:"expansion_sensitivity"`
	RealityIntegritySensitivity float64 `json:"reality_integrity_sensitivity"`
	SemanticText                string  `json:"semantic_text"`
}

type State struct {
	Schema              string         `json:"schema"`
	InstanceID          string         `json:"instance_id"`
	Stage               int            `json:"stage"`
	Revision            uint64         `json:"revision"`
	PulseID             uint64         `json:"pulse_id"`
	EventSeq            uint64         `json:"event_seq"`
	ReadyAt             string         `json:"ready_at"`
	LastPulseAt         string         `json:"last_pulse_at"`
	LastAttentionAt     string         `json:"last_attention_at,omitempty"`
	Body                BodySnapshot   `json:"body"`
	Background          []Event        `json:"background,omitempty"`
	Lease               *Lease         `json:"lease,omitempty"`
	PendingAction       *ActionState   `json:"pending_action,omitempty"`
	Mentor              MentorState    `json:"mentor"`
	Usage               []UsageRecord  `json:"usage,omitempty"`
	AffectiveState      AffectiveState `json:"affective_state"`
	ExplorationPressure float64        `json:"exploration_pressure"`
	Concerns            []Concern      `json:"active_concerns,omitempty"`
	CurrentFocus        string         `json:"current_focus,omitempty"`
}

type BodySnapshot struct {
	ObservedAt       string `json:"observed_at"`
	UptimeSeconds    int64  `json:"uptime_seconds"`
	RootFreeBytes    uint64 `json:"root_free_bytes"`
	AgentFreeBytes   uint64 `json:"agent_free_bytes"`
	QuotaUsedTokens  int    `json:"quota_used_tokens"`
	QuotaRemaining   int    `json:"quota_remaining_tokens"`
	NetworkAvailable bool   `json:"network_available"`
	DesktopAvailable bool   `json:"desktop_available"`
	ChromeAvailable  bool   `json:"chrome_available"`
	PlaywrightReady  bool   `json:"playwright_ready"`
	WechatRunning    bool   `json:"wechat_running"`
}

type Event struct {
	ID            string          `json:"id"`
	Seq           uint64          `json:"seq"`
	Kind          string          `json:"kind"`
	Source        string          `json:"source"`
	ObservedAt    string          `json:"observed_at"`
	Summary       string          `json:"summary"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	Status        string          `json:"status"`
	ConcernID     string          `json:"concern_id,omitempty"`
	LastFocusedAt string          `json:"last_focused_at,omitempty"`
	LastCommitErr string          `json:"last_commit_error,omitempty"`
}

type Lease struct {
	ID        string `json:"id"`
	Revision  uint64 `json:"revision"`
	PulseID   uint64 `json:"pulse_id"`
	FocusID   string `json:"focus_id"`
	StartedAt string `json:"started_at"`
}

type ActionState struct {
	ID        string `json:"id"`
	LeaseID   string `json:"lease_id"`
	Kind      string `json:"kind"`
	Request   string `json:"request"`
	Status    string `json:"status"`
	StartedAt string `json:"started_at"`
	EndedAt   string `json:"ended_at,omitempty"`
	Result    string `json:"result,omitempty"`
}

type MentorState struct {
	Received map[string]uint64 `json:"received"`
	Outbox   []MentorMessage   `json:"outbox,omitempty"`
}

type MentorMessage struct {
	MessageID   string `json:"message_id"`
	Body        string `json:"body"`
	ReplyTo     string `json:"reply_to,omitempty"`
	Status      string `json:"status"`
	QueuedAt    string `json:"queued_at"`
	DeliveredAt string `json:"delivered_at,omitempty"`
	RepliedAt   string `json:"replied_at,omitempty"`
}

type UsageRecord struct {
	Time         string `json:"time"`
	Model        string `json:"model"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	TotalTokens  int    `json:"total_tokens"`
	Status       string `json:"status"`
}

type AffectiveState struct {
	Valence    float64 `json:"valence"`
	Activation float64 `json:"activation"`
	Control    float64 `json:"control"`
	Certainty  float64 `json:"certainty"`
}

type Concern struct {
	ID            string  `json:"id"`
	OriginKind    string  `json:"origin_kind,omitempty"`
	Subject       string  `json:"subject"`
	Meaning       string  `json:"meaning"`
	Strength      float64 `json:"strength"`
	Difference    float64 `json:"difference"`
	Ownership     float64 `json:"ownership"`
	Value         float64 `json:"value"`
	Urgency       float64 `json:"urgency"`
	Answerability float64 `json:"answerability"`
	Activation    float64 `json:"activation"`
	Certainty     float64 `json:"certainty"`
	LastSourceID  string  `json:"last_source_id"`
	UpdatedAt     string  `json:"updated_at"`
	LastFocusedAt string  `json:"last_focused_at,omitempty"`
	Resolution    string  `json:"resolution,omitempty"`
}

type JournalRecord struct {
	Seq           uint64 `json:"seq"`
	Time          string `json:"time"`
	Kind          string `json:"kind"`
	InstanceID    string `json:"instance_id"`
	Revision      uint64 `json:"revision"`
	CorrelationID string `json:"correlation_id,omitempty"`
	Payload       any    `json:"payload,omitempty"`
}

type MentorInput struct {
	MessageID string `json:"message_id"`
	Body      string `json:"body"`
	ReplyTo   string `json:"reply_to,omitempty"`
}

type RuntimeCommand struct {
	Kind      string
	Mentor    MentorInput
	MessageID string
	Reply     chan CommandReply
}

type CommandReply struct {
	Status int
	Body   any
}

type CognitiveRequest struct {
	Lease      Lease
	Stage      int
	Focus      Event
	Candidates []Event
	State      State
	Config     Config
}

type CognitiveResult struct {
	LeaseID string
	FocusID string
	Text    string
	Stage4  *CognitiveCommit
	Error   error
}

type CandidateAppraisal struct {
	CandidateID   string  `json:"candidate_id"`
	Meaning       string  `json:"meaning"`
	Difference    float64 `json:"d"`
	Ownership     float64 `json:"o"`
	Value         float64 `json:"v"`
	Urgency       float64 `json:"u"`
	Answerability float64 `json:"a"`
	Certainty     float64 `json:"certainty"`
	Resolution    string  `json:"resolution"`
}

type CognitiveAction struct {
	Kind    string `json:"kind"`
	Command string `json:"command,omitempty"`
	Text    string `json:"text,omitempty"`
	ReplyTo string `json:"reply_to,omitempty"`
}

type CognitiveCommit struct {
	Appraisals    []CandidateAppraisal `json:"appraisals"`
	FocusID       string               `json:"focus_id"`
	ThoughtThread string               `json:"thought_thread"`
	Action        CognitiveAction      `json:"action"`
}

type WorkerNotice struct {
	LeaseID string
	Kind    string
	Payload any
	Ack     chan NoticeAck
}

type ShellActionRequest struct {
	ActionID       string
	Command        string
	TimeoutSeconds int
}

type ActionResultNotice struct {
	ActionID string
	Result   string
}

type MentorActionRequest struct {
	ActionID string
	Text     string
	ReplyTo  string
}

type NoticeAck struct {
	Accepted bool
	Output   string
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
