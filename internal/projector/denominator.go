package projector

import (
	"encoding/json"
	"fmt"
	"os"
)

type Denominator struct {
	Schema            string              `json:"schema"`
	Authority         string              `json:"authority"`
	ContractID        string              `json:"contract_id"`
	StatusPrecedence  []Status            `json:"status_precedence"`
	ProofFamilies     map[string][]string `json:"proof_families"`
	Invariants        []Invariant         `json:"invariants"`
	ProofDistribution map[string]int      `json:"proof_distribution"`
}

type Invariant struct {
	ID          string `json:"id"`
	ProofFamily string `json:"proof_family"`
	Vector      string `json:"vector"`
	Statement   string `json:"statement"`
}

func LoadDenominator(path string) (Denominator, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Denominator{}, err
	}
	var denominator Denominator
	if err := json.Unmarshal(data, &denominator); err != nil {
		return Denominator{}, err
	}
	if err := denominator.Validate(); err != nil {
		return Denominator{}, err
	}
	return denominator, nil
}

func (d Denominator) Validate() error {
	if d.Schema != "gooo.bounded-observational-equivalence/denominator/v1" || d.Authority != "metacode" || d.ContractID != "bounded-observational-equivalence-v1" {
		return fmt.Errorf("denominator header is not fixed")
	}
	if !sameStatuses(d.StatusPrecedence, requiredStatuses) || len(d.Invariants) != 12 {
		return fmt.Errorf("denominator must contain twelve invariants and fixed status precedence")
	}
	families := map[string][]string{
		"FOUNDATION": {"F01_SOURCE_OWNERSHIP", "F02_SCENARIO_INPUTS", "F03_OBSERVABLE_STATE", "F04_NORMALIZATION_POLICY"},
		"COHERENCE":  {"C01_SEMANTIC_TRACE", "C02_GENERATED_TRACE", "C03_VECTOR_COMPARISON", "C04_STATUS_PRECEDENCE"},
		"REGRESSION": {"R01_BOUND_ENFORCEMENT", "R02_EFFECT_VOCABULARY", "R03_DETERMINISM_REPLAY", "R04_ARTIFACT_MATERIALIZATION"},
	}
	for family, expected := range families {
		if !sameStrings(d.ProofFamilies[family], expected) {
			return fmt.Errorf("denominator proof family %s is not fixed", family)
		}
	}
	expectedVectors := map[string]int{"DRIVER": 4, "OUTCOME": 4, "GUARDRAIL": 4}
	seenIDs := map[string]bool{}
	counts := map[string]int{}
	for _, invariant := range d.Invariants {
		if invariant.ID == "" || seenIDs[invariant.ID] || invariant.Statement == "" {
			return fmt.Errorf("denominator has duplicate or incomplete invariant %q", invariant.ID)
		}
		seenIDs[invariant.ID] = true
		counts[invariant.Vector]++
	}
	for vector, count := range expectedVectors {
		if counts[vector] != count || d.ProofDistribution[vector] != count {
			return fmt.Errorf("denominator vector %s must have exactly %d cells", vector, count)
		}
	}
	for family, expected := range families {
		if d.ProofDistribution[family] != len(expected) {
			return fmt.Errorf("denominator proof distribution %s is not exact", family)
		}
	}
	return nil
}
