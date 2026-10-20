// Package text implements text primitives without owning snapshot business rules.
package text

import (
	"html"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	gmtext "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

type Parser struct{}

func (Parser) NFC(s string) string  { return norm.NFC.String(s) }
func (Parser) Fold(s string) string { return cases.Fold().String(s) }
func (Parser) Links(markdown string) ([]string, error) {
	source := []byte(markdown)
	root := goldmark.New().Parser().Parse(gmtext.NewReader(source))
	out := []string{}
	err := ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		var destination []byte
		switch n := node.(type) {
		case *ast.Link:
			destination = n.Destination
		case *ast.Image:
			destination = n.Destination
		case *ast.AutoLink:
			destination = n.URL(source)
		}
		if destination != nil {
			out = append(out, html.UnescapeString(string(util.UnescapePunctuations(destination))))
		}
		return ast.WalkContinue, nil
	})
	return out, err
}
