# Script de Instalação - Printer Agent
# Sistema Industrial Labels - barbosasystem.tech

param(
    [switch]$Uninstall
)

# Configurações
$ServiceName = "LabelAgent"
$DisplayName = "Industrial Labels Printer Agent"
$Description = "Agent local para comunicação entre o sistema web e impressoras de etiquetas ZPL. Sistema: barbosasystem.tech"

# Cores para output
$GREEN = [System.ConsoleColor]::Green
$RED = [System.ConsoleColor]::Red
$YELLOW = [System.ConsoleColor]::Yellow
$BLUE = [System.ConsoleColor]::Blue

function Write-ColorOutput($ForegroundColor) {
    $fc = $host.UI.RawUI.ForegroundColor
    $host.UI.RawUI.ForegroundColor = $ForegroundColor
    if ($args) {
        Write-Output $args
    }
    else {
        $input | Write-Output
    }
    $host.UI.RawUI.ForegroundColor = $fc
}

function Log-Info($message) {
    Write-ColorOutput $GREEN "[INFO] $message"
}

function Log-Error($message) {
    Write-ColorOutput $RED "[ERROR] $message"
}

function Log-Warn($message) {
    Write-ColorOutput $YELLOW "[WARN] $message"
}

function Show-Banner {
    Write-Output ""
    Write-ColorOutput $BLUE "🖨️  Industrial Labels Printer Agent - Instalador"
    Write-ColorOutput $BLUE "🌐 barbosasystem.tech"
    Write-Output "════════════════════════════════════════════════════════"
    Write-Output ""
}

function Check-AdminRights {
    $currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
    $isAdmin = $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

    if (-not $isAdmin) {
        Log-Error "❌ Este script precisa ser executado como Administrador!"
        Write-Output ""
        Write-Output "Para executar como administrador:"
        Write-Output "1. Clique com o botão direito no PowerShell"
        Write-Output "2. Selecione 'Executar como administrador'"
        Write-Output "3. Execute novamente: .\install.ps1"
        Write-Output ""
        Read-Host "Pressione Enter para fechar"
        exit 1
    }

    Log-Info "✅ Executando com privilégios de administrador"
}

function Check-Executable {
    $exePath = Join-Path $PSScriptRoot "label-agent.exe"

    if (!(Test-Path $exePath)) {
        Log-Error "❌ Arquivo label-agent.exe não encontrado!"
        Log-Info "Certifique-se de que o arquivo está na mesma pasta que este script."
        Read-Host "Pressione Enter para fechar"
        exit 1
    }

    Log-Info "✅ Executável encontrado: $exePath"
    return $exePath
}

function Test-Port {
    Log-Info "Verificando porta 7777..."

    $portInUse = Get-NetTCPConnection -LocalPort 7777 -ErrorAction SilentlyContinue

    if ($portInUse) {
        Log-Warn "⚠️  Porta 7777 já está em uso"
        $processId = $portInUse.OwningProcess
        $process = Get-Process -Id $processId -ErrorAction SilentlyContinue
        if ($process) {
            Log-Warn "Processo usando a porta: $($process.ProcessName) (PID: $processId)"
        }
        Write-Output ""
        $response = Read-Host "Deseja continuar mesmo assim? (y/N)"
        if ($response -ne "y" -and $response -ne "Y") {
            Log-Info "Instalação cancelada."
            exit 0
        }
    } else {
        Log-Info "✅ Porta 7777 disponível"
    }
}

function Show-PrinterList {
    Write-Output ""
    Log-Info "📄 Impressoras disponíveis no sistema:"

    try {
        $printers = Get-Printer | Select-Object Name, DriverName, PortName

        if ($printers) {
            $printers | ForEach-Object {
                Write-Output "  🖨️  $($_.Name)"
                if ($_.DriverName -like "*Zebra*" -or $_.DriverName -like "*ZPL*") {
                    Write-ColorOutput $GREEN "      ✅ Compatível com ZPL"
                }
            }
        } else {
            Log-Warn "Nenhuma impressora encontrada"
            Log-Info "Configure sua impressora Zebra através do Painel de Controle"
        }
    } catch {
        Log-Warn "Erro ao listar impressoras: $($_.Exception.Message)"
    }
    Write-Output ""
}

function Install-Service($exePath) {
    Log-Info "🔧 Instalando serviço $ServiceName..."

    # Verificar se serviço já existe
    $existingService = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue

    if ($existingService) {
        Log-Warn "Serviço já existe. Atualizando..."

        if ($existingService.Status -eq "Running") {
            Log-Info "Parando serviço existente..."
            Stop-Service -Name $ServiceName -Force
            Start-Sleep -Seconds 2
        }

        Log-Info "Removendo serviço antigo..."
        & sc.exe delete $ServiceName | Out-Null
        Start-Sleep -Seconds 2
    }

    # Criar novo serviço
    Log-Info "Criando serviço..."
    $result = & sc.exe create $ServiceName binPath= "`"$exePath`"" start= auto DisplayName= $DisplayName

    if ($LASTEXITCODE -eq 0) {
        Log-Info "✅ Serviço criado com sucesso"

        # Configurar descrição
        & sc.exe description $ServiceName $Description | Out-Null

        # Configurar recuperação em caso de falha
        & sc.exe failure $ServiceName reset= 86400 actions= restart/5000/restart/5000/restart/5000 | Out-Null

        # Iniciar serviço
        Log-Info "🚀 Iniciando serviço..."
        Start-Service -Name $ServiceName

        Start-Sleep -Seconds 3

        # Verificar status
        $service = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
        if ($service -and $service.Status -eq "Running") {
            Log-Info "✅ Serviço iniciado com sucesso!"
            Write-Output ""
            Write-ColorOutput $GREEN "🎉 INSTALAÇÃO CONCLUÍDA COM SUCESSO!"
            Write-Output ""
            Log-Info "🌐 Agent acessível em: http://localhost:7777"
            Log-Info "🔗 Teste em: http://localhost:7777/health"
            Write-Output ""

            # Configurar firewall
            Log-Info "🔒 Configurando firewall..."
            try {
                New-NetFirewallRule -DisplayName "LabelAgent" -Direction Inbound -Port 7777 -Protocol TCP -Action Allow -ErrorAction SilentlyContinue | Out-Null
                Log-Info "✅ Regra de firewall criada"
            } catch {
                Log-Warn "⚠️  Não foi possível configurar o firewall automaticamente"
            }

        } else {
            Log-Error "❌ Erro ao iniciar serviço"
            Log-Info "Verifique os logs do Windows Event Viewer"
        }
    } else {
        Log-Error "❌ Erro ao criar serviço"
        Log-Info "Código de erro: $LASTEXITCODE"
    }
}

function Uninstall-Service {
    Log-Info "🗑️  Removendo serviço $ServiceName..."

    $service = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue

    if ($service) {
        if ($service.Status -eq "Running") {
            Log-Info "Parando serviço..."
            Stop-Service -Name $ServiceName -Force
            Start-Sleep -Seconds 2
        }

        Log-Info "Removendo serviço..."
        & sc.exe delete $ServiceName | Out-Null

        if ($LASTEXITCODE -eq 0) {
            Log-Info "✅ Serviço removido com sucesso"

            # Remover regra do firewall
            try {
                Remove-NetFirewallRule -DisplayName "LabelAgent" -ErrorAction SilentlyContinue | Out-Null
                Log-Info "✅ Regra de firewall removida"
            } catch {
                Log-Warn "⚠️  Regra de firewall não encontrada"
            }

        } else {
            Log-Error "❌ Erro ao remover serviço"
        }
    } else {
        Log-Warn "⚠️  Serviço não encontrado"
    }
}

function Test-Installation {
    Log-Info "🧪 Testando instalação..."

    try {
        $response = Invoke-WebRequest -Uri "http://localhost:7777/health" -UseBasicParsing -TimeoutSec 10
        if ($response.StatusCode -eq 200) {
            Log-Info "✅ Agent está respondendo corretamente"
            $content = $response.Content | ConvertFrom-Json
            if ($content.ok) {
                Log-Info "✅ Health check passou"
            }
        }
    } catch {
        Log-Warn "⚠️  Agent não está respondendo em http://localhost:7777"
        Log-Info "Isso é normal se o serviço ainda estiver inicializando"
    }
}

function Show-NextSteps {
    Write-Output ""
    Write-ColorOutput $BLUE "📋 PRÓXIMOS PASSOS:"
    Write-Output "═══════════════════════════════════════════════════════════════════"
    Write-Output ""
    Write-Output "1. 🌐 Acesse https://barbosasystem.tech"
    Write-Output "2. 🔓 Faça login no sistema"
    Write-Output "3. 🖨️  Configure sua impressora Zebra no sistema"
    Write-Output "4. 🏷️  Teste criando algumas etiquetas"
    Write-Output ""
    Write-Output "🔧 COMANDOS ÚTEIS:"
    Write-Output "  Verificar status:    Get-Service LabelAgent"
    Write-Output "  Parar serviço:       Stop-Service LabelAgent"
    Write-Output "  Iniciar serviço:     Start-Service LabelAgent"
    Write-Output "  Testar agent:        Invoke-WebRequest http://localhost:7777/health"
    Write-Output ""
    Write-Output "📞 SUPORTE:"
    Write-Output "  Sistema: https://barbosasystem.tech"
    Write-Output "  Logs: Event Viewer > Windows Logs > Application"
    Write-Output ""
}

# Função principal
function Main {
    Show-Banner

    if ($Uninstall) {
        Check-AdminRights
        Uninstall-Service
        Log-Info "✅ Desinstalação concluída"
        Read-Host "Pressione Enter para fechar"
        return
    }

    # Instalação
    Check-AdminRights
    $exePath = Check-Executable
    Test-Port
    Show-PrinterList

    Write-Output ""
    $confirm = Read-Host "Deseja instalar o LabelAgent como serviço do Windows? (Y/n)"
    if ($confirm -eq "n" -or $confirm -eq "N") {
        Log-Info "Instalação cancelada pelo usuário"
        exit 0
    }

    Install-Service $exePath
    Test-Installation
    Show-NextSteps

    Write-Output ""
    Read-Host "Pressione Enter para fechar"
}

# Executar
try {
    Main
} catch {
    Log-Error "Erro durante a instalação: $($_.Exception.Message)"
    Write-Output ""
    Read-Host "Pressione Enter para fechar"
}
