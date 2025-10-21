import os
import sqlite3

DB_NAME = "containers.db"
SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
DB_PATH = os.path.join(SCRIPT_DIR, DB_NAME)

conn = None
cursor = None

def init_db():
    global conn, cursor
    conn = sqlite3.connect(DB_PATH, check_same_thread=False)
    cursor = conn.cursor()
    cursor.execute("""
    CREATE TABLE IF NOT EXISTS machines (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        name TEXT UNIQUE NOT NULL,
        host TEXT NOT NULL,
        port INTEGER NOT NULL,
        user TEXT NOT NULL,
        password TEXT NOT NULL,
        reserved INTEGER NOT NULL DEFAULT 0,
        reserved_by TEXT,
        reserved_until TEXT
    )
    """)
    cursor.execute("""
    CREATE TABLE IF NOT EXISTS reservations (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        machine_id INTEGER NOT NULL,
        username TEXT NOT NULL,
        reserved_until TEXT,
        FOREIGN KEY (machine_id) REFERENCES machines(id) ON DELETE CASCADE
    )
    """)
    conn.commit()

def get_conn_cursor():
    global conn, cursor
    if conn is None or cursor is None:
        init_db()
    return conn, cursor
