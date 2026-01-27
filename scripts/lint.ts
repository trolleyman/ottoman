import { $ } from "bun";

async function main(): Promise<void> {
  console.log("Running linter...\n");

  // Check if golangci-lint is available
  const { exitCode } = await $`which golangci-lint`.nothrow().quiet();

  if (exitCode !== 0) {
    console.log("golangci-lint not found, using go vet instead...\n");
    await $`go vet ./...`;
  } else {
    await $`golangci-lint run`;
  }

  console.log("\nLint complete!");
}

main().catch((err) => {
  console.error("Lint failed:", err.message);
  process.exit(1);
});
