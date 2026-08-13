// Minimal inline icon set (24x24 viewBox, 1.5px stroke), hand-picked so the
// bundle carries exactly what the panel uses.

const paths: Record<string, JSX.Element> = {
  dashboard: (
    <>
      <rect x="3" y="3" width="7.5" height="9" rx="1" />
      <rect x="13.5" y="3" width="7.5" height="5.5" rx="1" />
      <rect x="13.5" y="12" width="7.5" height="9" rx="1" />
      <rect x="3" y="15.5" width="7.5" height="5.5" rx="1" />
    </>
  ),
  storage: (
    <>
      <ellipse cx="12" cy="5.5" rx="8" ry="2.5" />
      <path d="M4 5.5v6c0 1.38 3.58 2.5 8 2.5s8-1.12 8-2.5v-6" />
      <path d="M4 11.5v6C4 18.88 7.58 20 12 20s8-1.12 8-2.5v-6" />
    </>
  ),
  services: (
    <>
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 1 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 1 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 1 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 1 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z" />
    </>
  ),
  minecraft: (
    <>
      <path d="M12 2 3 7v10l9 5 9-5V7l-9-5z" />
      <path d="M3 7l9 5 9-5" />
      <path d="M12 12v10" />
    </>
  ),
  play: <path d="M6 4.5 19 12 6 19.5v-15z" />,
  plex: (
    <>
      <rect x="2.5" y="4" width="19" height="13" rx="1.5" />
      <path d="M8 21h8M12 17v4" />
      <path d="M10 8l4 2.5-4 2.5v-5z" />
    </>
  ),
  activity: <path d="M2.5 12h4l3-8 5 16 3-8h4" />,
  settings: (
    <>
      <path d="M4 6h10M18 6h2M4 12h2M10 12h10M4 18h10M18 18h2" />
      <circle cx="16" cy="6" r="2" />
      <circle cx="8" cy="12" r="2" />
      <circle cx="16" cy="18" r="2" />
    </>
  ),
  power: (
    <>
      <path d="M12 2v9" />
      <path d="M17.5 5.5a8 8 0 1 1-11 0" />
    </>
  ),
  stop: <rect x="6" y="6" width="12" height="12" rx="1.5" />,
  restart: (
    <>
      <path d="M21 12a9 9 0 1 1-2.64-6.36" />
      <path d="M21 3v6h-6" />
    </>
  ),
  skull: (
    <>
      <path d="M12 2a8 8 0 0 0-8 8c0 2.5 1.1 4.6 3 6v4h10v-4c1.9-1.4 3-3.5 3-6a8 8 0 0 0-8-8z" />
      <circle cx="9" cy="10" r="1.3" fill="currentColor" stroke="none" />
      <circle cx="15" cy="10" r="1.3" fill="currentColor" stroke="none" />
      <path d="M10 20v-2M14 20v-2" />
    </>
  ),
  terminal: (
    <>
      <rect x="2.5" y="4" width="19" height="16" rx="1.5" />
      <path d="m7 9 3 3-3 3M13 15h4" />
    </>
  ),
  users: (
    <>
      <circle cx="9" cy="8" r="3.5" />
      <path d="M2.5 20c0-3.6 2.9-6 6.5-6s6.5 2.4 6.5 6" />
      <path d="M16 5.1a3.5 3.5 0 0 1 0 5.8M18.6 14.6c1.8.9 2.9 2.9 2.9 5.4" />
    </>
  ),
  archive: (
    <>
      <rect x="2.5" y="4" width="19" height="5" rx="1" />
      <path d="M4.5 9v9.5a1.5 1.5 0 0 0 1.5 1.5h12a1.5 1.5 0 0 0 1.5-1.5V9" />
      <path d="M10 13h4" />
    </>
  ),
  chip: (
    <>
      <rect x="6" y="6" width="12" height="12" rx="1.5" />
      <path d="M9 2v4M15 2v4M9 18v4M15 18v4M2 9h4M2 15h4M18 9h4M18 15h4" />
    </>
  ),
  clock: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3.5 2" />
    </>
  ),
  folder: (
    <path d="M3 6.5A1.5 1.5 0 0 1 4.5 5h4l2 2.5h9A1.5 1.5 0 0 1 21 9v9.5a1.5 1.5 0 0 1-1.5 1.5h-15A1.5 1.5 0 0 1 3 18.5v-12z" />
  ),
  file: (
    <>
      <path d="M6 3.5A1.5 1.5 0 0 1 7.5 2H14l4.5 4.5v14a1.5 1.5 0 0 1-1.5 1.5H7.5A1.5 1.5 0 0 1 6 20.5v-17z" />
      <path d="M14 2v5h5" />
    </>
  ),
  fan: (
    <>
      <circle cx="12" cy="12" r="2.2" />
      <path d="M12 9.8c0-3.2-1.2-5.8-3.4-5.8C6.8 4 6 5.4 6 6.7c0 2.3 2.7 3.4 6 3.1z" />
      <path d="M13.9 13.1c2.8 1.6 5.6 1.8 6.7-.1.9-1.6.2-3-1-3.7-2-1.1-4.3.6-5.7 3.8z" />
      <path d="M10.1 13.1c-2.8 1.6-4.4 3.9-3.3 5.8.9 1.6 2.5 1.7 3.6 1 2-1.1 1.7-4-.3-6.8z" />
    </>
  ),
  thermometer: (
    <>
      <path d="M10 4a2 2 0 1 1 4 0v9.5a4 4 0 1 1-4 0V4z" />
      <circle cx="12" cy="17" r="1.5" fill="currentColor" stroke="none" />
    </>
  ),
  network: (
    <>
      <rect x="9" y="2.5" width="6" height="5" rx="1" />
      <rect x="2.5" y="16.5" width="6" height="5" rx="1" />
      <rect x="15.5" y="16.5" width="6" height="5" rx="1" />
      <path d="M12 7.5v4.5M12 12H5.5v4.5M12 12h6.5v4.5" />
    </>
  ),
  check: <path d="m4.5 12.5 5 5 10-11" />,
  x: <path d="m6 6 12 12M18 6 6 18" />,
  warning: (
    <>
      <path d="M12 3 2.5 20h19L12 3z" />
      <path d="M12 9.5v4.5" />
      <circle cx="12" cy="17" r="1" fill="currentColor" stroke="none" />
    </>
  ),
  chevronDown: <path d="m6 9 6 6 6-6" />,
  chevronRight: <path d="m9 6 6 6-6 6" />,
  external: (
    <>
      <path d="M14 4h6v6" />
      <path d="M20 4 11 13" />
      <path d="M19 14v5a1.5 1.5 0 0 1-1.5 1.5h-11A1.5 1.5 0 0 1 5 19V8a1.5 1.5 0 0 1 1.5-1.5H10" />
    </>
  ),
  plus: <path d="M12 5v14M5 12h14" />,
  trash: (
    <>
      <path d="M4 7h16M9.5 7V4.5a1 1 0 0 1 1-1h3a1 1 0 0 1 1 1V7" />
      <path d="M6.5 7l1 13a1.5 1.5 0 0 0 1.5 1.4h6a1.5 1.5 0 0 0 1.5-1.4l1-13" />
      <path d="M10 11.5v5M14 11.5v5" />
    </>
  ),
  download: (
    <>
      <path d="M12 3v12M7 10l5 5 5-5" />
      <path d="M4 19.5h16" />
    </>
  ),
  refresh: (
    <>
      <path d="M20 12a8 8 0 1 1-2.34-5.66" />
      <path d="M20 4v4h-4" />
    </>
  ),
  eye: (
    <>
      <path d="M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12z" />
      <circle cx="12" cy="12" r="2.5" />
    </>
  ),
  logout: (
    <>
      <path d="M14 4H6.5A1.5 1.5 0 0 0 5 5.5v13A1.5 1.5 0 0 0 6.5 20H14" />
      <path d="M10 12h11M17.5 8.5 21 12l-3.5 3.5" />
    </>
  ),
  lock: (
    <>
      <rect x="5" y="10.5" width="14" height="10" rx="1.5" />
      <path d="M8 10.5V7a4 4 0 1 1 8 0v3.5" />
    </>
  ),
  edit: (
    <>
      <path d="M4 20h4.5L20 8.5a2.1 2.1 0 0 0-3-3L5.5 17 4 20z" />
      <path d="m14.5 7 3 3" />
    </>
  ),
  grip: (
    <>
      <circle cx="9" cy="6" r="1.1" fill="currentColor" stroke="none" />
      <circle cx="15" cy="6" r="1.1" fill="currentColor" stroke="none" />
      <circle cx="9" cy="12" r="1.1" fill="currentColor" stroke="none" />
      <circle cx="15" cy="12" r="1.1" fill="currentColor" stroke="none" />
      <circle cx="9" cy="18" r="1.1" fill="currentColor" stroke="none" />
      <circle cx="15" cy="18" r="1.1" fill="currentColor" stroke="none" />
    </>
  ),
};

export type IconName = keyof typeof paths;

export function Icon({
  name,
  size = 16,
  className,
}: {
  name: IconName;
  size?: number;
  className?: string;
}) {
  return (
    <svg
      className={className ? `icon ${className}` : "icon"}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.5"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      style={{ flex: "none" }}
    >
      {paths[name]}
    </svg>
  );
}
