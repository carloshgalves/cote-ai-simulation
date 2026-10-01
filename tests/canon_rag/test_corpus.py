from pathlib import Path
import sys
HERE=Path(__file__).resolve().parent
REPO=HERE.parents[1]
sys.path.insert(0,str(HERE))
from common import corpus_manifest,load_claims,load_corpus,load_source_registry

def test_six_focal_actors_and_contract():
    evidence=load_corpus(REPO)
    seen=set()
    for ev in evidence:
        assert ev["schema_version"]==2
        assert ev["retrieval_role"] in {"BEHAVIORAL_CANON","KNOWLEDGE_EVIDENCE"}
        assert 0 < len(ev["text"]) <= 1600
        assert ev["narrative_position"]["work"]
        assert isinstance(ev["narrative_position"]["ordinal"],int)
        primary=[x for x in ev["provenance"]["supports"] if x.get("strength")=="primary"]
        assert len(primary)==1
        assert primary[0].get("quote_policy","paraphrase-only")=="paraphrase-only"
        assert ev["provenance"]["epistemic_status"]=="UNVERIFIED"
        assert ev["provenance"].get("verified_by") is None
        assert ev["provenance"].get("verified_at") is None
        seen.update(ev.get("actors",[]))
    expected={"actor.kiyotaka-ayanokoji","actor.suzune-horikita","actor.kikyo-kushida","actor.yosuke-hirata","actor.kakeru-ryuen","actor.honami-ichinose"}
    assert expected <= seen

def test_manifest_is_deterministic():
    evidence=load_corpus(REPO)
    claims,registry=load_claims(REPO),load_source_registry(REPO)
    assert corpus_manifest(evidence,claims,registry)==corpus_manifest(list(reversed(evidence)),list(reversed(claims)),registry)
    reversed_registry={**registry,"works":list(reversed(registry["works"]))}
    assert corpus_manifest(evidence,claims,registry)==corpus_manifest(evidence,claims,reversed_registry)
    assert len(corpus_manifest(evidence,claims,registry)["digest"])==64
