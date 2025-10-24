import os

bind = "0.0.0.0:8080"

workers = int(os.getenv("WEB_CONCURRENCY", "2"))
threads = int(os.getenv("GTHREADS", "4"))
worker_class = os.getenv("WORKER_CLASS", "gthread")

timeout = int(os.getenv("WEB_TIMEOUT", "180"))
graceful_timeout = int(os.getenv("WEB_GRACEFUL_TIMEOUT", "30"))
keepalive = int(os.getenv("WEB_KEEPALIVE", "5"))

max_requests = int(os.getenv("WEB_MAX_REQUESTS", "1000"))
max_requests_jitter = int(os.getenv("WEB_MAX_REQUESTS_JITTER", "100"))

# Logging
loglevel = os.getenv("GUNICORN_LOGLEVEL", "info")
ENABLE_ACCESS_LOG = os.getenv("ENABLE_ACCESS_LOG", "").lower() in ("1", "true", "yes", "on")
accesslog = "-" if ENABLE_ACCESS_LOG else None
errorlog = "-"
capture_output = True

# Important in Docker: avoid /tmp overlay quirks
worker_tmp_dir = "/dev/shm"

# Optional: preload app is off by default
preload_app = os.getenv("WEB_PRELOAD_APP", "false").lower() in ("1", "true", "yes")
