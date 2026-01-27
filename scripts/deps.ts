import { $ } from "bun";

async function main(): Promise<void> {
  console.log("Installing Go dependencies...\n");

  await $`go mod tidy`;

  console.log("\nDependencies installed!");
}

main().catch((err) => {
  console.error("Failed to install dependencies:", err.message);
  process.exit(1);
});
