# Paths
SRC_DIR := "src"
API_DIR := "src/api"
COMPOSE_FILE := "src/docker-compose.yml"
DEV_OVERRIDE := "src/docker-compose.dev.override.yml"
VENV_DIR := "venv"
PY := "venv/bin/python3"
PIP := "venv/bin/pip"
REQUIREMENTS := "requirements.txt"

setup:
    python3 -m venv {{VENV_DIR}}
    {{PIP}} install -r {{REQUIREMENTS}}

# Bring up the Docker stack (detached) WITHOUT override
docker-up:
    docker compose -f {{COMPOSE_FILE}} up -d --build

# Bring up the Docker stack (detached) WITH dev override
docker-up-dev:
    docker compose -f {{COMPOSE_FILE}} -f {{DEV_OVERRIDE}} up -d --build

# Follow logs (no override)
logs:
    docker compose -f {{COMPOSE_FILE}} logs -f api scheduler

# Follow logs (dev override)
logs-dev:
    docker compose -f {{COMPOSE_FILE}} -f {{DEV_OVERRIDE}} logs -f api scheduler

# Stop the stack (no override)
docker-down:
    docker compose -f {{COMPOSE_FILE}} down

# Stop the stack (dev override)
docker-down-dev:
    docker compose -f {{COMPOSE_FILE}} -f {{DEV_OVERRIDE}} down

# Stop and delete volumes (fresh DB) (no override)
docker-reset:
    docker compose -f {{COMPOSE_FILE}} down -v

# Stop and delete volumes (dev override)
docker-reset-dev:
    docker compose -f {{COMPOSE_FILE}} -f {{DEV_OVERRIDE}} down -v

# Provision containers with Ansible
provision:
    ansible-playbook provision/provision.yml

# Unprovision containers with Ansible
unprovision:
    ansible-playbook provision/unprovision.yml

reprovision:
    just unprovision
    just provision

# Register machines into the API from provision/machines.txt
register-machine:
    PYTHONPATH={{SRC_DIR}} {{PY}} utils/register_machines.py provision/machines.txt

# One-shot: setup, provision, bring up Docker (no override), register
run:
    just setup
    just provision
    just docker-up
    just register-machine

# One-shot: setup, provision, bring up Docker with dev override, register
run-dev:
    just setup
    just provision
    just docker-up-dev
    just register-machine

# Renamed clean: teardown and cleanup
clean:
    just unprovision || true
    just docker-reset || true
    just docker-reset-dev || true
    rm -rf {{VENV_DIR}}
