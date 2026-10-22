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

Em **Actions → Bounded agent implementation fleet → Run workflow**, fornecer `issue_1` e, se realmente independentes, `issue_2`/`issue_3`.

- Cada Issue aberta e única recebe seu próprio runner/worktree e branch de checkpoint `agent/fleet/issue-N`. Até **2 agentes simultâneos**; terceiro aguarda. O workflow verifica números e estado da Issue; **não** comprova ausência de conflito de arquivos entre Issues. Avaliar paths/contratos/migrações antes de disparar em paralelo.
- Modelo `openai/gpt-oss-20b` por padrão, opção `openai/gpt-oss-120b`; usa OpenCode `1.19.3` e `GROQ_API_KEY`. O modelo precisa estar habilitado na conta e respeitar TPM/RPM/RPD compartilhados; `max-parallel=2` não garante cota para dois trabalhos. Se a quota falhar, baixar para uma única Issue por execução ou reduzir contexto/segmentos.
- Executa 1 ou 2 sessões explícitas de **6 minutos** por Issue, com checkpoint no final de cada sessão (inclusive falha comum/timeout), limitado a allowlist `internal src tests evals cmd`. Não persiste arquivos fora dessa allowlist nem pode salvar o último intervalo em um desligamento abrupto do runner.
- A branch pode ser retomada em nova execução da mesma Issue. Checkpoint WIP não significa CI green ou issue concluída. Um job de agente com erro permanece vermelho.
- Neste repo, checkpoint cria automaticamente Draft PR? **Não. A revisão independente posterior é dona dessa transição.**
- O token de GitHub é entregue apenas às etapas confiáveis de preparação/publicação; o worker recebe a chave de inferência. Ainda assim, permissões do agente e runners descartáveis não substituem um isolamento de segurança forte. Não utilizar Issues ou fontes de terceiros não confiáveis nem dados privados nas instruções.

## 4. Revisão headless (somente candidatos não verificados)

Em **Actions → Agent headless PR review → Run workflow**, informar a `pr_number`, a `issue_number` aceita e o modelo.

1. O workflow extrai Issue e **diff completo** de PR aberta contra seu HEAD exato, sem checkout/execução do código da PR.
2. **Falha fechada** quando diff excede 4.000 caracteres ou Issue excede 12.000; não trunca fingindo revisão completa.
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
- Testar retomada da Issue após falha simulada; verificar que só paths allowlisted foram persistidos.
- Rever segurança/custo/cotas e logs antes de ampliar o número de agentes.

Se o runner nem iniciar ou a cota estiver esgotada, classificar como **NÃO VALIDADO** e manter a PR em Draft.
