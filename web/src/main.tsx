import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { captureToken } from "./lib/api";
import { App } from "./App";
import "./styles.css";

if (location.hash.includes("token=")) {
  captureToken(location.hash);
  history.replaceState(null, "", location.pathname + "#/workspaces");
}

const root = document.getElementById("root");
if (root) {
  createRoot(root).render(
    <StrictMode>
      <App />
    </StrictMode>,
  );
}
