# go-supervisor

Small standalone supervisor tools for durable sandbox services.

## Binaries

- `supervisord` is the tiny data-plane daemon intended to run as PID 1 in a
  sandbox container.
- `supervisor` is the human/agent-facing companion CLI for editing the durable
  service config.

`supervisord` intentionally avoids the companion CLI dependencies. The daemon
loads the compiled config, starts configured services, reaps children through a
single `wait4` path, restarts services according to policy, reloads on `SIGHUP`,
and stops service process groups on `SIGTERM`/`SIGINT`/`SIGQUIT`.

Reload semantics are deliberately fail-safe:

- a valid config is reconciled with the running services,
- a missing config is treated as an empty config and stops configured services,
- a malformed config is ignored and the current services keep running.

During reload, replacing a changed service waits for the old process group to
stop before starting the new one. That keeps replacement simple and avoids
double-running a service, but it also means a large reload can briefly delay
handling another signal.

## Config path

Both binaries resolve the config path as:

1. explicit `--config` where supported by the command,
2. `SUPERVISORD_CONFIG`,
3. `/home/agent/state/supervisord.config.bin`.

The config format is owned by `initcfg` and encoded with `go-tape`.

Service environment entries override inherited environment variables with the
same key.

## Basic use

```bash
supervisor service enable --name web --cwd /home/agent/services/web -- ./web-server
supervisor service list
supervisor service remove web
```

`supervisor reload` is currently informational; live reload is performed by
sending `SIGHUP` to `supervisord`.
