import React, { useEffect, useState } from "react";
import { useAuth } from "../auth";
import { apiGet } from "../client";

export default function Available() {
  const { token } = useAuth();
  const [available, setAvailable] = useState<string[]>([]);
  const [reserved, setReserved] = useState<string[]>([]);
  const [err, setErr] = useState<string | null>(null);

  async function refresh() {
    try {
      setErr(null);
      const data = await apiGet("/available", token!);
      setAvailable(data.available || []);
      setReserved(data.reserved || []);
    } catch (e: any) {
      setErr(String(e));
    }
  }
  useEffect(() => { refresh(); }, []);

  return (
    <div className="container">
      <h3 className="mb-3">Pool</h3>
      {err && <div className="alert alert-danger">{err}</div>}
      <div className="row g-3">
        <div className="col-md-6">
          <div className="card">
            <div className="card-header">Available ({available.length})</div>
            <ul className="list-group list-group-flush">
              {available.map(a => <li key={a} className="list-group-item">{a}</li>)}
              {available.length === 0 && <li className="list-group-item text-muted">None</li>}
            </ul>
          </div>
        </div>
        <div className="col-md-6">
          <div className="card">
            <div className="card-header">Reserved ({reserved.length})</div>
            <ul className="list-group list-group-flush">
              {reserved.map(a => <li key={a} className="list-group-item">{a}</li>)}
              {reserved.length === 0 && <li className="list-group-item text-muted">None</li>}
            </ul>
          </div>
        </div>
      </div>
    </div>
  );
}
