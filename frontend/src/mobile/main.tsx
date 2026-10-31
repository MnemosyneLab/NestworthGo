import React from "react";
import ReactDOM from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { initializeMobileI18n } from "./i18n";
import "@/index.css";
import "./mobile.css";
import { MobileApp } from "./MobileApp";

const scheme = window.matchMedia("(prefers-color-scheme: dark)");
const applyScheme = () =>
  document.documentElement.classList.toggle("dark", scheme.matches);
applyScheme();
scheme.addEventListener("change", applyScheme);

const client = new QueryClient({
  defaultOptions: {
    queries: { retry: false, staleTime: 15000, refetchOnWindowFocus: false },
    mutations: { retry: false },
  },
});
void initializeMobileI18n().then(() => {
  ReactDOM.createRoot(document.getElementById("root")!).render(
    <React.StrictMode>
      <QueryClientProvider client={client}>
        <MobileApp />
      </QueryClientProvider>
    </React.StrictMode>,
  );
});
