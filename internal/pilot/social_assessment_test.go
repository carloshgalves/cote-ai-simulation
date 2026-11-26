package pilot
import "testing"
func TestSocialAssessmentPrivateAndSingleUse(t *testing.T){
 j:=autonomyJournal(t);j.Version=SocialVersion;initializeIdentities(&j);s,err:=initial(j);if err!=nil{t.Fatal(err)}
 appendEvent(&s,Event{Kind:"claim",Actor:"kushida",Target:"horikita",Text:"Posso ajudar na tarefa.",Audience:[]string{"kushida","horikita"}})
 e:=s.Events[len(s.Events)-1];a:=SocialAssessment{Person:"kushida",Dimension:"affinity",Delta:1,Evidence:e.ID}
 c:=actorContext(j,s,"horikita");c.Memory=append(c.Memory,e)
 if err:=validateAssessment(c,a);err!=nil{t.Fatal(err)}
 m:=s.Minds["horikita"];applyAssessment(&s,"horikita",&m,a);s.Minds["horikita"]=m
 c.Social=m.Social;if validateAssessment(c,a)==nil{t.Fatal("evidence replay")}
 if m.Social["kushida"].Affinity!=1||s.Minds["kushida"].Social["horikita"].Affinity!=0{t.Fatal("invented reciprocity")}
 if validateAssessment(actorContext(j,s,"ayanokoji"),a)==nil{t.Fatal("private evidence leak")}
}
