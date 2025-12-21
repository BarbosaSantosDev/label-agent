//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
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
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
)

/*
Endpoints:
GET  /health
GET  /printers
GET  /config
POST /config/printer   { "printer_name": "..." }
POST /print            { "printer_name": "...(optional)", "zpl": "^XA..." }
*/

const (
	serviceName = "LabelAgent"
	listenAddr  = "127.0.0.1:7777"
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

func main() {
	isInt, err := svc.IsWindowsService()
	if err != nil {
		// fallback: roda em console
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
		// sem eventlog, tenta console logger
		runConsole()
		return
	}
	defer elog.Close()

	a := newApp(elog)
	elog.Info(1, "LabelAgent starting...")

	err = svc.Run(serviceName, &winService{app: a, elog: elog})
	if err != nil {
		elog.Error(1, fmt.Sprintf("svc.Run error: %v", err))
	}
	elog.Info(1, "LabelAgent stopped.")
}

func runConsole() {
	// modo debug/console
	a := newApp(nil)
	_ = debug.Run(serviceName, &winService{app: a, elog: nil})
}

type winService struct {
	app  *app
	elog *eventlog.Log
}

func (m *winService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (bool, uint32) {
	const accepted = svc.AcceptStop | svc.AcceptShutdown
	changes <- svc.Status{State: svc.StartPending}

	srv, err := m.app.startHTTP()
	if err != nil {
		m.logErr(fmt.Sprintf("startHTTP failed: %v", err))
		changes <- svc.Status{State: svc.Stopped}
		return false, 1
	}

	changes <- svc.Status{State: svc.Running, Accepts: accepted}
	m.logInfo("HTTP server running on " + listenAddr)

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				m.logInfo("Stopping service...")
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				_ = srv.Shutdown(ctx)
				cancel()
				changes <- svc.Status{State: svc.StopPending}
				changes <- svc.Status{State: svc.Stopped}
				return false, 0
			default:
				// ignore
			}
		}
	}
}

func (m *winService) logInfo(msg string) {
	if m.elog != nil {
		_ = m.elog.Info(1, msg)
	} else {
		m.app.logger.Println("[INFO]", msg)
	}
}

func (m *winService) logErr(msg string) {
	if m.elog != nil {
		_ = m.elog.Error(1, msg)
	} else {
		m.app.logger.Println("[ERROR]", msg)
	}
}

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
		logger: log.New(out, "label-agent: ", log.LstdFlags|log.Lmicroseconds),
	}

	_ = a.loadConfig()
	return a
}

func defaultConfigDir() string {
	// C:\ProgramData\LabelAgent
	progData := os.Getenv("ProgramData")
	if progData == "" {
		progData = `C:\ProgramData`
	}
	return filepath.Join(progData, "LabelAgent")
}

func (a *app) configPath() string {
	return filepath.Join(a.cfgDir, "config.json")
}

func (a *app) loadConfig() error {
	p := a.configPath()
	b, err := os.ReadFile(p)
	if err != nil {
		return nil // ok se não existe
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return err
	}
	a.mu.Lock()
	a.cfg = c
	a.mu.Unlock()
	return nil
}

func (a *app) saveConfig() error {
	a.mu.RLock()
	c := a.cfg
	a.mu.RUnlock()

	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(a.configPath(), b, 0644)
}

func (a *app) startHTTP() (*http.Server, error) {
	mux := http.NewServeMux()

	mux.HandleFunc("/health", a.withCORS(a.handleHealth))
	mux.HandleFunc("/printers", a.withCORS(a.handlePrinters))
	mux.HandleFunc("/config", a.withCORS(a.handleGetConfig))
	mux.HandleFunc("/config/printer", a.withCORS(a.handleSetPrinter))
	mux.HandleFunc("/print", a.withCORS(a.handlePrint))

	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, err
	}

	srv := &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  30 * time.Second,
	}

	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			a.logger.Println("http serve error:", err)
		}
	}()

	return srv, nil
}

func (a *app) withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		// Melhor que "*": devolve o origin que chamou (site do seu SaaS)
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// ✅ Private Network Access (Chrome/Edge)
		// Se o browser mandar preflight pedindo acesso à rede privada, devolva true.
		if r.Header.Get("Access-Control-Request-Private-Network") == "true" {
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
		} else {
			// também pode devolver sempre, não atrapalha
			w.Header().Set("Access-Control-Allow-Private-Network", "true")
		}

		// Opcional: reduz preflights repetidos
		w.Header().Set("Access-Control-Max-Age", "86400")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next(w, r)
	}
}

func (a *app) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"service": serviceName,
	})
}

func (a *app) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	cfg := a.cfg
	a.mu.RUnlock()
	writeJSON(w, http.StatusOK, cfg)
}

func (a *app) handleSetPrinter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		PrinterName string `json:"printer_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	in.PrinterName = strings.TrimSpace(in.PrinterName)
	if in.PrinterName == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "printer_name is required"})
		return
	}

	a.mu.Lock()
	a.cfg.PrinterName = in.PrinterName
	a.mu.Unlock()
	_ = a.saveConfig()

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"printer_name": in.PrinterName,
	})
}

func (a *app) handlePrinters(w http.ResponseWriter, r *http.Request) {
	printers, err := listPrintersPowerShell()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"printers": printers})
}

func listPrintersPowerShell() ([]string, error) {
	// Get-Printer existe no Windows 10/11 (PowerShell)
	ps := `Get-Printer | Select-Object -ExpandProperty Name | ConvertTo-Json`
	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", ps)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("powershell error: %v: %s", err, string(out))
	}

	raw := strings.TrimSpace(string(out))
	if raw == "" || raw == "null" {
		return []string{}, nil
	}

	// ConvertTo-Json retorna string única ou array JSON
	var one string
	if err := json.Unmarshal([]byte(raw), &one); err == nil {
		if strings.TrimSpace(one) == "" {
			return []string{}, nil
		}
		return []string{one}, nil
	}

	var many []string
	if err := json.Unmarshal([]byte(raw), &many); err == nil {
		return many, nil
	}

	return nil, fmt.Errorf("could not parse printers json: %s", raw)
}

func (a *app) handlePrint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var in struct {
		PrinterName string `json:"printer_name"`
		ZPL         string `json:"zpl"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	in.ZPL = strings.TrimSpace(in.ZPL)
	if in.ZPL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "zpl is required"})
		return
	}

	// Usa printer enviada ou a salva no config
	printer := strings.TrimSpace(in.PrinterName)
	if printer == "" {
		a.mu.RLock()
		printer = strings.TrimSpace(a.cfg.PrinterName)
		a.mu.RUnlock()
	}
	if printer == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "printer_name not set. Call POST /config/printer first."})
		return
	}

	if err := printRawZPL(printer, in.ZPL); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

/* ============================
   RAW printing via winspool.drv
============================ */

type docInfo1 struct {
	pDocName    *uint16
	pOutputFile *uint16
	pDatatype   *uint16
}

var (
	modWinspool         = syscall.NewLazyDLL("winspool.drv")
	procOpenPrinter     = modWinspool.NewProc("OpenPrinterW")
	procClosePrinter    = modWinspool.NewProc("ClosePrinter")
	procStartDocPrinter = modWinspool.NewProc("StartDocPrinterW")
	procEndDocPrinter   = modWinspool.NewProc("EndDocPrinter")
	procStartPage       = modWinspool.NewProc("StartPagePrinter")
	procEndPage         = modWinspool.NewProc("EndPagePrinter")
	procWritePrinter    = modWinspool.NewProc("WritePrinter")
)

func printRawZPL(printerName string, zpl string) error {
	if printerName == "" {
		return errors.New("printer name empty")
	}

	pName, err := syscall.UTF16PtrFromString(printerName)
	if err != nil {
		return err
	}

	var hPrinter syscall.Handle
	r1, _, e1 := procOpenPrinter.Call(uintptr(unsafe.Pointer(pName)), uintptr(unsafe.Pointer(&hPrinter)), 0)
	if r1 == 0 {
		return fmt.Errorf("OpenPrinterW failed: %v", e1)
	}
	defer procClosePrinter.Call(uintptr(hPrinter))

	docName, _ := syscall.UTF16PtrFromString("LabelAgent ZPL Job")
	dataType, _ := syscall.UTF16PtrFromString("RAW")

	di := docInfo1{
		pDocName:    docName,
		pOutputFile: nil,
		pDatatype:   dataType,
	}

	jobID, _, e2 := procStartDocPrinter.Call(uintptr(hPrinter), 1, uintptr(unsafe.Pointer(&di)))
	if jobID == 0 {
		return fmt.Errorf("StartDocPrinterW failed: %v", e2)
	}
	defer procEndDocPrinter.Call(uintptr(hPrinter))

	r3, _, e3 := procStartPage.Call(uintptr(hPrinter))
	if r3 == 0 {
		return fmt.Errorf("StartPagePrinter failed: %v", e3)
	}
	defer procEndPage.Call(uintptr(hPrinter))

	// Zebra geralmente aceita \n normal no ZPL
	data := []byte(zpl)
	var written uint32
	r4, _, e4 := procWritePrinter.Call(
		uintptr(hPrinter),
		uintptr(unsafe.Pointer(&data[0])),
		uintptr(uint32(len(data))),
		uintptr(unsafe.Pointer(&written)),
	)
	if r4 == 0 {
		return fmt.Errorf("WritePrinter failed: %v", e4)
	}
	if written != uint32(len(data)) {
		return fmt.Errorf("WritePrinter wrote %d of %d bytes", written, len(data))
	}
	return nil
}
