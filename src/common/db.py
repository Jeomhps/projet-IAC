import os
import logging
import time
from datetime import datetime
from sqlalchemy import (
    create_engine,
    Column,
    Integer,
    String,
    Boolean,
    DateTime,
    ForeignKey,
    event,
    text,
)
from sqlalchemy.orm import declarative_base, relationship, sessionmaker

logger = logging.getLogger(__name__)

BASE_DIR = os.path.dirname(os.path.abspath(__file__))
DEFAULT_SQLITE_PATH = os.path.abspath(os.path.join(BASE_DIR, "containers.db"))

Base = declarative_base()

_engine = None
_SessionLocal = None

def _get_database_url():
    return os.environ.get("DATABASE_URL", f"sqlite:///{DEFAULT_SQLITE_PATH}")

def _create_engine(url=None):
    url = url or _get_database_url()
    engine_kwargs = {
        "pool_pre_ping": True,
        "future": True,
    }
    if url.startswith("sqlite:"):
        engine_kwargs["connect_args"] = {"check_same_thread": False}

    engine = create_engine(url, **engine_kwargs)

    if url.startswith("sqlite:"):
        @event.listens_for(engine, "connect")
        def set_sqlite_pragma(dbapi_connection, connection_record):
            cursor = dbapi_connection.cursor()
            cursor.execute("PRAGMA foreign_keys=ON")
            cursor.close()

    return engine

def _ensure_engine_and_session():
    global _engine, _SessionLocal
    if _engine is None or _SessionLocal is None:
        _engine = _create_engine()
        _SessionLocal = sessionmaker(
            bind=_engine,
            autoflush=False,
            autocommit=False,
            expire_on_commit=False,
            future=True,
        )
    return _engine, _SessionLocal

def _wait_for_db_ready(engine, max_retries=None, delay=None):
    max_retries = int(os.getenv("DB_CONNECT_MAX_RETRIES", max_retries or 60))
    delay = float(os.getenv("DB_CONNECT_RETRY_SECONDS", delay or 1.0))
    last_exc = None
    for attempt in range(1, max_retries + 1):
        try:
            with engine.connect() as conn:
                conn.execute(text("SELECT 1"))
            if attempt > 1:
                logger.info(f"Database became ready after {attempt} attempts")
            return
        except Exception as e:
            last_exc = e
            logger.info(f"Waiting for database... attempt {attempt}/{max_retries} ({e})")
            time.sleep(delay)
    raise last_exc

class User(Base):
    __tablename__ = "users"

    id = Column(Integer, primary_key=True)
    username = Column(String(255), unique=True, nullable=False, index=True)
    password_hash = Column(String(255), nullable=False)
    is_admin = Column(Boolean, nullable=False, default=False)
    created_at = Column(DateTime, nullable=False, default=datetime.utcnow)

class Machine(Base):
    __tablename__ = "machines"

    id = Column(Integer, primary_key=True)
    name = Column(String(255), unique=True, nullable=False)
    host = Column(String(255), nullable=False)
    port = Column(Integer, nullable=False, default=22)
    user = Column(String(255), nullable=False)
    password = Column(String(255), nullable=False)

    # Reservation state
    reserved = Column(Boolean, nullable=False, default=False)
    reserved_by = Column(String(255), nullable=True)
    reserved_until = Column(DateTime, nullable=True)

    # Eligibility state (no separate pools)
    enabled = Column(Boolean, nullable=False, default=True)
    online = Column(Boolean, nullable=False, default=True)
    last_seen_at = Column(DateTime, nullable=True)

    reservations = relationship("Reservation", back_populates="machine", cascade="all, delete-orphan")

class Reservation(Base):
    __tablename__ = "reservations"

    id = Column(Integer, primary_key=True)
    machine_id = Column(Integer, ForeignKey("machines.id", ondelete="CASCADE"), nullable=False)
    # new: reference the user row, not just username; keep username for compatibility/backfill
    user_id = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=True)
    username = Column(String(255), nullable=False)
    reserved_until = Column(DateTime, nullable=True)

    machine = relationship("Machine", back_populates="reservations")

def init_db():
    """
    Initialize the database and create tables, then apply light migrations for SQLite.
    """
    global _engine, _SessionLocal

    engine, _ = _ensure_engine_and_session()
    url = _get_database_url()

    if not url.startswith("sqlite:"):
        try:
            _wait_for_db_ready(engine)
        except Exception as e:
            if os.getenv("DB_FALLBACK_TO_SQLITE", "").lower() in ("1", "true", "yes"):
                logger.warning(f"External DB not ready, falling back to SQLite: {e}")
                _engine = _create_engine(f"sqlite:///{DEFAULT_SQLITE_PATH}")
                _SessionLocal = sessionmaker(
                    bind=_engine, autoflush=False, autocommit=False, expire_on_commit=False, future=True
                )
                engine = _engine
            else:
                logger.error(f"Database not ready and no fallback allowed: {e}")
                raise

    Base.metadata.create_all(bind=engine)

    # Minimal, SQLite-safe migrations:
    if url.startswith("sqlite:"):
        with engine.begin() as conn:
            # Add reservations.user_id if missing
            cols = [row[1] for row in conn.exec_driver_sql("PRAGMA table_info(reservations)").fetchall()]
            if "user_id" not in cols:
                conn.exec_driver_sql("ALTER TABLE reservations ADD COLUMN user_id INTEGER")
                # Backfill using username
                conn.exec_driver_sql("""
                    UPDATE reservations
                    SET user_id = (
                        SELECT id FROM users WHERE users.username = reservations.username
                    )
                    WHERE user_id IS NULL
                """)
            # Add machines.enabled/online/last_seen_at if missing (best-effort)
            mcols = [row[1] for row in conn.exec_driver_sql("PRAGMA table_info(machines)").fetchall()]
            if "enabled" not in mcols:
                conn.exec_driver_sql("ALTER TABLE machines ADD COLUMN enabled BOOLEAN DEFAULT 1 NOT NULL")
            if "online" not in mcols:
                conn.exec_driver_sql("ALTER TABLE machines ADD COLUMN online BOOLEAN DEFAULT 1 NOT NULL")
            if "last_seen_at" not in mcols:
                conn.exec_driver_sql("ALTER TABLE machines ADD COLUMN last_seen_at DATETIME")

def get_session():
    _, SessionLocal = _ensure_engine_and_session()
    return SessionLocal()
