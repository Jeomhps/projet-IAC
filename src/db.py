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
    CREATE TABLE IF NOT EXISTS reservations (
        username TEXT,
        container_name TEXT,
        reserved_until TEXT,
        PRIMARY KEY (username, container_name)
    )
    """)
    cursor.execute("""
    CREATE TABLE IF NOT EXISTS machines (
        name TEXT PRIMARY KEY,
        host TEXT NOT NULL,
        port INTEGER NOT NULL,
        user TEXT NOT NULL,
        password TEXT NOT NULL
    )
    """)
    conn.commit()

def get_conn_cursor():
    global conn, cursor
    if conn is None or cursor is None:
        init_db()
    return conn, cursor
