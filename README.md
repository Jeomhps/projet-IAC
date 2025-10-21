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

| Endpoint | Description | Example |
|----------|-------------|---------|
| `GET /reserve` | Reserve containers for a user | `curl "http://localhost:8080/reserve?username=alice&password=test&count=2&duration=60"` |
| `GET /release_all` | Release all containers and delete users | `curl "http://localhost:8080/release_all"` |
| `GET /available` | List available and reserved containers | `curl "http://localhost:8080/available"` |
| `GET /reservations` | List all active reservations with expiration times | `curl "http://localhost:8080/reservations"` |

#### **Reserve Containers**
```bash
# Reserve 2 containers for 60 minutes (default)
curl "http://localhost:8080/reserve?username=alice&password=test&count=2"

# Reserve 1 container for 5 minutes (for testing)
curl "http://localhost:8080/reserve?username=test&password=test&count=1&duration=5"
```

**Parameters**:
- `username`: User to create (required)
- `password`: Password for the user (default: "test")
- `count`: Number of containers to reserve (default: 1)
- `duration`: Reservation duration in minutes (default: 60)

#### **Check Available Containers**
```bash
curl "http://localhost:8080/available"
```
**Response**:
```json
{
  "available": ["alpine-1", "alpine-2", ...],
  "reserved": ["alpine-3", "alpine-4", ...]
}
```

#### **List Active Reservations**
```bash
curl "http://localhost:8080/reservations"
```
**Response**:
```json
{
  "reservations": [
    {
      "username": "alice",
      "container": "alpine-1",
      "expiration_time": "2023-11-15 14:30:00",
      "time_remaining": "58.5 minutes remaining"
    },
    ...
  ]
}
```

#### **Release All Containers**
```bash
curl "http://localhost:8080/release_all"
```
**Response**:
```json
{
  "status": "success",
  "message": "All containers released"
}
```

---
## **Features**
- **Automatic User Cleanup**: Users are automatically deleted when reservations expire
- **Custom Duration**: Set reservation duration (default: 60 minutes, minimum: 1 minute)
- **Real-time Monitoring**: Check active reservations and time remaining
- **Automatic Scheduling**: Background job checks for expired reservations every minute

---
## **Commands**
| Command | Description |
|---------|-------------|
| `just setup` | Install dependencies and set up the virtual environment |
| `just run` | Start the Flask API |
| `just provision` | Provision Docker containers using Ansible |
| `just test-playbook username="test" password="test" count=1` | Test the user creation playbook |
| `just clean` | Remove the virtual environment and database |

---
## **Project Structure**
```
.
├── src/
│   ├── api.py            # Flask API (updated with proper timestamp handling)
│   ├── create-users.yml  # Ansible playbook for user management
│   └── containers.db     # SQLite database (stores reservations with proper timestamps)
├── inventory.ini         # Ansible inventory file
├── provision.yml         # Docker provisioning playbook
├── requirements.txt      # Python dependencies
├── Justfile              # Task runner
└── README.md             # This file
```

---
## **Testing the Automatic Deletion**
1. Reserve a container for 2 minutes:
   ```bash
   curl "http://localhost:8080/reserve?username=test&password=test&count=1&duration=2"
   ```

2. Check the reservation:
   ```bash
   curl "http://localhost:8080/reservations"
   ```

3. After 2 minutes, verify the user is automatically deleted:
   ```bash
   curl "http://localhost:8080/reservations"  # Should show empty list
   curl "http://localhost:8080/available"     # Should show the container as available
   ```
