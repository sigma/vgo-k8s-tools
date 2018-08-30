package dependencies

import (
	"strings"
	"testing"
)

func TestGraphNormalization(t *testing.T) {
	g := Graph(map[string]*Node{
		"foo": &Node{
			Imports: []string{"bar", "baz"},
		},
		"bar": &Node{
			Imports:     []string{"baz"},
			TestImports: []string{"qux"},
		},
		"baz": &Node{
			Imports: []string{"quux"},
		},
	})

	g = g.Normalize(func(s string) string {
		if strings.HasPrefix(s, "b") {
			return "B" + s[1:]
		}
		return s
	})

	e, ok := g["Bar"]
	if !ok {
		t.Error("Bar not found")
	}
	if e.Imports[0] != "Baz" {
		t.Error("not Baz")
	}
}
