package localdemo

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestB4HostCallbackHasSingleGatedDispatch is a narrowly scoped AST guard.
// The synthetic host has no production model client: this test proves only
// that code in the localdemo package cannot invoke the stored callback
// outside Host.Run -> Gate.RunWork, not that future model hosts are covered.
func TestB4HostCallbackHasSingleGatedDispatch(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	runWorkCalls := 0
	workReferences := 0
	hostMethodNames := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		source, err := parser.ParseFile(token.NewFileSet(), filepath.Clean(entry.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range source.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			isHostMethod := false
			if fn.Recv != nil && len(fn.Recv.List) == 1 {
				switch recv := fn.Recv.List[0].Type.(type) {
				case *ast.StarExpr:
					id, ok := recv.X.(*ast.Ident)
					isHostMethod = ok && id.Name == "Host"
				case *ast.Ident:
					isHostMethod = recv.Name == "Host"
				}
			}
			if isHostMethod {
				hostMethodNames[fn.Name.Name] = true
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.SelectorExpr:
					receiver, ok := node.X.(*ast.Ident)
					if ok && receiver.Name == "h" && node.Sel.Name == "work" {
						workReferences++
						if !isHostMethod || fn.Name.Name != "Run" {
							t.Errorf("%s: callback field h.work referenced outside Host.Run", fn.Name.Name)
						}
					}
				case *ast.CallExpr:
					selector, ok := node.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					if owner, ok := selector.X.(*ast.Ident); ok &&
						owner.Name == "h" && selector.Sel.Name == "work" {
						t.Errorf("%s: callback called directly instead of using Gate", fn.Name.Name)
					}
					if selector.Sel.Name != "RunWork" {
						return true
					}
					receiver, ok := selector.X.(*ast.SelectorExpr)
					if !ok || receiver.Sel.Name != "gate" {
						return true
					}
					h, ok := receiver.X.(*ast.Ident)
					if !ok || h.Name != "h" {
						return true
					}
					runWorkCalls++
					if !isHostMethod || fn.Name.Name != "Run" || len(node.Args) != 5 {
						t.Errorf("%s: Gate.RunWork must occur in Host.Run with five arguments", fn.Name.Name)
						return true
					}
					callback, ok := node.Args[4].(*ast.SelectorExpr)
					if !ok || callback.Sel.Name != "work" {
						t.Errorf("Host.Run did not pass its pinned callback to Gate.RunWork")
					} else if receiver, ok := callback.X.(*ast.Ident); !ok || receiver.Name != "h" {
						t.Errorf("Host.Run passed a callback from another receiver")
					}
				}
				return true
			})
		}
	}
	if !hostMethodNames["Run"] || runWorkCalls != 1 || workReferences != 1 {
		t.Fatalf("localdemo callback boundary drift: Host.Run=%v gated calls=%d callback references=%d",
			hostMethodNames["Run"], runWorkCalls, workReferences)
	}
}
