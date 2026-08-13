// General file manager over configured roots: browse, upload (drag-drop,
// with overwrite-retry), download (folders stream as zip), rename/move,
// mkdir, unzip, delete. Mutating actions disappear on read-only roots;
// deletes confirm server-side (typed for directories).

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState, type DragEvent } from "react";
import { api, apiUpload, ApiError } from "../api/client";
import type {
  FileEntry,
  FilesListResponse,
  FilesRoot,
  FilesRootsResponse,
  Job,
} from "../api/types";
import { fmtBytes, fmtDateTime, fmtRelative } from "../lib/format";
import { useJobs } from "../state/live";
import { EmptyState, Spinner } from "../ui/bits";
import { useConfirm } from "../ui/Confirm";
import { Icon } from "../ui/Icon";
import { useToast } from "../ui/Toast";
import { JobPanel } from "./Storage";

export function FilesPage() {
  const { data } = useQuery({
    queryKey: ["files-roots"],
    queryFn: () => api<FilesRootsResponse>("/api/files"),
    staleTime: 60_000,
  });
  const [rootName, setRootName] = useState<string | null>(null);

  if (!data) {
    return (
      <div className="page">
        <Spinner />
      </div>
    );
  }
  const roots = data.roots ?? [];
  if (!data.configured || roots.length === 0) {
    return (
      <div className="page">
        <EmptyState
          icon="folder"
          title="No file roots configured"
          hint={
            <>
              List directories under <span className="mono">files.roots</span> in
              config.yaml (name + path, optional{" "}
              <span className="mono">read_only</span>) and restart the panel.
            </>
          }
        />
      </div>
    );
  }
  const active = roots.find((r) => r.name === rootName) ?? roots[0];

  return (
    <div className="page">
      <div className="row wrap">
        <div className="choice-row" role="tablist" aria-label="file roots">
          {roots.map((r) => (
            <button
              key={r.name}
              role="tab"
              aria-selected={r.name === active.name}
              className={`choice ${r.name === active.name ? "selected" : ""}`}
              onClick={() => setRootName(r.name)}
            >
              {r.name}
            </button>
          ))}
        </div>
        {active.readOnly && <span className="badge neutral">read-only</span>}
        <span className="small faint mono right">{active.path}</span>
      </div>
      <Browser key={active.name} root={active} />
    </div>
  );
}

function Browser({ root }: { root: FilesRoot }) {
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
  const [showJob, setShowJob] = useState<string | null>(null);

  const { data, isLoading } = useQuery({
    queryKey: ["files", root.name, path],
    queryFn: () =>
      api<FilesListResponse>(
        `/api/files/list?root=${encodeURIComponent(root.name)}&path=${encodeURIComponent(path)}`,
      ),
    refetchInterval: 10_000,
  });

  const unzipJob = jobs.find((j) => j.kind === "files.unzip" && j.target === root.name);
  const entries = data?.entries ?? [];
  const crumbs = path === "" ? [] : path.split("/");
  const writable = !root.readOnly;

  // Root switches reset the cursor (component is keyed, but keep tidy).
  useEffect(() => setPath(""), [root.name]);

  function refresh() {
    queryClient.invalidateQueries({ queryKey: ["files", root.name] });
  }

  function join(...parts: string[]): string {
    return parts.filter(Boolean).join("/");
  }

  function downloadURL(rel: string): string {
    return `/api/files/download?root=${encodeURIComponent(root.name)}&path=${encodeURIComponent(rel)}`;
  }

  async function op(body: Record<string, unknown>, okMsg: string, confirmValue?: string) {
    setBusy(true);
    try {
      const res = await api<Job | { ok: boolean }>("/api/files/op", {
        method: "POST",
        body: { root: root.name, ...body },
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
          The folder <span className="mono">{root.name}:{rel}</span> and everything
          inside it will be permanently deleted from the server.
        </>
      ) : (
        <>
          <span className="mono">{root.name}:{rel}</span> ({fmtBytes(entry.size)})
          will be permanently deleted.
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

  async function upload(fileList: File[], overwrite = false) {
    if (fileList.length === 0) return;
    setBusy(true);
    try {
      await apiUpload(
        `/api/files/upload?root=${encodeURIComponent(root.name)}&path=${encodeURIComponent(path)}${overwrite ? "&overwrite=1" : ""}`,
        fileList,
      );
      toast("ok", `uploaded ${fileList.length} file${fileList.length > 1 ? "s" : ""}`);
      refresh();
    } catch (e) {
      // A name collision offers a one-click overwrite retry.
      if (!overwrite && e instanceof ApiError && /already exists/.test(e.message)) {
        setBusy(false);
        const ok = await confirm({
          title: "Overwrite existing files?",
          target: "overwrite",
          body: (
            <>
              {e.message}. Upload again and <b>replace</b> the existing file
              {fileList.length > 1 ? "s" : ""}?
            </>
          ),
          confirmLabel: "Overwrite",
        });
        if (ok) await upload(fileList, true);
        return;
      }
      toast("error", e instanceof Error ? e.message : "upload failed");
    } finally {
      setBusy(false);
    }
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    setDragOver(false);
    if (writable) void upload(Array.from(e.dataTransfer.files));
  }

  return (
    <div
      className="card"
      onDragOver={(e) => {
        if (writable && e.dataTransfer.types.includes("Files")) {
          e.preventDefault();
          setDragOver(true);
        }
      }}
      onDragLeave={(e) => {
        if (e.currentTarget === e.target) setDragOver(false);
      }}
      onDrop={onDrop}
    >
      <div className="card-b" style={{ display: "flex", flexDirection: "column", gap: 10 }}>
        <div className="row wrap">
          <div className="row" style={{ gap: 4 }}>
            <button className="btn btn-ghost btn-sm mono" onClick={() => setPath("")}>
              {root.name}
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
            <a
              className="btn btn-sm"
              href={downloadURL(path)}
              title="Download this folder as .zip"
            >
              <Icon name="archive" size={12} />
              Download folder
            </a>
            {writable && (
              <>
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
              </>
            )}
          </div>
        </div>

        {unzipJob && (
          <JobPanel
            job={unzipJob}
            expanded={showJob === unzipJob.id || unzipJob.state === "running"}
            onToggle={() => setShowJob(showJob === unzipJob.id ? null : unzipJob.id)}
          />
        )}

        <div
          style={
            dragOver
              ? { outline: "2px dashed var(--accent)", outlineOffset: 4, borderRadius: 6 }
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
                  <th className="r" style={{ width: 190 }}>
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
                  <Row
                    key={e.name}
                    entry={e}
                    rel={join(path, e.name)}
                    writable={writable}
                    busy={busy}
                    downloadURL={downloadURL}
                    onOpen={() => e.dir && setPath(join(path, e.name))}
                    onRename={() => {
                      setRenaming(e);
                      setRenameTo(join(path, e.name));
                    }}
                    onDelete={() => void remove(e)}
                    onUnzip={() =>
                      void op({ action: "unzip", path: join(path, e.name) }, "unzip started")
                    }
                  />
                ))}
                {entries.length === 0 && (
                  <tr>
                    <td colSpan={4} className="small faint">
                      Empty folder{writable ? " — drop files here to upload." : "."}
                    </td>
                  </tr>
                )}
              </tbody>
            </table>
          )}
        </div>
        <div className="small faint">
          Folder downloads stream as uncompressed zip — full disk speed on the LAN.
          {writable &&
            " Drag & drop uploads into the current folder. Renaming accepts a full path to move between folders."}{" "}
          All operations stay inside the configured root.
        </div>
      </div>

      {renaming && (
        <div
          className="modal-overlay"
          onMouseDown={(e) => e.target === e.currentTarget && setRenaming(null)}
        >
          <div className="modal">
            <div className="modal-h">Rename / move {renaming.name}</div>
            <div className="modal-b">
              <div className="field">
                <span className="label">New path (relative to {root.name})</span>
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

function Row({
  entry,
  rel,
  writable,
  busy,
  downloadURL,
  onOpen,
  onRename,
  onDelete,
  onUnzip,
}: {
  entry: FileEntry;
  rel: string;
  writable: boolean;
  busy: boolean;
  downloadURL: (rel: string) => string;
  onOpen: () => void;
  onRename: () => void;
  onDelete: () => void;
  onUnzip: () => void;
}) {
  const isZip = entry.name.endsWith(".zip");
  return (
    <tr>
      <td onClick={onOpen} style={entry.dir ? { cursor: "pointer" } : undefined}>
        <span className="row" style={{ gap: 7 }}>
          <Icon
            name={entry.dir ? "folder" : isZip ? "archive" : "file"}
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
          <a
            className="btn btn-ghost btn-sm btn-icon"
            href={downloadURL(rel)}
            title={entry.dir ? "Download as .zip" : "Download"}
            download
          >
            <Icon name="download" size={12} />
          </a>
          {writable && isZip && (
            <button
              className="btn btn-ghost btn-sm"
              onClick={onUnzip}
              disabled={busy}
              title="Extract archive"
            >
              Unzip
            </button>
          )}
          {writable && (
            <>
              <button
                className="btn btn-ghost btn-sm btn-icon"
                onClick={onRename}
                disabled={busy}
                title="Rename / move"
              >
                <Icon name="edit" size={12} />
              </button>
              <button
                className="btn btn-ghost btn-sm btn-icon crit-text"
                onClick={onDelete}
                disabled={busy}
                title="Delete"
              >
                <Icon name="trash" size={12} />
              </button>
            </>
          )}
        </div>
      </td>
    </tr>
  );
}
