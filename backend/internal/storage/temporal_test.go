package storage

import "testing"

func TestTemporallyEffective(t *testing.T) {
	tests := []struct {
		name  string
		alias string
		want  string
	}{
		{
			name:  "asset alias",
			alias: "a",
			want:  "(a.valid_from IS NULL OR a.valid_from <= NOW()) AND (a.valid_to IS NULL OR a.valid_to > NOW())",
		},
		{
			name:  "location alias",
			alias: "l",
			want:  "(l.valid_from IS NULL OR l.valid_from <= NOW()) AND (l.valid_to IS NULL OR l.valid_to > NOW())",
		},
		{
			name:  "tag alias",
			alias: "i",
			want:  "(i.valid_from IS NULL OR i.valid_from <= NOW()) AND (i.valid_to IS NULL OR i.valid_to > NOW())",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := temporallyEffective(tc.alias)
			if got != tc.want {
				t.Fatalf("temporallyEffective(%q):\n  want: %s\n  got:  %s", tc.alias, tc.want, got)
			}
		})
	}
}
