$ErrorActionPreference = 'Stop'

python -m pytest tests/python -q

Push-Location services/auth-service
try {
    go test ./...
}
finally {
    Pop-Location
}

Push-Location services/evaluation-service
try {
    go test ./...
}
finally {
    Pop-Location
}
