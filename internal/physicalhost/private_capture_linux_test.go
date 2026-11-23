//go:build linux

package physicalhost

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPrivateA1CapturedSourceDurableAndNonOverwritable(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0700); err != nil { t.Fatal(err) }
	snapshot := []byte("unmodified raw supervised YAML\n")
	events := []byte("{\"wall_time\":\"genuine\"}\n")
	evidence := []byte("{\"complete\":true}\n")
	var cut SignedCut
	cut.SourceID = "physical.a1"
	cut.Epoch = 1
	cut.SnapshotSHA256[0] = 1
	cut.EventsSHA256[0] = 2
	cut.Signature[0] = 42 // only local file-writer test, not crypto validation
	if err := writePrivateA1Capture(root,1,snapshot,events,evidence,cut); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string][]byte{
		"snapshot.yaml": snapshot, "events.jsonl": events, "evidence.json": evidence,
		"signature.bin": cut.Signature[:],
	} {
		path := filepath.Join(root,"epoch-1",name)
		fi,err:=os.Lstat(path)
		if err!=nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0077!=0 { t.Fatalf("nonprivate %s: %v",name,err) }
		got,err:=os.ReadFile(path)
		if err!=nil || !bytes.Equal(got,want) { t.Fatalf("raw immutable writer changed %s: %v",name,err) }
	}
	canonical,err:=signedCutPayload(cut)
	if err!=nil { t.Fatal(err) }
	got,err:=os.ReadFile(filepath.Join(root,"epoch-1","cut.cbor"))
	if err!=nil || !bytes.Equal(got,canonical) { t.Fatalf("cut CBOR was normalized: %v",err) }
	if err:=writePrivateA1Capture(root,1,[]byte("different"),events,evidence,cut);err==nil {
		t.Fatal("capture silently overwrote a previously authenticated epoch")
	}
	check,err:=os.ReadFile(filepath.Join(root,"epoch-1","snapshot.yaml"))
	if err!=nil||!bytes.Equal(check,snapshot){t.Fatal("duplicate archive mutated the original")}
	if err:=writePrivateA1Capture(root,3,snapshot,events,evidence,cut);err==nil{
		t.Fatal("unexpected third producer epoch accepted")
	}
}

func TestPrivateA1CapturedSourceRefusesExposedAndSymlinkRoot(t *testing.T) {
	root:=t.TempDir()
	if err:=os.Chmod(root,0755);err!=nil{t.Fatal(err)}
	var cut SignedCut
	cut.SourceID="physical.a1"
	cut.Epoch=1
	cut.SnapshotSHA256[0]=1
	cut.EventsSHA256[0]=2
	if err:=writePrivateA1Capture(root,1,[]byte("a"),[]byte("b"),[]byte("c"),cut);err==nil {
		t.Fatal("nonprivate capture root accepted")
	}
	if err:=os.Chmod(root,0700);err!=nil{t.Fatal(err)}
	parent:=t.TempDir()
	link:=filepath.Join(parent,"external")
	if err:=os.Symlink(root,link);err!=nil{t.Fatal(err)}
	if err:=writePrivateA1Capture(link,1,[]byte("a"),[]byte("b"),[]byte("c"),cut);err==nil {
		t.Fatal("symlink archive root accepted")
	}
	if _,err:=os.Lstat(filepath.Join(root,"epoch-1"));!os.IsNotExist(err){
		t.Fatal("rejected host configuration created source files")
	}
}
