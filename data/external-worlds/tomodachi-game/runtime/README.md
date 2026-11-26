# Standalone reference runner

Node 22+; no third-party packages. From this directory:

```sh
node validate-corpus.mjs
node smoke.mjs
node adversarial.mjs
node full-runs.mjs
node cli.mjs --out /tmp/tg.owner-run.json
node cli.mjs --replay /tmp/tg.owner-run.json
```

For an optional first-game LLM experiment, set TG_LLM_URL to the full OpenAI-compatible chat completions endpoint, TG_LLM_MODEL, and optionally TG_LLM_KEY; run node cli.mjs --llm. Invalid response/action fails closed. Provider and model behavior not tested. Owner run includes private state and host inputs; never distribute it to players. Standalone runner is not connected to native COTE CSF, RAG, UI or physical engine. Other games accept scenario-provided trusted host adjudications; environment and full action menus are not autonomous. 20 curated actor priors and seven actor-specific case-guidance projections. Projection transfers abstract reasoning tendencies, not future events or guaranteed outcomes. No canon winner enforced.
