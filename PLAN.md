# harpy — instalador universal com TUI (plano de execução)

> Gerencia **pacotes de sistema** (winget, chocolatey, apt, pacman) em Windows e Linux.
> Gerencia dependências de **desenvolvimento** (runtimes, npm, pip) é papel do `upkg` — ver [TODO futuro](#10-todo-futuro-configs-de-desenvolvedor).
> Status: plano aprovado para execução. Estimativa: **55–72 dev-dias** (~11–14 semanas solo).

---

## 0. Correções aplicadas neste plano

| #   | Correção                                                                                                                | Onde                               |
| --- | ----------------------------------------------------------------------------------------------------------------------- | ---------------------------------- |
| 1   | Busca padrão é no **OS atual**; `--os` permite consultar outro OS (ex.: no Windows, verificar disponibilidade no Linux) | [§3](#3-semântica-de-busca)        |
| 2   | Índice de repositório em **SQLite dentro do HOME** (escopo de usuário, sem admin)                                       | [§4](#4-sqlite-índice-e-cache)     |
| 3   | Ordem de resolução: **config local → índice → query ao vivo**                                                           | [§3.3](#33-ordem-de-resolução)     |
| 4   | Query ao vivo consome os **primeiros 200** resultados brutos do PM                                                      | [§3.4](#34-limites-e-paginação)    |
| 5   | Fuzzy: exibe **25 por página**, passo **25**, teto **200** (8 páginas)                                                  | [§3.4](#34-limites-e-paginação)    |
| 6   | Binário/CLI/namespace é **`harpy`** — `upkg` já existe e é outra ferramenta                                             | [§1](#1-nomenclatura-e-identidade) |

---

## 1. Nomenclatura e identidade

| Item                   | Valor                                                                                       |
| ---------------------- | ------------------------------------------------------------------------------------------- |
| Módulo Go              | `github.com/lfaoliveira/harpy`                                                              |
| Binário / comando      | `harpy`                                                                                     |
| Config do usuário      | Linux `~/.config/harpy/config.yaml` · Windows `%APPDATA%\harpy\config.yaml`                 |
| Banco (índice + cache) | Linux `~/.local/share/harpy/harpy.db` · Windows `%LOCALAPPDATA%\harpy\harpy.db`             |
| Overrides              | `HARPY_CONFIG`, `HARPY_DB`                                                                  |
| Logs                   | `~/.local/state/harpy/logs/<run-id>/<pacote>.log` · Windows `%LOCALAPPDATA%\harpy\logs\...` |

Nunca usar `upkg` em path, comando, alias ou variável. Nada de `alias harpy=...` — o binário **é** `harpy`.

Toolchain: Go 1.23+ (já coberto por `go = "latest"` no `mise.toml`). Todas as dependências são CGO-free → binário estático único.

| Dependência                                        | Uso                                                       |
| -------------------------------------------------- | --------------------------------------------------------- |
| `charmbracelet/bubbletea` + `bubbles` + `lipgloss` | TUI                                                       |
| `sahilm/fuzzy`                                     | ranking fuzzy (pure Go, zero deps)                        |
| `modernc.org/sqlite`                               | SQLite CGO-free (+~8 MB no binário)                       |
| `gopkg.in/yaml.v3`                                 | config, com edição via `yaml.Node` (preserva comentários) |

---

## 2. Arquitetura

```
cmd/harpy/main.go            # flags, dispatch CLI, check de privilégio
internal/
  config/      # schema YAML v1, match de máquina, resolução de perfis
  manager/     # Manager + Searcher por PM
    winget.go  choco.go  apt.go  pacman.go
  index/       # SQLite: schema, upsert, query, TTL, refresh
  search/      # merge, filtros, fuzzy/literal, paginação
  engine/      # plan -> download -> install sequencial -> resumo
  ui/          # bubbletea: lista, busca, logs, confirmação
  logx/        # log por pacote
docs/TODO.md   # futuro (configs de dev, sync)
config.example.yaml
```

Contrato central (mantém os 4 PMs e a busca uniformes):

```go
type Manager interface {
    Name() string
    OS() string                                      // "windows" | "linux"
    Available() bool                                 // binário no PATH
    NeedsElevation() bool
    Installed(ctx context.Context, ids []string) (map[string]bool, error)
    Download(ctx context.Context, ids []string, w io.Writer) error  // ErrSemDownload
    Install(ctx context.Context, id string, w io.Writer) error
}

type Searcher interface {
    Manager() string
    OS() string
    // Catálogo completo, quando o PM permite extrair em lote (apt, pacman).
    Index(ctx context.Context) ([]index.Row, error)
    // Busca no servidor (winget, choco) ou no índice (apt, pacman).
    Query(ctx context.Context, term string, rawLimit int) ([]index.Row, error)
}
```

O core roda **headless** (`harpy plan`, `harpy apply --yes`, `harpy search --json`); a TUI é cliente da mesma engine. Instalação é **100% sequencial** (um `exec.Command` por vez). Sem DLL de instalador no processo — o problema de thread-safety nativa não existe por construção.

A única concorrência do projeto é I/O de UI: busca assíncrona via `tea.Cmd` (goroutine + canal) para não travar a TUI. Não é thread nativa, não toca instalador.

---

## 3. Semântica de busca

### 3.1 Seleção de OS

```
--os current   (default)  -> runtime.GOOS
--os windows | linux      -> OS remoto: SOMENTE índice, nunca query ao vivo
--os all                  -> união dos índices existentes (+ live do host)
```

TUI: tecla `o` cicla `current → windows → linux → all`, com badge visível no topo.

| OS selecionado | Config local            | Índice SQLite  | Query ao vivo    |
| -------------- | ----------------------- | -------------- | ---------------- |
| `current`      | ✅                      | ✅             | ✅ (PMs do host) |
| outro OS       | ✅ (filtrado por `os:`) | ✅ se indexado | ❌ impossível    |
| `all`          | ✅                      | ✅ (união)     | ✅ (só host)     |

**Regra dura:** resultado vindo de índice de outro OS é **read-only** — `i` (instalar) desabilitado; só `a` (adicionar à config). A UI mostra `índice de linux · atualizado 2026-11-18 · pode estar desatualizado`.

### 3.2 Como indexar outro OS

Não existe `apt-cache` no Windows. As três formas de popular índice remoto:

1. **Gerador via WSL** (Windows → Linux):
   `harpy index refresh --os linux --via wsl` → `wsl -d Debian -- apt-cache dumpavail`.
2. **Gerador via Docker**:
   `harpy index refresh --os linux --via docker` → `docker run --rm debian:12 sh -c 'apt-get update -qq && apt-cache dumpavail'`; idem `archlinux` com `pacman -Sy && pacman -Sl`.
3. **Snapshot portátil**:
   `harpy index export --os linux -o linux.idx` / `harpy index import linux.idx` (formato JSONL gzip; também serve para seeds publicados nos releases e para o sync futuro).

Se o índice do OS pedido não existir: erro explícito com os 3 caminhos acima. Nunca cair silenciosamente no índice do host.

> Fora de escopo v1: catálogo winget em Linux. O fonte do winget é um MSIX com SQLite interno (`index.db`) — extraível na gambiarra, **não** suportado. Anotar em TODO como experimento.

### 3.3 Ordem de resolução

1. **Config local** (`profiles`/`machines`) — substring/fuzzy no `name` e nos IDs por manager. Instantâneo, sem I/O. Resultados marcados `seu app` e **fixados no topo**.
2. **Índice SQLite** do(s) OS selecionado(s) — catálogo completo, offline.
3. **Query ao vivo** — só se o OS selecionado == host e o PM existir. Alimenta o índice quando faz sentido (`winget`/`choco`, que não têm dump).

Resultados de (2) e (3) são mesclados em **uma lista única**; os de (1) vêm antes com boost de score. Dedupe por `(manager, id)`, mantendo a versão de maior score.

### 3.4 Limites e paginação

| Constante                          | Valor                |
| ---------------------------------- | -------------------- |
| Resultados brutos de query ao vivo | **200** (`rawLimit`) |
| Tamanho de página                  | **25**               |
| Passo de paginação                 | 25                   |
| Teto total exibível                | **200** (8 páginas)  |

- Fuzzy: ranqueia o pool (até 200 do live; todas as linhas do índice) e exibe 25 por vez.
- Literal: mesmo enquadramento (substring case-insensitive no nome; `--desc` opcional inclui descrição).
- Rodapé: `1–25 de 200   [n] próx  [p] ant  página 1/8`.
- CLI: `--page N` (default 1), `--page-size 25` (travado em 25 no v1), `--max 200` (teto rígido, não configurável no v1).
- Indexação local de apt (~80k linhas): `LIKE` + varredura fuzzy em Go ≈ 10–20 ms. **Sem FTS5 no v1** (otimização futura documentada em TODO).

---

## 4. SQLite: índice e cache

```sql
PRAGMA journal_mode = WAL;
PRAGMA busy_timeout = 5000;

CREATE TABLE packages (
  manager     TEXT NOT NULL,      -- winget | chocolatey | apt | pacman
  os          TEXT NOT NULL,      -- windows | linux
  id          TEXT NOT NULL,      -- ID canônico (winget id, choco id, nome do pacote)
  name        TEXT NOT NULL,      -- nome/nome exibível
  version     TEXT,
  repo        TEXT,               -- source/repo/componente
  descr       TEXT,               -- truncado em 160 chars
  PRIMARY KEY (manager, os, id)
);
CREATE INDEX packages_os_manager ON packages(os, manager);
CREATE INDEX packages_name       ON packages(os, name);

CREATE TABLE index_meta (
  manager TEXT NOT NULL, os TEXT NOT NULL,
  source TEXT,                    -- host | wsl | docker | import
  generated_at INTEGER NOT NULL,
  total INTEGER NOT NULL,
  PRIMARY KEY (manager, os)
);

CREATE TABLE live_cache (
  manager TEXT NOT NULL, term TEXT NOT NULL,
  payload TEXT NOT NULL,          -- JSON com []Row
  fetched_at INTEGER NOT NULL,
  PRIMARY KEY (manager, term)
);
```

- TTL: índice 24 h (aviso na TUI quando vencido); `live_cache` 24 h winget / 7 d choco (rate-limit).
- `harpy index refresh` explícito; `harpy index status` mostra cobertura por (manager, os) e idade.
- Escrita só em refresh; busca é read-only. Cinco tabelas, zero ORM.

Coleta por PM:

| PM     | Índice                                 | Query                                                               |
| ------ | -------------------------------------- | ------------------------------------------------------------------- |
| apt    | `apt-cache dumpavail` (sem root, ~1 s) | `apt-cache search --names-only <term>`                              |
| pacman | `pacman -Sl` (sem root; exige sync db) | `pacman -Ss <term>` / detalhe `pacman -Si`                          |
| winget | — (não há dump)                        | `winget search "<term>" --source winget --accept-source-agreements` |
| choco  | —                                      | `choco search "<term>" --limit-output --no-progress`                |

---

## 5. Busca — implementação

```
term (query)
  └─ filtro: managers ∩ (os selecionado)
      ├─ config:   match em name/ids          -> boost +1000, badge "seu app"
      ├─ índice:   SELECT ... WHERE os IN (...) -> pool
      └─ live:     Query(term, 200)            -> pool   (só host)
  └─ ranking: fuzzy.Score | literal substring
  └─ corte em 200 -> página de 25
```

- Fuzzy: `sahilm/fuzzy.Find` sobre `name + " " + id`; empate por nome mais curto, depois `installed` primeiro.
- Literal: `strings.Contains(lower)`, ordena por (installed, nome).
- `Installed` cruza com a detecção (`Installed()` por manager) e é cacheado por run.
- Debounce 250 ms na TUI; `generation` counter descarta respostas fora de ordem.

### Parser do winget (o ponto frágil)

Saída é tabela de texto **localizada** (o host do usuário é pt-BR):

```
Nome               ID                              Versão
----------------------------------------------------------
Git                Git.Git                         2.47.1
```

- Delimitar colunas pela linha de `---` (posições fixas), não por split de espaços.
- Descartar as 2 primeiras linhas (título + separador); parar em linha vazia.
- Descrições truncadas com `...` são aceitas como estão.
- Vazio/erro: checar exit code, não a mensagem traduzida.
- **Fixtures obrigatórias** em `testdata/winget_{pt-BR,en-US}.txt`; o teste roda o parser contra as duas.

---

## 6. Config YAML

```yaml
version: 1
search:
    mode: fuzzy # fuzzy | literal
    include_description: false

defaults:
    managers: [winget, chocolatey] # precedência no Windows
    continue_on_error: true

profiles:
    base:
        apps:
            - git
            - name: vscode
              winget: Microsoft.VisualStudioCode
              choco: vscode
            - name: ripgrep
              winget: BurntSushi.ripgrep.MSVC
              choco: ripgrep
              apt: ripgrep
              pacman: ripgrep
    dev:
        apps: [neovim, jq]
    linux-base:
        apps:
            - name: neovim
              apt: neovim
              pacman: neovim

machines:
    - name: desktop-casa
      match: { os: windows, hostname: 'DESKTOP-*' }
      profiles: [base, dev]
    - name: thinkpad
      match: { os: linux, hostname: 'thinkpad-*' }
      profiles: [linux-base, dev]
    - name: generica
      match: { os: any }
      profiles: [base]
```

Resolução:

1. `--machine <nome>` vence; senão casa `match` (hostname glob + `os`) na ordem; `os: any` é fallback.
2. App: usa o ID do manager escolhido (precedência de `defaults.managers`); sem ID específico, `name` é o ID.
3. Detecção só dentro do manager escolhido — sem verificação cruzada winget↔choco no v1 (limitação documentada).
4. `version: 1` + chaves desconhecidas **geram aviso, não erro** → arquivo continua válido quando entram as configs de dev.

---

## 7. Engine de instalação

```
plan    : perfil -> apps -> manager por app -> detectar instalados -> diff
download: uma invocação por PM com TODOS os pacotes (delega paralelismo ao PM)
install : sequencial, 1 pacote por vez, log por pacote, continua em erro
summary : ok / instalado / já instalado / falhou (código + última linha)
```

- **Download:** só onde existe de verdade — `apt-get update` + `apt-get -d install -y <pkgs>`, `pacman -Sw --noconfirm <pkgs>`. O PM já paraleliza internamente. winget/choco retornam `ErrSemDownload` → baixam durante a instalação (documentado na UI como fase única).
- **Sequencial:** elimina `1618 ERROR_INSTALL_ALREADY_RUNNING`; 1 retry opcional só para 1618.
- **Exit codes:** winget `0`, `0x8A150061` (já instalado), `0x8A15002B` (sem atualização), `3010/1641` (reboot pendente); choco idem 1641/3010; apt `100` (erro **ou** lock — distinguir por stderr); pacman `1`.
- **Locks Linux:** `flock` próprio + detectar lock de terceiro (`/var/lib/dpkg/lock-frontend`, `/var/lib/pacman/db.lck`); `apt-get -o DPkg::Lock::Timeout=120`, `DEBIAN_FRONTEND=noninteractive`.
- **Pacman:** política `--pacman-sync=pkg|full` — padrão `pkg` (`-Sy` + aviso de partial upgrade no log); `full` roda `-Syu` antes.
- **Elevação:** checagem no startup; Windows auto-relaunch via `ShellExecuteEx "runas"`; Linux exige `sudo` e imprime o comando exato.

---

## 8. TUI

```
┌ harpy ─ windows · perfil: desktop-casa ─────────────── 3 instalados, 12 a instalar ┐
│ 🔍 neovim                                  [fuzzy]  OS: current  PM: todos        │
├────────────────────────────────────────────────────────────────────────────────────┤
│  seu app   neovim           0.10.2   apt                     já instalado           │
│  índice    neovim           0.10.2   pacman (linux)                                 │
│  índice    neovim-nightly   0.11.0   winget                                         │
│  ...                                                                                │
├────────────────────────────────────────────────────────────────────────────────────┤
│ 1–25 de 200   [n] próx  [p] ant  página 1/8   / buscar  Tab PM  o OS  ^F fuzzy     │
└────────────────────────────────────────────────────────────────────────────────────┘
```

| Tecla                       | Ação                                                                      |
| --------------------------- | ------------------------------------------------------------------------- |
| `/`                         | abre busca (debounce 250 ms, assíncrona)                                  |
| `Tab`                       | cicla filtro de instalador (`todos → winget → choco → apt → pacman`)      |
| `o`                         | cicla OS (`current → windows → linux → all`)                              |
| `Ctrl+F`                    | alterna fuzzy ↔ literal                                                   |
| `n` / `p` / `PgUp` / `PgDn` | paginação de 25 (teto 200)                                                |
| `Enter`                     | detalhes (`winget show` / `choco info` / `apt-cache show` / `pacman -Si`) |
| `Space`                     | marca/desmarca                                                            |
| `i`                         | instala seleção agora (bloqueado se OS ≠ host)                            |
| `a`                         | adiciona ao perfil no YAML (`yaml.Node`, preserva comentários)            |
| `l`                         | painel de log ao vivo do pacote em instalação                             |
| `q` / `Esc`                 | sai (com confirmação se houver instalação em curso)                       |

Telas: **Lista** (perfil/máquina), **Busca**, **Log**, **Resumo**. Spinner + fase + tempo por pacote (winget não fornece porcentagem).

---

## 9. CLI

```bash
harpy init                                   # gera config.example.yaml
harpy plan                                   # diff deseja x instalado
harpy apply [--yes] [--machine NAME]         # download + instalação sequencial
harpy search neovim --fuzzy --page 1         # OS atual, todos os PMs
harpy search node --os linux --literal       # cross-OS: só índice, read-only
harpy search git --manager winget,choco --json
harpy index refresh [--os linux --via wsl|docker]
harpy index export --os linux -o linux.idx   /  harpy index import linux.idx
harpy index status
harpy show <manager>/<id>
```

Toda saída `--json` é estável e versionada (`{"schema":1,...}`) — é a superfície de teste e a futura API para o `upkg`.

---

## 10. TODO futuro (configs de desenvolvedor)

`docs/TODO.md` com escopo explícito de que **harpy não gerencia dependências de dev** — isso é do `upkg`. Harpy apenas delega.

```yaml
# FUTURO — não implementado no v1
version: 2

packages: # v1 (atual, harpy)
    apt: [neovim]

dev: # TODO: delega ao upkg
    runtimes:
        node: '24'
        python: '3.13'
        go: 'latest'
    managers: [npm, pip, cargo]
    dotfiles:
        tool: chezmoi
        source: git@github.com:user/dotfiles.git

sync: # TODO: contas (até 3), Drive/git, servidor dedicado
    provider: git | gdrive | server
```

Hooks que custam 0,5 dia agora e evitam reescrita: `version: 1`, tolerância a chaves desconhecidas, `Manager`/`Searcher` plugáveis, pipeline `plan → download → install` extensível com steps (`dev`, `dotfiles`). Também no TODO: FTS5 para busca, catálogo winget em Linux, AUR, assinatura de código, verificação cruzada choco↔winget.

---

## 11. Marcos e estimativa

| Marco     | Entrega                                                                                                                                     | Dias      |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------- | --------- |
| **M0**    | Skeleton, module `harpy`, config YAML + match de máquina, `harpy init`                                                                      | 3–4       |
| **M1**    | Detecção (4 PMs), adapter winget, engine sequencial, `plan`/`apply` CLI                                                                     | 9–12      |
| **M2**    | SQLite (schema, TTL, refresh, export/import, WSL/Docker), search core (fuzzy/literal, filtros, paginação 25/200), adapters choco/apt/pacman | 17–24     |
| **M3**    | TUI completa (lista, busca paginada, filtros, log ao vivo, detalhes, add-to-config)                                                         | 10–14     |
| **M4**    | Elevação, logs por pacote, resumo, `--json`, goreleaser (2 OS, binário estático)                                                            | 6–8       |
| **M5**    | Testes (fixtures winget pt-BR/en-US, containers Debian/Arch, Windows Sandbox), docs, `docs/TODO.md`                                         | 6–9       |
| **Total** |                                                                                                                                             | **55–72** |

Cortes possíveis para chegar a ~40 dias: sem `a` (add-to-config), sem `--os all`, sem export/import de índice, sem TUI de detalhes.

---

## 12. Riscos

| Risco                                              | Mitigação                                                                    |
| -------------------------------------------------- | ---------------------------------------------------------------------------- |
| `winget search` sem saída estruturada e localizada | parser posicional + fixtures pt-BR/en-US + testes de regressão               |
| Cross-OS pedir índice inexistente                  | erro explícito com 3 caminhos (wsl/docker/import); nunca fallback silencioso |
| Índice remoto desatualizado iludir o usuário       | badge com data + instalar desabilitado + `index status`                      |
| `choco search` rate-limit (429)                    | cache 7 d + backoff + mensagem clara                                         |
| `pacman -Sy` + `-S` (partial upgrade)              | default `-Sy` com aviso; flag `--pacman-sync=full`                           |
| `apt-get update` ausente → índice velho            | step explícito no refresh/download                                           |
| choco exige admin, apt/pacman exigem root          | check no startup + comando exato na mensagem                                 |
| SQLite `modernc` adiciona ~8 MB ao binário         | aceito (CGO=0 vale mais)                                                     |
| Escrita no YAML corromper config do usuário        | `yaml.Node` + backup `.bak` + validação antes de gravar                      |
| Testar Linux no Windows                            | containers Debian/Arch para adapters; VM para smoke final                    |

---

## 13. Decisões pendentes

1. `--os all` entra no v1 ou só `current|windows|linux`?
2. `a` (add-to-config) no v1 (+4–5 dias) ou v1 apenas instala e a config é editada à mão?
3. Pacman default: `-Sy` rápido com aviso, ou exigir `-Syu` (seguro, lento)?
4. `harpy index refresh` automático (best-effort, sem travar) quando o índice estiver vencido, ou só manual?
