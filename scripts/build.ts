import { $ } from "bun";
import { parseArgs } from "util";

const { values } = parseArgs({
  args: Bun.argv.slice(2),
  options: {
    target: { type: "string" },
    all: { type: "boolean", default: false },
  },
});

const BUILD_DIR = process.env.BUILD_DIR || "build";
const BINARY = "ottoman";

// Get version from git
async function getVersion(): Promise<string> {
  try {
    const result = await $`git describe --tags --always --dirty`.quiet().text();
    return result.trim() || "dev";
  } catch {
    return "dev";
  }
}

interface BuildTarget {
  name: string;
  goos: string;
  goarch: string;
  goarm?: string;
  output: string;
}

const targets: Record<string, BuildTarget> = {
  pi: {
    name: "Raspberry Pi",
    goos: "linux",
    goarch: "arm",
    goarm: "7",
    output: `${BINARY}-linux-arm`,
  },
  windows: {
    name: "Windows",
    goos: "windows",
    goarch: "amd64",
    output: `${BINARY}-windows-amd64.exe`,
  },
  linux: {
    name: "Linux",
    goos: "linux",
    goarch: "amd64",
    output: `${BINARY}-linux-amd64`,
  },
};

async function build(target: BuildTarget, version: string): Promise<void> {
  console.log(`Building for ${target.name} (${target.goos}/${target.goarch})...`);

  const ldflags = `-X main.Version=${version}`;
  const outputPath = `${BUILD_DIR}/${target.output}`;

  const env: Record<string, string> = {
    ...process.env as Record<string, string>,
    GOOS: target.goos,
    GOARCH: target.goarch,
  };

  if (target.goarm) {
    env.GOARM = target.goarm;
  }

  await $`go build -ldflags ${ldflags} -o ${outputPath} ./cmd/ottoman`.env(env);

  console.log(`Built: ${outputPath}`);
}

async function buildCurrent(version: string): Promise<void> {
  console.log("Building for current platform...");

  const ldflags = `-X main.Version=${version}`;
  const ext = process.platform === "win32" ? ".exe" : "";
  const outputPath = `${BUILD_DIR}/${BINARY}${ext}`;

  await $`go build -ldflags ${ldflags} -o ${outputPath} ./cmd/ottoman`;

  console.log(`Built: ${outputPath}`);
}

async function main(): Promise<void> {
  // Ensure build directory exists
  await $`mkdir -p ${BUILD_DIR}`.nothrow();

  const version = await getVersion();
  console.log(`Version: ${version}\n`);

  if (values.all) {
    // Build all targets
    for (const target of Object.values(targets)) {
      await build(target, version);
    }
  } else if (values.target) {
    // Build specific target
    const target = targets[values.target];
    if (!target) {
      console.error(`Unknown target: ${values.target}`);
      console.error(`Available targets: ${Object.keys(targets).join(", ")}`);
      process.exit(1);
    }
    await build(target, version);
  } else {
    // Build for current platform
    await buildCurrent(version);
  }

  console.log("\nBuild complete!");
}

main().catch((err) => {
  console.error("Build failed:", err.message);
  process.exit(1);
});
