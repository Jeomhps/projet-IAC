import logging
from apscheduler.schedulers.background import BackgroundScheduler
import atexit

logger = logging.getLogger(__name__)

def delete_expired_users(app):
    from reservations import handle_expired_reservations
    with app.app_context():
        logger.info("Running expired reservation cleanup.")
        handle_expired_reservations()

def start_scheduler(app):
    scheduler = BackgroundScheduler()
    scheduler.add_job(lambda: delete_expired_users(app), 'interval', minutes=1)
    scheduler.start()
    atexit.register(lambda: scheduler.shutdown())
