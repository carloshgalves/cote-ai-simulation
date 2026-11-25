// Isolated deterministic reference resolver; not a CSF adapter.
const configs={
  "kokkuri-san": {
    "source": "%22Kokkuri-San%22_Game",
    "players": 5,
    "rounds": 5,
    "prize": 2000000,
    "initialDebt": 4000000,
    "model": [
      "questioner order configured by roster; integer rounding down on half-debt; prize applied to debt; failed-game tie shares liability; final subjective question uses configured answer"
    ]
  },
  "backbiting-sugoroku": {
    "source": "Backbiting_Sugoroku",
    "players": 5,
    "boardLength": 20,
    "prize": 2000000,
    "nonFinisherBonus": 1000000,
    "maxRounds": 20,
    "model": [
      "board length and tile table scenario parameters; audience rankings and false-card rulings are trusted adjudicator facts; tied finishers share accumulated game debt"
    ]
  },
  "hide-and-seek": {
    "source": "Friendship_Hide-And-Seek",
    "prize": 2000000,
    "deadline": 30,
    "loserPenalty": 2000000,
    "model": [
      "deadline and loss debt chosen for bounded simulation; trusted environment resolves discovery, supply and prohibited conduct; transfer liability modeled separately"
    ]
  },
  "friend-sin-trial": {
    "source": "Friend-Sin_Trial",
    "days": 7,
    "jurors": 28,
    "jurorFunds": 70000,
    "target": 1960000,
    "accusedDebt": 500000000,
    "model": [
      "jury decisions supplied by independent juror agents, no direct player payments; all-or-nothing threshold; debt divided among configured group"
    ]
  },
  "friends-prison": {
    "source": "Friend%27s_Prison_Game",
    "days": 7300,
    "dominoes": 120000,
    "clearPrize": 100000000,
    "exilePenalty": 100000000,
    "escapePrize": 200000000,
    "escapePenalty": 400000000,
    "model": [
      "one day per transition, work amounts supplied by environment; key acquisition and guard absence adjudicated by environment; no automatic escape"
    ]
  },
  "all-bet": {
    "source": "All_Bet",
    "days": 7,
    "boardingFee": 1000000,
    "feePenalty": 100000000,
    "breachPenalty": 100000000000,
    "model": [
      "voyage duration configurable; balances denominated in yen; arbitrary wager games need external trusted resolver, built-in coin and RPS outcomes; contracts use explicit consent and escrow-equivalent balance checks"
    ]
  },
  "friend-murder": {
    "source": "Friend_Murder_Game",
    "days": 10,
    "yellowPrice": 1000000,
    "redPrice": 100000000,
    "model": [
      "duration configurable; abstract non-graphic pursuit tokens; capture/defense resolved by trusted environment, not player declarations; loss confiscates current assets and flags future forfeiture"
    ]
  },
  "friendless": {
    "source": "The_Friendless_Game",
    "days": 14,
    "prize": 4500000000,
    "model": [
      "wrong-password penalty configured as exclusion; ties share prize; individualized tasks adjudicated by environment and do not invent canon task consequences"
    ]
  },
  "friendship-janken": {
    "source": "Friendship_Janken",
    "players": 5,
    "initialDebt": 1000000,
    "rounds": 3,
    "prize": 2000000,
    "model": [
      "five-player three-card game; winners transfer up to 500000 yen debt each; split between losers; aggregate equal net wins treated as draw; early failure when debt concentrates on one player"
    ]
  },
  "fake-game": {
    "source": "Episode_12",
    "rounds": 1,
    "model": [
      "unofficial optional symbolic RPS encounter; no injury execution; no official debt progression"
    ]
  }
};
function assert(ok,msg){if(!ok)throw new Error(msg)}

function split(n,ids,fn){assert(Number.isSafeInteger(n)&&n>=0&&ids.length>0,"bad split");const a=[...ids].sort();a.forEach((id,i)=>fn(id,Math.floor(n/a.length)+(i<n%a.length?1:0)))}

function clone(x){return JSON.parse(JSON.stringify(x))}

function amount(x){assert(Number.isSafeInteger(x)&&x>=0,"invalid money/quantity");return x}

function create(game,players,setup={}){
 assert(Object.hasOwn(configs,game),"unknown game");const c=configs[game];assert(Array.isArray(players)&&players.length>=2&&new Set(players).size===players.length&&players.every(p=>typeof p==="string"&&/^[a-zA-Z0-9_.-]+$/.test(p)&&!Object.hasOwn(Object.prototype,p)&&p!=="prototype"),"bad roster");
 if(c.players)assert(players.length===c.players,"wrong roster size");if(game==="fake-game")assert(players.length===2,"two players required");
 const s={game,round:0,terminal:false,winners:[],players:[...players],active:[...players],debt:{},cash:{},publicLog:[],private:{},config:clone(c),data:{}};
 for(const p of players){s.debt[p]=amount(setup.debt?.[p]??c.initialDebt??0);s.cash[p]=amount(setup.cash?.[p]??0);s.private[p]={};}
 if(game==="backbiting-sugoroku"){s.data.pos=Object.fromEntries(players.map(p=>[p,0]));s.data.gameDebt=Object.fromEntries(players.map(p=>[p,0]));s.data.tiles=clone(setup.tiles??{});for(const v of Object.values(s.data.tiles))assert(Number.isSafeInteger(v),"bad board tile")}
 if(game==="hide-and-seek"){const teams=setup.teams,hiders=setup.hiders;assert(teams&&hiders&&players.every(p=>["A","B"].includes(teams[p]))&&hiders.A!==hiders.B&&teams[hiders.A]==="A"&&teams[hiders.B]==="B","invalid teams/hiders");s.data.teams=clone(teams);s.data.originalTeams=clone(teams);s.data.hiders=clone(hiders);s.data.switched=[];s.data.supplied={};for(const p of players)s.private[p].role=hiders[teams[p]]===p?"hider":"seeker"}
 if(game==="friend-sin-trial"){assert(Array.isArray(setup.jurors)&&setup.jurors.length===c.jurors&&new Set(setup.jurors).size===c.jurors&&setup.jurors.every(j=>typeof j==="string"&&!players.includes(j)),"bad jurors");s.data.jurors=[...setup.jurors];s.data.paid=[];}
 if(game==="friends-prison"){s.data.dominoes=0;s.data.escaped=[];s.data.exiled=[];s.data.usedKeys=[]}
 if(game==="all-bet"){s.data.sold={};s.data.contracts={};s.data.valuations={};for(const p of players)s.data.valuations[p]=amount(setup.valuations?.[p]??0)}
 if(game==="friend-murder"){s.data.forfeiture=[];s.data.tokens=Object.fromEntries(players.map(p=>[p,{yellow:0,red:0}]))}
 if(game==="friendless"){assert(setup.passwords&&players.every(p=>typeof setup.passwords[p]==="string"&&setup.passwords[p].length>0)&&new Set(players.map(p=>setup.passwords[p])).size===players.length,"bad passwords");for(const p of players)s.private[p].password=setup.passwords[p];s.data.excluded=[];s.data.completed={};}
 if(game==="friendship-janken"){s.data.hands=Object.fromEntries(players.map(p=>[p,["rock","paper","scissors"]]));s.data.points=Object.fromEntries(players.map(p=>[p,0]));}
 return s
}

function rps(a,b){assert(["rock","paper","scissors"].includes(a)&&["rock","paper","scissors"].includes(b),"invalid hand");return a===b?0:((a==="rock"&&b==="scissors")||(a==="scissors"&&b==="paper")||(a==="paper"&&b==="rock"))?1:-1}

function view(s,actor){assert(s.players.includes(actor),"unknown viewer");const out={game:s.game,round:s.round,terminal:s.terminal,active:[...s.active],winners:[...s.winners],ownCash:s.cash[actor],ownDebt:s.debt[actor],publicLog:clone(s.publicLog),private:clone(s.private[actor]),publicRules:clone(s.config)};if(s.game==="kokkuri-san"&&!s.terminal){out.questioner=s.players[s.round];out.private.questionCard=actor===out.questioner?(s.round<4?"A yes result clears the game; a no result halves your own debt.":"Final question concerns continued friendship.") : null;}if(s.game==="backbiting-sugoroku")out.positions=clone(s.data.pos);if(s.game==="hide-and-seek")out.team=s.data.teams[actor];if(s.game==="friends-prison")out.dominoes=s.data.dominoes;if(s.game==="friendship-janken")out.cards=clone(s.data.hands[actor]);return out}

function step(before,actions={},host={}){
 assert(!before.terminal,"terminal");const s=clone(before),c=s.config,d=s.data,p=s.players,a=s.active;
 assert(actions&&typeof actions==="object"&&!Array.isArray(actions),"bad actions");for(const k of Object.keys(actions))assert(a.includes(k)&&actions[k]&&typeof actions[k]==="object"&&!Array.isArray(actions[k]),"inactive, unknown or malformed actor action");
 const act=id=>Object.hasOwn(actions,id)?actions[id]:{}, log=(event,details={})=>s.publicLog.push({round:s.round,event,...details});
 const debt=(id,n)=>{assert(p.includes(id),"unknown debtor");s.debt[id]=amount(s.debt[id]+n)};
 const credit=(id,n)=>{assert(p.includes(id),"unknown account");s.cash[id]=amount(s.cash[id]+n)};
 const finish=(winners)=>{assert(new Set(winners).size===winners.length&&winners.every(x=>p.includes(x)),"bad winners");s.terminal=true;s.winners=[...winners];log("terminal",{winners:[...winners]})};
 const remove=id=>{assert(s.active.includes(id),"inactive target");s.active=s.active.filter(x=>x!==id)};
 const transfer=(from,to,n)=>{assert(from!==to&&p.includes(from)&&p.includes(to),"bad transfer");amount(n);assert(s.cash[from]>=n,"insufficient funds");credit(from,-n);credit(to,n)};
 const requireAll=()=>assert(a.every(id=>Object.hasOwn(actions,id)),"missing simultaneous action");
 s.round++;
 switch(s.game){
 case "kokkuri-san":{
 requireAll();const q=p[s.round-1],votes=a.map(id=>act(id).answer);assert(votes.every(v=>v==="yes"||v==="no"),"bad answer");
 const yes=votes.filter(v=>v==="yes").length;const answer=yes===0?"no":yes===p.length?"yes":yes<p.length-yes?"yes":"no";
 for(const id of a)if(id!==q&&act(id).spoke===true)debt(id,s.debt[id]);
 if(answer==="no"&&s.round<5)s.debt[q]=Math.floor(s.debt[q]/2);
 const correct=s.round<5?"yes":host.finalAnswer;assert(["yes","no"].includes(correct),"final answer required");
 log("coin_result",{answer,questioner:q});if(answer===correct){split(c.prize,p,(id,n)=>{s.debt[id]=Math.max(0,s.debt[id]-n)});finish(p)}else if(s.round===c.rounds){const max=Math.max(...p.map(id=>s.debt[id])),worst=p.filter(id=>s.debt[id]===max),total=p.reduce((n,id)=>n+s.debt[id],0);for(const id of p)s.debt[id]=0;split(total,worst,debt);finish([])}break;}
 case "backbiting-sugoroku":{
 requireAll();for(const id of a)assert(Array.isArray(act(id).cards)&&act(id).cards.every(x=>typeof x==="string"),"invalid cards");let moves={};
 if(a.every(id=>act(id).cards.length===0))for(const id of a)moves[id]=1;
 else {assert(Array.isArray(host.ranking)&&host.ranking.length===a.length&&new Set(host.ranking).size===a.length&&host.ranking.every(id=>a.includes(id)),"bad audience ranking");host.ranking.forEach((id,i)=>moves[id]=a.length-i)}
 for(const id of a){d.pos[id]+=moves[id];const tile=d.tiles[d.pos[id]]??0;d.gameDebt[id]=Math.max(0,d.gameDebt[id]+tile)}
 for(const id of host.provenLiars??[]){assert(a.includes(id)&&act(id).cards.length,"invalid liar ruling");d.pos[id]=Math.max(...a.map(x=>d.pos[x]))+1}
 log("movement",{positions:clone(d.pos)});
 if(a.some(id=>d.pos[id]>=c.boardLength)||s.round>=c.maxRounds){const max=Math.max(...a.map(id=>d.pos[id])),losers=a.filter(id=>d.pos[id]>=c.boardLength||(s.round>=c.maxRounds&&d.pos[id]===max)),w=a.filter(id=>!losers.includes(id));split(Object.values(d.gameDebt).reduce((n,x)=>n+x,0),losers,debt);split(c.prize,p,credit);for(const id of w)credit(id,c.nonFinisherBonus);finish(w)}break;}
 case "hide-and-seek":{
 for(const id of a){const x=act(id);if(x.betray!==undefined){const target=x.betray;assert(!Object.values(d.hiders).includes(id)&&!d.switched.includes(id)&&a.includes(target)&&d.teams[target]!==d.teams[id]&&act(target).accept===id,"invalid switch");d.teams[id]=d.teams[target];d.switched.push(id);log("team_switch",{actor:id,team:d.teams[id]})}}
 let losing=null;
 const violations=host.violators??[];assert(Array.isArray(violations)&&violations.every(id=>a.includes(id)),"bad violation");
 const surrender=a.filter(id=>act(id).surrender===true);assert(surrender.every(id=>Object.values(d.hiders).includes(id)),"only hider can surrender");
 const discoveries=host.discoveredHiders??[];assert(discoveries.every(id=>Object.values(d.hiders).includes(id)),"invalid discovery");
 const lost=new Set([...violations,...surrender,...discoveries].map(id=>d.teams[id]));
 if(lost.size===2){for(const id of a.filter(x=>violations.some(v=>d.teams[v]===d.teams[x])))debt(id,s.debt[id]);finish([]);break}
 if(lost.size===1)losing=[...lost][0];
 if(losing){const w=a.filter(id=>d.teams[id]!==losing),l=a.filter(id=>d.teams[id]===losing);if(violations.some(id=>d.teams[id]===losing))for(const id of l)debt(id,s.debt[id]);split(c.loserPenalty,l,debt);split(c.prize,w,credit);for(const id of d.switched){if(w.includes(id)){const orig=p.filter(x=>x!==id&&d.originalTeams[x]===d.originalTeams[id]);if(orig.length)split(c.loserPenalty,orig,debt)} }finish(w)}
 else if(s.round>=c.deadline)finish([]);break;}
 case "friend-sin-trial":{
 if(s.round===1&&host.accepted===false){finish([]);break}
 for(const j of host.jurorPayments??[]){assert(d.jurors.includes(j)&&!d.paid.includes(j),"invalid/duplicate jury payment");d.paid.push(j)}
 log("jury_total",{total:d.paid.length*c.jurorFunds});
 if(d.paid.length*c.jurorFunds>=c.target){split(c.accusedDebt,p,debt);finish(p)}else if(s.round>=c.days)finish([]);break;}
 case "friends-prison":{
 const viol=host.violators??[];for(const id of viol){remove(id);debt(id,c.exilePenalty);d.exiled.push(id)}
 for(const e of host.escapes??[]){assert(e&&typeof e.key==="string"&&e.key.length&&!d.usedKeys.includes(e.key)&&s.active.includes(e.actor)&&act(e.actor).escape===true&&e.keyAcquired===true&&e.guardsAbsent===true,"invalid escape");d.usedKeys.push(e.key);remove(e.actor);credit(e.actor,c.escapePrize);d.escaped.push(e.actor);for(const id of s.active)debt(id,c.escapePenalty)}
 const voters=s.active,counts={};for(const id of voters){const t=act(id).vote;if(t!==undefined){assert(voters.includes(t),"bad vote");counts[t]=(counts[t]??0)+1}}
 const exile=voters.find(id=>(counts[id]??0)>voters.length/2);if(exile){remove(exile);debt(exile,c.exilePenalty);d.exiled.push(exile);log("exile",{actor:exile})}
 d.dominoes=Math.min(c.dominoes,d.dominoes+amount(host.dominoesPlaced??0));if(!s.active.length||d.dominoes>=c.dominoes||s.round>=c.days){for(const id of s.active)credit(id,c.clearPrize);finish([...d.escaped,...s.active])}break;}
 case "all-bet":{
 for(const id of a){const x=act(id);if(x.sell!==undefined){assert(!d.sold[id]&&a.includes(x.sell)&&x.sell!==id&&!d.sold[x.sell]&&act(x.sell).consentSale===id,"invalid sale");d.sold[x.sell]={buyer:id,price:d.valuations[x.sell]};credit(id,d.valuations[x.sell])}if(x.buyback!==undefined){const t=x.buyback,sale=d.sold[t];assert(sale&&sale.buyer===id&&s.cash[id]>=sale.price,"invalid buyback");credit(id,-sale.price);delete d.sold[t]}}
 for(const b of host.settlements??[]){assert(typeof b.id==="string"&&/^[a-zA-Z0-9_.-]+$/.test(b.id)&&!Object.hasOwn(Object.prototype,b.id)&&b.id!=="prototype"&&!Object.hasOwn(d.contracts,b.id)&&a.includes(b.a)&&a.includes(b.b)&&b.a!==b.b&&!d.sold[b.a]&&!d.sold[b.b],"invalid/replayed wager");amount(b.stake);assert(b.stake>0&&s.cash[b.a]>=b.stake&&s.cash[b.b]>=b.stake,"unfunded wager");
 const terms=JSON.stringify([b.id,b.a,b.b,b.stake,b.kind]);assert(act(b.a).contract===terms&&act(b.b).contract===terms,"missing consent");
 let win=b.winner;if(b.kind==="rps"){const v=rps(act(b.a).hand,act(b.b).hand);win=v===0?null:v>0?b.a:b.b}else if(b.kind==="coin"){assert(["heads","tails"].includes(b.coin)&&["heads","tails"].includes(act(b.a).guess)&&["heads","tails"].includes(act(b.b).guess)&&act(b.a).guess!==act(b.b).guess,"bad coin");win=act(b.a).guess===b.coin?b.a:b.b}else assert(b.kind==="adjudicated","unsupported wager");
 assert(win===null||win===b.a||win===b.b,"invalid winner");if(win!==null)transfer(win===b.a?b.b:b.a,win,b.stake);d.contracts[b.id]=true;log("wager_settled",{id:b.id,winner:win})}
 if(s.round>=c.days){const w=[];for(const id of p){if(s.cash[id]>=c.boardingFee){credit(id,-c.boardingFee);if(!d.sold[id])w.push(id)}else debt(id,c.feePenalty)}finish(w)}break;}
 case "friend-murder":{
 for(const id of a){const x=act(id);for(const k of ["yellow","red"]){const qty=amount(x.buy?.[k]??0),cost=amount(qty*c[k+"Price"]);assert(s.cash[id]>=cost,"insufficient funds");credit(id,-cost);d.tokens[id][k]+=qty}if(x.transfer){const {to,n}=x.transfer;assert(a.includes(to),"inactive recipient");transfer(id,to,n)}}
 for(const e of host.defenses??[]){assert(a.includes(e.actor)&&["yellow","red"].includes(e.kind)&&d.tokens[e.actor][e.kind]>0&&act(e.actor).defend===e.kind,"invalid defense");d.tokens[e.actor][e.kind]--}
 for(const id of host.captured??[]){remove(id);s.cash[id]=0;d.forfeiture.push(id);log("capture",{actor:id})}
 if(s.round>=c.days||!s.active.length)finish(s.active);break;}
 case "friendless":{
 const eliminated=new Set();for(const id of a){const x=act(id);if(x.report!==undefined){const t=x.report.target;assert(a.includes(t)&&t!==id&&typeof x.report.password==="string","bad report");eliminated.add(x.report.password===s.private[t].password?t:id)}}for(const id of a){if(eliminated.has(id)){remove(id);d.excluded.push(id);log("excluded",{actor:id})}}
 for(const e of host.completedTasks??[]){assert(s.active.includes(e.actor)&&typeof e.task==="string","invalid task");(d.completed[e.actor]??=[]).push(e.task)}
 if(s.round>=c.days||s.active.length<=1){const active=s.active;const count=Object.fromEntries(active.map(id=>[id,0]));for(const id of active){const t=act(id).vote;if(t!==undefined){assert(active.includes(t)&&t!==id,"invalid election vote");count[t]++}}if(active.length){const max=Math.max(...Object.values(count)),w=active.filter(id=>count[id]===max);for(const id of active)s.debt[id]=0;split(c.prize,w,credit);finish(w)}else finish([])}break;}
 case "friendship-janken":{
 requireAll();const hands=Object.fromEntries(a.map(id=>[id,act(id).hand]));for(const id of a){assert(d.hands[id].includes(hands[id]),"invalid or reused card");d.hands[id]=d.hands[id].filter(k=>k!==hands[id])}
 const distinct=[...new Set(Object.values(hands))];
 if(distinct.length===2){const winning=rps(distinct[0],distinct[1])>0?distinct[0]:distinct[1],w=a.filter(id=>hands[id]===winning),l=a.filter(id=>hands[id]!==winning);for(const id of w){const n=Math.min(500000,s.debt[id]);debt(id,-n);split(n,l,debt);d.points[id]++}for(const id of l)d.points[id]--;}
 log("janken_reveal",{hands});const debtors=a.filter(id=>s.debt[id]>0);
 if(debtors.length===1){debt(debtors[0],s.debt[debtors[0]]);finish([])}
 else if(s.round>=c.rounds){if(new Set(Object.values(d.points)).size===1){split(c.prize,p,(id,n)=>{s.debt[id]=Math.max(0,s.debt[id]-n)});finish(p)}else{for(const id of a)debt(id,s.debt[id]);finish([])}}break;}
 case "fake-game":{
 requireAll();const [x,y]=a,v=rps(act(x).hand,act(y).hand);finish(v===0?[]:v>0?[x]:[y]);break;}
 default:throw new Error("unknown game");
 }
 for(const id of p){amount(s.debt[id]);amount(s.cash[id])}return s
}

function smoke(){
 let n=0;const eq=(a,b)=>{assert(JSON.stringify(a)===JSON.stringify(b),`mismatch ${JSON.stringify(a)} / ${JSON.stringify(b)}`);n++};const reject=fn=>{let ok=false;try{fn()}catch{ok=true}assert(ok,"expected rejection");n++};
 const P=["a","b","c","d","e"], ballots=(p,v)=>Object.fromEntries(p.map(x=>[x,v]));
 let s=create("kokkuri-san",P),t=step(s,ballots(P,{answer:"yes"}));eq(t.winners,P);eq(t.debt.a,3600000);eq(s.round,0);eq(step(s,ballots(P,{answer:"yes"})),t);reject(()=>step(t,{}));reject(()=>step(s,{a:{answer:"yes"}}));
 s=create("backbiting-sugoroku",P);for(let i=0;i<20&&!s.terminal;i++)s=step(s,ballots(P,{cards:[]}));eq(s.terminal,true);eq(s.winners,[]);
 s=create("hide-and-seek",["a","b","c","d"],{teams:{a:"A",b:"A",c:"B",d:"B"},hiders:{A:"a",B:"c"}});reject(()=>step(s,{b:{surrender:true}}));eq(step(s,{a:{surrender:true}}).winners,["c","d"]);eq(view(s,"b").private,{role:"seeker"});reject(()=>step(s,{b:{betray:"c"}}));
 let jury=Array.from({length:28},(_,i)=>"j"+i);s=create("friend-sin-trial",P,{jurors:jury});t=step(s,{}, {jurorPayments:jury});eq(t.winners,P);eq(t.debt.a,100000000);reject(()=>step(s,{}, {jurorPayments:[jury[0],jury[0]]}));
 s=create("friends-prison",P);t=step(s,ballots(P,{vote:"e"}),{dominoesPlaced:120000});eq(t.winners,["a","b","c","d"]);eq(t.debt.e,100000000);reject(()=>step(s,{}, {escapes:[{actor:"a",key:"k"}]}));
 s=create("all-bet",["a","b"],{cash:{a:3000000,b:3000000}});const b={id:"w",a:"a",b:"b",stake:1000000,kind:"rps"},contract=JSON.stringify(["w","a","b",1000000,"rps"]);const acts={a:{contract,hand:"rock"},b:{contract,hand:"scissors"}};t=step(s,acts,{settlements:[b]});eq(t.cash,{a:4000000,b:2000000});reject(()=>step(t,acts,{settlements:[b]}));reject(()=>step(s,{a:acts.a},{settlements:[b]}));
 s=create("friend-murder",["a","b"],{cash:{a:1000000,b:1000000}});t=step(s,{a:{buy:{yellow:1},defend:"yellow"}},{defenses:[{actor:"a",kind:"yellow"}],captured:["b"]});eq(t.cash,{a:0,b:0});eq(t.data.forfeiture,["b"]);reject(()=>step(s,{a:{buy:{red:1}}}));
 s=create("friendless",["a","b","c"],{passwords:{a:"X",b:"Y",c:"Z"}});eq(view(s,"a").private,{password:"X"});assert(!JSON.stringify(view(s,"a")).includes('"Y"'),"secret leak");n++;t=step(s,{a:{report:{target:"b",password:"Y"}}});eq(t.active,["a","c"]);eq(step(s,{a:{report:{target:"b",password:"wrong"}}}).active,["b","c"]);reject(()=>step(s,{a:{report:{target:"a",password:"X"}}}));
 s=create("friendship-janken",P);for(const h of ["rock","paper","scissors"])s=step(s,ballots(P,{hand:h}));eq(s.winners,P);eq(s.debt.a,600000);s=create("friendship-janken",P);s=step(s,{...ballots(P,{hand:"paper"}),a:{hand:"rock"}});eq(s.debt.a,3000000);s=step(s,{...ballots(P,{hand:"scissors"}),a:{hand:"paper"}});eq(s.terminal,true);eq(s.debt.a,10000000);
 s=create("fake-game",["a","b"]);eq(step(s,{a:{hand:"paper"},b:{hand:"rock"}}).winners,["a"]);
 return n
}
export {configs,create,view,step,smoke};
