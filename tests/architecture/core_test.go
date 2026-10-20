package architecture

import (
	"encoding/json"
	"io"
	"os/exec"
	"strings"
	"testing"
)

// go list includes every transitive import, so an innocent looking helper cannot
// smuggle a driver or protocol dependency across the Core boundary.
func TestCoreDependencyBoundary(t *testing.T) {
	cmd := exec.Command("go", "list", "-deps", "-test", "-json", "./internal/core/...")
	cmd.Dir = "../.."
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(out)))
	count := 0
	for {
		var p struct {
			ImportPath string
			Standard   bool
			ForTest    string
		}
		if err := decoder.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(p.ImportPath, "github.com/epoch-fresh-2026/Wikit/internal/core/") {
			count++
			continue
		}
		if !p.Standard {
			t.Errorf("Core imports non-Core dependency %s", p.ImportPath)
		}
		// Test harness itself needs os and io; production Core is checked below.
	}
	if count == 0 {
		t.Fatal("no Core packages checked")
	}
	cmd = exec.Command("go", "list", "-deps", "-json", "./internal/core/...")
	cmd.Dir = "../.."
	out, err = cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	decoder = json.NewDecoder(strings.NewReader(string(out)))
	for {
		var p struct {
			ImportPath string
			Imports    []string
		}
		if err := decoder.Decode(&p); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(p.ImportPath, "github.com/epoch-fresh-2026/Wikit/internal/core/") {
			continue
		}
		for _, imported := range p.Imports {
			for _, forbidden := range []string{"net/http", "database/sql", "os", "io/fs"} {
				if imported == forbidden || strings.HasPrefix(imported, forbidden+"/") {
					t.Errorf("Core imports infrastructure %s", imported)
				}
			}
		}
	}
}
