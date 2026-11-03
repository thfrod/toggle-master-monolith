# Provisionamento AWS da Fase 2

Este roteiro transforma os artefatos do repositório em um ambiente AWS. Ele não executa recursos na conta do grupo: a criação de EKS, RDS, ElastiCache, DynamoDB, SQS e ECR precisa ser autorizada e executada na conta AWS escolhida.

## Topologia

Use a topologia descrita em [`docs/arquitetura-fase-2.mmd`](../docs/arquitetura-fase-2.mmd):

- duas subnets públicas, uma em cada AZ, para o Load Balancer criado pelo Nginx Ingress;
- duas subnets privadas de workloads, uma em cada AZ, para os nós gerenciados do EKS;
- duas subnets privadas de dados, uma em cada AZ, para os DB Subnet Groups dos três RDS e do ElastiCache;
- endpoints/rotas privadas para ECR, CloudWatch, Secrets Manager, SQS e DynamoDB quando a conta permitir, reduzindo a dependência de saída pública.

## Opção B — conta pessoal AWS

1. Crie o cluster e os nós com `eksctl` usando `infra/eksctl-cluster.yaml`. O arquivo usa uma VPC multi-AZ, nós privados e Auto Scaling 1/2/4.

   ```bash
   eksctl create cluster -f infra/eksctl-cluster.yaml
   aws eks update-kubeconfig --region us-east-1 --name togglemaster-fase2
   ```

2. Crie os cinco repositórios ECR e publique as imagens:

   ```powershell
   $repoNames = @('auth-service','flag-service','targeting-service','evaluation-service','analytics-service')
   $repoNames | ForEach-Object { aws ecr create-repository --repository-name "togglemaster-$_" --region us-east-1 }
   .\scripts\build-and-push.ps1 -AccountId '<AWS_ACCOUNT_ID>' -Region 'us-east-1' -Tag 'v1'
   ```

3. Crie três RDS PostgreSQL independentes, privados e Single-AZ, um para cada serviço relacional: `togglemaster-auth`, `togglemaster-flags` e `togglemaster-targeting`. Use o mesmo DB Subnet Group com as duas subnets privadas de dados, `publicly accessible = No` e Security Groups que aceitem 5432 somente do Security Group dos nós/pods do EKS.

4. Crie o ElastiCache Redis em subnets privadas de dados. Libere 6379 somente do Security Group dos nós/pods do EKS.

5. Crie a tabela DynamoDB e a fila SQS:

   ```bash
   aws dynamodb create-table --table-name togglemaster-analytics --attribute-definitions AttributeName=event_id,AttributeType=S --key-schema AttributeName=event_id,KeyType=HASH --billing-mode PAY_PER_REQUEST --region us-east-1
   aws sqs create-queue --queue-name togglemaster-analytics --region us-east-1
   ```

6. Crie no AWS Secrets Manager o segredo `togglemaster/fase2/runtime` como JSON. Ele deve conter `AUTH_DATABASE_URL`, `FLAGS_DATABASE_URL`, `TARGETING_DATABASE_URL`, `REDIS_ADDR` e `SQS_QUEUE_URL`. Crie as roles com as políticas de exemplo em `infra/iam/`: a aplicação usa SQS/DynamoDB/Secrets Manager por IRSA e o External Secrets Operator usa uma role separada somente para ler o segredo. A aplicação não precisa de chaves AWS estáticas.

7. Instale os componentes do cluster:

   ```bash
   kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
   helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
   helm repo update
   helm upgrade --install ingress-nginx ingress-nginx/ingress-nginx --namespace ingress-nginx --create-namespace --set controller.service.type=LoadBalancer
   ```

8. Instale o External Secrets Operator, configure a role IRSA indicada em `k8s/aws/secret-store.yaml`, aplique o SecretStore/ExternalSecret e depois os manifestos:

   ```bash
   helm repo add external-secrets https://charts.external-secrets.io
   helm repo update
   helm upgrade --install external-secrets external-secrets/external-secrets --namespace external-secrets --create-namespace
   kubectl apply -f k8s/aws/secret-store.yaml
   kubectl apply -f k8s/aws/external-secret.yaml
   kubectl apply -k k8s/overlays/aws
   ```

9. Substitua o account ID no overlay por `kubectl kustomize`/`kustomize edit set image`, aguarde os Pods e obtenha o endereço externo:

   ```bash
   kubectl -n togglemaster get pods,svc,ingress,hpa
   kubectl -n ingress-nginx get svc ingress-nginx-controller
   ```

## Opção A — AWS Academy

No Academy, crie o EKS pelo Console, selecione a role existente `LabRole`, crie um Managed Node Group também com `LabRole` e use mínimo 1, desejado 2 e máximo 4. Não use `eksctl create cluster`, KEDA ou IRSA se o laboratório não permitir novas roles.

Depois, instale Metrics Server e Nginx Ingress com os comandos acima, aplique o overlay de imagens e use `base/secret.example.yaml` apenas como arquivo local não versionado para os endpoints/credenciais. Para o analytics, mantenha o HPA por CPU; ele é o caminho compatível com a limitação do Academy. O worker usa as credenciais temporárias da `LabRole` disponíveis no ambiente do nó.

## Verificações pós-deploy

```bash
kubectl -n togglemaster rollout status deployment --all
kubectl -n togglemaster get pods -o wide
kubectl -n togglemaster get hpa
kubectl -n togglemaster describe ingress togglemaster
```

Teste os endpoints pelo DNS do Load Balancer:

```bash
curl -H 'Host: togglemaster.example.com' http://<LOAD_BALANCER>/flags
curl -H 'Host: togglemaster.example.com' 'http://<LOAD_BALANCER>/evaluation/my-flag?subject_id=demo'
```

Para limpar o laboratório, remova os Deployments/Ingress, o cluster, os RDS, o ElastiCache, a tabela, a fila e os repositórios ECR. Isso evita custos contínuos.
