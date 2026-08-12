// Plugins/mods manager: list with metadata parsed from the jars, and
// enable/disable via the conventional .jar.disabled rename.

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState } from "react";
import { api, apiUpload } from "../../api/client";
import type { Addon, MCServer } from "../../api/types";
import { fmtBytes } from "../../lib/format";
import { Spinner, Toggle } from "../../ui/bits";
import { Icon } from "../../ui/Icon";
import { useToast } from "../../ui/Toast";

export function AddonsTab({ id, server }: { id: string; server: MCServer }) {
  const toast = useToast();
  const queryClient = useQueryClient();
  const [busyFile, setBusyFile] = useState<string | null>(null);
  const fileInput = useRef<HTMLInputElement>(null);

  const { data, isLoading } = useQuery({
    queryKey: ["mc-addons", id],
    queryFn: () => api<{ addons: Addon[] }>(`/api/minecraft/${id}/addons`),
    refetchInterval: 15_000,
  });

  const addons = data?.addons ?? [];
  const plugins = addons.filter((a) => a.dir === "plugins");
  const mods = addons.filter((a) => a.dir === "mods");
  // Where uploads land: whichever addon dir already exists, else by software.
  const uploadDir =
    plugins.length > 0 || /paper|purpur|spigot|bukkit/i.test(server.software ?? "")
      ? "plugins"
      : "mods";

  async function toggle(a: Addon, enable: boolean) {
    setBusyFile(a.file);
    try {
      await api(`/api/minecraft/${id}/addons/toggle`, {
        method: "POST",
        body: { dir: a.dir, file: a.file, enable },
      });
      toast("ok", `${a.name}: ${enable ? "enabled" : "disabled"} — applies on restart`);
      queryClient.invalidateQueries({ queryKey: ["mc-addons", id] });
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "toggle failed");
    } finally {
      setBusyFile(null);
    }
  }

  async function upload(files: File[]) {
    const jars = files.filter((f) => f.name.endsWith(".jar"));
    if (jars.length === 0) {
      toast("error", "only .jar files belong here");
      return;
    }
    try {
      await apiUpload(
        `/api/minecraft/${id}/files/upload?path=${encodeURIComponent(uploadDir)}`,
        jars,
      );
      toast("ok", `installed ${jars.length} into ${uploadDir}/ — applies on restart`);
      queryClient.invalidateQueries({ queryKey: ["mc-addons", id] });
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "upload failed");
    }
  }

  if (isLoading) {
    return (
      <div className="card-b">
        <Spinner />
      </div>
    );
  }

  return (
    <div className="card-b" style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      <div className="row">
        <span className="small muted">
          Disabling renames the jar to <span className="mono">.jar.disabled</span>;
          changes apply on the next restart.
        </span>
        <button
          className="btn btn-sm btn-primary right"
          onClick={() => fileInput.current?.click()}
        >
          <Icon name="plus" size={12} />
          Install jar → {uploadDir}/
        </button>
        <input
          ref={fileInput}
          type="file"
          accept=".jar"
          multiple
          hidden
          onChange={(e) => {
            void upload(Array.from(e.target.files ?? []));
            e.target.value = "";
          }}
        />
      </div>

      {addons.length === 0 && (
        <div className="small faint">
          No plugins or mods found — install a jar to create the {uploadDir}/ folder.
        </div>
      )}
      {plugins.length > 0 && (
        <AddonTable title={`Plugins · ${plugins.length}`} addons={plugins} busyFile={busyFile} onToggle={toggle} />
      )}
      {mods.length > 0 && (
        <AddonTable title={`Mods · ${mods.length}`} addons={mods} busyFile={busyFile} onToggle={toggle} />
      )}
    </div>
  );
}

function AddonTable({
  title,
  addons,
  busyFile,
  onToggle,
}: {
  title: string;
  addons: Addon[];
  busyFile: string | null;
  onToggle: (a: Addon, enable: boolean) => void;
}) {
  return (
    <section>
      <div className="label" style={{ marginBottom: 6 }}>
        {title}
      </div>
      <table className="table">
        <thead>
          <tr>
            <th>Name</th>
            <th>File</th>
            <th className="r">Size</th>
            <th className="r">Enabled</th>
          </tr>
        </thead>
        <tbody>
          {addons.map((a) => (
            <tr key={a.file} style={a.enabled ? undefined : { opacity: 0.55 }}>
              <td>
                <span style={{ fontWeight: 550 }}>{a.name}</span>
                {a.version && <span className="small faint"> {a.version}</span>}
              </td>
              <td className="mono small muted truncate" style={{ maxWidth: 260 }}>
                {a.file}
              </td>
              <td className="r num small muted">{fmtBytes(a.sizeBytes)}</td>
              <td className="r">
                {busyFile === a.file ? (
                  <Spinner size={13} />
                ) : (
                  <Toggle
                    checked={a.enabled}
                    onChange={(v) => onToggle(a, v)}
                    label={`${a.name} enabled`}
                  />
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </section>
  );
}
