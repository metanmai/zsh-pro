#!/usr/bin/env bash
set -euo pipefail

spike_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
session=zsh-pro-spike-001
demo_root=
attach=1

while (($#)); do
  case "$1" in
    --session)
      session=$2
      shift 2
      ;;
    --root)
      demo_root=$2
      shift 2
      ;;
    --detached)
      attach=0
      shift
      ;;
    *)
      printf 'unknown argument: %s\n' "$1" >&2
      exit 2
      ;;
  esac
done

if [[ -z $demo_root ]]; then
  demo_root=$(mktemp -d /tmp/zsh-pro-spike-001.XXXXXX)
fi
mkdir -p "$demo_root/bin"

(cd "$spike_dir" && go build -o "$demo_root/bin/spike-sync" .)
"$demo_root/bin/spike-sync" init --root "$demo_root/state"

if tmux has-session -t "$session" 2>/dev/null; then
  printf 'tmux session already exists: %s\n' "$session" >&2
  exit 1
fi

tmux new-session -d -s "$session" -x 140 -y 36 'zsh -f'
tmux split-window -h -t "$session":0 'zsh -f'
tmux select-layout -t "$session":0 even-horizontal >/dev/null

shell_quote() {
  printf '%q' "$1"
}

hooks_q=$(shell_quote "$spike_dir/hooks.zsh")
root_q=$(shell_quote "$demo_root/state")
helper_q=$(shell_quote "$demo_root/bin/spike-sync")

for pane in 0 1; do
  shell_id=A
  ((pane == 1)) && shell_id=B
  tmux send-keys -t "$session":0."$pane" "typeset -g ZP_SPIKE_ROOT=$root_q ZP_SPIKE_HELPER=$helper_q ZP_SPIKE_SHELL=$shell_id; source $hooks_q; _zp_spike_attach; PS1='$shell_id> '; clear" Enter
done

for attempt in {1..100}; do
  pane_a=$(tmux capture-pane -p -t "$session":0.0 -S -5)
  pane_b=$(tmux capture-pane -p -t "$session":0.1 -S -5)
  if [[ $pane_a == *'A> '* && $pane_b == *'B> '* ]]; then
    break
  fi
  sleep 0.05
done

printf 'session=%s\nroot=%s\nhelper=%s\n' "$session" "$demo_root/state" "$demo_root/bin/spike-sync"

if ((attach)); then
  exec tmux attach-session -t "$session"
fi
