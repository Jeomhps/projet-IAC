from flask import Blueprint, request, jsonify, g
from datetime import datetime, timedelta
import tempfile
import subprocess
import os
import logging
from pathlib import Path
from sqlalchemy import and_
from common.db import get_session, Machine, Reservation
from common.utils import sha512_hash_openssl
from common.auth import require_auth, require_role

logger = logging.getLogger(__name__)
reservations_bp = Blueprint('reservations', __name__)

def _playbook_path() -> str:
    # api/ -> common/playbooks/create-users.yml
    return str((Path(__file__).resolve().parent.parent / "common" / "playbooks" / "create-users.yml").resolve())

def get_available_machines(session):
    return session.query(Machine).filter(Machine.reserved == False).all()  # noqa: E712

@reservations_bp.route("/reserve")
@require_auth()
def reserve_containers():
    """
    Authenticated: reserve N machines for a user (default: the authenticated API user).
    Query params:
      - username: logical username to create on target machines (defaults to current API user)
      - reservation_password OR password: password to set on target machines (hashed via SHA-512 for Ansible)
      - count: number of machines (default 1)
      - duration: minutes (default 60)
    """
    session = get_session()
    try:
        # Prefer explicit username, else default to the authenticated user
        username = request.args.get("username") or g.current_user
        # Backward compatibility: accept 'password' if 'reservation_password' not provided
        reservation_password = request.args.get("reservation_password") or request.args.get("password", "test")
        count = int(request.args.get("count", 1))
        duration = int(request.args.get("duration", 60))

        if not username:
            logger.warning("Username is required.")
            return jsonify({"error": "Username is required."}), 400

        hashed_password = sha512_hash_openssl(reservation_password)
        available_machines = get_available_machines(session)

        if len(available_machines) < count:
            logger.warning(
                "Not enough available machines for user '%s'. Requested %d, got %d.",
                username, count, len(available_machines)
            )
            return jsonify({"error": f"Only {len(available_machines)} machines available"}), 400

        reserved = available_machines[:count]
        reserved_until = (datetime.utcnow() + timedelta(minutes=duration))

        playbook_path = _playbook_path()
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
                logger.error("Ansible failed for user '%s': %s", username, result.stderr)
                return jsonify({"error": "Failed to create user", "ansible_error": result.stderr}), 500

            connection_details = []
            for m in reserved:
                session.add(Reservation(machine_id=m.id, username=username, reserved_until=reserved_until))
                m.reserved = True
                m.reserved_by = username
                m.reserved_until = reserved_until
                connection_details.append({"machine": m.name, "host": m.host, "port": m.port})

            session.commit()
            logger.info(
                "Reserved %d machines for user '%s' until %s UTC",
                count, username, reserved_until.isoformat(timespec='seconds')
            )
            return jsonify({
                "username": username,
                "machines": connection_details,
                "reserved_until": reserved_until.isoformat(timespec="seconds"),
                "duration_minutes": duration,
                "reservation_password": reservation_password
            }), 200
        finally:
            if os.path.exists(temp_inventory_path):
                os.unlink(temp_inventory_path)
    finally:
        session.close()

@reservations_bp.route("/release_all")
@require_role("admin")
def release_all_containers():
    """
    Admin-only: delete all users from all machines via Ansible and clear all reservations.
    """
    session = get_session()
    try:
        res = (
            session.query(Reservation.id, Reservation.machine_id, Machine.name, Reservation.username)
            .join(Machine, Machine.id == Reservation.machine_id)
            .all()
        )
        if not res:
            logger.info("No machines to release.")
            return jsonify({"message": "No machines to release."}), 200

        playbook_path = _playbook_path()
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
                    logger.warning("Ansible error when deleting user '%s': %s", username, result.stderr)
            finally:
                if os.path.exists(temp_inventory_path):
                    os.unlink(temp_inventory_path)

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
@require_auth()
def available_containers():
    """
    Authenticated: show available vs reserved machines.
    """
    session = get_session()
    try:
        available = [m.name for m in session.query(Machine).filter(Machine.reserved == False).all()]  # noqa: E712
        reserved = [m.name for m in session.query(Machine).filter(Machine.reserved == True).all()]    # noqa: E712
        logger.info("Available: %d, Reserved: %d", len(available), len(reserved))
        return jsonify({
            "available": available,
            "reserved": reserved
        }), 200
    finally:
        session.close()

@reservations_bp.route("/reservations")
@require_auth()
def list_reservations():
    """
    Authenticated: list active reservations with time remaining.
    """
    session = get_session()
    try:
        now = datetime.utcnow()
        rows = (
            session.query(
                Reservation.id,
                Reservation.username,
                Reservation.reserved_until,
                Machine.name,
                Machine.host,
                Machine.port
            )
            .join(Machine, Machine.id == Reservation.machine_id)
            .all()
        )

        items = []
        for res_id, username, reserved_until, machine_name, host, port in rows:
            seconds_remaining = None
            if reserved_until is not None:
                seconds_remaining = int((reserved_until - now).total_seconds())
                if seconds_remaining < 0:
                    seconds_remaining = 0

            items.append({
                "reservation_id": res_id,
                "username": username,
                "machine": machine_name,
                "host": host,
                "port": port,
                "reserved_until": reserved_until.isoformat() if reserved_until else None,
                "seconds_remaining": seconds_remaining,
            })
        return jsonify({"reservations": items}), 200
    finally:
        session.close()
