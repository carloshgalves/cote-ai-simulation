# Y1 V7 ↔ Volume 0 — Reconciliação de Atsuomi e do passado de Kiyotaka

**Status:** candidata para revisão externa; sem liberação para memória do agente, engine ou merge.  
**Data:** 2026-10-26.  
**Matriz legível por máquina:** `data/canon/consolidated/audits/y1-v07-v00-reconciliation.json`.  
**Apuração completa do V7:** `docs/canon/reports/Relatorio_COTE_Y1_V7.md` e `data/canon/consolidated/audits/y1-v07-extraction-candidates.json`.

## Base documental

- **Volume 0:** LegacyEMTLs v1.0.1, 265 páginas físicas, SHA-256 `a5f3360c145ed9aa038bf7965b1d261e8b733c943b0c7fb8ed976705e3e0f7d3`, aceita pelo operador. O registro `data/canon/consolidated/sources/volume0.json` não concede verificação automática dos 132 claims.
- **Ano 1 Volume 7:** Seven Seas inglês, 207 páginas físicas, SHA-256 `b41f00143f483fa7704bcc6c4bfda03323bd835c77ea1e95e53d27bd859e461b`, fornecido pelo operador. A fonte integral **não** é adicionada ao Git.
- **Política:** ADR 0005, ADR 0012, `docs/canon/conflict-policy.md` e preservação de quatro eixos temporais.

## Regra central

**Não substituir o passado mostrado no Volume 0 pela explicação apresentada no Volume 7 e não atribuir intenção onisciente aos participantes.** A fala de personagem é evidência de fala e, quando necessário, da crença do falante, não comprovação de todos os acontecimentos relatados.

O V0 descreve com mais detalhe a política, a criação e o financiamento da White Room, a imposição do protocolo Beta à quarta geração, a visita dos Sakayanagi, a interrupção associada a pressões políticas e os planos finais opacos de Atsuomi. O V7 acrescenta o encontro com o filho, a figura de Matsuo, a motivação declarada por Kiyotaka para buscar liberdade e a autoridade da mantenedora da ANHS.

## Pontos de conciliação importantes

### 1. Atsuomi e a fuga: hipótese, não revelação certificada

- **V0, pp. 259–261:** Atsuomi pondera usar Kiyotaka na política e encomenda providências futuras a Tsukishiro; a conversa não revela ao leitor os termos integrais do plano. Fonte V0: `v0.pol.021`, `v0.pol.022`.
- **V7, pp. 59–64:** Atsuomi afirma que Kiyotaka o desobedeceu; Kiyotaka sustenta que Matsuo lhe indicou a ANHS, recusa voltar e infere limites da influência paterna. Candidatos: `V7-E027`, `V7-E029`, `V7-E032`.
- **Conclusão permitida:** o V0 cria uma questão aberta sobre o alcance do planejamento paterno, **sem provar** que ele encenou a fuga, ordenou Matsuo a agir ou organizou a matrícula. A oposição ao filho no V7 tampouco prova surpresa genuína do pai.
- **Finding:** `V00V07-OPEN-01` — `UNRESOLVED`.

### 2. Suspensão e intervalo com Matsuo

- **V0, pp. 202–213 e 242–260:** o programa sofre pressão política e é temporariamente suspenso; Atsuomi prepara novas ações. Referências: `v0.pol.015`, `v0.pol.016`, `v0.wr.031`.
- **V7, pp. 59–63:** Atsuomi diz ter confiado Kiyotaka a Matsuo por cerca de um ano e lamenta a interrupção da White Room. Essa versão **complementa** a trajetória, mas não elimina as causas políticas evidenciadas no V0.
- O V7 relata, por boca de Atsuomi, perseguição a Matsuo e ao filho e morte por suicídio. Kiyotaka expressamente questiona a falta de prova direta. O falante oferece a possibilidade de documentos, mas os documentos não são examinados na passagem. Status `CHARACTER_CLAIM_NOT_VERIFIED`.
- **Findings:** `V00V07-OPEN-02` e `V00V07-OPEN-03`.

### 3. Formação, capacidades e limites

O protocolo Beta da quarta geração tem suporte no V0 (pp. 125–128); o V7 fornece autodescrição das disciplinas e autodefesa (p. 62), junto com avaliação do pai sobre as aptidões do filho (p. 65). Não é licenciado converter essas falas em QI, velocidade, invulnerabilidade, emoções ausentes permanentes ou desempenho garantido em qualquer cenário. Retrospectivamente, o V0 documenta limitações de adaptação externa e custos humanos do currículo.

### 4. Atsuomi: projeto político não desaparece

O V0 descreve a ambição política e possíveis usos de Kiyotaka (`v0.pol.020`–`022`). O V7 mostra exigência de retorno à White Room; **objetivos podem coexistir**. Evitar simplificar o pai como protetor altruísta, vilão onipotente ou estrategista cuja hipótese é necessariamente a verdade da simulação.

### 5. Sakayanagi, Arisu e segredo da White Room

A trajetória do pai de Arisu é a de intermediário/observador no V0 (pp. 54–56, 105–111) e presidente da mantenedora no V7 (pp. 65–70). A mudança de papel possui tempos distintos, não autoriza retrodatação de cargo ou jurisdição.

A visita de Arisu à White Room no V0 (pp. 172–176) explica plausivelmente sua familiaridade privada no confronto do V5. **Não** atribuir esse conhecimento a Suzune, Ryūen, estudantes da classe A em geral ou funcionários sem aquisição demonstrada.

O presidente Sakayanagi relata a admissão excepcional e os 50 pontos deliberados de Kiyotaka (V7 p. 69), mas limita a retirada contra a vontade do aluno pelas regras escolares (pp. 67–70). Essa autoridade é **situacional**, não uma regra geral que permite vender ou substituir todas as normas acadêmicas.

## Preservar a ordem temporal

1. **Tempo válido:** eventos políticos e White Room do V0 são antecedentes; confronto escolar do V7 ocorre no primeiro ano, em dezembro.
2. **Tempo da revelação:** os locators continuam ligados às páginas de cada fonte.
3. **Tempo de conhecimento:** Kiyotaka pode conhecer sua própria educação antes da escola, mas não adquire automaticamente o plano privado de Atsuomi ou a informação sobre Matsuo antes da conversa; Arisu possui apenas o que a visita e as demais aquisições demonstram.
4. **Tempo transacional:** estes novos registros são uma proposta em branch; a integração futura exige revisão e autorização.

## Gates

- V7: **113 evidências + 44 casos + 18 mudanças de objetivo = 175 registros candidatos**. Existem ainda 14 dossiês de reconciliação no relatório original e 32 controles adversariais **propostos**.
- Matriz adicional V0↔V7: **10 relações**, com três achados `UNRESOLVED`.
- A correspondência bibliográfica e de páginas foi verificada pelo assistente nas edições fornecidas; isso **não equivale** a segundo avaliador independente de cada item.
- Não registrar aprovação externa inexistente, alterar `verified:false`, promover para `engine_memory_admission:true` nem acionar atualização executável de metas.
- Esta branch não reescreve `v0.pol.021` nem os arquivos canônicos anteriores; somente anexa dados de apuração.

**Próxima transição de estado:** revisão semântica individual e adversarial independente dos 175 itens do V7, deliberação sobre 10 vínculos V0↔V7 e só então atualização dos packs globais, eventos retrospectivos e índices bidirecionais conforme ADR 0012.
