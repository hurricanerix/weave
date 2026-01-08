# Quickstart

## 1. Build

```bash
go build -o bin/weave ./cmd/weave
```

## 2. Start ollama

```bash
ollama serve
```

## 3. Start weave-compute (optional, for image generation)

```bash
cd compute-daemon && make
./weave-compute
```

## 4. Run weave

```bash
# Use default model (mistral:7b)
./bin/weave

# Or specify your model
./bin/weave --ollama-model phi3:latest
```

## 5. Open browser

http://localhost:8080

## Troubleshooting

| Error | Fix |
|-------|-----|
| "model not found" | Use `--ollama-model <your-model>` or `ollama pull mistral:7b` |
| "ollama not running" | Run `ollama serve` |
| "weave-compute not running" | Start daemon or skip image generation |
| Port in use | `./bin/weave --port 3000` |

List available models: `ollama list`

See [DEVELOPMENT.md](DEVELOPMENT.md) for full documentation.
