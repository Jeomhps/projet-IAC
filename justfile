# Project variables
API_DIR := "src"
VENV_DIR := "venv"
REQUIREMENTS := "requirements.txt"

PROVISION_DIR := "provision"
PROVISION_PLAYBOOK := "provision/provision.yml"
UNPROVISION_PLAYBOOK := "provision/unprovision.yml"
#DOCKERFILE := "{{PROVISION_DIR}}/Dockerfile"

# Install dependencies and set up venv
setup:
    python3 -m venv {{VENV_DIR}}
    . {{VENV_DIR}}/bin/activate && \
    pip install -r {{REQUIREMENTS}}

# Run the API
run:
    . {{VENV_DIR}}/bin/activate && \
    python3 {{API_DIR}}/api.py

run-detached:
    . {{VENV_DIR}}/bin/activate && \
    nohup python3 {{API_DIR}}/api.py > api.log 2>&1 &

# Provisionning containers
provision:
    ansible-playbook {{PROVISION_PLAYBOOK}}

unprovision:
    ansible-playbook {{UNPROVISION_PLAYBOOK}}

reprovision:
    just unprovision
    just provision

register-machine:
  . {{VENV_DIR}}/bin/activate && \
  python3 utils/register_machines.py \
    provision/machines.txt

# Clean up
clean:
    rm -rf {{VENV_DIR}} {{API_DIR}}/containers.db api.log

full-setup:
    just setup
    just provision
    just run-detached
    sleep 3
    just register-machine

full-clean:
    just unprovision
    pkill -f "{{API_DIR}}/api.py" || true
    just clean

watch:
  tail -f api.log
