package app

import (
	"fmt"
	"strings"
)

// globalOpts holds the flags that apply to every command, parsed out of the argument list before
// dispatch.
type globalOpts struct {
	output  string // raw --output/-o value ("" = auto)
	noColor bool
	config  string // overrides the credentials path when non-empty
}

// parseGlobals pulls the global flags (-o/--output, --no-color, --config) out of args wherever they
// appear and returns the remaining args (command name plus command-specific flags) untouched. Only
// these specific flags are recognized, so per-command flags such as --from or --all pass straight
// through to their own FlagSet — letting commands keep parsing exactly as they do today.
func parseGlobals(args []string) (rest []string, opt globalOpts, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-o" || arg == "--output":
			if i+1 >= len(args) {
				return nil, opt, fmt.Errorf("flag %s needs a value (table, json, or csv)", arg)
			}
			opt.output = args[i+1]
			i++
		case strings.HasPrefix(arg, "-o="):
			opt.output = strings.TrimPrefix(arg, "-o=")
		case strings.HasPrefix(arg, "--output="):
			opt.output = strings.TrimPrefix(arg, "--output=")
		case arg == "--no-color":
			opt.noColor = true
		case arg == "--config":
			if i+1 >= len(args) {
				return nil, opt, fmt.Errorf("flag --config needs a path")
			}
			opt.config = args[i+1]
			i++
		case strings.HasPrefix(arg, "--config="):
			opt.config = strings.TrimPrefix(arg, "--config=")
		default:
			rest = append(rest, arg)
		}
	}
	return rest, opt, nil
}
