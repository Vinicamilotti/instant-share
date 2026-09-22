import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter, Routes, Route } from "react-router";
import { Home } from "./pages/Home";
import { Host } from "./pages/Host";
import { Guest } from "./pages/Guest";
import "./index.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<Home />} />
        <Route path="/host/:sessionId" element={<Host />} />
        <Route path="/:sessionId" element={<Guest />} />
      </Routes>
    </BrowserRouter>
  </React.StrictMode>
);