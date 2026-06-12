package app

import (
	"fmt"
	"strings"
)

// completion prints a shell completion script to stdout. The command and alias list is taken from
// the command table, so completions stay in sync with the real commands automatically.
func (a *App) completion(args []string) error {
	shell := ""
	if len(args) > 0 {
		shell = args[0]
	}
	names := a.visibleCommandNames()
	switch shell {
	case "bash":
		fmt.Fprint(a.stdout(), bashCompletion(names))
	case "zsh":
		fmt.Fprint(a.stdout(), zshCompletion(names))
	case "fish":
		fmt.Fprint(a.stdout(), fishCompletion(names))
	default:
		return fmt.Errorf("usage: openbanking completion [bash|zsh|fish]")
	}
	return nil
}

// visibleCommandNames returns the user-facing command names plus aliases, for completion.
func (a *App) visibleCommandNames() []string {
	var names []string
	for _, c := range a.commands() {
		if c.hidden {
			continue
		}
		names = append(names, c.name)
		names = append(names, c.aliases...)
	}
	return names
}

func bashCompletion(names []string) string {
	return fmt.Sprintf(`# openbanking bash completion
# Enable with:  source <(openbanking completion bash)
_openbanking() {
    local cur cmds opts
    cur="${COMP_WORDS[COMP_CWORD]}"
    cmds="%s"
    opts="-o --output --no-color --config -h --help"
    if [ "$COMP_CWORD" -eq 1 ]; then
        COMPREPLY=( $(compgen -W "${cmds}" -- "${cur}") )
    else
        COMPREPLY=( $(compgen -W "${opts}" -- "${cur}") )
    fi
}
complete -F _openbanking openbanking
`, strings.Join(names, " "))
}

func zshCompletion(names []string) string {
	return fmt.Sprintf(`#compdef openbanking
# Enable with:  source <(openbanking completion zsh)
_openbanking() {
    local -a cmds
    cmds=(%s)
    _arguments \
        '(-o --output)'{-o,--output}'[output format]:format:(table json csv)' \
        '--no-color[disable colored output]' \
        '--config[path to credentials file]:file:_files' \
        '1: :->cmd' \
        '*:: :->args'
    if [[ $state == cmd ]]; then
        _describe 'command' cmds
    fi
}
_openbanking "$@"
`, strings.Join(names, " "))
}

func fishCompletion(names []string) string {
	var b strings.Builder
	b.WriteString("# openbanking fish completion\n")
	b.WriteString("# Save to ~/.config/fish/completions/openbanking.fish\n")
	for _, n := range names {
		fmt.Fprintf(&b, "complete -c openbanking -n '__fish_use_subcommand' -a '%s'\n", n)
	}
	b.WriteString("complete -c openbanking -s o -l output -d 'output format' -x -a 'table json csv'\n")
	b.WriteString("complete -c openbanking -l no-color -d 'disable colored output'\n")
	b.WriteString("complete -c openbanking -l config -d 'path to credentials file' -r\n")
	return b.String()
}
