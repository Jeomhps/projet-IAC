import requests
import sys
import os

API_URL = "http://localhost:8080/machines"  # Change if your API is elsewhere

def main(machines_file):
    # Resolve absolute path based on where you run the script
    machines_path = os.path.abspath(machines_file)

    if not os.path.exists(machines_path):
        print(f"Error: {machines_path} does not exist.")
        sys.exit(1)

    with open(machines_path) as f:
        for line in f:
            if not line.strip():
                continue
            parts = line.strip().split()
            if len(parts) != 5:
                print(f"Skipping invalid line: {line.strip()}")
                continue
            name, host, port, user, password = parts
            payload = {
                "name": name,
                "host": host,
                "port": int(port),
                "user": user,
                "password": password
            }
            try:
                resp = requests.post(API_URL, json=payload)
                if resp.status_code == 201:
                    print(f"Added {name}")
                else:
                    print(f"Failed to add {name}: {resp.text}")
            except requests.RequestException as e:
                print(f"Failed to add {name}: {e}")

if __name__ == "__main__":
    if len(sys.argv) != 2:
        print("Usage: python utils/register_machines.py <machines_file>")
        sys.exit(1)
    main(sys.argv[1])
