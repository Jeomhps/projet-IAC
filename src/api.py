from flask import Flask, jsonify, request
import subprocess
import sqlite3
import tempfile
from datetime import datetime, timedelta
from apscheduler.schedulers.background import BackgroundScheduler

app = Flask(__name__)

# Initialize SQLite DB
conn = sqlite3.connect("containers.db", check_same_thread=False)
cursor = conn.cursor()
cursor.execute("""
CREATE TABLE IF NOT EXISTS reservations (
    username TEXT,
    container_name TEXT,
    reserved_until TIMESTAMP,
    PRIMARY KEY (username, container_name)
)
""")
conn.commit()

# Container pool
CONTAINER_POOL = [f"alpine-{i}" for i in range(1, 11)]

@app.route("/reserve")
def reserve_containers():
    username = request.args.get("username")
    password = request.args.get("password", "test")
    count = int(request.args.get("count", 1))

    # Check available containers
    cursor.execute("SELECT container_name FROM reservations WHERE reserved_until > datetime('now')")
    reserved_containers = [row[0] for row in cursor.fetchall()]
    available_containers = [c for c in CONTAINER_POOL if c not in reserved_containers]

    if len(available_containers) < count:
        return jsonify({"error": f"Only {len(available_containers)} containers available"}), 400

    # Reserve containers
    reserved = available_containers[:count]
    reserved_until = (datetime.now() + timedelta(minutes=60)).strftime("%Y-%m-%d %H:%M:%S")

    # Create a temporary inventory file
    with tempfile.NamedTemporaryFile(mode='w', delete=False) as temp_inventory:
        for container in reserved:
            port = 2220 + int(container.split('-')[-1])
            temp_inventory.write(f"{container} ansible_host=localhost ansible_port={port} ansible_user=root ansible_password=test\n")
        temp_inventory_path = temp_inventory.name

    try:
        # Run Ansible with the temporary inventory file
        result = subprocess.run([
            "ansible-playbook",
            "-i", temp_inventory_path,
            "create-users.yml",
            "--extra-vars", f"username={username} password={password}"
        ], capture_output=True, text=True)

        print("Ansible output:", result.stdout)
        if result.returncode != 0:
            print("Ansible error:", result.stderr)
            return jsonify({"error": "Failed to create user", "details": result.stderr}), 500

        # Save reservations
        for container in reserved:
            cursor.execute(
                "INSERT OR REPLACE INTO reservations VALUES (?, ?, ?)",
                (username, container, reserved_until)
            )
        conn.commit()

        # Return connection details
        connection_details = [
            {"container": c, "host": "localhost", "port": 2220 + int(c.split('-')[-1])}
            for c in reserved
        ]

        return jsonify({
            "status": "success",
            "username": username,
            "password": password,
            "containers": connection_details,
            "reserved_until": reserved_until
        })
    finally:
        # Clean up the temporary file
        import os
        os.unlink(temp_inventory_path)

@app.route("/release_all")
def release_all_containers():
    cursor.execute("DELETE FROM reservations")
    conn.commit()
    return jsonify({"status": "success", "message": "All containers released"})

if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8080)
