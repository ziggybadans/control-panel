// App shell: sidebar navigation + topbar + content outlet.

import { useEffect, useRef, useState } from "react";
import { NavLink, Outlet, useLocation } from "react-router-dom";
import { useConnState, useJobs, useMCServers, useServices } from "../state/live";
import { useSystem } from "../state/system";
import { Icon, type IconName } from "../ui/Icon";
import { Spinner } from "../ui/bits";

const NAV: { to: string; label: string; icon: IconName }[] = [
  { to: "/", label: "Dashboard", icon: "dashboard" },
  { to: "/storage", label: "Storage", icon: "storage" },
  { to: "/files", label: "Files", icon: "folder" },
  { to: "/fans", label: "Fans", icon: "fan" },
  { to: "/services", label: "Services", icon: "services" },
  { to: "/minecraft", label: "Minecraft", icon: "minecraft" },
  { to: "/plex", label: "Plex", icon: "plex" },
  { to: "/apps", label: "Apps", icon: "download" },
  { to: "/terminal", label: "Terminal", icon: "terminal" },
  { to: "/activity", label: "Activity", icon: "activity" },
  { to: "/settings", label: "Settings", icon: "settings" },
];

const TITLES: Record<string, string> = {
  "/": "Dashboard",
  "/storage": "Storage",
  "/files": "Files",
  "/fans": "Fans",
  "/services": "Services",
  "/minecraft": "Minecraft",
  "/plex": "Plex",
  "/apps": "Apps",
  "/terminal": "Terminal",
  "/activity": "Activity",
  "/settings": "Settings",
};

export function Layout() {
  const conn = useConnState();
  const { info } = useSystem();
  const location = useLocation();
  const mcServers = useMCServers();
  const services = useServices();
  const jobs = useJobs();

  const runningMC = mcServers.filter(
    (s) => s.state === "running" || s.state === "starting",
  ).length;
  const failedServices = services.filter((s) => s.activeState === "failed").length;
  const activeJob = jobs.find((j) => j.state === "running");

  const title =
    TITLES[location.pathname] ??
    (location.pathname.startsWith("/minecraft/") ? "Minecraft" : "Control Panel");

  // Sliding active-item indicator: measured from the DOM so it tracks any
  // density/layout change, animated with transform only.
  const navRef = useRef<HTMLElement | null>(null);
  const [marker, setMarker] = useState<{ y: number; h: number } | null>(null);
  useEffect(() => {
    const nav = navRef.current;
    if (!nav) return;
    const measure = () => {
      const active = nav.querySelector<HTMLElement>(".nav-item.active");
      setMarker(active ? { y: active.offsetTop, h: active.offsetHeight } : null);
    };
    measure();
    const ro = new ResizeObserver(measure);
    ro.observe(nav);
    return () => ro.disconnect();
  }, [location.pathname]);

  return (
    <div className="app">
      <aside className="sidebar">
        <div className="sidebar-brand">
          <Icon name="chip" size={20} className="faint" />
          <div>
            <div className="host">{info?.hostname ?? "…"}</div>
            <div className="sub">{info?.os ?? "control panel"}</div>
          </div>
        </div>
        <nav className="nav" ref={navRef}>
          {marker && (
            <div
              className="nav-indicator"
              style={{ transform: `translateY(${marker.y}px)`, height: marker.h }}
            />
          )}
          {NAV.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              end={item.to === "/"}
              className={({ isActive }) => (isActive ? "nav-item active" : "nav-item")}
            >
              <Icon name={item.icon} className="icon" />
              {item.label}
              {item.to === "/minecraft" && runningMC > 0 && (
                <span className="count num">{runningMC} up</span>
              )}
              {item.to === "/services" && failedServices > 0 && (
                <span className="count crit-text num">{failedServices} failed</span>
              )}
            </NavLink>
          ))}
        </nav>
        <div className="sidebar-foot">
          {activeJob && (
            <div className="row small">
              <Spinner size={11} />
              <span className="truncate">
                {activeJob.kind} · {activeJob.target}
              </span>
            </div>
          )}
          <div className="row">
            <span
              className={`dot ${conn === "live" ? "ok" : "warn pulse"}`}
              title={conn}
            />
            <span>{conn === "live" ? "Live" : "Reconnecting…"}</span>
            {info?.mock && <span className="badge neutral right">mock</span>}
          </div>
          <div className="num">
            {info?.version ? (info.version === "dev" ? "dev build" : `v${info.version}`) : ""}
          </div>
        </div>
      </aside>
      <div className="main">
        <header className="topbar">
          {/* Keyed so the title replays its entrance on page change. */}
          <h1 key={title}>{title}</h1>
          <TopbarStatus />
        </header>
        <div className="content">
          <Outlet />
        </div>
      </div>
    </div>
  );
}

function TopbarStatus() {
  const { info, uptime } = useSystem();
  return (
    <div className="row right small muted">
      {info && (
        <>
          <span className="num" title="kernel">
            {info.kernel}
          </span>
          <span className="faint">·</span>
          <span className="num" title="uptime">
            up {uptime}
          </span>
        </>
      )}
    </div>
  );
}
