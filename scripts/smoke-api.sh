#!/usr/bin/env bash
# smoke-api.sh — read-only smoke test of an Isley instance's API v1 and MCP
# server. Safe against a live instance: it only reads.
#
#   ISLEY_KEY=<read key> scripts/smoke-api.sh https://isley.example
#
# Exits non-zero if any check fails. Needs bash and curl.
set -uo pipefail

BASE="${1:?usage: ISLEY_KEY=<key> $0 <base-url>}"
BASE="${BASE%/}"
KEY="${ISLEY_KEY:?set ISLEY_KEY to a read (or higher) API key}"
API="$BASE/api/v1"
fails=0
body=""

# check NAME EXPECTED_STATUS NEEDLE CURL_ARGS...: the response has the status
# and its body contains NEEDLE.
check() {
	local name=$1 want=$2 needle=$3
	shift 3
	local out status
	out=$(curl -sS --max-time 20 -w $'\n%{http_code}' "$@" 2>&1)
	status=${out##*$'\n'}
	body=${out%$'\n'*}
	if [[ "$status" == "$want" && "$body" == *"$needle"* ]]; then
		printf 'ok    %s\n' "$name"
	else
		printf 'FAIL  %s: HTTP %s, wanted %s with %q\n      %.300s\n' "$name" "$status" "$want" "$needle" "$body"
		fails=$((fails + 1))
	fi
}

auth=(-H "Authorization: Bearer $KEY")

check "health" 200 '"status"' "$BASE/health"
check "spec is public" 200 "openapi:" "$API/openapi.yaml"
check "guide is public" 200 "Isley" "$API/docs"
check "no key is refused" 401 '"unauthorized"' "$API/instance"
check "instance" 200 '"time_zone"' "${auth[@]}" "$API/instance"
instance=$body
for path in statuses metrics activity-types zones "plants?phase=growing" "plants?phase=harvested&limit=5" \
	strains breeders sensors streams smoke-criteria "activities?limit=5"; do
	check "GET /$path" 200 '"items"' "${auth[@]}" "$API/$path"
done

# The first plant, strain and sensor, if there are any.
first_id() { [[ "$1" =~ \"items\":\[\{\"id\":([0-9]+) ]] && echo "${BASH_REMATCH[1]}"; }
check "plants" 200 '"items"' "${auth[@]}" "$API/plants?limit=1"
if id=$(first_id "$body"); then
	check "GET /plants/$id" 200 '"status_history"' "${auth[@]}" "$API/plants/$id"
	check "GET /plants/$id/activities" 200 '"items"' "${auth[@]}" "$API/plants/$id/activities"
fi
check "strains" 200 '"items"' "${auth[@]}" "$API/strains?limit=1"
if id=$(first_id "$body"); then
	check "GET /strains/$id/lineage" 200 '"' "${auth[@]}" "$API/strains/$id/lineage"
fi
check "sensors" 200 '"items"' "${auth[@]}" "$API/sensors?limit=1"
if id=$(first_id "$body"); then
	check "GET /sensors/$id/data (last day)" 200 '"bucket"' "${auth[@]}" "$API/sensors/$id/data"
fi
check "unknown plant" 404 '"plant_not_found"' "${auth[@]}" "$API/plants/999999999"

# MCP, when it's switched on.
if [[ "$instance" == *'"mcp":true'* ]]; then
	mcp=(-H "Content-Type: application/json" -H "Accept: application/json, text/event-stream" "${auth[@]}")
	check "MCP tools/list" 200 '"get_overview"' "${mcp[@]}" \
		-d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' "$BASE/mcp"
	check "MCP get_overview" 200 'growing' "${mcp[@]}" \
		-d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_overview","arguments":{}}}' "$BASE/mcp"
else
	echo "skip  MCP (off on this instance)"
fi

if ((fails > 0)); then
	echo "$fails check(s) failed"
	exit 1
fi
echo "all checks passed"
