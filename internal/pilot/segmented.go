package pilot

import (
 "crypto/sha256"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "sort"
 "strings"
)

type journalSegment struct {
 Previous string `json:"previous"`
 Rounds []Round `json:"rounds"`
 Digest string `json:"digest"`
}
type journalCheckpoint struct {
 Head string `json:"head"`
 State State `json:"state"`
}
func segmentDigest(previous string,rounds []Round)string{
 b,_:=json.Marshal(struct{Previous string;Rounds []Round}{previous,rounds});return fmt.Sprintf("%x",sha256.Sum256(b))
}
func segmentDirectory(path string)string{return path+".segments"}
func privateDirectory(path string)error{
 if err:=os.Mkdir(path,0700);err!=nil&&!os.IsExist(err){return err}
 i,err:=os.Lstat(path);if err!=nil||!i.IsDir()||i.Mode().Perm()&0077!=0{return errors.New("invalid segment directory")};return nil
}
func atomicSegment(path string,v any)error{
 data,err:=json.Marshal(v);if err!=nil{return err}
 tmp,err:=os.CreateTemp(filepath.Dir(path),".pending-");if err!=nil{return err};defer os.Remove(tmp.Name())
 _,err=tmp.Write(data);if err==nil{err=tmp.Sync()};closeErr:=tmp.Close();if err!=nil{return err};if closeErr!=nil{return closeErr}
 return os.Link(tmp.Name(),path)
}
func readPrivate(path string,dst any)error{
 i,err:=os.Lstat(path);if err!=nil||!i.Mode().IsRegular()||i.Mode().Perm()&0077!=0||i.Size()>128<<20{return errors.New("invalid segment file")}
 b,err:=os.ReadFile(path);if err!=nil{return err};return json.Unmarshal(b,dst)
}
// Segment commits are exclusive, immutable files. A checkpoint is a
// verified audit artifact, never an unauthenticated shortcut for replay.
func loadSegments(path string,j Journal)(Journal,State,string,error){
 if len(j.Rounds)!=0{return j,State{},"",errors.New("segmented header contains rounds")}
 s,err:=initial(j);if err!=nil{return j,s,"",err};head:=Digest(j)
 dir:=segmentDirectory(path);if err:=privateDirectory(dir);err!=nil{return j,s,"",err}
 entries,err:=os.ReadDir(dir);if err!=nil{return j,s,"",err};sort.Slice(entries,func(a,b int)bool{return entries[a].Name()<entries[b].Name()})
 for _,e:=range entries{
  if !strings.HasPrefix(e.Name(),"round-"){continue}
  if e.Name()!=fmt.Sprintf("round-%09d.json",s.Round+1){return j,s,"",errors.New("segment gap or duplicate")}
  var seg journalSegment;if err:=readPrivate(filepath.Join(dir,e.Name()),&seg);err!=nil{return j,s,"",err}
  if len(seg.Rounds)==0||len(seg.Rounds)>96||seg.Previous!=head||seg.Digest!=segmentDigest(head,seg.Rounds){return j,s,"",errors.New("segment chain invalid")}
  for _,r:=range seg.Rounds{s,err=apply(j,s,r);if err!=nil{return j,s,"",err}}
  j.Rounds=append(j.Rounds,seg.Rounds...);head=seg.Digest
  cpPath:=filepath.Join(dir,fmt.Sprintf("checkpoint-%09d.json",s.Round))
  if _,e:=os.Lstat(cpPath);e==nil{var cp journalCheckpoint;if e:=readPrivate(cpPath,&cp);e!=nil{return j,s,"",e};a,_:=json.Marshal(cp.State);b,_:=json.Marshal(s);if cp.Head!=head||string(a)!=string(b){return j,s,"",errors.New("checkpoint mismatch")}}else if !os.IsNotExist(e){return j,s,"",e}
 }
 return j,s,head,nil
}
func(s *Session) commitRounds(j Journal,next State,rs []Round)error{
 if j.Version<SegmentedVersion{return replaceJournal(s.path,j)}
 if len(rs)==0||rs[0].Number!=s.state.Round+1{return errors.New("invalid segment start")}
 dir:=segmentDirectory(s.path);head:=segmentDigest(s.segmentHead,rs)
 if err:=atomicSegment(filepath.Join(dir,fmt.Sprintf("round-%09d.json",rs[0].Number)),journalSegment{s.segmentHead,rs,head});err!=nil{return err}
 s.segmentHead=head
 // Optional checkpoint cannot invalidate an already committed segment.
 if next.Round/512>s.state.Round/512{_ = atomicSegment(filepath.Join(dir,fmt.Sprintf("checkpoint-%09d.json",next.Round)),journalCheckpoint{head,next})}
 return nil
}
