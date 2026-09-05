#!/usr/bin/env bash
#
# Documentation freshness checks (NFR-12, NFR-14, NFR-09).
#
# This is a file rather than inline YAML on purpose: a CI check nobody can run
# locally is one people meet for the first time on a red pull request.
#
#   ./.github/scripts/check-docs.sh
#
# Each check answers a way documentation has actually rotted in repositories of
# this shape, not a generic style preference. Every failure prints what to do,
# because a checker that only says "no" gets disabled.

set -uo pipefail
cd "$(dirname "$0")/../.."

FAILED=0

fail() { printf '\n\033[31mFAIL\033[0m  %s\n' "$1"; FAILED=1; }
pass() { printf '\033[32mok\033[0m    %s\n' "$1"; }
note() { printf '        %s\n' "$1"; }

# ---------------------------------------------------------------------------
# 1. The ADR index and the ADR files agree, in both directions.
#
# A new ADR that is not indexed is invisible; an indexed ADR whose file was
# renamed is a dead link in the one document that is supposed to be the map.
# ---------------------------------------------------------------------------
missing_from_index=""
for f in docs/adr/[0-9]*.md; do
  base=$(basename "$f")
  grep -q "($base)" docs/adr/README.md || missing_from_index="$missing_from_index $base"
done

missing_files=""
for linked in $(grep -oE '\(([0-9]{4}-[a-z0-9-]+\.md)\)' docs/adr/README.md | tr -d '()'); do
  [ -f "docs/adr/$linked" ] || missing_files="$missing_files $linked"
done

if [ -n "$missing_from_index" ]; then
  fail "ADR files with no row in docs/adr/README.md:$missing_from_index"
  note "Add a row to the index table. An unindexed ADR is one nobody finds."
elif [ -n "$missing_files" ]; then
  fail "docs/adr/README.md links to files that do not exist:$missing_files"
else
  pass "ADR index matches the ADR files ($(ls docs/adr/[0-9]*.md | wc -l | tr -d ' ') records)"
fi

# ---------------------------------------------------------------------------
# 2. Every requirement id referenced anywhere resolves to a real requirement.
#
# Requirements are cited from ADRs, godoc, commit messages and tests (§1 of
# REQUIREMENTS.md). A citation of a requirement that does not exist is a reader
# following a reference to nothing, and it is exactly what happens when one is
# renumbered.
#
# Cross-project citations -- this ecosystem's other libraries have their own
# numbering -- are written qualified, as `crier/IR-06` or `moat/NFR-01`, and are
# skipped here. Unqualified, they would be silently checked against cairn's
# numbering and resolve to a different requirement or to none.
# ---------------------------------------------------------------------------
defined=$(grep -oE '\*\*(FR|SR|NFR|IR)-[0-9]{2}' REQUIREMENTS.md | tr -d '*' | sort -u)
referenced=$(grep -rhoE '(^|[^/[:alnum:]])(FR|SR|NFR|IR)-[0-9]{2}\b' \
               --include='*.md' --include='*.go' --include='*.yml' \
               . 2>/dev/null | grep -oE '(FR|SR|NFR|IR)-[0-9]{2}' | sort -u)

dangling=""
for id in $referenced; do
  printf '%s\n' "$defined" | grep -qx "$id" || dangling="$dangling $id"
done

if [ -n "$dangling" ]; then
  fail "references to requirement ids that REQUIREMENTS.md does not define:$dangling"
  note "Either the requirement was renumbered, or the citation is a typo."
else
  pass "every requirement citation resolves ($(printf '%s\n' "$defined" | wc -l | tr -d ' ') defined)"
fi

# ---------------------------------------------------------------------------
# 3. Every threat id referenced resolves to a real threat.
#
# Same failure mode as above, in the document a reviewer consults to ask
# "is the residual still what we said it was".
# ---------------------------------------------------------------------------
threats=$(grep -oE '^### T-[0-9]{2}' docs/THREAT-MODEL.md | awk '{print $2}' | sort -u)
threat_refs=$(grep -rhoE '\bT-[0-9]{2}\b' \
                --include='*.md' --include='*.go' . 2>/dev/null | sort -u)

dangling_t=""
for id in $threat_refs; do
  printf '%s\n' "$threats" | grep -qx "$id" || dangling_t="$dangling_t $id"
done

if [ -n "$dangling_t" ]; then
  fail "references to threat ids that docs/THREAT-MODEL.md does not define:$dangling_t"
else
  pass "every threat citation resolves ($(printf '%s\n' "$threats" | wc -l | tr -d ' ') defined)"
fi

# ---------------------------------------------------------------------------
# 4. Every relative link between documents resolves to a file that exists.
#
# The cheapest kind of rot: a document is moved and six links go stale in
# silence. External URLs are not checked -- a network check that fails on a
# transient outage teaches people to ignore the job.
# ---------------------------------------------------------------------------
broken=""
while IFS= read -r doc; do
  dir=$(dirname "$doc")
  for target in $(grep -oE '\]\([^)#][^)]*\)' "$doc" | sed 's/^](//; s/)$//' | grep -v '^https\?://' | grep -v '^mailto:'); do
    clean=${target%%#*}
    [ -z "$clean" ] && continue
    [ -e "$dir/$clean" ] || broken="$broken\n  $doc -> $target"
  done
done < <(find . -name '*.md' -not -path './.git/*')

if [ -n "$broken" ]; then
  fail "broken relative links:"
  printf "$broken\n"
else
  pass "every relative document link resolves"
fi

# ---------------------------------------------------------------------------
# 5. Every public package with exported identifiers has a testable example.
#
# NFR-09, matching moat and crier. Skipped for packages that export nothing
# yet, so this passes during the pre-implementation phase and starts biting the
# moment a package has an API to demonstrate.
# ---------------------------------------------------------------------------
if command -v go >/dev/null 2>&1; then
  no_example=""
  for mod in . redisstore; do
    while IFS= read -r pkg; do
      dir="$mod/${pkg#github.com/JonasBorgesLM/cairn}"
      dir=${dir#./}; dir=${dir%/}; [ -z "$dir" ] && dir="."
      [ -d "$dir" ] || continue
      # Exported identifiers, ignoring test files.
      exported=$(cd "$dir" && go doc -all . 2>/dev/null | grep -cE '^(func|type|var|const) [A-Z]' || true)
      [ "${exported:-0}" -eq 0 ] && continue
      examples=$(grep -rhoE '^func Example[A-Za-z_]*\(' "$dir"/*_test.go 2>/dev/null | wc -l | tr -d ' ')
      [ "${examples:-0}" -eq 0 ] && no_example="$no_example $dir"
    done < <(cd "$mod" && go list ./... 2>/dev/null)
  done

  if [ -n "$no_example" ]; then
    fail "packages exporting an API with no ExampleXxx (NFR-09):$no_example"
    note "A public package without a runnable example is documented by hope."
  else
    pass "every package exporting an API carries a testable example"
  fi
else
  note "skipping the example check: go is not on PATH"
fi

echo
if [ "$FAILED" -ne 0 ]; then
  echo "Documentation checks failed."
  exit 1
fi
echo "Documentation checks passed."
