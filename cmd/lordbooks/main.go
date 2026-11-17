package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"

	lordbooks "github.com/LordRadamanthys/lord-books"
	"github.com/LordRadamanthys/lord-books/internal/db"
	"github.com/LordRadamanthys/lord-books/internal/library"
	"github.com/LordRadamanthys/lord-books/internal/metadata"
	"github.com/LordRadamanthys/lord-books/internal/server"
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

	lib := library.New(store, *dataDir)
	srv := server.New(lordbooks.WebFS(), store, lib, metadata.New(*googleKey))
	log.Printf("Lord Books rodando em http://%s", *addr)
	log.Printf("Dados em %s", *dataDir)
	log.Fatal(http.ListenAndServe(*addr, srv))
}

func defaultDataDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".lordbooks")
}
