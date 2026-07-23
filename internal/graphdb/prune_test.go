package graphdb

import (
	"testing"
)

// TestEscapeString_PrevieneInjection verifica que paths con comillas/backslashes
// no rompen la query Cypher concatenada que arma PruneOrphanNodes.
func TestEscapeString_PrevieneInjection(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"normal/path.md", "normal/path.md"},
		{`with\backslash`, `with\\backslash`},
		{"with'quote", `with\'quote`},
		{`back\'slash`, `back\\\'slash`},
	}
	for _, c := range cases {
		got := escapeString(c.in)
		if got != c.want {
			t.Errorf("escapeString(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
