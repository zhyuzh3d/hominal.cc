package runtime

import (
	"encoding/json"
	"time"
)

const stateSchema = "hominal.runtime.state/v1"

type Config struct {
	Stage                   int                     `json:"stage"`
	Engineering             bool                    `json:"engineering"`
	GenerationKind          string                  `json:"generation_kind"`
	GenerationWindowSeconds int                     `json:"generation_window_seconds"`
	BirthBrief              string                  `json:"birth_brief"`
	Pulse                   PulseConfig             `json:"pulse"`
	ModelGateway            ModelGatewayConfig      `json:"model_gateway"`
	CognitiveResource       CognitiveResourceConfig `json:"cognitive_resource"`
	Dynamics                Dynamics                `json:"dynamics"`
	Seed                    Seed                    `json:"seed"`
}

type PulseConfig struct {
	IntervalSeconds int `json:"interval_seconds"`
	SlowScanSeconds int `json:"slow_scan_seconds"`
}

type ModelGatewayConfig struct {
	BaseURL         string `json:"base_url"`
	APIKey          string `json:"api_key"`
	MaxOutputTokens int    `json:"max_output_tokens"`
}

type CognitiveResourceConfig struct {
	PriceTableVersion        string                          `json:"price_table_version"`
	RollingHourLimitMicrousd int64                           `json:"rolling_hour_limit_microusd"`
	RollingDayLimitMicrousd  int64                           `json:"rolling_day_limit_microusd"`
	Models                   map[string]CognitiveModelConfig `json:"models"`
	InitialDefaultProfile    CognitiveProfile                `json:"initial_default_profile"`
	ValidationRetryPerFocus  int                             `json:"validation_retry_per_focus"`
	ContinuationPerFocus     int                             `json:"continuation_per_focus"`
	PaidFailureThreshold     int                             `json:"paid_failure_threshold"`
	PaidFailureWindowMinutes int                             `json:"paid_failure_window_minutes"`
	ModelProtectionMinutes   int                             `json:"model_protection_minutes"`
}

type CognitiveModelConfig struct {
	ID                            string   `json:"id"`
	InputPerMillionMicrousd       int64    `json:"input_per_million_microusd"`
	CachedInputPerMillionMicrousd int64    `json:"cached_input_per_million_microusd"`
	OutputPerMillionMicrousd      int64    `json:"output_per_million_microusd"`
	SupportedReasoningEfforts     []string `json:"supported_reasoning_efforts"`
}

type CognitiveProfile struct {
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort"`
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
	IntegrityPersistence       float64 `json:"integrity_persistence"`
	IntegrityGapGain           float64 `json:"integrity_gap_gain"`
	IntegrityRepairGain        float64 `json:"integrity_repair_gain"`
	IntegrityMirrorThreshold   float64 `json:"integrity_mirror_threshold"`
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
	Schema              string                     `json:"schema"`
	InstanceID          string                     `json:"instance_id"`
	Stage               int                        `json:"stage"`
	GenerationKind      string                     `json:"generation_kind"`
	BirthBriefEnteredAt string                     `json:"birth_brief_entered_at,omitempty"`
	T0                  string                     `json:"t0,omitempty"`
	SampleID            string                     `json:"sample_id,omitempty"`
	PlannedEnd          string                     `json:"planned_end,omitempty"`
	Revision            uint64                     `json:"revision"`
	PulseID             uint64                     `json:"pulse_id"`
	EventSeq            uint64                     `json:"event_seq"`
	ReadyAt             string                     `json:"ready_at"`
	LastPulseAt         string                     `json:"last_pulse_at"`
	LastAttentionAt     string                     `json:"last_attention_at,omitempty"`
	Body                BodySnapshot               `json:"body"`
	Perception          map[string]PerceptualTrace `json:"perception,omitempty"`
	Background          []Event                    `json:"background,omitempty"`
	Lease               *Lease                     `json:"lease,omitempty"`
	PendingAction       *ActionState               `json:"pending_action,omitempty"`
	Mentor              MentorState                `json:"mentor"`
	Usage               []UsageRecord              `json:"usage,omitempty"`
	CognitiveResource   CognitiveResourceState     `json:"cognitive_resource"`
	AffectiveState      AffectiveState             `json:"affective_state"`
	ExplorationPressure float64                    `json:"exploration_pressure"`
	SelfModelTension    float64                    `json:"self_model_tension"`
	Concerns            []Concern                  `json:"active_concerns,omitempty"`
	CurrentFocus        string                     `json:"current_focus,omitempty"`
	Commitments         []ActionCommitment         `json:"commitments,omitempty"`
	Experiences         []Experience               `json:"experiences,omitempty"`
	TotalCommitments    uint64                     `json:"total_commitments"`
	TotalExperiences    uint64                     `json:"total_experiences"`
	IntegrityDebt       float64                    `json:"integrity_debt"`
	IntegrityMirrorOpen bool                       `json:"integrity_mirror_open,omitempty"`
	Self                SelfState                  `json:"self"`
}

type PerceptualTrace struct {
	Digest           string   `json:"digest"`
	ObservedAt       string   `json:"observed_at"`
	Context          []string `json:"context,omitempty"`
	Pending          []string `json:"pending,omitempty"`
	Seen             []string `json:"seen,omitempty"`
	Saturation       float64  `json:"saturation,omitempty"`
	ExhaustedContext string   `json:"exhausted_context,omitempty"`
	ExhaustedAt      string   `json:"exhausted_at,omitempty"`
	ReturnPath       []string `json:"return_path,omitempty"`
}

type BodySnapshot struct {
	ObservedAt                     string `json:"observed_at"`
	UptimeSeconds                  int64  `json:"uptime_seconds"`
	RootFreeBytes                  uint64 `json:"root_free_bytes"`
	AgentFreeBytes                 uint64 `json:"agent_free_bytes"`
	CognitiveHourSpentMicrousd     int64  `json:"cognitive_hour_spent_microusd"`
	CognitiveHourRemainingMicrousd int64  `json:"cognitive_hour_remaining_microusd"`
	CognitiveDaySpentMicrousd      int64  `json:"cognitive_day_spent_microusd"`
	CognitiveDayRemainingMicrousd  int64  `json:"cognitive_day_remaining_microusd"`
	CognitiveResourceBand          string `json:"cognitive_resource_band"`
	CognitivePriceTableVersion     string `json:"cognitive_price_table_version"`
	NetworkAvailable               bool   `json:"network_available"`
	DesktopAvailable               bool   `json:"desktop_available"`
	ChromeAvailable                bool   `json:"chrome_available"`
	PlaywrightReady                bool   `json:"playwright_ready"`
	WechatRunning                  bool   `json:"wechat_running"`
	ClashVergeRunning              bool   `json:"clash_verge_running"`
}

type Event struct {
	ID                string          `json:"id"`
	Seq               uint64          `json:"seq"`
	Kind              string          `json:"kind"`
	Source            string          `json:"source"`
	ObservedAt        string          `json:"observed_at"`
	Summary           string          `json:"summary"`
	CorrelationID     string          `json:"correlation_id,omitempty"`
	Payload           json.RawMessage `json:"payload,omitempty"`
	Status            string          `json:"status"`
	ConcernID         string          `json:"concern_id,omitempty"`
	LastFocusedAt     string          `json:"last_focused_at,omitempty"`
	LastCommitErr     string          `json:"last_commit_error,omitempty"`
	CognitionAttempts int             `json:"cognition_attempts,omitempty"`
	WaitModel         string          `json:"wait_model,omitempty"`
}

type Lease struct {
	ID               string           `json:"id"`
	Revision         uint64           `json:"revision"`
	PulseID          uint64           `json:"pulse_id"`
	FocusID          string           `json:"focus_id"`
	StartedAt        string           `json:"started_at"`
	Profile          CognitiveProfile `json:"profile"`
	ProfileSource    string           `json:"profile_source"`
	ProfilePurpose   string           `json:"profile_purpose,omitempty"`
	RecoveryForModel string           `json:"recovery_for_model,omitempty"`
	ReservedMicrousd int64            `json:"reserved_microusd,omitempty"`
	VariationBias    string           `json:"variation_bias,omitempty"`
	VariationSeed    string           `json:"variation_seed,omitempty"`
}

type ActionState struct {
	ID           string `json:"id"`
	LeaseID      string `json:"lease_id"`
	CommitmentID string `json:"commitment_id,omitempty"`
	Kind         string `json:"kind"`
	Request      string `json:"request"`
	Status       string `json:"status"`
	StartedAt    string `json:"started_at"`
	EndedAt      string `json:"ended_at,omitempty"`
	Result       string `json:"result,omitempty"`
}

type MentorState struct {
	Received map[string]uint64 `json:"received"`
	Outbox   []MentorMessage   `json:"outbox,omitempty"`
}

type MentorMessage struct {
	MessageID    string `json:"message_id"`
	CommitmentID string `json:"commitment_id,omitempty"`
	Body         string `json:"body"`
	ReplyTo      string `json:"reply_to,omitempty"`
	Status       string `json:"status"`
	QueuedAt     string `json:"queued_at"`
	DeliveredAt  string `json:"delivered_at,omitempty"`
	RepliedAt    string `json:"replied_at,omitempty"`
}

type UsageRecord struct {
	Time              string `json:"time"`
	LeaseID           string `json:"lease_id"`
	AttentionPulseID  uint64 `json:"attention_pulse_id"`
	FocusID           string `json:"focus_id"`
	RequestedModel    string `json:"requested_model"`
	EffectiveModel    string `json:"effective_model"`
	ReasoningEffort   string `json:"reasoning_effort"`
	ProfileSource     string `json:"profile_source"`
	ProfilePurpose    string `json:"profile_purpose,omitempty"`
	InputTokens       int    `json:"input_tokens"`
	CachedInputTokens int    `json:"cached_input_tokens,omitempty"`
	OutputTokens      int    `json:"output_tokens"`
	ReasoningTokens   int    `json:"reasoning_tokens,omitempty"`
	TotalTokens       int    `json:"total_tokens"`
	ReservedMicrousd  int64  `json:"reserved_microusd"`
	ActualMicrousd    int64  `json:"actual_microusd"`
	Status            string `json:"status"`
	CostConfirmed     bool   `json:"cost_confirmed"`
	FailureCategory   string `json:"failure_category,omitempty"`
	HTTPStatus        int    `json:"http_status,omitempty"`
	RetryAfter        string `json:"retry_after,omitempty"`
	RequestID         string `json:"request_id,omitempty"`
	GatewayDate       string `json:"gateway_date,omitempty"`
}

type CognitiveResourceState struct {
	DefaultProfile  CognitiveProfile          `json:"default_profile"`
	NextProfile     *NextCognitiveProfile     `json:"next_profile,omitempty"`
	ProtectedModels map[string]ProtectedModel `json:"protected_models,omitempty"`
	Limited         *CognitiveResourceLimit   `json:"limited,omitempty"`
	LastSpend       *UsageRecord              `json:"last_spend,omitempty"`
	LastFailure     *ModelFailureFact         `json:"last_failure,omitempty"`
}

type NextCognitiveProfile struct {
	FocusID string           `json:"focus_id"`
	Purpose string           `json:"purpose"`
	Profile CognitiveProfile `json:"profile"`
	Source  string           `json:"source,omitempty"`
}

type ProtectedModel struct {
	Until           string `json:"until"`
	Reason          string `json:"reason"`
	RecoveryOffered bool   `json:"recovery_offered,omitempty"`
}

type ModelFailureFact struct {
	ObservedAt  string `json:"observed_at"`
	Model       string `json:"model"`
	Category    string `json:"category"`
	HTTPStatus  int    `json:"http_status,omitempty"`
	RetryAfter  string `json:"retry_after,omitempty"`
	RequestID   string `json:"request_id,omitempty"`
	GatewayDate string `json:"gateway_date,omitempty"`
	CostStatus  string `json:"cost_status"`
}

type CognitiveResourceLimit struct {
	FocusID          string           `json:"focus_id"`
	Profile          CognitiveProfile `json:"profile"`
	RequiredMicrousd int64            `json:"required_microusd"`
	ObservedAt       string           `json:"observed_at"`
}

type AffectiveState struct {
	Valence    float64 `json:"valence"`
	Activation float64 `json:"activation"`
	Control    float64 `json:"control"`
	Certainty  float64 `json:"certainty"`
}

type Concern struct {
	ID                string  `json:"id"`
	OriginKind        string  `json:"origin_kind,omitempty"`
	WithinConcernID   string  `json:"within_concern_id,omitempty"`
	ClosureCondition  string  `json:"closure_condition,omitempty"`
	CommitmentID      string  `json:"commitment_id,omitempty"`
	Subject           string  `json:"subject"`
	Meaning           string  `json:"meaning"`
	Strength          float64 `json:"strength"`
	Difference        float64 `json:"difference"`
	Ownership         float64 `json:"ownership"`
	Value             float64 `json:"value"`
	Urgency           float64 `json:"urgency"`
	Answerability     float64 `json:"answerability"`
	Activation        float64 `json:"activation"`
	Certainty         float64 `json:"certainty"`
	LastSourceID      string  `json:"last_source_id"`
	UpdatedAt         string  `json:"updated_at"`
	LastFocusedAt     string  `json:"last_focused_at,omitempty"`
	Resolution        string  `json:"resolution,omitempty"`
	WaitModel         string  `json:"wait_model,omitempty"`
	LastCommitErr     string  `json:"last_commit_error,omitempty"`
	CognitionAttempts int     `json:"cognition_attempts,omitempty"`
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

type EnvironmentInput struct {
	EventID string          `json:"event_id"`
	Summary string          `json:"summary"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

type RuntimeCommand struct {
	Kind        string
	Mentor      MentorInput
	Environment EnvironmentInput
	MessageID   string
	Reply       chan CommandReply
}

type CommandReply struct {
	Status int
	Body   any
}

type CognitiveRequest struct {
	Lease         Lease
	Stage         int
	Focus         Event
	Candidates    []Event
	State         State
	Config        Config
	Profile       CognitiveProfile
	VariationBias string
	VariationSeed string
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
	Kind          string `json:"kind"`
	Command       string `json:"command,omitempty"`
	Text          string `json:"text,omitempty"`
	ReplyTo       string `json:"reply_to,omitempty"`
	Intent        string `json:"intent,omitempty"`
	Prediction    string `json:"prediction,omitempty"`
	RealityCheck  string `json:"reality_check,omitempty"`
	StopCondition string `json:"stop_condition,omitempty"`
	CommitmentID  string `json:"-"`
}

type CognitiveCommit struct {
	Appraisals                 []CandidateAppraisal    `json:"appraisals"`
	FocusID                    string                  `json:"focus_id"`
	ContinuesConcernID         string                  `json:"continues_concern_id"`
	WithinConcernID            string                  `json:"within_concern_id"`
	ContributesToConcernID     string                  `json:"contributes_to_concern_id"`
	NewConcernClosureCondition string                  `json:"new_concern_closure_condition"`
	EmergingConsequence        string                  `json:"emerging_consequence"`
	ThoughtThread              string                  `json:"thought_thread"`
	Action                     CognitiveAction         `json:"action"`
	ResourceChoice             CognitiveResourceChoice `json:"resource_choice"`
	ExperienceUpdates          []ExperienceUpdate      `json:"experience_updates"`
	NarrativeUpdate            string                  `json:"narrative_update"`
}

type ActionCommitment struct {
	ID                string           `json:"id"`
	FocusID           string           `json:"focus_id"`
	ConcernID         string           `json:"concern_id,omitempty"`
	LeaseID           string           `json:"lease_id"`
	ActionID          string           `json:"action_id,omitempty"`
	ActionKind        string           `json:"action_kind"`
	Intent            string           `json:"intent"`
	Prediction        string           `json:"prediction"`
	RealityCheck      string           `json:"reality_check"`
	StopCondition     string           `json:"stop_condition,omitempty"`
	InitialDifference float64          `json:"initial_difference"`
	Profile           CognitiveProfile `json:"profile"`
	FormedAt          string           `json:"formed_at"`
	Status            string           `json:"status"`
	RealityEventID    string           `json:"reality_event_id,omitempty"`
	ExperienceID      string           `json:"experience_id,omitempty"`
}

type EndogenousValues struct {
	Continuance  float64 `json:"continuance"`
	Relatedness  float64 `json:"relatedness"`
	Expansion    float64 `json:"expansion"`
	SelfEndorsed float64 `json:"self_endorsed"`
}

type ExperienceUpdate struct {
	CommitmentID         string           `json:"commitment_id"`
	PredictionDifference float64          `json:"prediction_difference"`
	Meaning              string           `json:"meaning"`
	Values               EndogenousValues `json:"values"`
	ExperiencedCost      float64          `json:"experienced_cost"`
	Lesson               string           `json:"lesson"`
	Significance         string           `json:"significance"`
	MethodUpdate         string           `json:"method_update"`
	MethodSlot           int              `json:"method_slot"`
}

type Experience struct {
	ID                   string           `json:"id"`
	CommitmentID         string           `json:"commitment_id"`
	FocusID              string           `json:"focus_id"`
	SourceKind           string           `json:"source_kind,omitempty"`
	ActionKind           string           `json:"action_kind"`
	EnactedRequest       string           `json:"enacted_request,omitempty"`
	ObservedAt           string           `json:"observed_at"`
	PredictionDifference float64          `json:"prediction_difference"`
	RemainingDifference  float64          `json:"remaining_difference"`
	Meaning              string           `json:"meaning"`
	Values               EndogenousValues `json:"values"`
	ExperiencedCost      float64          `json:"experienced_cost"`
	Lesson               string           `json:"lesson,omitempty"`
	Significance         string           `json:"significance"`
	MethodUpdate         string           `json:"method_update,omitempty"`
	MethodSlot           int              `json:"method_slot,omitempty"`
}

type SelfState struct {
	Methods   []string `json:"methods,omitempty"`
	Narrative string   `json:"narrative,omitempty"`
	UpdatedAt string   `json:"updated_at,omitempty"`
}

type CognitiveResourceChoice struct {
	Apply           string `json:"apply"`
	Model           string `json:"model"`
	ReasoningEffort string `json:"reasoning_effort"`
	Purpose         string `json:"purpose"`
}

type WorkerNotice struct {
	LeaseID string
	Kind    string
	Payload any
	Ack     chan NoticeAck
}

type ModelReservation struct {
	Profile              CognitiveProfile `json:"profile"`
	InputTokenUpperBound int              `json:"input_token_upper_bound"`
	ReservedMicrousd     int64            `json:"reserved_microusd"`
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
