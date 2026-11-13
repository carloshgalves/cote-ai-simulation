# B3 — owner cognitivo: matriz de falhas de processo, fork e concorrência

**Ticket:** [#78](https://github.com/carloshgalves/cote-ai-simulation/issues/78) · **PR:** [#82](https://github.com/carloshgalves/cote-ai-simulation/pull/82) · **Premissa:** COTE AI Simulation pessoal, local, um operador. B1 e B2 estão na base; esta etapa não introduz LLM real nem servidor.

## Contrato observado e critérios

A matriz usa dois bancos SQLite distintos: **owner** (checkpoint privado e ACK) e **CSF** (escrow assinado, snapshot selado, clock e cinco ledgers). Um binário filho `go test` acessa os bancos e chama `os.Exit(86)` **sem fechar as conexões** no instante-alvo. O processo-pai exige exatamente o código 86, nenhum output, reabre os arquivos sob novos handles e não aceita WORK antes de um Restore/ACK exato.

| Crash do processo filho | Owner checkpoint | Escrow CSF | Snapshot selado | ACK owner | Recuperação |
| --- | ---: | ---: | ---: | ---: | --- |
| Antes de persist owner | 0 | 0 | 0 | 0 | Primeira publicação íntegra |
| Depois de owner persist/assinatura | 1 | 0 | 0 | 0 | Reuso idempotente do owner, não autoriza snapshot sem escrow |
| Depois do escrow assinado | 1 | 1 | 0 | 0 | Nova publicação completa; escrow órfão não autoriza WORK |
| Depois do snapshot selado | 1 | 1 | 1 | 0 | Novo processo restaura do owner, exige ACK |
| Depois de Restore, antes de ACK | 1 | 1 | 1 | 0 | Restore novamente e ACK sob nova sessão |
| Depois de ACK durável, antes do permit | 1 | 1 | 1 | 1 | ACK histórico não concede permit a um novo Gate |

Após reabrir, todos os casos devem produzir **uma única** linha de checkpoint, receipt, selo de snapshot e ACK; o mesmo `run/actor/coverage` não pode produzir bytes diferentes. O snapshot e o receipt são comparados por identidade derivada do **contrato canônico**, não por output do subprocesso. A assinatura Ed25519 e o escrow são revalidados pelo CSF. Controles negativos com outro ator e outro run não restauram memória. O callback sintético só executa depois da recuperação exata.

Antes e depois de cada crash e retry, as cinco tabelas causais (`input_ledger`, `decision_ledger`, `event_store`, `evidence_ledger`, `causal_outbox`) são comparadas por SHA-256 do conteúdo **canônico** ordenado e emoldurado por comprimento, sem registrar bytes privados. Isso verifica que persistir cognição não modifica world truth.

## Fork V1 → V2

São dois cenários separados: (1) fork real com owner assinado do pai e reatestação da filha, incluindo colisão de identidade, cross-run e cross-actor; (2) **processo filho encerra depois do commit de bootstrap V2**, antes de o chamador receber Genesis. Um novo processo reabre a CSF filha e verifica `RecoverPublishedForkV1ToV2`, replay e prova do owner histórico. O recibo pai não autoriza o WORK da filha; a filha tem de produzir novo recibo ligado ao novo run e completar Restore/ACK. Os cinco ledgers do pai não podem mudar por causa do fork; a reatestação da filha não altera os ledgers do filho.

## Concorrência e timeouts

Barreiras por canal sem sleeps forçam as ordens Stage→WORK, Restore bloqueado→Stage, ACK durável bloqueado→Stage. O Gate revoga permisos antes de qualquer operação potencialmente falha; uma geração antiga não autoriza callback. Esperas de barreira/retorno são limitadas por timeout focal e resultam em erro identificado, não no timeout geral do workflow.

## O que **não** está provado

- O crash é um **`os.Exit(86)` de processo filho**, não corte de energia, crash de kernel, falha física ou perda de dados do dispositivo.
- O intervalo dentro da função B1 entre `COMMIT` e `ed25519.Sign` não tem failpoint próprio; os testes cobrem os estados imediatamente **antes** da persistência e após o método `PersistAndSign` retornar. Isso não deve ser rotulado como crash precisamente entre commit e assinatura.
- Não há teste de `exactly-once` de LLM nem de serviços remotos. `Gate.RunWork` revalida a autoridade local antes do callback; o projeto ainda não tem host produtivo de modelo.
- A revisão feita pelo mesmo implementador não é revisão independente. B4 (#79) mantém a auditoria integrada e o gate humano.

## Como reproduzir

```sh
gofmt -l internal/csf/commit/owner58_b3_*_test.go
go test -count=1 -v -timeout=12m ./internal/csf/commit -run '^TestCSFRV18R58B3'
go test -race -count=1 -timeout=14m ./internal/csf/commit -run '^TestCSFRV18R58B3'
go test ./...
```

Registre resultados por commit no GitHub Actions; um workflow em andamento, cancelado ou sem runner **não equivale a PASS**. A PR é Draft e não concede merge/release automático.
