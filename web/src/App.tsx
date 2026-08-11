import { useEffect, useState } from "react";
import { createHashRouter, RouterProvider } from "react-router-dom";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { api, setUnauthorizedHandler } from "./api/client";
import { startLive, stopLive } from "./state/live";
import { loadSystem } from "./state/system";
import { PrefsProvider } from "./state/prefs";
import { ConfirmProvider } from "./ui/Confirm";
import { ToastProvider } from "./ui/Toast";
import { Layout } from "./shell/Layout";
import { Login } from "./pages/Login";
import { Dashboard } from "./pages/Dashboard";
import { StoragePage } from "./pages/Storage";
import { ServicesPage } from "./pages/Services";
import { MinecraftPage } from "./pages/Minecraft";
import { MCServerPage } from "./pages/McServer";
import { PlexPage } from "./pages/Plex";
import { ActivityPage } from "./pages/Activity";
import { SettingsPage } from "./pages/Settings";
import { Spinner } from "./ui/bits";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: { retry: 1, refetchOnWindowFocus: false, staleTime: 5000 },
  },
});

// Hash router keeps deep links working no matter how the panel is proxied.
const router = createHashRouter([
  {
    element: <Layout />,
    children: [
      { path: "/", element: <Dashboard /> },
      { path: "/storage", element: <StoragePage /> },
      { path: "/services", element: <ServicesPage /> },
      { path: "/minecraft", element: <MinecraftPage /> },
      { path: "/minecraft/:id", element: <MCServerPage /> },
      { path: "/plex", element: <PlexPage /> },
      { path: "/activity", element: <ActivityPage /> },
      { path: "/settings", element: <SettingsPage /> },
      { path: "*", element: <Dashboard /> },
    ],
  },
]);

type AuthState = "checking" | "login" | "ready";

export default function App() {
  const [authState, setAuthState] = useState<AuthState>("checking");

  useEffect(() => {
    setUnauthorizedHandler(() => {
      stopLive();
      setAuthState("login");
    });
    api<{ authRequired: boolean; authenticated: boolean }>("/api/auth/session")
      .then((s) => {
        setAuthState(!s.authRequired || s.authenticated ? "ready" : "login");
      })
      .catch(() => setAuthState("login"));
  }, []);

  useEffect(() => {
    if (authState === "ready") {
      startLive();
      loadSystem();
    }
  }, [authState]);

  if (authState === "checking") {
    return (
      <div className="login-page">
        <Spinner size={22} />
      </div>
    );
  }
  if (authState === "login") {
    return <Login onSuccess={() => setAuthState("ready")} />;
  }
  return (
    <QueryClientProvider client={queryClient}>
      <PrefsProvider>
        <ToastProvider>
          <ConfirmProvider>
            <RouterProvider router={router} />
          </ConfirmProvider>
        </ToastProvider>
      </PrefsProvider>
    </QueryClientProvider>
  );
}
