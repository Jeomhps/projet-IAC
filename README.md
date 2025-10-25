# Container/Machine Manager (API + Web)

A Docker-first, infrastructure-as-code system to manage a pool of machines/containers.

Includes:
- Web UI (SPA) served by Caddy, styled with Bootstrap 5
- Flask API (Gunicorn) to register machines, check availability, handle reservations
- JWT authentication with role-based access control (admin vs user)
- Scheduler worker for cleanup/maintenance
- MariaDB for persistence
- Ansible playbooks to provision/unprovision demo “machines”
- A Justfile and helper scripts for common workflows

---

## Architecture

High-level:
- Reverse proxy (Caddy): terminates TLS (self-signed in dev), serves the SPA, and proxies browser/API requests under /api to the Flask API.
- API: Flask app served by Gunicorn on HTTP :8080 inside the Docker network; stores users/machines/reservations in MariaDB; performs SSH/Ansible operations.
- Scheduler: background worker; expires reservations and performs maintenance.
- DB: MariaDB service; schema initialized by the API on first run.
- Demo machines: provisioned by Ansible as separate Docker containers with SSH exposed to host ports (22221, 22222, …). The API/Scheduler reach them via host.docker.internal.

```mermaid
flowchart LR
  Client["Client (Browser SPA + CLI curl)"]

  subgraph EDGE["edge network"]
    RP["Reverse Proxy (Caddy)\n- HTTPS :443\n- Serves SPA\n- Proxies /api → API"]
  end

  subgraph LAB["lab network"]
    API["Flask API\nGunicorn :8080"]
    SCHED["Scheduler Worker"]
    DB[(MariaDB)]
  end

  Client -->|HTTPS 443| RP
  RP -->|/api → HTTP :8080| API
  API -->|SQL| DB
  SCHED -->|SQL| DB

  Host["Host (Docker Engine)\nDemo 'alpine-container-N'\nports 22221..22230 → 22"]:::host
  API -. SSH host.docker.internal:222xx .-> Host
  SCHED -. SSH host.docker.internal:222xx .-> Host

  classDef host fill:#f6f6f6,stroke:#999,color:#333;
```

Notes:
- Separation: reverse-proxy attaches to edge and lab; API/DB/Scheduler only on lab.
- The SPA and API are same-origin (site + /api). This is the standard SPA pattern.
- macOS: host.docker.internal works by default.
- Linux: add extra_hosts for API/Scheduler so host.docker.internal resolves to the host gateway.

```yaml
# In src/docker-compose.yml (Linux only; macOS not required)
extra_hosts:
  - "host.docker.internal:host-gateway"
```

---

## Directory layout (partial)

```
.
├─ Justfile
├─ requirements.txt
├─ provision/
│  ├─ provision.yml
│  ├─ unprovision.yml
│  └─ machines.yml        # generated inventory (host ports for demo)
└─ src/
   ├─ docker-compose.yml
   ├─ docker-compose.dev.override.yml
   ├─ api/
   │  ├─ Dockerfile
   │  ├─ gunicorn.conf.py
   │  ├─ api.py            # Flask app
   │  ├─ auth.py           # /login, /whoami, /users
   │  ├─ machines.py       # manage/register machines
   │  └─ reservations.py   # reservation endpoints
   ├─ common/
   │  ├─ db.py             # SQLAlchemy models & session
   │  ├─ auth.py           # JWT utils, default admin seeding
   │  └─ playbooks/
   │     └─ create-users.yml
   ├─ reverse-proxy/
   │  ├─ Caddyfile
   │  └─ Dockerfile
   └─ web/
      ├─ index.html
      ├─ package.json
      ├─ tsconfig.json
      ├─ vite.config.ts
      └─ src/
         ├─ main.tsx
         ├─ App.tsx
         └─ pages/
            ├─ Login.tsx
            ├─ Machines.tsx
            ├─ Available.tsx
            ├─ Reservations.tsx
            └─ Users.tsx
```

---

## Getting started

- Build and run:
```bash
just run        # or: docker compose -f src/docker-compose.yml up -d --build
```

- Dev hot-reload (optional):
```bash
just run-dev    # uses src/docker-compose.dev.override.yml for API reload
```

- Logs:
```bash
just logs
```

- Teardown and cleanup:
```bash
just clean
```

Open the site at:
- https://localhost

Use -k with curl locally (self-signed cert in dev).

---

## Web UI (SPA)

Pages (all under https://localhost):
- /login: Sign in to get a JWT (stored client-side).
- /: Dashboard.
- /machines: List and (admins) register/delete machines.
- /available: Current pool status (available vs reserved).
- /reservations: Create reservations; view active ones; admin can “Release All”.
- /users: Admin-only user management.

The SPA calls the API via same-origin requests to /api/... behind the reverse proxy.

---

## Authentication

- POST /api/login issues a JWT. Include it as `Authorization: Bearer <token>` for protected endpoints.
- Roles:
  - Admin: manage machines and users; can release_all.
  - User: list machines and make reservations.

Seeded admin (dev):
- The API seeds a default admin on first boot from env:
  - `ADMIN_DEFAULT_USERNAME` (default: admin)
  - `ADMIN_DEFAULT_PASSWORD` (default: change-me)

Example: obtain a token (dev uses self-signed cert; note `-k`)
```bash
curl -k -X POST https://localhost/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"change-me"}'
# => { "access_token":"<JWT>", "token_type":"Bearer", "expires_in":3600 }
```

Check token:
```bash
TOKEN=<paste token>
curl -k -H "Authorization: Bearer $TOKEN" https://localhost/api/whoami
```

---

## API (through the reverse proxy)

Base: https://localhost/api

- Health
```bash
curl -k https://localhost/api/healthz
# {"status":"ok"}
```

- Auth
```bash
curl -k -X POST https://localhost/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"change-me"}'

TOKEN=<JWT>
curl -k -H "Authorization: Bearer $TOKEN" https://localhost/api/whoami
```

- Users (admin)
```bash
# Create
curl -k -X POST https://localhost/api/users \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"s3cret","is_admin":false}'

# List
curl -k -H "Authorization: Bearer $TOKEN" https://localhost/api/users

# Delete
curl -k -X DELETE https://localhost/api/users/alice \
  -H "Authorization: Bearer $TOKEN"
```

- Machines
```bash
# Register (admin)
curl -k -X POST https://localhost/api/machines \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"alpine-1","host":"host.docker.internal","port":22221,"user":"root","password":"test"}'

# List (auth)
curl -k -H "Authorization: Bearer $TOKEN" https://localhost/api/machines

# Delete (admin)
curl -k -X DELETE https://localhost/api/machines/alpine-1 \
  -H "Authorization: Bearer $TOKEN"
```

- Reservations
```bash
# Reserve N machines for a duration (minutes)
curl -k -H "Authorization: Bearer $TOKEN" \
  "https://localhost/api/reserve?count=2&duration=60&reservation_password=test"

# Release all (admin)
curl -k -H "Authorization: Bearer $TOKEN" https://localhost/api/release_all

# Pool status (auth)
curl -k -H "Authorization: Bearer $TOKEN" https://localhost/api/available

# Active reservations (auth)
curl -k -H "Authorization: Bearer $TOKEN" https://localhost/api/reservations
```

Production note: configure a trusted certificate and remove `-k`.

---

## Registering machines (admin)

Helper script: reads YAML and registers machines via the API.

Usage:
```bash
python utils/register_machines.py provision/machines.yml
```

YAML format:
```yaml
machines:
  - name: alpine-1
    host: localhost           # host.docker.internal is also fine
    port: 22221
    user: root
    password: test
  - name: alpine-2
    host: localhost
    port: 22222
    user: root
    password: test
```

The script:
- Logs in via POST /api/login using `ADMIN_DEFAULT_USERNAME`/`ADMIN_DEFAULT_PASSWORD`
- Posts each machine to POST /api/machines
- Rewrites `localhost`/`127.0.0.1` → `host.docker.internal` by default so API/Scheduler (in Docker) can reach the host-published ports.

Environment defaults (override as needed):
```bash
# SPA/API base and prefix
API_BASE=https://localhost
API_PREFIX=/api
# Admin creds
ADMIN_DEFAULT_USERNAME=admin
ADMIN_DEFAULT_PASSWORD=change-me
# Local TLS (self-signed)
VERIFY_TLS=false
# Host rewrite behavior
REWRITE_LOCALHOST=true
DOCKER_HOST_GATEWAY_NAME=host.docker.internal
```

macOS vs Linux:
- macOS: `host.docker.internal` works out of the box (no extra config).
- Linux: ensure API/Scheduler include:
```yaml
extra_hosts:
  - "host.docker.internal:host-gateway"
```

---

## Configuration

Key environment variables (not exhaustive):
- DATABASE_URL (e.g., `mysql+pymysql://appuser:apppass@db:3306/containers?charset=utf8mb4`)
- DB_CONNECT_MAX_RETRIES, DB_CONNECT_RETRY_SECONDS
- JWT_SECRET (required in prod), JWT_EXPIRES_MINUTES
- ADMIN_DEFAULT_USERNAME, ADMIN_DEFAULT_PASSWORD
- LOG_LEVEL, Gunicorn worker/settings
- WEB_PRELOAD_APP=true (preload seeding on boot)

---

## Production notes

- Only expose the reverse proxy (443). Keep API/Scheduler/DB on internal networks.
- Enforce HTTPS with trusted certs; remove `-k` in examples.
- Keep JWTs short-lived; consider refresh tokens in HttpOnly cookies if needed.
- Lock down CORS to your site’s origin if you split domains.
- Rate-limit /api/login and audit admin actions.
- Use strong `JWT_SECRET`; rotate periodically.

---
