import logging
from contextlib import contextmanager
from sqlalchemy import text
from sqlalchemy.orm import Session

logger = logging.getLogger(__name__)

@contextmanager
def mysql_advisory_lock(session: Session, lock_name: str):
    conn = session.connection()
    got = conn.execute(text("SELECT GET_LOCK(:name, 0)"), {"name": lock_name}).scalar()
    if got != 1:
        raise RuntimeError(f"Could not acquire MySQL advisory lock '{lock_name}'")
    try:
        yield
    finally:
        try:
            conn.execute(text("SELECT RELEASE_LOCK(:name)"), {"name": lock_name})
        except Exception as e:
            logger.warning(f"Failed to release MySQL advisory lock '{lock_name}': {e}")

@contextmanager
def noop_lock(_session: Session, lock_name: str):
    logger.warning(f"Using NOOP lock for '{lock_name}'. Fine for SQLite dev, not for production.")
    yield

def acquire_distributed_lock(session: Session, name: str):
    dialect = session.get_bind().dialect.name
    if dialect == "mysql":
        return mysql_advisory_lock(session, name)
    return noop_lock(session, name)
