package app

import "fmt"

// command is one CLI verb. Keeping name, aliases, help and handler in a single table gives one
// source of truth that dispatch, the help text, and shell completion all read from. The handler
// signature matches the existing command methods exactly, so they slot in as method expressions
// (e.g. (*App).accounts) with no change to their bodies.
type command struct {
	name    string
	aliases []string
	short   string
	run     func(*App, []string) error
	hidden  bool // kept out of help/completion (version, help)
}

func (a *App) commands() []command {
	return []command{
		{name: "login", short: "Sign in via the browser and save an API key", run: (*App).login},
		{name: "accounts", aliases: []string{"acc", "ls"}, short: "List your accounts with balances", run: (*App).accounts},
		{name: "transactions", aliases: []string{"tx"}, short: "Show an account's statement (current account if none given)", run: (*App).transactions},
		{name: "use", short: "Set or pick the current account", run: (*App).use},
		{name: "sync", short: "Pull fresh transactions for one account or --all", run: (*App).sync},
		{name: "connections", aliases: []string{"conn"}, short: "List your bank connections", run: (*App).connections},
		{name: "banks", short: "List banks available to connect (--country)", run: (*App).banks},
		{name: "key", short: "Import your encryption key (key import [<file>])", run: (*App).key},
		{name: "completion", short: "Output a shell completion script (bash|zsh|fish)", run: (*App).completion},
		{name: "version", aliases: []string{"--version", "-v"}, short: "Print the openbanking version", run: (*App).versionCmd, hidden: true},
		{name: "help", aliases: []string{"-h", "--help"}, short: "Show this help", run: (*App).helpCmd, hidden: true},
	}
}

// lookup finds a command by its name or any of its aliases.
func (a *App) lookup(name string) (command, bool) {
	for _, c := range a.commands() {
		if c.name == name {
			return c, true
		}
		for _, al := range c.aliases {
			if al == name {
				return c, true
			}
		}
	}
	return command{}, false
}

func (a *App) versionCmd([]string) error {
	fmt.Fprintf(a.stdout(), "openbanking %s\n", a.versionString())
	return nil
}

func (a *App) helpCmd([]string) error {
	a.usage()
	return nil
}
