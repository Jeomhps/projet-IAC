from flask import Blueprint, request, jsonify
from datetime import datetime, timedelta
import tempfile
import subprocess
import os
import logging
from sqlalchemy import and_
from db import get_session, Machine, Reservation
from utils import sha512_hash_openssl

logger = logging.getLogger(__name__)
reservations_bp = Blueprint('reservations', __name__)

def get_available_machines(session):
    return session.query(Machine).filter(Machine.reserved == False).all()  # noqa: E712

def get_machine_by_id(session, machine_id):
    return session.query(Machine).filter(Machine.id == machine_id).one_or_none()

@reservations_bp.route("/reserve")
def reserve_containers():
    session = get_session()
    try:
        username = request.args.get("username")
        password = request.args.get("password", "test")
        count = int(request.args.get("count", 1))
        duration = int(request.args.get("duration", 60))

        if not username:
            logger.warning("Username not provided for reservation.")
            return jsonify({"error": "Username is required."}), 400

        hashed_password = sha512_hash_openssl(password)
        available_machines = get_available_machines(session)

        if len(available_machines) < count:
            logger.warning(f"Not enough available machines for user '{username}'. Requested {count}, got {len(available_machines)}.")
            return jsonify({"error": f"Only {len(available_machines)} machines available"}), 400

        reserved = available_machines[:count]
        # Use UTC for consistency across environments
        reserved_until = (datetime.utcnow() + timedelta(minutes=duration))

        playbook_path = os.path.join(os.path.dirname(__file__), "create-users.yml")
        with tempfile.NamedTemporaryFile(mode='w', delete=False) as temp_inventory:
            for m in reserved:
                temp_inventory.write(
                    f"{m.name} ansible_host={m.host} ansible_port={m.port} ansible_user={m.user} ansible_password={m.password}\n"
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
            for m in reserved:
                session.add(Reservation(machine_id=m.id, username=username, reserved_until=reserved_until))
                m.reserved = True
                m.reserved_by = username
                m.reserved_until = reserved_until
                connection_details.append({"machine": m.name, "host": m.host, "port": m.port})

            session.commit()
            logger.info(f"Reserved {count} machines for user '{username}' until {reserved_until.isoformat(timespec='seconds')} UTC")
            return jsonify({
                "username": username,
                "machines": connection_details,
                "reserved_until": reserved_until.isoformat(timespec="seconds"),
                "duration_minutes": duration,
                "password": password
            }), 200
        finally:
            if os.path.exists(temp_inventory_path):
                os.unlink(temp_inventory_path)
    finally:
        session.close()

@reservations_bp.route("/release_all")
def release_all_containers():
    session = get_session()
    try:
        # Fetch all reservations joined with machines
        res = (
            session.query(Reservation.id, Reservation.machine_id, Machine.name, Reservation.username)
            .join(Machine, Machine.id == Reservation.machine_id)
            .all()
        )
        if not res:
            logger.info("No machines to release.")
            return jsonify({"message": "No machines to release."}), 200

        playbook_path = os.path.join(os.path.dirname(__file__), "create-users.yml")
        users_to_delete = {}
        for res_id, machine_id, machine_name, username in res:
            users_to_delete.setdefault(username, []).append((res_id, machine_id, machine_name))

        for username, tuples in users_to_delete.items():
            machine_ids = [m_id for _, m_id, _ in tuples]
            machines_info = session.query(Machine).filter(Machine.id.in_(machine_ids)).all()

            with tempfile.NamedTemporaryFile(mode='w', delete=False) as temp_inventory:
                for m in machines_info:
                    temp_inventory.write(
                        f"{m.name} ansible_host={m.host} ansible_port={m.port} ansible_user={m.user} ansible_password={m.password}\n"
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
            for res_id, machine_id, _ in tuples:
                m = session.query(Machine).filter(Machine.id == machine_id).one_or_none()
                if m:
                    m.reserved = False
                    m.reserved_by = None
                    m.reserved_until = None
                session.query(Reservation).filter(Reservation.id == res_id).delete()

        session.commit()
        logger.info("Released all machines and deleted users.")
        return jsonify({"message": "All machines released"}), 200
    finally:
        session.close()

@reservations_bp.route("/available")
def available_containers():
    session = get_session()
    try:
        available = [m.name for m in session.query(Machine).filter(Machine.reserved == False).all()]  # noqa: E712
        reserved = [m.name for m in session.query(Machine).filter(Machine.reserved == True).all()]    # noqa: E712
        logger.info(f"Available: {len(available)}, Reserved: {len(reserved)}")
        return jsonify({
            "available": available,
            "reserved": reserved
        }), 200
    finally:
        session.close()

@reservations_bp.route("/reservations")
def list_reservations():
    session = get_session()
    try:
        rows = (
            session.query(Reservation.username, Machine.name, Reservation.reserved_until)
            .join(Machine, Machine.id == Reservation.machine_id)
            .all()
        )
        result = []
        now = datetime.utcnow()
        for username, machine, expiration_dt in rows:
            if expiration_dt is None:
                minutes_remaining = None
                time_remaining = "unknown"
                expiration_str = None
            else:
                minutes_remaining = (expiration_dt - now).total_seconds() / 60
                time_remaining = f"{minutes_remaining:.1f} minutes remaining"
                expiration_str = expiration_dt.isoformat(timespec="seconds")
            result.append({
                "username": username,
                "machine": machine,
                "expiration_time": expiration_str,
                "time_remaining": time_remaining
            })
        logger.info(f"Listed {len(result)} reservations.")
        return jsonify({"reservations": result}), 200
    finally:
        session.close()

def handle_expired_reservations():
    session = get_session()
    try:
        now = datetime.utcnow()
        expired = (
            session.query(
                Reservation.id,
                Reservation.machine_id,
                Reservation.username,
                Reservation.reserved_until,
                Machine.name,
                Machine.host,
                Machine.port,
                Machine.user,
                Machine.password,
            )
            .join(Machine, Machine.id == Reservation.machine_id)
            .filter(and_(Reservation.reserved_until.isnot(None), Reservation.reserved_until <= now))
            .all()
        )
        if not expired:
            logger.info("No expired reservations to clean up.")
            return

        playbook_path = os.path.join(os.path.dirname(__file__), "create-users.yml")
        users_to_delete = {}
        for res_id, machine_id, username, reserved_until, name, host, port, user, password in expired:
            users_to_delete.setdefault(username, []).append((res_id, machine_id, name, host, port, user, password))

        for username, tuples in users_to_delete.items():
            with tempfile.NamedTemporaryFile(mode='w', delete=False) as temp_inventory:
                for _, machine_id, name, host, port, user, password in tuples:
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

            for res_id, machine_id, *_ in tuples:
                m = session.query(Machine).filter(Machine.id == machine_id).one_or_none()
                if m:
                    m.reserved = False
                    m.reserved_by = None
                    m.reserved_until = None
                session.query(Reservation).filter(Reservation.id == res_id).delete()

        session.commit()
        logger.info(f"Cleaned up {len(expired)} expired reservations.")
    finally:
        session.close()
