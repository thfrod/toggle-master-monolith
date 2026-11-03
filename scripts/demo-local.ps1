$ErrorActionPreference = 'Stop'

docker compose up --build -d
docker compose ps

Write-Host 'Health checks:'
Invoke-RestMethod http://localhost:8081/health
Invoke-RestMethod http://localhost:8082/health
Invoke-RestMethod http://localhost:8083/health
Invoke-RestMethod http://localhost:8084/health
Invoke-RestMethod http://localhost:8085/health

Write-Host 'Criando uma flag e avaliando-a:'
Invoke-RestMethod -Method Post -Uri http://localhost:8082/flags -ContentType 'application/json' -Body '{"name":"demo","is_enabled":true}'
Invoke-RestMethod 'http://localhost:8084/evaluation/demo?subject_id=demo'
Start-Sleep -Seconds 3
Invoke-RestMethod http://localhost:8085/analytics/events
