#!/usr/bin/env bash
set -euo pipefail

repo_root="${GITHUB_WORKSPACE:-$(pwd)}"
runner_root="${RUNNER_TEMP:-$(mktemp -d)}"
evidence_root="${EVIDENCE_DIR:-$(mktemp -d "$runner_root/gooo-bounded-evidence.XXXXXX")}"
generated_root="$(mktemp -d "$repo_root/.ci-generated.XXXXXX")"
projector="$evidence_root/gooo-projector"
semantics="$repo_root/.gooo/semantics.gooo"
corpus="$repo_root/.gooo/corpus.gooo"

mkdir -p "$evidence_root/cases"
go build -o "$projector" ./cmd/gooo-projector

while IFS=$'\t' read -r case_id program variant expected replay; do
  case_root="$evidence_root/cases/$case_id"
  generated_case_root="$generated_root/$case_id"
  mkdir -p "$case_root" "$generated_case_root"
  scenario="$repo_root/$program"
  "$projector" reference --semantics "$semantics" --scenario "$scenario" --output "$case_root/reference-trace.json"
  "$projector" emit --semantics "$semantics" --scenario "$scenario" --variant "$variant" --output "$generated_case_root/main.go"
  gofmt -w "$generated_case_root/main.go"
  go build -o "$generated_case_root/program" "$generated_case_root/main.go"
  "$generated_case_root/program" > "$case_root/generated-go-trace.json"
  "$projector" compare --semantics "$semantics" --scenario "$scenario" --reference "$case_root/reference-trace.json" --generated "$case_root/generated-go-trace.json" --expected "$expected" --output "$case_root/normalized-comparison.json" --report "$case_root/human-report.md"
  jq '{mismatch_witnesses,unknown_witnesses}' "$case_root/normalized-comparison.json" > "$case_root/witness.json"
  jq -e --arg id "$case_id" --arg expected "$expected" 'select(.case_id == $id and .verdict == $expected) | (.normalized_comparison | length == 4)' "$case_root/normalized-comparison.json" > /dev/null
  if [[ "$replay" == "true" ]]; then
    "$generated_case_root/program" > "$case_root/generated-go-replay.json"
    cmp "$case_root/generated-go-trace.json" "$case_root/generated-go-replay.json"
    digest="$(sha256sum "$case_root/generated-go-trace.json" | awk '{print $1}')"
    jq -n --arg case_id "$case_id" --arg digest "sha256:$digest" '{schema:"gooo.bounded-observational-equivalence/replay/v1",case_id:$case_id,byte_identical:true,output_digest:$digest}' > "$case_root/replay.json"
  fi
done < <(jq -r '.cases[] | [.id,.program,.variant,.expected_status,(.replay|tostring)] | @tsv' "$corpus")

cp "$semantics" "$evidence_root/semantics.gooo"
cp "$corpus" "$evidence_root/corpus.gooo"
cp "$repo_root/contracts/denominator-v1.json" "$evidence_root/denominator-v1.json"

case_count="$(jq '.cases | length' "$corpus")"
normal_count="$(jq '[.cases[] | select(.class == "normal")] | length' "$corpus")"
unknown_count="$(jq '[.cases[] | select(.class == "unknown")] | length' "$corpus")"
refuted_count="$(jq '[.cases[] | select(.class == "refuted")] | length' "$corpus")"
unknown_verdicts="$(jq -s '[.[] | select(.verdict == "UNKNOWN")] | length' "$evidence_root"/cases/*/normalized-comparison.json)"
failed_verdicts="$(jq -s '[.[] | select(.verdict != "CLOSED" and .verdict != "UNKNOWN" and .verdict != "REFUTED")] | length' "$evidence_root"/cases/*/normalized-comparison.json)"
vector_cells="$(jq -s '[.[] | .normalized_comparison[] | {fixture: input_filename, vector: .vector, state: .state, reason: .reason}]' "$evidence_root"/cases/*/normalized-comparison.json)"
go_files="$(find "$repo_root" -path '*/.git' -prune -o -path '*/.ci-generated.*' -prune -o -name '*.go' -type f -print | wc -l | tr -d ' ')"
gooo_files="$(find "$repo_root" -path '*/.git' -prune -o -path '*/.ci-generated.*' -prune -o -name '*.gooo' -type f -print | wc -l | tr -d ' ')"
go_lines="$(find "$repo_root" -path '*/.git' -prune -o -path '*/.ci-generated.*' -prune -o -name '*.go' -type f -print0 | xargs -0 awk 'END {print NR + 0}')"
gooo_lines="$(find "$repo_root" -path '*/.git' -prune -o -path '*/.ci-generated.*' -prune -o -name '*.gooo' -type f -print0 | xargs -0 awk 'END {print NR + 0}')"

jq -n \
  --arg schema "gooo.bounded-observational-equivalence/evidence/v1" \
  --arg commit "${GITHUB_SHA:-local-ci}" \
  --arg workflow "${GITHUB_WORKFLOW:-ci}" \
  --arg run "${GITHUB_RUN_ID:-local}" \
  --argjson case_count "$case_count" \
  --argjson normal_count "$normal_count" \
  --argjson unknown_count "$unknown_count" \
  --argjson refuted_count "$refuted_count" \
  --argjson unknown_verdicts "$unknown_verdicts" \
  --argjson failed_verdicts "$failed_verdicts" \
  --argjson vector_cells "$vector_cells" \
  --argjson go_files "$go_files" \
  --argjson gooo_files "$gooo_files" \
  --argjson go_lines "$go_lines" \
  --argjson gooo_lines "$gooo_lines" \
  '{schema:$schema,authority:"github-actions",commit_sha:$commit,workflow:$workflow,run_id:$run,toolchain:{go:"1.27.x",target:"go1.27"},status_precedence:["REFUTED","UNKNOWN","CLOSED"],corpus:{total:$case_count,normal:$normal_count,unknown:$unknown_count,refuted:$refuted_count},test_counts:{total:$case_count,selected:$case_count,executed:$case_count,reused:0,failed:$failed_verdicts,unknown:$unknown_verdicts},vector_cells:$vector_cells,denominator:{foundation:4,coherence:4,regression:4,driver:4,outcome:4,guardrail:4},runtime_contract:{repository_writes:0,local_test_executions:0,cross_project_required_gates:0},inventory:{go_files:$go_files,gooo_files:$gooo_files,go_physical_lines:$go_lines,gooo_physical_lines:$gooo_lines},improvement:{status:"UNKNOWN",before:null,after:null,reason:"exact same scenario/source/contract/fixture/toolchain/runner integer before-after evidence is absent"},local_validation_command_count:0}' > "$evidence_root/evidence.json"

if [[ -n "$(git -C "$repo_root" status --porcelain)" ]]; then
  echo "tracked repository changed during conformance" >&2
  exit 1
fi

printf '%s\n' "$evidence_root"
