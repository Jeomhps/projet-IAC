from flask import Blueprint, request, jsonify
import logging
from db import get_conn_cursor

logger = logging.getLogger(__name__)
machines_bp = Blueprint('machines', __name__)

@machines_bp.route("/machines", methods=["POST"])
def add_machine():
    conn, cursor = get_conn_cursor()
    data = request.get_json()
    name = data.get("name")
    host = data.get("host")
    port = int(data.get("port", 22))
    user = data.get("user", "root")
    password = data.get("password", "test")
    if not all([name, host, port, user, password]):
        logger.warning(f"Missing parameters for machine: {data}")
        return jsonify({"error": "Missing required machine parameters."}), 400
    try:
        cursor.execute(
            "INSERT INTO machines (name, host, port, user, password) VALUES (?, ?, ?, ?, ?)",
            (name, host, port, user, password)
        )
        conn.commit()
        logger.info(f"Added machine: {name} ({host}:{port})")
        return jsonify({"name": name, "host": host, "port": port, "user": user}), 201
    except Exception as e:
        logger.warning(f"Failed to add machine {name}: {e}")
        return jsonify({"error": f"Failed to add machine: {e}"}), 400

@machines_bp.route("/machines/<name>", methods=["DELETE"])
def delete_machine(name):
    conn, cursor = get_conn_cursor()
    cursor.execute("DELETE FROM machines WHERE name = ?", (name,))
    conn.commit()
    logger.info(f"Removed machine: {name}")
    return jsonify({"removed": name}), 200

@machines_bp.route("/machines", methods=["GET"])
def list_machines():
    conn, cursor = get_conn_cursor()
    cursor.execute("SELECT name, host, port, user FROM machines")
    machines = [
        {"name": name, "host": host, "port": port, "user": user}
        for name, host, port, user in cursor.fetchall()
    ]
    logger.info(f"Listing {len(machines)} machines.")
    return jsonify({"machines": machines}), 200
