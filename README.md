# Projet-IAC

# Container Manager API

A dynamic container assignment system using Docker, Ansible, and Flask.

---

## **Requirements**
- **Docker**: Installed and running on the host.
- **Ansible**: Installed with the `community.docker` collection.
- **Python 3.9+**: With `venv` support.
- **Just**: Command runner (`cargo install just` or `brew install just`).

---

## **Installation**
1. **Install dependencies**:
   ```bash
   just setup
   ```

2. **Provision Docker containers** (using Ansible):
   ```bash
   just provision
   ```

---

## **Usage**
### **Run the API**
```bash
just run
```

### **API Endpoints**
| Endpoint | Description |
|----------|-------------|
| `GET /reserve?username=<name>&password=<pass>&count=<n>` | Reserve `n` containers for a user. |
| `GET /release_all` | Release all containers. |

**Example**:
```bash
curl "http://localhost:8080/reserve?username=alice&password=test&count=2"
```

---

## **Commands**
| Command | Description |
|---------|-------------|
| `just setup` | Install dependencies and set up the virtual environment. |
| `just run` | Start the Flask API. |
| `just provision` | Provision Docker containers using Ansible. |
| `just test-playbook username="dog" password="test" count=2` | Test the user creation playbook. |
| `just clean` | Remove the virtual environment and database. |

---

## **Project Structure**
```
.
├── src/
│   ├── api.py            # Flask API
│   ├── create-users.yml  # Ansible playbook
│   └── containers.db     # SQLite database
├── provision.yml         # Docker provisioning playbook
├── requirements.txt      # Python dependencies
├── Justfile              # Task runner
└── README.md             # This file
```

---
### **Notes**
- Ensure Docker is running before provisioning.
- The API listens on `0.0.0.0:8080`.

