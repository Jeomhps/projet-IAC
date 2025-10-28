from flask import Blueprint, request, jsonify, g
import logging
from sqlalchemy.exc import IntegrityError
from sqlalchemy import and_
from common.db import get_session, Machine
from common.auth import require_auth, require_role

logger = logging.getLogger(__name__)
machines_bp = Blueprint('machines', __name__)

def _is_admin() -> bool:
    return "admin" in (getattr(g, "current_roles", []) or [])

def _serialize_machine(m: Machine, admin: bool):
    # Non-admin: only minimal info (privacy and security)
    data = {
        "name": m.name,
        "reserved": m.reserved,
        "reserved_until": m.reserved_until.isoformat() if m.reserved_until else None,
    }
    if admin:
        data.update({
            "host": m.host,
            "port": m.port,
            "user": m.user,
            "reserved_by": m.reserved_by,
            "enabled": m.enabled,
            "online": m.online,
            "last_seen_at": m.last_seen_at.isoformat() if m.last_seen_at else None,
        })
    return data

@machines_bp.post("/machines")
@require_role("admin")
def add_machine():
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
            enabled=True,
            online=True,
            last_seen_at=None,
        )
        session.add(m)
        session.commit()
        logger.info("machine.add name=%s host=%s port=%s", m.name, m.host, m.port)
        return jsonify(_serialize_machine(m, True)), 201
    except IntegrityError:
        session.rollback()
        return jsonify({"error": "conflict", "message": "Machine with this name already exists"}), 409
    finally:
        session.close()

@machines_bp.get("/machines")
@require_auth()
def list_machines():
    session = get_session()
    try:
        q = session.query(Machine)

        eligible = request.args.get("eligible")
        reserved = request.args.get("reserved")
        name_prefix = request.args.get("name")

        if eligible is not None:
            val = str(eligible).lower() in ("1", "true", "yes")
            if val:
                q = q.filter(and_(Machine.enabled == True, Machine.online == True, Machine.reserved == False))  # noqa: E712
            else:
                q = q.filter(~and_(Machine.enabled == True, Machine.online == True, Machine.reserved == False))  # noqa: E712
        if reserved is not None:
            val = str(reserved).lower() in ("1", "true", "yes")
            q = q.filter(Machine.reserved == val)
        if name_prefix:
            q = q.filter(Machine.name.like(f"{name_prefix}%"))

        machines = q.order_by(Machine.name.asc()).all()
        admin = _is_admin()
        out = [_serialize_machine(m, admin) for m in machines]
        logger.info("machines.list admin=%s count=%d", admin, len(out))
        return jsonify(out), 200
    finally:
        session.close()

@machines_bp.get("/machines/<name>")
@require_auth()
def get_machine(name: str):
    session = get_session()
    try:
        m = session.query(Machine).filter(Machine.name == name).one_or_none()
        if not m:
            return jsonify({"error": "not_found", "message": "Machine not found"}), 404
        admin = _is_admin()
        logger.info("machines.get name=%s admin=%s", name, admin)
        return jsonify(_serialize_machine(m, admin)), 200
    finally:
        session.close()

@machines_bp.patch("/machines/<name>")
@require_role("admin")
def update_machine(name: str):
    data = request.get_json(silent=True) or {}
    session = get_session()
    try:
        m = session.query(Machine).filter(Machine.name == name).one_or_none()
        if not m:
            return jsonify({"error": "not_found", "message": "Machine not found"}), 404

        for field in ["host", "port", "user", "password", "enabled", "online", "name"]:
            if field in data:
                setattr(m, field, data[field])
        session.commit()
        logger.info("machine.update name=%s", m.name)
        return jsonify(_serialize_machine(m, True)), 200
    finally:
        session.close()

@machines_bp.delete("/machines/<name>")
@require_role("admin")
def delete_machine(name: str):
    session = get_session()
    try:
        m = session.query(Machine).filter(Machine.name == name).one_or_none()
        if not m:
            return jsonify({"error": "not_found", "message": "Machine not found"}), 404
        if m.reserved:
            return jsonify({"error": "bad_request", "message": "Machine is currently reserved; release first"}), 400

        session.delete(m)
        session.commit()
        logger.info("machine.delete name=%s", name)
        return jsonify({"message": "deleted"}), 200
    finally:
        session.close()
