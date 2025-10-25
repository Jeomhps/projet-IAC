from flask import Blueprint, request, jsonify
import logging
from sqlalchemy.exc import IntegrityError
from common.db import get_session, Machine
from common.auth import require_auth, require_role

logger = logging.getLogger(__name__)
machines_bp = Blueprint('machines', __name__)

@machines_bp.route("/machines", methods=["POST"])
@require_role("admin")
def add_machine():
    """
    Admin-only: register a machine in the pool.
    Body JSON: { "name": "...", "host": "...", "port": 22, "user": "...", "password": "..." }
    """
    data = request.get_json(silent=True) or {}
    required = ["name", "host", "port", "user", "password"]
    missing = [k for k in required if not data.get(k)]
    if missing:
        return jsonify({"error": "invalid_request", "message": f"Missing fields: {', '.join(missing)}"}), 400

    session = get_session()
    try:
        m = Machine(
            name=str(data["name"]).strip(),
            host=str(data["host"]).strip(),
            port=int(data["port"]),
            user=str(data["user"]).strip(),
            password=str(data["password"]),
            reserved=False,
            reserved_by=None,
            reserved_until=None,
        )
        session.add(m)
        session.commit()
        logger.info("Added machine %s (%s:%s)", m.name, m.host, m.port)
        return jsonify({
            "name": m.name,
            "host": m.host,
            "port": m.port,
            "user": m.user,
            "reserved": m.reserved,
            "reserved_by": m.reserved_by,
            "reserved_until": m.reserved_until.isoformat() if m.reserved_until else None
        }), 201
    except IntegrityError:
        session.rollback()
        return jsonify({"error": "conflict", "message": "Machine with this name already exists"}), 409
    finally:
        session.close()

@machines_bp.route("/machines", methods=["GET"])
@require_auth()
def list_machines():
    """
    Authenticated: list all machines and their reservation status.
    """
    session = get_session()
    try:
        machines = session.query(Machine).order_by(Machine.name.asc()).all()
        return jsonify([
            {
                "name": m.name,
                "host": m.host,
                "port": m.port,
                "user": m.user,
                "reserved": m.reserved,
                "reserved_by": m.reserved_by,
                "reserved_until": m.reserved_until.isoformat() if m.reserved_until else None,
            }
            for m in machines
        ]), 200
    finally:
        session.close()

@machines_bp.route("/machines/<name>", methods=["DELETE"])
@require_role("admin")
def delete_machine(name: str):
    """
    Admin-only: delete a machine from the pool.
    Disallows deleting a machine that is currently reserved.
    """
    session = get_session()
    try:
        m = session.query(Machine).filter(Machine.name == name).one_or_none()
        if not m:
            return jsonify({"error": "not_found", "message": "Machine not found"}), 404
        if m.reserved:
            return jsonify({"error": "bad_request", "message": "Machine is currently reserved; release first"}), 400

        session.delete(m)
        session.commit()
        logger.info("Deleted machine %s", name)
        return jsonify({"message": "deleted"}), 200
    finally:
        session.close()
