package cli

import "strings"

// commonScanValueFlags is the value-flag base shared by list, search, and
// audit: {offset, limit, diff, commit}. Callers merge in any flags of their
// own (e.g. search's --pattern, audit's --reason/--min-confidence).
func commonScanValueFlags() map[string]bool {
	return map[string]bool{"offset": true, "limit": true, "diff": true, "commit": true}
}

func mergeValueFlags(base map[string]bool, extra map[string]bool) map[string]bool {
	for k, v := range extra {
		base[k] = v
	}
	return base
}

// reorderArgs: a flag listed in valueFlags consumes the next token as its
// value only when not given as "--flag=value"; a bare "--" stops reordering
// and everything after it is passed through verbatim as positional.
func reorderArgs(args []string, valueFlags map[string]bool) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			pos = append(pos, args[i+1:]...)
			break
		}
		if a != "-" && strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			name, hasValue := flagName(a)
			if hasValue {
				continue
			}
			if valueFlags[name] && i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		pos = append(pos, a)
	}
	return append(flags, pos...)
}

func flagName(a string) (name string, hasInlineValue bool) {
	name = strings.TrimLeft(a, "-")
	if idx := strings.IndexByte(name, '='); idx >= 0 {
		return name[:idx], true
	}
	return name, false
}
