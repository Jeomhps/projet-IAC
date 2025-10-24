from flask import Blueprint, request, jsonify
import logging
from sqlalchemy.exc import IntegrityError
from common.db import get_session, Machine

logger = logging.getLogger(__name__)
machines_bp = Blueprint('machines', __name__)

@machines_bp.route("/machines", methods=["POST"])
def add_machine():
    data = request.get_json() or {}
    name = data.get("name")
    host = data.get("host")
    port = int(data.get("port", 22))
    user = data.get("user", "root")
    password = data.get("password", "test")
    if not all([name, host, port, user, password]):
        logger.warning(f"Missing parameters for machine: {data}")
        return jsonify({"error": "Missing required machine parameters."}), 400

    session = get_session()
    try:
        machine = Machine(name=name, host=host, port=port, user=user, password=password)
        session.add(machine)
        session.commit()
        logger.info(f"Added machine: {name} ({host}:{port})")
        return jsonify({"name": name, "host": host, "port": port, "user": user}), 201
    except IntegrityError:
        session.rollback()
        return jsonify({"error": "Machine name must be unique."}), 400
    except Exception as e:
        session.rollback()
        logger.warning(f"Failed to add machine {name}: {e}")
        return jsonify({"error": f"Failed to add machine: {e}"}), 400
    finally:
        session.close()

@machines_bp.route("/machines/<name>", methods=["DELETE"])
def delete_machine(name):
    session = get_session()
    try:
        machine = session.query(Machine).filter(Machine.name == name).one_or_none()
        if not machine:
            return jsonify({"error": f"Machine '{name}' not found"}), 404
        session.delete(machine)
        session.commit()
        logger.info(f"Removed machine: {name}")
        return jsonify({"removed": name}), 200
    finally:
        session.close()

@machines_bp.route("/machines", methods=["GET"])
def list_machines():
    session = get_session()
    try:
        machines = session.query(Machine).all()
        result = []
        for m in machines:
            result.append({
                "id": m.id,
                "name": m.name,
                "host": m.host,
                "port": m.port,
                "user": m.user,
                "password": m.password,
                "reserved": bool(m.reserved),
                "reserved_by": m.reserved_by,
                "reserved_until": m.reserved_until.isoformat(timespec="seconds") if m.reserved_until else None,
            })
        logger.info(f"Listing {len(result)} machines.")
        return jsonify({"machines": result}), 200
    finally:
        session.close()
