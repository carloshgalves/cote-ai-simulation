package sqlite

import (
    "bytes"
    "context"
    "errors"
    "fmt"

    "github.com/carloshgalves/cote-ai-simulation/internal/csf/codec"
)

// The live CycleControlState folds the final status; after successful WORK
// it becomes IDLE and no longer holds its past abort/retry refs. A replay
// that validates only the current control row could therefore miss a
// corrupted *historical* CycleAbortRecord or AttemptRetryRecord. The full
// ordered journal must validate those roots and causal relationships even
// after the last terminal commit.
type historicalControl struct {
    id [32]byte
    fields []any
    digest [32]byte
    row int64
}
func historicalID(field any)([32]byte,bool){
    var id [32]byte
    raw,ok:=field.([]byte)
    if !ok||len(raw)!=32{return id,false}
    copy(id[:],raw)
    return id,true
}
func historicalControlKind(f []any)uint8{
    // The decision ledger is heterogeneous. Do NOT classify on field
    // count alone; deferred occurrence, candidate and clock records may
    // have similar arities but different authoritative field layouts.
    if len(f)==15 {
        _,attempt:=f[2].(uint64)
        _,reason:=f[10].(uint64)
        _,subjects:=f[11].([]any)
        _,digest:=historicalID(f[14])
        if attempt&&reason&&subjects&&digest{return 13}
    }
    if len(f)==7 {
        _,from:=f[2].(uint64)
        _,to:=f[3].(uint64)
        ref,ok:=f[4].([]any)
        _,reason:=f[5].(string)
        _,rules:=f[6].([]any)
        if from&&to&&ok&&len(ref)==3&&ref[0]==uint64(13)&&reason&&rules{return 14}
    }
    if len(f)==18 {
        _,attempt:=f[2].(uint64)
        _,plan:=f[6].([]any)
        _,membership:=f[11].([]any)
        _,digest:=historicalID(f[17])
        if attempt&&plan&&membership&&digest{return 11}
    }
    return 0
}
func (s *Store) verifyHistoricalAbortRetry(ctx context.Context)error{
    rows,err:=s.db.QueryContext(ctx,"SELECT rowid,id,canonical FROM decision_ledger ORDER BY rowid")
    if err!=nil{return err}
    kindMap:=map[uint8]map[[32]byte]historicalControl{
        11:{},13:{},14:{},
    }
    for rows.Next(){
        var seq int64
        var id,raw []byte
        if err:=rows.Scan(&seq,&id,&raw);err!=nil{rows.Close();return err}
        if len(id)!=32||len(raw)==0{rows.Close();return errors.New("historical decision journal entry malformed")}
        decoded,err:=codec.StrictDecodePayload(raw)
        if err!=nil{rows.Close();return fmt.Errorf("historical decision journal CBOR: %w",err)}
        fields,ok:=decoded.([]any)
        if !ok{rows.Close();return errors.New("historical decision journal must contain canonical array")}
        kind:=historicalControlKind(fields)
        if kind==0{continue}
        run,rok:=historicalID(fields[0])
        cycle,cok:=historicalID(fields[1])
        if !rok||!cok{rows.Close();return errors.New("historical terminal missing run/cycle")}
        recordID,_:=historicalID(id)
        var digest [32]byte
        switch kind{
        case 11:
            attempt,ok:=fields[2].(uint64)
            if !ok||attempt>uint64(^uint32(0)){rows.Close();return errors.New("historical fence attempt invalid")}
            want,err:=registeredDigest("cote.csf.id.admission-fence","cote.csf.schema.admission-fence-id-preimage",
                []any{run[:],cycle[:],attempt})
            if err!=nil||want!=recordID{rows.Close();return errors.New("historical fence identity mismatch")}
            digest,err=registeredDigest("cote.csf.digest.fence","cote.csf.schema.admission-fence.body",fields[:17])
            if err!=nil||!equalBytes(fields[17],digest[:]){rows.Close();return errors.New("historical fence digest mismatch")}
        case 13:
            attempt,ok:=fields[2].(uint64)
            reason,reasonOK:=fields[10].(uint64)
            subjects:=fields[11].([]any)
            if !ok||attempt>uint64(^uint32(0))||!reasonOK||reason>6||
                (reason==0&&len(subjects)==0){rows.Close();return errors.New("historical abort reason/attempt invalid")}
            want,err:=registeredDigest("cote.csf.id.cycle-abort","cote.csf.schema.cycle-abort-id-preimage",
                []any{run[:],cycle[:],attempt})
            if err!=nil||want!=recordID{rows.Close();return errors.New("historical abort identity mismatch")}
            digest,err=registeredDigest("cote.csf.digest.abort","cote.csf.schema.cycle-abort.body",fields[:14])
            if err!=nil||!equalBytes(fields[14],digest[:]){rows.Close();return errors.New("historical abort digest mismatch")}
        case 14:
            from:=fields[2].(uint64)
            to:=fields[3].(uint64)
            if from>=uint64(^uint32(0))||to!=from+1{
                rows.Close();return errors.New("historical retry does not advance immediate attempt")
            }
            want,err:=registeredDigest("cote.csf.id.attempt-retry","cote.csf.schema.attempt-retry-id-preimage",
                []any{run[:],cycle[:],from,to})
            if err!=nil||want!=recordID{rows.Close();return errors.New("historical retry identity mismatch")}
            digest,err=registeredDigest("cote.csf.digest.attempt-retry","cote.csf.schema.attempt-retry",fields)
            if err!=nil{rows.Close();return err}
        }
        if _,dup:=kindMap[kind][recordID];dup{rows.Close();return errors.New("duplicate historical control record")}
        kindMap[kind][recordID]=historicalControl{id:recordID,fields:fields,digest:digest,row:seq}
    }
    if err:=rows.Err();err!=nil{rows.Close();return err}
    if err:=rows.Close();err!=nil{return err}
    fences,aborts,retries:=kindMap[11],kindMap[13],kindMap[14]
    for _,abort:=range aborts{
        fields:=abort.fields
        run,_:=historicalID(fields[0]);cycle,_:=historicalID(fields[1])
        attempt:=fields[2].(uint64)
        fenceID,err:=registeredDigest("cote.csf.id.admission-fence","cote.csf.schema.admission-fence-id-preimage",
            []any{run[:],cycle[:],attempt})
        if err!=nil{return err}
        fence,ok:=fences[fenceID]
        if !ok||fence.row>=abort.row{return errors.New("historical abort has no preceding frozen AdmissionFence")}
        for _,pair:=range [][2]int{{4,3},{5,4},{6,5},{7,6},{8,7},{12,9},{16,13}}{
            if !sameCanonical(fence.fields[pair[0]],fields[pair[1]]){
                return errors.New("historical abort changed frozen causal/policy field")
            }
        }
        if !equalBytes(fields[8],fence.digest[:]){
            return errors.New("historical abort cites wrong fence digest")
        }
    }
    for _,retry:=range retries{
        f:=retry.fields
        run,_:=historicalID(f[0]);cycle,_:=historicalID(f[1])
        from:=f[2].(uint64)
        abortID,err:=registeredDigest("cote.csf.id.cycle-abort","cote.csf.schema.cycle-abort-id-preimage",
            []any{run[:],cycle[:],from})
        if err!=nil{return err}
        abort,ok:=aborts[abortID]
        if !ok||abort.row>=retry.row{return errors.New("historical retry has no preceding authoritative abort")}
        if !sameCanonical(f[4],[]any{uint64(13),abortID[:],abort.digest[:]})||
           !sameCanonical(f[6],abort.fields[13]){
            return errors.New("historical retry refers to a foreign abort or altered policy")
        }
        fenceID,err:=registeredDigest("cote.csf.id.admission-fence","cote.csf.schema.admission-fence-id-preimage",
            []any{run[:],cycle[:],from+1})
        if err!=nil{return err}
        next,hasNext:=fences[fenceID]
        if hasNext{
            if next.row<=retry.row{return errors.New("retry fence predates authorization")}
            if !sameCanonical(next.fields[3],[]any{uint64(1),[]any{uint64(14),retry.id[:],retry.digest[:]}}){
                return errors.New("historical retry record changed after successor fence")
            }
            priorID,err:=registeredDigest("cote.csf.id.admission-fence","cote.csf.schema.admission-fence-id-preimage",
                []any{run[:],cycle[:],from})
            if err!=nil{return err}
            old,ok:=fences[priorID]
            if !ok{return errors.New("historical retry predecessor AdmissionFence missing")}
            for _,index:=range []int{0,1,4,5,6,7,8,9,10,11,12,13,14,15,16}{
                if !sameCanonical(next.fields[index],old.fields[index]){
                    return fmt.Errorf("historical retry changed frozen semantic field %d",index)
                }
            }
        }
    }
    // A successful retry cannot exist without an authoritative abort, nor
    // can a historical abort be silently altered after control reached IDLE.
    // Return only after the entire journal has been revalidated.
    return nil
}
