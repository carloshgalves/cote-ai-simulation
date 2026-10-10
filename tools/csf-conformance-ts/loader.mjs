// The runner intentionally uses the JavaScript-compatible TypeScript subset.
// This tiny local loader avoids a network-installed compiler in conformance CI.
import { readFile } from "node:fs/promises";

export async function load(url, context, nextLoad) {
  if (url.endsWith(".ts")) {
    return { format: "module", shortCircuit: true, source: await readFile(new URL(url), "utf8") };
  }
  return nextLoad(url, context);
}
