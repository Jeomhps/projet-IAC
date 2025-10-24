import argparse
import logging
import signal
import threading
from common.db import init_db, get_session
from common.locks import acquire_distributed_lock
from common.services.cleanup import run_expired_reservations_cleanup

logging.basicConfig(level=logging.INFO, format="%(asctime)s | %(levelname)s | %(name)s | %(message)s")
logger = logging.getLogger("maintenance")

LOCK_NAME = "reservation-expiry-cleanup"
_shutdown = threading.Event()

def _handle_signal(signum, frame):
    logger.info(f"Received signal {signum}; initiating graceful shutdown")
    _shutdown.set()

def run_once():
    if _shutdown.is_set():
        return
    init_db()  # should be quick after first run; if it may block, move it into main() before loop
    session = get_session()
    try:
        if _shutdown.is_set():
            return
        try:
            with acquire_distributed_lock(session, LOCK_NAME):
                # pass cancel so cleanup can stop ansible promptly
                run_expired_reservations_cleanup(session, cancel=_shutdown)
        except RuntimeError as e:
            # lock not acquired (another runner); skip
            logger.info(f"Skip run: {e}")
    finally:
        session.close()

def main():
    parser = argparse.ArgumentParser(description="Maintenance runner")
    parser.add_argument("--once", action="store_true", help="Run cleanup once and exit")
    parser.add_argument("--interval", type=int, default=60, help="Loop interval in seconds")
    args = parser.parse_args()

    signal.signal(signal.SIGTERM, _handle_signal)
    signal.signal(signal.SIGINT, _handle_signal)

    if args.once:
        run_once()
        return

    logger.info(f"Starting maintenance loop (interval={args.interval}s)")
    while not _shutdown.is_set():
        run_once()
        # interruptible sleep
        _shutdown.wait(args.interval)

    logger.info("Scheduler exited gracefully")

if __name__ == "__main__":
    main()
