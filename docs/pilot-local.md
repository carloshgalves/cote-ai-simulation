# Piloto cognitivo local

## Rodar

Linux, toolchain Go do repositório, um endpoint Chat Completions compatível com JSON object. Não é preciso Python A1, Docker, banco externo, conta de aplicação ou servidor remoto. Ollama é uma opção local: instale e selecione um modelo já disponível no seu computador. Por exemplo, com `qwen2.5:7b` disponível:

```sh
export COTE_MODEL=qwen2.5:7b
go run ./cmd/cote-local --save "$HOME/cote-pilot.json"
```

Abra http://127.0.0.1:8765. `--steps 1` executa uma rodada no terminal. O modelo e o endpoint são configuráveis; `COTE_LLM_URL` aceita o base URL incluindo /v1, `COTE_API_KEY` é opcional e nunca salvo. Se usar um provedor HTTPS externo, o contexto de cada personagem será enviado a ele. O journal privado é criado com modo 0600, retomado ao reabrir e não sobrescrito por outra instância cooperante. Não publique saves. Para uma nova simulação, indique um arquivo diferente.

## O que funciona

Seis perfis provisórios derivados dos packs existentes, sem importar acontecimentos futuros como lembranças. A lista inicial de pessoas/classes é conhecida por todos neste cenário de orientação autoral. Cada rodada usa a mesma visão inicial de mundo e chama o LLM uma vez por personagem, sequencialmente para não sobrecarregar um modelo local. A resposta JSON propõe ação (esperar, observar, conversar em privado com alguém presente, mudar de local), intenção breve, hipóteses com evidência, revisão de meta/plano e relação. O motor valida todas as propostas antes de publicar qualquer efeito. Falas são alegações; não criam fatos, pontos ou resultados por texto. Movimentos têm observações separadas de saída/chegada, não telepatia de destino. Cada nova rodada recebe apenas as memórias endereçadas ao agente, as metas, crenças e relações próprias. O relógio lógico existente avança cinco minutos por rodada. A interface exibe o tempo, atores, eventos, metas, planos, crenças e memória filtrada por perspectiva; operador pode ver tudo. Run/pause/step; pausa após a rodada corrente; falha do provedor interrompe a execução sem publicar uma rodada parcial.

Apenas decisões válidas são salvas. O journal contém as respostas aceitas, não a chave do provedor nem prompts completos. Replay reconstrói o estado sem consultar o modelo. Mesma sequência de decisões aceitas produz o mesmo resultado; novas chamadas estocásticas do modelo podem divergir. Até 200 rodadas por save neste piloto.

## Limites honestos

Este é um módulo de produto Go **experimental**, não uma alegação de que o CSF canônico já tem handlers gerais para conversa, movimento e cognição de seis atores. Ele reutiliza o relógio CSF, mas seu journal não é um ledger CSF nem uma segunda autoridade sobre o mesmo RunID. Não importa os bancos H2, não os modifica e não concede novos write ports. A próxima integração deve substituir a publicação do piloto por handlers CSF conforme as necessidades observadas. Sem motor de exame, economia, fisiologia integrada, calendário completo, RAG V2 verificado, avaliações de fidelidade ou prevenção garantida de spoilers oriundos do conhecimento pré-treinado do LLM. Os seis perfis são adaptações experimentais, não evidências canônicas homologadas. O modelo pode alucinar e o motor não certifica semanticamente cada frase. O operador deve avaliar isso na experiência real.

O teste de integração HTTP usa um provedor de teste controlado, não afirma ter consultado um modelo real. Testes verificam rodada com seis chamadas, privacidade, mudança de objetivo e plano, persistência/replay, recusa de ação inválida e indisponibilidade do provedor. O primeiro teste com o LLM escolhido pelo operador é manual. Não exigir 109 provas normativas para experimentar o piloto.
