import {m,priors,allowed,runGame,replay} from './runner.mjs';
import {readFile} from 'node:fs/promises';
const scenarios=JSON.parse(await readFile(new URL('./scenarios.json',import.meta.url),'utf8'));
for(const sc of scenarios){const run=await runGame(m,sc,async i=>sc.scriptedRounds[i.observation.round][i.actor],priors,allowed);replay(m,run);console.log(sc.game,run.events.length,'terminal/replay PASS')}
const probe=m.create('kokkuri-san',['a','b','c','d','e']);if(JSON.stringify(m.view(probe,'b')).includes('half-debt')||JSON.stringify(m.view(probe,'b')).includes('halves your own debt'))throw Error('private rule leaked');
