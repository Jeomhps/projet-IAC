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

Base = declarative_base()
_engine = None
_SessionLocal = None

def _get_database_url():
    # Expect an external DB (e.g., MariaDB/MySQL via docker-compose)
    return os.environ["DATABASE_URL"]

def _create_engine(url=None):
    url = url or _get_database_url()
    return create_engine(
        url,
        pool_pre_ping=True,
        future=True,
    )

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
    max_retries = int(os.getenv("DB_CONNECT_MAX_RETRIES", 60 if max_retries is None else max_retries))
    delay = float(os.getenv("DB_CONNECT_RETRY_SECONDS", 1.0 if delay is None else delay))
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

    reserved = Column(Boolean, nullable=False, default=False)
    reserved_by = Column(String(255), nullable=True)
    reserved_until = Column(DateTime, nullable=True)

    enabled = Column(Boolean, nullable=False, default=True)
    online = Column(Boolean, nullable=False, default=True)
    last_seen_at = Column(DateTime, nullable=True)

    reservations = relationship("Reservation", back_populates="machine", cascade="all, delete-orphan")

class Reservation(Base):
    __tablename__ = "reservations"
    id = Column(Integer, primary_key=True)
    machine_id = Column(Integer, ForeignKey("machines.id", ondelete="CASCADE"), nullable=False)
    user_id = Column(Integer, ForeignKey("users.id", ondelete="CASCADE"), nullable=False)
    username = Column(String(255), nullable=False)  # optional for readability/back-compat
    reserved_until = Column(DateTime, nullable=True)

    machine = relationship("Machine", back_populates="reservations")

def init_db():
    engine, _ = _ensure_engine_and_session()
    _wait_for_db_ready(engine)
    Base.metadata.create_all(bind=engine)

def get_session():
    _, SessionLocal = _ensure_engine_and_session()
    return SessionLocal()
