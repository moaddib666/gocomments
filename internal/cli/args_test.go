package cli

import (
	"reflect"
	"testing"
)

func TestReorderArgs(t *testing.T) {
	listFlags := map[string]bool{"offset": true, "limit": true, "diff": true, "commit": true, "pattern": true}
	deleteFlags := map[string]bool{"id": true, "ids": true}

	cases := []struct {
		name       string
		args       []string
		valueFlags map[string]bool
		want       []string
	}{
		{
			name:       "value flag after path",
			args:       []string{"./pkg", "--pattern", "foo"},
			valueFlags: listFlags,
			want:       []string{"--pattern", "foo", "./pkg"},
		},
		{
			name:       "idempotent flag first",
			args:       []string{"--pattern", "foo", "./pkg"},
			valueFlags: listFlags,
			want:       []string{"--pattern", "foo", "./pkg"},
		},
		{
			name:       "pattern pairing preserved",
			args:       []string{"./pkg", "--json", "--pattern", "x"},
			valueFlags: listFlags,
			want:       []string{"--json", "--pattern", "x", "./pkg"},
		},
		{
			name:       "multi bool and value flags mixed",
			args:       []string{"./pkg", "--include-protected", "--offset", "5", "--json"},
			valueFlags: listFlags,
			want:       []string{"--include-protected", "--offset", "5", "--json", "./pkg"},
		},
		{
			name:       "repeatable id",
			args:       []string{"./pkg", "--id", "aaa", "--id", "bbb"},
			valueFlags: deleteFlags,
			want:       []string{"--id", "aaa", "--id", "bbb", "./pkg"},
		},
		{
			name:       "value starting with dash paired regardless",
			args:       []string{"./pkg", "--pattern", "-foo"},
			valueFlags: listFlags,
			want:       []string{"--pattern", "-foo", "./pkg"},
		},
		{
			name:       "terminator stops reordering",
			args:       []string{"--json", "--", "--pattern", "./pkg"},
			valueFlags: listFlags,
			want:       []string{"--json", "--pattern", "./pkg"},
		},
		{
			name:       "flag-like positional needs terminator",
			args:       []string{"--", "-weird-path"},
			valueFlags: listFlags,
			want:       []string{"-weird-path"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := reorderArgs(tc.args, tc.valueFlags)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("reorderArgs(%v) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}
