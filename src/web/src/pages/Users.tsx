import React, { useEffect, useState } from "react";
import { useAuth } from "../auth";
import { apiGet, apiPost, apiDelete } from "../client";

type User = { username: string; is_admin: boolean; created_at: string };

export default function Users() {
  const { token } = useAuth();
  const [users, setUsers] = useState<User[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [form, setForm] = useState({ username: "", password: "", is_admin: false });

  async function refresh() {
    try {
      setErr(null);
      const data = await apiGet("/users", token!);
      setUsers(data);
    } catch (e: any) {
      setErr(String(e));
    }
  }
  useEffect(() => { refresh(); }, []);

  async function addUser() {
    try {
      await apiPost("/users", form, token!);
      setForm({ username: "", password: "", is_admin: false });
      await refresh();
    } catch (e: any) {
      setErr(String(e));
    }
  }

  async function deleteUser(username: string) {
    try {
      await apiDelete(`/users/${encodeURIComponent(username)}`, token!);
      await refresh();
    } catch (e: any) {
      setErr(String(e));
    }
  }

  return (
    <div className="container">
      <h3 className="mb-3">Users (admin)</h3>
      {err && <div className="alert alert-danger">{err}</div>}

      <div className="card mb-3">
        <div className="card-body">
          <h5 className="card-title">Create user</h5>
          <div className="row g-2 align-items-end">
            <div className="col-md-4">
              <label className="form-label">Username</label>
              <input className="form-control" placeholder="username" value={form.username} onChange={e => setForm({ ...form, username: e.target.value })} />
            </div>
            <div className="col-md-4">
              <label className="form-label">Password</label>
              <input className="form-control" placeholder="password" value={form.password} onChange={e => setForm({ ...form, password: e.target.value })} />
            </div>
            <div className="col-md-2 form-check mt-4 ms-2">
              <input id="isAdmin" className="form-check-input" type="checkbox" checked={form.is_admin} onChange={e => setForm({ ...form, is_admin: e.target.checked })} />
              <label htmlFor="isAdmin" className="form-check-label">Admin</label>
            </div>
            <div className="col-md-2 mt-3 mt-md-0">
              <button className="btn btn-primary w-100" onClick={addUser}>Create</button>
            </div>
          </div>
        </div>
      </div>

      <div className="table-responsive">
        <table className="table table-striped align-middle">
          <thead><tr><th>Username</th><th>Admin</th><th>Created</th><th></th></tr></thead>
          <tbody>
            {users.map(u => (
              <tr key={u.username}>
                <td>{u.username}</td>
                <td>{u.is_admin ? "Yes" : "No"}</td>
                <td>{u.created_at}</td>
                <td className="text-end"><button className="btn btn-sm btn-outline-danger" onClick={() => deleteUser(u.username)}>Delete</button></td>
              </tr>
            ))}
            {users.length === 0 && <tr><td colSpan={4} className="text-muted">No users.</td></tr>}
          </tbody>
        </table>
      </div>
    </div>
  );
}
