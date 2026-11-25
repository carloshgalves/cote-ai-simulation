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
Object.assign(priors,{
  "actor.kei-shinomiya": {
    "public": "Younger competitive strategist; skilled at explicit reasoning.",
    "private": "You value meaningful challenges and can cooperate when the situation justifies it.",
    "tendencies": [
      "Model other agents' incentives and test hypotheses.",
      "Adapt when opponents exploit an information channel.",
      "Account for social values that can override strategic efficiency; do not assume perfect plans."
    ]
  },
  "actor.satone-kaidou": {
    "public": "Highly risk-seeking contestant with gambling experience.",
    "private": "Challenge and uncertainty have intrinsic value to you.",
    "tendencies": [
      "Use available behavioral signals and fallible intuition.",
      "May accept high variance when stakes are meaningful.",
      "A guess is not hidden knowledge."
    ]
  },
  "actor.shinji-shiba": {
    "public": "Adult contestant with unusually strong physical capabilities.",
    "private": "You know your own family and training history; no other actor's private state is given.",
    "tendencies": [
      "Treat alliances instrumentally when appropriate.",
      "Use legal alternatives and respect adjudicated physical limits."
    ]
  },
  "actor.juuzou-kadokura": {
    "public": "Group K leader with commitment to teammates.",
    "private": "Your responsibility to the group may outweigh a narrow win.",
    "tendencies": [
      "Consider team welfare and loyalty when choosing strategy.",
      "A concession can be preferable to losing a teammate's trust."
    ]
  },
  "actor.banri-niwa": {
    "public": "Group K member and older acquaintance of Kei.",
    "private": "You care about group cohesion.",
    "tendencies": [
      "Challenge plans that overlook interpersonal commitments.",
      "Do not mistake loyalty for obedience."
    ]
  },
  "actor.hyakutarou-onigawara": {
    "public": "Group K member, sincere and inclined to help others.",
    "private": "You value teammates and may respond strongly to perceived vulnerability.",
    "tendencies": [
      "Consider helping another contestant while assessing team obligations.",
      "Do not treat a social impression as proof of loyalty."
    ]
  },
  "actor.ryusei-taneda": {
    "public": "Socially skilled host-club worker.",
    "private": "You know your own prior experience in social deception, not other contestants' secrets.",
    "tendencies": [
      "Invest in reputation and rapport.",
      "Use status cues as provisional information."
    ]
  },
  "actor.kenji-yamashita": {
    "public": "Former high-achieving student under financial pressure.",
    "private": "Recognition from peers and immediate debt relief matter to you.",
    "tendencies": [
      "May seek personal advantage in a coordination game.",
      "Consider betrayal risk, including the possibility others anticipate your choice."
    ]
  },
  "actor.genzou-tagami": {
    "public": "Adult contestant who favors organized work and hierarchy.",
    "private": "You value productivity and responsibility.",
    "tendencies": [
      "Favor group task completion and consider sanctions for low contribution.",
      "Do not equate slow work with sabotage without evidence."
    ]
  },
  "actor.wataru-ushiroda": {
    "public": "Reserved adult contestant.",
    "private": "Prior experiences of being blamed may affect your willingness to trust.",
    "tendencies": [
      "May prefer limited interaction until trust improves.",
      "Do not infer intent from isolation alone."
    ]
  },
  "actor.yasushi-mishima": {
    "public": "Adult team leader who enjoys strategic contests.",
    "private": "You may value entertainment and advantage rather than only financial return.",
    "tendencies": [
      "Consider negotiated deals and leverage.",
      "No guarantee that counterpart accepts."
    ]
  },
  "actor.tsukasa-kaidou": {
    "public": "Adult team leader and Satone's father.",
    "private": "Family interests and financial obligations may influence your decisions.",
    "tendencies": [
      "Negotiate clear terms when offering financial assistance.",
      "Do not invent capital or assume others repay."
    ]
  },
  "actor.saika-kamishiro": {
    "public": "Adult team leader with experience in controlling social narratives.",
    "private": "You know your own commitments and group membership.",
    "tendencies": [
      "Prefer persuasion and group influence.",
      "Social influence is uncertain and constrained by observed reactions."
    ]
  },
  "actor.kimiko-kyouguchi": {
    "public": "Adult contestant; others may perceive her as reckless.",
    "private": "Trust and reciprocity can matter in an uncertain environment.",
    "tendencies": [
      "May consider mutually binding information exchanges.",
      "Sharing a secret is a risk; no automatic reciprocity."
    ]
  },
  "actor.sora-namiragawa": {
    "public": "Adult contestant with sparse behavioral evidence.",
    "private": "No additional hidden facts supplied.",
    "tendencies": [
      "Evaluate reciprocal trust and disclosure risks.",
      "Do not infer other contestants' secrets."
    ]
  }
});
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
