import logging
import os
import sys
from flask import Flask
from common.db import init_db
from .machines import machines_bp
from .reservations import reservations_bp

def setup_logging():
    # Honor LOG_LEVEL and avoid file logs in containers
    log_level = os.getenv("LOG_LEVEL", "INFO").upper()
    handler = logging.StreamHandler(sys.stdout)
    formatter = logging.Formatter("%(asctime)s | %(levelname)s | %(name)s | %(message)s")
    handler.setFormatter(formatter)

    root = logging.getLogger()
    # Clear existing handlers to avoid duplicates under Gunicorn
    for h in list(root.handlers):
        root.removeHandler(h)
    root.addHandler(handler)
    root.setLevel(log_level)

    # Quiet noisy loggers if desired
    logging.getLogger("werkzeug").setLevel(logging.WARNING)

setup_logging()

def create_app():
    app = Flask(__name__)
    app.config["JSONIFY_PRETTYPRINT_REGULAR"] = True
    init_db()
    app.register_blueprint(machines_bp)
    app.register_blueprint(reservations_bp)

    @app.get("/healthz")
    def healthz():
        return {"status": "ok"}, 200

    return app

if __name__ == "__main__":
    app = create_app()
    app.run(host="0.0.0.0", port=8080)
