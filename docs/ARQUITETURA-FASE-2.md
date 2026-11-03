# Arquitetura da Fase 2

O diagrama fonte está em [`arquitetura-fase-2.mmd`](arquitetura-fase-2.mmd). Ele corrige a observação da correção da Fase 1 ao mostrar todas as subnets efetivamente utilizadas: duas públicas para o Load Balancer, duas privadas de workloads para os nós do EKS e duas privadas de dados para RDS/ElastiCache, distribuídas entre `us-east-1a` e `us-east-1b`.

## Fluxo

O Nginx Ingress recebe o tráfego externo e encaminha `/auth`, `/flags`, `/targeting` e `/evaluation` para Services `ClusterIP` no namespace `togglemaster`. O `evaluation-service` consulta flags e regras, usa o Redis como cache e publica eventos. No AWS, os eventos vão para SQS e são consumidos pelo `analytics-service`, que grava na tabela DynamoDB.

O auth, flags e targeting possuem RDS PostgreSQL independentes em produção, reduzindo o acoplamento e permitindo escala/backup por domínio. No Compose local há duas instâncias PostgreSQL: `postgres-auth` e `postgres-flags`; flag e targeting compartilham a segunda instância apenas para manter o laboratório local nos 9 contêineres pedidos. A configuração continua separada por `*_DATABASE_URL`, então a promoção para três RDS não exige alteração de código.

## Correções da Fase 1 incorporadas

- Configuração: serviços usam variáveis de ambiente para portas, endpoints, nomes de filas e conexões; `DB_PORT`/`PORT` não ficam hardcoded no código novo.
- Build/release/run: cada serviço é uma imagem independente, construída em multi-stage e promovida pelo ECR; Kubernetes implanta imagens imutáveis por tag.
- Paridade dev/prod: o Compose executa os mesmos cinco serviços e usa os mesmos contratos de rede; apenas os adaptadores locais de fila/DynamoDB trocam Redis/DynamoDB Local pelos serviços AWS.
- Disposability e logs: probes, reinício pelo Deployment e logs em stdout permitem que o orquestrador recicle Pods sem estado local.
- Secrets: valores reais não entram no Git. Na conta pessoal, o caminho recomendado é Secrets Manager + External Secrets + IRSA; no Academy há um fallback local com Secret base64 não versionado, devido à limitação da `LabRole`.
- Diagrama: subnets públicas, privadas de workloads e privadas de dados aparecem individualmente por AZ.
