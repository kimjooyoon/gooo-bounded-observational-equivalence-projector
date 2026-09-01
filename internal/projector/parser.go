package projector

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

var requiredUnknownFields = []string{"stage", "step", "reason", "unknown_class", "next_operation", "blocked_by"}
var requiredVectors = []string{"value", "event", "effect", "determinism"}
var requiredStatuses = []Status{StatusRefuted, StatusUnknown, StatusClosed}
var requiredOperations = []string{"set_state", "add_state", "emit", "result", "unsupported_effect", "nondeterministic", "repeat"}
var requiredEffects = []string{"state.write", "event.emit", "audit.record"}
var requiredEventFields = []string{"ordinal", "effect", "subject", "value"}

func LoadSemantics(path string) (Semantics, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Semantics{}, nil, err
	}
	semantics, err := ParseSemantics(raw)
	if err != nil {
		return Semantics{}, nil, fmt.Errorf("parse semantics %s: %w", path, err)
	}
	return semantics, raw, nil
}

func ParseSemantics(raw []byte) (Semantics, error) {
	var semantics Semantics
	if err := decodeJSON(raw, &semantics); err != nil {
		return Semantics{}, fmt.Errorf("semantic input is malformed: %w", err)
	}
	if err := semantics.Validate(); err != nil {
		return Semantics{}, err
	}
	return semantics, nil
}

func (s Semantics) Validate() error {
	if s.Schema != SemanticsSchema || s.Authority != "metacode" || s.Language != "Gooo" || s.Version == "" {
		return errors.New("semantics must be authoritative metacode Gooo semantics v1")
	}
	if !sameStrings(s.ValueTypes, []string{"int", "bool", "string"}) {
		return errors.New("semantics value_types are not the fixed typed-value vocabulary")
	}
	if !sameStrings(s.Operations, requiredOperations) || !sameStrings(s.Effects, requiredEffects) {
		return errors.New("semantics operation or effect vocabulary is not fixed")
	}
	if !sameStrings(s.ComparisonVectors, requiredVectors) || !sameStatuses(s.StatusPrecedence, requiredStatuses) {
		return errors.New("semantics comparison vectors or precedence is not fixed")
	}
	if !sameStrings(s.UnknownFields, requiredUnknownFields) {
		return errors.New("semantics UNKNOWN fields are not fixed")
	}
	if s.DefaultBounds.MaxSteps < 1 || s.DefaultBounds.MaxEvents < 1 {
		return errors.New("semantics default bounds must be positive")
	}
	if s.Normalization.ValueEncoding != "typed-json" || s.Normalization.StateOrder != "declared" || s.Normalization.Digest != "sha256-canonical-json" || !sameStrings(s.Normalization.EventFields, requiredEventFields) {
		return errors.New("semantics normalization policy is not fixed")
	}
	if s.RuntimeContract.RepositoryWrites != 0 || s.RuntimeContract.LocalTestExecutions != 0 || s.RuntimeContract.CrossProjectGates != 0 || s.RuntimeContract.OutputBoundary != "caller-owned-output-or-temp-only" {
		return errors.New("runtime contract permits an out-of-bound authority")
	}
	return nil
}

func LoadScenario(path string, semantics Semantics) (Scenario, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Scenario{}, nil, err
	}
	scenario, err := ParseScenario(raw, semantics)
	if err != nil {
		return Scenario{}, nil, fmt.Errorf("parse scenario %s: %w", path, err)
	}
	return scenario, raw, nil
}

func ParseScenario(raw []byte, semantics Semantics) (Scenario, error) {
	var scenario Scenario
	if err := decodeJSON(raw, &scenario); err != nil {
		return Scenario{}, fmt.Errorf("scenario input is malformed: %w", err)
	}
	if err := scenario.Validate(semantics); err != nil {
		return Scenario{}, err
	}
	return scenario, nil
}

func (s Scenario) Validate(semantics Semantics) error {
	if s.Schema != ScenarioSchema || s.CaseID == "" || s.Variant == "" {
		return errors.New("scenario header is incomplete")
	}
	if s.Class != NormalClass && s.Class != UnknownClass && s.Class != RefutedClass {
		return fmt.Errorf("scenario %q has invalid class %q", s.CaseID, s.Class)
	}
	if len(s.ObservableState) == 0 {
		return errors.New("scenario must declare observable_state")
	}
	stateNames := make(map[string]bool, len(s.ObservableState))
	for _, name := range s.ObservableState {
		if name == "" || stateNames[name] {
			return fmt.Errorf("scenario observable state has duplicate or empty name %q", name)
		}
		stateNames[name] = true
	}
	if err := validateValueMap(s.Inputs, "input"); err != nil {
		return err
	}
	if err := validateValueMap(s.InitialState, "initial state"); err != nil {
		return err
	}
	if err := validateValueMap(s.ExpectedState, "expected state"); err != nil {
		return err
	}
	for _, name := range s.ObservableState {
		if _, ok := s.InitialState[name]; !ok {
			return fmt.Errorf("initial state omits observable field %q", name)
		}
		if _, ok := s.ExpectedState[name]; !ok {
			return fmt.Errorf("expected state omits observable field %q", name)
		}
	}
	if len(s.InitialState) != len(s.ObservableState) || len(s.ExpectedState) != len(s.ObservableState) {
		return errors.New("state maps must contain exactly the declared observable state")
	}
	if err := validateEffects(s.AllowedEffects, semantics.Effects); err != nil {
		return err
	}
	if s.Bounds.MaxSteps < 1 || s.Bounds.MaxEvents < 1 || s.Bounds.MaxSteps > 1024 || s.Bounds.MaxEvents > 1024 {
		return errors.New("scenario bounds must be positive and bounded")
	}
	if s.Normalization.ValueEncoding != semantics.Normalization.ValueEncoding || s.Normalization.StateOrder != semantics.Normalization.StateOrder || s.Normalization.Digest != semantics.Normalization.Digest || !sameStrings(s.Normalization.EventFields, requiredEventFields) {
		return errors.New("scenario normalization policy does not match semantics")
	}
	for _, operation := range s.Operations {
		if err := operation.Validate(semantics, stateNames, s.Inputs); err != nil {
			return err
		}
	}
	if len(s.Operations) == 0 {
		return errors.New("scenario must contain at least one operation")
	}
	if err := s.Result.Validate(stateNames, s.Inputs); err != nil {
		return fmt.Errorf("result: %w", err)
	}
	for _, event := range s.DeclaredTrace {
		if event.Effect == "" || event.Value.Validate() != nil {
			return errors.New("declared_trace contains an invalid event")
		}
	}
	return nil
}

func (o Operation) Validate(semantics Semantics, stateNames map[string]bool, inputs map[string]Value) error {
	switch o.Op {
	case "set_state":
		if !stateNames[o.Field] || o.Value == nil {
			return errors.New("set_state requires a declared field and value")
		}
		return o.Value.Validate(stateNames, inputs)
	case "add_state":
		if !stateNames[o.Field] {
			return errors.New("add_state requires a declared field")
		}
		if o.Value != nil {
			return errors.New("add_state does not accept value")
		}
		return nil
	case "emit":
		if o.Effect == "" || o.Value == nil {
			return errors.New("emit requires effect and value")
		}
		return o.Value.Validate(stateNames, inputs)
	case "unsupported_effect":
		if o.Effect == "" {
			return errors.New("unsupported_effect requires effect")
		}
		return nil
	case "nondeterministic":
		return nil
	case "repeat":
		if o.Count < 1 || o.Effect == "" || o.Value == nil {
			return errors.New("repeat requires positive count, effect, and value")
		}
		return o.Value.Validate(stateNames, inputs)
	case "result":
		return errors.New("result is declared by the result field, not operations")
	default:
		return fmt.Errorf("unsupported operation %q", o.Op)
	}
}

func (e Expr) Validate(stateNames map[string]bool, inputs map[string]Value) error {
	switch e.Kind {
	case "literal":
		if e.Value == nil {
			return errors.New("literal expression is missing value")
		}
		return e.Value.Validate()
	case "input":
		if _, ok := inputs[e.Name]; !ok {
			return fmt.Errorf("expression references unknown input %q", e.Name)
		}
	case "state":
		if !stateNames[e.Name] {
			return fmt.Errorf("expression references unknown state %q", e.Name)
		}
	default:
		return fmt.Errorf("unsupported expression kind %q", e.Kind)
	}
	return nil
}

func LoadCorpus(path string, semantics Semantics) (Corpus, []byte, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Corpus{}, nil, err
	}
	corpus, err := ParseCorpus(raw, semantics)
	if err != nil {
		return Corpus{}, nil, fmt.Errorf("parse corpus %s: %w", path, err)
	}
	return corpus, raw, nil
}

func ParseCorpus(raw []byte, semantics Semantics) (Corpus, error) {
	var corpus Corpus
	if err := decodeJSON(raw, &corpus); err != nil {
		return Corpus{}, fmt.Errorf("corpus input is malformed: %w", err)
	}
	if corpus.Schema != CorpusSchema || len(corpus.Cases) == 0 {
		return Corpus{}, errors.New("corpus header is incomplete")
	}
	seen := map[string]bool{}
	for _, item := range corpus.Cases {
		if item.ID == "" || item.Program == "" || seen[item.ID] {
			return Corpus{}, fmt.Errorf("corpus has duplicate or incomplete case %q", item.ID)
		}
		seen[item.ID] = true
		if item.ExpectedStatus != StatusClosed && item.ExpectedStatus != StatusUnknown && item.ExpectedStatus != StatusRefuted {
			return Corpus{}, fmt.Errorf("corpus case %q has invalid expected status", item.ID)
		}
		if item.Class != NormalClass && item.Class != UnknownClass && item.Class != RefutedClass {
			return Corpus{}, fmt.Errorf("corpus case %q has invalid class", item.ID)
		}
		if item.Variant == "" {
			return Corpus{}, fmt.Errorf("corpus case %q has empty variant", item.ID)
		}
	}
	_ = semantics
	return corpus, nil
}

func validateValueMap(values map[string]Value, label string) error {
	for name, value := range values {
		if name == "" {
			return fmt.Errorf("%s has an empty name", label)
		}
		if err := value.Validate(); err != nil {
			return fmt.Errorf("%s %q: %w", label, name, err)
		}
	}
	return nil
}

func validateEffects(actual, vocabulary []string) error {
	seen := map[string]bool{}
	for _, effect := range actual {
		if seen[effect] {
			return fmt.Errorf("allowed effects contain duplicate %q", effect)
		}
		seen[effect] = true
		found := false
		for _, declared := range vocabulary {
			if effect == declared {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("allowed effect %q is not in semantic vocabulary", effect)
		}
	}
	return nil
}

func sameStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func sameStatuses(left, right []Status) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func decodeJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("trailing JSON value")
		}
		return err
	}
	return nil
}
