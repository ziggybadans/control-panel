// Small shared presentational components.

import type { ReactNode } from "react";
import type { MCState } from "../api/types";
import { Icon, type IconName } from "./Icon";

export function Card({
  title,
  actions,
  children,
  flush,
  className,
}: {
  title?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  flush?: boolean;
  className?: string;
}) {
  return (
    <div className={className ? `card ${className}` : "card"}>
      {title !== undefined && (
        <div className="card-h">
          <span className="label">{title}</span>
          {actions && <div className="row right">{actions}</div>}
        </div>
      )}
      <div className={flush ? "card-b flush grow" : "card-b grow"}>{children}</div>
    </div>
  );
}

/** systemd ActiveState → badge. */
export function ServiceBadge({ active, sub }: { active: string; sub: string }) {
  if (active === "active" && (sub === "running" || sub === "waiting" || sub === "listening")) {
    return (
      <span className="badge ok">
        <span className="dot ok" />
        {sub}
      </span>
    );
  }
  if (active === "activating" || active === "deactivating" || active === "reloading") {
    return (
      <span className="badge warn">
        <span className="dot warn pulse" />
        {active}
      </span>
    );
  }
  if (active === "failed") {
    return (
      <span className="badge crit">
        <span className="dot crit" />
        failed
      </span>
    );
  }
  return (
    <span className="badge neutral">
      <span className="dot off" />
      {sub || active || "unknown"}
    </span>
  );
}

export function MCStateBadge({ state }: { state: MCState }) {
  switch (state) {
    case "running":
      return (
        <span className="badge ok">
          <span className="dot ok" />
          running
        </span>
      );
    case "starting":
      return (
        <span className="badge warn">
          <span className="dot warn pulse" />
          starting
        </span>
      );
    case "stopping":
      return (
        <span className="badge warn">
          <span className="dot warn pulse" />
          stopping
        </span>
      );
    case "crashed":
      return (
        <span className="badge crit">
          <span className="dot crit" />
          crashed
        </span>
      );
    default:
      return (
        <span className="badge neutral">
          <span className="dot off" />
          stopped
        </span>
      );
  }
}

export function EmptyState({
  icon,
  title,
  hint,
}: {
  icon: IconName;
  title: string;
  hint?: ReactNode;
}) {
  return (
    <div className="empty">
      <Icon name={icon} size={28} />
      <div style={{ color: "var(--text-2)", fontWeight: 550 }}>{title}</div>
      {hint && <div className="small">{hint}</div>}
    </div>
  );
}

export function Spinner({ size = 14 }: { size?: number }) {
  return (
    <svg className="spin" width={size} height={size} viewBox="0 0 24 24" fill="none" aria-label="loading">
      <circle cx="12" cy="12" r="9" stroke="var(--border-strong)" strokeWidth="2.5" />
      <path d="M21 12a9 9 0 0 0-9-9" stroke="var(--accent)" strokeWidth="2.5" strokeLinecap="round" />
    </svg>
  );
}

export function Toggle({
  checked,
  onChange,
  disabled,
  label,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  disabled?: boolean;
  label?: string;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      className="switch"
      disabled={disabled}
      onClick={() => onChange(!checked)}
    />
  );
}

/** Capacity bar with % label and threshold coloring. */
export function CapacityBar({
  used,
  total,
  thick,
}: {
  used: number;
  total: number;
  thick?: boolean;
}) {
  const pct = total > 0 ? (used / total) * 100 : 0;
  const level = pct >= 95 ? "crit" : pct >= 85 ? "warn" : "";
  return (
    <div className={thick ? "bar thick" : "bar"}>
      <i className={level} style={{ width: `${Math.min(pct, 100).toFixed(1)}%` }} />
    </div>
  );
}
