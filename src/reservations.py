from flask import Blueprint, request, jsonify
from datetime import datetime, timedelta
import tempfile
import subprocess
import os
import logging
from db import get_conn_cursor
from utils import sha512_hash_openssl

logger = logging.getLogger(__name__)
reservations_bp = Blueprint('reservations', __name__)

def get_machine_pool():
    conn, cursor = get_conn_cursor()
    cursor.execute("SELECT name, host, port, user, password FROM machines")
    return cursor.fetchall()

@reservations_bp.route("/reserve")
def reserve_containers():
    conn, cursor = get_conn_cursor()
    username = request.args.get("username")
    password = request.args.get("password", "test")
    count = int(request.args.get("count", 1))
    duration = int(request.args.get("duration", 60))

    if not username:
        logger.warning("Username not provided for reservation.")
        return jsonify({"error": "Username is required."}), 400

    hashed_password = sha512_hash_openssl(password)
    cursor.execute("""
        SELECT container_name
        FROM reservations
        WHERE reserved_until > ?
    """, (datetime.now().isoformat(timespec="seconds"),))
    reserved_containers = [row[0] for row in cursor.fetchall()]

    machines = get_machine_pool()
    available_machines = [m for m in machines if m[0] not in reserved_containers]

    if len(available_machines) < count:
        logger.warning(f"Not enough available machines for user '{username}'. Requested {count}, got {len(available_machines)}.")
        return jsonify({"error": f"Only {len(available_machines)} machines available"}), 400

    reserved = available_machines[:count]
    reserved_names = [m[0] for m in reserved]
    reserved_until = (datetime.now() + timedelta(minutes=duration)).isoformat(timespec="seconds")

    playbook_path = os.path.join(os.path.dirname(__file__), "create-users.yml")
    with tempfile.NamedTemporaryFile(mode='w', delete=False) as temp_inventory:
        for name, host, port, user, machine_password in reserved:
            temp_inventory.write(
                f"{name} ansible_host={host} ansible_port={port} ansible_user={user} ansible_password={machine_password}\n"
            )
        temp_inventory_path = temp_inventory.name

    try:
        result = subprocess.run([
            "ansible-playbook",
            "-i", temp_inventory_path,
            playbook_path,
            "--extra-vars", f"username={username} hashed_password={hashed_password} user_action=create"
        ], capture_output=True, text=True)

        if result.returncode != 0:
            logger.error(f"Ansible failed for user '{username}': {result.stderr}")
            return jsonify({"error": "Failed to create user", "ansible_error": result.stderr}), 500

        for name in reserved_names:
            cursor.execute(
                "INSERT OR REPLACE INTO reservations VALUES (?, ?, ?)",
                (username, name, reserved_until)
            )
        conn.commit()
        connection_details = [
            {"machine": name, "host": host, "port": port}
            for name, host, port, user, machine_password in reserved
        ]
        logger.info(f"Reserved {count} machines for user '{username}' until {reserved_until}")
        return jsonify({
            "username": username,
            "machines": connection_details,
            "reserved_until": reserved_until,
            "duration_minutes": duration,
            "password": password
        }), 200
    finally:
        if os.path.exists(temp_inventory_path):
            os.unlink(temp_inventory_path)

@reservations_bp.route("/release_all")
def release_all_containers():
    conn, cursor = get_conn_cursor()
    cursor.execute("SELECT username, container_name FROM reservations")
    all_reservations = cursor.fetchall()
    if not all_reservations:
        logger.info("No machines to release.")
        return jsonify({"message": "No machines to release."}), 200

    users_to_delete = {}
    for username, container in all_reservations:
        users_to_delete.setdefault(username, []).append(container)

    playbook_path = os.path.join(os.path.dirname(__file__), "create-users.yml")
    for username, containers in users_to_delete.items():
        cursor.execute(
            "SELECT name, host, port, user, password FROM machines WHERE name IN ({})".format(
                ",".join("?" for _ in containers)
            ),
            containers,
        )
        machines_info = cursor.fetchall()
        with tempfile.NamedTemporaryFile(mode='w', delete=False) as temp_inventory:
            for name, host, port, user, machine_password in machines_info:
                temp_inventory.write(
                    f"{name} ansible_host={host} ansible_port={port} ansible_user={user} ansible_password={machine_password}\n"
                )
            temp_inventory_path = temp_inventory.name

        try:
            result = subprocess.run([
                "ansible-playbook",
                "-i", temp_inventory_path,
                playbook_path,
                "--extra-vars", f"username={username} user_action=delete"
            ], capture_output=True, text=True)
            if result.returncode != 0:
                logger.warning(f"Ansible error when deleting user '{username}': {result.stderr}")
        finally:
            if os.path.exists(temp_inventory_path):
                os.unlink(temp_inventory_path)

    cursor.execute("DELETE FROM reservations")
    conn.commit()
    logger.info("Released all machines and deleted users.")
    return jsonify({"message": "All machines released"}), 200

@reservations_bp.route("/available")
def available_containers():
    conn, cursor = get_conn_cursor()
    cursor.execute("""
        SELECT container_name
        FROM reservations
        WHERE reserved_until > ?
    """, (datetime.now().isoformat(timespec="seconds"),))
    reserved_containers = [row[0] for row in cursor.fetchall()]
    machines = get_machine_pool()
    available = [m[0] for m in machines if m[0] not in reserved_containers]
    logger.info(f"Available: {len(available)}, Reserved: {len(reserved_containers)}")
    return jsonify({
        "available": available,
        "reserved": reserved_containers
    }), 200

@reservations_bp.route("/reservations")
def list_reservations():
    conn, cursor = get_conn_cursor()
    cursor.execute("""
        SELECT username, container_name, reserved_until
        FROM reservations
        WHERE reserved_until > ?
    """, (datetime.now().isoformat(timespec="seconds"),))
    reservations = cursor.fetchall()
    result = []
    now = datetime.now()
    for row in reservations:
        username, machine, expiration_time = row
        expiration_dt = datetime.fromisoformat(expiration_time)
        minutes_remaining = (expiration_dt - now).total_seconds() / 60
        result.append({
            "username": username,
            "machine": machine,
            "expiration_time": expiration_time,
            "time_remaining": f"{minutes_remaining:.1f} minutes remaining"
        })
    logger.info(f"Listed {len(result)} reservations.")
    return jsonify({"reservations": result}), 200

def handle_expired_reservations():
    conn, cursor = get_conn_cursor()
    cursor.execute("""
        SELECT username, container_name
        FROM reservations
        WHERE reserved_until <= ?
    """, (datetime.now().isoformat(timespec="seconds"),))
    expired_reservations = cursor.fetchall()
    if not expired_reservations:
        logger.info("No expired reservations to clean up.")
        return

    users_to_delete = {}
    for username, container in expired_reservations:
        users_to_delete.setdefault(username, []).append(container)

    playbook_path = os.path.join(os.path.dirname(__file__), "create-users.yml")
    for username, containers in users_to_delete.items():
        cursor.execute(
            "SELECT name, host, port, user, password FROM machines WHERE name IN ({})".format(
                ",".join("?" for _ in containers)
            ),
            containers,
        )
        machines_info = cursor.fetchall()
        with tempfile.NamedTemporaryFile(mode='w', delete=False) as temp_inventory:
            for name, host, port, user, machine_password in machines_info:
                temp_inventory.write(
                    f"{name} ansible_host={host} ansible_port={port} ansible_user={user} ansible_password={machine_password}\n"
                )
            temp_inventory_path = temp_inventory.name

        try:
            subprocess.run([
                "ansible-playbook",
                "-i", temp_inventory_path,
                playbook_path,
                "--extra-vars", f"username={username} user_action=delete"
            ], capture_output=True, text=True)
        finally:
            if os.path.exists(temp_inventory_path):
                os.unlink(temp_inventory_path)
        for container in containers:
            cursor.execute(
                "DELETE FROM reservations WHERE username = ? AND container_name = ?",
                (username, container)
            )
        conn.commit()
    logger.info(f"Cleaned up {len(expired_reservations)} expired reservations.")
