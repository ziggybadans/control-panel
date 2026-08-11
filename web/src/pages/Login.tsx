import { useState, type FormEvent } from "react";
import { api, ApiError } from "../api/client";
import { Icon } from "../ui/Icon";
import { Spinner } from "../ui/bits";

export function Login({ onSuccess }: { onSuccess: () => void }) {
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit(e: FormEvent) {
    e.preventDefault();
    if (!password || busy) return;
    setBusy(true);
    setError("");
    try {
      await api("/api/auth/login", { method: "POST", body: { password } });
      onSuccess();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "login failed");
      setBusy(false);
    }
  }

  return (
    <div className="login-page">
      <form className="card login-card" onSubmit={submit}>
        <div className="row" style={{ justifyContent: "center", marginBottom: 4 }}>
          <Icon name="lock" size={22} className="faint" />
        </div>
        <h1 style={{ textAlign: "center", fontSize: "var(--fs-lg)" }}>Control Panel</h1>
        <p className="small muted" style={{ textAlign: "center" }}>
          Enter the panel password to continue
        </p>
        <input
          className="input"
          type="password"
          placeholder="Password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          autoFocus
          autoComplete="current-password"
        />
        {error && <div className="small crit-text">{error}</div>}
        <button className="btn btn-primary" type="submit" disabled={!password || busy}>
          {busy ? <Spinner size={13} /> : "Sign in"}
        </button>
      </form>
    </div>
  );
}
