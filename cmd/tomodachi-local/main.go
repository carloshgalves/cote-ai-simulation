// Command tomodachi-local executes the experimental native-Go Kokkuri slice.
// It does not claim native CSF persistence or registration.
package main
import(
 "context"
 "encoding/json"
 "flag"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "os/signal"
 "syscall"

 game "github.com/carloshgalves/cote-ai-simulation/internal/csf/reducer/tomodachi"
 tg "github.com/carloshgalves/cote-ai-simulation/internal/tomodachi"
)
func main(){
 if err:=run();err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
}
func run()error{
 out:=flag.String("out","","new private owner run JSON path")
 replay:=flag.String("replay","","existing owner run")
 scripted:=flag.Bool("scripted",false,"test-only deterministic decisions")
 repo:=flag.String("repo",".","repository root")
 python:=flag.String("python","python3","Python executable for native RAG")
 flag.Parse()
 if *replay!=""{
  f,err:=os.Open(*replay);if err!=nil{return err};defer f.Close()
  d:=json.NewDecoder(io.LimitReader(f,16<<20));d.DisallowUnknownFields();var r tg.Run
  if err=d.Decode(&r);err!=nil{return err};if err=d.Decode(new(any));err!=io.EOF{return fmt.Errorf("invalid trailing data")}
  if err=tg.Replay(r);err!=nil{return err};fmt.Println("PASS canonical candidate replay");return nil
 }
 if *out==""{return fmt.Errorf("--out required")}
 root,err:=filepath.Abs(*repo);if err!=nil{return err}
 name:=os.Getenv("TG_LLM_MODEL");var model tg.Model
 if *scripted{name="scripted-test-only";model=func(_ context.Context,i tg.Input)(game.Decision,error){
  if i.Observation.Phase==game.Discussion{return game.Decision{Choice:"pass"},nil}
  if i.Observation.Round==1&&i.Observation.Actor=="actor.makoto-shibe"{return game.Decision{Choice:"no"},nil}
  return game.Decision{Choice:"yes"},nil
 }}else{model,err=tg.HTTPModel(os.Getenv("TG_LLM_URL"),name,os.Getenv("TG_LLM_KEY"));if err!=nil{return err}}
 f,err:=os.OpenFile(*out,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600);if err!=nil{return err}
 defer f.Close()
 ctx,cancel:=signal.NotifyContext(context.Background(),os.Interrupt,syscall.SIGTERM);defer cancel()
 result,err:=tg.Execute(ctx,name,tg.PythonRAG(root,*python),model)
 if err!=nil{return err}
 if err=tg.Replay(result);err!=nil{return err}
 enc:=json.NewEncoder(f);enc.SetIndent("","  ");if err=enc.Encode(result);err!=nil{return err};if err=f.Sync();err!=nil{return err}
 fmt.Printf("Kokkuri model=%s questions=%d revisions=%d winners=%d owner=%s\n",name,result.Final.Round,result.Final.Revision,len(result.Final.Winners),*out)
 return nil
}
