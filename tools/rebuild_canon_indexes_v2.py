#!/usr/bin/env python3
"""Recompute or check topic/page indexes from consolidated canonical records."""
import argparse, json
from pathlib import Path

def read(p): return json.loads(p.read_text(encoding="utf-8"))
def main():
  args=argparse.ArgumentParser()
  args.add_argument("--check",action="store_true")
  args.add_argument("--write",action="store_true")
  o=args.parse_args()
  root=Path(__file__).resolve().parents[1]/"data/canon/consolidated"
  actors=[read(p) for p in sorted((root/"characters").glob("*.json"))]
  case_files=sorted((root/"behavioral_cases").glob("*/*.json"))
  cases=[read(p) for p in case_files]
  case_paths={c["id"]:p.relative_to(root.parents[2]).as_posix() for p,c in zip(case_files,cases)}
  assert len(case_paths)==len(case_files), "duplicate behavioral case IDs"
  evidence=[e for p in sorted((root/"evidence").glob("*.json")) for e in read(p)["records"]]
  em={e["id"]:e for e in evidence}
  cm={c["id"]:c for c in cases}
  primary={};mentions={};topics={}
  for p in actors:
    a=p["character_id"];primary[a]={};mentions[a]={};topics[a]={}
    for t,domain in sorted(p["domains"].items()):
      cs=sorted(set(domain.get("behavioral_case_refs",[])))
      es=sorted(set(domain.get("context_evidence_refs",[])))
      for c in cs: assert c in cm and cm[c]["primary_actor_id"]==a,(a,t,c)
      for e in es: assert e in em and a in em[e]["mention_actor_ids"],(a,t,e)
      primary[a][t]=cs; mentions[a][t]=es; topics[a][t]=sorted(set(cs+es))
  evtopics={}
  for e in evidence:
    for t in e["topics"]:evtopics.setdefault(t,set()).add(e["id"])
  evtopics={t:sorted(ids) for t,ids in evtopics.items()}
  pages={}
  for e in evidence:
    for n in e["source"]["pdf_pages"]:pages.setdefault(str(n),{"evidence":[],"cases":[]})["evidence"].append(e["id"])
  for c in cases:
    for n in c["source"]["pdf_pages"]:pages.setdefault(str(n),{"evidence":[],"cases":[]})["cases"].append(c["id"])
  pages={k:{field:sorted(set(v)) for field,v in d.items()} for k,d in pages.items()}
  plans=[
    ("character-topic-map.json",{"topics_by_actor":topics,"behavioral_by_actor_topic":primary,"context_mentions_by_actor_topic":mentions,"behavioral_case_paths":case_paths}),
    ("evidence-topic-map.json",{"evidence_by_topic":evtopics}),
    ("volume0-page-map.json",{"pages":pages})
  ]
  stale=[]
  for name,fields in plans:
    path=root/"indexes"/name;current=read(path)
    for k,value in fields.items():
      if current.get(k)!=value:stale.append(name+":"+k)
      if o.write:current[k]=value
    if o.write:path.write_text(json.dumps(current,ensure_ascii=False,indent=2,sort_keys=True)+"\n",encoding="utf-8")
  if o.check and stale:raise SystemExit("STALE INDEX: "+", ".join(stale))
  print("PASS" if not stale else "STALE",len(actors),"actors",len(cases),"cases",len(evidence),"evidence",stale)
if __name__=="__main__":main()
