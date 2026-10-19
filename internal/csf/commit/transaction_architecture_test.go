package commit

import (
    "go/ast"
    "go/parser"
    "go/token"
    "io/fs"
    "path/filepath"
    "strings"
    "testing"
)

// CSFRV1-3 must have exactly one production invocation of the atomic
// CLOCK_ADVANCE persistence port. A future WORK coordinator extends this
// contract explicitly rather than silently adding a second publisher.
func TestClockAdvanceTransactionPortHasOneProductionCaller(t *testing.T) {
    root:=filepath.Join("..","..","..")
    csfRoot:=filepath.Join(root,"internal","csf")
    expected:=filepath.Clean(filepath.Join(csfRoot,"commit","clock_advance.go"))
    count:=0
    err:=filepath.WalkDir(csfRoot,func(path string,entry fs.DirEntry,walkErr error)error{
        if walkErr!=nil{return walkErr}
        if entry.IsDir()||!strings.HasSuffix(entry.Name(),".go")||
           strings.HasSuffix(entry.Name(),"_test.go"){return nil}
        parsed,err:=parser.ParseFile(token.NewFileSet(),path,nil,0)
        if err!=nil{return err}
        ast.Inspect(parsed,func(node ast.Node)bool{
            call,ok:=node.(*ast.CallExpr)
            if !ok{return true}
            selector,ok:=call.Fun.(*ast.SelectorExpr)
            if !ok||selector.Sel.Name!="CommitClockAdvance"{return true}
            count++
            if filepath.Clean(path)!=expected {
                t.Errorf("second CSF transaction publisher in %s",path)
            }
            return true
        })
        return nil
    })
    if err!=nil{t.Fatal(err)}
    if count!=1{t.Fatalf("expected one CLOCK_ADVANCE port caller, found %d",count)}
}
