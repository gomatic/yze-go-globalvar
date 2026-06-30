# yze-go-globalvar

A [`yze`](https://github.com/gomatic/yze) analyzer (category `immutability`) that reports package-level mutable `var`s, which the gomatic immutability/dependency-injection standard forbids: prefer a constant or dependency injection. A small allow-listed set of sanctioned package vars (`version`, `Analyzer`, `Registration`) — plus any configured via the `-allow` flag — is permitted.

- **Rule:** `yze/globalvar`
- **Library:** exports `Analyzer` (a standard `go/analysis` analyzer) and `Registration` for the [`yze`](https://github.com/gomatic/yze) aggregator and [`stickler`](https://github.com/gomatic/stickler) runner.
- **Binary:** `cmd/yze-go-globalvar` runs it standalone (`text`/`-json`, and as a `go vet -vettool`).
- **Config:** `-allow` takes a comma-separated list of additional permitted package-level var names.

Built on the [`go-yze`](https://github.com/gomatic/go-yze) framework.
