param(
    [Parameter(Mandatory = $true)] [string] $AccountId,
    [string] $Region = 'us-east-1',
    [string] $Tag = 'v1'
)

$ErrorActionPreference = 'Stop'
$registry = "$AccountId.dkr.ecr.$Region.amazonaws.com"
$services = @('auth-service', 'flag-service', 'targeting-service', 'evaluation-service', 'analytics-service')

aws ecr get-login-password --region $Region | docker login --username AWS --password-stdin $registry

foreach ($service in $services) {
    $image = "$registry/togglemaster-$service`:$Tag"
    docker build -t $image "services/$service"
    docker push $image
}

Write-Host "Imagens publicadas em $registry com a tag $Tag"
