import * as m from './resolver.mjs';
function adversarial(m){
 let n=0;const check=(b,msg)=>{if(!b)throw Error(msg);n++};const reject=(f,msg)=>{let ok=false;try{f()}catch{ok=true}check(ok,msg)};
 const P=["a","b","c","d","e"],votes=v=>Object.fromEntries(P.map(p=>[p,{answer:v}]));
 let s=m.create("kokkuri-san",P);let t=m.step(s,votes("no"));check(!t.terminal,"R01 incorrect unanimity");check(t.debt.a===2000000,"no halves asker");t=m.step(s,votes("yes"));check(t.cash.a===0&&t.debt.a===3600000,"R02 debt reward");
 check(m.view(s,"a").private.questionCard.includes("halves")&&m.view(s,"b").private.questionCard===null,"private card");
 reject(()=>m.create("constructor",P),"unknown game prototype");reject(()=>m.create("fake-game",["a","toString"]),"prototype roster");
 s=m.create("friendless",["a","b","c"],{passwords:{a:"A",b:"B",c:"C"}});t=m.step(s,{a:{report:{target:"b",password:"B"}},b:{report:{target:"a",password:"A"}}});check(t.active.length===1&&t.active[0]==="c","R03 simultaneous");const r=m.step(s,{b:{report:{target:"a",password:"A"}},a:{report:{target:"b",password:"B"}}});check(JSON.stringify(t)===JSON.stringify(r),"action input order independent");
 s=m.create("all-bet",["a","b"],{cash:{a:3000000,b:3000000}});let b={id:"__proto__",a:"a",b:"b",stake:1,kind:"rps"},contract=JSON.stringify([b.id,b.a,b.b,b.stake,b.kind]);reject(()=>m.step(s,{a:{contract,hand:"rock"},b:{contract,hand:"paper"}},{settlements:[b]}),"R04 prototype wager");
 s=m.create("friends-prison",P);reject(()=>m.step(s,{a:{escape:true}},{escapes:[{actor:"a",key:"k"}]}),"R06 missing authorization");t=m.step(s,{a:{escape:true}},{escapes:[{actor:"a",key:"k",keyAcquired:true,guardsAbsent:true}]});check(t.cash.a===200000000&&t.debt.b===400000000,"authorized escape");reject(()=>m.step(t,{b:{escape:true}},{escapes:[{actor:"b",key:"k",keyAcquired:true,guardsAbsent:true}]}),"reused key");
 s=m.create("hide-and-seek",["a","b","c","d"],{teams:{a:"A",b:"A",c:"B",d:"B"},hiders:{A:"a",B:"c"},debt:{a:100,b:100,c:100,d:100}});t=m.step(s,{}, {violators:["b"],discoveredHiders:["c"]});check(t.terminal&&t.debt.a===200&&t.debt.c===100,"violation penalty separate");
 return n
}
console.log('PASS '+adversarial(m)+' adversarial checks');
