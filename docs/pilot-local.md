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

## Desempenho e recuperação de respostas

O endpoint local usa saída estruturada por JSON Schema, temperatura 0,35 e limite de 512 tokens por decisão. Os identificadores possíveis de alvo e evidência vêm do contexto do personagem. A validação do motor permanece obrigatória. Respostas incompletas, JSON inválido ou decisões semanticamente rejeitadas recebem até duas novas tentativas somente para o ator em questão. Falha de conexão interrompe a rodada. A interface exibe ator, tentativa, tempo e total concluído, e permite cancelar a rodada corrente. Decisões válidas anteriores ficam em memória durante a sessão, inclusive após cancelamento ou falha; não são eventos publicados nem sobrevivem ao encerramento do processo. A próxima tentativa mantém a mesma visão inicial. A publicação continua atômica. O timeout de cada chamada local é de quatro minutos; a velocidade depende do modelo e do hardware.

## Escolha de ação e prévia das decisões

O modelo local seleciona um ID de ação previamente oferecido pelo motor (esperar, observar, mover para local permitido ou conversar com pessoa presente). O alvo é derivado desse ID. Campos cognitivos opcionais inválidos são descartados individualmente e registrados para o operador, sem invalidar ação legítima. Na tentativa de reparo, o esquema exige somente a escolha; a fala é exigida pelo motor se a ação for conversar. A intenção é opcional, e não é inventada quando ausente. Se três respostas inválidas persistirem, o personagem não executa ação nessa rodada; o registro identifica falha técnica, não decisão do personagem, e a memória não recebe esse evento. Se nenhum personagem decidir, a rodada não avança; falhas de conexão continuam interrompendo. A prévia mostra decisões validadas assim que disponíveis, separadas dos eventos já executados; não altera o contexto dos outros agentes.

## Piloto v2: acontecimentos, intenção e provedor gratuito

Novos saves usam a versão 2; saves v1 continuam com o relógio de cinco minutos e replay original. Use outro arquivo para experimentar a nova manhã. Ações relevantes avançam 1 minuto (conversa/observação nova) ou 2 minutos (movimento), sem ultrapassar o próximo anúncio escolar; espera e observação repetida saltam ao próximo acontecimento. A manhã autoral encerra às 09:00. Não é calendário canônico nem scheduler individual completo: a decisão permanece simultânea por coorte. O modelo recebe a ação anterior e a intenção ficcional, e observação de lista de presença já conhecida deixa de ser oferecida; esperar é sempre possível. Pensamentos/intenções do personagem são conteúdo ficcional do modelo, não fatos. Texto em ação não verbal pode ser nota privada não verificada, sem efeitos no mundo. O painel separa memórias de acontecimentos e diagnóstico de formato.

Provedor remoto opcional: mantenha a conta Groq no plano gratuito e crie uma chave em https://console.groq.com/keys. Consulte https://console.groq.com/docs/rate-limits para os limites atuais. No shell: `read -rsp 'Groq API key: ' GROQ_API_KEY; echo; export GROQ_API_KEY`; depois `go run ./cmd/cote-local --provider groq --model qwen/qwen3.8-27b --save "$HOME/cote-groq-v2.json"`. Não habilite cobrança para testar gratuitamente. O endpoint recebe contexto individual do personagem; a chave não é persistida. HTTP 429 interrompe e preserva escolhas anteriores em memória. A compatibilidade real do provedor depende de uma chave válida e modelo disponível. Para Ollama: `go run ./cmd/cote-local --provider ollama --model qwen2.5:3b --save "$HOME/cote-local-v2.json"`.
