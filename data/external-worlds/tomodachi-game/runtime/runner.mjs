// Decision boundary: this module does not expose owner state to decide().
import * as m from './resolver.mjs';
const priors={
  "actor.yuuichi-katagiri": {
    "public": "Part-time worker with modest resources; values friendships.",
    "private": "You know your own formative experiences with money and interpersonal exploitation. No future game outcomes are known.",
    "tendencies": [
      "Compare personal and collective payoffs before committing.",
      "When evidence is ambiguous, consider multiple hypotheses and seek discriminating observations.",
      "Accept a local cost only when justified by a larger objective; a risky strategy may fail."
    ]
  },
  "actor.tenji-mikasa": {
    "public": "Academic high achiever and mediator.",
    "private": "You entered the contest with an undisclosed investigative motive involving your father's death. You suspect a connection to Shiho but do not have proof.",
    "tendencies": [
      "Protect private objectives until disclosure is useful.",
      "Distinguish suspicion from demonstrated fact.",
      "Evaluate cooperation as information and circumstances change."
    ]
  },
  "actor.shiho-sawaragi": {
    "public": "Class vice-president with strong fairness orientation.",
    "private": "You have a police family background; you care about the group's welfare.",
    "tendencies": [
      "Prefer open cooperation and fair treatment.",
      "Question unreliable accusations and distinguish reputation from evidence.",
      "May initially trust friends but revise with new evidence."
    ]
  },
  "actor.makoto-shibe": {
    "public": "Class president with wealthy family background; direct and socially impulsive.",
    "private": "Family loyalty and social acceptance matter to you.",
    "tendencies": [
      "Respond more directly than a calculating strategist.",
      "Give weight to personal relationships and immediate social approval.",
      "May make impulsive choices without foreseeing their consequences."
    ]
  },
  "actor.yutori-kokorogi": {
    "public": "Appears shy and dependent on friends; enjoys manga.",
    "private": "Your public presentation need not reveal all of your private intentions. Do not assume other players know your undisclosed motives.",
    "tendencies": [
      "Preserve distinction between public demeanor and private objectives.",
      "Can cooperate in a limited test without making unrestricted commitments.",
      "Adapt presentation to available information; do not infer undisclosed secrets."
    ]
  }
};
const allowed={
  "kokkuri-san": [
    "answer",
    "spoke"
  ],
  "backbiting-sugoroku": [
    "cards"
  ],
  "hide-and-seek": [
    "betray",
    "accept",
    "surrender"
  ],
  "friend-sin-trial": [],
  "friends-prison": [
    "vote",
    "escape"
  ],
  "all-bet": [
    "sell",
    "consentSale",
    "buyback",
    "contract",
    "hand",
    "guess"
  ],
  "friend-murder": [
    "buy",
    "transfer",
    "defend"
  ],
  "friendless": [
    "report",
    "vote"
  ],
  "friendship-janken": [
    "hand"
  ],
  "fake-game": [
    "hand"
  ]
};
function actorInput(m,s,actor,priors,allowed){
 const observation=m.view(s,actor);
 const p=Object.hasOwn(priors,actor)?priors[actor]:{public:"Contestant; individual behavioral evidence insufficient.",private:"No additional background supplied.",tendencies:["Choose legal actions from available observations; no automatic superior capacity."]};
 return {actor,identity:p.public,privateBiography:p.private,tendencies:p.tendencies,observation,allowedActionKeys:allowed[s.game]};
}

function validateAction(action,keys){
 if(!action||typeof action!=="object"||Array.isArray(action)||Object.keys(action).some(k=>!keys.includes(k)))throw Error("invalid decision shape");
 return action;
}

async function runGame(m,scenario,decide,priors,allowed){
 let s=m.create(scenario.game,scenario.players,scenario.setup??{});const events=[];
 const max=scenario.maxRounds??10000;if(!Number.isSafeInteger(max)||max<1||max>10000)throw Error("invalid round limit");
 while(!s.terminal&&s.round<max){
  const round=s.round;const actions={};const active=[...s.active];
  // Collect from identical round-start state; no actor sees another pending response.
  const pairs=await Promise.all(active.map(async actor=>[actor,validateAction(await decide(actorInput(m,s,actor,priors,allowed)),allowed[s.game])]));
  for(const [actor,decision] of pairs)actions[actor]=decision;
  const host=scenario.hostRounds?.[round]??{};
  const next=m.step(s,actions,host);events.push({round:round+1,actions,host});s=next;
 }
 if(!s.terminal)throw Error("scenario reached configured round limit without terminal state");
 return {scenario,events,finalState:s};
}

function replay(m,run){
 let s=m.create(run.scenario.game,run.scenario.players,run.scenario.setup??{});for(const event of run.events){if(event.round!==s.round+1)throw Error("event sequence mismatch");s=m.step(s,event.actions,event.host)}
 if(!s.terminal||JSON.stringify(s)!==JSON.stringify(run.finalState))throw Error("replay mismatch");return s;
}
export {m,priors,allowed,actorInput,runGame,replay};
