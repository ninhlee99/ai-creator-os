import { copyFileSync, mkdirSync } from "node:fs";

const source = "node_modules/htmx.org/dist/htmx.min.js";
const target = "internal/web/static/htmx.min.js";
mkdirSync("internal/web/static", { recursive: true });
copyFileSync(source, target);
copyFileSync("node_modules/htmx.org/LICENSE", "internal/web/static/htmx.LICENSE");
