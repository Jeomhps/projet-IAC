import logging
import os
import sys
from flask import Flask
from werkzeug.middleware.proxy_fix import ProxyFix
from common.db import init_db
from .machines import machines_bp
from .reservations import reservations_bp

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
    logging.getLogger("werkzeug").setLevel(logging.WARNING)

setup_logging()

def create_app():
    app = Flask(__name__)
    app.config["JSONIFY_PRETTYPRINT_REGULAR"] = True
    init_db()
    app.register_blueprint(machines_bp)
    app.register_blueprint(reservations_bp)

    # Trust one proxy hop (Caddy/Nginx in front)
    app.wsgi_app = ProxyFix(app.wsgi_app, x_for=1, x_proto=1, x_host=1, x_port=1)

    @app.get("/healthz")
    def healthz():
        return {"status": "ok"}, 200

    return app
