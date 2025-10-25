import React, { useEffect, useState } from "react";
import { useAuth, hasRole } from "../auth";
import { apiDelete, apiGet, apiPost } from "../client";

type Machine = {
  name: string;
  host: string;
  port: number;
  user: string;
  reserved: boolean;
  reserved_by?: string | null;
  reserved_until?: string | null;
};

export default function Machines() {
  const { token, roles } = useAuth();
  const [list, setList] = useState<Machine[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const isAdmin = hasRole(roles, "admin");

  const [form, setForm] = useState({ name: "", host: "", port: 22, user: "root", password: "" });

  async function refresh() {
    try {
      setErr(null);
      const data = await apiGet("/machines", token!);
      setList(data);
    } catch (e: any) {
      setErr(String(e));
    }
  }

  async function addMachine() {
    try {
      setErr(null);
      await apiPost("/machines", form, token!);
      setForm({ name: "", host: "", port: 22, user: "root", password: "" });
      await refresh();
    } catch (e: any) {
      setErr(String(e));
    }
  }

  async function delMachine(name: string) {
    try {
      setErr(null);
      await apiDelete(`/machines/${encodeURIComponent(name)}`, token!);
      await refresh();
    } catch (e: any) {
      setErr(String(e));
    }
  }

  useEffect(() => { refresh(); }, []);

  return (
    <div className="container">
      <div className="d-flex align-items-center justify-content-between mb-3">
        <h3 className="mb-0">Machines</h3>
      </div>

      {err && <div className="alert alert-danger">{err}</div>}

      {isAdmin && (
        <div className="card mb-3">
          <div className="card-body">
            <h5 className="card-title">Add machine</h5>
            <div className="row g-2">
              <div className="col-sm-3"><input className="form-control" placeholder="name" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} /></div>
              <div className="col-sm-3"><input className="form-control" placeholder="host" value={form.host} onChange={e => setForm({ ...form, host: e.target.value })} /></div>
              <div className="col-sm-2"><input className="form-control" placeholder="port" type="number" value={form.port} onChange={e => setForm({ ...form, port: Number(e.target.value) })} /></div>
              <div className="col-sm-2"><input className="form-control" placeholder="user" value={form.user} onChange={e => setForm({ ...form, user: e.target.value })} /></div>
              <div className="col-sm-2"><input className="form-control" placeholder="password" value={form.password} onChange={e => setForm({ ...form, password: e.target.value })} /></div>
            </div>
            <button className="btn btn-primary mt-2" onClick={addMachine}>Add</button>
          </div>
        </div>
      )}

      <div className="table-responsive">
        <table className="table table-striped align-middle">
          <thead><tr><th>Name</th><th>Host</th><th>Port</th><th>User</th><th>Reserved</th><th>By</th><th>Until</th><th></th></tr></thead>
          <tbody>
            {list.map(m => (
              <tr key={m.name}>
                <td>{m.name}</td>
                <td>{m.host}</td>
                <td>{m.port}</td>
                <td>{m.user}</td>
                <td>{m.reserved ? <span className="badge bg-danger">Yes</span> : <span className="badge bg-success">No</span>}</td>
                <td>{m.reserved_by || "-"}</td>
                <td>{m.reserved_until || "-"}</td>
                <td className="text-end">{isAdmin && <button className="btn btn-sm btn-outline-danger" onClick={() => delMachine(m.name)}>Delete</button>}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
