from pathlib import Path
import sys
HERE=Path(__file__).resolve().parent
REPO=HERE.parents[1]
sys.path.insert(0,str(HERE))
from common import load_claims,load_corpus,load_source_registry,load_suite
from eval_runner import evaluate_suite

def test_golden_suite_is_clean():
    report=evaluate_suite(load_corpus(REPO),load_claims(REPO),load_source_registry(REPO),load_suite(REPO))
    assert report["critical_failures"]==[]
    m=report["metrics"]
    assert m["recall_at_k"]==1.0
    assert m["mean_reciprocal_rank"]==1.0
    assert m["irrelevant_context_rate"]==0.0
    assert m["wrong_character_leakage"]==0
    assert m["forbidden_timeline_leakage"]==0
    assert m["post_divergence_canon_leakage"]==0
    assert m["provenance_incomplete_items"]==0
