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
    Acquire a MySQL/MariaDB advisory lock using a dedicated DB connection.
    - Uses GET_LOCK(:name, :timeout)
    - Always attempts RELEASE_LOCK(:name) and logs the outcome.
    - The dedicated connection ensures the lock isn't impacted by Session transaction boundaries.
    """
    engine = session.get_bind()
    timeout = int(os.getenv("DB_LOCK_TIMEOUT", "2"))  # seconds
    conn = engine.connect()  # dedicated connection for the lock
    acquired = False
    try:
        got = conn.execute(
            text("SELECT GET_LOCK(:name, :timeout)"),
            {"name": lock_name, "timeout": timeout},
        ).scalar()
        if got == 1:
            acquired = True
            logger.debug(f"Acquired MySQL advisory lock '{lock_name}'")
            yield
        else:
            # got can be 0 on timeout or NULL on error
            raise RuntimeError(f"Could not acquire MySQL advisory lock '{lock_name}' (result={got})")
    finally:
        if acquired:
            try:
                rel = conn.execute(
                    text("SELECT RELEASE_LOCK(:name)"),
                    {"name": lock_name},
                ).scalar()
                # rel: 1=released, 0=not owner, NULL=no such lock
                if rel == 1:
                    logger.debug(f"Released MySQL advisory lock '{lock_name}'")
                elif rel == 0:
                    logger.info(f"MySQL advisory lock '{lock_name}' was not owned by this connection at release time")
                else:
                    logger.info(f"MySQL advisory lock '{lock_name}' did not exist at release time (result={rel})")
            except Exception as e:
                # Not fatal; the server releases the lock if the connection closes anyway.
                logger.warning(f"Failed to release MySQL advisory lock '{lock_name}': {e}")
        try:
            conn.close()
        except Exception:
            pass

@contextmanager
def noop_lock(_session: Session, lock_name: str):
    logger.warning(f"Using NOOP lock for '{lock_name}'. Fine for SQLite dev, not for production.")
    yield

def acquire_distributed_lock(session: Session, name: str):
    """
    Returns a context manager that acquires a distributed lock.
    - MySQL/MariaDB: GET_LOCK/RELEASE_LOCK via dedicated connection
    - SQLite/others: no-op with warning
    """
    if _dialect_name(session) == "mysql":
        return mysql_advisory_lock(session, name)
    return noop_lock(session, name)
