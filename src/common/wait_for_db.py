import os
import sys
import time
from sqlalchemy import create_engine, text

def main():
    url = os.getenv("DATABASE_URL")
    if not url:
        print("wait_for_db: DATABASE_URL not set; skipping wait", file=sys.stderr)
        return 0

    # SQLite needs no wait
    if url.startswith("sqlite:"):
        print("wait_for_db: SQLite URL detected; no wait needed", file=sys.stderr)
        return 0

    max_retries = int(os.getenv("DB_CONNECT_MAX_RETRIES", "120"))
    delay = float(os.getenv("DB_CONNECT_RETRY_SECONDS", "1"))

    # Use minimal engine, no pooling persistence across the check
    engine = create_engine(url, future=True, pool_pre_ping=True)

    last_exc = None
    for attempt in range(1, max_retries + 1):
        try:
            with engine.connect() as conn:
                conn.execute(text("SELECT 1"))
            if attempt > 1:
                print(f"wait_for_db: database became ready after {attempt} attempts", file=sys.stderr)
            return 0
        except Exception as e:
            last_exc = e
            print(f"wait_for_db: waiting for database... attempt {attempt}/{max_retries} ({e})", file=sys.stderr)
            time.sleep(delay)

    print(f"wait_for_db: database not ready after {max_retries} attempts: {last_exc}", file=sys.stderr)
    return 1

if __name__ == "__main__":
    sys.exit(main())
