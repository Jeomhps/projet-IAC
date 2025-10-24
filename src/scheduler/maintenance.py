import argparse
import logging
import time
from common.db import init_db, get_session
from common.locks import acquire_distributed_lock
from common.services.cleanup import run_expired_reservations_cleanup

logging.basicConfig(level=logging.INFO, format="%(asctime)s | %(levelname)s | %(name)s | %(message)s")
logger = logging.getLogger("maintenance")

LOCK_NAME = "reservation-expiry-cleanup"

def run_once():
    init_db()
    session = get_session()
    try:
        with acquire_distributed_lock(session, LOCK_NAME):
            run_expired_reservations_cleanup(session)
    finally:
        session.close()

def main():
    parser = argparse.ArgumentParser(description="Maintenance runner")
    parser.add_argument("--once", action="store_true", help="Run cleanup once and exit")
    parser.add_argument("--interval", type=int, default=60, help="Loop interval in seconds")
    args = parser.parse_args()

    if args.once:
        run_once()
        return

    logger.info(f"Starting maintenance loop (interval={args.interval}s)")
    while True:
        try:
            run_once()
        except Exception as e:
            logger.exception(f"Cleanup error: {e}")
        time.sleep(args.interval)

if __name__ == "__main__":
    main()
