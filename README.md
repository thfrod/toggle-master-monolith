# ToggleMaster — Tech Challenge Fase 2

O projeto evolui o MVP monolítico da Fase 1 para um ecossistema de cinco microsserviços containerizados e pronto para Kubernetes/EKS.

## Estrutura

```text
services/
  auth-service/          Go + PostgreSQL
  flag-service/          Python + PostgreSQL
  targeting-service/     Python + PostgreSQL
  evaluation-service/    Go + Redis + SQS
  analytics-service/     Python + DynamoDB + SQS
k8s/                     Namespace, Deployments, Services, Ingress, HPA, secrets
infra/                   EKS, ECR, RDS, Redis, SQS, DynamoDB e checklist AWS
docs/                    Diagrama e análise da arquitetura
entregaveis/fase-2/      Relatório da entrega
```

## Execução local

Pré-requisitos: Docker Desktop com Compose v2.

```bash
docker compose up --build -d
docker compose ps
```

Isso inicia exatamente 9 contêineres: 5 aplicações, 2 PostgreSQL, Redis e DynamoDB Local. O Compose usa Redis como fila local para manter a demonstração nos 9 contêineres; no AWS, `evaluation-service` e `analytics-service` usam SQS.

Endpoints locais:

| Serviço | URL |
|---|---|
| auth-service | http://localhost:8081 |
| flag-service | http://localhost:8082 |
| targeting-service | http://localhost:8083 |
| evaluation-service | http://localhost:8084 |
| analytics-service | http://localhost:8085 |

Exemplo ponta a ponta:

```bash
curl -X POST http://localhost:8082/flags -H 'Content-Type: application/json' -d '{"name":"demo","is_enabled":true}'
curl 'http://localhost:8084/evaluation/demo?subject_id=user-1'
curl http://localhost:8085/analytics/events
```

No PowerShell, o roteiro equivalente é `./scripts/demo-local.ps1`.

## Testes unitários

Instale as dependências de desenvolvimento e execute os testes dos serviços Python:

```powershell
python -m pip install -r requirements-dev.txt
python -m pytest tests/python -q
```

Os serviços Go possuem testes nativos com `go test`:

```powershell
Push-Location services/auth-service; go test ./...; Pop-Location
Push-Location services/evaluation-service; go test ./...; Pop-Location
```

O comando `./scripts/test.ps1` executa toda a suíte. Os testes não precisam de PostgreSQL, Redis, DynamoDB ou AWS ativos: as integrações são substituídas por objetos falsos e servidores HTTP de teste.

## AWS/EKS

Leia [`infra/PROVISIONAMENTO-AWS.md`](infra/PROVISIONAMENTO-AWS.md) antes de criar recursos. O roteiro cobre:

- EKS com nós gerenciados e Auto Scaling 1/2/4;
- cinco repositórios ECR e publicação das imagens;
- três RDS PostgreSQL independentes, ElastiCache Redis, DynamoDB e SQS;
- Metrics Server, Nginx Ingress, Secrets Manager/External Secrets e IRSA;
- fallback AWS Academy com `LabRole`, HPA por CPU e Secret local base64 não versionado.

Depois de publicar as imagens:

```bash
kubectl apply -k k8s/overlays/aws
kubectl -n togglemaster get pods,svc,ingress,hpa
```

Substitua o account ID e a tag no overlay `k8s/overlays/aws/kustomization.yaml`, ou use `scripts/build-and-push.ps1` e ajuste o overlay antes do `kubectl apply`.

## Documentação da entrega

- [Relatório da Fase 2](entregaveis/fase-2/fase-2.md)
- [Arquitetura da Fase 2](docs/ARQUITETURA-FASE-2.md)
- [Diagrama Mermaid](docs/arquitetura-fase-2.mmd)
- [Análise da Fase 1](docs/ANALISE-ARQUITETURA.md)
- [Relatório da Fase 1](entregaveis/fase-1/fase-1.md)

## Encerramento

Depois da avaliação, remova o cluster, RDS, ElastiCache, fila, tabela e repositórios ECR para evitar custos contínuos. Não versione credenciais, URLs reais de banco ou chaves AWS.
