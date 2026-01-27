import { $ } from "bun";

const component = Bun.argv[2];

async function main(): Promise<void> {
  if (!component || !["server", "client"].includes(component)) {
    console.error("Usage: bun run scripts/run.ts <server|client>");
    process.exit(1);
  }

  console.log(`Running ${component}...\n`);

  await $`go run ./cmd/ottoman ${component} run`;
}

main().catch((err) => {
  console.error(`Failed to run ${component}:`, err.message);
  process.exit(1);
});
