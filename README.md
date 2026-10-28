# tmt

Multi-repo oriented worktree tool with some baked in multiplexer functionality 

## Usage

A *base* is a directory of repos (path or zoxide query). A *task* is a topic within a base: chosen repos get worktrees on a fresh branch, the rest are linked in, and a multiplexer session is opened.

```sh
# worktrees for api + web on branch u/kwkaiser/auth-1, opened in session work-auth
tmt new work auth api web
tmt new -B fix/login work login api   # explicit branch

tmt create                            # same, but pick base/topic/repos interactively

# inside a task
tmt add infra                         # promote another repo to a worktree
tmt pick                              # jump to a repo in this task
tmt close                             # close session, cleaning up the task

# across tasks
tmt ls -p                             # tasks, worktree status, PR state
tmt switch                            # fuzzy-pick any task and open it
tmt open work-auth
tmt fresh work                        # fetch + fast-forward default branches
tmt done work-auth                    # remove task (asks to force if work is unpushed; -f to force, -y to refuse)
tmt gc                                # remove tasks whose PRs are merged/closed
```

## Configuration

| Variable            | Default                               |
| ------------------- | ------------------------------------- |
| `TMT_ROOT`          | `~/tasks`                             |
| `TMT_BRANCH_PREFIX` | `u/kwkaiser/`                         |
| `TMT_CLONE`         | `node_modules .env .env.local .env.mcp` (copied into new worktrees) |
