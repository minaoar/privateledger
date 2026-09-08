#!/usr/bin/env bash

# Automate the PrivateLedger coding -> independent review loop in HerdR.
#
# Recommended pane setup
# ----------------------
# Start HerdR, open one pane for each provider, and run the appropriate command
# in each pane. Either provider can be assigned either workflow role.
#
#   # Claude: lets Claude handle routine permissions automatically.
#   claude --permission-mode auto
#
#   # Codex: --approve-for-me sends approvals to Codex's separate automatic
#   # reviewer and selects the workspace-write sandbox automatically.
#   codex --approve-for-me
#
# You can also register/start the agents from another terminal when you know
# the HerdR pane IDs:
#
#   herdr agent start coding_agent --kind claude --pane <PANE_ID> -- --permission-mode auto
#   herdr agent start review_agent --kind codex  --pane <PANE_ID> -- --approve-for-me
#
# Swap the providers, names, and provider-specific options when Codex owns
# production and Claude owns independent review.
#
# Confirm the names that this script should use with:
#
#   herdr agent list
#
# Normal use
# ----------
# If the initial production pass is already running (or has just finished), run:
#
#   ./.agent_status.sh coding_agent review_agent
#
# To have this script send the initial production prompt too, add "start":
#
#   ./.agent_status.sh coding_agent review_agent start
#
# MAX_CYCLES defaults to 2. TIMEOUT_MS defaults to two hours per agent turn.
# Prompts can be customized through PRODUCTION_PROMPT, FIX_PROMPT, and
# REVIEW_PROMPT environment variables.
# The current AI-DLC handoff and its declared review artifact are discovered
# automatically. Set HANDOFF_FILE only when intentionally revisiting an older
# unit whose handoff is no longer the most recently changed one.
# A PASS is accepted only when the review file changed during that review and
# its first Gate Result section cites the production HEAD captured beforehand.
#
# A blocked pane is never answered with a blind Enter. The script prints the
# pane's recent output and stops, because the block may be a product decision,
# browser confirmation, denied automatic approval, or another real question.

set -Eeuo pipefail

usage() {
    cat <<'EOF'
Usage:
  ./.agent_status.sh <coding-agent> <review-agent> [start]

Examples:
  ./.agent_status.sh coding_agent review_agent

  ./.agent_status.sh coding_agent review_agent start
EOF
}

if (( $# < 2 || $# > 3 )); then
    usage
    exit 2
fi

CODING_AGENT=$1
REVIEW_AGENT=$2
INITIAL_MODE=${3:-wait}
MAX_CYCLES=${MAX_CYCLES:-2}
TIMEOUT_MS=${TIMEOUT_MS:-7200000}

if [[ "$INITIAL_MODE" != "wait" && "$INITIAL_MODE" != "start" ]]; then
    echo "Error: the optional fourth argument must be 'start'." >&2
    usage >&2
    exit 2
fi

if ! [[ "$MAX_CYCLES" =~ ^[1-9][0-9]*$ ]]; then
    echo "Error: MAX_CYCLES must be a positive integer." >&2
    exit 2
fi

if ! [[ "$TIMEOUT_MS" =~ ^[1-9][0-9]*$ ]]; then
    echo "Error: TIMEOUT_MS must be a positive integer." >&2
    exit 2
fi

file_mtime() {
    stat -f '%m' "$1" 2>/dev/null || stat -c '%Y' "$1"
}

discover_handoff() {
    local candidate
    local commit_time
    local score
    local newest_score=-1
    local newest_count=0
    local newest_file=
    local candidates=(aidlc-docs/construction/*/code/independent-review-handoff.md)

    if [[ -n "${HANDOFF_FILE:-}" ]]; then
        [[ -f "$HANDOFF_FILE" ]] || {
            echo "Error: HANDOFF_FILE does not exist: $HANDOFF_FILE" >&2
            exit 2
        }
        printf '%s\n' "$HANDOFF_FILE"
        return
    fi

    [[ -e "${candidates[0]}" ]] || {
        echo "Error: no AI-DLC independent-review-handoff.md was found." >&2
        exit 2
    }

    for candidate in "${candidates[@]}"; do
        if [[ -n "$(git status --porcelain -- "$candidate")" ]]; then
            score=$(file_mtime "$candidate")
        else
            commit_time=$(git log -1 --format='%ct' -- "$candidate")
            score=${commit_time:-0}
        fi

        if (( score > newest_score )); then
            newest_score=$score
            newest_count=1
            newest_file=$candidate
        elif (( score == newest_score )); then
            (( newest_count += 1 ))
        fi
    done

    if (( newest_count != 1 )); then
        echo "Error: could not uniquely identify the current AI-DLC handoff." >&2
        echo "Set HANDOFF_FILE to the intended independent-review-handoff.md." >&2
        exit 2
    fi

    printf '%s\n' "$newest_file"
}

extract_review_file() {
    local handoff=$1
    local paths
    local path_count

    paths=$(grep -Eo 'aidlc-docs/construction/[[:alnum:]_.\/-]+/code-review/independent-review\.md' "$handoff" | sort -u)
    path_count=$(printf '%s\n' "$paths" | awk 'NF { count++ } END { print count + 0 }')

    if (( path_count != 1 )); then
        echo "Error: expected exactly one declared independent-review.md path in $handoff; found $path_count." >&2
        exit 2
    fi

    printf '%s\n' "$paths"
}

ACTIVE_HANDOFF=$(discover_handoff)
REVIEW_FILE=$(extract_review_file "$ACTIVE_HANDOFF")

echo "AI-DLC handoff: $ACTIVE_HANDOFF"
echo "Review artifact: $REVIEW_FILE"

PRODUCTION_PROMPT=${PRODUCTION_PROMPT:-"Begin or continue production coding for the current AI-DLC unit. Read PROJECT_GUIDELINES.md, aidlc-docs/aidlc-state.md, and the current approved code-generation handoff before editing. Follow the production ownership boundary, run the required production checks, then commit and push the production changes."}
default_fix_prompt() {
    # Quoted heredoc: nothing in the body is expanded, so a backtick or $( )
    # pasted into the text later cannot execute when this script loads.
    # @REVIEW_FILE@ is substituted below instead.
    cat <<'PROMPT'
The independent review is complete.

1. Before editing anything, run "git fetch origin", "git log --oneline -5 @{upstream}", and "git status --porcelain". If the reviewer pushed commits you do not have, pull them - their tests are what your build must compile against.

2. Read @REVIEW_FILE@ end to end, including the newest re-review section and not only the top Gate Result. The current findings are at the end.

3. Fix the findings by following PROJECT_GUIDELINES.md, in particular its "Responding to Independent Review Findings" and "Version Control" sections. They are binding: stop and ask rather than deciding a product question, amend approved artifacts before changing code, never edit a verification test, and stage explicit paths.

4. Run gofmt, go vet, go build ./..., go test -count=1 ./..., and go test -race -short -count=1 ./... Report precisely which packages fail and why.

5. Write a revision summary under the unit code/ directory naming each finding and what changed, then commit and push.
PROMPT
}

FIX_PROMPT=${FIX_PROMPT:-$(default_fix_prompt)}
FIX_PROMPT=${FIX_PROMPT//@REVIEW_FILE@/$REVIEW_FILE}
REVIEW_PROMPT=${REVIEW_PROMPT:-"Production coding is complete and pushed. Read ${ACTIVE_HANDOFF}, then run its independent review and required tests. Follow the independent review/test ownership boundary. Update ${REVIEW_FILE} with findings and an explicit Gate Result, then commit and push the review and test changes."}

is_blocked() {
    herdr agent wait "$1" --until blocked --timeout 50 >/dev/null 2>&1
}

show_blocked_context() {
    local agent=$1

    echo
    echo "Blocked: $agent needs human input. Recent pane output follows:" >&2
    herdr agent read "$agent" --source recent-unwrapped --lines 80 --format text >&2 || true
    echo >&2
    echo "Resolve the question in that pane, then run this script again." >&2
}

ensure_not_blocked() {
    local agent=$1

    if is_blocked "$agent"; then
        show_blocked_context "$agent"
        exit 20
    fi
}

wait_for_existing_turn() {
    local agent=$1
    local label=$2

    echo "Waiting for $label ($agent) to settle..."
    if ! herdr agent wait "$agent" --timeout "$TIMEOUT_MS"; then
        echo "Error: timed out or failed while waiting for $agent." >&2
        exit 21
    fi
    ensure_not_blocked "$agent"
    echo "$label is ready."
}

run_agent() {
    local agent=$1
    local label=$2
    local prompt=$3

    ensure_not_blocked "$agent"
    echo
    echo "Sending task to $label ($agent)..."
    if ! herdr agent prompt "$agent" "$prompt" --wait --timeout "$TIMEOUT_MS"; then
        echo "Error: $agent did not complete its turn successfully." >&2
        exit 21
    fi
    ensure_not_blocked "$agent"
    echo "$label completed its turn."
}

file_signature() {
    if [[ -f "$REVIEW_FILE" ]]; then
        cksum "$REVIEW_FILE" | awk '{ print $1 ":" $2 }'
    else
        printf '%s\n' missing
    fi
}

gate_is_pass_for_revision() {
    local production_revision=$1

    [[ -f "$REVIEW_FILE" ]] || return 1

    awk -v revision="$production_revision" '
        /^## Gate Result[[:space:]]*$/ { in_gate = 1; next }
        in_gate && /^##[[:space:]]/ { exit }
        in_gate && /^\*\*PASS/ { found = 1 }
        in_gate && index($0, revision) { cites_revision = 1 }
        END { exit(found && cites_revision ? 0 : 1) }
    ' "$REVIEW_FILE"
}

if [[ "$INITIAL_MODE" == "start" ]]; then
    run_agent "$CODING_AGENT" "Coding agent" "$PRODUCTION_PROMPT"
else
    wait_for_existing_turn "$CODING_AGENT" "Coding agent"
fi

for (( cycle = 1; cycle <= MAX_CYCLES; cycle++ )); do
    echo
    echo "Review cycle $cycle of $MAX_CYCLES"
    production_revision=$(git rev-parse --short HEAD)
    review_before=$(file_signature)
    review_task="${REVIEW_PROMPT} In the first ## Gate Result section, cite production revision ${production_revision} explicitly."
    run_agent "$REVIEW_AGENT" "Independent review agent" "$review_task"
    review_after=$(file_signature)

    if [[ "$review_before" == "$review_after" ]]; then
        echo "Warning: $REVIEW_FILE was not updated during review cycle $cycle." >&2
    elif gate_is_pass_for_revision "$production_revision"; then
        echo
        echo "PASS: the review agent recorded a fresh passing gate for production revision $production_revision in $REVIEW_FILE."
        exit 0
    fi

    if (( cycle == MAX_CYCLES )); then
        echo >&2
        echo "FAIL: no fresh PASS was recorded after $MAX_CYCLES review cycles." >&2
        echo "Inspect $REVIEW_FILE and the two panes for the remaining findings." >&2
        exit 1
    fi

    run_agent "$CODING_AGENT" "Coding agent fix" "$FIX_PROMPT"
done
