"""Partial *canonical* executable resolver: Y1 first midterm, Class D English.

No generated test questions, arbitrary score changes, automatic expulsion,
unpublished school scoring formulas, or inferred ties in nearest rounding.
"""
from decimal import Decimal, InvalidOperation

class CanonRuleUnknown(ValueError):
    """The novel does not establish a rule needed for the proposed action."""

ID = "exam.y1.midterm-1"
RECORDED_CONCESSION = dict(authority="actor.sae-chiyabashira",
    target="actor.ken-sudo", subject="English", additional_marks=1,
    price_private_points=100000)

def _decimal(x):
    if x is None or isinstance(x,bool): raise ValueError("invalid number")
    try: d=Decimal(str(x))
    except (InvalidOperation,ValueError,TypeError) as e: raise ValueError("invalid number") from e
    if not d.is_finite() or not (0<=d<=100): raise ValueError("mean must be finite and in 0..100")
    return d

def cutoff_from_reported_mean(mean):
    """The two observed examples: 79.6/2=39.8 -> 40; 64.4/2=32.2 -> 32."""
    h=_decimal(mean)/2
    base=int(h)
    fractional=h-base
    if fractional==Decimal(".5"): raise CanonRuleUnknown("half-integer rounding was not specified")
    return base+(fractional>Decimal(".5"))

def _check_grades(grades):
    if not isinstance(grades,dict) or not grades: raise ValueError("grades required")
    if any(not isinstance(a,str) or not a.startswith("actor.") or
           isinstance(g,bool) or not isinstance(g,int) or g<0 or g>100
           for a,g in grades.items()): raise ValueError("invalid actor/grade")

def evaluate(*, class_id, subject, reported_mean, reported_grades):
    """Uses only externally supplied results; no grade or hidden rule invention."""
    if class_id!="Y1_D" or subject!="English":
        raise CanonRuleUnknown("other class/subject execution not established")
    _check_grades(reported_grades)
    cutoff=cutoff_from_reported_mean(reported_mean)
    return dict(schema_version="canon-midterm-partial-v1",exam_id=ID,
       class_id=class_id,subject=subject,reported_mean=str(_decimal(reported_mean)),
       cutoff=cutoff,grades={a:dict(score=g,status=("BELOW_CUTOFF" if g<cutoff else "AT_OR_ABOVE_CUTOFF"))
           for a,g in sorted(reported_grades.items())},
       administrative_status="UNDECIDED",expelled_actor_ids=[],
       authorized_purchases=[],scope="OBSERVED_RESULTS_ONLY")

def evaluate_complete_roster(*,class_id,subject,roster,grades):
    """Optional hypothetical input with complete grades; absent roster -> explicit failure."""
    if not isinstance(roster,list) or not roster or len(roster)!=len(set(roster)):
        raise ValueError("invalid roster")
    if not isinstance(grades,dict) or set(grades)!=set(roster):
        raise CanonRuleUnknown("complete roster and score set required; no imputation")
    _check_grades(grades)
    mean=Decimal(sum(grades.values()))/len(roster)
    result=evaluate(class_id=class_id,subject=subject,reported_mean=mean,reported_grades=grades)
    result["scope"]="COMPLETE_SUPPLIED_ROSTER"
    return result

def apply_recorded_concession(*,evaluation,authorization,balances):
    """Atomic, one-time *observed* concession, not permission to buy arbitrary grades."""
    if evaluation.get("exam_id")!=ID or evaluation.get("class_id")!="Y1_D" or evaluation.get("subject")!="English":
        raise CanonRuleUnknown("exam mismatch")
    if not isinstance(authorization,dict) or authorization.get("approved") is not True:
        raise CanonRuleUnknown("explicit school approval required")
    if any(authorization.get(k)!=v for k,v in RECORDED_CONCESSION.items()):
        raise CanonRuleUnknown("no canonical arbitrary-purchase policy")
    tx=authorization.get("transaction_id")
    if not isinstance(tx,str) or not tx: raise ValueError("transaction id required")
    if any(x["transaction_id"]==tx for x in evaluation.get("authorized_purchases",[])):
        raise ValueError("duplicate transaction")
    score=evaluation.get("grades",{}).get("actor.ken-sudo")
    if not isinstance(score,dict) or score.get("score")!=39 or evaluation.get("cutoff")!=40:
        raise CanonRuleUnknown("only the documented 39 -> 40 situation supported")
    payers=authorization.get("payers")
    if not isinstance(payers,dict) or set(payers)!={"actor.kiyotaka-ayanokoji","actor.suzune-horikita"}:
        raise CanonRuleUnknown("only the documented joint payment supported")
    if any(isinstance(n,bool) or not isinstance(n,int) or n<=0 for n in payers.values()):
        raise ValueError("payment must be positive integer")
    if sum(payers.values())!=100000: raise CanonRuleUnknown("documented total is 100000")
    if not isinstance(balances,dict): raise ValueError("balances required")
    new_balances=dict(balances)
    for a,amount in payers.items():
        balance=new_balances.get(a)
        if isinstance(balance,bool) or not isinstance(balance,int) or balance<amount:
            raise ValueError("insufficient balance, transfer rejected atomically")
        new_balances[a]-=amount
    result={**evaluation,"grades":{a:dict(v) for a,v in evaluation["grades"].items()},
            "authorized_purchases":list(evaluation.get("authorized_purchases",[]))}
    result["grades"]["actor.ken-sudo"]={"score":40,"status":"AT_OR_ABOVE_CUTOFF"}
    result["authorized_purchases"].append(dict(transaction_id=tx,payers=dict(sorted(payers.items())),
        authorized_by=RECORDED_CONCESSION["authority"],target="actor.ken-sudo",marks=1,points=100000))
    result["administrative_status"]="OBSERVED_SUDOU_EXPULSION_RESCINDED"
    result["expelled_actor_ids"]=[]
    return dict(evaluation=result,balances=new_balances)
