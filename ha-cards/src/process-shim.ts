declare global {
  var process: { env: { NODE_ENV: string } } | undefined;
}

var process = globalThis.process || { env: { NODE_ENV: "production" } };
globalThis.process = process;

export {};
