# Standalone reference runner

Node 22+; no third-party packages. From this directory:

```sh
node validate-corpus.mjs
node smoke.mjs
node adversarial.mjs
node cli.mjs --out /tmp/tg.owner-run.json
node cli.mjs --replay /tmp/tg.owner-run.json
````

Set TG_LLM_URL, TG_LLM_MODEL, optionally TG_LLM_KEY; run node cli.mjs --llm for a first-game LLM experiment. The provider must support JSON-compatible chat completions. Invalid responses fail closed. Owner run JSON includes private state and host inputs; never distribute it to players. The standalone runner is not connected to native COTE CSF, RAG, UI or physical engine. Other games can accept scenario-provided trusted host adjudications and scripted decisions; the environment and complete action menus are not autonomous. LLM decision quality and provider integration are untested. The curated initial prior is deliberately spoiler-safe and covers only five Group C actors; later-game profiles need projected priors. No canon winner enforced.
