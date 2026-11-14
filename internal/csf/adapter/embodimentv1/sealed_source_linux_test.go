//go:build linux

package embodimentv1

import (
	"bytes"
	"os"
	"testing"

	"golang.org/x/sys/unix"
)

func testMemfd(t *testing.T, raw []byte, sealMask int) *os.File {
	t.Helper()
	fd, err := unix.MemfdCreate("psv12-source", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if err != nil { t.Fatal(err) }
	file := os.NewFile(uintptr(fd), "psv12-memfd")
	t.Cleanup(func(){ _ = file.Close() })
	if n,err:=file.Write(raw);err!=nil||n!=len(raw){t.Fatalf("memfd write failed: %v",err)}
	if sealMask!=0 {
		if _,err:=unix.FcntlInt(file.Fd(),unix.F_ADD_SEALS,sealMask);err!=nil{t.Fatal(err)}
	}
	return file
}

func TestKernelSealedPSV12PairPositiveStillUnadmitted(t *testing.T) {
	fixture:=realSource(t)
	s:=testMemfd(t,fixture.Snapshot,requiredMemfdSeals)
	e:=testMemfd(t,fixture.Events,requiredMemfdSeals)
	pins:=SourcePins{SnapshotSHA256:fixture.PinnedSnapshotSHA256,EventsSHA256:fixture.PinnedEventsSHA256}
	v,err:=InspectSealedUnadmitted(s,e,fixture.Evidence,pins)
	if err!=nil{t.Fatalf("real kernel-sealed Python PSV1-2 fixture rejected: %v",err)}
	if v.commandCount!=2||v.transitionCount!=6{t.Fatal("unexpected physical completion count")}
	if _,ok:=any(v).(interface{AdmitToWorld() error});ok{
		t.Fatal("sealed transport must never add a causal commit method")
	}
	// No writer may change the kernel-sealed file, including this same FD.
	if _,err:=s.WriteAt([]byte{'X'},0);err==nil{t.Fatal("kernel seal did not prevent writes")}
	if _,err:=e.WriteAt([]byte{'X'},0);err==nil{t.Fatal("kernel seal did not prevent writes")}
}

func TestSealedPhysicalCutRejectsUnsealedOrPartiallySealedFiles(t *testing.T){
	fixture:=realSource(t)
	pins:=SourcePins{SnapshotSHA256:fixture.PinnedSnapshotSHA256,EventsSHA256:fixture.PinnedEventsSHA256}
	full:=testMemfd(t,fixture.Events,requiredMemfdSeals)
	for _,mask:=range []int{0,unix.F_SEAL_SHRINK|unix.F_SEAL_GROW|unix.F_SEAL_SEAL,
		unix.F_SEAL_FUTURE_WRITE|unix.F_SEAL_SHRINK|unix.F_SEAL_GROW|unix.F_SEAL_SEAL}{
		untrusted:=testMemfd(t,fixture.Snapshot,mask)
		if _,err:=InspectSealedUnadmitted(untrusted,full,fixture.Evidence,pins);err==nil{
			t.Fatalf("accepted insufficient kernel source seals: %d",mask)
		}
	}
	var nilFile *os.File
	if _,err:=InspectSealedUnadmitted(nilFile,full,fixture.Evidence,pins);err==nil{
		t.Fatal("nil fd accepted")
	}
	ordinary,err:=os.CreateTemp(t.TempDir(),"physical-ordinary-*");if err!=nil{t.Fatal(err)}
	defer ordinary.Close()
	if _,err:=ordinary.Write(fixture.Snapshot);err!=nil{t.Fatal(err)}
	if _,err:=InspectSealedUnadmitted(ordinary,full,fixture.Evidence,pins);err==nil{
		t.Fatal("ordinary mutable path accepted as kernel-sealed source")
	}
}

func TestSealedPhysicalCutRejectsRepinAndMalformedCompletedSource(t *testing.T){
	fixture:=realSource(t)
	s:=testMemfd(t,fixture.Snapshot,requiredMemfdSeals)
	e:=testMemfd(t,fixture.Events,requiredMemfdSeals)
	pins:=SourcePins{SnapshotSHA256:fixture.PinnedSnapshotSHA256,EventsSHA256:fixture.PinnedEventsSHA256}
	changed:=pins
	changed.EventsSHA256[0]^=1
	if _,err:=InspectSealedUnadmitted(s,e,fixture.Evidence,changed);err==nil{
		t.Fatal("wrong external pin accepted")
	}
	cutAt:=bytes.Index(fixture.Events,[]byte("body.advance.complete.v1"))
	if cutAt<0{t.Fatal("fixture missing completion")}
	unfinished:=bytes.TrimSuffix(fixture.Events[:cutAt],[]byte{'\n'})
	lastNewline:=bytes.LastIndexByte(unfinished,'\n')
	if lastNewline<0{t.Fatal("bad fixture")}
	unfinished=unfinished[:lastNewline+1]
	cutFile:=testMemfd(t,unfinished,requiredMemfdSeals)
	wrong:=pins
	// Even a newly pinned internally coherent byte cut is not equivalent to
	// the completed operation described by the real A2 evidence.
	if _,err:=InspectSealedUnadmitted(s,cutFile,fixture.Evidence,wrong);err==nil{
		t.Fatal("truncated source matched a completion receipt")
	}
}

func TestSealedPhysicalSourceLengthBoundsBeforeRead(t *testing.T){
	tooLarge:=make([]byte,maxSnapshot+1)
	s:=testMemfd(t,tooLarge,requiredMemfdSeals)
	if _,err:=immutableMemfdBytes(s,maxSnapshot);err==nil{
		t.Fatal("oversized kernel memfd was consumed")
	}
	empty:=testMemfd(t,nil,requiredMemfdSeals)
	if _,err:=immutableMemfdBytes(empty,maxSnapshot);err==nil{
		t.Fatal("empty sealed physical source accepted")
	}
}
