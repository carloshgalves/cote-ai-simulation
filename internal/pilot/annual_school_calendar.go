package pilot

import (
 "fmt"
 "sort"
)

// New annual plan balances three grades. Only the two already admitted
// live adapters are scheduled; the settlement-reference library must not
// be mistaken for live examination support.
func annualSchoolCalendar(j Journal)[]ExamSlot{
 out:=[]ExamSlot{};years:=[]int{1,2,3}
 sort.Slice(years,func(a,b int)bool{return examKey(j,fmt.Sprintf("annual-year/%d",years[a]))<examKey(j,fmt.Sprintf("annual-year/%d",years[b]))})
 counts:=map[int]map[string]int{}
 for i:=0;i<18;i++{
  year:=years[i%3];day:=int64(3+20*i);kind:="kokkuri"
  if counts[year]==nil{counts[year]=map[string]int{}}
  if day>=32&&(counts[year]["vip"]<counts[year]["kokkuri"]||counts[year]["vip"]==counts[year]["kokkuri"]&&examKey(j,fmt.Sprintf("annual-kind/%d/vip",i))<examKey(j,fmt.Sprintf("annual-kind/%d/kokkuri",i))){kind="vip"}
  counts[year][kind]++
  slot:=buildAnnualSlot(j,fmt.Sprintf("exam-%02d",i+1),kind,year,day);out=append(out,slot)
 }
 return out
}
func buildAnnualSlot(j Journal,id,kind string,year int,day int64)ExamSlot{
 title,source:="Kokkuri — incentivos e confiança","Tomodachi Game: adaptação escolar com créditos virtuais"
 if kind=="vip"{title="Zodiac/VIP — três dias";source="COTE: y1-zodiac-cruise.json; adaptação escolar por ano"}
 roster:=gradeRoster(j,year);mixed:=[]string{}
 for _,c:=range []string{"A","B","C","D"}{g:=roster[c];sort.Slice(g,func(a,b int)bool{return examKey(j,id+g[a])<examKey(j,id+g[b])});roster[c]=g}
 for k:=0;k<40;k++{for _,c:=range []string{"A","B","C","D"}{mixed=append(mixed,roster[c][k])}}
 slot:=ExamSlot{ID:id,Kind:kind,Day:day,Start:(day-1)*1440+480,Group:clone(mixed),Title:title,Source:source,Years:[]int{year}}
 if kind=="kokkuri"{for k:=0;k<len(mixed);k+=5{slot.Groups=append(slot.Groups,clone(mixed[k:k+5]))};for k:=int64(0);k<=6;k++{slot.Boundaries=append(slot.Boundaries,slot.Start+k*60)}}else{
  at:=0;for k:=0;k<12;k++{n:=len(mixed)/12;if k<len(mixed)%12{n++};slot.Groups=append(slot.Groups,clone(mixed[at:at+n]));at+=n}
  slot.Boundaries=append(slot.Boundaries,slot.Start);for d:=int64(0);d<3;d++{base:=(day-1+d)*1440;slot.Boundaries=append(slot.Boundaries,base+780,base+840,base+1200,base+1260)};last:=(day+1)*1440;slot.Boundaries=append(slot.Boundaries,last+1290,last+1320,last+1380)
 };return slot
}
func applyGenericVIPFinal(j Journal,s *State,e *VIPExam){
 // A generic player knows only their own role and the group roster.
 // No selection of a guessed identity reads the secret VIP.
 choices:=[]Chosen{};for _,id:=range e.Group{if known(j.Profiles,id)||id==e.VIP||e.Submitted[id]{continue}
  candidates:=[]string{};for _,p:=range e.Group{if p!=id{candidates=append(candidates,p)}};sort.Slice(candidates,func(a,b int)bool{return examKey(j,e.ID+"/"+id+"/"+candidates[a])<examKey(j,e.ID+"/"+id+"/"+candidates[b])})
  if len(candidates)>0{choices=append(choices,Chosen{Actor:id,Decision:Decision{Action:"guess",Target:candidates[0],Source:routineSource}})}
 }
 before:=clone(*s);settleGuesses(j,before,s,choices)
}
