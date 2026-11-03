# Relatório de Entrega — Fase 2 — ToggleMaster

> Tech Challenge · POSTECH DCLT · Microsserviços, Docker e Kubernetes na AWS

## Participantes

- Wesley Ferreira de Oliveira — rm376487
- Thiago Ferreira Rodrigues — rm376442

## Links

| Item | Link |
|---|---|
| Repositório | https://github.com/dougls/toggle-master-monolith |
| Diagrama de arquitetura | [`docs/arquitetura-fase-2.mmd`](../../docs/arquitetura-fase-2.mmd) |
| Análise da arquitetura | [`docs/ARQUITETURA-FASE-2.md`](../../docs/ARQUITETURA-FASE-2.md) |
| Vídeo de demonstração | A preencher após a gravação |

## Implementação

O monólito foi decomposto em cinco serviços independentes:

| Serviço | Stack | Responsabilidade | Persistência |
|---|---|---|---|
| `auth-service` | Go | Criação e verificação de chaves de API | PostgreSQL |
| `flag-service` | Python | CRUD de feature flags | PostgreSQL |
| `targeting-service` | Python | Regras de segmentação e matching | PostgreSQL |
| `evaluation-service` | Go | Hot path de avaliação, cache e publicação de eventos | Redis + SQS |
| `analytics-service` | Python | Consumo de eventos e persistência analítica | DynamoDB |

Cada serviço possui Dockerfile multi-stage. O Compose sobe os 5 apps, 2 PostgreSQL, Redis e DynamoDB Local: 9 contêineres. No ambiente local, Redis também funciona como fila de eventos para não introduzir um décimo contêiner; em produção o driver é trocado para SQS por `EVENT_BUS_DRIVER=sqs`.

## Kubernetes

Os manifestos em `k8s/base` incluem Namespace, Deployment, Service `ClusterIP`, ConfigMap, Ingress, requests/limits, readiness/liveness probes e HPA de CPU para `evaluation-service` e `analytics-service`. O overlay AWS troca as imagens pelos cinco repositórios ECR.

Para conta pessoal, `k8s/aws/keda-scaledobject.yaml` oferece o caminho recomendado de KEDA para escalar analytics diretamente pela profundidade da SQS. Para AWS Academy, o HPA por CPU é mantido porque não depende de IRSA nem de roles adicionais.

## Secrets e melhoria da Fase 1

O caminho de produção usa AWS Secrets Manager com o segredo `togglemaster/fase2/runtime`, External Secrets Operator e IRSA. O Kubernetes Secret é materializado em runtime, e não armazenado no repositório. O arquivo `k8s/base/secret.example.yaml` existe somente para laboratório e contém valores de exemplo em base64; nenhum segredo real deve ser commitado.

Também foram corrigidos os pontos conceituais destacados na análise de 12-Factor: configuração por ambiente, artefatos imutáveis ECR, paridade entre Compose e Kubernetes, probes/disposability, concorrência horizontal e logs em stdout. O diagrama agora exibe todas as subnets públicas e privadas usadas pela solução.

## Roteiro de demonstração

1. `docker compose up --build` e `docker compose ps`, mostrando os 9 contêineres.
2. Criar uma flag em `http://localhost:8082/flags` e avaliar em `http://localhost:8084/evaluation/demo`.
3. Conferir o evento persistido em `http://localhost:8085/analytics/events`.
4. Criar/usar o EKS conforme `infra/PROVISIONAMENTO-AWS.md`, aplicar `kubectl apply -k k8s/overlays/aws` e mostrar `kubectl get pods`.
5. Mostrar o DNS do Load Balancer do Nginx Ingress e chamar `/flags` e `/evaluation` com `curl`.
6. Gerar carga no endpoint de evaluation, verificar `kubectl get hpa`/`kubectl get pods` e enviar mensagens à SQS.
7. Mostrar o HPA ou KEDA do analytics e a tabela `togglemaster-analytics` no DynamoDB.

## Data stores

- RDS PostgreSQL guarda dados relacionais e transacionais de auth, flags e regras.
- ElastiCache Redis guarda dados temporários de baixa latência e o cache do hot path.
- DynamoDB guarda eventos analíticos com escala e modelo de acesso simples, sem joins.
- SQS desacopla a avaliação da persistência analítica e absorve picos.

## Estado da nuvem

O código, os manifestos e o roteiro de provisionamento estão entregues no repositório. A criação efetiva dos recursos AWS depende da conta/role do grupo e deve ser executada conforme a opção AWS Academy ou conta pessoal escolhida.
