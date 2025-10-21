# Projet-IAC
# Container/Machine Manager API

A dynamic container/machine assignment system using Docker, Ansible, and Flask.  
Supports dynamic pool management, automatic user provisioning/deprovisioning, and real-time API monitoring.

---

## **Requirements**
- **Docker**: Installed and running on the host.
- **Ansible**: Installed with the `community.docker` collection.
- **Python 3.9+**: With `venv` support.
- **Just**: Command runner (`cargo install just` or `brew install just`).

---

## **Installation & Setup**

1. **Install dependencies and set up the virtual environment**:
   ```bash
   just setup
   ```

2. **Provision Docker containers or machines** (using Ansible):
   ```bash
   just provision
   ```

3. **Start the API (detached/background)**:
   ```bash
   just run-detached
   ```

4. **Register provisioned machines with the API**:
   ```bash
   just register-machine
   ```

**Or run everything with one command:**
```bash
just full-setup
```

---

## **Usage**

### **Run the API**
```bash
just run          # Foreground
just run-detached # Background (logs to api.log)
```

### **Watch API Logs**
```bash
tail -f api.log
```

---

## **API Endpoints (Dynamic Pool)**

| Endpoint         | Method | Description                         | Example |
|------------------|--------|-------------------------------------|---------|
| `/machines`      | POST   | Add a machine to the pool           | `curl -X POST -H "Content-Type: application/json" -d '{"name":"alpine-1","host":"localhost","port":2221,"user":"root","password":"test"}' http://localhost:8080/machines` |
| `/machines`      | GET    | List all machines in the pool        | `curl http://localhost:8080/machines` |
| `/machines/<name>` | DELETE | Remove a machine from the pool     | `curl -X DELETE http://localhost:8080/machines/alpine-1` |
| `/reserve`       | GET    | Reserve machines for a user         | `curl "http://localhost:8080/reserve?username=alice&password=test&count=2&duration=60"` |
| `/release_all`   | GET    | Release all machines and delete users | `curl "http://localhost:8080/release_all"` |
| `/available`     | GET    | List available and reserved machines | `curl "http://localhost:8080/available"` |
| `/reservations`  | GET    | List all active reservations with expiration times | `curl "http://localhost:8080/reservations"` |

---

### **Reserve Machines**
```bash
# Reserve 2 machines for 60 minutes (default)
curl "http://localhost:8080/reserve?username=alice&password=test&count=2"

# Reserve 1 machine for 5 minutes (for testing)
curl "http://localhost:8080/reserve?username=test&password=test&count=1&duration=5"
```

**Parameters**:
- `username`: User to create (required)
- `password`: Password for the user (default: "test")
- `count`: Number of machines to reserve (default: 1)
- `duration`: Reservation duration in minutes (default: 60)

---

### **Check Available Machines**
```bash
curl "http://localhost:8080/available"
```
**Response**:
```json
{
  "available": ["alpine-container-1", "alpine-container-2", ...],
  "reserved": ["alpine-container-3", ...]
}
```

### **List Active Reservations**
```bash
curl "http://localhost:8080/reservations"
```
**Response**:
```json
{
  "reservations": [
    {
      "username": "alice",
      "machine": "alpine-container-1",
      "expiration_time": "2025-10-21T20:30:00",
      "time_remaining": "58.5 minutes remaining"
    },
    ...
  ]
}
```

### **Release All Machines**
```bash
curl "http://localhost:8080/release_all"
```
**Response**:
```json
{
  "status": "success",
  "message": "All machines released"
}
```

---

## **Features**
- **Dynamic Pool Management**: Add/remove machines at runtime via API
- **Automatic User Cleanup**: Users automatically deleted when reservations expire
- **Custom Duration**: Set reservation duration (default: 60 minutes, minimum: 1 minute)
- **Real-time Monitoring**: Check active reservations and time remaining
- **Automatic Scheduling**: Background job checks for expired reservations every minute
- **Bulk Register Machines**: Script to POST a batch of machines from a file

---

## **Commands**
| Command            | Description                                   |
|--------------------|-----------------------------------------------|
| `just setup`       | Install dependencies and set up venv          |
| `just provision`   | Provision Docker containers/machines via Ansible |
| `just run`         | Start the Flask API (foreground)              |
| `just run-detached`| Start the Flask API (background, logs to api.log) |
| `just register-machine` | Register provisioned machines from file   |
| `just full-setup`  | Run all setup/provision/start/register steps  |
| `just unprovision` | Unprovision all containers/machines via Ansible |
| `just clean`       | Remove venv, database, and logs               |
| `just full-clean`  | Unprovision, kill API, clean everything       |

---

## **Project Structure (2025)**
```
.
├── LICENSE
├── README.md
├── justfile
├── requirements.txt
├── src/
│   ├── api.py                # Flask API (dynamic pool, timestamp fix)
│   ├── create-users.yml      # Ansible playbook for user management
│   └── requirements.txt
├── provision/
│   ├── Dockerfile
│   ├── machines.txt          # List of provisioned machines for registration
│   ├── provision.yml         # Ansible provisioning playbook
│   └── unprovision.yml       # Ansible cleanup playbook
├── utils/
│   └── register_machines.py  # Script to bulk register machines with API
└── api.log                   # API server log (when run in detached mode)
```

---

## **Testing the Automatic Deletion**
1. Reserve a machine for 2 minutes:
   ```bash
   curl "http://localhost:8080/reserve?username=test&password=test&count=1&duration=2"
   ```

2. Check the reservation:
   ```bash
   curl "http://localhost:8080/reservations"
   ```

3. After 2+ minutes, verify the user is automatically deleted:
   ```bash
   curl "http://localhost:8080/reservations"  # Should show empty list
   curl "http://localhost:8080/available"     # Should show the machine as available
   ```

---

## **Notes**
- The API pool is now dynamic: you can POST new machines to `/machines` any time.
- The `register_machines.py` script bulk-imports from the `machines.txt` file after provisioning.
- Use `tail -f api.log` to watch server logs in real-time.
- All connection info is stored in `src/containers.db`.
