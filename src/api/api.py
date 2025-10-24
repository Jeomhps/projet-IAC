import logging
import sys
from flask import Flask
from common.db import init_db
from .machines import machines_bp
from .reservations import reservations_bp

def setup_logging():
    logging.basicConfig(
        level=logging.INFO,
        format="%(asctime)s | %(levelname)s | %(name)s | %(message)s",
        handlers=[
            logging.FileHandler("api.log"),
            logging.StreamHandler(sys.stdout)
        ],
    )
    logging.getLogger("werkzeug").setLevel(logging.WARNING)

setup_logging()

def create_app():
    app = Flask(__name__)
    app.config["JSONIFY_PRETTYPRINT_REGULAR"] = True
    init_db()
    app.register_blueprint(machines_bp)
    app.register_blueprint(reservations_bp)
    return app

if __name__ == "__main__":
    app = create_app()
    app.run(host="0.0.0.0", port=8080)
