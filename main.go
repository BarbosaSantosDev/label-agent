//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
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
POST /config/printer
POST /printer/connect
POST /printer/disconnect
POST /print
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
	_ = debug.Run(serviceName, &winService{app: a})
}

type winService struct {
	app  *app
	elog *eventlog.Log
}

func (s *winService) Execute(args []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	status <- svc.Status{State: svc.StartPending}

	server, err := s.app.startHTTP()
	if err != nil {
		status <- svc.Status{State: svc.Stopped}
		return false, 1
	}

	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	for c := range r {
		if c.Cmd == svc.Stop || c.Cmd == svc.Shutdown {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = server.Shutdown(ctx)
			cancel()
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
		if origin := r.Header.Get("Origin"); origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		}

		if h := r.Header.Get("Access-Control-Request-Headers"); h != "" {
			w.Header().Set("Access-Control-Allow-Headers", h)
			w.Header().Add("Vary", "Access-Control-Request-Headers")
		} else {
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")

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

func (a *app) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
		PrinterName string `json:"printer_name"`
		ZPL         string `json:"zpl"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || in.ZPL == "" {
		writeJSON(w, 400, map[string]string{"error": "invalid payload"})
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

	if err := printRawZPL(printer, in.ZPL); err != nil {
		writeJSON(w, 500, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, 200, map[string]any{"ok": true})
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

	if r, _, _ := openPrinter.Call(uintptr(unsafe.Pointer(p)), uintptr(unsafe.Pointer(&h)), 0); r == 0 {
		return errors.New("OpenPrinter failed")
	}
	defer closePrinter.Call(uintptr(h))

	doc, _ := syscall.UTF16PtrFromString("LabelAgent")
	raw, _ := syscall.UTF16PtrFromString("RAW")

	di := docInfo1{doc, nil, raw}
	r1, _, err := startDoc.Call(uintptr(h), 1, uintptr(unsafe.Pointer(&di)))
	if r1 == 0 {
		log.Printf("Erro ao iniciar o documento: %v", err)
		return err
	}
	defer endDoc.Call(uintptr(h))

	startPage.Call(uintptr(h))
	defer endPage.Call(uintptr(h))

	b := []byte(zpl)
	var written uint32
	r2, _, err := writePrinter.Call(
		uintptr(h),
		uintptr(unsafe.Pointer(&b[0])),
		uintptr(len(b)),
		uintptr(unsafe.Pointer(&written)),
	)
	if r2 == 0 {
		log.Printf("Erro ao escrever na impressora: %v", err)
		return err
	}

	return nil
}
