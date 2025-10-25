import os
import time
import logging
import tempfile
import subprocess
from datetime import datetime
from typing import Optional
from sqlalchemy import and_
from sqlalchemy.orm import Session
from common.db import get_session, Machine, Reservation
from pathlib import Path

logger = logging.getLogger(__name__)

def _tmp_dir() -> str:
    return os.getenv("TMPDIR", "/dev/shm")

def _playbook_path() -> str:
    # common/services/ -> common/playbooks/create-users.yml
    return str((Path(__file__).resolve().parent.parent / "playbooks" / "create-users.yml").resolve())

def _run_ansible_with_cancel(cmd: list[str], cancel: Optional[object], forks: Optional[int] = None) -> tuple[int, str, str]:
    """
    Run ansible-playbook with optional forks, and honor a shutdown event.
    - Do NOT pipe stdout/stderr to avoid blocking on large outputs.
    - Insert '-f <forks>' if provided.
    - Return (rc, "", "").
    """
    if forks:
        if cmd and os.path.basename(cmd[0]) == "ansible-playbook":
            cmd = [cmd[0], "-f", str(forks)] + cmd[1:]
        else:
            cmd = cmd + ["-f", str(forks)]

    proc = subprocess.Popen(cmd)
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
        return (proc.returncode, "", "")
    finally:
        # nothing to close when not piping
        pass

def run_expired_reservations_cleanup(session: Session = None, cancel: Optional[object] = None) -> int:
    """
    For each expired username group:
      - Build a temporary inventory file
      - Run ansible-playbook to delete the user on those hosts (in batches)
      - Regardless of ansible rc, clear DB reservations to avoid dangling state
    """
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

        total_cleared = 0
        batch_size = int(os.getenv("CLEANUP_BATCH_SIZE", "20"))
        forks = int(os.getenv("ANSIBLE_FORKS", "5"))

        for username, tuples in by_user.items():
            if cancel is not None and getattr(cancel, "is_set", None) and cancel.is_set():
                logger.info("Shutdown requested; aborting cleanup loop")
                break

            # Process in batches to limit parallel SSH pressure
            for i in range(0, len(tuples), batch_size):
                chunk = tuples[i:i + batch_size]

                # Build a temp inventory file for this chunk
                with tempfile.NamedTemporaryFile(mode='w', delete=False, dir=_tmp_dir()) as inv:
                    for _, _, name, host, port, user, password in chunk:
                        inv.write(f"{name} ansible_host={host} ansible_port={port} ansible_user={user} ansible_password={password}\n")
                    inv_path = inv.name

                try:
                    rc, _, err = _run_ansible_with_cancel([
                        "ansible-playbook",
                        "-i", inv_path,
                        playbook_path,
                        "--extra-vars", f"username={username} user_action=delete ansible_ssh_timeout=15",
                    ], cancel, forks=forks)

                    if rc != 0 and rc != 143:
                        logger.warning(f"Ansible returned rc={rc} for '{username}' batch starting at {i}: {err}")
                finally:
                    try:
                        os.unlink(inv_path)
                    except Exception:
                        pass

            # Regardless of ansible result, clear DB reservations to avoid dangling state
            for res_id, machine_id, *_ in tuples:
                m = session.query(Machine).filter(Machine.id == machine_id).one_or_none()
                if m:
                    m.reserved = False
                    m.reserved_by = None
                    m.reserved_until = None
                session.query(Reservation).filter(Reservation.id == res_id).delete()
                total_cleared += 1

        session.commit()
        logger.info(f"Cleaned up {total_cleared} expired reservations.")
        return total_cleared
    finally:
        if owns_session:
            session.close()
