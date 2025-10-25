# Container/Machine Manager API

A Docker-first, infrastructure-as-code system to manage a pool of machines/containers. It provides:
- A Flask API (served by Gunicorn) to register machines, check availability, and handle reservations
- JWT authentication with role-based access control (admin vs user)
- A scheduler worker that cleans up expired reservations and performs maintenance
- MariaDB for persistence
- Ansible playbooks to provision/unprovision the pool
- A Justfile to orchestrate common workflows (run, run-dev, clean, logs)

---

## What changed

Authentication is now implemented:
- POST /login issues short-lived JWT access tokens
- Admin-only endpoints:
  - POST /machines, DELETE /machines/<name>, GET /release_all, and user management (/users)
- Authenticated (user or admin) endpoints:
  - GET /machines, GET /available, GET /reservations, GET /reserve
- Users are stored in the DB; passwords are hashed. Tokens carry roles (["admin"] or []).

By default in dev:
- A default admin is seeded from environment variables:
  - ADMIN_DEFAULT_USERNAME (default: admin)
  - ADMIN_DEFAULT_PASSWORD (default: change-me)
- TLS is terminated by the reverse proxy using a self-signed certificate. Use -k with curl locally.

---

## Requirements

- Docker and Docker Compose plugin (Docker Desktop on macOS/Windows, Docker Engine on Linux)
- Ansible installed on the host (with the community.docker collection if you target Docker)
- Python 3.9+ on the host (for helper scripts via Just)
- just command runner (brew install just or cargo install just)

---

## Architecture

High level:
- Reverse proxy: terminates TLS (self-signed in dev), exposes HTTPS on localhost, forwards to the API service
- API: Flask app served by Gunicorn on HTTP :8080 inside the Docker network; reads/writes reservations and machines in MariaDB
- Scheduler: background worker; periodically scans DB to expire reservations and perform maintenance
- DB: MariaDB service; schema initialized by the API on first run

```mermaid
flowchart LR
  Client["Client: curl, utils/register_machines.py"]

  subgraph Docker_Network
    RP["Reverse Proxy<br/>(HTTPS → HTTP)"]
    API["Flask API<br/>Gunicorn :8080<br/><i>invokes Ansible</i>"]
    SCHED["Scheduler Worker<br/><i>invokes Ansible</i>"]
    DB[(MariaDB)]

    RP -->|HTTP :8080| API
    API -->|SQL| DB
    SCHED -->|SQL| DB
  end

  Client -->|HTTPS 443| RP
```

Notes:
- Code inside containers lives at /app/src.
- In dev, src/docker-compose.dev.override.yml bind-mounts host ./src to /app/src and runs Gunicorn with --reload for the API.
- The reverse proxy uses a self-signed cert in dev; use curl -k to skip verification locally.

---

## Directory layout (partial)

```
.
├─ Justfile
├─ requirements.txt                 # Host-side requirements for helper scripts
├─ provision/
│  ├─ machines.txt
│  ├─ provision.yml
│  └─ unprovision.yml
└─ src/
   ├─ docker-compose.yml
   ├─ docker-compose.dev.override.yml
   ├─ api/
   │  ├─ Dockerfile
   │  ├─ gunicorn.conf.py
   │  ├─ api.py                     # Flask app factory and blueprint registration
   │  ├─ auth.py                    # Auth blueprint (/login, /whoami, /users)
   │  ├─ machines.py                # Machines endpoints (admin-protected for mutations)
   │  └─ reservations.py            # Reservation endpoints (auth required; admin for release_all)
   ├─ common/
   │  ├─ db.py                      # SQLAlchemy models and session helpers (User, Machine, Reservation)
   │  ├─ auth.py                    # JWT utils, decorators, default admin seeding
   │  ├─ utils.py                   # Helpers (e.g., sha512 hash with openssl)
   │  └─ playbooks/
   │     └─ create-users.yml
   └─ reverse-proxy/
      └─ Caddyfile
```

---

## Configuration

Environment variables (not exhaustive):
- DATABASE_URL: SQLAlchemy DB URL (e.g., mysql+pymysql://appuser:apppass@127.0.0.1:3306/containers?charset=utf8mb4)
- DB_CONNECT_MAX_RETRIES, DB_CONNECT_RETRY_SECONDS
- JWT_SECRET: long random string for signing JWTs (required in prod)
- JWT_EXPIRES_MINUTES: token lifetime (default: 60)
- ADMIN_DEFAULT_USERNAME: default seeded admin username (default: admin)
- ADMIN_DEFAULT_PASSWORD: default seeded admin password (default: change-me)
- LOG_LEVEL, GUNICORN settings, etc.

The API container preloads the app so initialization (including default admin seeding) runs once:
- WEB_PRELOAD_APP=true in src/docker-compose.yml

---

## Getting started

Dev (hot reload):
```
just run-dev
```

Run without dev override:
```
just run
```

Follow logs:
```
just logs
```

Teardown and cleanup:
```
just clean
```

---

## Authentication

- POST /login issues a JWT. Include it as Authorization: Bearer <token> for protected endpoints.
- Roles:
  - Admin: can manage machines and users; can release_all.
  - User: can list machines and make reservations.

Seeded admin (dev):
- The API seeds a default admin on first boot using ADMIN_DEFAULT_USERNAME/ADMIN_DEFAULT_PASSWORD.

Example: obtain a token (dev uses self-signed cert; note -k)
```bash
curl -k -X POST https://localhost/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"change-me"}'
# => { "access_token":"<JWT>", "token_type":"Bearer", "expires_in":3600 }
```

Check token:
```bash
TOKEN=<paste token>
curl -k -H "Authorization: Bearer $TOKEN" https://localhost/whoami
```

---

## API

Base URL (through the reverse proxy): https://localhost

Use -k with curl locally because the example uses a self-signed certificate.

### Health
- GET /healthz → {"status":"ok"}
```bash
curl -k https://localhost/healthz
```

### Auth
- POST /login → obtain JWT
  - Body: {"username":"...", "password":"..."}
- GET /whoami → verify token
```bash
curl -k -X POST https://localhost/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"change-me"}'

TOKEN=<JWT>
curl -k -H "Authorization: Bearer $TOKEN" https://localhost/whoami
```

- Admin user management (admin only)
  - POST /users → create user
    - Body: {"username":"u","password":"p","is_admin":false}
  - GET /users → list users
  - DELETE /users/<username> → delete user

```bash
# Create a non-admin user
curl -k -X POST https://localhost/users \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"username":"alice","password":"s3cret","is_admin":false}'
```

### Machines
- POST /machines (admin) → register a machine
```bash
curl -k -X POST https://localhost/machines \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"name":"alpine-1","host":"localhost","port":2221,"user":"root","password":"test"}'
```

- GET /machines (auth) → list machines
```bash
curl -k -H "Authorization: Bearer $TOKEN" https://localhost/machines
```

- DELETE /machines/<name> (admin) → remove a machine
```bash
curl -k -X DELETE https://localhost/machines/alpine-1 \
  -H "Authorization: Bearer $TOKEN"
```

### Reservations
- GET /reserve (auth) → reserve N machines for a user
  - Query params:
    - username: logical username to create on target machines (defaults to the authenticated API user)
    - reservation_password: password to set on target machines (hashed with SHA-512 for Ansible) [alias: password]
    - count: number of machines (default 1)
    - duration: minutes (default 60)

```bash
curl -k -H "Authorization: Bearer $TOKEN" \
  "https://localhost/reserve?count=2&duration=60&reservation_password=test"
```

- GET /release_all (admin) → release all machines and delete users
```bash
curl -k -H "Authorization: Bearer $TOKEN" https://localhost/release_all
```

- GET /available (auth) → current available/reserved pool status
```bash
curl -k -H "Authorization: Bearer $TOKEN" https://localhost/available
```

- GET /reservations (auth) → list active reservations and time remaining
```bash
curl -k -H "Authorization: Bearer $TOKEN" https://localhost/reservations
```

In production, configure the reverse proxy with a trusted certificate and remove -k.

---

## Registering machines (admin)

The helper script logs in with the seeded admin and registers machines from YAML.

Usage:
```bash
python utils/register_machines.py provision/machines.yml
```

YAML format:
```yaml
machines:
  - name: alpine-1
    host: localhost
    port: 2221
    user: root
    password: test
  - name: alpine-2
    host: localhost
    port: 2222
    user: root
    password: test
```

The script:
- Calls POST /login using ADMIN_DEFAULT_USERNAME/ADMIN_DEFAULT_PASSWORD from the environment
- Uses the returned JWT to call POST /machines for each entry

Environment defaults (override as needed):
- API_BASE=https://localhost
- ADMIN_DEFAULT_USERNAME=admin
- ADMIN_DEFAULT_PASSWORD=change-me
- VERIFY_TLS=false (set true if you have a trusted cert)

---

## Production notes

- Always set a strong JWT_SECRET and rotate periodically.
- Keep tokens short-lived (e.g., 15–60 minutes); consider refresh tokens if needed.
- Enforce HTTPS; remove -k from curl examples and enable TLS verification.
- Restrict admin credentials and consider 2FA for admin accounts if you build a UI.
- Rate-limit /login and audit sensitive admin actions.
