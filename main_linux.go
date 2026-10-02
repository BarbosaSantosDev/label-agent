//go:build linux

package main

import (
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

// version é definida no build via -ldflags "-X main.version=..."
var version = "dev"

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
   Main (Linux)
========================= */

func main() {
	a := newApp()

	_, err := a.startHTTP()
	if err != nil {
		log.Fatal(err)
	}

	log.Printf("🖨️  LabelAgent rodando no Linux em %s", listenAddr)
	log.Printf("🌐 Aceitando conexões de: https://label.barbosasystem.tech")
	log.Printf("📁 Configurações em: %s", a.cfgDir)

	// mantém o processo vivo
	select {}
}

/* =========================
   App / Config
========================= */

func newApp() *app {
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
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".label-agent")
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
			"https://label.barbosasystem.tech",
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
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": serviceName,
		"version": version,
		"address": listenAddr,
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
	printers, err := listPrintersCUPS()
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

/* =========================
   CUPS
========================= */

func listPrintersCUPS() ([]string, error) {
	out, err := exec.Command("lpstat", "-p").Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(out), "\n")
	var printers []string

	for _, l := range lines {
		if strings.HasPrefix(l, "printer ") {
			parts := strings.Fields(l)
			if len(parts) >= 2 {
				printers = append(printers, parts[1])
			}
		}
	}

	return printers, nil
}

func printRawZPL(printerName, zpl string) error {
	cmd := exec.Command("lp", "-d", printerName, "-o", "raw")
	cmd.Stdin = strings.NewReader(zpl)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("lp failed: %v | %s", err, string(out))
	}
	return nil
}
