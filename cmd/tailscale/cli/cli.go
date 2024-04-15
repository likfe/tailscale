// Copyright (c) Tailscale Inc & AUTHORS
// SPDX-License-Identifier: BSD-3-Clause

// Package cli contains the cmd/tailscale CLI code in a package that can be included
// in other wrapper binaries such as the Mac and Windows clients.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"strings"
	"sync"
	"text/tabwriter"

	"github.com/mattn/go-colorable"
	"github.com/mattn/go-isatty"
	"github.com/peterbourgon/ff/v3/ffcli"
	"tailscale.com/client/tailscale"
	"tailscale.com/envknob"
	"tailscale.com/paths"
	"tailscale.com/version/distro"
)

var Stderr io.Writer = os.Stderr
var Stdout io.Writer = os.Stdout

func errf(format string, a ...any) {
	fmt.Fprintf(Stderr, format, a...)
}

func printf(format string, a ...any) {
	fmt.Fprintf(Stdout, format, a...)
}

// outln is like fmt.Println in the common case, except when Stdout is
// changed (as in js/wasm).
//
// It's not named println because that looks like the Go built-in
// which goes to stderr and formats slightly differently.
func outln(a ...any) {
	fmt.Fprintln(Stdout, a...)
}

func newFlagSet(name string) *flag.FlagSet {
	onError := flag.ExitOnError
	if runtime.GOOS == "js" {
		onError = flag.ContinueOnError
	}
	fs := flag.NewFlagSet(name, onError)
	fs.SetOutput(Stderr)
	return fs
}

// CleanUpArgs rewrites command line arguments for simplicity and backwards compatibility.
// In particular, it rewrites --authkey to --auth-key.
func CleanUpArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		// Rewrite --authkey to --auth-key, and --authkey=x to --auth-key=x,
		// and the same for the -authkey variant.
		switch {
		case arg == "--authkey", arg == "-authkey":
			arg = "--auth-key"
		case strings.HasPrefix(arg, "--authkey="), strings.HasPrefix(arg, "-authkey="):
			arg = strings.TrimLeft(arg, "-")
			arg = strings.TrimPrefix(arg, "authkey=")
			arg = "--auth-key=" + arg
		}
		out = append(out, arg)
	}
	return out
}

var localClient tailscale.LocalClient

// Run runs the CLI. The args do not include the binary name.
func Run(args []string) (err error) {
	args = CleanUpArgs(args)

	if len(args) == 1 && (args[0] == "-V" || args[0] == "--version") {
		args = []string{"version"}
	}

	var warnOnce sync.Once
	tailscale.SetVersionMismatchHandler(func(clientVer, serverVer string) {
		warnOnce.Do(func() {
			fmt.Fprintf(Stderr, "Warning: client version %q != tailscaled server version %q\n", clientVer, serverVer)
		})
	})

	rootCmd := newRootCmd()
	if err := rootCmd.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}

	if envknob.Bool("TS_DUMP_HELP") {
		walkCommands(rootCmd, func(c *ffcli.Command) {
			fmt.Println("===")
			// UsageFuncs are typically called during Command.Run which ensures
			// FlagSet is not nil.
			if c.FlagSet == nil {
				c.FlagSet = flag.NewFlagSet(c.Name, flag.ContinueOnError)
			}
			if c.UsageFunc != nil {
				fmt.Println(c.UsageFunc(c))
			} else {
				fmt.Println(ffcli.DefaultUsageFunc(c))
			}
		})
		return
	}

	localClient.Socket = rootArgs.socket
	rootCmd.FlagSet.Visit(func(f *flag.Flag) {
		if f.Name == "socket" {
			localClient.UseSocketOnly = true
		}
	})

	err = rootCmd.Run(context.Background())
	if tailscale.IsAccessDeniedError(err) && os.Getuid() != 0 && runtime.GOOS != "windows" {
		return fmt.Errorf("%v\n\nUse 'sudo tailscale %s' or 'tailscale up --operator=$USER' to not require root.", err, strings.Join(args, " "))
	}
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	return err
}

func newRootCmd() *ffcli.Command {
	rootfs := newFlagSet("tailscale")
	rootfs.StringVar(&rootArgs.socket, "socket", paths.DefaultTailscaledSocket(), "path to tailscaled socket")

	rootCmd := &ffcli.Command{
		Name:       "tailscale",
		ShortUsage: "tailscale [flags] <subcommand> [command flags]",
		ShortHelp:  "The easiest, most secure way to use WireGuard.",
		LongHelp: strings.TrimSpace(`
For help on subcommands, add --help after: "tailscale status --help".

This CLI is still under active development. Commands and flags will
change in the future.
`),
		Subcommands: []*ffcli.Command{
			upCmd,
			downCmd,
			setCmd,
			loginCmd,
			logoutCmd,
			switchCmd,
			configureCmd,
			netcheckCmd,
			ipCmd,
			statusCmd,
			pingCmd,
			ncCmd,
			sshCmd,
			funnelCmd(),
			serveCmd(),
			versionCmd,
			webCmd,
			fileCmd,
			bugReportCmd,
			certCmd,
			netlockCmd,
			licensesCmd,
			exitNodeCmd(),
			updateCmd,
			whoisCmd,
			debugCmd,
			driveCmd,
		},
		FlagSet: rootfs,
		Exec:    func(context.Context, []string) error { return flag.ErrHelp },
	}
	if envknob.UseWIPCode() {
		rootCmd.Subcommands = append(rootCmd.Subcommands,
			idTokenCmd,
		)
	}

	if runtime.GOOS == "linux" && distro.Get() == distro.Synology {
		rootCmd.Subcommands = append(rootCmd.Subcommands, configureHostCmd)
	}

	walkCommands(rootCmd, func(c *ffcli.Command) {
		if c.UsageFunc == nil {
			c.UsageFunc = usageFunc
		}
	})
	return rootCmd
}

func fatalf(format string, a ...any) {
	if Fatalf != nil {
		Fatalf(format, a...)
		return
	}
	log.SetFlags(0)
	log.Fatalf(format, a...)
}

// Fatalf, if non-nil, is used instead of log.Fatalf.
var Fatalf func(format string, a ...any)

var rootArgs struct {
	socket string
}

func walkCommands(cmd *ffcli.Command, f func(*ffcli.Command)) {
	f(cmd)
	for _, sub := range cmd.Subcommands {
		walkCommands(sub, f)
	}
}

// usageFuncNoDefaultValues is like usageFunc but doesn't print default values.
func usageFuncNoDefaultValues(c *ffcli.Command) string {
	return usageFuncOpt(c, false)
}

func usageFunc(c *ffcli.Command) string {
	return usageFuncOpt(c, true)
}

func usageFuncOpt(c *ffcli.Command, withDefaults bool) string {
	var b strings.Builder
	const hiddenPrefix = "HIDDEN: "

	if c.ShortHelp != "" {
		fmt.Fprintf(&b, "%s\n\n", c.ShortHelp)
	}

	fmt.Fprintf(&b, "USAGE\n")
	if c.ShortUsage != "" {
		fmt.Fprintf(&b, "  %s\n", strings.ReplaceAll(c.ShortUsage, "\n", "\n  "))
	} else {
		fmt.Fprintf(&b, "  %s\n", c.Name)
	}
	fmt.Fprintf(&b, "\n")

	if c.LongHelp != "" {
		help, _ := strings.CutPrefix(c.LongHelp, hiddenPrefix)
		fmt.Fprintf(&b, "%s\n\n", help)
	}

	if len(c.Subcommands) > 0 {
		fmt.Fprintf(&b, "SUBCOMMANDS\n")
		tw := tabwriter.NewWriter(&b, 0, 2, 2, ' ', 0)
		for _, subcommand := range c.Subcommands {
			if strings.HasPrefix(subcommand.LongHelp, hiddenPrefix) {
				continue
			}
			fmt.Fprintf(tw, "  %s\t%s\n", subcommand.Name, subcommand.ShortHelp)
		}
		tw.Flush()
		fmt.Fprintf(&b, "\n")
	}

	if countFlags(c.FlagSet) > 0 {
		fmt.Fprintf(&b, "FLAGS\n")
		tw := tabwriter.NewWriter(&b, 0, 2, 2, ' ', 0)
		c.FlagSet.VisitAll(func(f *flag.Flag) {
			var s string
			name, usage := flag.UnquoteUsage(f)
			if strings.HasPrefix(usage, hiddenPrefix) {
				return
			}
			if isBoolFlag(f) {
				s = fmt.Sprintf("  --%s, --%s=false", f.Name, f.Name)
			} else {
				s = fmt.Sprintf("  --%s", f.Name) // Two spaces before --; see next two comments.
				if len(name) > 0 {
					s += " " + name
				}
			}
			// Four spaces before the tab triggers good alignment
			// for both 4- and 8-space tab stops.
			s += "\n    \t"
			s += strings.ReplaceAll(usage, "\n", "\n    \t")

			showDefault := f.DefValue != "" && withDefaults
			// Issue 6766: don't show the default Windows socket path. It's long
			// and distracting. And people on on Windows aren't likely to ever
			// change it anyway.
			if runtime.GOOS == "windows" && f.Name == "socket" && strings.HasPrefix(f.DefValue, `\\.\pipe\ProtectedPrefix\`) {
				showDefault = false
			}
			if showDefault {
				s += fmt.Sprintf(" (default %s)", f.DefValue)
			}

			fmt.Fprintln(&b, s)
		})
		tw.Flush()
		fmt.Fprintf(&b, "\n")
	}

	return strings.TrimSpace(b.String())
}

func isBoolFlag(f *flag.Flag) bool {
	bf, ok := f.Value.(interface {
		IsBoolFlag() bool
	})
	return ok && bf.IsBoolFlag()
}

func countFlags(fs *flag.FlagSet) (n int) {
	fs.VisitAll(func(*flag.Flag) { n++ })
	return n
}

// colorableOutput returns a colorable writer if stdout is a terminal (not, say,
// redirected to a file or pipe), the Stdout writer is os.Stdout (we're not
// embedding the CLI in wasm or a mobile app), and NO_COLOR is not set (see
// https://no-color.org/). If any of those is not the case, ok is false
// and w is Stdout.
func colorableOutput() (w io.Writer, ok bool) {
	if Stdout != os.Stdout ||
		os.Getenv("NO_COLOR") != "" ||
		!isatty.IsTerminal(os.Stdout.Fd()) {
		return Stdout, false
	}
	return colorable.NewColorableStdout(), true
}
