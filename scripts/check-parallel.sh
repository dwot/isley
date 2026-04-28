#!/usr/bin/env bash
# scripts/check-parallel.sh
#
# Phase 4 of docs/TEST_PLAN_2.md — fail when a top-level test function in
# a "parallel" test file omits t.Parallel(). Files annotated at the top
# with `// +parallel:serial` opt out of the lint entirely (they document
# why the file's tests are serial; CONTRIBUTING.md and TEST_PLAN_2.md
# enumerate the acceptable causes).
#
# Intent: parallelism is the project's smell-detector for shared mutable
# state in tests. The lint enforces "every test parallelizes or the file
# names the cause."
#
# Exits 0 when every *_test.go file is either annotated serial or has
# t.Parallel() in every top-level Test function. Exits 1 with a list of
# offenders otherwise.
set -euo pipefail

status=0
while IFS= read -r f; do
    # Whole-file opt-out — the annotation comment must be present
    # somewhere in the file, conventionally in a top-of-file block.
    if grep -q '// +parallel:serial' "$f"; then
        continue
    fi

    # Walk every top-level Test function and flag those without
    # t.Parallel(). Termination relies on gofmt placing a top-level
    # function's closing `}` at column 0; this sidesteps the
    # brace-counting mistake of confusing `{` inside a string or slice
    # literal with the function body.
    awk -v file="$f" '
        BEGIN { in_fn = 0; saw = 0; name = "" }
        /^func Test[A-Z][A-Za-z0-9_]*\(t \*testing\.T\) \{[ \t]*$/ {
            in_fn = 1
            saw = 0
            name = $2
            sub(/\(.*$/, "", name)
            next
        }
        in_fn && $0 ~ /[^a-zA-Z_]t\.Parallel\(\)/ {
            saw = 1
        }
        in_fn && /^\}$/ {
            if (!saw) {
                printf "%s: %s missing t.Parallel()\n", file, name
                rc = 1
            }
            in_fn = 0
        }
        END { exit rc }
    ' "$f" || status=1
done < <(find . -name "*_test.go" -not -path "./vendor/*" -not -path "./node_modules/*" | sort)

if [ "$status" -ne 0 ]; then
    cat >&2 <<'MSG'

A top-level test function above is missing t.Parallel(). Either add it
to the test, or annotate the file's package block with
`// +parallel:serial — <concrete cause>` if the test legitimately cannot
parallelize. See CONTRIBUTING.md and docs/TEST_PLAN_2.md (Phase 4) for
the convention.
MSG
fi

exit "$status"
