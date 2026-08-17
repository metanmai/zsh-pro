#!/usr/bin/env bash
set -euo pipefail

spike_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
session="zsh-pro-spike-001-verify-$$"
verify_root=$(mktemp -d /tmp/zsh-pro-spike-001-verify.XXXXXX)
helper="$verify_root/bin/spike-sync"
state_root="$verify_root/state"

cleanup() {
  tmux kill-session -t "$session" 2>/dev/null || true
}
trap cleanup EXIT

fail() {
  printf 'FAIL: %s\n' "$1" >&2
  printf '%s\n' '--- pane A ---' >&2
  tmux capture-pane -p -J -t "$session":0.0 -S -80 >&2 2>/dev/null || true
  printf '%s\n' '--- pane B ---' >&2
  tmux capture-pane -p -J -t "$session":0.1 -S -80 >&2 2>/dev/null || true
  exit 1
}

pane_output() {
  tmux capture-pane -p -J -t "$session":0."$1" -S -1000
}

wait_line() {
  local pane=$1 expected=$2 output
  for _attempt in {1..200}; do
    output=$(pane_output "$pane")
    if grep -Fqx -- "$expected" <<<"$output"; then
      return 0
    fi
    sleep 0.05
  done
  fail "pane $pane did not print: $expected"
}

wait_prefix() {
  local pane=$1 prefix=$2 output
  for _attempt in {1..300}; do
    output=$(pane_output "$pane")
    if grep -q "^${prefix}" <<<"$output"; then
      return 0
    fi
    sleep 0.05
  done
  fail "pane $pane did not print prefix: $prefix"
}

send_wait() {
  local pane=$1 command=$2 expected=$3
  tmux send-keys -t "$session":0."$pane" "$command" Enter
  wait_line "$pane" "$expected"
}

json_shell_field() {
  local shell=$1 expression=$2
  "$helper" status --root "$state_root" --json | jq -r ".shells[] | select(.shell==\"$shell\") | $expression"
}

wait_shell_field() {
  local shell=$1 expression=$2 expected=$3 actual
  for _attempt in {1..200}; do
    actual=$(json_shell_field "$shell" "$expression")
    [[ $actual == "$expected" ]] && return 0
    sleep 0.05
  done
  fail "shell $shell field $expression was $actual, expected $expected"
}

printf '%s\n' '[1/8] starting retained-terminal-compatible harness'
"$spike_dir/demo.sh" --detached --session "$session" --root "$verify_root" >/dev/null
[[ -x $helper ]] || fail 'helper was not built'

printf '%s\n' '[2/8] lifecycle boundary and semantic categories'
send_wait 0 "export SPIKE_OK=one; export SPIKE_FAILED=kept; false; builtin print -r -- '@@A_BASE_DONE@@'" '@@A_BASE_DONE@@'
send_wait 1 "builtin print -r -- \"@@ENV=\${SPIKE_OK:-missing}/\${SPIKE_FAILED:-missing}@@\"" '@@ENV=one/kept@@'

send_wait 0 "source =(builtin print -r -- \$'export SPIKE_SOURCED=from-source\\nspike_fn() { builtin print -r -- function-ok; }'); alias spike_hi='builtin print -r -- alias-ok'; path=(\"/tmp/spike path\" \$path); setopt AUTO_CD; export SPIKE_MULTILINE=\$'alpha\\nbeta'; builtin print -r -- '@@A_CATEGORIES_DONE@@'" '@@A_CATEGORIES_DONE@@'
send_wait 1 "spike_hi; spike_fn; _multi=bad; [[ \$SPIKE_MULTILINE == \$'alpha\\nbeta' ]] && _multi=ok; builtin print -r -- \"@@CATEGORY=\${SPIKE_SOURCED:-missing}|\$path[1]|\$options[autocd]|\$_multi@@\"" '@@CATEGORY=from-source|/tmp/spike path|on|ok@@'
wait_line 1 'alias-ok'
wait_line 1 'function-ok'

send_wait 0 "unalias spike_hi; unfunction spike_fn; unset SPIKE_SOURCED SPIKE_MULTILINE; path=(\${path[2,-1]}); unsetopt AUTO_CD; builtin print -r -- '@@A_REMOVALS_DONE@@'" '@@A_REMOVALS_DONE@@'
send_wait 1 "builtin print -r -- \"@@REMOVED=\${+aliases[spike_hi]}/\${+functions[spike_fn]}/\${+SPIKE_SOURCED}/\$path[1]/\$options[autocd]@@\"" '@@REMOVED=0/0/0/'"${PATH%%:*}"'/off@@'

printf '%s\n' '[3/8] secret/bookkeeping exclusion and foreground safety'
send_wait 0 "export SPIKE_API_TOKEN='super-secret-value'; export ZP_INTERNAL_SHOULD_NOT_SYNC='internal-value'; builtin print -r -- '@@A_EXCLUSION_DONE@@'" '@@A_EXCLUSION_DONE@@'
send_wait 1 "builtin print -r -- \"@@EXCLUDED=\${SPIKE_API_TOKEN:-missing}/\${ZP_INTERNAL_SHOULD_NOT_SYNC:-missing}@@\"" '@@EXCLUDED=missing/missing@@'
if rg -n 'super-secret-value|internal-value' "$state_root" >/dev/null; then
  fail 'an excluded value reached state or logs'
fi
rg -q '"name":"SPIKE_API_TOKEN","reason":"secret-name"' "$state_root/events.jsonl" || fail 'secret exclusion was not observable'
rg -q '"name":"ZP_INTERNAL_SHOULD_NOT_SYNC","reason":"bookkeeping"' "$state_root/events.jsonl" || fail 'bookkeeping exclusion was not observable'

tmux send-keys -t "$session":0.0 "sleep 0.6; builtin print -r -- \"@@FOREGROUND_DURING=\${SPIKE_FOREGROUND:-missing}@@\"" Enter
sleep 0.15
send_wait 1 "export SPIKE_FOREGROUND=remote; builtin print -r -- '@@B_FOREGROUND_PUBLISHED@@'" '@@B_FOREGROUND_PUBLISHED@@'
wait_line 0 '@@FOREGROUND_DURING=missing@@'
send_wait 0 "builtin print -r -- \"@@FOREGROUND_AFTER=\${SPIKE_FOREGROUND:-missing}@@\"" '@@FOREGROUND_AFTER=remote@@'

printf '%s\n' '[4/8] manual mode and explicit sync'
send_wait 1 "_zp_spike_set_auto_apply off; builtin print -r -- '@@B_MANUAL@@'" '@@B_MANUAL@@'
send_wait 0 "export SPIKE_MANUAL=waiting; builtin print -r -- '@@A_MANUAL_PUBLISHED@@'" '@@A_MANUAL_PUBLISHED@@'
send_wait 1 "builtin print -r -- \"@@MANUAL_BEFORE=\${SPIKE_MANUAL:-missing}@@\"" '@@MANUAL_BEFORE=missing@@'
wait_shell_field B '.behind' true
send_wait 1 "_zp_spike_sync; builtin print -r -- \"@@MANUAL_AFTER=\${SPIKE_MANUAL:-missing}@@\"" '@@MANUAL_AFTER=waiting@@'
wait_shell_field B '.behind' false

printf '%s\n' '[5/8] stale disjoint composition and same-identity conflict'
send_wait 0 "export SPIKE_LEFT=A; builtin print -r -- '@@A_LEFT@@'" '@@A_LEFT@@'
send_wait 1 "export SPIKE_RIGHT=B; builtin print -r -- '@@B_RIGHT@@'" '@@B_RIGHT@@'
for _attempt in {1..200}; do
  if jq -e '.entries | to_entries | map(select(.value.name=="SPIKE_LEFT" or .value.name=="SPIKE_RIGHT")) | length == 2' "$state_root/state.json" >/dev/null; then
    break
  fi
  sleep 0.05
done
jq -e '.entries | to_entries | map(select(.value.name=="SPIKE_LEFT" or .value.name=="SPIKE_RIGHT")) | length == 2' "$state_root/state.json" >/dev/null || fail 'disjoint stale writes did not compose'
send_wait 1 "_zp_spike_sync; builtin print -r -- '@@B_COMPOSE_SYNCED@@'" '@@B_COMPOSE_SYNCED@@'
send_wait 0 "builtin print -r -- \"@@A_COMPOSE=\$SPIKE_LEFT/\$SPIKE_RIGHT@@\"" '@@A_COMPOSE=A/B@@'

race_gate="$verify_root/race-gate"
tmux send-keys -t "$session":0.0 "export SPIKE_RACE=from-A; while [[ ! -e $race_gate ]]; do sleep 0.01; done; builtin print -r -- '@@A_RACE_DONE@@'" Enter
tmux send-keys -t "$session":0.1 "export SPIKE_RACE=from-B; while [[ ! -e $race_gate ]]; do sleep 0.01; done; builtin print -r -- '@@B_RACE_DONE@@'" Enter
sleep 0.15
touch "$race_gate"
wait_line 0 '@@A_RACE_DONE@@'
wait_line 1 '@@B_RACE_DONE@@'
for _attempt in {1..200}; do
  status_json=$("$helper" status --root "$state_root" --json)
  conflict_count=$(jq '[.shells[] | select(.conflict != null)] | length' <<<"$status_json")
  [[ $conflict_count == 1 ]] && break
  sleep 0.05
done
[[ $conflict_count == 1 ]] || fail "expected one visible same-key conflict, got $conflict_count"
conflict_shell=$(jq -r '.shells[] | select(.conflict != null) | .shell' <<<"$status_json")
conflict_pane=0
[[ $conflict_shell == B ]] && conflict_pane=1
send_wait "$conflict_pane" "_zp_spike_resolve_shared; builtin print -r -- '@@CONFLICT_RESOLVED@@'" '@@CONFLICT_RESOLVED@@'
[[ $(json_shell_field "$conflict_shell" '.conflict // "none"') == none ]] || fail 'conflict did not clear after explicit shared resolution'

printf '%s\n' '[6/8] fail-open missing helper, lock timeout, parser rejection, interrupted write'
send_wait 0 "typeset -g ZP_SPIKE_HELPER=/definitely/missing/spike-sync; builtin print -r -- '@@HELPER_DISABLED@@'" '@@HELPER_DISABLED@@'
send_wait 0 "builtin print -r -- \"@@MISSING_HELPER_RAN=\$ZP_SPIKE_LAST_HOOK_STATUS@@\"; typeset -g ZP_SPIKE_HELPER=$helper" '@@MISSING_HELPER_RAN=127@@'

ready_file="$verify_root/lock-ready"
"$helper" hold-lock --root "$state_root" --duration 1s --ready "$ready_file" &
holder_pid=$!
for _attempt in {1..100}; do [[ -f $ready_file ]] && break; sleep 0.01; done
send_wait 0 "builtin print -r -- \"@@LOCK_TIMEOUT_RAN=\$ZP_SPIKE_LAST_HOOK_STATUS@@\"" '@@LOCK_TIMEOUT_RAN=124@@'
wait "$holder_pid"
send_wait 0 "builtin print -r -- '@@RECOVERED_AFTER_LOCK@@'" '@@RECOVERED_AFTER_LOCK@@'

revision_before=$(jq -r '.revision' "$state_root/state.json")
if builtin printf 'malformed' | "$helper" cycle --root "$state_root" --shell A --apply=true >/dev/null 2>&1; then
  fail 'malformed snapshot was accepted'
fi
[[ $(jq -r '.revision' "$state_root/state.json") == "$revision_before" ]] || fail 'malformed snapshot mutated shared state'
"$helper" inject-partial --root "$state_root"
"$helper" status --root "$state_root" >/dev/null || fail 'interrupted temp write corrupted authoritative state'

printf '%s\n' '[7/8] bounded no-op performance and no-Git steady-state path'
tmux send-keys -t "$session":0.0 "zmodload zsh/datetime; _bench_start=\$EPOCHREALTIME; repeat 50 _zp_spike_cycle; _bench_elapsed=\$(( EPOCHREALTIME - _bench_start )); builtin print -r -- \"@@BENCH50=\$_bench_elapsed@@\"" Enter
wait_prefix 0 '@@BENCH50='
bench_line=$(pane_output 0 | grep '^@@BENCH50=' | tail -1)
bench_seconds=${bench_line#@@BENCH50=}
bench_seconds=${bench_seconds%@@}
awk -v seconds="$bench_seconds" 'BEGIN { exit !(seconds < 3.0) }' || fail "50 no-op cycles took ${bench_seconds}s"
summary_json=$("$helper" summary --root "$state_root")
[[ $(jq -r '.p95_micros < 100000' <<<"$summary_json") == true ]] || fail 'helper p95 exceeded 100ms'
if rg -n 'os/exec|exec\.Command|\bgit\b' "$spike_dir/main.go" >/dev/null; then
  fail 'helper steady-state source contains a subprocess/Git path'
fi

printf '%s\n' '[8/8] final evidence'
final_status=$("$helper" status --root "$state_root" --json)
final_summary=$("$helper" summary --root "$state_root")
printf 'PASS: spike 001 verification complete\n'
printf 'VERIFY_ROOT=%s\n' "$verify_root"
printf 'BENCH50_SECONDS=%s\n' "$bench_seconds"
printf 'STATUS=%s\n' "$final_status"
printf 'SUMMARY=%s\n' "$final_summary"
