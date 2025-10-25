import os
import tempfile
import pathlib

# Bind/address
bind = "0.0.0.0:8080"

# Concurrency
workers = int(os.getenv("WEB_CONCURRENCY", "2"))
threads = int(os.getenv("GTHREADS", "4"))
worker_class = os.getenv("WORKER_CLASS", "gthread")

# Timeouts
timeout = int(os.getenv("WEB_TIMEOUT", "180"))
graceful_timeout = int(os.getenv("WEB_GRACEFUL_TIMEOUT", "30"))
keepalive = int(os.getenv("WEB_KEEPALIVE", "5"))

# Max requests (avoid leaks)
max_requests = int(os.getenv("WEB_MAX_REQUESTS", "1000"))
max_requests_jitter = int(os.getenv("WEB_MAX_REQUESTS_JITTER", "100"))

# Logging
loglevel = os.getenv("GUNICORN_LOGLEVEL", "info")
ENABLE_ACCESS_LOG = os.getenv("ENABLE_ACCESS_LOG", "").lower() in ("1", "true", "yes", "on")
accesslog = "-" if ENABLE_ACCESS_LOG else None
errorlog = "-"
capture_output = True

# Worker tmp dir: prefer /dev/shm (Linux/Docker), else fallback to /tmp on macOS
def _pick_worker_tmp_dir() -> str:
    # Allow explicit override
    env_dir = os.getenv("WORKER_TMP_DIR") or os.getenv("GUNICORN_WORKER_TMP_DIR")
    if env_dir:
        return env_dir

    dev_shm = pathlib.Path("/dev/shm")
    if dev_shm.exists() and dev_shm.is_dir() and os.access(str(dev_shm), os.W_OK):
        return str(dev_shm)

    # Portable fallback
    fallback = pathlib.Path(tempfile.gettempdir()) / "gunicorn"
    try:
        fallback.mkdir(parents=True, exist_ok=True)
    except Exception:
        # Last resort: just use the system temp dir
        return tempfile.gettempdir()
    return str(fallback)

worker_tmp_dir = _pick_worker_tmp_dir()

# Optional: preload
preload_app = os.getenv("WEB_PRELOAD_APP", "false").lower() in ("1", "true", "yes")
