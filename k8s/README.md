# Manifestos Kubernetes

O diretório `base` contém os cinco Deployments, cinco Services `ClusterIP`, ConfigMap, Ingress e os dois HPAs mínimos pedidos pelo desafio. O overlay `overlays/aws` troca as imagens locais pelas imagens do ECR.

## Secrets

Não versione credenciais reais. Para o modo recomendado, instale o External Secrets Operator, configure a role IRSA descrita em `aws/secret-store.yaml` e crie no AWS Secrets Manager um segredo JSON chamado `togglemaster/fase2/runtime` com as chaves:

`AUTH_DATABASE_URL`, `FLAGS_DATABASE_URL`, `TARGETING_DATABASE_URL`, `REDIS_ADDR` e `SQS_QUEUE_URL`.

Depois aplique `aws/secret-store.yaml` e `aws/external-secret.yaml`. Em AWS Academy, onde IRSA pode não estar disponível, use uma cópia local de `base/secret.example.yaml` com valores reais codificados em base64 e não a versione.

## Aplicação

```bash
kubectl apply -k k8s/overlays/aws
kubectl -n togglemaster get pods,svc,ingress,hpa
```

O arquivo `aws/keda-scaledobject.yaml` é opcional para conta pessoal. Se ele for aplicado, remova o HPA de CPU do `analytics-service` para não haver dois controladores disputando a mesma escala:

```bash
kubectl -n togglemaster delete hpa analytics-service
kubectl apply -f k8s/aws/keda-scaledobject.yaml
```
