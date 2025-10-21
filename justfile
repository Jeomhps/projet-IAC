# Project variables
API_DIR := "src"
VENV_DIR := "venv"
REQUIREMENTS := "requirements.txt"
PLAYBOOK := "src/create-users.yml"
PROVISION_PLAYBOOK := "provision.yml"

# Install dependencies and set up venv
setup:
    python3 -m venv {{VENV_DIR}}
    . {{VENV_DIR}}/bin/activate && pip install -r {{REQUIREMENTS}}

# Run the API
run:
    . {{VENV_DIR}}/bin/activate && python3 {{API_DIR}}/api.py

# Provision containers (using inventory.ini)
provision:
    ansible-playbook {{PROVISION_PLAYBOOK}}

# Clean up
clean:
    rm -rf {{VENV_DIR}} {{API_DIR}}/containers.db
