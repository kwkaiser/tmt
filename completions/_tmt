#compdef tmt

_tmt_tasks() {
  local -a tasks
  tasks=(${(f)"$(tmt _complete tasks 2>/dev/null)"})
  _wanted tasks expl task compadd -a tasks
}

_tmt_repos() {
  local -a repos
  repos=(${(f)"$(tmt _complete "$@" 2>/dev/null)"})
  _wanted repos expl repo compadd -F line -a repos
}

_tmt() {
  local curcontext="$curcontext" state state_descr line
  typeset -A opt_args
  local -a subcmds
  subcmds=(
    'create:interactively pick a base, topic and repos for a new task'
    'new:create a task with worktrees for the given repos'
    'add:turn repos in a task into worktrees'
    'open:open the tmux session for a task'
    'ls:list tasks and worktree status'
    'fresh:fetch and fast-forward the default branch of every repo in a base'
    'done:remove a task and its worktrees'
    'close:close the current session, running tmt done if it belongs to a task'
    'gc:remove tasks whose PRs are merged or closed'
    'pick:pick a repo in the current task, or a task'
    'switch:pick any task and open its session'
  )

  if (( CURRENT == 2 )); then
    _describe -t commands 'tmt command' subcmds
    return
  fi

  local cmd="$words[2]"
  shift words
  (( CURRENT-- ))
  curcontext="${curcontext%:*:*}:tmt-$cmd:"

  case "$cmd" in
    new)
      _arguments -s \
        '-B[branch name]:branch: ' \
        '1:base (directory or zoxide query):_directories' \
        '2:topic: ' \
        '*:repo:->repos'
      [[ "$state" == repos ]] && _tmt_repos base-repos "${line[1]}"
      ;;
    add)
      _arguments -s \
        '-t[task]:task:_tmt_tasks' \
        '*:repo:->repos'
      [[ "$state" == repos ]] && _tmt_repos task-repos "${opt_args[-t]:-}"
      ;;
    open)
      _arguments '1:task:_tmt_tasks'
      ;;
    fresh)
      _arguments '1:base (directory or zoxide query):_directories'
      ;;
    ls)
      _arguments '-p[include GitHub PR state]'
      ;;
    done)
      _arguments -s \
        '-f[remove even with uncommitted or unpushed work]' \
        '-y[skip confirmation]' \
        '1:task:_tmt_tasks'
      ;;
  esac
}

_tmt "$@"
