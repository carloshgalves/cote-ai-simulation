package observatory

import (
    "go/parser"
    "go/token"
    "os"
    "strconv"
    "strings"
    "testing"
)

// O1 architecture rule: queries never depend on command/write ports,
// database adapters, model callbacks, or storage driver APIs.
func TestCSFRV19O1QueryPortCannotImportWritableAuthority(t *testing.T){
    items,err:=os.ReadDir(".")
    if err!=nil{t.Fatal(err)}
    for _,item:=range items{
        if item.IsDir()||!strings.HasSuffix(item.Name(),".go")||strings.HasSuffix(item.Name(),"_test.go"){continue}
        f,err:=parser.ParseFile(token.NewFileSet(),item.Name(),nil,parser.ImportsOnly)
        if err!=nil{t.Fatal(err)}
        for _,imp:=range f.Imports{
            path,err:=strconv.Unquote(imp.Path.Value)
            if err!=nil{t.Fatal(err)}
            switch{
            case path=="database/sql",
                strings.Contains(path,"/persistence/"),
                strings.Contains(path,"/ports"),
                strings.Contains(path,"/ownercheckpoint"),
                strings.Contains(path,"/commit"),
                strings.Contains(path,"/genesis"):
                t.Fatalf("Observatory query-only contract imports writable authority: %q",path)
            }
        }
    }
}
