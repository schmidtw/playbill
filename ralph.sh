#!/usr/bin/bash
set -e

usage() {
    cat <<EOF
Usage: $(basename "$0") <PRD_ISSUE> [MAX_CYCLES]

Automatically work through child issues of a PRD using Claude Code.

Arguments:
  PRD_ISSUE    GitHub issue number for the PRD (required)
  MAX_CYCLES   Maximum number of work cycles to run (default: 10)

Options:
  -h, --help   Show this help message and exit

Examples:
  $(basename "$0") 42         # Work PRD #42, up to 10 cycles
  $(basename "$0") 42 5       # Work PRD #42, up to 5 cycles
EOF
    exit "${1:-0}"
}

# Handle help flag anywhere in args
for arg in "$@"; do
    case "$arg" in
        -h|--help) usage 0 ;;
    esac
done

if [ $# -lt 1 ] || [ $# -gt 2 ]; then
    echo "Error: expected 1 or 2 arguments, got $#" >&2
    echo >&2
    usage 1 >&2
fi

# Validate that arguments are positive integers
for arg in "$@"; do
    if ! [[ "$arg" =~ ^[1-9][0-9]*$ ]]; then
        echo "Error: '$arg' is not a valid positive integer" >&2
        echo >&2
        usage 1 >&2
    fi
done

PRD=$1
MAX_CYCLES=${2:-10}

# Get the current GitHub username for issue assignment
GH_USER=$(gh api user --jq '.login')

# Build the prompt with available issue list
make_prompt() {
    local available_issues=$1
    cat <<EOF
You are working on PRD #$PRD. Your GitHub username is $GH_USER.

The following child issues are AVAILABLE (open, unassigned, and all blockers closed):
$available_issues

This list is pre-sorted by issue number and pre-filtered — any issue whose
"Blocked by #N" is still open has already been removed. Issues assigned to
someone are being worked on by another agent; do NOT touch them.

Do the following:
1. Fetch the PRD for context: gh issue view $PRD
2. Read each available issue (gh issue view) to understand its acceptance criteria, blocked-by status, and scope.                                        
3. From the available issues, pick the single highest-priority one to work on. Prioritize:                                                               
   - Tracer bullets / foundational slices                                                                                                                
   - Issues that unblock other issues                                                                                                                    
   - Earlier vertical slices over later ones 
4. Claim the issue by assigning it to yourself: gh issue edit <number> --add-assignee $GH_USER
5. Implement the issue, using tdd-go skill, following the acceptance criteria.
6. Verify tests pass: go test ./...
7. Ensure test coverage of 80%+ for each package using go test ./... -cover
8. Ensure golangci-lint run passes with no errors.
9. Work on a per-issue branch.
   - Before starting implementation:
     git checkout main && git pull && git checkout -b ralph/issue-<number>
   - Commit as work progresses. tdd-go naturally produces several small
     commits (red/green/refactor) — KEEP them. The commit sequence is the
     record and we do NOT squash.
   - DO NOT add authorship or co-authorship lines to commit messages.
   - Every commit message must include a line "Issue #<number>" at the bottom.
10. Push the branch and open a pull request:
     git push -u origin ralph/issue-<number>
     gh pr create --base main --head ralph/issue-<number> \
       --title "<short summary of the change>" \
       --body "Fixes #<number>

<brief summary of what was done>"
11. Enable auto-merge with a regular merge commit (NOT squash — the
    individual commits are the record):
     gh pr merge --auto --merge
    The PR will merge itself once required checks pass; "Fixes #<number>"
    in the body will auto-close the issue at that moment.
12. Add a comment on the PRD issue (#$PRD) noting what was completed, the
    PR URL, and any context the next agent should know.

Failure recovery — if at any point you cannot complete all acceptance
criteria in this cycle (tests won't pass after reasonable attempts, lint
errors you cannot resolve, a tool prompts for confirmation you cannot give,
a missing dependency, scope turns out larger than one cycle, etc.):

- DO NOT merge a partial or broken PR.
- DO NOT close the issue.
- If you have NOT yet pushed anything: throw away the branch
  (git checkout main && git branch -D ralph/issue-<number>), unassign
  yourself, comment on the issue, exit.
- If you have pushed and opened a PR but cannot finish: convert the PR to
  draft so auto-merge will not fire (gh pr ready --undo), add a comment on
  the PR explaining the blocker, unassign yourself on the issue, comment
  on the issue pointing at the draft PR, exit.
- Unassign command: gh issue edit <number> --remove-assignee $GH_USER
- The goal: the next cycle should be able to pick the issue up cleanly
  (or at minimum, a human reviewing the draft PR knows what to do).
- DO NOT add any AI/Claude attribution footer to PR bodies, issue
    comments, code comments, README/docs, or any other artifact. 

ONLY WORK ON A SINGLE ISSUE.
If all child issues are complete after this work close the PRD issue.
EOF
}

for cycle in $(seq 1 $MAX_CYCLES); do
    echo ""
    echo "========================================="
    echo "  Ralph cycle $cycle / $MAX_CYCLES"
    echo "========================================="

    # Stop if the PRD issue is already closed
    PRD_STATE=$(gh issue view "$PRD" --json state --jq '.state')
    if [ "$PRD_STATE" = "CLOSED" ]; then
        echo "✅ PRD #$PRD is closed — all done!"
        exit 0
    fi

    # Fetch all open child issues with their bodies so we can evaluate
    # "Blocked by #N" references.
    ALL_OPEN=$(gh issue list --state open --search "Parent PRD #$PRD in:body" \
        --json number,title,assignees,body)

    # Available = unassigned AND every "Blocked by #N" reference points to a
    # closed issue (i.e., N is not in the currently-open set).
    AVAILABLE=$(echo "$ALL_OPEN" | jq '
        [.[].number] as $open
        | [ .[]
            | select(.assignees | length == 0)
            | . as $i
            | ([$i.body | scan("(?i)Blocked by #[0-9]+") | split("#") | .[1] | tonumber]) as $blockers
            | select($blockers | all(. as $b | $open | index($b) | not))
          ]
        | sort_by(.number)
    ')

    AVAILABLE_COUNT=$(echo "$AVAILABLE" | jq 'length')
    if [ "$AVAILABLE_COUNT" = "0" ]; then
        TOTAL_OPEN=$(echo "$ALL_OPEN" | jq 'length')
        IN_PROGRESS=$(echo "$ALL_OPEN" | jq '[.[] | select(.assignees | length > 0)] | length')
        UNASSIGNED_BLOCKED=$(echo "$ALL_OPEN" | jq '[.[] | select(.assignees | length == 0)] | length')

        if [ "$TOTAL_OPEN" = "0" ]; then
            echo "✅ No open child issues remain — closing PRD #$PRD"
            gh issue close "$PRD" --comment "All child slices completed and merged. Closing automatically."
            exit 0
        fi

        if [ "$UNASSIGNED_BLOCKED" -gt 0 ]; then
            echo "⏳ $UNASSIGNED_BLOCKED unassigned issue(s) waiting on open blockers; $IN_PROGRESS in progress."
        else
            echo "⏳ No available issues — $IN_PROGRESS issue(s) in progress."
        fi

        # Wait for in-progress PRs to merge (which closes their issues via
        # "Fixes #N") so either all issues clear and we close the PRD, or
        # blockers clear and new work becomes available.
        # Set RALPH_WAIT_TIMEOUT=0 to skip waiting and exit immediately.
        WAIT_TIMEOUT=${RALPH_WAIT_TIMEOUT:-1800}
        POLL=${RALPH_POLL_INTERVAL:-30}

        if [ "$WAIT_TIMEOUT" = "0" ]; then
            echo "   RALPH_WAIT_TIMEOUT=0 — exiting without waiting."
            exit 0
        fi

        echo "   Waiting up to ${WAIT_TIMEOUT}s for in-progress work to settle (poll every ${POLL}s)..."
        elapsed=0
        while [ "$elapsed" -lt "$WAIT_TIMEOUT" ]; do
            sleep "$POLL"
            elapsed=$(( elapsed + POLL ))

            NEW_OPEN=$(gh issue list --state open --search "Parent PRD #$PRD in:body" \
                --json number,assignees,body)
            NEW_TOTAL=$(echo "$NEW_OPEN" | jq 'length')

            if [ "$NEW_TOTAL" = "0" ]; then
                echo "✅ All child issues closed — closing PRD #$PRD"
                gh issue close "$PRD" --comment "All child slices completed and merged. Closing automatically."
                exit 0
            fi

            # Re-evaluate availability with the same blocker-aware filter as
            # the main loop so we resume work when blockers clear.
            NEW_AVAILABLE=$(echo "$NEW_OPEN" | jq '
                [.[].number] as $open
                | [ .[]
                    | select(.assignees | length == 0)
                    | . as $i
                    | ([$i.body | scan("(?i)Blocked by #[0-9]+") | split("#") | .[1] | tonumber]) as $blockers
                    | select($blockers | all(. as $b | $open | index($b) | not))
                  ]
            ')
            NEW_AVAILABLE_COUNT=$(echo "$NEW_AVAILABLE" | jq 'length')
            NEW_IN_PROGRESS=$(echo "$NEW_OPEN" | jq '[.[] | select(.assignees | length > 0)] | length')

            if [ "$NEW_AVAILABLE_COUNT" -gt 0 ]; then
                echo "↻ Work available again ($NEW_AVAILABLE_COUNT issue(s)) — resuming"
                continue 2
            fi

            echo "   ${elapsed}s/${WAIT_TIMEOUT}s — $NEW_TOTAL open ($NEW_IN_PROGRESS in progress)"
        done

        echo "⚠️  Timeout after ${WAIT_TIMEOUT}s waiting for in-progress work."
        echo "    $NEW_TOTAL issue(s) still open. Re-run when the situation clears."
        exit 1
    fi

    # Format the available issues as a readable list for Claude
    ISSUE_LIST=$(echo "$AVAILABLE" | jq -r '.[] | "  - #\(.number): \(.title)"')

    echo "  $AVAILABLE_COUNT available issue(s):"
    echo "$ISSUE_LIST"
    echo "  Handing off to Claude..."

    PROMPT=$(make_prompt "$ISSUE_LIST")

    claude --permission-mode acceptEdits \
        --allowedTools 'Bash(gh *),Bash(go *),Bash(git *),Bash(golangci-lint *),Bash(ls *),Bash(rm -- *),Read,Edit,Write,Glob,Grep' \
        -p --verbose \
        "$PROMPT"
done

echo ""
echo "⚠️  Reached max cycles ($MAX_CYCLES) without closing PRD #$PRD."
exit 1
