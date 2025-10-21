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

def get_available_machines():
    conn, cursor = get_conn_cursor()
    cursor.execute("SELECT id, name, host, port, user, password FROM machines WHERE reserved = 0")
    return cursor.fetchall()

def get_machine_by_id(machine_id):
    conn, cursor = get_conn_cursor()
    cursor.execute("SELECT id, name, host, port, user, password FROM machines WHERE id = ?", (machine_id,))
    return cursor.fetchone()

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
    available_machines = get_available_machines()

    if len(available_machines) < count:
        logger.warning(f"Not enough available machines for user '{username}'. Requested {count}, got {len(available_machines)}.")
        return jsonify({"error": f"Only {len(available_machines)} machines available"}), 400

    reserved = available_machines[:count]
    reserved_until = (datetime.now() + timedelta(minutes=duration)).isoformat(timespec="seconds")

    playbook_path = os.path.join(os.path.dirname(__file__), "create-users.yml")
    with tempfile.NamedTemporaryFile(mode='w', delete=False) as temp_inventory:
        for id, name, host, port, user, machine_password in reserved:
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

        connection_details = []
        for id, name, host, port, user, machine_password in reserved:
            cursor.execute(
                "INSERT INTO reservations (machine_id, username, reserved_until) VALUES (?, ?, ?)",
                (id, username, reserved_until)
            )
            cursor.execute(
                "UPDATE machines SET reserved = 1, reserved_by = ?, reserved_until = ? WHERE id = ?",
                (username, reserved_until, id)
            )
            connection_details.append({"machine": name, "host": host, "port": port})

        conn.commit()
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
    cursor.execute("SELECT r.id, r.machine_id, m.name, r.username FROM reservations r JOIN machines m ON r.machine_id = m.id")
    all_reservations = cursor.fetchall()
    if not all_reservations:
        logger.info("No machines to release.")
        return jsonify({"message": "No machines to release."}), 200

    playbook_path = os.path.join(os.path.dirname(__file__), "create-users.yml")
    users_to_delete = {}
    for res_id, machine_id, machine_name, username in all_reservations:
        users_to_delete.setdefault(username, []).append((machine_id, machine_name))

    for username, machine_tuples in users_to_delete.items():
        ids = [str(m[0]) for m in machine_tuples]
        cursor.execute(
            f"SELECT id, name, host, port, user, password FROM machines WHERE id IN ({','.join(['?']*len(ids))})",
            ids,
        )
        machines_info = cursor.fetchall()
        with tempfile.NamedTemporaryFile(mode='w', delete=False) as temp_inventory:
            for id, name, host, port, user, machine_password in machines_info:
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

        # Update machine status and remove reservation
        for machine_id, machine_name in machine_tuples:
            cursor.execute("UPDATE machines SET reserved = 0, reserved_by = NULL, reserved_until = NULL WHERE id = ?", (machine_id,))
            cursor.execute("DELETE FROM reservations WHERE machine_id = ? AND username = ?", (machine_id, username))

    conn.commit()
    logger.info("Released all machines and deleted users.")
    return jsonify({"message": "All machines released"}), 200

@reservations_bp.route("/available")
def available_containers():
    conn, cursor = get_conn_cursor()
    cursor.execute("SELECT name FROM machines WHERE reserved = 0")
    available = [row[0] for row in cursor.fetchall()]
    cursor.execute("SELECT name FROM machines WHERE reserved = 1")
    reserved = [row[0] for row in cursor.fetchall()]
    logger.info(f"Available: {len(available)}, Reserved: {len(reserved)}")
    return jsonify({
        "available": available,
        "reserved": reserved
    }), 200

@reservations_bp.route("/reservations")
def list_reservations():
    conn, cursor = get_conn_cursor()
    cursor.execute("""
        SELECT r.username, m.name, r.reserved_until
        FROM reservations r
        JOIN machines m ON r.machine_id = m.id
    """)
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
        SELECT r.id, r.machine_id, r.username, r.reserved_until, m.name, m.host, m.port, m.user, m.password
        FROM reservations r
        JOIN machines m ON r.machine_id = m.id
        WHERE r.reserved_until <= ?
    """, (datetime.now().isoformat(timespec="seconds"),))
    expired_reservations = cursor.fetchall()
    if not expired_reservations:
        logger.info("No expired reservations to clean up.")
        return

    playbook_path = os.path.join(os.path.dirname(__file__), "create-users.yml")
    users_to_delete = {}
    for res_id, machine_id, username, reserved_until, name, host, port, user, password in expired_reservations:
        users_to_delete.setdefault(username, []).append((machine_id, name, host, port, user, password, res_id))

    for username, machine_tuples in users_to_delete.items():
        with tempfile.NamedTemporaryFile(mode='w', delete=False) as temp_inventory:
            for machine_id, name, host, port, user, password, res_id in machine_tuples:
                temp_inventory.write(
                    f"{name} ansible_host={host} ansible_port={port} ansible_user={user} ansible_password={password}\n"
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

        for machine_id, name, host, port, user, password, res_id in machine_tuples:
            cursor.execute("UPDATE machines SET reserved = 0, reserved_by = NULL, reserved_until = NULL WHERE id = ?", (machine_id,))
            cursor.execute("DELETE FROM reservations WHERE id = ?", (res_id,))
    conn.commit()
    logger.info(f"Cleaned up {len(expired_reservations)} expired reservations.")
