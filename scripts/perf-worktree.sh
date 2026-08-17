#!/usr/bin/env bash
set -euo pipefail

cycles=50
if [[ $# -gt 0 ]]; then
	if [[ $# -ne 2 || $1 != --cycles || ! $2 =~ ^[1-9][0-9]*$ ]]; then
		echo "usage: perf-worktree.sh [--cycles <positive-count>]" >&2
		exit 2
	fi
	cycles=$2
fi

binary=${ZSHPRO_BIN:?set ZSHPRO_BIN to an absolute production zsh-pro binary}
case "$binary" in
	/*) ;;
	*) echo "ZSHPRO_BIN must be absolute" >&2; exit 2 ;;
esac
[[ -x $binary ]] || { echo "ZSHPRO_BIN is not executable: $binary" >&2; exit 1; }
zsh_binary=$(command -v zsh)
repo_dir=$(cd "$(dirname "$0")/.." && pwd)
system_path=$PATH

perf_root=$(mktemp -d)
home_dir=$perf_root/home
runtime_root=$perf_root/runtime
bin_dir=$perf_root/bin
shim_dir=$perf_root/shims
duration_file=$perf_root/durations-ms
sorted_file=$perf_root/durations-sorted-ms
git_counter=$perf_root/git-calls
zsh_counter=$perf_root/child-zsh-calls
mkdir -m 700 "$home_dir" "$bin_dir" "$shim_dir"
trap 'rm -rf "$perf_root"' EXIT

source_file=$home_dir/perf-source.zsh
printf '%s\n' 'export DEMO_SHARED_ENV=ready' "alias demo_seed='print -r -- seeded'" > "$source_file"
HOME=$home_dir ZDOTDIR=$home_dir ZSHPRO_HOME=$runtime_root PATH=$system_path \
	"$binary" ingest "$source_file" >/dev/null
loader=$runtime_root/loader.zsh
[[ -r $loader ]] || { echo "installed loader missing" >&2; exit 1; }
ln -s "$binary" "$bin_dir/zsh-pro"

printf '%s\n' '#!/bin/sh' 'printf x >> "$ZP_PERF_GIT_COUNTER"' 'exit 97' > "$shim_dir/git"
printf '%s\n' '#!/bin/sh' 'printf x >> "$ZP_PERF_ZSH_COUNTER"' 'exit 97' > "$shim_dir/zsh"
chmod 700 "$shim_dir/git" "$shim_dir/zsh"

perf_script='source "$1" || exit 10
PATH="$2:$PATH"
rehash
_zp_worktree_precmd
_zp_worktree_line_finish
[[ $ZP_WORKTREE_ATTACHED == 1 && $ZP_WORKTREE_APPLIED_REVISION == 1 && $DEMO_SHARED_ENV == ready ]] || {
  print -u2 -r -- "initial convergence failed: attached=$ZP_WORKTREE_ATTACHED applied=$ZP_WORKTREE_APPLIED_REVISION value=${DEMO_SHARED_ENV-unset} error=${(q)ZP_WORKTREE_LAST_ERROR}"
  exit 11
}
zmodload zsh/datetime || exit 12
PATH="$3:$2:/usr/bin:/bin"
rehash
: > "$4"
integer index
float started elapsed
for (( index = 0; index < $5; index++ )); do
  started=$EPOCHREALTIME
  _zp_worktree_line_finish || exit 13
  elapsed=$(( (EPOCHREALTIME - started) * 1000.0 ))
  printf "%.6f\n" $elapsed >> "$4"
done
print -r -- "$ZP_WORKTREE_APPLIED_REVISION|${+ZP_WORKTREE_REPLY_COMPLETE}|${#ZP_WORKTREE_LAST_ERROR}"'

shell_result=$(/usr/bin/env -i HOME=$home_dir ZDOTDIR=$home_dir ZSHPRO_HOME=$runtime_root \
	ZP_PERF_GIT_COUNTER=$git_counter ZP_PERF_ZSH_COUNTER=$zsh_counter \
	PATH=$bin_dir:/usr/bin:/bin TERM=dumb LC_ALL=C \
	"$zsh_binary" -f -c "$perf_script" worktree-perf "$loader" "$bin_dir" "$shim_dir" "$duration_file" "$cycles")
[[ $shell_result == "1|0|0" ]] || { echo "no-op shell did not remain converged: $shell_result" >&2; exit 1; }

measured_count=$(wc -l < "$duration_file")
[[ $measured_count -eq $cycles ]] || { echo "measured $measured_count cycles, want $cycles" >&2; exit 1; }
git_calls=0
child_zsh_calls=0
[[ ! -f $git_counter ]] || git_calls=$(wc -c < "$git_counter")
[[ ! -f $zsh_counter ]] || child_zsh_calls=$(wc -c < "$zsh_counter")
[[ $git_calls -eq 0 && $child_zsh_calls -eq 0 ]] || {
	echo "no-op cycles spawned git=$git_calls child-zsh=$child_zsh_calls" >&2
	exit 1
}

LC_ALL=C sort -n "$duration_file" > "$sorted_file"
awk '
  { values[NR] = $1; total += $1 }
  END {
    p50 = int((NR * 50 + 99) / 100)
    p95 = int((NR * 95 + 99) / 100)
    printf "count=%d p50_ms=%.6f p95_ms=%.6f max_ms=%.6f total_ms=%.6f\n", NR, values[p50], values[p95], values[NR], total
  }
' "$sorted_file"
printf 'git_calls=%d child_zsh_calls=%d\n' "$git_calls" "$child_zsh_calls"

state_bytes=$(wc -c < "$runtime_root/worktree.json")
(( state_bytes <= 16777216 )) || { echo "combined state exceeded cap: $state_bytes" >&2; exit 1; }
status=$(HOME=$home_dir ZDOTDIR=$home_dir ZSHPRO_HOME=$runtime_root PATH=$system_path "$binary" status)
grep -qx 'revision: 1' <<< "$status"
grep -qx 'dirty identities: 0' <<< "$status"
printf 'state_bytes=%d revision=1 dirty=0\n' "$state_bytes"

(
	cd "$repo_dir"
	GOTOOLCHAIN=local go test ./core/cli ./core/shell/zsh \
		-run 'Test(WorktreeFakeClockCumulative249And251Milliseconds|WorktreeDeadlineRealStageMarginsFailOpen|WorktreeTransitionFakeClockCumulative249And251|WorktreeTransitionRealTime25And500MillisecondMargins)$' \
		-count=1
)
printf '%s\n' 'deadline_checks=fake-249/251,real-25/500'
