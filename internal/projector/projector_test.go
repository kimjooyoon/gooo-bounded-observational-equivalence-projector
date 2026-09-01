package projector

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFixedDenominator(t *testing.T) {
	root := filepath.Join("..", "..")
	if _, err := LoadDenominator(filepath.Join(root, "contracts", "denominator-v1.json")); err != nil {
		t.Fatal(err)
	}
}

func TestCanonicalCorpus(t *testing.T) {
	root := filepath.Join("..", "..")
	semantics, _, err := LoadSemantics(filepath.Join(root, ".gooo", "semantics.gooo"))
	if err != nil {
		t.Fatal(err)
	}
	corpus, _, err := LoadCorpus(filepath.Join(root, ".gooo", "corpus.gooo"), semantics)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[Status]int{}
	for _, item := range corpus.Cases {
		scenario, _, err := LoadScenario(filepath.Join(root, item.Program), semantics)
		if err != nil {
			t.Fatalf("%s: %v", item.ID, err)
		}
		reference := EvaluateReference(scenario, semantics)
		generated := EvaluateGenerated(scenario, semantics, item.Variant)
		comparison := CompareOutcomes(scenario, reference, generated)
		if comparison.Verdict != item.ExpectedStatus {
			t.Fatalf("%s: verdict=%s expected=%s", item.ID, comparison.Verdict, item.ExpectedStatus)
		}
		counts[comparison.Verdict]++
		if item.Replay {
			replay := EvaluateGenerated(scenario, semantics, item.Variant)
			if !sameJSON(generated, replay) {
				t.Fatalf("%s: replay changed the generated outcome", item.ID)
			}
		}
		for _, unknown := range comparison.UnknownWitnesses {
			if err := unknown.Validate(); err != nil {
				t.Fatalf("%s: invalid UNKNOWN witness: %v", item.ID, err)
			}
		}
	}
	if counts[StatusClosed] < 3 || counts[StatusUnknown] < 3 || counts[StatusRefuted] < 3 {
		t.Fatalf("corpus minimum distribution not met: closed=%d unknown=%d refuted=%d", counts[StatusClosed], counts[StatusUnknown], counts[StatusRefuted])
	}
}

func TestMalformedScenarioFailsClosed(t *testing.T) {
	semantics, _, err := LoadSemantics(filepath.Join("..", "..", ".gooo", "semantics.gooo"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseScenario([]byte(`{"schema":"gooo.bounded-observational-equivalence/scenario/v1","case_id":"bad"}`), semantics); err == nil {
		t.Fatal("malformed scenario was accepted")
	}
}

func TestDigestAloneCannotClose(t *testing.T) {
	root := filepath.Join("..", "..")
	semantics, _, err := LoadSemantics(filepath.Join(root, ".gooo", "semantics.gooo"))
	if err != nil {
		t.Fatal(err)
	}
	scenario, _, err := LoadScenario(filepath.Join(root, "fixtures", "closed-basic.gooo"), semantics)
	if err != nil {
		t.Fatal(err)
	}
	reference := EvaluateReference(scenario, semantics)
	generated := EvaluateGenerated(scenario, semantics, "none")
	changed := *generated.Value.Int + 9
	generated.Value = valuePointer(IntValue(changed))
	comparison := CompareOutcomes(scenario, reference, generated)
	if comparison.Verdict != StatusRefuted || comparison.Normalized[0].State != StatusRefuted {
		t.Fatal("a value mismatch was incorrectly closed by a matching stale digest")
	}
}

func TestOutputHelpersUseCallerPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "out.json")
	if err := WriteJSON(path, map[string]string{"status": "CLOSED"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
