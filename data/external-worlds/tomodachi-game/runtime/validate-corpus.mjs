import {readFile,readdir} from 'node:fs/promises';
import {fileURLToPath} from 'node:url';
import path from 'node:path';
const base=fileURLToPath(new URL('../consolidated/',import.meta.url));
const repo=fileURLToPath(new URL('../../../../',import.meta.url));
const read=async p=>JSON.parse(await readFile(p,'utf8'));
const require=(ok,msg)=>{if(!ok)throw Error(msg)};
const idx=await read(path.join(base,'indexes/character-topic-map.json'));
const ev=(await read(path.join(base,'evidence/secondary-2026-11-25.json'))).records;
const evals=(await read(path.join(base,'evals/counterfactual.json'))).records;
const em=new Map(ev.map(e=>[e.id,e]));require(em.size===ev.length,'duplicate evidence');
const cm=new Map();for(const [id,p] of Object.entries(idx.behavioral_case_paths)){const c=await read(path.join(repo,p));require(c.id===id&&c.actor_id===c.primary_actor_id&&p.endsWith('/behavioral_cases/'+c.primary_actor_id.slice(6)+'/'+path.basename(p)),'case identity/path mismatch '+id);require(c.runtime_memory===false&&c.evidence_refs.every(e=>em.get(e)?.mention_actor_ids.includes(c.primary_actor_id)),'case evidence/memory '+id);cm.set(id,c)}
const packFiles=(await readdir(path.join(base,'characters'))).filter(x=>x.endsWith('.json'));require(packFiles.length===Object.keys(idx.character_packs).length,'pack count mismatch');
for(const [id,meta] of Object.entries(idx.character_packs)){const p=await read(path.join(repo,meta.path));require(p.schema_version==='consolidated-character-v2'&&p.character_id===id&&p.knowledge_time_policy.canonical_after_divergence_is_not_actor_memory===true&&p.knowledge_time_policy.actor_knowledge_requires_acquisition_event===true,'pack mismatch '+id);for(const [t,d] of Object.entries(p.domains)){require(!('evidence_refs' in d),'legacy refs');require(d.behavioral_case_refs.every(c=>cm.get(c)?.primary_actor_id===id),'case ownership');require(d.context_evidence_refs.every(e=>em.get(e)?.mention_actor_ids.includes(id)),'context ownership');const union=[...d.behavioral_case_refs,...d.context_evidence_refs].sort();require(JSON.stringify(union)===JSON.stringify([...idx.topics_by_actor[id][t]].sort()),'topic index mismatch');require(JSON.stringify([...d.behavioral_case_refs].sort())===JSON.stringify([...idx.behavioral_by_actor_topic[id][t]].sort()),'behavior index mismatch');require(JSON.stringify([...d.context_evidence_refs].sort())===JSON.stringify([...idx.context_mentions_by_actor_topic[id][t]].sort()),'context index mismatch')}}
require(new Set(evals.map(x=>x.id)).size===evals.length,'duplicate eval IDs');for(const e of evals){require(idx.character_packs[e.actor_id]&&(!e.basis_case||cm.get(e.basis_case)?.primary_actor_id===e.actor_id),'eval linkage');if(e.contrast_actor_id)require(idx.character_packs[e.contrast_actor_id],'contrast actor')}
console.log(JSON.stringify({actors:packFiles.length,cases:cm.size,evidence:em.size,evals:evals.length,status:'PASS external V2 structural invariants'}));
