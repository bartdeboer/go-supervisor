# go-supervisor

Standalone supervisor building blocks for ctgbot sandboxes.

This repository follows a strict split:

- `initcfg` defines the compiled service config contract used by both ctgbot and `ctg-init`.
- `cmd/ctg-init` is intended to become the tiny PID 1 data-plane supervisor.
- Rich authoring UX, JSON, validation policy, Docker integration, and database state belong in ctgbot.

The previous JSON-oriented supervisor spike was moved aside locally to `/workspace/src/go-supervisor-v1`.
