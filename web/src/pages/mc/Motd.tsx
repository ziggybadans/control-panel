// Server-list-style MOTD chip: the server icon next to the advertised MOTD
// rendered with Minecraft's color codes, on the console-dark background so
// it looks like the real multiplayer list in both themes.

import type { MOTDSegment } from "../../api/types";

/** The classic 16-color Minecraft palette. */
const MC_COLORS: Record<string, string> = {
  black: "#000000",
  dark_blue: "#0000AA",
  dark_green: "#00AA00",
  dark_aqua: "#00AAAA",
  dark_red: "#AA0000",
  dark_purple: "#AA00AA",
  gold: "#FFAA00",
  gray: "#AAAAAA",
  dark_gray: "#555555",
  blue: "#5555FF",
  green: "#55FF55",
  aqua: "#55FFFF",
  red: "#FF5555",
  light_purple: "#FF55FF",
  yellow: "#FFFF55",
  white: "#FFFFFF",
};

export function MotdText({ segments }: { segments: MOTDSegment[] }) {
  return (
    <span className="mc-motd">
      {segments.map((seg, i) => (
        <span
          key={i}
          style={{
            color: seg.color
              ? seg.color.startsWith("#")
                ? seg.color
                : MC_COLORS[seg.color]
              : undefined,
            fontWeight: seg.bold ? 700 : undefined,
            fontStyle: seg.italic ? "italic" : undefined,
            textDecoration:
              [seg.underline && "underline", seg.strike && "line-through"]
                .filter(Boolean)
                .join(" ") || undefined,
          }}
        >
          {seg.text}
        </span>
      ))}
    </span>
  );
}

/** Icon + MOTD block, shown only when the ping has produced something. */
export function ServerListEntry({
  icon,
  motd,
}: {
  icon?: string;
  motd?: MOTDSegment[];
}) {
  if (!icon && !(motd && motd.length > 0)) return null;
  return (
    <div className="mc-slp">
      {icon && (
        <img
          src={icon}
          width={40}
          height={40}
          alt=""
          style={{ borderRadius: 4, imageRendering: "pixelated", flex: "none" }}
        />
      )}
      {motd && motd.length > 0 && <MotdText segments={motd} />}
    </div>
  );
}
