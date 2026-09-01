package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/kimjooyoon/gooo-bounded-observational-equivalence-projector/internal/projector"
)

func main() {
	if len(os.Args) < 2 {
		fatal(errors.New("command is required: reference, emit, compare, or corpus"))
	}
	var err error
	switch os.Args[1] {
	case "reference":
		err = reference(os.Args[2:])
	case "emit":
		err = emit(os.Args[2:])
	case "compare":
		err = compare(os.Args[2:])
	case "corpus":
		err = corpus(os.Args[2:])
	default:
		err = fmt.Errorf("unknown command %q", os.Args[1])
	}
	if err != nil {
		fatal(err)
	}
}

func reference(args []string) error {
	flags := flag.NewFlagSet("reference", flag.ContinueOnError)
	semanticsPath := flags.String("semantics", "", "authoritative .gooo semantics")
	scenarioPath := flags.String("scenario", "", "bounded .gooo scenario")
	outputPath := flags.String("output", "", "caller-owned outcome path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *semanticsPath == "" || *scenarioPath == "" || *outputPath == "" {
		return errors.New("reference requires --semantics, --scenario, and --output")
	}
	semantics, _, err := projector.LoadSemantics(*semanticsPath)
	if err != nil {
		return writeClosedFailure(*outputPath, "", "parse", err)
	}
	scenario, _, err := projector.LoadScenario(*scenarioPath, semantics)
	if err != nil {
		return writeClosedFailure(*outputPath, "", "parse", err)
	}
	return projector.WriteJSON(*outputPath, projector.EvaluateReference(scenario, semantics))
}

func emit(args []string) error {
	flags := flag.NewFlagSet("emit", flag.ContinueOnError)
	semanticsPath := flags.String("semantics", "", "authoritative .gooo semantics")
	scenarioPath := flags.String("scenario", "", "bounded .gooo scenario")
	variant := flags.String("variant", "none", "generated candidate variant")
	outputPath := flags.String("output", "", "caller-owned generated Go path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *semanticsPath == "" || *scenarioPath == "" || *outputPath == "" {
		return errors.New("emit requires --semantics, --scenario, and --output")
	}
	semantics, _, err := projector.LoadSemantics(*semanticsPath)
	if err != nil {
		return err
	}
	scenario, _, err := projector.LoadScenario(*scenarioPath, semantics)
	if err != nil {
		return err
	}
	source, err := projector.GenerateGo(scenario, semantics, *variant)
	if err != nil {
		return err
	}
	return projector.WriteText(*outputPath, string(source))
}

func compare(args []string) error {
	flags := flag.NewFlagSet("compare", flag.ContinueOnError)
	semanticsPath := flags.String("semantics", "", "authoritative .gooo semantics")
	scenarioPath := flags.String("scenario", "", "bounded .gooo scenario")
	referencePath := flags.String("reference", "", "reference trace path")
	generatedPath := flags.String("generated", "", "generated Go trace path")
	outputPath := flags.String("output", "", "caller-owned normalized comparison path")
	reportPath := flags.String("report", "", "caller-owned human report path")
	expected := flags.String("expected", "", "optional expected verdict assertion")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *semanticsPath == "" || *scenarioPath == "" || *referencePath == "" || *generatedPath == "" || *outputPath == "" || *reportPath == "" {
		return errors.New("compare requires --semantics, --scenario, --reference, --generated, --output, and --report")
	}
	semantics, _, err := projector.LoadSemantics(*semanticsPath)
	if err != nil {
		return err
	}
	scenario, _, err := projector.LoadScenario(*scenarioPath, semantics)
	if err != nil {
		return err
	}
	reference, err := loadOutcome(*referencePath)
	if err != nil {
		return err
	}
	generated, err := loadOutcome(*generatedPath)
	if err != nil {
		return err
	}
	result := projector.CompareOutcomes(scenario, reference, generated)
	if err := projector.WriteJSON(*outputPath, result); err != nil {
		return err
	}
	if err := projector.WriteText(*reportPath, result.HumanReport); err != nil {
		return err
	}
	if *expected != "" && result.Verdict != projector.Status(*expected) {
		return fmt.Errorf("case %s verdict=%s, expected=%s", scenario.CaseID, result.Verdict, *expected)
	}
	return nil
}

func corpus(args []string) error {
	flags := flag.NewFlagSet("corpus", flag.ContinueOnError)
	semanticsPath := flags.String("semantics", "", "authoritative .gooo semantics")
	corpusPath := flags.String("path", "", "corpus path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	semantics, _, err := projector.LoadSemantics(*semanticsPath)
	if err != nil {
		return err
	}
	value, _, err := projector.LoadCorpus(*corpusPath, semantics)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(value)
}

func loadOutcome(path string) (projector.Outcome, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return projector.Outcome{}, err
	}
	var outcome projector.Outcome
	if err := json.Unmarshal(data, &outcome); err != nil {
		return projector.Outcome{}, fmt.Errorf("parse outcome %s: %w", path, err)
	}
	if outcome.Schema != projector.OutcomeSchema || outcome.CaseID == "" || !outcome.Status.Valid() {
		return projector.Outcome{}, errors.New("outcome is not a valid canonical trace")
	}
	if outcome.Unknown != nil {
		if err := outcome.Unknown.Validate(); err != nil {
			return projector.Outcome{}, err
		}
	}
	return outcome, nil
}

func writeClosedFailure(path, caseID, stage string, err error) error {
	detail := projector.UnknownDetail{
		Stage: stage, Step: "fail-closed", Reason: err.Error(), UnknownClass: "MALFORMED_INPUT",
		NextOperation: "repair the input and replay the bounded scenario", BlockedBy: []string{"input-validation"},
	}
	outcome := projector.Outcome{
		Schema: projector.OutcomeSchema, CaseID: caseID, Status: projector.StatusUnknown,
		Trace: []projector.Event{}, Effects: []projector.EffectCell{}, TraceAvailable: false,
		Deterministic: false, Reason: err.Error(), Unknown: &detail,
	}
	if writeErr := projector.WriteJSON(path, outcome); writeErr != nil {
		return fmt.Errorf("input failure: %v; fail-closed output: %w", err, writeErr)
	}
	return err
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
