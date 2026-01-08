# Build Script para Windows - Printer Agent
# Sistema Industrial Labels - barbosasystem.tech

param(
    [switch]$Service,
    [switch]$Console,
    [switch]$Install,
    [switch]$Uninstall,
    [switch]$Clean,
    [switch]$Help
)

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

function Log-Step($message) {
    Write-ColorOutput $BLUE "[STEP] $message"
}

function Show-Help {
    Write-Output ""
    Write-ColorOutput $BLUE "🖨️  Industrial Labels Printer Agent - Build Script"
    Write-Output "════════════════════════════════════════════════════════"
    Write-Output "USAGE:"
    Write-Output "  .\build-windows.ps1 [OPTIONS]"
    Write-Output ""
    Write-Output "OPTIONS:"
    Write-Output "  -Console     Build para execução em console"
    Write-Output "  -Service     Build para instalação como serviço"
    Write-Output "  -Install     Instalar como serviço do Windows"
    Write-Output "  -Uninstall   Remover serviço do Windows"
    Write-Output "  -Clean       Limpar arquivos de build"
    Write-Output "  -Help        Mostrar esta ajuda"
    Write-Output ""
    Write-Output "EXEMPLOS:"
    Write-Output "  .\build-windows.ps1 -Console     # Build e execução manual"
    Write-Output "  .\build-windows.ps1 -Service     # Build para serviço"
    Write-Output "  .\build-windows.ps1 -Install     # Instalar serviço"
    Write-Output ""
}

function Check-GoInstalled {
    Log-Step "Verificando instalação do Go..."

    if (!(Get-Command "go" -ErrorAction SilentlyContinue)) {
        Log-Error "Go não está instalado ou não está no PATH!"
        Log-Info "Baixe e instale o Go em: https://golang.org/dl/"
        exit 1
    }

    $goVersion = go version
    Log-Info "✅ $goVersion"
}

function Check-AdminRights {
    $currentPrincipal = New-Object Security.Principal.WindowsPrincipal([Security.Principal.WindowsIdentity]::GetCurrent())
    $isAdmin = $currentPrincipal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)

    if (-not $isAdmin) {
        Log-Error "❌ Este script precisa ser executado como Administrador!"
        Log-Info "Clique com o botão direito no PowerShell e selecione 'Executar como administrador'"
        exit 1
    }

    Log-Info "✅ Executando com privilégios de administrador"
}

function Build-Agent($isService = $false) {
    Log-Step "Fazendo build do printer agent..."

    # Definir variáveis de build
    $env:CGO_ENABLED = "0"
    $env:GOOS = "windows"
    $env:GOARCH = "amd64"

    # Nome do executável
    $exeName = if ($isService) { "label-agent-service.exe" } else { "label-agent.exe" }

    # Flags de build
    $buildFlags = @(
        "-ldflags", "-s -w"
        "-o", $exeName
        "."
    )

    if ($isService) {
        $buildFlags += @("-tags", "service")
    }

    # Build
    $result = & go build @buildFlags

    if ($LASTEXITCODE -eq 0) {
        Log-Info "✅ Build concluído com sucesso: $exeName"
        return $exeName
    } else {
        Log-Error "❌ Erro no build!"
        exit 1
    }
}

function Test-Port($port = 7777) {
    Log-Step "Verificando se a porta $port está disponível..."

    $portInUse = Get-NetTCPConnection -LocalPort $port -ErrorAction SilentlyContinue

    if ($portInUse) {
        Log-Warn "Porta $port já está em uso!"
        $response = Read-Host "Deseja continuar mesmo assim? (y/N)"
        if ($response -ne "y" -and $response -ne "Y") {
            Log-Info "Operação cancelada."
            exit 0
        }
    } else {
        Log-Info "Porta $port disponível"
    }
}

function Get-WindowsPrinters {
    Log-Step "Impressoras disponíveis:"

    try {
        $printers = Get-Printer | Select-Object Name, DriverName, PortName

        if ($printers) {
            $printers | ForEach-Object {
                Write-Output "  - $($_.Name) ($($_.DriverName))"
            }
        } else {
            Log-Warn "Nenhuma impressora encontrada"
            Log-Info "Configure uma impressora através do Painel de Controle"
        }
    } catch {
        Log-Error "Erro ao listar impressoras: $($_.Exception.Message)"
    }
}

function Install-Service($exePath) {
    Log-Step "Instalando serviço LabelAgent..."

    # Verificar se serviço já existe
    $existingService = Get-Service -Name "LabelAgent" -ErrorAction SilentlyContinue

    if ($existingService) {
        Log-Warn "Serviço já existe. Removendo primeiro..."
        if ($existingService.Status -eq "Running") {
            Stop-Service -Name "LabelAgent" -Force
        }
        & sc.exe delete "LabelAgent"
        Start-Sleep -Seconds 2
    }

    # Criar serviço
    $result = & sc.exe create "LabelAgent" binPath= "$exePath" start= auto DisplayName= "Industrial Labels Printer Agent"

    if ($LASTEXITCODE -eq 0) {
        Log-Info "✅ Serviço instalado com sucesso"

        # Configurar descrição
        & sc.exe description "LabelAgent" "Agent local para comunicação entre o sistema web e impressoras de etiquetas ZPL. Sistema: barbosasystem.tech"

        # Configurar recuperação em caso de falha
        & sc.exe failure "LabelAgent" reset= 86400 actions= restart/5000/restart/5000/restart/5000

        # Iniciar serviço
        Start-Service -Name "LabelAgent"

        $service = Get-Service -Name "LabelAgent"
        if ($service.Status -eq "Running") {
            Log-Info "✅ Serviço iniciado com sucesso"
            Log-Info "🌐 Agent acessível em: http://localhost:7777"
        } else {
            Log-Error "❌ Erro ao iniciar serviço"
        }
    } else {
        Log-Error "❌ Erro ao instalar serviço"
    }
}

function Uninstall-Service {
    Log-Step "Removendo serviço LabelAgent..."

    $service = Get-Service -Name "LabelAgent" -ErrorAction SilentlyContinue

    if ($service) {
        if ($service.Status -eq "Running") {
            Log-Info "Parando serviço..."
            Stop-Service -Name "LabelAgent" -Force
        }

        & sc.exe delete "LabelAgent"

        if ($LASTEXITCODE -eq 0) {
            Log-Info "✅ Serviço removido com sucesso"
        } else {
            Log-Error "❌ Erro ao remover serviço"
        }
    } else {
        Log-Warn "Serviço não encontrado"
    }
}

function Run-Console($exePath) {
    Log-Step "Iniciando Printer Agent em modo console..."
    Log-Info "🌐 Será acessível em: http://localhost:7777"
    Log-Info "🖨️  Compatível com: https://barbosasystem.tech"
    Log-Info "📁 Logs serão salvos em: %ProgramData%\LabelAgent\"
    Log-Info ""
    Log-Info "Para parar, pressione Ctrl+C"
    Log-Info ""

    & $exePath
}

function Clean-BuildFiles {
    Log-Step "Limpando arquivos de build..."

    $filesToRemove = @("label-agent.exe", "label-agent-service.exe")

    foreach ($file in $filesToRemove) {
        if (Test-Path $file) {
            Remove-Item $file -Force
            Log-Info "Removido: $file"
        }
    }

    Log-Info "✅ Limpeza concluída"
}

function Show-Banner {
    Write-Output ""
    Write-ColorOutput $BLUE "🚀 Printer Agent - Sistema Industrial Labels"
    Write-ColorOutput $BLUE "🌐 barbosasystem.tech"
    Write-Output ""
}

# Função principal
function Main {
    Show-Banner

    if ($Help) {
        Show-Help
        return
    }

    if ($Clean) {
        Clean-BuildFiles
        return
    }

    if ($Uninstall) {
        Check-AdminRights
        Uninstall-Service
        return
    }

    # Verificar Go
    Check-GoInstalled

    if ($Install) {
        Check-AdminRights
        Test-Port
        $exePath = Build-Agent -isService $true
        $fullPath = Join-Path (Get-Location) $exePath
        Install-Service $fullPath
        Get-WindowsPrinters
        return
    }

    if ($Service) {
        $exePath = Build-Agent -isService $true
        Log-Info "✅ Build para serviço concluído: $exePath"
        Log-Info "Execute com -Install para instalar como serviço"
        return
    }

    if ($Console) {
        Test-Port
        $exePath = Build-Agent -isService $false
        Get-WindowsPrinters
        Run-Console $exePath
        return
    }

    # Menu interativo se nenhum parâmetro foi fornecido
    do {
        Write-Output ""
        Write-Output "🖨️  Industrial Labels Printer Agent"
        Write-Output "════════════════════════════════════════"
        Write-Output "1) Build e executar em modo console"
        Write-Output "2) Build para serviço"
        Write-Output "3) Instalar como serviço do Windows"
        Write-Output "4) Remover serviço"
        Write-Output "5) Listar impressoras"
        Write-Output "6) Limpar arquivos de build"
        Write-Output "7) Sair"
        Write-Output ""
        $choice = Read-Host "Escolha uma opção [1-7]"

        switch ($choice) {
            "1" {
                Test-Port
                $exePath = Build-Agent -isService $false
                Get-WindowsPrinters
                Run-Console $exePath
                break
            }
            "2" {
                $exePath = Build-Agent -isService $true
                Log-Info "✅ Build para serviço concluído: $exePath"
                break
            }
            "3" {
                Check-AdminRights
                Test-Port
                $exePath = Build-Agent -isService $true
                $fullPath = Join-Path (Get-Location) $exePath
                Install-Service $fullPath
                Get-WindowsPrinters
                break
            }
            "4" {
                Check-AdminRights
                Uninstall-Service
                break
            }
            "5" {
                Get-WindowsPrinters
                break
            }
            "6" {
                Clean-BuildFiles
                break
            }
            "7" {
                Log-Info "Saindo..."
                return
            }
            default {
                Log-Error "Opção inválida!"
            }
        }
    } while ($choice -ne "7")
}

# Executar
Main
