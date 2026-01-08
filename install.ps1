# Instalação do LabelAgent - Sistema Industrial Labels
# Serviço Windows para barbosasystem.tech

param(
    [switch]$Uninstall,
    [switch]$Start,
    [switch]$Stop,
    [switch]$Status
)

$ServiceName = "LabelAgent"
$DisplayName = "Industrial Labels Printer Agent"
$ExeFileName = "label-agent.exe"

# Cores
function Write-Success { param($msg) Write-Host "✅ $msg" -ForegroundColor Green }
function Write-Error { param($msg) Write-Host "❌ $msg" -ForegroundColor Red }
function Write-Info { param($msg) Write-Host "ℹ️  $msg" -ForegroundColor Cyan }
function Write-Warning { param($msg) Write-Host "⚠️  $msg" -ForegroundColor Yellow }

function Show-Banner {
    Write-Host ""
    Write-Host "🖨️  Industrial Labels Printer Agent" -ForegroundColor Blue
    Write-Host "🌐 barbosasystem.tech" -ForegroundColor Blue
    Write-Host "══════════════════════════════════════════════" -ForegroundColor Blue
    Write-Host ""
}

function Test-IsAdmin {
    $currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
    return $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Require-Admin {
    if (-not (Test-IsAdmin)) {
        Write-Error "Este script precisa ser executado como Administrador!"
        Write-Host ""
        Write-Host "Como executar como administrador:"
        Write-Host "1. Clique com botão direito no PowerShell"
        Write-Host "2. Selecione 'Executar como administrador'"
        Write-Host "3. Execute: .\install.ps1"
        Write-Host ""
        Read-Host "Pressione Enter para sair"
        exit 1
    }
}

function Get-ServiceStatus {
    $service = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if ($service) {
        return @{
            Exists = $true
            Status = $service.Status
            StartType = (Get-WmiObject -Class Win32_Service -Filter "Name='$ServiceName'").StartMode
        }
    }
    return @{ Exists = $false }
}

function Show-ServiceStatus {
    $status = Get-ServiceStatus

    Write-Host "📊 STATUS DO SERVIÇO:" -ForegroundColor Yellow
    Write-Host "─────────────────────────────────────────"

    if ($status.Exists) {
        $statusColor = if ($status.Status -eq "Running") { "Green" } else { "Red" }
        Write-Host "Serviço: $ServiceName" -ForegroundColor White
        Write-Host "Status: $($status.Status)" -ForegroundColor $statusColor
        Write-Host "Inicialização: $($status.StartType)" -ForegroundColor White

        if ($status.Status -eq "Running") {
            Write-Host "🌐 Agent acessível em: http://localhost:7777" -ForegroundColor Green

            # Testar conectividade
            try {
                $response = Invoke-WebRequest -Uri "http://localhost:7777/health" -UseBasicParsing -TimeoutSec 5
                Write-Success "Agent respondendo corretamente"
            } catch {
                Write-Warning "Agent não está respondendo (pode estar inicializando)"
            }
        }
    } else {
        Write-Host "Serviço: NÃO INSTALADO" -ForegroundColor Red
    }
    Write-Host ""
}

function Install-ServiceAgent {
    Write-Info "🔧 Instalando LabelAgent como serviço..."

    # Verificar se executável existe
    $exePath = Join-Path $PSScriptRoot $ExeFileName
    if (-not (Test-Path $exePath)) {
        Write-Error "Arquivo $ExeFileName não encontrado!"
        Write-Info "Certifique-se de que o arquivo está na pasta: $PSScriptRoot"
        return $false
    }
    Write-Success "Executável encontrado: $ExeFileName"

    # Remover serviço existente se houver
    $status = Get-ServiceStatus
    if ($status.Exists) {
        Write-Info "Removendo instalação anterior..."
        if ($status.Status -eq "Running") {
            Stop-Service -Name $ServiceName -Force
            Start-Sleep 2
        }
        sc.exe delete $ServiceName | Out-Null
        Start-Sleep 2
    }

    # Criar serviço
    Write-Info "Criando serviço Windows..."
    $result = sc.exe create $ServiceName binPath= "`"$exePath`"" start= auto DisplayName= $DisplayName

    if ($LASTEXITCODE -ne 0) {
        Write-Error "Falha ao criar serviço (Código: $LASTEXITCODE)"
        return $false
    }

    # Configurar propriedades do serviço
    sc.exe description $ServiceName "Agent local para comunicação com impressoras ZPL. Sistema: barbosasystem.tech" | Out-Null
    sc.exe failure $ServiceName reset= 86400 actions= restart/5000/restart/5000/restart/5000 | Out-Null

    Write-Success "Serviço criado com sucesso"

    # Iniciar serviço
    Write-Info "Iniciando serviço..."
    Start-Service -Name $ServiceName
    Start-Sleep 3

    # Verificar se iniciou corretamente
    $newStatus = Get-ServiceStatus
    if ($newStatus.Status -eq "Running") {
        Write-Success "Serviço iniciado com sucesso!"

        # Configurar firewall
        Write-Info "Configurando firewall..."
        try {
            New-NetFirewallRule -DisplayName "LabelAgent-7777" -Direction Inbound -Port 7777 -Protocol TCP -Action Allow -ErrorAction SilentlyContinue | Out-Null
            Write-Success "Regra de firewall criada"
        } catch {
            Write-Warning "Não foi possível configurar firewall automaticamente"
        }

        return $true
    } else {
        Write-Error "Falha ao iniciar serviço"
        return $false
    }
}

function Uninstall-ServiceAgent {
    Write-Info "🗑️ Removendo LabelAgent..."

    $status = Get-ServiceStatus
    if (-not $status.Exists) {
        Write-Warning "Serviço não está instalado"
        return
    }

    if ($status.Status -eq "Running") {
        Write-Info "Parando serviço..."
        Stop-Service -Name $ServiceName -Force
        Start-Sleep 2
    }

    Write-Info "Removendo serviço..."
    sc.exe delete $ServiceName | Out-Null

    if ($LASTEXITCODE -eq 0) {
        Write-Success "Serviço removido com sucesso"

        # Remover regra do firewall
        try {
            Remove-NetFirewallRule -DisplayName "LabelAgent-7777" -ErrorAction SilentlyContinue | Out-Null
            Write-Success "Regra de firewall removida"
        } catch {
            # Ignorar erro se regra não existe
        }
    } else {
        Write-Error "Erro ao remover serviço"
    }
}

function Show-PrinterList {
    Write-Host "🖨️ IMPRESSORAS DISPONÍVEIS:" -ForegroundColor Yellow
    Write-Host "─────────────────────────────────────────"

    try {
        $printers = Get-Printer | Select-Object Name, DriverName
        if ($printers) {
            foreach ($printer in $printers) {
                Write-Host "• $($printer.Name)" -ForegroundColor White
                if ($printer.DriverName -like "*Zebra*" -or $printer.DriverName -like "*ZPL*") {
                    Write-Host "  ✅ Compatível com ZPL" -ForegroundColor Green
                } else {
                    Write-Host "  ⚠️  Driver: $($printer.DriverName)" -ForegroundColor Gray
                }
            }
        } else {
            Write-Warning "Nenhuma impressora encontrada"
            Write-Info "Instale sua impressora Zebra através do Painel de Controle"
        }
    } catch {
        Write-Error "Erro ao listar impressoras: $($_.Exception.Message)"
    }
    Write-Host ""
}

function Show-PostInstall {
    Write-Host ""
    Write-Host "🎉 INSTALAÇÃO CONCLUÍDA COM SUCESSO!" -ForegroundColor Green
    Write-Host "═══════════════════════════════════════════════════════════" -ForegroundColor Green
    Write-Host ""
    Write-Host "✅ Serviço instalado e iniciado automaticamente" -ForegroundColor Green
    Write-Host "✅ Configurado para iniciar com o Windows" -ForegroundColor Green
    Write-Host "✅ Firewall configurado" -ForegroundColor Green
    Write-Host ""
    Write-Host "🌐 PRÓXIMOS PASSOS:" -ForegroundColor Cyan
    Write-Host "1. Acesse: https://barbosasystem.tech"
    Write-Host "2. Faça login no sistema"
    Write-Host "3. Configure sua impressora"
    Write-Host "4. Teste criando etiquetas"
    Write-Host ""
    Write-Host "🔧 COMANDOS ÚTEIS:" -ForegroundColor Yellow
    Write-Host "• Verificar status: .\install.ps1 -Status"
    Write-Host "• Parar serviço: .\install.ps1 -Stop"
    Write-Host "• Iniciar serviço: .\install.ps1 -Start"
    Write-Host "• Remover: .\install.ps1 -Uninstall"
    Write-Host ""
    Write-Host "📞 Suporte: https://barbosasystem.tech" -ForegroundColor Blue
    Write-Host ""
}

# FUNÇÃO PRINCIPAL
function Main {
    Show-Banner

    # Operações que requerem admin
    if ($Uninstall -or (-not $Status -and -not $Start -and -not $Stop)) {
        Require-Admin
    }

    # Comandos específicos
    if ($Status) {
        Show-ServiceStatus
        return
    }

    if ($Start) {
        Require-Admin
        Write-Info "Iniciando serviço..."
        Start-Service -Name $ServiceName -ErrorAction SilentlyContinue
        Start-Sleep 2
        Show-ServiceStatus
        return
    }

    if ($Stop) {
        Require-Admin
        Write-Info "Parando serviço..."
        Stop-Service -Name $ServiceName -Force -ErrorAction SilentlyContinue
        Start-Sleep 2
        Show-ServiceStatus
        return
    }

    if ($Uninstall) {
        Uninstall-ServiceAgent
        Write-Host "Desinstalação concluída!" -ForegroundColor Green
        Read-Host "Pressione Enter para continuar"
        return
    }

    # Instalação padrão
    Write-Info "Verificando sistema..."

    # Verificar se já está instalado
    $status = Get-ServiceStatus
    if ($status.Exists -and $status.Status -eq "Running") {
        Write-Success "LabelAgent já está instalado e rodando!"
        Show-ServiceStatus
        Read-Host "Pressione Enter para continuar"
        return
    }

    # Mostrar impressoras disponíveis
    Show-PrinterList

    # Confirmar instalação
    $response = Read-Host "Instalar LabelAgent como serviço do Windows? (Y/n)"
    if ($response -eq "n" -or $response -eq "N") {
        Write-Info "Instalação cancelada"
        return
    }

    # Executar instalação
    if (Install-ServiceAgent) {
        Show-PostInstall
    } else {
        Write-Error "Instalação falhou!"
        Write-Info "Verifique os logs do Event Viewer para mais detalhes"
    }

    Read-Host "Pressione Enter para continuar"
}

# EXECUTAR
try {
    Main
} catch {
    Write-Error "Erro durante execução: $($_.Exception.Message)"
    Read-Host "Pressione Enter para continuar"
}
