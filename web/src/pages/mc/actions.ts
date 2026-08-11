// Shared Minecraft lifecycle actions with the confirmation flows attached.

import { useState } from "react";
import { api } from "../../api/client";
import { useConfirm } from "../../ui/Confirm";
import { useToast } from "../../ui/Toast";

export function useMCActions(id: string) {
  const confirm = useConfirm();
  const toast = useToast();
  const [busy, setBusy] = useState<string | null>(null);

  async function run(verb: string, confirmValue?: string) {
    setBusy(verb);
    try {
      await api(`/api/minecraft/${encodeURIComponent(id)}/${verb}`, {
        method: "POST",
        confirm: confirmValue,
      });
      toast("ok", `${id}: ${verb} issued`);
      return true;
    } catch (e) {
      toast("error", e instanceof Error ? e.message : `${verb} failed`);
      return false;
    } finally {
      setBusy(null);
    }
  }

  return {
    busy,
    start: () => run("start"),
    stop: async () => {
      const ok = await confirm({
        title: `Stop ${id}`,
        target: id,
        body: "Players will be disconnected. The world is saved before shutdown.",
        confirmLabel: "Stop server",
      });
      if (ok) await run("stop", id);
    },
    restart: async () => {
      const ok = await confirm({
        title: `Restart ${id}`,
        target: id,
        body: "Players will be disconnected while the server restarts.",
        confirmLabel: "Restart server",
      });
      if (ok) await run("restart", id);
    },
    kill: async () => {
      const ok = await confirm({
        title: `Force-kill ${id}`,
        target: id,
        typed: true,
        body: "SIGKILL skips world saving — recent changes may be lost or chunks corrupted. Only use this when the server is unresponsive.",
        confirmLabel: "Kill process",
      });
      if (ok) await run("kill", id);
    },
  };
}
