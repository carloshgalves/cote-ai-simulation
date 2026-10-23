import unittest
from tools.canon_first_midterm_v1 import (CanonRuleUnknown,cutoff_from_reported_mean,
    evaluate,evaluate_complete_roster,apply_recorded_concession,RECORDED_CONCESSION)

class MidtermContract(unittest.TestCase):
    def test_reported_cutoffs(self):
        self.assertEqual(cutoff_from_reported_mean("79.6"),40)
        self.assertEqual(cutoff_from_reported_mean("64.4"),32)
        x=evaluate(class_id="Y1_D",subject="English",reported_mean="79.6",
                   reported_grades={"actor.ken-sudo":39})
        self.assertEqual(x["grades"]["actor.ken-sudo"]["status"],"BELOW_CUTOFF")
        self.assertEqual(x["expelled_actor_ids"],[])
        self.assertEqual(x["administrative_status"],"UNDECIDED")

    def test_unknown_ties_subjects_and_missing_scores(self):
        for mean in ("79","65"):
            with self.assertRaises(CanonRuleUnknown):cutoff_from_reported_mean(mean)
        with self.assertRaises(CanonRuleUnknown):
            evaluate(class_id="Y1_D",subject="Math",reported_mean="79.6",
                     reported_grades={"actor.ken-sudo":39})
        with self.assertRaises(CanonRuleUnknown):
            evaluate_complete_roster(class_id="Y1_D",subject="English",
                roster=["actor.a","actor.b"],grades={"actor.a":80})

    def test_complete_supplied_roster(self):
        x=evaluate_complete_roster(class_id="Y1_D",subject="English",
            roster=["actor.a","actor.b","actor.c","actor.d","actor.e"],
            grades={"actor.a":39,"actor.b":90,"actor.c":90,"actor.d":90,"actor.e":89})
        self.assertEqual(x["cutoff"],40)

    def test_concession_atomic_and_replay_denied(self):
        a={**RECORDED_CONCESSION,"approved":True,"transaction_id":"unique-observed-event",
           "payers":{"actor.kiyotaka-ayanokoji":40000,"actor.suzune-horikita":60000}}
        e=evaluate(class_id="Y1_D",subject="English",reported_mean="79.6",
            reported_grades={"actor.ken-sudo":39})
        b={"actor.kiyotaka-ayanokoji":41000,"actor.suzune-horikita":60000}
        result=apply_recorded_concession(evaluation=e,authorization=a,balances=b)
        self.assertEqual(result["evaluation"]["grades"]["actor.ken-sudo"]["score"],40)
        self.assertEqual(result["balances"]["actor.kiyotaka-ayanokoji"],1000)
        self.assertEqual(b["actor.kiyotaka-ayanokoji"],41000)
        with self.assertRaises(ValueError):
            apply_recorded_concession(evaluation=result["evaluation"],authorization=a,
                balances=result["balances"])
        with self.assertRaises(ValueError):
            apply_recorded_concession(evaluation=e,authorization=a,balances=b,
                previous_transaction_ids=("unique-observed-event",))
        with self.assertRaises(CanonRuleUnknown):
            apply_recorded_concession(evaluation=evaluate(class_id="Y1_D",subject="English",reported_mean="79.7",reported_grades={"actor.ken-sudo":39}),authorization=a,balances=b)
        for patch in [{"approved":False},{"target":"actor.other"},
                      {"price_private_points":1},
                      {"payers":{"actor.kiyotaka-ayanokoji":100000}}]:
            with self.assertRaises((ValueError,CanonRuleUnknown)):
                apply_recorded_concession(evaluation=e,authorization={**a,**patch},balances=b)
        with self.assertRaises(ValueError):
            apply_recorded_concession(evaluation=e,authorization=a,
                balances={"actor.kiyotaka-ayanokoji":1,"actor.suzune-horikita":60000})
if __name__=="__main__":unittest.main()
