@echo off
:: Instalador LabelAgent - Sistema Industrial Labels
:: barbosasystem.tech

title LabelAgent - Instalador

echo.
echo ========================================
echo  LabelAgent - Sistema Industrial Labels
echo  barbosasystem.tech
echo ========================================
echo.

:: Verificar se esta executando como administrador
net session >nul 2>&1
if %errorLevel% == 0 (
    echo [OK] Executando como Administrador
) else (
    echo [ERRO] Este instalador precisa ser executado como Administrador!
    echo.
    echo Como executar como Administrador:
    echo 1. Clique com botao direito neste arquivo
    echo 2. Selecione "Executar como administrador"
    echo.
    pause
    exit /b 1
)

:: Verificar se o executavel existe
if not exist "label-agent.exe" (
    echo [ERRO] Arquivo label-agent.exe nao encontrado!
    echo Certifique-se de que todos os arquivos estao na mesma pasta.
    echo.
    pause
    exit /b 1
)

echo [OK] Arquivo label-agent.exe encontrado
echo.

:: Executar o script PowerShell
echo Iniciando instalacao...
echo.

powershell -ExecutionPolicy Bypass -File "install.ps1"

if %errorLevel% == 0 (
    echo.
    echo ========================================
    echo  INSTALACAO CONCLUIDA!
    echo ========================================
    echo.
    echo O LabelAgent agora esta rodando como servico do Windows
    echo e iniciara automaticamente quando ligar o computador.
    echo.
    echo Acesse: https://barbosasystem.tech
    echo.
) else (
    echo.
    echo ========================================
    echo  ERRO NA INSTALACAO
    echo ========================================
    echo.
    echo Verifique os erros acima e tente novamente.
    echo.
)

pause
