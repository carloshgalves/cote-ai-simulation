//go:build linux

package commit

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/carloshgalves/cote-ai-simulation/internal/physicalhost"
)

func TestH2IndependentD3ReplayRejectsEmptyOrForgedCutsBeforeCreatingDatabases(t *testing.T){
	host,plans:=referenceD2TestHost(t)
	cases:=[]struct{name string; epochs [2]OfflineOriginalA1Epoch}{
		{"empty", [2]OfflineOriginalA1Epoch{}},
		{"unsigned raw data", [2]OfflineOriginalA1Epoch{
			{Snapshot:[]byte("raw"),Events:[]byte("x"),Evidence:[]byte("x"),Cut:[]byte("not CBOR"),Signature:make([]byte,64)},
			{Snapshot:[]byte("raw"),Events:[]byte("x"),Evidence:[]byte("x"),Cut:[]byte("not CBOR"),Signature:make([]byte,64)},
		}},
	}
	for _,tc:=range cases{
		t.Run(tc.name,func(t *testing.T){
			dir:=t.TempDir()
			causal:=filepath.Join(dir,"second.db")
			owner:=filepath.Join(dir,"owner.db")
			_,err:=RunReferenceD3OfflineSameSignedA1(context.Background(),causal,owner,bundlePath,
				ReferenceD2Config{Host:host,EpochPlans:plans},tc.epochs,ReferenceOptions{})
			if err==nil{t.Fatal("offline replay admitted unknown or unsigned A1 sources")}
			if _,err:=os.Lstat(causal);!os.IsNotExist(err){t.Fatalf("forged source created causal DB before signature validation: %v",err)}
		})
	}
}

func TestH2IndependentD3ReplayRejectsActiveOriginalArchiveCapability(t *testing.T){
	host,plans:=referenceD2TestHost(t)
	host.PrivateCaptureRoot=t.TempDir()
	_,err:=RunReferenceD3OfflineSameSignedA1(context.Background(),filepath.Join(t.TempDir(),"next.db"),
		filepath.Join(t.TempDir(),"owner.db"),bundlePath,
		ReferenceD2Config{Host:host,EpochPlans:plans},[2]OfflineOriginalA1Epoch{},ReferenceOptions{})
	if err==nil{t.Fatal("offline run received an active live original capture output capability")}
}

// Compile-time separation: the offline replay source is strictly an in-memory
// copy of authenticated operator evidence. No ordinary actor port is created.
var _ physicalhost.ApprovedCut
