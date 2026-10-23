# Operação das automações de desenvolvimento

O protocolo principal continua em `docs/agents/implementation-pipeline.md`. Estas ferramentas **estão versionadas nesta branch/PR**, mas os workflows `workflow_dispatch` só aparecem/funcionam após sua integração à branch padrão **main**. Não houve integração nem execução de agentes com chave neste commit.

## 1. Hooks pós-edição (Claude Code local)

- `.claude/settings.json` executa `scripts/agents/post_edit_check.py` em `PostToolUse` para `Edit|Write|MultiEdit`. Não edita arquivos automaticamente.
- Verifica parse Python/JSON, `gofmt -d` sem correção, e `node --check` em JavaScript normal. TS/TSX, Java, .yml, testes completos e regressão de browser continuam com as suites do projeto.
- Requer `python3` no terminal em que Claude Code é iniciado; `gofmt` para arquivos Go e `node` para JS. Hook se aplica quando Claude Code carrega settings dessa branch e não é uma barreira de segurança.
- Validação offline: `python3 -m unittest discover -s scripts/agents -p 'test_*.py' -v`.

## 2. Mutações isoladas e restauradas

Com o código já commitado e os testes relevantes verdes, usar:

```bash
python3 scripts/agents/mutation_probe.py \
  --file caminho/para/arquivo.py \
  --old 'expressao exata' --new 'expressao mutada' \
  --must-contain 'FAILED' \
  -- python3 -m pytest -q tests/test_regressao.py
```

O probe executa baseline, injeta um único mutante numa **worktree descartável do HEAD**, exige RED com marcador esperado, destrói a worktree e preserva o checkout original. Para Go, usar `-- go test -count=1 ./internal/... `; para Java, `-- ./mvnw ... test`. O comando deve caber no limite de 900s. Ausência de mutação detectada = **NO-GO**, não teste aprovado. SIGKILL/queda de VM pode deixar apenas o worktree temporário; limpeza posterior deve ser restrita ao diretório registrado, nunca `git worktree prune`/prune global sem inspeção.

## 3. Fleet real de agentes de implementação

Depois do merge, em **Actions → Bounded agent implementation fleet → Run workflow**, selecione uma Issue aberta que já tenha sido explicitamente marcada com o label `ready-for-agent`; até outras duas Issues realmente independentes são opcionais. Somente `carloshgalves` está autorizado pelo workflow a disparar essa fleet.

- **Isolamento de credenciais**: cada worker de OpenCode recebe um GITHUB_TOKEN apenas de leitura, com uma chave Groq e um worktree próprio. O código do agente pode ser não confiável. O job **publish** nasce em outro runner, obtém scripts da branch padrão e recebe separadamente o GITHUB_TOKEN de escrita. Nunca executa código recebido do worker.
- O worker coleta só arquivos rastreados e novos não ignorados da allowlist `internal src tests evals cmd`, com limites de tamanho e proibição de segredos comuns/symlinks, e gera um `manifest.json` com hashes SHA-256. `actions/upload-artifact` persiste o WIP, mesmo se a inferência falhar normalmente. O publicador verifica integridade, allowlist e symlinks antes de restaurar o snapshot e executar `fleet_checkpoint.sh`.
- **Um único worker de inferência por repositório neste estágio** (`max-parallel: 1`). Há no máximo três slots por disparo, mas são processados serialmente; no máximo uma execução fleet por repositório. Cotas da organização Groq **não são sincronizadas entre projetos**. Não executar fleets simultâneas em repositórios diferentes até instalar um escalonador central.
- Uma sessão de **6 minutos por Issue por disparo**, seguida de artifact e publicação separada. Nova execução da mesma Issue retoma sua branch `agent/fleet/issue-N`. Se a VM cair antes do upload do artifact, as edições da sessão podem ser perdidas. Checkpoint WIP não é CI verde; ausência de alteração não cria PR.
- O publicador salva somente a branch de checkpoint; a PR continua pertencendo à fase separada de code-review.
- O modelo padrão é `openai/gpt-oss-20b`, opcionalmente `openai/gpt-oss-120b`; a organização ainda está sujeita aos limites efetivos de TPM/RPM/RPD e à disponibilidade desses modelos.
- A publicação com escrita está isolada do shell do agente, mas **não é um sandbox completo**: workloads maliciosos podem tentar exfiltrar a chave de inferência ou alterar os arquivos enviados ao artifact. Usar somente Issues aprovadas e evitar dados privados/clientes.

Consulte `docs/agents/autonomous-orchestration-policy.md` para o escopo futuro de ciclos completos. Ela é **uma proposta NÃO ATIVA**; não autoriza IMPLEMENT -> REVIEW -> REMEDIATE automaticamente.

## 4. Revisão headless (somente candidatos não verificados)

Em **Actions → Agent headless PR review → Run workflow**, informar a `pr_number`, a `issue_number` aceita e o modelo.

1. O workflow extrai Issue e **diff completo** de PR aberta contra seu HEAD exato, sem checkout/execução do código da PR.
2. **Falha fechada** quando diff excede 12.000 caracteres ou Issue excede 4.000; não trunca fingindo revisão completa.
3. O Groq retorna JSON com candidatos sustentados pelo diff. A etapa confiável confere HEAD ainda idêntico e publica comentário como `ai-headless-review:v1`, sem o marcador canônico de finding OPEN.
4. Isso não executa testes nem fecha findings. Um reviewer independente, conforme a skill original de cada repo, deve validar ou descartar sugestões e realizar review final; **zero candidatos não implica GO**.

**Privacidade**: o envio da Issue, diffs e instruções de agente ao Groq é explícito. Antes de acionar, confirme que não há dados de clientes, manuscritos ou segredos no material.

## 5. Permissões e ativação

- Integrar a PR de infraestrutura após revisão. Até lá, o conteúdo dos workflows pode ser verificado via `pull_request` (Agent harness validation), mas `workflow_dispatch` requer workflow presente na branch padrão.
- Configurar `GROQ_API_KEY` em **Settings → Secrets and variables → Actions** deste repositório. O IWrite pode reutilizar a credencial existente se continuar válida; não copiar chave de uma PR ou arquivo.
- Garantir Actions habilitado e runners GitHub-hosted disponíveis; em repositórios privados, conferir billing/minutos e políticas de Actions se jobs falharem sem adquirir runner.
- Se uma automação futura criar Draft PR, a opção **Allow GitHub Actions to create and approve pull requests** precisa permitir a ação. Nenhuma aprovação ou merge automático está prevista.
- `Agent harness validation` em PR roda testes offline e actionlint **sem** chave de modelo. `agent-implementation-fleet` e `headless-agent-review` são **manuais**, nunca disparados por `pull_request_target`.
- Se falhar um provider, registrar o resultado e conservar o branch/PR checkpoint; **não** rotular a sessão como verde só porque `git push` deu certo.

## 6. Restrições especiais deste projeto

`implement`, `code-review`, `address-review-findings` are separate explicit invocations. The fleet ends strictly at implementation checkpoint; **no Draft PR is opened here**. The later `code-review` skill owns the Draft PR/finding format. Require deterministic kernel tests and versioned evals before merge.

## 7. Critérios de aceite das automações

- Harness offline passou + YAML/actionlint passaram em CI.
- Pilotar uma Issue **sintética ou de baixo risco** via fleet, observar execução real e branch WIP.
- Revisão headless manual em PR com conteúdo compartilhável, conferir comentário/head SHA; não registrar candidatos como findings formais sem prova.
- Testar retomada da Issue após falha simulada; verificar que só paths allowlisted foram persistidos e que o publicador executou em runner distinto.
- Rever segurança/custo/cotas e logs antes de ampliar o número de agentes.

Se o runner nem iniciar ou a cota estiver esgotada, classificar como **NÃO VALIDADO** e manter a PR em Draft.
