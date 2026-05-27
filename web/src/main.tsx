import React from "react";
import ReactDOM from "react-dom/client";
import "@mantine/core/styles.css";
import "./index.css";
import "./i18n";
import { MantineProvider, createTheme } from "@mantine/core";
import App from "./App";

const theme = createTheme({ primaryColor: "indigo" });

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <MantineProvider theme={theme} defaultColorScheme="dark">
      <App />
    </MantineProvider>
  </React.StrictMode>
);
