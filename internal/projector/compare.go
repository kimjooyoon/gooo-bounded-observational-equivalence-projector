package projector

import (
	"fmt"
	"sort"
)

type ValueVector struct {
	Value *Value      `json:"value,omitempty"`
	State []StateCell `json:"state"`
}

func NormalizeOutcome(scenario Scenario, outcome Outcome) Normalized {
	stateByName := make(map[string]Value, len(outcome.State))
	for _, cell := range outcome.State {
		stateByName[cell.Name] = cell.Value
	}
	state := orderedState(scenario.ObservableState, stateByName)
	events := append([]Event(nil), outcome.Trace...)
	sort.SliceStable(events, func(left, right int) bool {
		return events[left].Ordinal < events[right].Ordinal
	})
	effects := make([]EffectCell, 0, len(events))
	for _, event := range events {
		effects = append(effects, EffectCell{Ordinal: event.Ordinal, Effect: event.Effect})
	}
	return Normalized{
		Value: outcome.Value, State: state, Events: events, Effects: effects,
		Determinism: DeterminismCell{TraceAvailable: outcome.TraceAvailable, Deterministic: outcome.Deterministic, Digest: outcome.Digest},
	}
}

func CompareOutcomes(scenario Scenario, reference, generated Outcome) Comparison {
	referenceNormalized := NormalizeOutcome(scenario, reference)
	generatedNormalized := NormalizeOutcome(scenario, generated)
	vectors := []VectorComparison{
		compareValueVector(referenceNormalized, generatedNormalized, reference, generated),
		compareEventVector(referenceNormalized, generatedNormalized, reference, generated),
		compareEffectVector(referenceNormalized, generatedNormalized, reference, generated),
		compareDeterminismVector(referenceNormalized, generatedNormalized, reference, generated),
	}
	comparison := Comparison{
		Schema: CompareSchema, CaseID: scenario.CaseID,
		Verdict: StatusClosed, StatusPrecedence: append([]Status(nil), requiredStatuses...),
		ReferenceTrace: reference, GeneratedGoTrace: generated, Normalized: vectors,
	}
	for _, vector := range vectors {
		comparison.MismatchWitnesses = append(comparison.MismatchWitnesses, vector.Witnesses...)
		comparison.UnknownWitnesses = append(comparison.UnknownWitnesses, vector.Unknown...)
		if vector.State == StatusRefuted {
			comparison.Verdict = StatusRefuted
		}
	}
	if comparison.Verdict != StatusRefuted {
		for _, vector := range vectors {
			if vector.State == StatusUnknown {
				comparison.Verdict = StatusUnknown
			}
		}
	}
	if reference.Status == StatusRefuted || generated.Status == StatusRefuted {
		comparison.Verdict = StatusRefuted
	}
	if comparison.Verdict != StatusRefuted && (reference.Status == StatusUnknown || generated.Status == StatusUnknown) {
		comparison.Verdict = StatusUnknown
	}
	if reference.Unknown != nil {
		comparison.UnknownWitnesses = append(comparison.UnknownWitnesses, *reference.Unknown)
	}
	if generated.Unknown != nil {
		comparison.UnknownWitnesses = append(comparison.UnknownWitnesses, *generated.Unknown)
	}
	comparison.MismatchWitnesses = append(comparison.MismatchWitnesses, reference.Witnesses...)
	comparison.MismatchWitnesses = append(comparison.MismatchWitnesses, generated.Witnesses...)
	comparison.HumanReport = RenderHumanReport(comparison)
	return comparison
}

func compareValueVector(reference, generated Normalized, referenceOutcome, generatedOutcome Outcome) VectorComparison {
	left := ValueVector{Value: reference.Value, State: reference.State}
	right := ValueVector{Value: generated.Value, State: generated.State}
	result := VectorComparison{Vector: "value", State: StatusClosed, Reference: left, Generated: right, Reason: "typed result value and declared observable state match"}
	if referenceOutcome.Status == StatusUnknown || generatedOutcome.Status == StatusUnknown {
		return unknownVector(result, referenceOutcome, generatedOutcome, "value is not closed because an execution stage is UNKNOWN")
	}
	if !sameJSON(left, right) {
		result.State = StatusRefuted
		result.Reason = "typed result value or observable state differs"
		result.Witnesses = []Witness{{Kind: "value-mismatch", Stage: "compare", Step: "value-vector", Item: "value", Reason: result.Reason, Expected: left, Actual: right}}
	}
	result = applyOutcomeWitnesses(result, referenceOutcome, generatedOutcome)
	return result
}

func compareEventVector(reference, generated Normalized, referenceOutcome, generatedOutcome Outcome) VectorComparison {
	result := VectorComparison{Vector: "event", State: StatusClosed, Reference: reference.Events, Generated: generated.Events, Reason: "ordered event trace matches item by item"}
	if !referenceOutcome.TraceAvailable || !generatedOutcome.TraceAvailable || referenceOutcome.Status == StatusUnknown || generatedOutcome.Status == StatusUnknown {
		return unknownVector(result, referenceOutcome, generatedOutcome, "ordered event trace is not fully available")
	}
	if !sameJSON(reference.Events, generated.Events) {
		result.State = StatusRefuted
		result.Reason = "ordered event trace differs at one or more items"
		result.Witnesses = []Witness{{Kind: "event-mismatch", Stage: "compare", Step: "event-vector", Item: firstEventMismatch(reference.Events, generated.Events), Reason: result.Reason, Expected: reference.Events, Actual: generated.Events}}
	}
	result = applyOutcomeWitnesses(result, referenceOutcome, generatedOutcome)
	return result
}

func compareEffectVector(reference, generated Normalized, referenceOutcome, generatedOutcome Outcome) VectorComparison {
	result := VectorComparison{Vector: "effect", State: StatusClosed, Reference: reference.Effects, Generated: generated.Effects, Reason: "ordered effect vocabulary matches item by item"}
	if !referenceOutcome.TraceAvailable || !generatedOutcome.TraceAvailable || referenceOutcome.Status == StatusUnknown || generatedOutcome.Status == StatusUnknown {
		return unknownVector(result, referenceOutcome, generatedOutcome, "effect vector is not fully available")
	}
	if !sameJSON(reference.Effects, generated.Effects) {
		result.State = StatusRefuted
		result.Reason = "ordered effect vector differs"
		result.Witnesses = []Witness{{Kind: "effect-mismatch", Stage: "compare", Step: "effect-vector", Item: firstEffectMismatch(reference.Effects, generated.Effects), Reason: result.Reason, Expected: reference.Effects, Actual: generated.Effects}}
	}
	result = applyOutcomeWitnesses(result, referenceOutcome, generatedOutcome)
	return result
}

func compareDeterminismVector(reference, generated Normalized, referenceOutcome, generatedOutcome Outcome) VectorComparison {
	result := VectorComparison{Vector: "determinism", State: StatusClosed, Reference: reference.Determinism, Generated: generated.Determinism, Reason: "both complete traces are deterministic and their canonical digests match"}
	if !reference.Determinism.TraceAvailable || !generated.Determinism.TraceAvailable || !reference.Determinism.Deterministic || !generated.Determinism.Deterministic || referenceOutcome.Status == StatusUnknown || generatedOutcome.Status == StatusUnknown {
		return unknownVector(result, referenceOutcome, generatedOutcome, "determinism cannot be closed without complete deterministic traces")
	}
	if reference.Determinism.Digest != generated.Determinism.Digest {
		result.State = StatusRefuted
		result.Reason = "canonical observation digests differ"
		result.Witnesses = []Witness{{Kind: "determinism-mismatch", Stage: "compare", Step: "determinism-vector", Item: "digest", Reason: result.Reason, Expected: reference.Determinism.Digest, Actual: generated.Determinism.Digest}}
	}
	return result
}

func unknownVector(result VectorComparison, reference, generated Outcome, reason string) VectorComparison {
	result.State = StatusUnknown
	result.Reason = reason
	if result.Unknown == nil {
		result.Unknown = make([]UnknownDetail, 0, 2)
	}
	if reference.Unknown != nil {
		result.Unknown = append(result.Unknown, *reference.Unknown)
	}
	if generated.Unknown != nil {
		result.Unknown = append(result.Unknown, *generated.Unknown)
	}
	if !reference.TraceAvailable || !generated.TraceAvailable {
		result.Unknown = append(result.Unknown, UnknownDetail{
			Stage: "compare", Step: "trace-presence", Reason: "one or both canonical traces are missing",
			UnknownClass: "MISSING_TRACE", NextOperation: "rerun the generated candidate with trace output enabled",
			BlockedBy: []string{"reference.trace_available", "generated.trace_available"},
		})
	}
	return result
}

func applyOutcomeWitnesses(result VectorComparison, reference, generated Outcome) VectorComparison {
	for _, witness := range append(append([]Witness(nil), reference.Witnesses...), generated.Witnesses...) {
		switch witness.Kind {
		case "forbidden-effect", "effect-mismatch":
			if result.Vector == "effect" {
				result.State = StatusRefuted
				result.Witnesses = append(result.Witnesses, witness)
			}
		case "event-mismatch", "source-trace-declaration-mismatch":
			if result.Vector == "event" {
				result.State = StatusRefuted
				result.Witnesses = append(result.Witnesses, witness)
			}
		case "value-mismatch", "state-type-mismatch", "source-state-declaration-mismatch", "invalid-result":
			if result.Vector == "value" {
				result.State = StatusRefuted
				result.Witnesses = append(result.Witnesses, witness)
			}
		}
	}
	return result
}

func firstEventMismatch(left, right []Event) string {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for index := 0; index < limit; index++ {
		if !sameJSON(left[index], right[index]) {
			return fmt.Sprintf("ordinal=%d", index)
		}
	}
	return fmt.Sprintf("length=%d/%d", len(left), len(right))
}

func firstEffectMismatch(left, right []EffectCell) string {
	limit := len(left)
	if len(right) < limit {
		limit = len(right)
	}
	for index := 0; index < limit; index++ {
		if !sameJSON(left[index], right[index]) {
			return fmt.Sprintf("ordinal=%d", index)
		}
	}
	return fmt.Sprintf("length=%d/%d", len(left), len(right))
}
