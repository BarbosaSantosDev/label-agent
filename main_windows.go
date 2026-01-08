//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
)

/*
Endpoints:
GET  /health
GET  /printers
GET  /config
POST /config/printer
POST /printer/connect
POST /printer/disconnect
POST /print
*/

const (
	serviceName    = "LabelAgent"
	listenAddr     = "0.0.0.0:7777" // Aceita conexões de qualquer IP
	MAX_COPIES     = 50             // Limite máximo de cópias
	DEFAULT_COPIES = 1              // Valor padrão
)

type Config struct {
	PrinterName string `json:"printer_name"`
}

type app struct {
	mu     sync.RWMutex
	cfg    Config
	cfgDir string
	logger *log.Logger
}

/* =========================
   Windows service bootstrap
========================= */

func main() {
	isInt, err := svc.IsWindowsService()
	if err != nil {
		runConsole()
		return
	}

	if isInt {
		runService()
	} else {
		runConsole()
	}
}

func runService() {
	elog, err := eventlog.Open(serviceName)
	if err != nil {
		runConsole()
		return
	}
	defer elog.Close()

	a := newApp(elog)
	elog.Info(1, "LabelAgent starting")

	_ = svc.Run(serviceName, &winService{app: a, elog: elog})
}

func runConsole() {
	a := newApp(nil)

	_, err := a.startHTTP()
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("🖨️  LabelAgent rodando no Windows (Console Mode) em %s", listenAddr)
	log.Printf("🌐 Aceitando conexões de: https://barbosasystem.tech")
	log.Printf("📁 Configurações em: %s", a.cfgDir)
	log.Printf("⚠️  Para uso em produção, instale como serviço do Windows")
	log.Printf("💡 Execute: sc create LabelAgent binPath=\"%s\"", os.Args[0])

	// Para modo console, manter vivo sem service
	select {}
}

type winService struct {
	app  *app
	elog *eventlog.Log
}

func (s *winService) Execute(args []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}

	server, err := s.app.startHTTP()
	if err != nil {
		if s.elog != nil {
			s.elog.Error(1, fmt.Sprintf("Erro ao iniciar HTTP: %v", err))
		}
		status <- svc.Status{State: svc.Stopped}
		return false, 1
	}

	if s.elog != nil {
		s.elog.Info(1, fmt.Sprintf("🖨️ LabelAgent iniciado como serviço Windows em %s", listenAddr))
		s.elog.Info(1, "🌐 Aceitando conexões de: https://barbosasystem.tech")
		s.elog.Info(1, "🔄 Serviço configurado para inicialização automática")
	}

	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for c := range r {
		if c.Cmd == svc.Stop || c.Cmd == svc.Shutdown {
			if s.elog != nil {
				s.elog.Info(1, "🛑 LabelAgent parando...")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = server.Shutdown(ctx)
			cancel()
			if s.elog != nil {
				s.elog.Info(1, "✅ LabelAgent parado com sucesso")
			}
			status <- svc.Status{State: svc.Stopped}
			return false, 0
		}
	}
	return false, 0
}

/* =========================
   App / Config
========================= */

func newApp(elog *eventlog.Log) *app {
	cfgDir := defaultConfigDir()
	_ = os.MkdirAll(cfgDir, 0755)

	logFile := filepath.Join(cfgDir, "label-agent.log")
	f, _ := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)

	var out io.Writer = os.Stdout
	if f != nil {
		out = io.MultiWriter(os.Stdout, f)
	}

	a := &app{
		cfgDir: cfgDir,
		logger: log.New(out, "label-agent: ", log.LstdFlags),
	}

	_ = a.loadConfig()
	return a
}

func defaultConfigDir() string {
	pd := os.Getenv("ProgramData")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	return filepath.Join(pd, "LabelAgent")
}

func (a *app) configPath() string {
	return filepath.Join(a.cfgDir, "config.json")
}

func (a *app) loadConfig() error {
	b, err := os.ReadFile(a.configPath())
	if err != nil {
		return nil
	}
	return json.Unmarshal(b, &a.cfg)
}

func (a *app) saveConfig() {
	b, _ := json.MarshalIndent(a.cfg, "", "  ")
	_ = os.WriteFile(a.configPath(), b, 0644)
}

/* =========================
   HTTP + CORS
========================= */

func (a *app) startHTTP() (*http.Server, error) {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", a.withCORS(a.handleHealth))
	mux.HandleFunc("/printers", a.withCORS(a.handlePrinters))
	mux.HandleFunc("/config", a.withCORS(a.handleGetConfig))
	mux.HandleFunc("/config/printer", a.withCORS(a.handleSetPrinter))

	// 🔥 NOVAS ROTAS (resolvem seu CORS)
	mux.HandleFunc("/printer/connect", a.withCORS(a.handleConnectPrinter))
	mux.HandleFunc("/printer/disconnect", a.withCORS(a.handleDisconnectPrinter))

	mux.HandleFunc("/print", a.withCORS(a.handlePrint))

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, err
	}

	srv := &http.Server{Handler: mux}
	go srv.Serve(ln)
	return srv, nil
}

func (a *app) withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Permitir origens específicas para produção
		allowedOrigins := []string{
			"https://barbosasystem.tech",
			"https://www.barbosasystem.tech",
			"http://localhost:8080",
			"http://localhost:3000",
		}

		// Verificar se a origem está na lista permitida ou se é localhost
		isAllowed := false
		if origin != "" {
			for _, allowed := range allowedOrigins {
				if origin == allowed {
					isAllowed = true
					break
				}
			}
			// Permitir qualquer origem localhost para desenvolvimento
			if strings.Contains(origin, "localhost") || strings.Contains(origin, "127.0.0.1") {
				isAllowed = true
			}
		}

		if isAllowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}

		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS,PUT,DELETE")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		if r.Header.Get("Access-Control-Request-Private-Network") == "true" {
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
			w.Header().Add("Vary", "Access-Control-Request-Private-Network")
		}

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

/* =========================
   Handlers
========================= */

func (a *app) handleHealth(w http.ResponseWriter, r *http.Request) {
	a.logger.Printf("Health check from: %s", r.RemoteAddr)

	// Verificar se está rodando como serviço
	isService, _ := svc.IsWindowsService()

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"service":    serviceName,
		"version":    "1.0.0",
		"address":    listenAddr,
		"platform":   "windows",
		"running_as": map[string]bool{"service": isService, "console": !isService},
		"auto_start": isService,
	})
}

func (a *app) handleGetConfig(w http.ResponseWriter, _ *http.Request) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	writeJSON(w, http.StatusOK, a.cfg)
}

func (a *app) handleSetPrinter(w http.ResponseWriter, r *http.Request) {
	var in Config
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.PrinterName == "" {
		writeJSON(w, 400, map[string]string{"error": "invalid printer_name"})
		return
	}
	a.mu.Lock()
	a.cfg.PrinterName = in.PrinterName
	a.saveConfig()
	a.mu.Unlock()
	writeJSON(w, 200, map[string]any{"ok": true})
}

func (a *app) handleConnectPrinter(w http.ResponseWriter, r *http.Request) {
	a.handleSetPrinter(w, r)
}

func (a *app) handleDisconnectPrinter(w http.ResponseWriter, _ *http.Request) {
	a.mu.Lock()
	a.cfg.PrinterName = ""
	a.saveConfig()
	a.mu.Unlock()
	writeJSON(w, 200, map[string]any{"connected": false})
}

func (a *app) handlePrinters(w http.ResponseWriter, _ *http.Request) {
	printers, err := listPrintersPowerShell()
	if err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"printers": printers})
}

func (a *app) handlePrint(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PrinterName string   `json:"printer_name"`
		ZPLs        []string `json:"zpls"`
		Copies      int      `json:"copies"`
	}

	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, 400, map[string]string{"error": "invalid json"})
		return
	}

	if len(in.ZPLs) == 0 {
		writeJSON(w, 400, map[string]string{"error": "no zpls provided"})
		return
	}

	// VALIDAÇÃO DO NÚMERO DE CÓPIAS
	if in.Copies <= 0 {
		in.Copies = DEFAULT_COPIES
	}

	if in.Copies > MAX_COPIES {
		writeJSON(w, 400, map[string]string{
			"error": fmt.Sprintf("número de cópias excede o limite máximo de %d", MAX_COPIES),
		})
		return
	}

	printer := in.PrinterName
	if printer == "" {
		printer = a.cfg.PrinterName
	}
	if printer == "" {
		writeJSON(w, 400, map[string]string{"error": "printer not set"})
		return
	}

	// LOG de impressão
	a.logger.Printf("🖨️  Imprimindo %d cópias de %d etiqueta(s) na impressora '%s' (origem: %s)",
		in.Copies, len(in.ZPLs), printer, r.RemoteAddr)

	// IMPRESSÃO COM MÚLTIPLAS CÓPIAS
	totalPrinted := 0
	var lastError error

	for copyNum := 0; copyNum < in.Copies; copyNum++ {
		for i, zpl := range in.ZPLs {
			if err := printRawZPL(printer, zpl); err != nil {
				lastError = err
				a.logger.Printf("Erro na cópia %d, etiqueta %d: %v", copyNum+1, i+1, err)

				writeJSON(w, 500, map[string]any{
					"error":           lastError.Error(),
					"index":           i,
					"copy_number":     copyNum + 1,
					"total_copies":    in.Copies,
					"printed_success": totalPrinted,
					"expected_total":  in.Copies * len(in.ZPLs),
					"printer":         printer,
				})
				return
			}
			totalPrinted++
		}

		// Log de progresso a cada 10 cópias
		if (copyNum+1)%10 == 0 {
			a.logger.Printf("Progresso: %d/%d cópias concluídas", copyNum+1, in.Copies)
		}
	}

	// RESPOSTA DE SUCESSO
	a.logger.Printf("✅ Impressão concluída: %d etiquetas impressas com sucesso", totalPrinted)

	writeJSON(w, 200, map[string]any{
		"ok":                 true,
		"printed":            totalPrinted,
		"copies":             in.Copies,
		"labels_per_copy":    len(in.ZPLs),
		"printer":            printer,
		"max_copies_allowed": MAX_COPIES,
	})
}

/* =========================
   Utils
========================= */

func writeJSON(w http.ResponseWriter, s int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(s)
	_ = json.NewEncoder(w).Encode(v)
}

func listPrintersPowerShell() ([]string, error) {
	ps := `Get-Printer | Select -Expand Name | ConvertTo-Json`
	out, err := exec.Command("powershell", "-NoProfile", "-Command", ps).Output()
	if err != nil {
		return nil, err
	}

	var list []string
	if json.Unmarshal(out, &list) == nil {
		return list, nil
	}

	var one string
	if json.Unmarshal(out, &one) == nil {
		return []string{one}, nil
	}

	return []string{}, nil
}

/* =========================
   RAW ZPL printing
========================= */

var (
	winspool     = syscall.NewLazyDLL("winspool.drv")
	openPrinter  = winspool.NewProc("OpenPrinterW")
	closePrinter = winspool.NewProc("ClosePrinter")
	startDoc     = winspool.NewProc("StartDocPrinterW")
	endDoc       = winspool.NewProc("EndDocPrinter")
	startPage    = winspool.NewProc("StartPagePrinter")
	endPage      = winspool.NewProc("EndPagePrinter")
	writePrinter = winspool.NewProc("WritePrinter")
)

type docInfo1 struct {
	pDocName    *uint16
	pOutputFile *uint16
	pDatatype   *uint16
}

func printRawZPL(printerName, zpl string) error {
	p, _ := syscall.UTF16PtrFromString(printerName)
	var h syscall.Handle
	r1, _, err := openPrinter.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&h)), 0)
	if r1 == 0 {
		return fmt.Errorf("OpenPrinter failed: %v", err)
	}
	defer closePrinter.Call(uintptr(h))

	doc, _ := syscall.UTF16PtrFromString("LabelAgent")
	raw, _ := syscall.UTF16PtrFromString("RAW")
	di := docInfo1{doc, nil, raw}

	r1, _, err = startDoc.Call(uintptr(h), 1, uintptr(unsafe.Pointer(&di)))
	if r1 == 0 {
		return fmt.Errorf("StartDocPrinter failed: %v", err)
	}
	defer endDoc.Call(uintptr(h))

	startPage.Call(uintptr(h))
	defer endPage.Call(uintptr(h))

	b := []byte(zpl)
	var written uint32
	r1, _, err = writePrinter.Call(uintptr(h), uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)), uintptr(unsafe.Pointer(&written)))
	if r1 == 0 {
		return fmt.Errorf("WritePrinter failed: %v", err)
	}

	return nil
}
