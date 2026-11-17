# Lord Books

Gerenciador de biblioteca de ebooks — alternativa pessoal ao Calibre, com visual de e-reader.

## Rodar
```bash
make run   # abre em http://127.0.0.1:8484
```

## Gerar binários para outros PCs
```bash
make dist
```

## Estrutura
- `cmd/lordbooks` — ponto de entrada
- `internal/server` — HTTP + API
- `web/static` — interface (embutida no binário via go:embed)

## Roadmap do MVP
1. Biblioteca: importar livros, SQLite, busca e filtros
2. Metadados e capas: OPF do EPUB, Google Books / Open Library
3. Envio por USB para o cartão do e-reader
4. Servidor OPDS e envio por Wi-Fi (CrossPoint)
