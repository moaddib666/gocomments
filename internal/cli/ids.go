package cli

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

type idList []string

func (l *idList) String() string { return strings.Join(*l, ",") }

func (l *idList) Set(v string) error {
	*l = append(*l, v)
	return nil
}

// CollectIDs merges the repeatable --id flag, the comma-separated --ids flag,
// and (when stdin is true) newline-separated ids read from r, in that order,
// preserving input order and dropping blank lines.
func CollectIDs(idFlag []string, idsFlag string, stdin bool, r io.Reader) ([]string, error) {
	var out []string
	out = append(out, idFlag...)
	if idsFlag != "" {
		for _, id := range strings.Split(idsFlag, ",") {
			id = strings.TrimSpace(id)
			if id != "" {
				out = append(out, id)
			}
		}
	}
	if stdin {
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			id := strings.TrimSpace(sc.Text())
			if id != "" {
				out = append(out, id)
			}
		}
		if err := sc.Err(); err != nil {
			return nil, fmt.Errorf("reading ids from stdin: %w", err)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no ids given: use --id, --ids, or --stdin")
	}
	return out, nil
}
