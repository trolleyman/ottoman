import { $ } from "bun";
import { parseArgs } from "util";

const { values } = parseArgs({
  args: Bun.argv.slice(2),
  options: {
    target: { type: "string" },
    host: { type: "string" },
  },
});

const BUILD_DIR = process.env.BUILD_DIR || "build";

async function deployToPi(host: string): Promise<void> {
  if (!host) {
    console.error("Usage: bun run scripts/deploy.ts --target pi --host pi@raspberrypi.local");
    process.exit(1);
  }

  console.log(`Deploying to Raspberry Pi (${host})...\n`);

  // Build for Pi first
  console.log("Building for Raspberry Pi...");
  await $`bun run scripts/build.ts --target pi`;

  const binaryPath = `${BUILD_DIR}/ottoman-linux-arm`;

  // Copy to target
  console.log(`\nCopying to ${host}...`);
  await $`scp ${binaryPath} ${host}:/tmp/ottoman`;

  // Install on target
  console.log("\nInstalling on target...");
  const installScript = `
    sudo mv /tmp/ottoman /usr/local/bin/ottoman && \
    sudo chmod +x /usr/local/bin/ottoman && \
    sudo /usr/local/bin/ottoman server install && \
    sudo systemctl restart ottoman-server
  `;

  await $`ssh ${host} ${installScript}`;

  console.log("\nDeployment complete!");
}

async function deployClient(): Promise<void> {
  console.log("Deploying client locally...\n");

  // Build for current platform
  await $`bun run scripts/build.ts`;

  // Run install command
  const ext = process.platform === "win32" ? ".exe" : "";
  await $`${BUILD_DIR}/ottoman${ext} client install`;

  console.log("\nClient deployment complete!");
}

async function main(): Promise<void> {
  if (!values.target) {
    console.error("Usage: bun run scripts/deploy.ts --target <pi|client> [--host user@host]");
    process.exit(1);
  }

  switch (values.target) {
    case "pi":
      await deployToPi(values.host || "");
      break;
    case "client":
      await deployClient();
      break;
    default:
      console.error(`Unknown target: ${values.target}`);
      console.error("Available targets: pi, client");
      process.exit(1);
  }
}

main().catch((err) => {
  console.error("Deployment failed:", err.message);
  process.exit(1);
});
