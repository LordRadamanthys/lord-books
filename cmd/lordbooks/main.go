package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	lordbooks "github.com/LordRadamanthys/lord-books"
	"github.com/LordRadamanthys/lord-books/internal/db"
	"github.com/LordRadamanthys/lord-books/internal/library"
	"github.com/LordRadamanthys/lord-books/internal/metadata"
	"github.com/LordRadamanthys/lord-books/internal/server"
	"github.com/LordRadamanthys/lord-books/internal/settings"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8484", "endereço do servidor")
	dataDir := flag.String("data", defaultDataDir(), "diretório de dados (banco e biblioteca)")
	googleKey := flag.String("google-key", os.Getenv("LORDBOOKS_GOOGLE_KEY"), "chave da API do Google Books (opcional)")
	flag.Parse()

	if err := os.MkdirAll(*dataDir, 0o755); err != nil {
		log.Fatalf("criar diretório de dados: %v", err)
	}

	dbPath := filepath.Join(*dataDir, "lordbooks.db")
	store, err := db.Open(dbPath)
	if err != nil {
		log.Fatalf("abrir banco: %v", err)
	}
	defer store.Close()

	cfg, err := settings.Load(*dataDir)
	if err != nil {
		log.Fatalf("carregar configurações: %v", err)
	}

	lib := library.New(store, *dataDir)
	handler, stopNetwork := server.New(lordbooks.WebFS(), store, lib, metadata.New(*googleKey), cfg)
	httpSrv := &http.Server{Addr: *addr, Handler: handler, ReadHeaderTimeout: 10 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		// Disconnect a Wi-Fi reader cleanly and close the LAN ports before exiting.
		stopNetwork()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpSrv.Shutdown(shutdownCtx)
	}()

	log.Printf("Lord Books rodando em http://%s", *addr)
	log.Printf("Dados em %s", *dataDir)
	if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}

func defaultDataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".lordbooks")
}
