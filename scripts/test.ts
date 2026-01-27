import { $ } from "bun";

async function main(): Promise<void> {
  console.log("Running tests...\n");

  await $`go test -v ./...`;

  console.log("\nTests complete!");
}

main().catch((err) => {
  console.error("Tests failed:", err.message);
  process.exit(1);
});
