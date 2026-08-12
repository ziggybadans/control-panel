// Map tab: embeds the web map served by Dynmap/BlueMap/squaremap/Pl3xMap
// when one is installed. The map runs on its own port on the same host as
// the panel; a manual URL override is kept per server in this browser.

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { MapInfo, MCServer } from "../../api/types";
import { EmptyState, Spinner } from "../../ui/bits";
import { Icon } from "../../ui/Icon";

export function MapTab({ id, server }: { id: string; server: MCServer }) {
  const storageKey = `cp-map-url-${id}`;
  const [override, setOverride] = useState(
    () => localStorage.getItem(storageKey) ?? "",
  );
  const [editing, setEditing] = useState(false);
  const [draft, setDraft] = useState(override);

  const { data, isLoading } = useQuery({
    queryKey: ["mc-map", id],
    queryFn: () => api<MapInfo>(`/api/minecraft/${id}/map`),
  });

  if (isLoading) {
    return (
      <div className="card-b">
        <Spinner />
      </div>
    );
  }

  const detectedURL = data?.detected
    ? `http://${window.location.hostname}:${data.port}`
    : "";
  const url = override || detectedURL;
  const running = server.state === "running";

  if (!url) {
    return (
      <div className="card-b">
        <EmptyState
          icon="eye"
          title="No web map detected"
          hint={
            <>
              Install Dynmap, BlueMap, squaremap, or Pl3xMap in this server's plugins/
              mods (the Addons tab can upload the jar) and the panel will pick it up.
              You can also{" "}
              <button className="btn btn-ghost btn-sm" onClick={() => setEditing(true)}>
                set a map URL manually
              </button>
            </>
          }
        />
        {editing && (
          <UrlEditor
            draft={draft}
            setDraft={setDraft}
            onSave={() => {
              localStorage.setItem(storageKey, draft);
              setOverride(draft);
              setEditing(false);
            }}
            onCancel={() => setEditing(false)}
          />
        )}
      </div>
    );
  }

  return (
    <div className="card-b flush" style={{ display: "flex", flexDirection: "column", flex: 1 }}>
      <div className="row" style={{ padding: "8px 12px" }}>
        <span className="small muted">
          {data?.detected ? (
            <>
              {data.type} on port <span className="num">{data.port}</span>
            </>
          ) : (
            "manual map URL"
          )}
          {!running && " — server is stopped, the map may be offline"}
        </span>
        <div className="row right">
          <button
            className="btn btn-ghost btn-sm"
            onClick={() => {
              setDraft(override || detectedURL);
              setEditing(true);
            }}
          >
            <Icon name="edit" size={12} />
            URL
          </button>
          <a className="btn btn-sm" href={url} target="_blank" rel="noreferrer">
            <Icon name="external" size={12} />
            Open full screen
          </a>
        </div>
      </div>
      {editing && (
        <div style={{ padding: "0 12px 8px" }}>
          <UrlEditor
            draft={draft}
            setDraft={setDraft}
            onSave={() => {
              localStorage.setItem(storageKey, draft);
              setOverride(draft);
              setEditing(false);
            }}
            onCancel={() => setEditing(false)}
          />
        </div>
      )}
      <iframe
        src={url}
        title={`${id} map`}
        style={{
          border: "none",
          borderTop: "1px solid var(--border)",
          flex: 1,
          minHeight: 420,
          width: "100%",
          background: "#0b0d10",
        }}
      />
    </div>
  );
}

function UrlEditor({
  draft,
  setDraft,
  onSave,
  onCancel,
}: {
  draft: string;
  setDraft: (v: string) => void;
  onSave: () => void;
  onCancel: () => void;
}) {
  return (
    <div className="row" style={{ gap: 6 }}>
      <input
        className="input mono grow"
        placeholder="http://server:8123"
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        spellCheck={false}
        autoFocus
      />
      <button className="btn btn-sm btn-primary" onClick={onSave}>
        Save
      </button>
      <button className="btn btn-sm" onClick={onCancel}>
        Cancel
      </button>
    </div>
  );
}
