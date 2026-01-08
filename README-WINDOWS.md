# 🖨️ Printer Agent para Windows - Industrial Labels

**Serviço automático para impressoras Zebra ZPL**

Sistema: https://barbosasystem.tech

---

## 🎯 Características

✅ **Serviço Windows** - Inicia automaticamente com o sistema  
✅ **Zero configuração** - Instalação em um clique  
✅ **Compatível** - Funciona com qualquer impressora Zebra  
✅ **Seguro** - Aceita conexões apenas do sistema autorizado  
✅ **Logs completos** - Registra tudo no Event Viewer  

---

## 📋 Requisitos

- **Windows 10/11** ou Windows Server 2019+
- **Impressora Zebra** instalada no sistema
- **PowerShell 5.1+** (já incluso no Windows)
- **Privilégios de Administrador** para instalação

---

## 🚀 Instalação (Cliente Final)

### Passo 1: Download
Extraia os arquivos para uma pasta (ex: `C:\LabelAgent\`)

### Passo 2: Executar como Administrador
1. Clique com botão direito no **PowerShell**
2. Selecione **"Executar como administrador"**
3. Navegue até a pasta: `cd C:\LabelAgent\`
4. Execute: `.\install.ps1`

### Passo 3: Confirmar Instalação
- O script mostrará suas impressoras disponíveis
- Confirme com **Y** para instalar
- Aguarde a conclusão

### ✅ Pronto!
O serviço estará rodando automaticamente e iniciará sempre que ligar o computador.

---

## 🔧 Comandos Úteis

```powershell
# Verificar status do serviço
.\install.ps1 -Status

# Parar o serviço
.\install.ps1 -Stop

# Iniciar o serviço
.\install.ps1 -Start

# Desinstalar completamente
.\install.ps1 -Uninstall
```

**Ou usando comandos nativos do Windows:**
```powershell
# Status
Get-Service LabelAgent

# Iniciar/Parar
Start-Service LabelAgent
Stop-Service LabelAgent

# Testar conectividade
Invoke-WebRequest http://localhost:7777/health
```

---

## 🧪 Como Testar

### 1. Verificar se está rodando
```powershell
Get-Service LabelAgent
```
**Status deve ser:** `Running`

### 2. Testar API
```powershell
Invoke-WebRequest http://localhost:7777/health
```
**Resposta deve conter:** `"ok": true`

### 3. Testar no navegador
- Acesse: https://barbosasystem.tech
- Faça login no sistema
- Crie uma etiqueta de teste
- Deve conectar automaticamente na impressora

---

## 🖨️ Configuração de Impressoras

### Impressoras Suportadas
- **Zebra ZPL** - Todas as impressoras que suportam comandos ZPL
- **Modelos testados**: ZD220, ZD420, GX420d, GK420d

### Configuração Recomendada
1. **Instale a impressora** via Painel de Controle
2. **Configure como impressora padrão** (opcional)
3. **Teste uma impressão** antes de usar o sistema

### Verificar Drivers
```powershell
Get-Printer | Select Name, DriverName
```

---

## 🔍 Solução de Problemas

### Serviço não inicia
```powershell
# Verificar logs no Event Viewer
Get-EventLog -LogName Application -Source LabelAgent -Newest 10

# Ou executar em modo console para debug
.\label-agent.exe
```

### Erro de porta ocupada
```powershell
# Verificar que processo está usando a porta 7777
netstat -ano | findstr :7777

# Matar processo se necessário
taskkill /PID [NUMERO_DO_PID] /F
```

### Impressora não encontrada
```powershell
# Listar impressoras disponíveis
Get-Printer

# Testar impressão manual
echo "Teste" | Out-Printer -Name "Nome_da_Impressora"
```

### Sistema web não conecta
1. **Verifique se o serviço está rodando**
2. **Desabilite bloqueador de anúncios** no navegador para `barbosasystem.tech`
3. **Teste a URL**: http://localhost:7777/health
4. **Verifique firewall do Windows**

---

## 🛡️ Segurança

### Firewall
O instalador configura automaticamente o Windows Firewall para permitir conexões na porta 7777.

### CORS
O agent aceita conexões apenas de:
- `https://barbosasystem.tech`
- `https://www.barbosasystem.tech`
- `localhost` (para desenvolvimento)

### Logs
Todas as atividades são registradas no **Event Viewer** do Windows:
- Caminho: `Windows Logs > Application`
- Source: `LabelAgent`

---

## 📁 Estrutura de Arquivos

```
C:\LabelAgent\
├── label-agent.exe       # Executável principal
├── install.ps1          # Script de instalação
└── README-WINDOWS.md    # Este arquivo

%ProgramData%\LabelAgent\
├── config.json          # Configurações da impressora
└── label-agent.log      # Arquivo de log local
```

---

## 🔄 Atualizações

Para atualizar o agent:

1. **Pare o serviço**:
   ```powershell
   .\install.ps1 -Stop
   ```

2. **Substitua o arquivo** `label-agent.exe`

3. **Inicie o serviço**:
   ```powershell
   .\install.ps1 -Start
   ```

---

## 🏢 Deploy Empresarial

### Instalação Silenciosa (IT/Admin)
```powershell
# Via GPO ou script corporativo
powershell -ExecutionPolicy Bypass -File "C:\Deploy\install.ps1"
```

### Instalação em Múltiplas Máquinas
```powershell
# Script para múltiplos computadores
$computers = @("PC001", "PC002", "PC003")

foreach ($pc in $computers) {
    Invoke-Command -ComputerName $pc -FilePath "install.ps1"
}
```

---

## 📊 Monitoramento

### Via PowerShell
```powershell
# Status de múltiplas máquinas
$computers = @("PC001", "PC002")
Invoke-Command -ComputerName $computers -ScriptBlock {
    Get-Service LabelAgent | Select PSComputerName, Status
}
```

### Via Event Viewer
- **Caminho**: `Applications and Services Logs > LabelAgent`
- **Eventos importantes**:
  - **1001**: Serviço iniciado
  - **1002**: Impressão realizada
  - **1003**: Erro de impressão

---

## 🆘 Suporte

### Informações para Suporte
Ao contatar o suporte, forneça:

1. **Status do serviço**:
   ```powershell
   Get-Service LabelAgent | Format-List *
   ```

2. **Logs recentes**:
   ```powershell
   Get-EventLog -LogName Application -Source LabelAgent -Newest 5
   ```

3. **Informações da impressora**:
   ```powershell
   Get-Printer | Where-Object {$_.Name -like "*Zebra*"}
   ```

### Contato
- **Sistema**: https://barbosasystem.tech
- **Documentação**: README.md
- **Logs**: Event Viewer > Application > LabelAgent

---

## 🎯 FAQ

**P: O serviço consome muitos recursos?**  
R: Não, o agent usa menos de 10MB RAM e praticamente zero CPU quando inativo.

**P: Posso usar em Terminal Server/RDS?**  
R: Sim, mas instale apenas na sessão que tem acesso à impressora.

**P: Funciona com impressoras de rede?**  
R: Sim, desde que a impressora esteja instalada como local no Windows.

**P: Preciso reiniciar após instalar?**  
R: Não, o serviço inicia imediatamente após a instalação.

**P: Como desinstalar completamente?**  
R: Execute `.\install.ps1 -Uninstall` e delete a pasta.

---

**🖨️ Sistema Industrial Labels v1.0**  
**🌐 barbosasystem.tech**

*Serviço Windows para impressão automática de etiquetas ZPL*