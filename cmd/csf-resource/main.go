// Command csf-resource writes non-authoritative CSFRV1-4 decision evidence.
// It cannot publish a world event, settlement, CycleCommit or WorldRevision.
package main

import (
 "bytes"
 "flag"
 "fmt"
 "os"
 "path/filepath"

 "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/decision"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/extensions/testresourcev1"
 "github.com/carloshgalves/cote-ai-simulation/internal/csf/policy"
)

func main(){
 path:=flag.String("output","","path to provisional-only canonical CBOR evidence")
 flag.Parse()
 if *path==""{fmt.Fprintln(os.Stderr,"--output is required");os.Exit(2)}
 if err:=run(*path);err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
}

func run(path string)error{
 bundle,err:=policy.Load("docs/architecture/canonical-codec-v1-bundle")
 if err!=nil{return err}
 input,err:=testresourcev1.LastUnit(bundle);if err!=nil{return err}
 result,err:=decision.Run(input);if err!=nil{return err}
 if len(result.Candidates)!=2||len(result.Conflicts)!=1||len(result.Draws)!=1||result.PostAvailable!=0{
  return fmt.Errorf("resource scenario did not resolve exactly one last unit")
 }
 if err:=os.MkdirAll(filepath.Dir(path),0750);err!=nil{return err}
 temp:=path+".tmp"
 if err:=os.WriteFile(temp,result.Canonical,0600);err!=nil{return err}
 defer os.Remove(temp)
 copyBytes,err:=os.ReadFile(temp);if err!=nil{return err}
 if _,err:=codec.StrictDecodePayload(copyBytes);err!=nil{return err}
 if !bytes.Equal(copyBytes,result.Canonical){return fmt.Errorf("provisional evidence failed byte-for-byte verification")}
 if err:=os.Rename(temp,path);err!=nil{return err}
 fmt.Printf("wrote %d provisional canonical bytes to %s; no world truth was published\n",len(result.Canonical),path)
 return nil
}
