# 🖨️ Printer Agent - Sistema Industrial Labels

Agent local para comunicação entre o sistema web e impressoras de etiquetas ZPL.

## 🌐 Compatibilidade

- **Sistema Web**: https://barbosasystem.tech
- **Protocolo**: ZPL (Zebra Programming Language)
- **Plataforma**: Linux (Ubuntu/Debian)

## 📋 Pré-requisitos

### Sistema Operacional
```bash
# Ubuntu/Debian
sudo apt update
sudo apt install cups cups-client golang-go
```

### Impressora Configurada
- Impressora deve estar instalada e funcionando no sistema
- Testar com: `lpstat -p`

## 🚀 Instalação e Uso

### Método 1: Script Automático (Recomendado)
```bash
# Executar script interativo
./run.sh

# Ou comandos diretos
./run.sh build    # Apenas build
./run.sh run      # Apenas executar
./run.sh service  # Criar serviço systemd
```

### Método 2: Manual
```bash
# Build
go build -o printer-agent .

# Executar
./printer-agent
```

## 🔧 Configuração

### Modificações Principais
- ✅ **Bind Address**: `0.0.0.0:7777` (aceita conexões externas)
- ✅ **CORS**: Configurado para `barbosasystem.tech`
- ✅ **Logs melhorados**: Com emojis e informações detalhadas
- ✅ **Health check expandido**: Mais informações de debug

### Origens Permitidas
- `https://barbosasystem.tech`
- `https://www.barbosasystem.tech`
- `http://localhost:*` (desenvolvimento)

## 📡 API Endpoints

| Método | Endpoint | Descrição |
|--------|----------|-----------|
| GET | `/health` | Status do agent |
| GET | `/printers` | Lista impressoras disponíveis |
| GET | `/config` | Configuração atual |
| POST | `/config/printer` | Definir impressora |
| POST | `/printer/connect` | Conectar impressora |
| POST | `/printer/disconnect` | Desconectar impressora |
| POST | `/print` | Imprimir etiquetas ZPL |

## 🧪 Testando

### Health Check
```bash
curl http://localhost:7777/health
```

### Listar Impressoras
```bash
curl http://localhost:7777/printers
```

### Imprimir Teste
```bash
curl -X POST http://localhost:7777/print \
  -H "Content-Type: application/json" \
  -d '{
    "printer_name": "ZTC-ZD220-203dpi-ZPL",
    "zpls": ["^XA^LH0,0^FO50,50^ADN,18,10^FDTeste^FS^XZ"],
    "copies": 1
  }'
```

## 🔒 Segurança

### Firewall (se necessário)
```bash
# Permitir porta 7777 apenas para localhost
sudo ufw allow from 127.0.0.1 to any port 7777

# Ou para rede local (ex: 192.168.1.0/24)
sudo ufw allow from 192.168.1.0/24 to any port 7777
```

### Bloqueador de Anúncios
Se o navegador bloquear conexões localhost:
1. Desabilite o bloqueador para `barbosasystem.tech`
2. Ou adicione exceção para `localhost:7777`

## 🛠️ Solução de Problemas

### Agent não inicia
```bash
# Verificar se porta está livre
sudo netstat -tlnp | grep :7777

# Verificar permissões
ls -la printer-agent
```

### Erro de impressão
```bash
# Testar impressora manualmente
echo "Teste" | lp -d NOME_DA_IMPRESSORA

# Verificar status CUPS
systemctl status cups
lpstat -p
```

### Conexão negada do navegador
1. **Verificar Origin**: Agent logará tentativas de conexão
2. **CORS**: Verificar se domínio está na lista permitida
3. **HTTPS → HTTP**: Navegador pode bloquear Mixed Content

### Logs do Agent
```bash
# Logs em tempo real
tail -f ~/.label-agent/label-agent.log

# Se usando systemd
journalctl -u printer-agent -f
```

## 📁 Estrutura de Arquivos

```
printer-agent/
├── main_linux.go          # Código principal
├── run.sh                  # Script de build/execução
├── README.md              # Este arquivo
├── printer-agent          # Binário (após build)
└── ~/.label-agent/        # Configurações e logs
    ├── config.json        # Configuração da impressora
    └── label-agent.log    # Arquivo de log
```

## 🔄 Como Funciona

1. **Sistema Web** (`barbosasystem.tech`) gera etiquetas ZPL
2. **JavaScript** tenta conectar no agent local (`localhost:7777`)
3. **Agent** recebe comando de impressão
4. **CUPS** envia ZPL para a impressora física
5. **Impressora** imprime as etiquetas

## 🎯 Deploy para Clientes

### Distribuição
1. Compilar para Linux: `GOOS=linux go build`
2. Empacotar com script de instalação
3. Cliente executa em máquina com impressora

### Instalação Cliente
```bash
# Download e extração
tar -xzf printer-agent-linux.tar.gz
cd printer-agent

# Execução
./run.sh
```

## 📞 Suporte

- **Sistema funcionando**: Agent faz logs detalhados
- **Simulação automática**: Se agent não conectar, sistema simula impressão
- **Logs centralizados**: Tudo registrado em `~/.label-agent/label-agent.log`

## 🔗 Links Úteis

- **Sistema Web**: https://barbosasystem.tech
- **Documentação ZPL**: [Zebra Programming Guide]
- **CUPS**: https://www.cups.org/

---

**Sistema Industrial Labels v1.0**
  
🌐 barbosasystem.tech