import {readFile,writeFile} from 'node:fs/promises';
import {m,priors,allowed,runGame,replay} from './runner.mjs';
const args=process.argv.slice(2);
const get=key=>{const i=args.indexOf(key);return i<0?undefined:args[i+1]};
if(get('--replay')){const run=JSON.parse(await readFile(get('--replay'),'utf8'));replay(m,run);console.log('PASS replay');process.exit(0)}
const scenario=JSON.parse(await readFile(get('--scenario')??new URL('./kokkuri-demo.json',import.meta.url),'utf8'));
let decide;
if(args.includes('--llm')){
 const endpoint=process.env.TG_LLM_URL,model=process.env.TG_LLM_MODEL,key=process.env.TG_LLM_KEY;
 if(!endpoint||!model)throw Error('Set TG_LLM_URL (OpenAI-compatible chat/completions URL) and TG_LLM_MODEL');
 decide=async input=>{
   const r=await fetch(endpoint,{method:'POST',headers:{'Content-Type':'application/json',...(key?{Authorization:'Bearer '+key}:{})},body:JSON.stringify({model,messages:[{role:'system',content:'You are one contestant in a fictional game. Choose one legal action using only the given information. Return a JSON object, no markdown, no narration. Never access or invent hidden facts. Your result is not authoritative.'},{role:'user',content:JSON.stringify(input)}],temperature:0})});
   if(!r.ok)throw Error('LLM HTTP '+r.status);const body=await r.json();return JSON.parse(body.choices[0].message.content);
 };
}else{
 decide=async input=>{const scripted=scenario.scriptedRounds?.[input.observation.round]?.[input.actor];if(scripted!==undefined)return scripted;if(scenario.game==='kokkuri-san')return {answer:'yes'};throw Error('Scripted action missing for '+input.actor)};
}
const run=await runGame(m,scenario,decide,priors,allowed);replay(m,run);
const output=get('--out')??'tg-owner-run.json';await writeFile(output,JSON.stringify(run,null,2)+'\n',{mode:0o600});console.log(JSON.stringify({game:scenario.game,rounds:run.events.length,winners:run.finalState.winners,ownerRun:output}));
