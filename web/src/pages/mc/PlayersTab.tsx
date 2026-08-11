// Player management: online players with moderation actions, whitelist,
// operators, and bans. Live-server actions go through console commands so
// the server stays the source of truth.

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { api } from "../../api/client";
import type { MCServer, PlayerInfo } from "../../api/types";
import { Spinner } from "../../ui/bits";
import { useConfirm } from "../../ui/Confirm";
import { Icon } from "../../ui/Icon";
import { useToast } from "../../ui/Toast";

export function PlayersTab({ id, server }: { id: string; server: MCServer }) {
  const toast = useToast();
  const confirm = useConfirm();
  const queryClient = useQueryClient();
  const [wlInput, setWlInput] = useState("");
  const running = server.state === "running";

  const { data, isLoading } = useQuery({
    queryKey: ["mc-players", id],
    queryFn: () => api<PlayerInfo>(`/api/minecraft/${id}/players`),
    refetchInterval: 5000,
  });

  async function playerAction(action: string, player: string) {
    if (action === "ban") {
      const ok = await confirm({
        title: `Ban ${player}`,
        target: player,
        typed: true,
        body: `${player} will be disconnected and unable to rejoin ${id} until pardoned.`,
        confirmLabel: "Ban player",
      });
      if (!ok) return;
    } else if (action === "kick") {
      const ok = await confirm({
        title: `Kick ${player}`,
        target: player,
        body: `${player} will be disconnected but can rejoin immediately.`,
        confirmLabel: "Kick",
      });
      if (!ok) return;
    }
    try {
      await api(`/api/minecraft/${id}/players/${action}`, {
        method: "POST",
        body: { player },
        confirm: action === "ban" ? player : undefined,
      });
      toast("ok", `${action} ${player}`);
      setTimeout(
        () => queryClient.invalidateQueries({ queryKey: ["mc-players", id] }),
        400,
      );
    } catch (e) {
      toast("error", e instanceof Error ? e.message : `${action} failed`);
    }
  }

  if (isLoading || !data) {
    return (
      <div className="card-b">
        <Spinner />
      </div>
    );
  }

  const online = data.online ?? [];
  const ops = new Set((data.ops ?? []).map((p) => p.name));

  return (
    <div className="card-b" style={{ display: "flex", flexDirection: "column", gap: "var(--gap)" }}>
      {!running && (
        <div className="small muted row">
          <Icon name="warning" size={13} />
          Player management commands need the server running; lists below are read from
          the server's files.
        </div>
      )}

      <section>
        <div className="label" style={{ marginBottom: 6 }}>
          Online · {online.length} / {data.maxPlayers || server.maxPlayers || "—"}
        </div>
        {online.length === 0 ? (
          <div className="small faint">Nobody is online.</div>
        ) : (
          <table className="table">
            <thead>
              <tr>
                <th>Player</th>
                <th>Role</th>
                <th className="r">Actions</th>
              </tr>
            </thead>
            <tbody>
              {online.map((p) => (
                <tr key={p}>
                  <td style={{ fontWeight: 550 }}>{p}</td>
                  <td>
                    {ops.has(p) ? (
                      <span className="badge accent">op</span>
                    ) : (
                      <span className="small faint">player</span>
                    )}
                  </td>
                  <td className="r">
                    <div className="row" style={{ justifyContent: "flex-end", gap: 4 }}>
                      {ops.has(p) ? (
                        <button className="btn btn-sm" onClick={() => playerAction("deop", p)} disabled={!running}>
                          De-op
                        </button>
                      ) : (
                        <button className="btn btn-sm" onClick={() => playerAction("op", p)} disabled={!running}>
                          Op
                        </button>
                      )}
                      <button className="btn btn-sm" onClick={() => playerAction("kick", p)} disabled={!running}>
                        Kick
                      </button>
                      <button className="btn btn-sm btn-danger" onClick={() => playerAction("ban", p)} disabled={!running}>
                        Ban
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      <section>
        <div className="row" style={{ marginBottom: 6 }}>
          <span className="label">
            Whitelist · {(data.whitelist ?? []).length}
          </span>
          <span className={`badge ${data.whitelistEnabled ? "ok" : "neutral"}`}>
            {data.whitelistEnabled ? "enforced" : "not enforced"}
          </span>
        </div>
        <div className="row wrap" style={{ gap: 6 }}>
          {(data.whitelist ?? []).map((p) => (
            <span key={p.name} className="player-chip">
              {p.name}
              <button
                className="btn btn-ghost btn-sm"
                title="remove from whitelist"
                disabled={!running}
                onClick={() => playerAction("whitelist-remove", p.name)}
              >
                <Icon name="x" size={11} />
              </button>
            </span>
          ))}
          <form
            className="row"
            style={{ gap: 4 }}
            onSubmit={(e) => {
              e.preventDefault();
              const name = wlInput.trim();
              if (name) {
                void playerAction("whitelist-add", name);
                setWlInput("");
              }
            }}
          >
            <input
              className="input"
              style={{ width: 160, height: 26 }}
              placeholder="add player…"
              value={wlInput}
              disabled={!running}
              onChange={(e) => setWlInput(e.target.value)}
              spellCheck={false}
            />
            <button className="btn btn-sm" type="submit" disabled={!running || !wlInput.trim()}>
              <Icon name="plus" size={12} />
              Add
            </button>
          </form>
        </div>
      </section>

      <div className="grid" style={{ gridTemplateColumns: "1fr 1fr" }}>
        <section>
          <div className="label" style={{ marginBottom: 6 }}>
            Operators · {(data.ops ?? []).length}
          </div>
          {(data.ops ?? []).length === 0 ? (
            <div className="small faint">No operators.</div>
          ) : (
            <div className="mini-rows">
              {(data.ops ?? []).map((p) => (
                <div key={p.name} className="mini-row">
                  <span>{p.name}</span>
                  {p.level ? <span className="small faint">level {p.level}</span> : null}
                  <button
                    className="btn btn-ghost btn-sm right"
                    disabled={!running}
                    onClick={() => playerAction("deop", p.name)}
                  >
                    De-op
                  </button>
                </div>
              ))}
            </div>
          )}
        </section>
        <section>
          <div className="label" style={{ marginBottom: 6 }}>
            Banned · {(data.banned ?? []).length}
          </div>
          {(data.banned ?? []).length === 0 ? (
            <div className="small faint">No bans.</div>
          ) : (
            <div className="mini-rows">
              {(data.banned ?? []).map((p) => (
                <div key={p.name} className="mini-row">
                  <span>{p.name}</span>
                  {p.reason && <span className="small faint truncate">{p.reason}</span>}
                  <button
                    className="btn btn-ghost btn-sm right"
                    disabled={!running}
                    onClick={() => playerAction("pardon", p.name)}
                  >
                    Pardon
                  </button>
                </div>
              ))}
            </div>
          )}
        </section>
      </div>

    </div>
  );
}
