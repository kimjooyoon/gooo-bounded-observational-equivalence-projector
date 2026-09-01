package projector

import (
	"fmt"
)

func EvaluateReference(scenario Scenario, semantics Semantics) Outcome {
	return execute(scenario, semantics, "none")
}

func EvaluateGenerated(scenario Scenario, semantics Semantics, variant string) Outcome {
	outcome := execute(scenario, semantics, variant)
	if outcome.Status != StatusClosed {
		return outcome
	}
	switch variant {
	case "none":
		return outcome
	case "value-divergence":
		if outcome.Value != nil && outcome.Value.Type == "int" && outcome.Value.Int != nil {
			changed := *outcome.Value.Int + 1
			outcome.Value = valuePointer(IntValue(changed))
		}
		finalizeOutcome(&outcome)
	case "event-divergence":
		if len(outcome.Trace) > 0 && outcome.Trace[0].Value.Type == "int" && outcome.Trace[0].Value.Int != nil {
			changed := *outcome.Trace[0].Value.Int + 1
			outcome.Trace[0].Value = IntValue(changed)
		}
		finalizeOutcome(&outcome)
	case "effect-divergence":
		if len(outcome.Trace) > 0 {
			outcome.Trace[0].Effect = "audit.record"
		}
		finalizeOutcome(&outcome)
	case "missing-trace":
		outcome.Trace = nil
		outcome.Effects = nil
		outcome.TraceAvailable = false
		outcome.Deterministic = false
		finalizeOutcome(&outcome)
		outcome.TraceAvailable = false
		outcome.Deterministic = false
	default:
		outcome.Status = StatusUnknown
		outcome.Reason = fmt.Sprintf("generated variant %q is not defined by the semantic contract", variant)
		outcome.Unknown = &UnknownDetail{
			Stage: "generate", Step: "candidate-variant", Reason: outcome.Reason,
			UnknownClass: "UNSUPPORTED_VARIANT", NextOperation: "declare a supported candidate variant",
			BlockedBy: []string{variant},
		}
		outcome.TraceAvailable = false
		outcome.Deterministic = false
		finalizeOutcome(&outcome)
	}
	return outcome
}

func execute(scenario Scenario, semantics Semantics, variant string) Outcome {
	outcome := Outcome{
		Schema: OutcomeSchema, CaseID: scenario.CaseID, Status: StatusClosed,
		Trace: make([]Event, 0), TraceAvailable: true, Deterministic: true,
	}
	state := cloneValues(scenario.InitialState)
	steps := 0
	for index, operation := range scenario.Operations {
		cost := 1
		if operation.Op == "repeat" {
			cost = operation.Count
		}
		if steps+cost > scenario.Bounds.MaxSteps {
			return unknownOutcome(outcome, state, scenario, UnknownDetail{
				Stage: "evaluate", Step: "bound-enforcement",
				Reason:       fmt.Sprintf("operation %d would consume %d steps beyond max_steps=%d", index, cost, scenario.Bounds.MaxSteps),
				UnknownClass: "BOUND_EXHAUSTION", NextOperation: "raise the declared bound or reduce the scenario",
				BlockedBy: []string{fmt.Sprintf("operation_index=%d", index), fmt.Sprintf("max_steps=%d", scenario.Bounds.MaxSteps)},
			})
		}
		if operation.Op == "unsupported_effect" {
			return unknownOutcome(outcome, state, scenario, UnknownDetail{
				Stage: "evaluate", Step: "effect-vocabulary",
				Reason:       fmt.Sprintf("effect %q is outside the supported semantic effect vocabulary", operation.Effect),
				UnknownClass: "UNSUPPORTED_EFFECT", NextOperation: "declare the effect vocabulary and its observation rule",
				BlockedBy: []string{operation.Effect},
			})
		}
		if operation.Op == "nondeterministic" {
			return unknownOutcome(outcome, state, scenario, UnknownDetail{
				Stage: "evaluate", Step: "determinism-source",
				Reason:       "the scenario declares a nondeterministic source, so one bounded replay is not a proof",
				UnknownClass: "NONDETERMINISTIC_SOURCE", NextOperation: "replace the source with a declared deterministic input",
				BlockedBy: []string{fmt.Sprintf("operation_index=%d", index)},
			})
		}
		steps += cost
		switch operation.Op {
		case "set_state":
			if issue := requireAllowedEffect(scenario, "state.write", index); issue != nil {
				return refutedOutcome(outcome, state, scenario, *issue)
			}
			value, err := evalExpr(operation.Value, state, scenario.Inputs)
			if err != nil {
				return refutedOutcome(outcome, state, scenario, Witness{Kind: "invalid-value", Stage: "evaluate", Step: "state-transition", Item: fmt.Sprintf("operation_index=%d", index), Reason: err.Error()})
			}
			current := state[operation.Field]
			if value.Type != current.Type {
				return refutedOutcome(outcome, state, scenario, Witness{Kind: "state-type-mismatch", Stage: "evaluate", Step: "state-transition", Item: operation.Field, Reason: "state assignment changes the declared type", Expected: current.Type, Actual: value.Type})
			}
			state[operation.Field] = value
			if issue := appendEvent(&outcome, scenario, "state.write", operation.Field, value, index); issue != nil {
				return refutedOutcome(outcome, state, scenario, *issue)
			}
		case "add_state":
			if issue := requireAllowedEffect(scenario, "state.write", index); issue != nil {
				return refutedOutcome(outcome, state, scenario, *issue)
			}
			current := state[operation.Field]
			if current.Type != "int" || current.Int == nil {
				return refutedOutcome(outcome, state, scenario, Witness{Kind: "state-type-mismatch", Stage: "evaluate", Step: "state-transition", Item: operation.Field, Reason: "add_state requires an int observable field", Expected: "int", Actual: current.Type})
			}
			changed := *current.Int + operation.Delta
			state[operation.Field] = IntValue(changed)
			if issue := appendEvent(&outcome, scenario, "state.write", operation.Field, IntValue(changed), index); issue != nil {
				return refutedOutcome(outcome, state, scenario, *issue)
			}
		case "emit":
			if issue := requireAllowedEffect(scenario, operation.Effect, index); issue != nil {
				return refutedOutcome(outcome, state, scenario, *issue)
			}
			value, err := evalExpr(operation.Value, state, scenario.Inputs)
			if err != nil {
				return refutedOutcome(outcome, state, scenario, Witness{Kind: "invalid-value", Stage: "evaluate", Step: "event-transition", Item: fmt.Sprintf("operation_index=%d", index), Reason: err.Error()})
			}
			if issue := appendEvent(&outcome, scenario, operation.Effect, "", value, index); issue != nil {
				return refutedOutcome(outcome, state, scenario, *issue)
			}
		case "repeat":
			if len(outcome.Trace)+operation.Count > scenario.Bounds.MaxEvents {
				return unknownOutcome(outcome, state, scenario, UnknownDetail{
					Stage: "evaluate", Step: "bound-enforcement",
					Reason:       fmt.Sprintf("operation %d would consume events beyond max_events=%d", index, scenario.Bounds.MaxEvents),
					UnknownClass: "BOUND_EXHAUSTION", NextOperation: "raise the declared event bound or reduce repetitions",
					BlockedBy: []string{fmt.Sprintf("operation_index=%d", index), fmt.Sprintf("max_events=%d", scenario.Bounds.MaxEvents)},
				})
			}
			if issue := requireAllowedEffect(scenario, operation.Effect, index); issue != nil {
				return refutedOutcome(outcome, state, scenario, *issue)
			}
			value, err := evalExpr(operation.Value, state, scenario.Inputs)
			if err != nil {
				return refutedOutcome(outcome, state, scenario, Witness{Kind: "invalid-value", Stage: "evaluate", Step: "repeat-transition", Item: fmt.Sprintf("operation_index=%d", index), Reason: err.Error()})
			}
			for repeat := 0; repeat < operation.Count; repeat++ {
				if issue := appendEvent(&outcome, scenario, operation.Effect, "", value, index); issue != nil {
					return refutedOutcome(outcome, state, scenario, *issue)
				}
			}
		}
	}
	value, err := evalExpr(&scenario.Result, state, scenario.Inputs)
	if err != nil {
		return refutedOutcome(outcome, state, scenario, Witness{Kind: "invalid-result", Stage: "evaluate", Step: "result", Reason: err.Error()})
	}
	outcome.Value = valuePointer(value)
	outcome.State = orderedState(scenario.ObservableState, state)
	if expected := orderedState(scenario.ObservableState, scenario.ExpectedState); !sameJSON(outcome.State, expected) {
		return refutedOutcome(outcome, state, scenario, Witness{Kind: "source-state-declaration-mismatch", Stage: "evaluate", Step: "observable-state", Reason: "final state does not satisfy the .gooo expected observable state", Expected: expected, Actual: outcome.State})
	}
	declared := declaredEvents(scenario.DeclaredTrace)
	if !sameJSON(outcome.Trace, declared) {
		return refutedOutcome(outcome, state, scenario, Witness{Kind: "source-trace-declaration-mismatch", Stage: "evaluate", Step: "declared-trace", Reason: "semantic execution does not satisfy the declared ordered event trace", Expected: declared, Actual: outcome.Trace})
	}
	finalizeOutcome(&outcome)
	_ = semantics
	_ = variant
	return outcome
}

func requireAllowedEffect(scenario Scenario, effect string, index int) *Witness {
	for _, allowed := range scenario.AllowedEffects {
		if allowed == effect {
			return nil
		}
	}
	return &Witness{Kind: "forbidden-effect", Stage: "evaluate", Step: "effect-vocabulary", Item: fmt.Sprintf("operation_index=%d", index), Reason: "effect is not in the scenario allowed_effects vocabulary", Expected: scenario.AllowedEffects, Actual: effect}
}

func appendEvent(outcome *Outcome, scenario Scenario, effect, subject string, value Value, index int) *Witness {
	if len(outcome.Trace) >= scenario.Bounds.MaxEvents {
		return &Witness{Kind: "event-bound-exhaustion", Stage: "evaluate", Step: "bound-enforcement", Item: fmt.Sprintf("operation_index=%d", index), Reason: "event count exceeded max_events", Expected: scenario.Bounds.MaxEvents, Actual: len(outcome.Trace) + 1}
	}
	outcome.Trace = append(outcome.Trace, Event{Ordinal: len(outcome.Trace), Effect: effect, Subject: subject, Value: value})
	return nil
}

func evalExpr(expr *Expr, state map[string]Value, inputs map[string]Value) (Value, error) {
	if expr == nil {
		return Value{}, fmt.Errorf("expression is missing")
	}
	switch expr.Kind {
	case "literal":
		if expr.Value == nil {
			return Value{}, fmt.Errorf("literal value is missing")
		}
		return *expr.Value, nil
	case "state":
		value, ok := state[expr.Name]
		if !ok {
			return Value{}, fmt.Errorf("state %q is unavailable", expr.Name)
		}
		return value, nil
	case "input":
		value, ok := inputs[expr.Name]
		if !ok {
			return Value{}, fmt.Errorf("input %q is unavailable", expr.Name)
		}
		return value, nil
	default:
		return Value{}, fmt.Errorf("expression kind %q is unsupported", expr.Kind)
	}
}

func unknownOutcome(outcome Outcome, state map[string]Value, scenario Scenario, detail UnknownDetail) Outcome {
	outcome.Status = StatusUnknown
	outcome.Reason = detail.Reason
	outcome.Unknown = &detail
	outcome.State = orderedState(scenario.ObservableState, state)
	outcome.TraceAvailable = false
	outcome.Deterministic = false
	finalizeOutcome(&outcome)
	return outcome
}

func refutedOutcome(outcome Outcome, state map[string]Value, scenario Scenario, witness Witness) Outcome {
	outcome.Status = StatusRefuted
	outcome.Reason = witness.Reason
	outcome.Witnesses = append(outcome.Witnesses, witness)
	outcome.State = orderedState(scenario.ObservableState, state)
	outcome.TraceAvailable = true
	outcome.Deterministic = true
	finalizeOutcome(&outcome)
	return outcome
}

func finalizeOutcome(outcome *Outcome) {
	outcome.Effects = make([]EffectCell, 0, len(outcome.Trace))
	for _, event := range outcome.Trace {
		outcome.Effects = append(outcome.Effects, EffectCell{Ordinal: event.Ordinal, Effect: event.Effect})
	}
	payload := struct {
		Value   *Value       `json:"value,omitempty"`
		State   []StateCell  `json:"state"`
		Events  []Event      `json:"events"`
		Effects []EffectCell `json:"effects"`
	}{outcome.Value, outcome.State, outcome.Trace, outcome.Effects}
	digest, err := digestCanonical(payload)
	if err == nil {
		outcome.Digest = digest
	}
}

func orderedState(names []string, state map[string]Value) []StateCell {
	result := make([]StateCell, 0, len(names))
	for _, name := range names {
		result = append(result, StateCell{Name: name, Value: state[name]})
	}
	return result
}

func declaredEvents(events []DeclaredEvent) []Event {
	result := make([]Event, 0, len(events))
	for index, event := range events {
		result = append(result, Event{Ordinal: index, Effect: event.Effect, Subject: event.Subject, Value: event.Value})
	}
	return result
}

func cloneValues(values map[string]Value) map[string]Value {
	result := make(map[string]Value, len(values))
	for key, value := range values {
		result[key] = value
	}
	return result
}

func valuePointer(value Value) *Value { return &value }

func sameJSON(left, right any) bool {
	leftData, leftErr := canonicalJSON(left)
	rightData, rightErr := canonicalJSON(right)
	return leftErr == nil && rightErr == nil && string(leftData) == string(rightData)
}
