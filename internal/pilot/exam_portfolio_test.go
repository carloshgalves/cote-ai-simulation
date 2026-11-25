package pilot
import("reflect";"testing")
func TestPortfolioCalendar(t *testing.T){
 j:=autonomyJournal(t);j.Version=PortfolioVersion;if err:=admitExternalStudents(&j);err!=nil{t.Fatal(err)};initializeIdentities(&j)
 a:=generateExamCalendar(j);if len(a)!=18||!reflect.DeepEqual(a,generateExamCalendar(j)){t.Fatal("non-reproducible calendar")}
 kinds:=map[string]bool{};years:=map[int]bool{}
 for _,slot:=range a{t0,ok:=portfolioTemplate(slot.Kind);if !ok||len(slot.Group)<t0.MinGroup||len(slot.Years)!=1{t.Fatal(slot)};kinds[slot.Kind]=true;years[slot.Years[0]]=true;for _,id:=range slot.Group{if !isStudent(j,id)||j.Population[id].Year!=slot.Years[0]{t.Fatal("invalid grade")}};if slot.Kind=="vip"&&slot.Start<schoolDisclosure{t.Fatal("early CP disclosure")}}
 if len(kinds)<7||len(years)<3{t.Fatal("insufficient coverage",kinds,years)}
}
func TestPortfolioResolution(t *testing.T){
 group:=[]string{"a","b","c","d"}
 cases:=[]struct{kind string; ballots map[string]string;want map[string]int64}{
 {"kokkuri",map[string]string{"a":"yes","b":"yes","c":"yes","d":"yes"},map[string]int64{"a":25000,"b":25000,"c":25000,"d":25000}},
 {"kokkuri",map[string]string{"a":"yes","b":"yes","c":"yes"},map[string]int64{}},
 {"janken",map[string]string{"a":"rock","b":"scissors","c":"rock","d":"scissors"},map[string]int64{"a":25000,"c":25000}},
 {"poll",map[string]string{"a":"praise-b","c":"praise-b","d":"censure-a"},map[string]int64{"b":25000}},
 {"island",map[string]string{"a":"basic","b":"enhanced","c":"basic","d":"enhanced"},map[string]int64{"a":18000,"b":18000,"c":18000,"d":18000}},
 {"trial",map[string]string{"a":"support","b":"withhold"},map[string]int64{}},
 {"hide",map[string]string{"a":"a","b":"a","c":"b","d":"c"},map[string]int64{"b":25000,"c":25000}},
 }
 for _,c:=range cases{e:=&VIPExam{Kind:c.kind,Group:group,Ballots:c.ballots};if got:=portfolioWinners(e);!reflect.DeepEqual(got,c.want){t.Fatalf("%s: %v != %v",c.kind,got,c.want)}}
}
