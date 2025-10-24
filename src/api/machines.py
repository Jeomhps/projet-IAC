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
        # Downgrade to DEBUG to reduce noise; switch LOG_LEVEL=DEBUG if you want to see these
        logger.debug(f"Added machine: {name} ({host}:{port})")
        return jsonify({"name": name, "host": host, "port": port, "user": user}), 201
    except IntegrityError:
        session.rollback()
        logger.warning(f"Duplicate machine name attempted: {name}")
        return jsonify({"error": "Machine name must be unique."}), 400
    except Exception as e:
        session.rollback()
        logger.error(f"Failed to add machine {name}: {e}")
        return jsonify({"error": f"Failed to add machine: {e}"}), 400
    finally:
        session.close()
