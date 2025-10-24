import os

bind = "0.0.0.0:8080"

# Concurrency
workers = int(os.getenv("WEB_CONCURRENCY", "2"))
threads = int(os.getenv("GTHREADS", "4"))
worker_class = os.getenv("WORKER_CLASS", "gthread")

# Timeouts
timeout = int(os.getenv("WEB_TIMEOUT", "180"))
graceful_timeout = int(os.getenv("WEB_GRACEFUL_TIMEOUT", "30"))
keepalive = int(os.getenv("WEB_KEEPALIVE", "5"))

# Reliability
max_requests = int(os.getenv("WEB_MAX_REQUESTS", "1000"))
max_requests_jitter = int(os.getenv("WEB_MAX_REQUESTS_JITTER", "100"))

# Logging
# Set GUNICORN_LOGLEVEL=warning to quiet master/worker info lines
loglevel = os.getenv("GUNICORN_LOGLEVEL", "info")
errorlog = "-"     # stderr
# Access log: enable only when explicitly requested
ENABLE_ACCESS_LOG = os.getenv("ENABLE_ACCESS_LOG", "").lower() in ("1", "true", "yes", "on")
accesslog = "-" if ENABLE_ACCESS_LOG else None
capture_output = True

# App loading
preload_app = os.getenv("WEB_PRELOAD_APP", "false").lower() in ("1", "true", "yes")
