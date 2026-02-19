# Weave

> Thread by thread, the image emerges — not from your hand alone, nor the machine's, but from the space between where meaning is made.

Weave is an open-source, local-first desktop application for image generation through natural conversation.

- :warning: This project is an early prototype
- :warning: This project is a vibe coding experiment!  To date nearly the entire code base was written by agents using the rules under the [claude](.claude) directory.
- :warning: There are no gurantees the code in this project is correct or safe to run, use at your own risk.

See the [docs](./docs) for more details about the project.

![Preview of Weave](docs/images/weave-preview.png)

## Quick start

Prerequisites:
- Go 1.25.5+
- Node.js 18+
- Make
- [ollama](https://ollama.com/) with `llama3.1:8b` model

Pull the LLM model
Build and run

```bash
ollama pull llama3.1:8b
make run
```

See [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md) for detailed build instructions and development workflow.

