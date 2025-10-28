import os
import functools
from datetime import datetime, timedelta, timezone
from typing import Callable, Optional, List

import jwt
from flask import request, jsonify, g
from werkzeug.security import check_password_hash, generate_password_hash
from sqlalchemy.exc import IntegrityError

from common.db import get_session, User

JWT_SECRET = os.getenv("JWT_SECRET", "dev-secret-change-me")
JWT_ALGORITHM = os.getenv("JWT_ALGORITHM", "HS256")
JWT_EXPIRES_MINUTES = int(os.getenv("JWT_EXPIRES_MINUTES", "60"))
JWT_ISSUER = os.getenv("JWT_ISSUER", "projet-iac-api")

ADMIN_DEFAULT_USERNAME = os.getenv("ADMIN_DEFAULT_USERNAME", "")
ADMIN_DEFAULT_PASSWORD = os.getenv("ADMIN_DEFAULT_PASSWORD", "")

# Reserved usernames for provisioning safety (prevent changing privileged accounts)
_DEFAULT_BLOCKED = {"root"}
_EXTRA = {u.strip() for u in os.getenv("BLOCKED_USERNAMES", "").split(",") if u.strip()}
BLOCKED_USERNAMES = {*(name for name in _DEFAULT_BLOCKED if name)} | _EXTRA

def _now() -> datetime:
    return datetime.now(timezone.utc)

def create_access_token(username: str, roles: Optional[List[str]] = None, expires_minutes: Optional[int] = None) -> str:
    roles = roles or []
    now = _now()
    exp_minutes = expires_minutes or JWT_EXPIRES_MINUTES
    payload = {
        "iss": JWT_ISSUER,
        "sub": username,
        "roles": roles,
        "iat": int(now.timestamp()),
        "nbf": int(now.timestamp()),
        "exp": int((now + timedelta(minutes=exp_minutes)).timestamp()),
    }
    return jwt.encode(payload, JWT_SECRET, algorithm=JWT_ALGORITHM)

def _decode_token(token: str) -> dict:
    return jwt.decode(
        token,
        JWT_SECRET,
        algorithms=[JWT_ALGORITHM],
        options={"require": ["exp", "iat", "nbf"]},
        issuer=JWT_ISSUER,
    )

def _unauthorized(message: str):
    return jsonify({"error": "unauthorized", "message": message}), 401

def _forbidden(message: str):
    return jsonify({"error": "forbidden", "message": message}), 403

def get_bearer_token_from_request() -> Optional[str]:
    auth = request.headers.get("Authorization", "")
    if not auth.startswith("Bearer "):
        return None
    return auth[len("Bearer ") :].strip()

def require_auth(optional: bool = False) -> Callable:
    def decorator(view_func: Callable) -> Callable:
        @functools.wraps(view_func)
        def wrapper(*args, **kwargs):
            token = get_bearer_token_from_request()
            if not token:
                if optional:
                    g.current_user = None
                    g.current_roles = []
                    return view_func(*args, **kwargs)
                return _unauthorized("Missing Bearer token")
            try:
                payload = _decode_token(token)
            except jwt.ExpiredSignatureError:
                return _unauthorized("Token expired")
            except jwt.InvalidTokenError as e:
                return _unauthorized(f"Invalid token: {e}")
            g.current_user = payload.get("sub")
            g.current_roles = payload.get("roles", [])
            if not g.current_user:
                return _forbidden("Invalid subject in token")
            return view_func(*args, **kwargs)
        return wrapper
    return decorator

def require_role(role: str) -> Callable:
    def decorator(view_func: Callable) -> Callable:
        @functools.wraps(view_func)
        @require_auth()
        def wrapper(*args, **kwargs):
            if role not in (g.get("current_roles") or []):
                return _forbidden(f"Missing required role: {role}")
            return view_func(*args, **kwargs)
        return wrapper
    return decorator

# User helpers

def get_user_by_username(session, username: str) -> Optional[User]:
    return session.query(User).filter(User.username == username).one_or_none()

def verify_user_credentials(session, username: str, password: str) -> Optional[User]:
    user = get_user_by_username(session, username)
    if not user:
        return None
    if not check_password_hash(user.password_hash, password):
        return None
    return user

def create_user(session, username: str, password: str, is_admin: bool = False) -> User:
    """
    Create a user, with blocked username guard.
    """
    if username in BLOCKED_USERNAMES:
        raise ValueError(f"Username '{username}' is not allowed")
    existing = get_user_by_username(session, username)
    if existing:
        raise ValueError("User already exists")

    user = User(
        username=username,
        password_hash=generate_password_hash(password),
        is_admin=is_admin,
    )
    session.add(user)
    try:
        session.commit()
    except IntegrityError:
        session.rollback()
        raise ValueError("User already exists")
    return user

def ensure_default_admin() -> None:
    """
    Create a default admin user if ADMIN_DEFAULT_USERNAME and ADMIN_DEFAULT_PASSWORD are set.
    Ignores duplicate insert races.
    """
    if not ADMIN_DEFAULT_USERNAME or not ADMIN_DEFAULT_PASSWORD:
        return
    session = get_session()
    try:
        try:
            # allow default admin even if BLOCKED_USERNAMES contains it
            existing = get_user_by_username(session, ADMIN_DEFAULT_USERNAME)
            if not existing:
                user = User(
                    username=ADMIN_DEFAULT_USERNAME,
                    password_hash=generate_password_hash(ADMIN_DEFAULT_PASSWORD),
                    is_admin=True,
                )
                session.add(user)
                session.commit()
        except IntegrityError:
            session.rollback()
    finally:
        session.close()
