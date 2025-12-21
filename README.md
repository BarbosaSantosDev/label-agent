

````md
# 🖨️ Label Agent

Agente local responsável por receber comandos via HTTP (`localhost:7777`) e enviar etiquetas ZPL para impressoras Zebra no Windows.

---

## ✅ Requisitos

- Windows 10 ou superior  
- Impressora Zebra instalada no sistema  
- PowerShell (já vem instalado no Windows)

---

## 🚀 Instalação

### 1. Baixe os arquivos do projeto e extraia na pasta:

```text
C:\Program Files\printer-agent-main
````

---

## ▶️ Como rodar o `install.ps1` no Windows

Se você clicar duas vezes no `install.ps1`, o Windows pode abrir o Bloco de Notas. Para executar corretamente:

### Passo 1: Abrir o PowerShell como Administrador

1. Clique no botão **Iniciar**
2. Digite `PowerShell`
3. Clique com o botão direito em **Windows PowerShell**
4. Clique em **Executar como administrador**

---

### Passo 2: Navegar até a pasta do projeto

```powershell
cd "C:\Program Files\printer-agent-main"
```

---

### Passo 3: Rodar o script de instalação

```powershell
Set-ExecutionPolicy Bypass -Scope Process -Force; ./install.ps1
```

---

### 📌 Isso irá:

* Registrar o `label-agent.exe` como um **serviço do Windows**
* Iniciar o agente automaticamente na porta `localhost:7777`

---

## ✅ Verificar se o agente está rodando

Abra o navegador e acesse:

```
http://localhost:7777/health
```

Se estiver funcionando, a resposta será:

```json
{"status":"ok"}
```

---

## ❌ Desinstalação

Para remover o agente do sistema:

### 1. Parar o serviço

```powershell
Stop-Service -Name LabelAgent
```

### 2. Remover o serviço

```powershell
sc.exe delete LabelAgent
```

### 3. (Opcional) Apagar a pasta

```text
C:\Program Files\printer-agent-main
```

---

## 🧪 Teste rápido de API

Você também pode testar via terminal:

```bash
curl http://localhost:7777/health
```

---

## 🛠️ Build manual do agente (dev)

Para gerar o executável manualmente:

```bash
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w -H=windowsgui" -o dist/label-agent.exe
```

---

## 📡 Endpoints disponíveis

| Método | Rota               | Descrição                       |
| ------ | ------------------ | ------------------------------- |
| GET    | `/health`          | Verifica se o agente está ativo |
| POST   | `/printer/connect` | Conecta à impressora padrão     |
| POST   | `/printer/print`   | Envia ZPL para a impressora     |

---

## 🧠 Observações

* A comunicação é feita **exclusivamente via `localhost`**
* O **Frontend precisa rodar no mesmo computador** que o agente para evitar erros de CORS
* Verifique se o firewall ou navegador não está bloqueando requisições para `127.0.0.1`

---

Se quiser, posso:

* 📄 Gerar esse README em **PDF**
* 📦 Empacotar junto com o `.exe`
* 🧾 Criar uma versão **simplificada para clientes finais**

É só falar 😉
