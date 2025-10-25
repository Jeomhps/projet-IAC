import React, { useState } from "react";
import { useAuth } from "../auth";
import { useNavigate } from "react-router-dom";

export default function Login() {
  const { login } = useAuth();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const navigate = useNavigate();

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError(null);
    const ok = await login(username.trim(), password);
    if (ok) navigate("/");
    else setError("Invalid credentials");
  }

  return (
    <div className="container" style={{ maxWidth: 480 }}>
      <div className="card shadow-sm">
        <div className="card-body">
          <h3 className="card-title mb-3">Sign in</h3>
          <form onSubmit={onSubmit} className="vstack gap-3">
            <div>
              <label className="form-label">Username</label>
              <input className="form-control" value={username} onChange={e => setUsername(e.target.value)} autoComplete="username" />
            </div>
            <div>
              <label className="form-label">Password</label>
              <input className="form-control" type="password" value={password} onChange={e => setPassword(e.target.value)} autoComplete="current-password" />
            </div>
            {error && <div className="alert alert-danger py-2">{error}</div>}
            <button className="btn btn-primary" type="submit" disabled={!username || !password}>Login</button>
          </form>
        </div>
      </div>
    </div>
  );
}
