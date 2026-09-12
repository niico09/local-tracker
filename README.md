# local-tracker

Self-hosted, single-binary tracker for a two-person shared watch/read list. It
runs on your LAN over plain HTTP, stores everything in one SQLite database, and
ships its templates, CSS and PWA manifest inside the binary.

## What it does

- Two profiles with PIN login (PBKDF2, rate-limited).
- A shared catalog of items (series, movie, book, course) with optional covers.
- Couple goals and personal goals. Personal goals are private by default; the
  owner can grant the partner read-only visibility.
- Progress with an optional target, derived state (`pending`, `in_progress`,
  `completed`) and one shared advance for couple goals.
- An idempotent `seed` command for the Top 100 list and a WAL-safe `backup`
  command.

## Requirements

- Go 1.25+ (toolchain 1.27.x used in development). No CGO: `modernc.org/sqlite`.
- Node.js only to regenerate CSS (Tailwind CLI is a dev dependency).

## Build and run

```bash
make build                 # regenerate CSS, then build ./bin/tracker
./bin/tracker serve        # default: 0.0.0.0:8080, data in ./data
```

Open `http://<host>:8080/`, complete the setup wizard with two names and PINs,
and sign in. Useful flags:

```bash
./bin/tracker serve -addr 127.0.0.1:8080 -data /var/lib/local-tracker
TRACKER_DATA=/var/lib/local-tracker ./bin/tracker serve
```

Flags override `TRACKER_*` environment variables (`TRACKER_ADDR`,
`TRACKER_DATA`), which override the defaults.

## Seed the Top 100

```bash
./bin/tracker seed
```

`seed` is idempotent: it inserts the missing items and the couple goal
`external_key = 'top100'` (owner unset, target 100) only once, then joins every
seeded item to that goal. Running it again inserts nothing. Both partners share
the same advance toward 100.

> **Placeholder data.** No authoritative Top 100 series list was supplied, so
> `internal/seed/top100.json` contains 100 clearly generic placeholder entries
> (`Series 001` … `Series 100`). Replace them with your real list before relying
> on the seeded titles; only `external_id` must stay unique and stable, because
> idempotency is keyed on it. An item already in the catalog under a different
> `external_id` is not deduplicated by title.

## Back up

```bash
./bin/tracker backup /backups/tracker-$(date +%F).db
./bin/tracker backup -uploads /backups/tracker.db   # also copies data/uploads
```

`backup` snapshots the database with `VACUUM INTO`, so the copy is
transactionally consistent under WAL, then atomically renames it into place. The
destination must not already exist. With `-uploads`, the uploads tree is copied
beside the snapshot to `<destination>.uploads`.

## Install as a service (systemd)

```bash
sudo install -Dm0755 bin/tracker /usr/local/bin/tracker
sudo install -Dm0644 deploy/local-tracker.service /etc/systemd/system/local-tracker.service
sudo useradd --system --home /var/lib/local-tracker --shell /usr/sbin/nologin tracker
sudo mkdir -p /var/lib/local-tracker && sudo chown tracker:tracker /var/lib/local-tracker
sudo systemctl daemon-reload
sudo systemctl enable --now local-tracker
```

Edit `User=`, `ExecStart=` and the data path in the unit if your layout differs.
Logs: `journalctl -u local-tracker -f`.

## Home-screen shortcut (PWA) — no offline mode

The app serves `/manifest.webmanifest` and an SVG icon, so a phone can add an
add-to-home-screen shortcut. There is deliberately **no service worker** and no
offline behavior: the app is served over plaintext LAN HTTP, where a service
worker would add caching/staleness and HTTPS requirements for no benefit. The
manifest alone provides the shortcut.

Covers are served from `/uploads/{name}`; cookies are `HttpOnly`, `SameSite=Lax`
and intentionally **not** `Secure`, matching the plaintext LAN deployment.

## Development

```bash
make css     # regenerate internal/ui/static/app.css (Tailwind v4)
make test    # go test ./...
make run     # build and serve
```

Architecture: `web -> service -> domain <- storage`, wired in
`cmd/tracker/main.go`. `domain` holds the single authorization rule
(`domain.Authorize`); `storage` owns all SQL and funnels it through
actor-guarded helpers; `service` orchestrates use cases; `web` only handles
HTTP. Every repository method requires an acting user.
