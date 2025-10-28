from flask import Blueprint, request, jsonify, g
import logging
from werkzeug.security import generate_password_hash
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

def _is_admin() -> bool:
    return "admin" in (getattr(g, "current_roles", []) or [])

# Auth

@auth_bp.post("/auth/login")
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
            return jsonify({"error": "invalid_grant", "message": "invalid credentials"}), 401
        roles = ["admin"] if user.is_admin else []
        token = create_access_token(user.username, roles=roles)
        return jsonify({"access_token": token, "token_type": "Bearer", "expires_in": 60 * 60}), 200
    finally:
        session.close()

@auth_bp.get("/auth/me")
@require_auth()
def me():
    return jsonify({"username": g.current_user, "roles": g.current_roles, "is_admin": _is_admin()}), 200

@auth_bp.post("/auth/refresh")
@require_auth()
def refresh():
    token = create_access_token(g.current_user, roles=g.current_roles)
    return jsonify({"access_token": token, "token_type": "Bearer", "expires_in": 60 * 60}), 200

# Users (admin)

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
        return jsonify([{"username": u.username, "is_admin": u.is_admin, "created_at": u.created_at.isoformat()} for u in users]), 200
    finally:
        session.close()

@auth_bp.get("/users/<username>")
@require_role("admin")
def get_user_endpoint(username: str):
    session = get_session()
    try:
        user = get_user_by_username(session, username)
        if not user:
            return jsonify({"error": "not_found", "message": "user not found"}), 404
        return jsonify({"username": user.username, "is_admin": user.is_admin, "created_at": user.created_at.isoformat()}), 200
    finally:
        session.close()

@auth_bp.patch("/users/<username>")
@require_role("admin")
def update_user_endpoint(username: str):
    data = request.get_json(silent=True) or {}
    session = get_session()
    try:
        user = get_user_by_username(session, username)
        if not user:
            return jsonify({"error": "not_found", "message": "user not found"}), 404

        updated = False
        if "is_admin" in data:
            user.is_admin = bool(data["is_admin"])
            updated = True
        if "password" in data and data["password"]:
            user.password_hash = generate_password_hash(data["password"])
            updated = True

        if not updated:
            return jsonify({"error": "invalid_request", "message": "no updatable fields provided"}), 400

        session.commit()
        return jsonify({"username": user.username, "is_admin": user.is_admin, "created_at": user.created_at.isoformat()}), 200
    finally:
        session.close()

@auth_bp.delete("/users/<username>")
@require_role("admin")
def delete_user_endpoint(username: str):
    if g.current_user == username:
        return jsonify({"error": "bad_request", "message": "cannot delete yourself"}), 400

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
