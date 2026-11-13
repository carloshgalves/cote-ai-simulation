package localowner

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// b4CheckOwnerCommitSignOrder guards the normative source ordering without
// introducing a test-only crash hook into production storage. It is a static
// proof, NOT a process crash precisely between COMMIT and Ed25519.Sign.
func b4CheckOwnerCommitSignOrder(source *ast.File) error {
	var method *ast.FuncDecl
	for _, decl := range source.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "PersistAndSign" || fn.Recv == nil {
			continue
		}
		method = fn
		break
	}
	if method == nil || method.Body == nil {
		return fmt.Errorf("owner PersistAndSign method is absent")
	}
	var commit, sign token.Pos
	commits, signs := 0, 0
	ast.Inspect(method.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		receiver, ok := sel.X.(*ast.Ident)
		if !ok {
			return true
		}
		if receiver.Name == "tx" && sel.Sel.Name == "Commit" {
			commits++
			commit = call.Pos()
		}
		if receiver.Name == "ed25519" && sel.Sel.Name == "Sign" {
			signs++
			sign = call.Pos()
		}
		return true
	})
	if commits != 1 || signs != 1 || commit >= sign {
		return fmt.Errorf("owner durable COMMIT must appear exactly once BEFORE Ed25519.Sign: commits=%d signs=%d", commits, signs)
	}
	return nil
}

func TestB4OwnerCommitPrecedesEd25519Signature(t *testing.T) {
	source, err := parser.ParseFile(token.NewFileSet(), "sqlite.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := b4CheckOwnerCommitSignOrder(source); err != nil {
		t.Fatal(err)
	}
}

// A reversed implementation must fail independently of successful runtime
// crash tests. This is a negative fixture, never linked into the product.
func TestB4OwnerCommitSignOrderNegativeControl(t *testing.T) {
	code := `package localowner
type Store struct{}
func (s *Store) PersistAndSign() {
    ed25519.Sign(nil, nil)
    tx.Commit()
}`
	source, err := parser.ParseFile(token.NewFileSet(), "reversed.go", code, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := b4CheckOwnerCommitSignOrder(source); err == nil {
		t.Fatal("reversed COMMIT/signing order bypassed B4 structural regression")
	}
}
