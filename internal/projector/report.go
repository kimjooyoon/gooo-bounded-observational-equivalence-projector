package projector

import (
	"encoding/json"
	"fmt"
	"strings"
)

func RenderHumanReport(comparison Comparison) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "# Bounded observational equivalence: %s\n\n", comparison.CaseID)
	fmt.Fprintf(&builder, "Verdict: `%s`\n\n", comparison.Verdict)
	builder.WriteString("Scope: the fixed scenario inputs, observable state, ordered event trace, allowed effects, bounds, and normalization policy declared by `.gooo`. This is not a general program-equivalence claim.\n\n")
	builder.WriteString("| vector | state | reason |\n|---|---|---|\n")
	for _, vector := range comparison.Normalized {
		fmt.Fprintf(&builder, "| `%s` | `%s` | %s |\n", vector.Vector, vector.State, vector.Reason)
	}
	builder.WriteString("\n## Normalized comparison\n\n")
	for _, vector := range comparison.Normalized {
		fmt.Fprintf(&builder, "### %s\n\n", vector.Vector)
		fmt.Fprintf(&builder, "Reference: `%s`\n\n", compactJSON(vector.Reference))
		fmt.Fprintf(&builder, "Generated Go: `%s`\n\n", compactJSON(vector.Generated))
	}
	if len(comparison.MismatchWitnesses) > 0 {
		builder.WriteString("## Mismatch witnesses\n\n")
		for _, witness := range comparison.MismatchWitnesses {
			fmt.Fprintf(&builder, "- `%s` at `%s/%s` (%s): %s\n", witness.Kind, witness.Stage, witness.Step, witness.Item, witness.Reason)
		}
		builder.WriteString("\n")
	}
	if len(comparison.UnknownWitnesses) > 0 {
		builder.WriteString("## UNKNOWN witnesses\n\n")
		for _, unknown := range comparison.UnknownWitnesses {
			fmt.Fprintf(&builder, "- stage=`%s`, step=`%s`, reason=%s, unknown_class=`%s`, next_operation=%s, blocked_by=`%s`\n", unknown.Stage, unknown.Step, unknown.Reason, unknown.UnknownClass, unknown.NextOperation, strings.Join(unknown.BlockedBy, ","))
		}
		builder.WriteString("\n")
	}
	return builder.String()
}

func compactJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		return "<unencodable>"
	}
	return string(data)
}
