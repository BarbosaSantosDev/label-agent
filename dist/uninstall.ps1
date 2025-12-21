$ErrorActionPreference = "Stop"

$ServiceName = "LabelAgent"
$InstallDir = "C:\Program Files\LabelAgent"

Write-Host "🗑️ Removendo LabelAgent..."

if (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) {
    sc.exe stop $ServiceName | Out-Null
    sc.exe delete $ServiceName | Out-Null
    Start-Sleep -Seconds 2
}

if (Test-Path $InstallDir) {
    Remove-Item $InstallDir -Recurse -Force
}

Write-Host "✅ LabelAgent removido com sucesso!"

