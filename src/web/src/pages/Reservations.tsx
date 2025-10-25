import React, { useEffect, useState } from "react";
import { useAuth, hasRole } from "../auth";
import { apiGet } from "../client";

type Item = {
  reservation_id: number;
  username: string;
  machine: string;
  host: string;
  port: number;
  reserved_until: string | null;
  seconds_remaining: number | null;
};

export default function Reservations() {
  const { token, roles, user } = useAuth();
  const [items, setItems] = useState<Item[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);
  const isAdmin = hasRole(roles, "admin");

  // Reserve form
  const [reserveCount, setReserveCount] = useState(1);
  const [reserveDuration, setReserveDuration] = useState(60);
  const [reservePassword, setReservePassword] = useState("");
  const [reserveUsername, setReserveUsername] = useState(user || "");

  async function refresh() {
    try {
      setErr(null);
      const data = await apiGet("/reservations", token!);
      setItems(data.reservations || []);
    } catch (e: any) {
      setErr(String(e));
    }
  }

  async function reserveNow() {
    setErr(null);
    setMsg(null);
    try {
      const params = new URLSearchParams();
      params.set("count", String(reserveCount));
      params.set("duration", String(reserveDuration));
      if (reserveUsername) params.set("username", reserveUsername);
      if (reservePassword) params.set("reservation_password", reservePassword);

      const resp = await fetch(`/api/reserve?${params.toString()}`, {
        headers: { Authorization: `Bearer ${token}` }
      });
      if (!resp.ok) throw new Error(await resp.text());
      const data = await resp.json().catch(() => ({}));
      setMsg(`Reserved ${reserveCount} machine(s).`);
      await refresh();
    } catch (e: any) {
      setErr(String(e));
    }
  }

  async function releaseAll() {
    setErr(null);
    setMsg(null);
    try {
      const resp = await fetch("/api/release_all", {
        headers: { Authorization: `Bearer ${token}` }
      });
      if (!resp.ok) throw new Error(await resp.text());
      setMsg("All reservations released.");
      await refresh();
    } catch (e: any) {
      setErr(String(e));
    }
  }

  useEffect(() => { refresh(); }, []);

  return (
    <div className="container">
      <div className="d-flex align-items-center justify-content-between mb-3">
        <h3 className="mb-0">Reservations</h3>
        {isAdmin && (
          <button className="btn btn-outline-danger btn-sm" onClick={releaseAll}>
            Release All (admin)
          </button>
        )}
      </div>

      <div className="card mb-3">
        <div className="card-body">
          <h5 className="card-title">Reserve machines</h5>
          <div className="row g-2">
            <div className="col-sm-2">
              <label className="form-label">Count</label>
              <input className="form-control" type="number" min={1} value={reserveCount} onChange={e => setReserveCount(Number(e.target.value))} />
            </div>
            <div className="col-sm-2">
              <label className="form-label">Duration (min)</label>
              <input className="form-control" type="number" min={1} value={reserveDuration} onChange={e => setReserveDuration(Number(e.target.value))} />
            </div>
            <div className="col-sm-4">
              <label className="form-label">Reservation password</label>
              <input className="form-control" type="password" placeholder="Password to set on machines" value={reservePassword} onChange={e => setReservePassword(e.target.value)} />
            </div>
            <div className="col-sm-4">
              <label className="form-label">Username (optional)</label>
              <input className="form-control" placeholder="Defaults to your API user" value={reserveUsername} onChange={e => setReserveUsername(e.target.value)} />
            </div>
          </div>
          <button className="btn btn-primary mt-3" onClick={reserveNow} disabled={reserveCount < 1 || reserveDuration < 1 || !reservePassword}>
            Reserve
          </button>
        </div>
      </div>

      {msg && <div className="alert alert-success">{msg}</div>}
      {err && <div className="alert alert-danger">{err}</div>}

      <div className="table-responsive">
        <table className="table table-striped align-middle">
          <thead><tr><th>User</th><th>Machine</th><th>Host</th><th>Port</th><th>Reserved until</th><th>Remaining</th></tr></thead>
          <tbody>
            {items.map(r => (
              <tr key={r.reservation_id}>
                <td>{r.username}</td>
                <td>{r.machine}</td>
                <td>{r.host}</td>
                <td>{r.port}</td>
                <td>{r.reserved_until || "-"}</td>
                <td>{r.seconds_remaining != null ? `${r.seconds_remaining}s` : "-"}</td>
              </tr>
            ))}
            {items.length === 0 && <tr><td colSpan={6} className="text-muted">No active reservations.</td></tr>}
          </tbody>
        </table>
      </div>
    </div>
  );
}
