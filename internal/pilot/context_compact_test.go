package pilot
import (
 "encoding/json"
 "strings"
 "testing"
)
func TestCompactModelContextPreservesOptionsAndEvidence(t *testing.T){
 j:=autonomyJournal(t);j.Version=CompactContextVersion;initializeIdentities(&j);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 c:=actorContext(j,s,"horikita")
 if c.Body!=nil||c.Interoception==nil{t.Fatal("raw body meter projected")}
 raw,err:=modelContextPayload(c);if err!=nil{t.Fatal(err)}
 var p struct{Options []string `json:"options"`;Memory []Event `json:"memory"`;People []struct{ID string `json:"id"`;Role string `json:"role"`} `json:"people"`}
 if err:=json.Unmarshal(raw,&p);err!=nil{t.Fatal(err)}
 for _,o:=range offeredActions(c){if !has(p.Options,o.ID){t.Fatal("lost legal option",o.ID)}}
 if len(p.Memory)!=len(c.Memory)||len(p.People)!=len(j.Profiles){t.Fatal("evidence or directory lost")}
 for _,key:=range []string{"\"population\":","\"body\":"}{if strings.Contains(string(raw),key){t.Fatal("redundant or raw field",key)}}
 old:=c;old.Version=PhysicalVersion;old.Body=s.Minds["horikita"].Body;old.Interoception=nil
 legacy,err:=modelContextPayload(old);if err!=nil{t.Fatal(err)}
 if len(raw)>=len(legacy){t.Fatal("context not smaller",len(raw),len(legacy))}
}
