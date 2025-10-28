from flask import Blueprint, request, jsonify, g
from datetime import datetime, timedelta
import tempfile
import subprocess
import os
import logging
from pathlib import Path
from sqlalchemy import and_
from common.db import get_session, Machine, Reservation, User
from common.utils import sha512_hash_openssl
from common.auth import require_auth

logger = logging.getLogger(__name__)
reservations_bp = Blueprint('reservations', __name__)

def _playbook_path() -> str:
    return str((Path(__file__).resolve().parent.parent / "common" / "playbooks" / "create-users.yml").resolve())

def _eligible_machines(session):
    return session.query(Machine).filter(and_(Machine.enabled == True, Machine.online == True, Machine.reserved == False)).all()  # noqa: E712

def _inventory_lines(machines):
    for m in machines:
        yield f"{m.name} ansible_host={m.host} ansible_port={m.port} ansible_user={m.user} ansible_password={m.password}\n"

def _is_admin() -> bool:
    return "admin" in (getattr(g, "current_roles", []) or [])

@reservations_bp.get("/reservations")
@require_auth()
def list_reservations():
    session = get_session()
    try:
        q = (
            session.query(
                Reservation.id,
                Reservation.username,
                Reservation.reserved_until,
                Machine.name,
                Machine.host,
                Machine.port
            )
            .join(Machine, Machine.id == Reservation.machine_id)
        )

        admin = _is_admin()
        current_user = getattr(g, "current_user", None)
        if admin:
            username_filter = request.args.get("username")
            machine_name = request.args.get("machine")
            if username_filter:
                q = q.filter(Reservation.username == username_filter)
            if machine_name:
                q = q.filter(Machine.name == machine_name)
        else:
            q = q.filter(Reservation.username == current_user)

        now = datetime.utcnow()
        rows = q.all()
        items = []
        for res_id, username, reserved_until, machine_name, host, port in rows:
            seconds_remaining = None
            if reserved_until is not None:
                seconds_remaining = max(0, int((reserved_until - now).total_seconds()))
            if admin:
                items.append({
                    "id": res_id,
                    "username": username,
                    "machine": machine_name,
                    "host": host,
                    "port": port,
                    "reserved_until": reserved_until.isoformat() if reserved_until else None,
                    "seconds_remaining": seconds_remaining,
                })
            else:
                items.append({
                    # no id, no username for non-admin
                    "machine": machine_name,
                    "host": host,
                    "port": port,
                    "reserved_until": reserved_until.isoformat() if reserved_until else None,
                    "seconds_remaining": seconds_remaining,
                })
        logger.info("reservations.list admin=%s count=%d", admin, len(items))
        return jsonify({"reservations": items}), 200
    finally:
        session.close()

@reservations_bp.post("/reservations")
@require_auth()
def create_reservation():
    data = request.get_json(silent=True) or {}
    count = int(data.get("count", 1))
    duration = int(data.get("duration_minutes", 60))
    reservation_password = data.get("reservation_password") or data.get("password")
    # Do not allow overriding username in body; always use the authenticated API user
    if "username" in data:
        return jsonify({"error": "invalid_request", "message": "username must not be provided"}), 400
    if not reservation_password:
        return jsonify({"error": "invalid_request", "message": "reservation_password is required"}), 400

    username = getattr(g, "current_user", None)
    if not username:
        return jsonify({"error": "invalid_request", "message": "username could not be determined"}), 400

    session = get_session()
    try:
        user = session.query(User).filter(User.username == username).one_or_none()
        if not user:
            logger.error("reservations.create user not found in DB: %s", username)
            return jsonify({"error": "server_error", "message": "user not found"}), 500

        available = _eligible_machines(session)
        if len(available) < count:
            logger.warning("reservations.create insufficient available requested=%d available=%d user=%s", count, len(available), username)
            return jsonify({"error": "not_enough_available", "available": len(available)}), 409

        reserved = available[:count]
        reserved_until = (datetime.utcnow() + timedelta(minutes=duration))
        hashed_password = sha512_hash_openssl(reservation_password)

        playbook_path = _playbook_path()
        with tempfile.NamedTemporaryFile(mode='w', delete=False) as temp_inventory:
            for line in _inventory_lines(reserved):
                temp_inventory.write(line)
            temp_inventory_path = temp_inventory.name

        try:
            result = subprocess.run([
                "ansible-playbook",
                "-i", temp_inventory_path,
                playbook_path,
                "--extra-vars", f"username={username} hashed_password={hashed_password} user_action=create"
            ], capture_output=True, text=True)
            if result.returncode != 0:
                logger.error("reservations.create ansible_failed user=%s stderr=%s", username, result.stderr)
                return jsonify({"error": "ansible_failed", "details": result.stderr}), 500

            conn = []
            for m in reserved:
                session.add(Reservation(machine_id=m.id, user_id=user.id, username=username, reserved_until=reserved_until))
                m.reserved = True
                m.reserved_by = username
                m.reserved_until = reserved_until
                conn.append({"machine": m.name, "host": m.host, "port": m.port})

            session.commit()
            logger.info("reservations.create ok user=%s count=%d until=%s", username, count, reserved_until.isoformat())
            # KISS: do not return username; caller already knows who they are
            return jsonify({
                "machines": conn,
                "reserved_until": reserved_until.isoformat(timespec="seconds"),
                "duration_minutes": duration
            }), 201
        finally:
            if os.path.exists(temp_inventory_path):
                os.unlink(temp_inventory_path)
    finally:
        session.close()

@reservations_bp.get("/reservations/<int:reservation_id>")
@require_auth()
def get_reservation(reservation_id: int):
    session = get_session()
    try:
        admin = _is_admin()
        current_user = getattr(g, "current_user", None)
        row = (
            session.query(
                Reservation.id,
                Reservation.username,
                Reservation.reserved_until,
                Machine.name,
                Machine.host,
                Machine.port
            )
            .join(Machine, Machine.id == Reservation.machine_id)
            .filter(Reservation.id == reservation_id)
            .one_or_none()
        )
        if not row:
            return jsonify({"error": "not_found"}), 404

        res_id, username, reserved_until, machine_name, host, port = row
        if not admin and username != current_user:
            return jsonify({"error": "forbidden"}), 403

        logger.info("reservations.get id=%d admin=%s user=%s", res_id, admin, current_user)
        if admin:
            return jsonify({
                "id": res_id,
                "username": username,
                "machine": machine_name,
                "host": host,
                "port": port,
                "reserved_until": reserved_until.isoformat() if reserved_until else None
            }), 200
        # Non-admin: hide id and username
        return jsonify({
            "machine": machine_name,
            "host": host,
            "port": port,
            "reserved_until": reserved_until.isoformat() if reserved_until else None
        }), 200
    finally:
        session.close()

@reservations_bp.delete("/reservations/<int:reservation_id>")
@require_auth()
def delete_reservation(reservation_id: int):
    session = get_session()
    try:
        admin = _is_admin()
        current_user = getattr(g, "current_user", None)

        res = session.query(Reservation).filter(Reservation.id == reservation_id).one_or_none()
        if not res:
            return jsonify({"error": "not_found"}), 404
        if not admin and res.username != current_user:
            return jsonify({"error": "forbidden"}), 403

        m = session.query(Machine).filter(Machine.id == res.machine_id).one_or_none()
        if m:
            playbook_path = _playbook_path()
            with tempfile.NamedTemporaryFile(mode='w', delete=False) as temp_inventory:
                temp_inventory.write(f"{m.name} ansible_host={m.host} ansible_port={m.port} ansible_user={m.user} ansible_password={m.password}\n")
                temp_inventory_path = temp_inventory.name
            try:
                result = subprocess.run([
                    "ansible-playbook",
                    "-i", temp_inventory_path,
                    playbook_path,
                    "--extra-vars", f"username={res.username} user_action=delete"
                ], capture_output=True, text=True)
                if result.returncode != 0:
                    logger.warning("reservations.delete ansible_warn id=%d user=%s stderr=%s", res.id, res.username, result.stderr)
            finally:
                if os.path.exists(temp_inventory_path):
                    os.unlink(temp_inventory_path)
            m.reserved = False
            m.reserved_by = None
            m.reserved_until = None

        session.delete(res)
        session.commit()
        logger.info("reservations.delete id=%d admin=%s by=%s", reservation_id, admin, current_user)
        return jsonify({"message": "deleted"}), 200
    finally:
        session.close()
