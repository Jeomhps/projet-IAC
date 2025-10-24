import logging
import os
from contextlib import contextmanager
from sqlalchemy import text
from sqlalchemy.orm import Session

logger = logging.getLogger(__name__)

def _dialect_name(session: Session) -> str:
    return session.get_bind().dialect.name

@contextmanager
def mysql_advisory_lock(session: Session, lock_name: str):
    """
    Dedicated connection for the lock.
    Use 0-second timeout so shutdown never blocks on lock waits.
    """
    engine = session.get_bind()
    # Force no-wait for faster shutdown behavior
    timeout = int(os.getenv("DB_LOCK_TIMEOUT", "0"))
    conn = engine.connect()
    acquired = False
    try:
        got = conn.execute(text("SELECT GET_LOCK(:name, :timeout)"), {"name": lock_name, "timeout": timeout}).scalar()
        if got == 1:
            acquired = True
            yield
        else:
            raise RuntimeError(f"Could not acquire MySQL advisory lock '{lock_name}' (result={got})")
    finally:
        if acquired:
            try:
                rel = conn.execute(text("SELECT RELEASE_LOCK(:name)"), {"name": lock_name}).scalar()
                if rel != 1:
                    logger.debug(f"RELEASE_LOCK result for '{lock_name}': {rel}")
            except Exception as e:
                logger.debug(f"Failed to release lock '{lock_name}': {e}")
        try:
            conn.close()
        except Exception:
            pass

@contextmanager
def noop_lock(_session: Session, lock_name: str):
    logger.debug(f"NOOP lock for '{lock_name}' (non-MySQL dialect)")
    yield

def acquire_distributed_lock(session: Session, name: str):
    if _dialect_name(session) == "mysql":
        return mysql_advisory_lock(session, name)
    return noop_lock(session, name)
