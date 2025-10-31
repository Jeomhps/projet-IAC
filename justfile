# Paths
COMPOSE_FILE := "src/docker-compose.yml"
REMOTE_COMPOSE_FILE := "./docker-compose.yml"

# Bring up the Docker stack (detached)
docker-up:
    docker compose -f {{COMPOSE_FILE}} up -d --build

docker-up-remote:
    docker compose -f {{REMOTE_COMPOSE_FILE}} up -d

# Follow logs
logs:
    docker compose -f {{COMPOSE_FILE}} logs -f api scheduler

# Stop the stack
docker-down:
    docker compose -f {{COMPOSE_FILE}} down

# Stop and delete volumes
docker-reset:
    docker compose -f {{COMPOSE_FILE}} down -v

docker-reset-remote:
    docker compose -f {{REMOTE_COMPOSE_FILE}} down -v

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
    go build -C utils/register-go -o registrar
    ./utils/register-go/registrar

# One-shot: provision, bring up Docker, register
run count="10" password="test":
    just provision {{count}} {{password}}
    just docker-up
    just register-machine

run-remote count="10" password="test":
    just provision {{count}} {{password}}
    just docker-up-remote
    just register-machine

# Teardown and cleanup
clean:
    just unprovision || true
    just docker-reset || true
    rm -f ./utils/register-go/registrar

clean-remote:
    just unprovision || true
    just docker-reset-remote || true
    rm -f ./utils/register-go/registrar
