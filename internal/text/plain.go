package text

import (
	"html"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	gmtext "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Plain extracts searchable text without HTML source, link targets or rendered markup.
func (Parser) Plain(markdown string) (string, error) {
	source := []byte(markdown)
	root := goldmark.New().Parser().Parse(gmtext.NewReader(source))
	return plainNode(root, source)
}

func plainNode(root ast.Node, source []byte) (string, error) {
	var out strings.Builder
	err := ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			if node.Type() == ast.TypeBlock {
				out.WriteByte(' ')
			}
			return ast.WalkContinue, nil
		}
		switch n := node.(type) {
		case *ast.HTMLBlock, *ast.RawHTML:
			return ast.WalkSkipChildren, nil
		case *ast.Text:
			out.WriteString(html.UnescapeString(string(util.UnescapePunctuations(n.Segment.Value(source)))))
			if n.SoftLineBreak() || n.HardLineBreak() {
				out.WriteByte(' ')
			}
		case *ast.String:
			out.Write(n.Value)
		case *ast.AutoLink:
			out.Write(n.Label(source))
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			for i := 0; i < node.Lines().Len(); i++ {
				line := node.Lines().At(i)
				out.Write(line.Value(source))
			}
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", err
	}
	return strings.Join(strings.Fields(out.String()), " "), nil
}
