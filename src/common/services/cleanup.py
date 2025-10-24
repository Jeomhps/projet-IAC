import logging
import os
import tempfile
import subprocess
from pathlib import Path
from sqlalchemy import and_
from sqlalchemy.orm import Session
from datetime import datetime
from common.db import get_session, Machine, Reservation

logger = logging.getLogger(__name__)

def _playbook_path() -> str:
    # common/services/ -> common/playbooks/create-users.yml
    return str((Path(__file__).resolve().parent.parent / "playbooks" / "create-users.yml").resolve())

def run_expired_reservations_cleanup(session: Session = None) -> int:
    """
    Finds expired reservations, removes users via Ansible, and clears DB state.
    Returns the number of expired reservations processed.
    """
    owns_session = False
    if session is None:
        session = get_session()
        owns_session = True

    try:
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
            by_user.setdefault(username, []).append(
                (res_id, machine_id, name, host, port, user, password)
            )

        processed = 0

        for username, tuples in by_user.items():
            with tempfile.NamedTemporaryFile(mode='w', delete=False) as temp_inventory:
                for _, _, name, host, port, user, password in tuples:
                    temp_inventory.write(
                        f"{name} ansible_host={host} ansible_port={port} ansible_user={user} ansible_password={password}\n"
                    )
                temp_inventory_path = temp_inventory.name

            try:
                result = subprocess.run([
                    "ansible-playbook",
                    "-i", temp_inventory_path,
                    playbook_path,
                    "--extra-vars", f"username={username} user_action=delete"
                ], capture_output=True, text=True)

                if result.returncode != 0:
                    logger.warning(f"Ansible error when deleting user '{username}': {result.stderr}")

                # Update DB regardless, to avoid dangling reservations
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
                if os.path.exists(temp_inventory_path):
                    os.unlink(temp_inventory_path)

        logger.info(f"Cleaned up {processed} expired reservations.")
        return processed
    finally:
        if owns_session:
            session.close()
