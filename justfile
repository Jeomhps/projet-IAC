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
    . {{VENV_DIR}}/bin/activate && pip install -r {{REQUIREMENTS}}

# Run the API
run:
    . {{VENV_DIR}}/bin/activate && python3 {{API_DIR}}/api.py

# Provisionning containers
provision:
    ansible-playbook {{PROVISION_PLAYBOOK}}

unprovision:
    ansible-playbook {{UNPROVISION_PLAYBOOK}}

reprovision:
    just unprovision
    just provision

# Clean up
clean:
    rm -rf {{VENV_DIR}} {{API_DIR}}/containers.db
