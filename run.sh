#!/bin/bash

# Script de Build e Execução - Printer Agent
# Sistema Industrial Labels - barbosasystem.tech

set -e

# Cores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Função para log colorido
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

# Verificar se Go está instalado
check_go() {
    if ! command -v go &> /dev/null; then
        log_error "Go não está instalado!"
        echo "Instale o Go: https://golang.org/dl/"
        exit 1
    fi
    log_info "Go $(go version | cut -d' ' -f3) encontrado"
}

# Build do agent
build_agent() {
    log_step "Fazendo build do printer agent..."

    # Definir variáveis de build
    export CGO_ENABLED=0
    export GOOS=linux
    export GOARCH=amd64

    # Build
    go build -ldflags="-s -w" -o printer-agent .

    if [ $? -eq 0 ]; then
        log_info "✅ Build concluído com sucesso!"
        chmod +x printer-agent
    else
        log_error "❌ Erro no build!"
        exit 1
    fi
}

# Verificar portas
check_ports() {
    log_step "Verificando se a porta 7777 está disponível..."

    if lsof -i:7777 > /dev/null 2>&1; then
        log_warn "Porta 7777 já está em uso!"
        echo -n "Deseja continuar mesmo assim? [y/N]: "
        read -r response
        if [[ ! "$response" =~ ^[Yy]$ ]]; then
            log_info "Operação cancelada."
            exit 0
        fi
    else
        log_info "Porta 7777 disponível"
    fi
}

# Verificar dependências do sistema
check_dependencies() {
    log_step "Verificando dependências do sistema..."

    # Verificar CUPS
    if ! command -v lpstat &> /dev/null; then
        log_error "CUPS não está instalado!"
        echo "Instale com: sudo apt install cups cups-client"
        exit 1
    fi

    if ! command -v lp &> /dev/null; then
        log_error "Comando 'lp' não encontrado!"
        echo "Instale com: sudo apt install cups cups-client"
        exit 1
    fi

    log_info "✅ Dependências verificadas"
}

# Listar impressoras disponíveis
list_printers() {
    log_step "Impressoras disponíveis:"

    printers=$(lpstat -p 2>/dev/null | awk '/^printer/ {print "  - " $2}')

    if [ -z "$printers" ]; then
        log_warn "Nenhuma impressora encontrada"
        log_info "Configure uma impressora com: sudo system-config-printer"
    else
        echo "$printers"
    fi
}

# Executar o agent
run_agent() {
    log_step "Iniciando Printer Agent..."
    log_info "🌐 Será acessível em: http://0.0.0.0:7777"
    log_info "🖨️  Compatível com: https://barbosasystem.tech"
    log_info "📁 Logs serão salvos em: ~/.label-agent/"
    echo ""
    log_info "Para parar, pressione Ctrl+C"
    echo ""

    # Executar
    ./printer-agent
}

# Criar service systemd (opcional)
create_service() {
    log_step "Criando serviço systemd..."

    current_dir=$(pwd)
    current_user=$(whoami)

    sudo tee /etc/systemd/system/printer-agent.service > /dev/null << EOF
[Unit]
Description=Industrial Labels Printer Agent
After=network.target cups.service
Wants=cups.service

[Service]
Type=simple
User=$current_user
WorkingDirectory=$current_dir
ExecStart=$current_dir/printer-agent
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF

    sudo systemctl daemon-reload
    sudo systemctl enable printer-agent

    log_info "✅ Serviço criado! Use:"
    echo "  sudo systemctl start printer-agent    # Iniciar"
    echo "  sudo systemctl stop printer-agent     # Parar"
    echo "  sudo systemctl status printer-agent   # Status"
    echo "  journalctl -u printer-agent -f        # Ver logs"
}

# Menu principal
show_menu() {
    echo ""
    echo "🖨️  Industrial Labels Printer Agent"
    echo "════════════════════════════════════════"
    echo "1) Build e executar"
    echo "2) Apenas build"
    echo "3) Apenas executar"
    echo "4) Listar impressoras"
    echo "5) Criar serviço systemd"
    echo "6) Verificar dependências"
    echo "7) Sair"
    echo ""
    echo -n "Escolha uma opção [1-7]: "
}

# Função principal
main() {
    # Banner
    echo ""
    echo "🚀 Printer Agent - Sistema Industrial Labels"
    echo "🌐 barbosasystem.tech"
    echo ""

    # Verificar Go
    check_go

    # Menu
    if [ $# -eq 0 ]; then
        while true; do
            show_menu
            read -r choice

            case $choice in
                1)
                    check_dependencies
                    check_ports
                    build_agent
                    list_printers
                    run_agent
                    break
                    ;;
                2)
                    build_agent
                    ;;
                3)
                    check_dependencies
                    check_ports
                    list_printers
                    run_agent
                    break
                    ;;
                4)
                    list_printers
                    ;;
                5)
                    build_agent
                    create_service
                    ;;
                6)
                    check_dependencies
                    list_printers
                    ;;
                7)
                    log_info "Saindo..."
                    exit 0
                    ;;
                *)
                    log_error "Opção inválida!"
                    ;;
            esac
        done
    else
        # Argumentos da linha de comando
        case $1 in
            build)
                build_agent
                ;;
            run)
                check_dependencies
                check_ports
                list_printers
                run_agent
                ;;
            service)
                build_agent
                create_service
                ;;
            *)
                echo "Uso: $0 [build|run|service]"
                exit 1
                ;;
        esac
    fi
}

# Executar
main "$@"
