package projector

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

const (
	SemanticsSchema = "gooo.bounded-observational-equivalence/semantics/v1"
	ScenarioSchema  = "gooo.bounded-observational-equivalence/scenario/v1"
	CorpusSchema    = "gooo.bounded-observational-equivalence/corpus/v1"
	OutcomeSchema   = "gooo.bounded-observational-equivalence/outcome/v1"
	CompareSchema   = "gooo.bounded-observational-equivalence/comparison/v1"
	NormalClass     = "normal"
	UnknownClass    = "unknown"
	RefutedClass    = "refuted"
	StatusClosed    = Status("CLOSED")
	StatusUnknown   = Status("UNKNOWN")
	StatusRefuted   = Status("REFUTED")
)

type Status string

type Semantics struct {
	Schema                 string            `json:"schema"`
	Authority              string            `json:"authority"`
	Language               string            `json:"language"`
	Version                string            `json:"version"`
	ValueTypes             []string          `json:"value_types"`
	Operations             []string          `json:"operations"`
	Effects                []string          `json:"effects"`
	ComparisonVectors      []string          `json:"comparison_vectors"`
	StatusPrecedence       []Status          `json:"status_precedence"`
	UnknownFields          []string          `json:"unknown_fields"`
	DefaultBounds          Bounds            `json:"default_bounds"`
	Normalization           Normalization     `json:"normalization"`
	RuntimeContract        RuntimeContract   `json:"runtime_contract"`
}

type RuntimeContract struct {
	RepositoryWrites       int    `json:"repository_writes"`
	LocalTestExecutions    int    `json:"local_test_executions"`
	CrossProjectGates      int    `json:"cross_project_required_gates"`
	OutputBoundary         string `json:"output_boundary"`
}

type Bounds struct {
	MaxSteps  int `json:"max_steps"`
	MaxEvents int `json:"max_events"`
}

type Normalization struct {
	ValueEncoding string   `json:"value_encoding"`
	StateOrder    string   `json:"state_order"`
	EventFields   []string `json:"event_fields"`
	Digest        string   `json:"digest"`
}

type Scenario struct {
	Schema           string              `json:"schema"`
	CaseID           string              `json:"case_id"`
	Class            string              `json:"class"`
	Variant          string              `json:"variant"`
	Inputs           map[string]Value    `json:"inputs"`
	ObservableState  []string            `json:"observable_state"`
	InitialState     map[string]Value    `json:"initial_state"`
	ExpectedState    map[string]Value    `json:"expected_state"`
	DeclaredTrace    []DeclaredEvent     `json:"declared_trace"`
	AllowedEffects   []string            `json:"allowed_effects"`
	Bounds           Bounds              `json:"bounds"`
	Normalization    Normalization       `json:"normalization"`
	Operations       []Operation         `json:"operations"`
	Result           Expr                `json:"result"`
}

type Value struct {
	Type   string  `json:"type"`
	Int    *int64  `json:"int,omitempty"`
	Bool   *bool   `json:"bool,omitempty"`
	String *string `json:"string,omitempty"`
}

type Expr struct {
	Kind  string  `json:"kind"`
	Name  string  `json:"name,omitempty"`
	Value *Value  `json:"value,omitempty"`
}

type Operation struct {
	Op       string `json:"op"`
	Field    string `json:"field,omitempty"`
	Effect   string `json:"effect,omitempty"`
	Value    *Expr  `json:"value,omitempty"`
	Delta    int64  `json:"delta,omitempty"`
	Count    int    `json:"count,omitempty"`
}

type DeclaredEvent struct {
	Effect  string `json:"effect"`
	Subject string `json:"subject"`
	Value   Value  `json:"value"`
}

type Corpus struct {
	Schema string        `json:"schema"`
	Cases  []CorpusCase  `json:"cases"`
}

type CorpusCase struct {
	ID             string `json:"id"`
	Program        string `json:"program"`
	Class          string `json:"class"`
	ExpectedStatus Status `json:"expected_status"`
	Variant        string `json:"variant"`
	Replay         bool   `json:"replay"`
}

type UnknownDetail struct {
	Stage         string   `json:"stage"`
	Step          string   `json:"step"`
	Reason        string   `json:"reason"`
	UnknownClass  string   `json:"unknown_class"`
	NextOperation string   `json:"next_operation"`
	BlockedBy     []string `json:"blocked_by"`
}

type Witness struct {
	Kind     string `json:"kind"`
	Stage    string `json:"stage"`
	Step     string `json:"step"`
	Item     string `json:"item,omitempty"`
	Reason   string `json:"reason"`
	Expected any    `json:"expected,omitempty"`
	Actual   any    `json:"actual,omitempty"`
}

type StateCell struct {
	Name  string `json:"name"`
	Value Value  `json:"value"`
}

type Event struct {
	Ordinal int    `json:"ordinal"`
	Effect  string `json:"effect"`
	Subject string `json:"subject"`
	Value   Value  `json:"value"`
}

type EffectCell struct {
	Ordinal int    `json:"ordinal"`
	Effect  string `json:"effect"`
}

type DeterminismCell struct {
	TraceAvailable bool   `json:"trace_available"`
	Deterministic  bool   `json:"deterministic"`
	Digest         string `json:"digest"`
}

type Outcome struct {
	Schema         string          `json:"schema"`
	CaseID         string          `json:"case_id"`
	Status         Status          `json:"status"`
	Value          *Value          `json:"value,omitempty"`
	State          []StateCell     `json:"state"`
	Trace          []Event         `json:"trace"`
	Effects        []EffectCell    `json:"effects"`
	TraceAvailable bool            `json:"trace_available"`
	Deterministic  bool            `json:"deterministic"`
	Digest         string          `json:"digest"`
	Reason         string          `json:"reason,omitempty"`
	Unknown        *UnknownDetail  `json:"unknown,omitempty"`
	Witnesses      []Witness       `json:"witnesses,omitempty"`
}

type Normalized struct {
	Value       *Value          `json:"value,omitempty"`
	State       []StateCell     `json:"state"`
	Events      []Event         `json:"events"`
	Effects     []EffectCell    `json:"effects"`
	Determinism DeterminismCell `json:"determinism"`
}

type VectorComparison struct {
	Vector    string          `json:"vector"`
	State     Status          `json:"state"`
	Reference any             `json:"reference"`
	Generated any             `json:"generated"`
	Reason    string          `json:"reason"`
	Witnesses []Witness       `json:"witnesses,omitempty"`
	Unknown   []UnknownDetail `json:"unknown,omitempty"`
}

type Comparison struct {
	Schema            string             `json:"schema"`
	CaseID            string             `json:"case_id"`
	Verdict           Status             `json:"verdict"`
	StatusPrecedence  []Status           `json:"status_precedence"`
	ReferenceTrace    Outcome            `json:"reference_trace"`
	GeneratedGoTrace  Outcome            `json:"generated_go_trace"`
	Normalized        []VectorComparison `json:"normalized_comparison"`
	MismatchWitnesses []Witness          `json:"mismatch_witnesses,omitempty"`
	UnknownWitnesses  []UnknownDetail    `json:"unknown_witnesses,omitempty"`
	HumanReport       string             `json:"human_report"`
}

func IntValue(v int64) Value { return Value{Type: "int", Int: &v} }
func BoolValue(v bool) Value { return Value{Type: "bool", Bool: &v} }
func StringValue(v string) Value { return Value{Type: "string", String: &v} }

func (v Value) Validate() error {
	switch v.Type {
	case "int":
		if v.Int == nil || v.Bool != nil || v.String != nil {
			return errors.New("int value must contain only int")
		}
	case "bool":
		if v.Bool == nil || v.Int != nil || v.String != nil {
			return errors.New("bool value must contain only bool")
		}
	case "string":
		if v.String == nil || v.Int != nil || v.Bool != nil {
			return errors.New("string value must contain only string")
		}
	default:
		return fmt.Errorf("unsupported value type %q", v.Type)
	}
	return nil
}

func (v Value) Equal(other Value) bool {
	left, _ := json.Marshal(v)
	right, _ := json.Marshal(other)
	return string(left) == string(right)
}

func canonicalJSON(value any) ([]byte, error) {
	return json.Marshal(value)
}

func digestCanonical(value any) (string, error) {
	data, err := canonicalJSON(value)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func (u UnknownDetail) Validate() error {
	if u.Stage == "" || u.Step == "" || u.Reason == "" || u.UnknownClass == "" || u.NextOperation == "" || len(u.BlockedBy) == 0 {
		return errors.New("UNKNOWN must preserve stage, step, reason, unknown_class, next_operation, and blocked_by")
	}
	return nil
}

func (s Status) Valid() bool {
	return s == StatusClosed || s == StatusUnknown || s == StatusRefuted
}
