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

# Follow logs (no override)
logs:
    docker compose -f {{COMPOSE_FILE}} logs -f api scheduler

# Stop the stack (no override)
docker-down:
    docker compose -f {{COMPOSE_FILE}} down

# Stop and delete volumes (fresh DB) (no override)
docker-reset:
    docker compose -f {{COMPOSE_FILE}} down -v

# Provision containers with Ansible (set count and password)
# Usage:
#   just provision                # defaults to 10 containers, password "test"
#   just provision 25             # 25 containers, password "test"
#   just provision 40 mypass      # 40 containers, password "mypass"
provision count="10" password="test":
    ansible-playbook provision/provision.yml --extra-vars "container_count={{count}} docker_password={{password}}"

# Unprovision containers: remove only those listed in provision/machines.yml
unprovision:
    ansible-playbook provision/unprovision.yml

# Reprovision with a given count/password
reprovision count="10" password="test":
    just unprovision
    just provision {{count}} {{password}}

# Register machines into the API from provision/machines.yml
register-machine:
    PYTHONPATH={{SRC_DIR}} {{PY}} utils/register_machines.py provision/machines.yml

# One-shot: setup, provision, bring up Docker (no override), register
run count="10" password="test":
    just setup
    just provision {{count}} {{password}}
    just docker-up
    just register-machine

# Teardown and cleanup
clean:
    just unprovision || true
    just docker-reset || true
    rm -rf {{VENV_DIR}}
