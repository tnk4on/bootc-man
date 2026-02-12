package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const (
	completionDescription = `Generate shell autocompletions for bootc-man.

Valid arguments are bash, zsh, fish, and powershell.

To load completions:

Bash:
  $ source <(bootc-man completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ bootc-man completion bash > /etc/bash_completion.d/bootc-man
  # macOS:
  $ bootc-man completion bash > $(brew --prefix)/etc/bash_completion.d/bootc-man

Zsh:
  # If shell completion is not already enabled in your environment,
  # you will need to enable it. You can execute the following once:
  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ bootc-man completion zsh > "${fpath[1]}/_bootc-man"

  # You will need to start a new shell for this setup to take effect.

  # Or, for macOS with Homebrew:
  $ bootc-man completion zsh > $(brew --prefix)/share/zsh/site-functions/_bootc-man

fish:
  $ bootc-man completion fish | source

  # To load completions for each session, execute once:
  $ bootc-man completion fish > ~/.config/fish/completions/bootc-man.fish

PowerShell:
  PS> bootc-man completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> bootc-man completion powershell > bootc-man.ps1
  # and source this file from your PowerShell profile.`
)

var (
	completionFile string
	completionNoDesc bool
	completionShells = []string{"bash", "zsh", "fish", "powershell"}
	completionCmd = &cobra.Command{
		Use:       fmt.Sprintf("completion [options] {%s}", strings.Join(completionShells, "|")),
		Short:     "Generate shell autocompletions",
		Long:      completionDescription,
		ValidArgs: completionShells,
		Args:      cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE:      completionRun,
		Example: `  bootc-man completion bash
  bootc-man completion zsh -f _bootc-man
  bootc-man completion fish --no-desc`,
	}
)

func init() {
	flags := completionCmd.Flags()
	flags.StringVarP(&completionFile, "file", "f", "",
		"Output the completion to file rather than stdout")
	flags.BoolVar(&completionNoDesc, "no-desc", false,
		"Don't include descriptions in the completion output")
}

func completionRun(cmd *cobra.Command, args []string) error {
	var w io.Writer

	if completionFile != "" {
		f, err := os.Create(completionFile)
		if err != nil {
			return fmt.Errorf("failed to create file %s: %w", completionFile, err)
		}
		defer f.Close()
		w = f
	} else {
		w = os.Stdout
	}

	var err error
	shell := args[0]
	switch shell {
	case "bash":
		err = cmd.Root().GenBashCompletionV2(w, !completionNoDesc)
	case "zsh":
		if completionNoDesc {
			err = cmd.Root().GenZshCompletionNoDesc(w)
		} else {
			err = cmd.Root().GenZshCompletion(w)
		}
	case "fish":
		err = cmd.Root().GenFishCompletion(w, !completionNoDesc)
	case "powershell":
		if completionNoDesc {
			err = cmd.Root().GenPowerShellCompletion(w)
		} else {
			err = cmd.Root().GenPowerShellCompletionWithDesc(w)
		}
	default:
		return fmt.Errorf("unsupported shell: %s", shell)
	}

	if err != nil {
		return fmt.Errorf("failed to generate %s completion: %w", shell, err)
	}

	if completionFile != "" {
		fmt.Fprintf(os.Stderr, "Completion script written to %s\n", completionFile)
	}

	return nil
}
