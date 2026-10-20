package architecture

import (
 "go/parser"
 "go/token"
 "path/filepath"
 "strconv"
 "strings"
 "testing"
)

// The decision functions may consume immutable inputs but must never acquire
// a persistence/commit port. FromPersistedFence is the separate read-only
// adapter: it authenticates the fence before calling the pure kernel.
func TestResourceDecisionBoundaryHasNoMutableStoreDependency(t *testing.T) {
 paths:=[]string{
  "internal/csf/decision/kernel.go",
  "internal/csf/decision/run.go",
  "internal/csf/decision/claims.go",
  "internal/csf/extensions/testresourcev1/fixture.go",
 }
 disallowed:=[]string{
  "context", "database/sql", "os", "net/", "io",
  "github.com/carloshgalves/cote-ai-simulation/internal/csf/ports",
  "github.com/carloshgalves/cote-ai-simulation/internal/csf/persistence",
  "github.com/carloshgalves/cote-ai-simulation/internal/csf/commit",
 }
 for _,relative:=range paths {
  file,err:=parser.ParseFile(token.NewFileSet(),
   filepath.Join("..","..",filepath.FromSlash(relative)),nil,parser.ImportsOnly)
  if err!=nil{t.Fatal(err)}
  for _,imp:=range file.Imports {
   imported,err:=strconv.Unquote(imp.Path.Value)
   if err!=nil{t.Fatal(err)}
   for _,blocked:=range disallowed {
    if imported==blocked||strings.HasPrefix(imported,blocked+"/")||
     (strings.HasSuffix(blocked,"/")&&strings.HasPrefix(imported,blocked)) {
     t.Errorf("%s cannot import mutable or effectful dependency %q",relative,imported)
    }
   }
  }
 }
}
