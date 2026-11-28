package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"git.kwkaiser.io/kwkaiser/tmt/internal/tmt"
)

var version = "dev"

const usage = `usage:
  tmt create
  tmt new [-d] [-B branch] [--prompt text | --prompt-file path] <base> <topic> [repo...]
  tmt add [-t topic] <repo...>
  tmt open [-d] [--prompt text | --prompt-file path] [topic]
  tmt ls [-p] [--json]
  tmt status [-t topic] <running|blocked|finished> [message...]
  tmt fresh [base]
  tmt done [-f] [-y] [topic]
  tmt close [-y]
  tmt gc [-y]
  tmt pick
  tmt switch

base is a directory or zoxide query; tasks live under $TMT_ROOT/<base name>/<base name>-<topic>
`

var errUsage = errors.New("usage")

type app interface {
	New(ctx context.Context, o tmt.NewOptions) error
	Add(ctx context.Context, topic string, repos []string) error
	Open(ctx context.Context, o tmt.OpenOptions) error
	Ls(ctx context.Context, o tmt.LsOptions) error
	SetAgentStatus(ctx context.Context, o tmt.StatusOptions) error
	Fresh(ctx context.Context, query string) error
	Done(ctx context.Context, o tmt.DoneOptions) error
	Close(ctx context.Context, yes bool) error
	GC(ctx context.Context, yes bool) error
	Pick(ctx context.Context) error
	Switch(ctx context.Context) error
	Create(ctx context.Context) error
	Fill(ctx context.Context, topic string, repos []string) (string, error)
	Preview(ctx context.Context, kind, dir, name string) error
	Complete(ctx context.Context, kind, arg string) error
}

var execShell = func(command string) error {
	return syscall.Exec("/bin/sh", []string{"sh", "-c", "clear && " + command}, os.Environ())
}

type factory func() (app, func() error, error)

type action func(ctx context.Context, a app, args []string) error

func argsBetween(min, max int, use string) cobra.PositionalArgs {
	return func(_ *cobra.Command, args []string) error {
		if len(args) < min || (max >= 0 && len(args) > max) {
			return fmt.Errorf("usage: tmt %s", use)
		}
		return nil
	}
}

func arg(args []string, i int) string {
	if i < len(args) {
		return args[i]
	}
	return ""
}

type promptFlags struct {
	text  string
	file  string
	stdin io.Reader
}

func (p *promptFlags) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&p.text, "prompt", "", "initial instruction for the task agent")
	cmd.Flags().StringVar(&p.file, "prompt-file", "", "read the initial instruction from a file, or - for stdin")
	cmd.MarkFlagsMutuallyExclusive("prompt", "prompt-file")
	cmd.PreRun = func(cmd *cobra.Command, _ []string) { p.stdin = cmd.InOrStdin() }
}

func (p *promptFlags) read() (string, error) {
	var data []byte
	var err error
	switch p.file {
	case "":
		return strings.TrimSpace(p.text), nil
	case "-":
		data, err = io.ReadAll(p.stdin)
	default:
		data, err = os.ReadFile(p.file)
	}
	if err != nil {
		return "", fmt.Errorf("read prompt: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

func newRootCmd(build factory) *cobra.Command {
	with := func(f action) func(*cobra.Command, []string) error {
		return func(cmd *cobra.Command, args []string) (err error) {
			a, cleanup, err := build()
			if err != nil {
				return err
			}
			if cleanup != nil {
				defer func() {
					if cerr := cleanup(); err == nil {
						err = cerr
					}
				}()
			}
			return f(cmd.Context(), a, args)
		}
	}

	sub := func(use, short string, min, max int, f action) *cobra.Command {
		return &cobra.Command{
			Use:   use,
			Short: short,
			Args:  argsBetween(min, max, use),
			RunE:  with(f),
		}
	}

	root := &cobra.Command{
		Use:               "tmt",
		Short:             "manage multi-repo worktree tasks",
		Version:           version,
		SilenceUsage:      true,
		SilenceErrors:     true,
		Args:              cobra.ArbitraryArgs,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		RunE: func(cmd *cobra.Command, _ []string) error {
			fmt.Fprint(cmd.ErrOrStderr(), usage)
			return errUsage
		},
	}
	root.SetHelpFunc(func(cmd *cobra.Command, _ []string) {
		fmt.Fprint(cmd.OutOrStdout(), usage)
	})

	root.AddCommand(sub("create", "interactively pick a base, topic and repos for a new task", 0, 0,
		func(ctx context.Context, a app, _ []string) error { return a.Create(ctx) }))

	var branch string
	var newDetach bool
	var newPrompt promptFlags
	newCmd := sub("new [-d] [-B branch] [--prompt text | --prompt-file path] <base> <topic> [repo...]", "create a task with worktrees for the given repos", 2, -1,
		func(ctx context.Context, a app, args []string) error {
			prompt, err := newPrompt.read()
			if err != nil {
				return err
			}
			return a.New(ctx, tmt.NewOptions{Base: args[0], Topic: args[1], Branch: branch, Repos: args[2:], Prompt: prompt, Detach: newDetach})
		})
	newCmd.Flags().StringVarP(&branch, "branch", "B", "", "branch name")
	newCmd.Flags().BoolVarP(&newDetach, "detach", "d", false, "create the session without attaching to it")
	newPrompt.register(newCmd)
	root.AddCommand(newCmd)

	var addTopic string
	addCmd := sub("add [-t topic] <repo...>", "turn repos in a task into worktrees", 1, -1,
		func(ctx context.Context, a app, args []string) error { return a.Add(ctx, addTopic, args) })
	addCmd.Flags().StringVarP(&addTopic, "topic", "t", "", "task topic")
	root.AddCommand(addCmd)

	var openDetach bool
	var openPrompt promptFlags
	openCmd := sub("open [-d] [--prompt text | --prompt-file path] [topic]", "open the session for a task", 0, 1,
		func(ctx context.Context, a app, args []string) error {
			prompt, err := openPrompt.read()
			if err != nil {
				return err
			}
			return a.Open(ctx, tmt.OpenOptions{Topic: arg(args, 0), Prompt: prompt, Detach: openDetach})
		})
	openCmd.Flags().BoolVarP(&openDetach, "detach", "d", false, "start the session without attaching to it")
	openPrompt.register(openCmd)
	root.AddCommand(openCmd)

	var ls tmt.LsOptions
	lsCmd := sub("ls [-p] [--json]", "list tasks and worktree status", 0, 0,
		func(ctx context.Context, a app, _ []string) error { return a.Ls(ctx, ls) })
	lsCmd.Flags().BoolVarP(&ls.PR, "pr", "p", false, "include PR state")
	lsCmd.Flags().BoolVar(&ls.JSON, "json", false, "print tasks as JSON")
	root.AddCommand(lsCmd)

	var statusTopic string
	statusCmd := sub("status [-t topic] <running|blocked|finished> [message...]", "record the agent status of a task", 1, -1,
		func(ctx context.Context, a app, args []string) error {
			return a.SetAgentStatus(ctx, tmt.StatusOptions{Topic: statusTopic, State: tmt.AgentState(args[0]), Message: strings.Join(args[1:], " ")})
		})
	statusCmd.Flags().StringVarP(&statusTopic, "topic", "t", "", "task topic")
	root.AddCommand(statusCmd)

	root.AddCommand(sub("fresh [base]", "fetch and fast-forward the default branch of every repo in a base", 0, 1,
		func(ctx context.Context, a app, args []string) error { return a.Fresh(ctx, arg(args, 0)) }))

	var done tmt.DoneOptions
	doneCmd := sub("done [-f] [-y] [topic]", "remove a task and its worktrees", 0, 1,
		func(ctx context.Context, a app, args []string) error {
			o := done
			o.Topic = arg(args, 0)
			return a.Done(ctx, o)
		})
	doneCmd.Flags().BoolVarP(&done.Force, "force", "f", false, "remove even with uncommitted or unpushed work")
	doneCmd.Flags().BoolVarP(&done.Yes, "yes", "y", false, "never prompt; refuse instead of offering to force")
	root.AddCommand(doneCmd)

	var closeYes bool
	closeCmd := sub("close [-y]", "close the current session, running tmt done if it belongs to a task", 0, 0,
		func(ctx context.Context, a app, _ []string) error { return a.Close(ctx, closeYes) })
	closeCmd.Flags().BoolVarP(&closeYes, "yes", "y", false, "never prompt; refuse instead of offering to force")
	root.AddCommand(closeCmd)

	var gcYes bool
	gcCmd := sub("gc [-y]", "remove tasks whose PRs are merged or closed", 0, 0,
		func(ctx context.Context, a app, _ []string) error { return a.GC(ctx, gcYes) })
	gcCmd.Flags().BoolVarP(&gcYes, "yes", "y", false, "remove without confirming")
	root.AddCommand(gcCmd)
	root.AddCommand(sub("pick", "pick a repo in the current task, or a task", 0, 0,
		func(ctx context.Context, a app, _ []string) error { return a.Pick(ctx) }))
	root.AddCommand(sub("switch", "pick any task and open its session", 0, 0,
		func(ctx context.Context, a app, _ []string) error { return a.Switch(ctx) }))

	fillCmd := sub("_fill <topic> [repo...]", "", 1, -1,
		func(ctx context.Context, a app, args []string) error {
			command, err := a.Fill(ctx, args[0], args[1:])
			if err != nil {
				return err
			}
			return execShell(command)
		})
	fillCmd.Hidden = true
	fillCmd.DisableFlagParsing = true
	root.AddCommand(fillCmd)

	previewCmd := sub("_preview <kind> <dir> [name]", "", 2, 3,
		func(ctx context.Context, a app, args []string) error {
			return a.Preview(ctx, args[0], args[1], arg(args, 2))
		})
	previewCmd.Hidden = true
	previewCmd.DisableFlagParsing = true
	root.AddCommand(previewCmd)

	completeCmd := sub("_complete <kind> [arg]", "", 1, 2,
		func(ctx context.Context, a app, args []string) error {
			return a.Complete(ctx, args[0], arg(args, 1))
		})
	completeCmd.Hidden = true
	completeCmd.DisableFlagParsing = true
	root.AddCommand(completeCmd)

	return root
}

func run(ctx context.Context, cmd *cobra.Command, args []string, stderr io.Writer) int {
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(ctx)
	switch {
	case err == nil:
		return 0
	case errors.Is(err, errUsage):
		return 1
	}
	fmt.Fprintf(stderr, "tmt: %v\n", err)
	if errors.Is(err, tmt.ErrUnsafe) {
		return 2
	}
	return 1
}

func Execute() int {
	if err := applyBundled(bundledPath, bundledEnv, os.Getenv, os.LookupEnv, os.Setenv); err != nil {
		fmt.Fprintf(os.Stderr, "tmt: %v\n", err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return run(ctx, newRootCmd(newApp), os.Args[1:], os.Stderr)
}
