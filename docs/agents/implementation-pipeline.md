# Pipeline de implementação, revisão e entrega com agentes

> Protocolo operacional proposto a partir do relatório *Claude Code Insights* de 22/10/2026. Complementa — não substitui — `AGENTS.md`, `CONTEXT.md`, skills, specs e ADRs aceitos. Não instala um agente, gatilho ou execução em background por si só.

## 0. Intake e contrato de aceite

1. Ler Issue e comentários, specs, ADRs aceitos, `AGENTS.md` e contexto roteado. Código/HEAD vigente é a verdade da implementação; Issue define o aceite, não a conversa.
2. Classificar: bug, slice clara, decisão de domínio pendente, trabalho exploratório, release. Não escrever código enquanto requisito essencial estiver sem decisão.
3. Fazer mapa de impacto: `arquivo/módulo -> responsabilidade -> seam público -> mudança -> teste -> risco` e listar dependências e casos negativos.
4. Dividir ticket grande em slices verticais, cada uma com entrada, critério observável, owner, dependência e fora de escopo. Nunca tratar ausência de teste como aceite.

## 1. Implementação (uma Issue, um branch/worktree)

1. Criar branch dedicada a partir do base correto; checar HEAD e working tree; escolher paths exclusivos para trabalho paralelo.
2. Registrar a reprodução/verificação **RED que falha pela razão certa** quando aplicável. Em bugs, demonstrar defeito antes do patch; em mudança sem seam estável, anotar limitação sem inventar RED.
3. Implementar a menor slice ponta a ponta; executar teste focal imediatamente após **cada** alteração de comportamento, não somente no final.
4. Executar checks por camada afetada; finalizar com suite relevante completa, build/evals quando cabíveis, `git diff --check`, revisão de arquivos alterados e documentação sincronizada.
5. Commit e push de checkpoint verificável. Persistir Issue/branch/HEAD, comandos, códigos de saída, resultados, riscos, trabalho não concluído e próximo passo em PR/handoff. Falha de cota/desconexão não pode apagar trabalho local.

## 2. Revisão independente, remediação e re-review

1. Revisão só sobre checkpoint persistido, com spec e base fixados. Identificar falhas **com evidência**, severidade, requisito, location, reprodução e menor correção segura; publicar no formato de finding já aceito pelo projeto.
2. Agente de remediação valida cada finding OPEN do lote existente; para defect válido, escrever regressão RED, corrigir, rodar teste focal e suite, commitar e atualizar status com SHA. Evidência insuficiente -> DISMISSED com motivo; escopo futuro -> DEFERRED com issue/decisão.
3. Re-review independente comprova cada resolução, olha regressões do delta, compara HEAD e verifica status persistidos. Zero OPEN **só** se confirmado, nunca por inferência.
4. Um teto sugerido de quatro rodadas serve como limite de esforço, **não** como licença para invocar a próxima fase. Se não convergir, relatar NO-GO e bloqueios; mudança de API, contrato, ADR, escopo ou risco crítico requer decisão explícita.

## 3. Regressão adversarial e restauração segura

- Selecionar controles negativos/inversões de alto risco, um por vez; registrar `invariante -> mutante -> teste que deveria falhar -> resultado`.
- **Nunca** aplicar mutante em alterações não commitadas nem usar `git checkout --` ou `stash pop` como limpeza automática: essas práticas podem destruir trabalho do usuário.
- Executar mutantes em worktree/cópia temporária isolada, com status de base conhecido e limpeza exclusivamente do recurso criado pelo teste (try/finally + trap quando aplicável). Se o processo cair, a branch de trabalho original deve permanecer intacta.
- Mutante sobrevivente significa contrato não provado e exige novo teste/finding. Informar taxa de detecção apenas sobre mutantes realmente executados.

## 4. Paralelização controlada

- Paralelizar apenas Issues **ready** independentes após analisar a sobreposição de arquivos, migrações, contratos, schemas, fixtures compartilhadas, versão e invariantes.
- Cada agente tem branch/worktree próprio, Issue única, suite focal e Draft PR/handoff conforme as skills do repositório. Não editar simultaneamente `AGENTS.md`, `CLAUDE.md`, lockfiles, migrações, specs ou APIs compartilhadas.
- Publicar matriz de `Issue -> worktree -> PR/branch -> HEAD -> RED -> testes finais -> paths -> risco de integração`. Merge continua decisão humana e pode exigir rebase e revalidação da pilha.

## 5. Armadilhas estruturais e verificabilidade

- Comentário longo do `gh`: produzir arquivo UTF-8; usar `gh pr comment <N> --body-file <arquivo>`/`-F` (não `-f`). Verificar por leitura de volta; antes de editar comentário existente, salvar corpo e ID originais.
- Evitar heredocs frágeis, substituição de shell em Markdown e misturar caminhos Git Bash/Windows. Preferir editor/API ou arquivo temporário validado; não introduzir diretórios que sombreiem bibliotecas instaladas.
- Hooks opcionais devem começar **somente com validação não mutante** (parser/lint/check de arquivo focal); não rodar `ruff --fix` automático em cada edit e não adicionar Stop hook que esconda exit code ou rode suite custosa/infinita.
- Não chamar 'testado' o que não foi executado: usar `PASSOU`, `FALHOU`, `NÃO EXECUTADO`, `MANUAL PENDENTE`. Evidência de GUI/instalador/serviço externo deve ser distinguida da prova automatizada de adapter.

## 6. Gate final e entrega

- Checkpoint commitado, evidência das suites e mutantes essenciais, critérios da Issue rastreados, review final independente sem finding acionável OPEN, documentação e riscos operacionais registrados.
- CI verde é necessário quando requerido, mas não concede release nem merge. Se qualquer gate exigido estiver ausente, declarar `NO-GO`, incluindo quem/qual ambiente deve confirmar.

### Regras específicas: COTE AI Simulation

- Engine é autoridade de world truth, eventos, scoring, replay e causalidade; belief/perception/knowledge não podem revelar estado secreto.
- Reproduzir runs por seed, substreams, versão/config e evidências; validar fronteiras temporais do cânone.
- Testar regras determinísticas por testes comuns e comportamento dependente de modelos com evals versionados (LLM/RAG/simulação).
- Não versionar corpus privado/protegido; jamais suavizar invariantes determinísticas para fazer um prompt passar.

### Limite entre fases deste repositório

Fronteira contratual **estrita**: `implement`, `code-review` e `address-review-findings` são invocações distintas, cada uma termina com STOP. O coordenador NÃO invoca a próxima fase nem delega uma transição via subagente. Cada fase posterior exige nova solicitação do usuário. A Draft PR nasce em `code-review`, não em `implement`.

### Skills especializadas

`.agents/skills/implement`, `code-review`, `address-review-findings`, `llm-evals`, `rag-evals`, `simulation-evals`. Não substituir os comentários `<!-- cote-review-finding:v1 -->` nem automatizar a revisão independente a partir do agente de implementação. Revisão olha o diff completo; remediação consome lote congelado de OPENs.

### Checkpoint/handoff mínimo

```text
Projeto/Issue:
Branch / base / HEAD / PR:
Fase e skill executada:
Spec / ADRs aplicados:
RED (caso/saída) ou justificativa de N/A:
Testes focais / suite final / exit codes:
Mutantes e controles negativos (se cabíveis):
Findings OPEN / RESOLVED / DISMISSED / DEFERRED:
Artefato/browser/ambiente MANUAL PENDENTE:
Riscos, limitações, efeitos externos não autorizados:
Próxima fase permitida e autorização exigida:
```

**Atenção:** este documento não declara que o pipeline está automatizado em CI ou que os testes foram executados; descreve o processo que cada execução deve demonstrar.
