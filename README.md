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

# any session
tmt window api                        # focus the window/tab named api, opening it in $PWD if missing
tmt window -s w1 -C ~/src/api api -- nvim +12 main.go
                                      # run in an idle shell pane of that window, else split it
```

### Driving tmt from another agent

Every lifecycle command can run without a TTY:

```sh
tmt new -d --prompt-file plan.md work auth api web   # create without attaching; the agent starts with the prompt
tmt open -d --prompt "continue" work-auth            # restart a stopped session with a new prompt
tmt ls --json -p                                     # tasks, repos, session up/down, agent status
tmt status -t work-auth blocked "need API keys"      # record running|blocked|finished for ls --json
tmt done -y work-auth                                # exit 2 instead of prompting when work is unpushed
tmt gc -y                                            # clean up without confirming
tmt close -y
```

The prompt is passed to the agent command as its first argument and recorded under `## Initial prompt` in `CLAUDE.md`/`AGENTS.md`. Agent status starts as `creating`, becomes `running` once worktrees exist (or `blocked` if that fails), and is otherwise set with `tmt status`.

## Task dev shell

Repos whose `flake.nix` defines a `devShell` are combined into `<task>/.tmt-flake`, with every repo following the newest locked `nixpkgs`. The task `.envrc` loads it via `use flake`, refreshing `.tmt-flake/flake.lock` only when a repo's `flake.nix`/`flake.lock` is newer than it, repeats each repo's `dotenv`/`dotenv_if_exists` lines, and is `direnv allow`ed; the root agent window runs under `direnv exec`. Each allowed `.envrc` (task root and new worktrees) is warmed with a detached `direnv exec <dir> true` so the dev shell is cached before a shell opens. Conflicting tools resolve to a single version; open a repo's own window for its exact toolchain.

## Home Manager

The flake exports `homeModules.default` (also `homeManagerModules.default`), which installs tmt with zsh completions and writes `config.yaml`:

```nix
{
  inputs.tmt.url = "git+https://git.kwkaiser.io/kwkaiser/tmt";

  # in a home-manager module
  imports = [ inputs.tmt.homeModules.default ];

  programs.tmt = {
    enable = true;
    environment.TMT_BRANCH_PREFIX = "me/";
    settings = {
      agent = "claude";
      agents.claude = "ccpm";
      dirs."~/Documents/personal/website" = "codex";
    };
  };
}
```

`environment` is baked into the binary and applies only to variables unset at runtime. `tmuxPackage` defaults to `programs.tmux.package`. When the stylix home-manager module is enabled, tmt's colors and fzf pickers follow its palette; set `programs.tmt.stylix.enable = false` to opt out.

## Configuration

| Variable            | Default                               |
| ------------------- | ------------------------------------- |
| `TMT_ROOT`          | `~/tasks`                             |
| `TMT_BRANCH_PREFIX` | `u/kwkaiser/`                         |
| `TMT_CLONE`         | `node_modules .env .env.local .env.mcp` (copied into new worktrees) |
| `TMT_AGENT`         | `claude` (overrides `agent` in the config file) |
| `TMT_CLAUDE_COMMAND`| overrides `agents.claude` in the config file |
| `TMT_CONFIG`        | `$XDG_CONFIG_HOME/tmt/config.yaml`    |

### Agents

The task root window runs a coding agent: `claude` (writes `CLAUDE.md` and `.mcp.json`) or `codex` (writes `AGENTS.md` and trusts the task in `$CODEX_HOME/config.toml`). Pick the default, override commands, and map bases to agents in `config.yaml`; the longest matching `dirs` prefix of a task's base wins.

```yaml
agent: claude
agents:
  claude: ccpm
  codex: codex --dangerously-bypass-approvals-and-sandbox
dirs:
  ~/Documents/personal/website: codex
```
