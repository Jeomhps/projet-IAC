import os
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
)
from sqlalchemy.orm import declarative_base, relationship, sessionmaker

# Default to SQLite in the src directory for development
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
DEFAULT_SQLITE_PATH = os.path.join(SCRIPT_DIR, "containers.db")
DATABASE_URL = os.environ.get("DATABASE_URL", f"sqlite:///{DEFAULT_SQLITE_PATH}")

# Create engine with sensible defaults
engine_kwargs = {
    "pool_pre_ping": True,
    "future": True,
}
connect_args = {}

# SQLite-specific adjustments
if DATABASE_URL.startswith("sqlite:"):
    # Thread-safety for Flask + background jobs
    connect_args["check_same_thread"] = False
    engine_kwargs["connect_args"] = connect_args

engine = create_engine(DATABASE_URL, **engine_kwargs)
SessionLocal = sessionmaker(bind=engine, autoflush=False, autocommit=False, expire_on_commit=False, future=True)
Base = declarative_base()

# Enable foreign key constraints on SQLite
if DATABASE_URL.startswith("sqlite:"):
    @event.listens_for(engine, "connect")
    def set_sqlite_pragma(dbapi_connection, connection_record):
        cursor = dbapi_connection.cursor()
        cursor.execute("PRAGMA foreign_keys=ON")
        cursor.close()

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

    reservations = relationship("Reservation", back_populates="machine", cascade="all, delete-orphan")

class Reservation(Base):
    __tablename__ = "reservations"

    id = Column(Integer, primary_key=True)
    machine_id = Column(Integer, ForeignKey("machines.id", ondelete="CASCADE"), nullable=False)
    username = Column(String(255), nullable=False)
    reserved_until = Column(DateTime, nullable=True)

    machine = relationship("Machine", back_populates="reservations")

def init_db():
    Base.metadata.create_all(bind=engine)

def get_session():
    # Caller is responsible for closing; prefer:
    #   with get_session() as session: ...
    return SessionLocal()
