package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/branchbase/branchbase/internal/config"
	"github.com/branchbase/branchbase/internal/git"
)

// Root commands advertised to shells. Includes doctor so completion stays
// aligned with the CLI roadmap even before that command lands.
var completionRootCommands = []string{
	"init",
	"status",
	"proxy",
	"switch",
	"list",
	"prune",
	"tui",
	"hooks",
	"version",
	"doctor",
	"completion",
	"help",
}

func runCompletion(args []string) {
	if err := writeCompletionScript(os.Stdout, args); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func writeCompletionScript(w io.Writer, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: branchbase completion [bash|zsh|fish|powershell]")
	}
	var script string
	switch args[0] {
	case "bash":
		script = bashCompletionScript
	case "zsh":
		script = zshCompletionScript
	case "fish":
		script = fishCompletionScript
	case "powershell", "pwsh":
		script = powershellCompletionScript
	default:
		return fmt.Errorf("unknown shell %q; use bash, zsh, fish, or powershell", args[0])
	}
	_, err := io.WriteString(w, script)
	return err
}

// runComplete is a hidden helper the generated scripts call for dynamic values.
// Usage: branchbase __complete <kind>
func runComplete(cwd string, args []string) {
	_ = writeComplete(os.Stdout, cwd, args)
}

func writeComplete(w io.Writer, cwd string, args []string) error {
	kind := "commands"
	if len(args) > 0 {
		kind = args[0]
	}
	var lines []string
	switch kind {
	case "commands":
		lines = append([]string{}, completionRootCommands...)
	case "switch":
		lines = completeSwitchBranches(cwd)
	case "hooks":
		lines = []string{"install", "uninstall", "status"}
	case "shells":
		lines = []string{"bash", "zsh", "fish", "powershell"}
	default:
		return nil
	}
	for _, line := range lines {
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

func completeSwitchBranches(cwd string) []string {
	seen := make(map[string]struct{})
	var out []string
	add := func(name string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}

	if branches, err := git.ResolveLocalBranches(cwd); err == nil {
		for _, b := range branches {
			add(b)
		}
	}

	if cfg, err := config.LoadConfig(cwd); err == nil {
		if drv, err := getDriverForConfig(cfg, true); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
			list, listErr := drv.ListBranches(ctx)
			cancel()
			_ = drv.Close()
			if listErr == nil {
				for _, b := range list {
					add(b.Name)
				}
			}
		}
	}

	sort.Strings(out)
	return out
}

const bashCompletionScript = `# bash completion for branchbase
# Install: source <(branchbase completion bash)
# or: branchbase completion bash > /usr/local/etc/bash_completion.d/branchbase

_branchbase() {
  local cur prev cmd
  COMPREPLY=()
  cur="${COMP_WORDS[COMP_CWORD]}"
  prev="${COMP_WORDS[COMP_CWORD-1]}"
  cmd="${COMP_WORDS[1]}"

  if [[ ${COMP_CWORD} -eq 1 ]]; then
    local commands
    commands="$(branchbase __complete commands 2>/dev/null)"
    if [[ -z "${commands}" ]]; then
      commands="init status proxy switch list prune tui hooks version doctor completion help"
    fi
    COMPREPLY=( $(compgen -W "${commands}" -- "${cur}") )
    return 0
  fi

  case "${cmd}" in
    switch)
      if [[ ${COMP_CWORD} -eq 2 ]]; then
        local branches
        branches="$(branchbase __complete switch 2>/dev/null)"
        COMPREPLY=( $(compgen -W "${branches} --no-create" -- "${cur}") )
      elif [[ ${COMP_CWORD} -ge 3 ]]; then
        COMPREPLY=( $(compgen -W "--no-create" -- "${cur}") )
      fi
      ;;
    hooks)
      COMPREPLY=( $(compgen -W "$(branchbase __complete hooks 2>/dev/null)" -- "${cur}") )
      ;;
    completion)
      COMPREPLY=( $(compgen -W "$(branchbase __complete shells 2>/dev/null)" -- "${cur}") )
      ;;
    init)
      COMPREPLY=( $(compgen -W "--skip-hooks --no-hooks" -- "${cur}") )
      ;;
    status|list)
      COMPREPLY=( $(compgen -W "--json" -- "${cur}") )
      ;;
    prune)
      COMPREPLY=( $(compgen -W "--dry-run --force -f -y" -- "${cur}") )
      ;;
  esac
}

complete -F _branchbase branchbase
`

const zshCompletionScript = `#compdef branchbase
# zsh completion for branchbase
# Install: branchbase completion zsh > "${fpath[1]}/_branchbase"

_branchbase() {
  local -a commands
  local cmd
  commands=(
    'init:Initialize BranchBase in the current repository'
    'status:Show current Git branch, target database, and proxy status'
    'proxy:Start the local transparent TCP routing proxy'
    'switch:Switch or provision an isolated database for a branch'
    'list:List active and ephemeral databases'
    'prune:Delete databases for merged or deleted Git branches'
    'tui:Launch the interactive terminal UI dashboard'
    'hooks:Install, uninstall, or inspect Git hooks'
    'version:Print the BranchBase version'
    'doctor:Run environment diagnostics'
    'completion:Print a shell completion script'
    'help:Show help'
  )

  if (( CURRENT == 2 )); then
    _describe -t commands 'branchbase command' commands
    return
  fi

  cmd=${words[2]}
  case $cmd in
    switch)
      local -a branches
      branches=(${(f)"$(branchbase __complete switch 2>/dev/null)"})
      _arguments \
        '--no-create[Do not provision a missing database]' \
        '1:branch:($branches)'
      ;;
    hooks)
      _arguments '1:action:(install uninstall status)'
      ;;
    completion)
      _arguments '1:shell:(bash zsh fish powershell)'
      ;;
    init)
      _arguments '--skip-hooks[Skip Git hook installation]' '--no-hooks[Skip Git hook installation]'
      ;;
    status|list)
      _arguments '--json[JSON output]'
      ;;
    prune)
      _arguments '--dry-run[Show what would be deleted]' '--force[Skip confirmation]' '-f[Skip confirmation]' '-y[Skip confirmation]'
      ;;
    *)
      _default
      ;;
  esac
}

_branchbase "$@"
`

const fishCompletionScript = `# fish completion for branchbase
# Install: branchbase completion fish > ~/.config/fish/completions/branchbase.fish

function __branchbase_complete_switch
    branchbase __complete switch 2>/dev/null
end

complete -c branchbase -f

complete -c branchbase -n "__fish_use_subcommand" -a init -d "Initialize BranchBase in the current repository"
complete -c branchbase -n "__fish_use_subcommand" -a status -d "Show current Git branch, target database, and proxy status"
complete -c branchbase -n "__fish_use_subcommand" -a proxy -d "Start the local transparent TCP routing proxy"
complete -c branchbase -n "__fish_use_subcommand" -a switch -d "Switch or provision an isolated database for a branch"
complete -c branchbase -n "__fish_use_subcommand" -a list -d "List active and ephemeral databases"
complete -c branchbase -n "__fish_use_subcommand" -a prune -d "Delete databases for merged or deleted Git branches"
complete -c branchbase -n "__fish_use_subcommand" -a tui -d "Launch the interactive terminal UI dashboard"
complete -c branchbase -n "__fish_use_subcommand" -a hooks -d "Install, uninstall, or inspect Git hooks"
complete -c branchbase -n "__fish_use_subcommand" -a version -d "Print the BranchBase version"
complete -c branchbase -n "__fish_use_subcommand" -a doctor -d "Run environment diagnostics"
complete -c branchbase -n "__fish_use_subcommand" -a completion -d "Print a shell completion script"
complete -c branchbase -n "__fish_use_subcommand" -a help -d "Show help"

complete -c branchbase -n "__fish_seen_subcommand_from switch" -a "(__branchbase_complete_switch)"
complete -c branchbase -n "__fish_seen_subcommand_from switch" -l no-create -d "Do not provision a missing database"

complete -c branchbase -n "__fish_seen_subcommand_from hooks" -a "install uninstall status"

complete -c branchbase -n "__fish_seen_subcommand_from completion" -a "bash zsh fish powershell"

complete -c branchbase -n "__fish_seen_subcommand_from init" -l skip-hooks -d "Skip Git hook installation"
complete -c branchbase -n "__fish_seen_subcommand_from init" -l no-hooks -d "Skip Git hook installation"

complete -c branchbase -n "__fish_seen_subcommand_from status" -l json -d "JSON output"
complete -c branchbase -n "__fish_seen_subcommand_from list" -l json -d "JSON output"

complete -c branchbase -n "__fish_seen_subcommand_from prune" -l dry-run -d "Show what would be deleted"
complete -c branchbase -n "__fish_seen_subcommand_from prune" -l force -d "Skip confirmation"
complete -c branchbase -n "__fish_seen_subcommand_from prune" -s f -d "Skip confirmation"
complete -c branchbase -n "__fish_seen_subcommand_from prune" -s y -d "Skip confirmation"
`

const powershellCompletionScript = `# powershell completion for branchbase
# Install: branchbase completion powershell | Out-String | Invoke-Expression

Register-ArgumentCompleter -Native -CommandName branchbase -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)

    $commands = @('init','status','proxy','switch','list','prune','tui','hooks','version','doctor','completion','help')
    $elements = @($commandAst.CommandElements | ForEach-Object { $_.Extent.Text })

    if ($elements.Count -le 1) {
        $commands | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
        }
        return
    }

    $cmd = $elements[1]
    switch ($cmd) {
        'switch' {
            $branches = @()
            try { $branches = @(branchbase __complete switch 2>$null) } catch { }
            $opts = $branches + @('--no-create')
            $opts | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
            }
        }
        'hooks' {
            @('install','uninstall','status') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
            }
        }
        'completion' {
            @('bash','zsh','fish','powershell') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
            }
        }
        'init' {
            @('--skip-hooks','--no-hooks') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
            }
        }
        { $_ -in @('status','list') } {
            @('--json') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
            }
        }
        'prune' {
            @('--dry-run','--force','-f','-y') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
            }
        }
    }
}
`
