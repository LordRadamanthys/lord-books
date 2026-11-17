run:
	go run ./cmd/lordbooks

build:
	go build -o bin/lordbooks ./cmd/lordbooks

# Binários para qualquer PC (um arquivo, sem dependências)
dist:
	GOOS=darwin  GOARCH=arm64 go build -o dist/lordbooks-mac-arm64     ./cmd/lordbooks
	GOOS=darwin  GOARCH=amd64 go build -o dist/lordbooks-mac-intel     ./cmd/lordbooks
	GOOS=windows GOARCH=amd64 go build -o dist/lordbooks-windows.exe   ./cmd/lordbooks
	GOOS=linux   GOARCH=amd64 go build -o dist/lordbooks-linux         ./cmd/lordbooks
