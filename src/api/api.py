import logging
import os
import sys
import time
from flask import Flask, request, g
from werkzeug.middleware.proxy_fix import ProxyFix
from common.db import init_db
from .machines import machines_bp
from .reservations import reservations_bp
from .auth import auth_bp
from common.auth import ensure_default_admin

def setup_logging():
    log_level = os.getenv("LOG_LEVEL", "INFO").upper()
    handler = logging.StreamHandler(sys.stdout)
    formatter = logging.Formatter("%(asctime)s | %(levelname)s | %(name)s | %(message)s")
    handler.setFormatter(formatter)
    root = logging.getLogger()
    for h in list(root.handlers):
        root.removeHandler(h)
    root.addHandler(handler)
    root.setLevel(log_level)
    # Keep werkzeug access logs visible (INFO)
    logging.getLogger("werkzeug").setLevel(logging.INFO)

setup_logging()

def create_app():
    app = Flask(__name__)
    app.config["JSONIFY_PRETTYPRINT_REGULAR"] = True
    init_db()
    ensure_default_admin()  # Seed a default admin if env vars provided

    # Register blueprints
    app.register_blueprint(auth_bp)
    app.register_blueprint(machines_bp)
    app.register_blueprint(reservations_bp)

    # Trust one proxy hop (Caddy/Nginx in front)
    app.wsgi_app = ProxyFix(app.wsgi_app, x_for=1, x_proto=1, x_host=1, x_port=1)

    # Simple request logging (method, path, status, duration, user)
    @app.before_request
    def _start_timer():
        g._start_time = time.time()

    @app.after_request
    def _log_request(resp):
        try:
            duration_ms = 1000.0 * (time.time() - getattr(g, "_start_time", time.time()))
            user = getattr(g, "current_user", "-")
            app.logger.info("%s %s -> %d in %.1fms user=%s",
                            request.method, request.path, resp.status_code, duration_ms, user)
        except Exception:
            # avoid breaking responses due to logging
            pass
        return resp

    @app.get("/healthz")
    def healthz():
        return {"status": "ok"}, 200

    return app
