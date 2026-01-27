import { $ } from "bun";

const BUILD_DIR = process.env.BUILD_DIR || "build";

async function main(): Promise<void> {
  console.log("Cleaning build artifacts...");

  // Remove build directory
  await $`rm -rf ${BUILD_DIR}`.nothrow();

  // Run go clean
  await $`go clean`.nothrow();

  console.log("Clean complete!");
}

main().catch((err) => {
  console.error("Clean failed:", err.message);
  process.exit(1);
});
