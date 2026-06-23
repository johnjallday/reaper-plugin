// Command reaper-plugin is a small CLI helper for the reaper-plugin skills.
//
// The plugin drives a running REAPER over its Web Remote HTTP interface using
// plain shell (curl) and file operations — no MCP server. This binary exists
// only for the bits that are fiddly to do in shell: editing reaper-kb.ini to
// register ReaScripts (so they get Web Remote command IDs) and pruning stale
// entries. The remaining filesystem/Web Remote operations are also exposed for
// convenience, but the skills generally use shell directly.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/johnjallday/reaper-plugin/internal/reaper"
)

const version = "0.2.1"

// command maps a user-facing subcommand to a reaper.Manager operation.
type command struct {
	op      string
	summary string
	// needsScript indicates the subcommand requires --script.
	needsScript bool
}

var commands = map[string]command{
	// Primary purpose: ReaScript registration helpers (fiddly in shell).
	"register-script": {op: "register_script", summary: "Register a ReaScript in reaper-kb.ini so it gets a Web Remote command ID", needsScript: true},
	"register-all":    {op: "register_all_scripts", summary: "Register every ReaScript found in the Scripts directory"},
	"clean-scripts":   {op: "clean_scripts", summary: "Remove reaper-kb.ini entries whose script files no longer exist"},

	// Convenience read/file operations (skills usually do these in shell).
	"list":    {op: "list", summary: "List ReaScripts in the Scripts directory"},
	"status":  {op: "get_status", summary: "Report whether REAPER is running"},
	"tracks":  {op: "get_tracks", summary: "Print the current project's tracks (via Web Remote)"},
	"port":    {op: "get_web_remote_port", summary: "Print the Web Remote port (from reaper.ini)"},
	"context": {op: "get_context", summary: "Print the open project name/path"},
	"add":     {op: "add", summary: "Write a ReaScript file (requires --script, --content; --type)", needsScript: true},
	"delete":  {op: "delete", summary: "Delete a ReaScript file (requires --script)", needsScript: true},
}

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage(os.Stderr)
		return 2
	}
	switch args[0] {
	case "-h", "--help", "help":
		usage(os.Stdout)
		return 0
	case "-v", "--version", "version":
		fmt.Println(version)
		return 0
	case "install-runner":
		return runInstallRunner()
	case "runner-id":
		return runRunnerID()
	case "exec":
		return runExec(args[1:])
	}

	name := args[0]
	cmd, ok := commands[name]
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", name)
		usage(os.Stderr)
		return 2
	}

	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	script := fs.String("script", "", "script name (with or without extension)")
	content := fs.String("content", "", "script source (for add)")
	scriptType := fs.String("type", "", "script type: lua|eel|py (for add)")
	filename := fs.String("filename", "", "optional full filename")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	if cmd.needsScript && strings.TrimSpace(*script) == "" {
		fmt.Fprintf(os.Stderr, "%s requires --script\n", name)
		return 2
	}

	manager := reaper.NewManagerFromEnv()
	out, err := manager.Execute(reaper.Params{
		Operation:  cmd.op,
		Script:     *script,
		Content:    *content,
		ScriptType: *scriptType,
		Filename:   *filename,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	fmt.Println(out)
	return 0
}

// runInstallRunner performs the one-time runner install + registration.
func runInstallRunner() int {
	out, err := reaper.NewManagerFromEnv().InstallRunner()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	fmt.Println(out)
	return 0
}

// runRunnerID prints the runner's Web Remote command ID (once it has been seeded).
func runRunnerID() int {
	id, err := reaper.NewManagerFromEnv().ReadRunnerID()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	fmt.Println(id)
	return 0
}

// runExec writes Lua to the inbox and triggers the runner so REAPER runs it live.
// Source is --content "<lua>" or --file <path> ("-" reads stdin).
func runExec(args []string) int {
	fs := flag.NewFlagSet("exec", flag.ContinueOnError)
	content := fs.String("content", "", "Lua source to run")
	file := fs.String("file", "", "path to a Lua file to run (\"-\" for stdin)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	lua := *content
	if *file != "" {
		data, err := readSource(*file)
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			return 1
		}
		lua = data
	}
	if strings.TrimSpace(lua) == "" {
		fmt.Fprintln(os.Stderr, "exec requires --content or --file")
		return 2
	}

	out, err := reaper.NewManagerFromEnv().Exec(lua)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	fmt.Println(out)
	return 0
}

func readSource(path string) (string, error) {
	if path == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return string(data), nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // CLI helper reads a user-specified script file by design
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return string(data), nil
}

func usage(w *os.File) {
	fmt.Fprintf(w, `reaper-plugin %s — helper CLI for the reaper-plugin skills

The skills drive REAPER over its Web Remote HTTP interface with plain shell
(curl) and file operations. This binary is an optional helper, mainly for
registering ReaScripts in reaper-kb.ini (which is fiddly to do in shell).

Usage:
  reaper-plugin <command> [flags]

Runner commands (drive REAPER live; see `+"`install-runner`"+` first):
  install-runner   One-time: install + register the runner action in REAPER
  exec             Run Lua in REAPER now (--content "<lua>" or --file <path>|-)
  runner-id        Print the runner's Web Remote command ID (once seeded)

Commands:
`, version)

	names := make([]string, 0, len(commands))
	for n := range commands {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(w, "  %-16s %s\n", n, commands[n].summary)
	}

	fmt.Fprintf(w, `
Flags:
  --script NAME     script name (with or without extension)
  --content STR     script source (for add / exec)
  --file PATH       Lua file to run (for exec; "-" reads stdin)
  --type lua|eel|py script type (for add)
  --filename NAME   optional full filename

Environment:
  REAPER_SCRIPTS_DIR       override the Scripts directory
  REAPER_WEB_REMOTE_PORT   override the Web Remote port (else auto-detected)
  REAPER_MARKETPLACE_URL   marketplace URL shown by marketplace operations
`)
}
