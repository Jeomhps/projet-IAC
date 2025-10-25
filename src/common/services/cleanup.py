import logging
import os
import tempfile
import subprocess
from pathlib import Path
from sqlalchemy import and_
from sqlalchemy.orm import Session
from datetime import datetime
import time
from typing import Optional
from common.db import get_session, Machine, Reservation

logger = logging.getLogger(__name__)

def _playbook_path() -> str:
    return str((Path(__file__).resolve().parent.parent / "playbooks" / "create-users.yml").resolve())

def _tmp_dir() -> str:
    # Use shared memory to avoid overlayfs slowness; configurable via TMPDIR
    return os.getenv("TMPDIR", "/dev/shm")

def _run_ansible_with_cancel(cmd: list[str], cancel: Optional[object]) -> tuple[int, str, str]:
    """
    Run ansible-playbook but allow fast shutdown:
    - If cancel is set, send SIGTERM then SIGKILL after small grace.
    """
    proc = subprocess.Popen(cmd, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
    try:
        while proc.poll() is None:
            if cancel is not None and getattr(cancel, "is_set", None) and cancel.is_set():
                logger.info("Shutdown requested; terminating ansible-playbook...")
                try:
                    proc.terminate()
                except Exception:
                    pass
                try:
                    proc.wait(timeout=5)
                except Exception:
                    try:
                        proc.kill()
                    except Exception:
                        pass
                return (143, "", "terminated by scheduler shutdown")
            time.sleep(0.2)
        out, err = proc.communicate()
        return (proc.returncode, out or "", err or "")
    finally:
        try:
            if proc.stdout:
                proc.stdout.close()
            if proc.stderr:
                proc.stderr.close()
        except Exception:
            pass

def run_expired_reservations_cleanup(session: Session = None, cancel: Optional[object] = None) -> int:
    owns_session = False
    if session is None:
        session = get_session()
        owns_session = True

    try:
        if cancel is not None and getattr(cancel, "is_set", None) and cancel.is_set():
            logger.info("Shutdown requested before cleanup begins; skipping")
            return 0

        now = datetime.utcnow()
        expired = (
            session.query(
                Reservation.id,
                Reservation.machine_id,
                Reservation.username,
                Reservation.reserved_until,
                Machine.name,
                Machine.host,
                Machine.port,
                Machine.user,
                Machine.password,
            )
            .join(Machine, Machine.id == Reservation.machine_id)
            .filter(and_(Reservation.reserved_until.isnot(None), Reservation.reserved_until <= now))
            .all()
        )
        if not expired:
            logger.info("No expired reservations to clean up.")
            return 0

        playbook_path = _playbook_path()
        by_user = {}
        for res_id, machine_id, username, reserved_until, name, host, port, user, password in expired:
            by_user.setdefault(username, []).append((res_id, machine_id, name, host, port, user, password))

        processed = 0
        for username, tuples in by_user.items():
            if cancel is not None and getattr(cancel, "is_set", None) and cancel.is_set():
                logger.info("Shutdown requested; aborting cleanup loop")
                break

            with tempfile.NamedTemporaryFile(mode='w', delete=False, dir=_tmp_dir()) as inv:
                for _, _, name, host, port, user, password in tuples:
                    inv.write(f"{name} ansible_host={host} ansible_port={port} ansible_user={user} ansible_password={password}\n")
                inv_path = inv.name

            try:
                # Reduce SSH/connection waits by constraining ansible timeout
                rc, out, err = _run_ansible_with_cancel([
                    "ansible-playbook",
                    "-i", inv_path,
                    playbook_path,
                    "--extra-vars", f"username={username} user_action=delete ansible_ssh_timeout=15",
                ], cancel)

                if rc != 0:
                    logger.warning(f"Ansible returned rc={rc} for '{username}': {err.strip()}")

                # Clear DB state regardless (avoid dangling reservations)
                for res_id, machine_id, *_ in tuples:
                    m = session.query(Machine).filter(Machine.id == machine_id).one_or_none()
                    if m:
                        m.reserved = False
                        m.reserved_by = None
                        m.reserved_until = None
                    session.query(Reservation).filter(Reservation.id == res_id).delete()

                session.commit()
                processed += len(tuples)
            finally:
                if os.path.exists(inv_path):
                    os.unlink(inv_path)

        if processed:
            logger.info(f"Cleaned up {processed} expired reservations.")
        return processed
    finally:
        if owns_session:
            session.close()
