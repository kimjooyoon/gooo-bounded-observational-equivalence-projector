package projector

import (
	"encoding/json"
	"fmt"
	"strconv"
)

func GenerateGo(scenario Scenario, semantics Semantics, variant string) ([]byte, error) {
	scenario.Variant = variant
	scenarioRaw, err := json.Marshal(scenario)
	if err != nil {
		return nil, fmt.Errorf("marshal generated scenario: %w", err)
	}
	semanticsRaw, err := json.Marshal(semantics)
	if err != nil {
		return nil, fmt.Errorf("marshal generated semantics: %w", err)
	}
	return []byte(fmt.Sprintf(`package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/kimjooyoon/gooo-bounded-observational-equivalence-projector/internal/projector"
)

const scenarioJSON = %s
const semanticsJSON = %s

func main() {
	semantics, err := projector.ParseSemantics([]byte(semanticsJSON))
	if err != nil {
		fail(err)
	}
	scenario, err := projector.ParseScenario([]byte(scenarioJSON), semantics)
	if err != nil {
		fail(err)
	}
	outcome := projector.EvaluateGenerated(scenario, semantics, %s)
	if err := json.NewEncoder(os.Stdout).Encode(outcome); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
`, strconv.Quote(string(scenarioRaw)), strconv.Quote(string(semanticsRaw)), strconv.Quote(variant))), nil
}
