from flask import Blueprint, request, jsonify, g
import logging
from common.auth import (
    create_access_token,
    verify_user_credentials,
    create_user,
    get_user_by_username,
    require_auth,
    require_role,
)
from common.db import get_session, User

logger = logging.getLogger(__name__)
auth_bp = Blueprint("auth", __name__)

@auth_bp.post("/login")
def login():
    data = request.get_json(silent=True) or {}
    username = data.get("username")
    password = data.get("password")
    if not username or not password:
        return jsonify({"error": "invalid_request", "message": "username and password are required"}), 400
    session = get_session()
    try:
        user = verify_user_credentials(session, username, password)
        if not user:
            logger.warning("Login failed for user=%s", username)
            return jsonify({"error": "invalid_grant", "message": "invalid credentials"}), 401
        roles = ["admin"] if user.is_admin else []
        token = create_access_token(user.username, roles=roles)
        return jsonify({
            "access_token": token,
            "token_type": "Bearer",
            "expires_in": 60 * 60
        }), 200
    finally:
        session.close()

@auth_bp.get("/whoami")
@require_auth()
def whoami():
    return jsonify({"user": g.current_user, "roles": g.current_roles}), 200

# Admin-only: user management

@auth_bp.post("/users")
@require_role("admin")
def create_user_endpoint():
    data = request.get_json(silent=True) or {}
    username = data.get("username")
    password = data.get("password")
    is_admin = bool(data.get("is_admin", False))
    if not username or not password:
        return jsonify({"error": "invalid_request", "message": "username and password are required"}), 400
    session = get_session()
    try:
        user = create_user(session, username, password, is_admin=is_admin)
        return jsonify({"username": user.username, "is_admin": user.is_admin, "created_at": user.created_at.isoformat()}), 201
    except ValueError as e:
        return jsonify({"error": "conflict", "message": str(e)}), 409
    finally:
        session.close()

@auth_bp.get("/users")
@require_role("admin")
def list_users():
    session = get_session()
    try:
        users = session.query(User).all()
        return jsonify([
            {"username": u.username, "is_admin": u.is_admin, "created_at": u.created_at.isoformat()}
            for u in users
        ]), 200
    finally:
        session.close()

@auth_bp.delete("/users/<username>")
@require_role("admin")
def delete_user(username: str):
    session = get_session()
    try:
        user = get_user_by_username(session, username)
        if not user:
            return jsonify({"error": "not_found", "message": "user not found"}), 404
        session.delete(user)
        session.commit()
        return jsonify({"message": "deleted"}), 200
    finally:
        session.close()
