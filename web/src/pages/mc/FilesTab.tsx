// File manager: browse, rename/move, mkdir, upload (incl. drag-drop),
// download, zip/unzip, delete — all confined server-side to the server dir.

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useRef, useState, type DragEvent } from "react";
import { api, apiUpload } from "../../api/client";
import type { FileEntry, FilesResponse, Job } from "../../api/types";
import { fmtBytes, fmtDateTime, fmtRelative } from "../../lib/format";
import { useJobs } from "../../state/live";
import { Spinner } from "../../ui/bits";
import { useConfirm } from "../../ui/Confirm";
import { Icon } from "../../ui/Icon";
import { useToast } from "../../ui/Toast";
import { JobPanel } from "../Storage";

export function FilesTab({ id }: { id: string }) {
  const [path, setPath] = useState("");
  const [renaming, setRenaming] = useState<FileEntry | null>(null);
  const [renameTo, setRenameTo] = useState("");
  const [dragOver, setDragOver] = useState(false);
  const [busy, setBusy] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);
  const toast = useToast();
  const confirm = useConfirm();
  const queryClient = useQueryClient();
  const jobs = useJobs();

  const { data, isLoading } = useQuery({
    queryKey: ["mc-files", id, path],
    queryFn: () =>
      api<FilesResponse>(
        `/api/minecraft/${id}/files?path=${encodeURIComponent(path)}`,
      ),
    refetchInterval: 10_000,
  });

  const zipJob = jobs.find(
    (j) => j.kind.startsWith("mc.file.") && j.target === id,
  );
  const [showJob, setShowJob] = useState<string | null>(null);

  const entries = data?.entries ?? [];
  const crumbs = path === "" ? [] : path.split("/");

  function refresh() {
    queryClient.invalidateQueries({ queryKey: ["mc-files", id] });
  }

  function join(...parts: string[]): string {
    return parts.filter(Boolean).join("/");
  }

  async function op(body: Record<string, unknown>, okMsg: string, confirmValue?: string) {
    setBusy(true);
    try {
      const res = await api<Job | { ok: boolean }>(`/api/minecraft/${id}/files/op`, {
        method: "POST",
        body,
        confirm: confirmValue,
      });
      if ("id" in res && "state" in res) setShowJob(res.id);
      toast("ok", okMsg);
      refresh();
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "operation failed");
    } finally {
      setBusy(false);
    }
  }

  async function remove(entry: FileEntry) {
    const rel = join(path, entry.name);
    const ok = await confirm({
      title: `Delete ${entry.name}`,
      target: entry.name,
      typed: entry.dir, // directories get the typed tier
      body: entry.dir ? (
        <>
          The folder <span className="mono">{rel}</span> and everything inside it will
          be permanently deleted from the server.
        </>
      ) : (
        <>
          <span className="mono">{rel}</span> ({fmtBytes(entry.size)}) will be
          permanently deleted.
        </>
      ),
      confirmLabel: "Delete",
    });
    if (!ok) return;
    await op({ action: "delete", path: rel }, `deleted ${entry.name}`, entry.name);
  }

  async function mkdir() {
    const name = window.prompt("New folder name:");
    if (!name) return;
    await op({ action: "mkdir", path: join(path, name) }, `created ${name}/`);
  }

  async function upload(files: File[]) {
    if (files.length === 0) return;
    setBusy(true);
    try {
      await apiUpload(
        `/api/minecraft/${id}/files/upload?path=${encodeURIComponent(path)}`,
        files,
      );
      toast("ok", `uploaded ${files.length} file${files.length > 1 ? "s" : ""}`);
      refresh();
    } catch (e) {
      toast("error", e instanceof Error ? e.message : "upload failed");
    } finally {
      setBusy(false);
    }
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    setDragOver(false);
    void upload(Array.from(e.dataTransfer.files));
  }

  return (
    <div
      className="card-b"
      style={{ display: "flex", flexDirection: "column", gap: 10 }}
      onDragOver={(e) => {
        if (e.dataTransfer.types.includes("Files")) {
          e.preventDefault();
          setDragOver(true);
        }
      }}
      onDragLeave={(e) => {
        if (e.currentTarget === e.target) setDragOver(false);
      }}
      onDrop={onDrop}
    >
      <div className="row wrap">
        <div className="row" style={{ gap: 4 }}>
          <button className="btn btn-ghost btn-sm mono" onClick={() => setPath("")}>
            {id}
          </button>
          {crumbs.map((c, i) => (
            <span key={i} className="row" style={{ gap: 4 }}>
              <span className="faint">/</span>
              <button
                className="btn btn-ghost btn-sm mono"
                onClick={() => setPath(crumbs.slice(0, i + 1).join("/"))}
              >
                {c}
              </button>
            </span>
          ))}
        </div>
        <div className="row right">
          {busy && <Spinner size={13} />}
          <button className="btn btn-sm" onClick={mkdir} disabled={busy}>
            <Icon name="plus" size={12} />
            Folder
          </button>
          <button
            className="btn btn-sm btn-primary"
            onClick={() => fileInput.current?.click()}
            disabled={busy}
          >
            <Icon name="download" size={12} />
            Upload
          </button>
          <input
            ref={fileInput}
            type="file"
            multiple
            hidden
            onChange={(e) => {
              void upload(Array.from(e.target.files ?? []));
              e.target.value = "";
            }}
          />
        </div>
      </div>

      {zipJob && (
        <JobPanel
          job={zipJob}
          expanded={showJob === zipJob.id || zipJob.state === "running"}
          onToggle={() => setShowJob(showJob === zipJob.id ? null : zipJob.id)}
        />
      )}

      <div
        style={
          dragOver
            ? {
                outline: "2px dashed var(--accent)",
                outlineOffset: 4,
                borderRadius: 6,
              }
            : undefined
        }
      >
        {isLoading ? (
          <Spinner />
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th>Name</th>
                <th className="r">Size</th>
                <th className="r">Modified</th>
                <th className="r" style={{ width: 210 }}>
                  Actions
                </th>
              </tr>
            </thead>
            <tbody>
              {path !== "" && (
                <tr
                  style={{ cursor: "pointer" }}
                  onClick={() => setPath(crumbs.slice(0, -1).join("/"))}
                >
                  <td className="mono muted" colSpan={4}>
                    ..
                  </td>
                </tr>
              )}
              {entries.map((e) => (
                <FileRow
                  key={e.name}
                  entry={e}
                  serverId={id}
                  rel={join(path, e.name)}
                  busy={busy}
                  onOpen={() => e.dir && setPath(join(path, e.name))}
                  onRename={() => {
                    setRenaming(e);
                    setRenameTo(join(path, e.name));
                  }}
                  onDelete={() => void remove(e)}
                  onZip={() =>
                    void op({ action: "zip", path: join(path, e.name) }, "zip started")
                  }
                  onUnzip={() =>
                    void op({ action: "unzip", path: join(path, e.name) }, "unzip started")
                  }
                />
              ))}
              {entries.length === 0 && (
                <tr>
                  <td colSpan={4} className="small faint">
                    Empty folder — drop files here to upload.
                  </td>
                </tr>
              )}
            </tbody>
          </table>
        )}
      </div>
      <div className="small faint">
        Drag &amp; drop uploads into the current folder. Renaming accepts a full path
        to move between folders. All operations stay inside this server's directory.
      </div>

      {renaming && (
        <div className="modal-overlay" onMouseDown={(e) => e.target === e.currentTarget && setRenaming(null)}>
          <div className="modal">
            <div className="modal-h">Rename / move {renaming.name}</div>
            <div className="modal-b">
              <div className="field">
                <span className="label">New path (relative to the server folder)</span>
                <input
                  className="input mono"
                  value={renameTo}
                  onChange={(e) => setRenameTo(e.target.value)}
                  autoFocus
                  spellCheck={false}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" && renameTo) {
                      void op(
                        { action: "rename", path: join(path, renaming.name), to: renameTo },
                        "renamed",
                      );
                      setRenaming(null);
                    }
                  }}
                />
              </div>
            </div>
            <div className="modal-f">
              <button className="btn" onClick={() => setRenaming(null)}>
                Cancel
              </button>
              <button
                className="btn btn-primary"
                disabled={!renameTo}
                onClick={() => {
                  void op(
                    { action: "rename", path: join(path, renaming.name), to: renameTo },
                    "renamed",
                  );
                  setRenaming(null);
                }}
              >
                Rename
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function FileRow({
  entry,
  serverId,
  rel,
  busy,
  onOpen,
  onRename,
  onDelete,
  onZip,
  onUnzip,
}: {
  entry: FileEntry;
  serverId: string;
  rel: string;
  busy: boolean;
  onOpen: () => void;
  onRename: () => void;
  onDelete: () => void;
  onZip: () => void;
  onUnzip: () => void;
}) {
  const isZip = entry.name.endsWith(".zip");
  return (
    <tr>
      <td
        onClick={onOpen}
        style={entry.dir ? { cursor: "pointer" } : undefined}
      >
        <span className="row" style={{ gap: 7 }}>
          <Icon
            name={entry.dir ? "storage" : isZip ? "archive" : "terminal"}
            size={13}
            className="faint"
          />
          <span className="mono" style={entry.dir ? { fontWeight: 600 } : undefined}>
            {entry.name}
            {entry.dir ? "/" : ""}
          </span>
        </span>
      </td>
      <td className="r num small muted">{entry.dir ? "—" : fmtBytes(entry.size)}</td>
      <td className="r num small muted" title={fmtDateTime(entry.modTime)}>
        {fmtRelative(entry.modTime)}
      </td>
      <td className="r">
        <div className="row" style={{ justifyContent: "flex-end", gap: 2 }}>
          {!entry.dir && (
            <a
              className="btn btn-ghost btn-sm btn-icon"
              href={`/api/minecraft/${serverId}/files/download?path=${encodeURIComponent(rel)}`}
              title="Download"
              download
            >
              <Icon name="download" size={12} />
            </a>
          )}
          {isZip ? (
            <button className="btn btn-ghost btn-sm" onClick={onUnzip} disabled={busy} title="Extract archive">
              Unzip
            </button>
          ) : (
            <button className="btn btn-ghost btn-sm" onClick={onZip} disabled={busy} title="Compress to .zip">
              Zip
            </button>
          )}
          <button className="btn btn-ghost btn-sm btn-icon" onClick={onRename} disabled={busy} title="Rename / move">
            <Icon name="edit" size={12} />
          </button>
          <button className="btn btn-ghost btn-sm btn-icon crit-text" onClick={onDelete} disabled={busy} title="Delete">
            <Icon name="trash" size={12} />
          </button>
        </div>
      </td>
    </tr>
  );
}
