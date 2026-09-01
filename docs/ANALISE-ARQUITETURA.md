# Análise de Arquitetura — ToggleMaster (Fase 1)

Este documento cobre os dois primeiros requisitos técnicos da Fase 1 ("Cultura DevOps e Arquitetura de Aplicações"):

1. Por que a aplicação é considerada um monolito, e vantagens/desvantagens dessa abordagem para um MVP.
2. Análise dos 12 Fatores (12-Factor App) aplicada ao código atual.

---

## 1. Por que `toggle-master-monolith` é um monolito

Um monolito é uma **única unidade implantável** que empacota a camada de API, a lógica de negócio e o acesso a dados juntos, rodando como um processo único contra um único banco de dados. Todos esses sinais estão presentes no código:

- **Um processo, um arquivo, todas as camadas.** `app.py` define roteamento HTTP (`@app.route`), regras de negócio (validação, checagem de conflito) *e* acesso a dados via SQL cru (`app.py:64-65`, `app.py:83-84`, `app.py:126`) — sem camada de serviço, sem abstração de repositório, sem separação de responsabilidades entre camadas.
- **Um banco de dados, acoplado diretamente.** `get_db_connection()` (`app.py:14`) abre uma conexão `psycopg2` direto do handler da rota para uma única instância Postgres. Não existe fronteira de acesso a dados por trás da qual outra coisa poderia se esconder (um repositório, uma camada ORM, um serviço de dados separado).
- **Um artefato de build.** O `Dockerfile` empacota tudo — código da app + dependências + entrypoint — em uma única imagem. Existe exatamente uma coisa para buildar, versionar e implantar.
- **Uma unidade de runtime que escala como um todo.** `entrypoint.sh` inicia um único processo `gunicorn` servindo *todas* as rotas (`/health` e as quatro rotas de `/flags`). Para escalar, rodam-se mais cópias da app *inteira* — não é possível escalar "leitura de flags" independentemente de "escrita de flags".
- **`docker-compose.yaml` confirma o formato:** exatamente dois serviços — `app` (o monolito) e `db`. Sem API gateway, sem message broker, sem workers independentes, sem serviços separados por capacidade.

Essa é a definição clássica: um único codebase → um único build → um único deploy → uma única unidade de escala, com todas as camadas acopladas/compiladas juntas.

### Vantagens do monolito para um MVP

1. **Velocidade de entrega.** Sem contratos de API entre serviços para desenhar, sem o custo de sistemas distribuídos (chamadas de rede, retries, service discovery). Um codebase para escrever, um artefato para implantar.
2. **Desenvolvimento local trivial.** `docker compose up` sobe o sistema inteiro — app + banco — de uma vez. Não é preciso orquestrar N serviços.
3. **Operação simples para um time pequeno.** Uma coisa para monitorar, logar e colocar atrás de um load balancer. Menos peças móveis = menos modos de falha nesta fase.
4. **Depuração fácil.** Um bug vive em um único processo, com um único stack trace — não é preciso tracing distribuído para descobrir qual serviço se comportou mal.
5. **Infraestrutura barata.** Uma instância EC2 + um RDS já bastam (exatamente o que esta fase pede para provisionar) — sem service mesh, sem filas, sem múltiplos load balancers.
6. **Barato para refatorar enquanto os requisitos ainda são instáveis.** Mudar o modelo de domínio (ex.: adicionar um campo `description` às flags) toca um único codebase — sem coordenar mudanças quebradoras entre serviços versionados independentemente.

### Desvantagens (e por que o desafio prevê evoluir disso)

1. **Sem escala independente.** A *avaliação* de flags (provavelmente alto volume de leitura, sensível a latência, chamada por todo cliente a cada requisição) e a *gestão* de flags (CRUD administrativo, baixo volume) compartilham os mesmos workers do gunicorn. Não dá pra escalar uma sem escalar a outra.
2. **Ponto único de falha / blast radius total.** Uma rota que quebra pode derrubar até o `/health`. Todo deploy — mesmo um fix de uma linha em `PUT /flags/<name>` — arrisca todos os endpoints, não só o que mudou.
3. **Sem connection pooling.** `get_db_connection()` abre (e fecha) uma conexão Postgres nova a cada requisição (`app.py:63`, `82`, `99`, `124`). Tranquilo no volume de tráfego de um MVP; não aguenta carga real — é o tipo de coisa que "pronto para produção" precisaria corrigir.
4. **Camada de dados acoplada à lógica de negócio.** SQL cru está inline nos handlers de rota — sem fronteira de repositório/serviço para trocar o banco, adicionar cache, ou introduzir uma réplica de leitura sem tocar no código de tratamento de requisições.
5. **Lock-in tecnológico.** Tudo é Python/Flask/gunicorn. Uma capacidade que se beneficiaria de outra stack (ex.: um serviço de avaliação de flags de baixa latência) não pode ser introduzida sem reescrever a app inteira.
6. **Atrito ao escalar o time.** Mais devs = todo mundo no mesmo repositório, mesmo trem de release, sem peças implantáveis/possuídas de forma independente. É a razão organizacional clássica pela qual empresas quebram monolitos conforme crescem.
7. **Fronteira de segurança grosseira.** Um processo, um único conjunto de credenciais de banco, lida tanto com escritas administrativas confiáveis quanto (eventualmente) com tráfego de leitura não confiável — sem separação de privilégio mínimo entre os dois.

> **Nota para o relatório:** essas desvantagens não são acidentes — são exatamente a motivação do roadmap do próprio desafio ("Começaremos com um MVP monolítico e, gradualmente, o transformaremos em um sistema distribuído, resiliente e altamente automatizado"). A Fase 1 é deliberadamente monolítica *porque* é um MVP sendo validado para obter feedback rápido; as Fases 2-4 devem endereçar exatamente essas lacunas de escala/acoplamento/resiliência.

---

## 2. Análise dos 12 Fatores (12-Factor App)

| # | Fator | Status | Resumo |
|---|-------|--------|--------|
| I | Codebase | ✅ | Um repositório, um codebase, implantado em múltiplos ambientes |
| II | Dependencies | ✅ | Bem resolvido, com uma pequena inconsistência |
| III | Config | ⚠️ | Credenciais de banco via env — bem; porta da app e do banco, não |
| IV | Backing services | ✅ | Ponto forte — o banco troca de forma limpa via variáveis de ambiente |
| V | Build, release, run | ⚠️/❌ | OK no caminho Docker, quebrado no caminho manual via EC2 |
| VI | Processes | ✅ | Totalmente stateless |
| VII | Port binding | ⚠️ | Auto-contido (gunicorn), mas a porta está fixa no código |
| VIII | Concurrency | ⚠️ | O modelo suporta, mas nada está configurado para usar |
| IX | Disposability | ⚠️ | Boa robustez na inicialização, história fraca de desligamento no EC2 |
| X | Dev/prod parity | ❌ | Maior lacuna — dev roda em Docker, "prod" roda em venv puro |
| XI | Logs | ⚠️ | Stream para stdout corretamente, nada captura isso em produção |
| XII | Admin processes | ✅ | `flask init-db` é um comando one-off correto via Flask CLI |

### Já atendidos

**I. Codebase — ✅**
Um único repositório git (`dougls/toggle-master-monolith`), sem repositórios separados por componente. O mesmo codebase é implantado duas vezes — localmente via `docker compose`, em "produção" no EC2 — exatamente o modelo de um-codebase/muitos-deploys.

**IV. Backing services — ✅ (destaque positivo para o relatório)**
O Postgres é endereçado inteiramente através de `DB_HOST`/`DB_NAME`/`DB_USER`/`DB_PASSWORD` (`app.py:9-12`) — a aplicação não tem conhecimento algum se está falando com o container `db` local ou com uma instância RDS. Trocar um pelo outro é uma mudança de configuração, não de código. Esse é o padrão *correto* e vale a pena destacar como ponto positivo.

**VI. Processes — ✅**
Sem estado de sessão, sem cache em memória, nada é mantido entre requisições — toda leitura/escrita vai direto ao Postgres (`app.py:64-140`). Totalmente stateless, então seria possível rodar N cópias idênticas sem mudança de código.

**XII. Admin processes — ✅**
`init_db()` é exposto como `flask init-db` via `@app.cli.command` (`app.py:45-47`), executado no mesmo codebase/ambiente da aplicação, em vez de um script ad-hoc separado — exatamente o que o fator XII pede.

### Parcialmente atendidos — vale destacar explicitamente no relatório

**II. Dependencies — quase ✅, uma inconsistência**
`requirements.txt` fixa versões exatas e a imagem Docker isola essas dependências de forma limpa. A lacuna: o pacote de nível de sistema (`postgresql-client`, `Dockerfile:18`) também é declarado explicitamente — bom — mas o caminho manual de deploy no EC2 (README) reinstala tudo em um `venv` puro no host em vez de usar a imagem Docker, então "como as dependências são isoladas" difere entre os dois caminhos de deploy.

**III. Config — ⚠️**
Credenciais de banco feitas corretamente (via env). Mas **a porta está fixa em quatro lugares** — `app.py:143`, `Dockerfile:20` (`EXPOSE 5000`), `Dockerfile:23`/`entrypoint.sh:37` (`--bind 0.0.0.0:5000`) — em vez de ler uma variável `PORT`. `DB_PORT` é exigida por `entrypoint.sh`, mas nunca é de fato consumida por `get_db_connection()` em `app.py:14-21`, que silenciosamente cai para a porta padrão 5432 do Postgres. Para produção: ler `PORT` e `DB_PORT` do ambiente em todos os lugares, e gerenciar segredos via AWS Secrets Manager / SSM Parameter Store em vez de `export` manual numa sessão SSH (o próprio README já sinaliza isso como inseguro/efêmero).

**V. Build, release, run — ⚠️ no Docker, ❌ no EC2 manual**
O caminho Docker separa build (`docker build`) de run (`entrypoint.sh` → gunicorn) razoavelmente bem. O deploy manual no EC2 descrito no README **não separa nada**: `git clone` → `pip install` → `gunicorn` direto, sem artefato versionado, nada para o qual seria possível fazer rollback. Para produção: buildar um artefato versionado (imagem Docker publicada no ECR, ou uma AMI) e implantar esse artefato imutável em vez de mutar um host ao vivo.

**VII. Port binding — ⚠️**
A aplicação *de fato* se exporta via seu próprio port binding (gunicorn, não injetado em um webserver externo) — essa é a forma correta do fator VII. Só não é configurável — ver Config acima.

**VIII. Concurrency — ⚠️**
O modelo de workers do gunicorn é inerentemente capaz de escalar horizontalmente, mas nada aqui usa isso: sem flag `--workers` (padrão é 1), e a arquitetura desta fase é uma única instância EC2 sem Auto Scaling Group/load balancer. Esperado para um MVP — vale registrar explicitamente como "suportado pelo modelo, ainda não exercitado", e um alvo natural para a Fase 2+.

**IX. Disposability — ⚠️**
A inicialização é robusta: `entrypoint.sh` aguarda o Postgres com `pg_isready` antes de subir a app (`entrypoint.sh:22-25`), então degrada graciosamente se o banco ainda não estiver pronto. O gunicorn também trata `SIGTERM` graciosamente por padrão. Mas as instruções do README para o EC2 rodam o gunicorn em **foreground de uma sessão SSH** — fechar essa sessão mata a aplicação (o próprio README admite isso). Para produção seria necessário um supervisor de processo (`systemd`, ou simplesmente rodar a mesma imagem Docker) para que o processo reinicie em caso de crash e trate sinais como um serviço gerenciado.

**XI. Logs — ⚠️**
Tanto o gunicorn quanto o Flask logam para stdout/stderr em vez de escrever/rotacionar seus próprios arquivos de log — essa é a postura correta do 12-factor (não gerenciar seu próprio roteamento de logs). A lacuna está mais adiante: nada captura esse stream de forma persistente depois que o container/sessão SSH termina. Em produção seria desejável o agente do CloudWatch Logs (ou similar) enviando o stdout para um destino durável.

### A falha real

**X. Dev/prod parity — ❌**
Esta é a lacuna mais evidente, e merece um parágrafo próprio no relatório: localmente roda-se uma aplicação **containerizada** contra um Postgres **containerizado** via `docker compose`. Em "produção" (segundo as próprias instruções desta fase) roda-se uma instalação **bare-metal via venv** direto no host EC2 contra uma instância **RDS gerenciada** — gerenciador de processo diferente, mecanismo de instalação de dependências diferente, superfície de SO totalmente diferente. O que foi testado localmente não é o que de fato vai para produção. A correção para um setup mais robusto: rodar a *mesma imagem Docker* no EC2 (ou migrar para ECS/Elastic Beanstalk, provavelmente o caminho de uma fase futura deste desafio) em vez de reinstalar a partir do código-fonte no host.

---

## 3. Estimativa de Custos (AWS Pricing Calculator)

Estimativa mensal para a arquitetura descrita acima — EC2 `t3.micro` (Linux, EBS gp3 8 GB) + RDS PostgreSQL `db.t3.micro` (Single-AZ, gp3 20 GB), ambos On-Demand, 100% de utilização/mês. Sem NAT Gateway, sem Multi-AZ, conforme as decisões de arquitetura da seção 2.

| Serviço | sa-east-1 (São Paulo) | us-east-1 (N. da Virgínia) |
|---|---|---|
| Amazon EC2 | $13.48/mês | $8.23/mês |
| Amazon RDS for PostgreSQL | $94.17/mês | $55.59/mês |
| **Total/mês** | **$107.65** | **$63.82** |
| **Total/12 meses** | **$1.291,80** | **$765,84** |

- Estimativa São Paulo: https://calculator.aws/#/estimate?id=64ccbc1e1057357bc1dc88a10a028a4d2f7cea00
- Estimativa N. da Virgínia: https://calculator.aws/#/estimate?id=587162758a89feffe6299186aeac1fd6c3f3493e

**Trade-off de região:** us-east-1 sai ~41% mais barato (US$ 43,83/mês a menos) que sa-east-1. A diferença está quase inteiramente no RDS, não no EC2. São Paulo permanece a escolha correta se o time/usuários estiverem no Brasil (latência, possível exigência de residência de dados); us-east-1 é a escolha correta se o objetivo é minimizar custo durante o desafio.

**Decisão:** região escolhida para o provisionamento — **us-east-1 (N. da Virgínia)**, priorizando custo (US$ 63,82/mês vs. US$ 107,65/mês). Latência para usuários no Brasil não é uma restrição relevante para este MVP de validação.

**Nota:** ambos os valores são On-Demand, sem AWS Free Tier aplicado (a calculadora não desconta o Free Tier automaticamente). Se o provisionamento for feito em uma conta pessoal dentro dos primeiros 12 meses, EC2 `t3.micro`, RDS `db.t3.micro` Single-AZ, 30 GB de EBS e 20 GB de armazenamento RDS são cobertos pelo Free Tier — o custo real tenderia a zero. Em uma AWS Academy Learner Lab, o Free Tier não se aplica da mesma forma; os valores acima são o número relevante, e vale conferir o orçamento do laboratório antes de deixar os recursos rodando por um mês inteiro.

---

## Referências

- Código-fonte: `app.py`, `Dockerfile`, `docker-compose.yaml`, `entrypoint.sh`
- 12-Factor App: https://12factor.net/pt_br/
- Exports da AWS Pricing Calculator: `estimativa-fase-1.json` (sa-east-1), `estivativa-fase-1-us.json` (us-east-1)
