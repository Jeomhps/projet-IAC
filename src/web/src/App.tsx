import React from "react";
import { Navigate, Route, Routes, Link, useNavigate } from "react-router-dom";
import { useAuth, hasRole } from "./auth";
import Login from "./pages/Login";
import Machines from "./pages/Machines";
import Reservations from "./pages/Reservations";
import Available from "./pages/Available";
import Users from "./pages/Users";

function Protected({ children }: { children: React.ReactNode }) {
  const { token } = useAuth();
  if (!token) return <Navigate to="/login" replace />;
  return <>{children}</>;
}
function AdminOnly({ children }: { children: React.ReactNode }) {
  const { roles } = useAuth();
  if (!hasRole(roles, "admin")) return <Navigate to="/" replace />;
  return <>{children}</>;
}

function NavBar() {
  const { user, roles, logout, token } = useAuth();
  const navigate = useNavigate();
  const isAdmin = hasRole(roles, "admin");

  return (
    <nav className="navbar navbar-expand-lg navbar-dark bg-dark mb-3">
      <div className="container">
        <Link className="navbar-brand" to="/">Projet IAC</Link>
        <button className="navbar-toggler" type="button" data-bs-toggle="collapse" data-bs-target="#navbars">
          <span className="navbar-toggler-icon"></span>
        </button>
        <div id="navbars" className="collapse navbar-collapse">
          {token && (
            <ul className="navbar-nav me-auto mb-2 mb-lg-0">
              <li className="nav-item"><Link className="nav-link" to="/machines">Machines</Link></li>
              <li className="nav-item"><Link className="nav-link" to="/available">Available</Link></li>
              <li className="nav-item"><Link className="nav-link" to="/reservations">Reservations</Link></li>
              {isAdmin && <li className="nav-item"><Link className="nav-link" to="/users">Users</Link></li>}
            </ul>
          )}
          <ul className="navbar-nav ms-auto">
            {!token ? (
              <li className="nav-item"><Link className="nav-link" to="/login">Login</Link></li>
            ) : (
              <li className="nav-item d-flex align-items-center gap-2">
                <span className="navbar-text text-light">
                  {user} {isAdmin && <span className="badge bg-warning text-dark">admin</span>}
                </span>
                <button className="btn btn-outline-light btn-sm" onClick={() => { logout(); navigate("/login"); }}>
                  Logout
                </button>
              </li>
            )}
          </ul>
        </div>
      </div>
    </nav>
  );
}

function Dashboard() {
  const { token } = useAuth();
  return (
    <div className="container">
      <div className="p-4 bg-light border rounded-3">
        <h2 className="mb-3">Welcome</h2>
        {!token ? (
          <p>Please <Link to="/login">login</Link> to continue.</p>
        ) : (
          <ul>
            <li><Link to="/machines">Manage Machines</Link></li>
            <li><Link to="/available">Pool Availability</Link></li>
            <li><Link to="/reservations">Reservations</Link></li>
            <li><Link to="/users">User Management (admin)</Link></li>
          </ul>
        )}
      </div>
    </div>
  );
}

export default function App() {
  return (
    <>
      <NavBar />
      <Routes>
        <Route path="/" element={<Protected><Dashboard /></Protected>} />
        <Route path="/login" element={<Login />} />
        <Route path="/machines" element={<Protected><Machines /></Protected>} />
        <Route path="/available" element={<Protected><Available /></Protected>} />
        <Route path="/reservations" element={<Protected><Reservations /></Protected>} />
        <Route path="/users" element={<Protected><AdminOnly><Users /></AdminOnly></Protected>} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </>
  );
}
