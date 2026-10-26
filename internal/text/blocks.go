package text

import (
	"fmt"
	"slices"
	"sort"

	"github.com/epoch-fresh-2026/Wikit/internal/core/search"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	gmtext "github.com/yuin/goldmark/text"
)

// Blocks preserves source positions and heading ancestry without trusting HTML.
func (Parser) Blocks(markdown string) ([]search.TextBlock, error) {
	source := []byte(markdown)
	root := goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser().Parse(gmtext.NewReader(source))
	lineStarts := []int{0}
	for i, b := range source {
		if b == '\n' {
			lineStarts = append(lineStarts, i+1)
		}
	}
	lineAt := func(offset int) int {
		return sort.Search(len(lineStarts), func(i int) bool { return lineStarts[i] > offset })
	}
	type heading struct {
		level    int
		text, id string
	}
	stack := []heading{}
	out := []search.TextBlock{}
	headingIndex := 0
	err := ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		if _, ok := node.(*ast.HTMLBlock); ok {
			return ast.WalkSkipChildren, nil
		}
		if h, ok := node.(*ast.Heading); ok {
			label, err := plainNode(node, source)
			if err != nil {
				return ast.WalkStop, err
			}
			for len(stack) > 0 && stack[len(stack)-1].level >= h.Level {
				stack = stack[:len(stack)-1]
			}
			stack = append(stack, heading{h.Level, label, fmt.Sprintf("section-%d", headingIndex)})
			headingIndex++
			return ast.WalkSkipChildren, nil
		}
		switch node.(type) {
		case *ast.Paragraph, *ast.TextBlock, *ast.FencedCodeBlock, *ast.CodeBlock, *extast.TableRow, *extast.TableHeader:
		default:
			return ast.WalkContinue, nil
		}
		text, err := plainNode(node, source)
		if err != nil {
			return ast.WalkStop, err
		}
		if text == "" {
			return ast.WalkSkipChildren, nil
		}
		start, end := node.Pos(), node.Pos()
		if node.Lines().Len() > 0 {
			start = node.Lines().At(0).Start
			end = start
		}
		err = ast.Walk(node, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
			if !entering {
				return ast.WalkContinue, nil
			}
			if child.Type() == ast.TypeBlock {
				for i := 0; i < child.Lines().Len(); i++ {
					line := child.Lines().At(i)
					end = max(end, line.Stop-1)
				}
			}
			if t, ok := child.(*ast.Text); ok {
				end = max(end, t.Segment.Stop-1)
			}
			return ast.WalkContinue, nil
		})
		if err != nil {
			return ast.WalkStop, err
		}
		path := []string{}
		var id *string
		for _, h := range stack {
			path = append(path, h.text)
			v := h.id
			id = &v
		}
		out = append(out, search.TextBlock{Text: text, SectionPath: slices.Clone(path), SectionID: id, StartLine: lineAt(start), EndLine: lineAt(end)})
		return ast.WalkSkipChildren, nil
	})
	return out, err
}
