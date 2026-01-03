$ErrorActionPreference = "Stop"

$ServiceName = "LabelAgent"
$InstallDir = "C:\Program Files\LabelAgent"
$ExePath = "$InstallDir\label-agent.exe"

Write-Host "Instalando LabelAgent..."

# Cria diretório
New-Item -ItemType Directory -Force $InstallDir | Out-Null

# Copia exe
Copy-Item ".\label-agent.exe" $ExePath -Force

# Remove serviço antigo se existir
if (Get-Service -Name $ServiceName -ErrorAction SilentlyContinue) {
    Write-Host "Serviço antigo encontrado. Removendo..."
    sc.exe stop $ServiceName | Out-Null
    sc.exe delete $ServiceName | Out-Null
    Start-Sleep -Seconds 2
}

# Cria serviço
sc.exe create $ServiceName `
  binPath= "`"$ExePath`"" `
  start= auto | Out-Null

# Inicia serviço
sc.exe start $ServiceName | Out-Null

Write-Host "LabelAgent instalado e rodando!"
