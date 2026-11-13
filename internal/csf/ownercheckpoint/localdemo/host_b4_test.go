package localdemo

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// b4AuditHostDispatch is a deliberately narrow structural guard for this
// synthetic localdemo package. A future model host outside this package must
// be audited separately; this is not a universal LLM authorization proof.
func b4AuditHostDispatch(files ...*ast.File) []string {
	var findings []string
	runWorkCalls, workReferences := 0, 0
	hasRun := false
	for _, source := range files {
		for _, decl := range source.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			receiverName := ""
			isHostMethod := false
			if fn.Recv != nil && len(fn.Recv.List) == 1 {
				recv := fn.Recv.List[0]
				if len(recv.Names) == 1 {
					receiverName = recv.Names[0].Name
				}
				switch t := recv.Type.(type) {
				case *ast.StarExpr:
					id, ok := t.X.(*ast.Ident)
					isHostMethod = ok && id.Name == "Host"
				case *ast.Ident:
					isHostMethod = t.Name == "Host"
				}
			}
			isRun := isHostMethod && fn.Name.Name == "Run"
			if isRun {
				hasRun = true
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.SelectorExpr:
					if node.Sel.Name == "work" {
						workReferences++
						x, ok := node.X.(*ast.Ident)
						if !isRun || !ok || x.Name != receiverName {
							findings = append(findings, fmt.Sprintf("%s: cognitive callback field accessed outside pinned Host.Run receiver", fn.Name.Name))
						}
					}
				case *ast.CallExpr:
					selector, ok := node.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					if selector.Sel.Name == "work" {
						findings = append(findings, fmt.Sprintf("%s: direct cognitive callback call bypasses Gate", fn.Name.Name))
					}
					if selector.Sel.Name != "RunWork" {
						return true
					}
					runWorkCalls++
					gateField, ok := selector.X.(*ast.SelectorExpr)
					if !ok || gateField.Sel.Name != "gate" || !isRun || len(node.Args) != 5 {
						findings = append(findings, fmt.Sprintf("%s: Gate.RunWork must be called inside Host.Run", fn.Name.Name))
						return true
					}
					receiver, ok := gateField.X.(*ast.Ident)
					if !ok || receiver.Name != receiverName {
						findings = append(findings, fmt.Sprintf("%s: Gate.RunWork uses a foreign gate receiver", fn.Name.Name))
					}
					cb, ok := node.Args[4].(*ast.SelectorExpr)
					if !ok || cb.Sel.Name != "work" {
						findings = append(findings, "Host.Run must pass its exact pinned callback to Gate.RunWork")
						return true
					}
					actor, ok := cb.X.(*ast.Ident)
					if !ok || actor.Name != receiverName {
						findings = append(findings, "Host.Run supplied another receiver's callback")
					}
				}
				return true
			})
		}
	}
	if !hasRun || runWorkCalls != 1 || workReferences != 1 {
		findings = append(findings, fmt.Sprintf("localdemo callback invariant: Host.Run=%v gated calls=%d callback refs=%d",
			hasRun, runWorkCalls, workReferences))
	}
	return findings
}

// TestB4HostCallbackHasSingleGatedDispatch checks every production Go file
// in localdemo. It is scoped to the synthetic host, not every model producer
// in a future simulation runtime.
func TestB4HostCallbackHasSingleGatedDispatch(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	var parsed []*ast.File
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") ||
			strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		source, err := parser.ParseFile(token.NewFileSet(), filepath.Clean(entry.Name()), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		parsed = append(parsed, source)
	}
	if len(parsed) == 0 {
		t.Fatal("no production localdemo source parsed")
	}
	for _, finding := range b4AuditHostDispatch(parsed...) {
		t.Error(finding)
	}
}

// Negative control: renaming the Go receiver may never bypass the audit.
// The fixture is parsed only; type-checking it is neither necessary nor
// sufficient for demonstrating the source-level callback selector check.
func TestB4GuardRejectsRenamedReceiverBypass(t *testing.T) {
	source := `package localdemo
type Host struct{}
func (self *Host) Run() { self.gate.RunWork(nil, nil, nil, nil, self.work) }
func (other *Host) Bypass() { other.work(nil) }
`
	file, err := parser.ParseFile(token.NewFileSet(), "negative.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	found := b4AuditHostDispatch(file)
	if len(found) == 0 {
		t.Fatal("renamed receiver bypass survived cognitive callback guard")
	}
	if !strings.Contains(strings.Join(found, "\n"), "Bypass") {
		t.Fatalf("negative control failed for unrelated reason: %v", found)
	}
}

func TestB4GuardAllowsLegitimateRenamedReceiver(t *testing.T) {
	source := `package localdemo
type Host struct{}
func (self *Host) Run() { self.gate.RunWork(nil, nil, nil, nil, self.work) }
`
	file, err := parser.ParseFile(token.NewFileSet(), "positive.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := b4AuditHostDispatch(file); len(got) != 0 {
		t.Fatalf("legal Host.Run with another receiver name was rejected: %v", got)
	}
}
