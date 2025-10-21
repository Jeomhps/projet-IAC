import logging
from flask import Flask, jsonify, request
import subprocess
import sqlite3
import tempfile
import os
from datetime import datetime, timedelta
from apscheduler.schedulers.background import BackgroundScheduler
import atexit

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = Flask(__name__)

# Get the directory where this script is located
script_dir = os.path.dirname(os.path.abspath(__file__))

# Initialize SQLite DB in the script directory
db_path = os.path.join(script_dir, "containers.db")
conn = sqlite3.connect(db_path, check_same_thread=False)
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

CONTAINER_POOL = [f"alpine-{i}" for i in range(1, 11)]
playbook_path = os.path.join(script_dir, "create-users.yml")

scheduler = BackgroundScheduler()
scheduler.start()

@atexit.register
def shutdown_scheduler():
    scheduler.shutdown()

def sha512_hash_openssl(password):
    result = subprocess.run(['openssl', 'passwd', '-6', password], capture_output=True, text=True)
    return result.stdout.strip()

def delete_expired_users():
    with app.app_context():
        logger.info(f"[{datetime.now()}] Checking for expired reservations...")

        cursor.execute("""
            SELECT username, container_name
            FROM reservations
            WHERE reserved_until <= datetime('now', 'localtime')
        """)
        expired_reservations = cursor.fetchall()

        if expired_reservations:
            logger.info(f"[{datetime.now()}] Found {len(expired_reservations)} expired reservations")

            users_to_delete = {}
            for username, container in expired_reservations:
                users_to_delete.setdefault(username, []).append(container)

            for username, containers in users_to_delete.items():
                with tempfile.NamedTemporaryFile(mode='w', delete=False) as temp_inventory:
                    for container in containers:
                        port = 2220 + int(container.split('-')[-1])
                        temp_inventory.write(f"{container} ansible_host=localhost ansible_port={port} ansible_user=root ansible_password=test\n")
                    temp_inventory_path = temp_inventory.name

                try:
                    result = subprocess.run([
                        "ansible-playbook",
                        "-i", temp_inventory_path,
                        playbook_path,
                        "--extra-vars", f"username={username} user_action=delete"
                    ], capture_output=True, text=True)

                    logger.info(f"[{datetime.now()}] Deleting user {username} from containers: {containers}")
                    logger.info("Ansible output:\n%s", result.stdout)
                    if result.returncode != 0:
                        logger.error("Ansible error:\n%s", result.stderr)

                finally:
                    if os.path.exists(temp_inventory_path):
                        os.unlink(temp_inventory_path)

                for container in containers:
                    cursor.execute("""
                        DELETE FROM reservations
                        WHERE username = ? AND container_name = ?
                    """, (username, container))
                conn.commit()
        else:
            logger.info(f"[{datetime.now()}] No expired reservations found")

scheduler.add_job(delete_expired_users, 'interval', minutes=1)

@app.route("/reserve")
def reserve_containers():
    username = request.args.get("username")
    password = request.args.get("password", "test")
    count = int(request.args.get("count", 1))
    duration = int(request.args.get("duration", 60))

    hashed_password = sha512_hash_openssl(password)

    cursor.execute("""
        SELECT container_name
        FROM reservations
        WHERE reserved_until > datetime('now', 'localtime')
    """)
    reserved_containers = [row[0] for row in cursor.fetchall()]
    available_containers = [c for c in CONTAINER_POOL if c not in reserved_containers]

    if len(available_containers) < count:
        return jsonify({"error": f"Only {len(available_containers)} containers available"}), 400

    reserved = available_containers[:count]
    reserved_until = datetime.now() + timedelta(minutes=duration)

    with tempfile.NamedTemporaryFile(mode='w', delete=False) as temp_inventory:
        for container in reserved:
            port = 2220 + int(container.split('-')[-1])
            temp_inventory.write(f"{container} ansible_host=localhost ansible_port={port} ansible_user=root ansible_password=test\n")
        temp_inventory_path = temp_inventory.name

    try:
        result = subprocess.run([
            "ansible-playbook",
            "-i", temp_inventory_path,
            playbook_path,
            "--extra-vars", f"username={username} hashed_password={hashed_password} user_action=create"
        ], capture_output=True, text=True)

        logger.info(f"[{datetime.now()}] Creating user {username} on containers: {reserved}")
        logger.info("Ansible output:\n%s", result.stdout)
        if result.returncode != 0:
            logger.error("Ansible error:\n%s", result.stderr)
            return jsonify({"error": "Failed to create user", "details": result.stderr}), 500

        for container in reserved:
            cursor.execute("""
                INSERT OR REPLACE INTO reservations
                VALUES (?, ?, ?)
            """, (username, container, reserved_until))
        conn.commit()

        connection_details = [
            {"container": c, "host": "localhost", "port": 2220 + int(c.split('-')[-1])}
            for c in reserved
        ]

        return jsonify({
            "status": "success",
            "username": username,
            "password": password,
            "containers": connection_details,
            "reserved_until": reserved_until.isoformat(),
            "duration_minutes": duration
        })
    finally:
        if 'temp_inventory_path' in locals() and os.path.exists(temp_inventory_path):
            os.unlink(temp_inventory_path)

@app.route("/release_all")
def release_all_containers():
    cursor.execute("SELECT username, container_name FROM reservations")
    all_reservations = cursor.fetchall()

    if not all_reservations:
        return jsonify({"status": "success", "message": "No containers to release"})

    users_to_delete = {}
    for username, container in all_reservations:
        users_to_delete.setdefault(username, []).append(container)

    for username, containers in users_to_delete.items():
        with tempfile.NamedTemporaryFile(mode='w', delete=False) as temp_inventory:
            for container in containers:
                port = 2220 + int(container.split('-')[-1])
                temp_inventory.write(f"{container} ansible_host=localhost ansible_port={port} ansible_user=root ansible_password=test\n")
            temp_inventory_path = temp_inventory.name

        try:
            result = subprocess.run([
                "ansible-playbook",
                "-i", temp_inventory_path,
                playbook_path,
                "--extra-vars", f"username={username} user_action=delete"
            ], capture_output=True, text=True)

            logger.info(f"[{datetime.now()}] Deleting user {username} from containers: {containers}")
            logger.info("Ansible output:\n%s", result.stdout)
            if result.returncode != 0:
                logger.error("Ansible error:\n%s", result.stderr)

        finally:
            if os.path.exists(temp_inventory_path):
                os.unlink(temp_inventory_path)

    cursor.execute("DELETE FROM reservations")
    conn.commit()

    return jsonify({"status": "success", "message": "All containers released"})

@app.route("/available")
def available_containers():
    cursor.execute("""
        SELECT container_name
        FROM reservations
        WHERE reserved_until > datetime('now', 'localtime')
    """)
    reserved_containers = [row[0] for row in cursor.fetchall()]
    available = [c for c in CONTAINER_POOL if c not in reserved_containers]
    return jsonify({"available": available, "reserved": reserved_containers})

@app.route("/reservations")
def list_reservations():
    cursor.execute("""
        SELECT username, container_name,
               datetime(reserved_until, 'unixepoch', 'localtime') as expiration_time,
               printf('%.1f', (julianday(reserved_until) - julianday('now', 'localtime')) * 24 * 60)
               || ' minutes remaining' as time_remaining
        FROM reservations
        WHERE reserved_until > datetime('now', 'localtime')
    """)
    reservations = cursor.fetchall()

    result = []
    for row in reservations:
        result.append({
            "username": row[0],
            "container": row[1],
            "expiration_time": row[2],
            "time_remaining": row[3]
        })

    return jsonify({"reservations": result})

if __name__ == "__main__":
    app.run(host="0.0.0.0", port=8080)
