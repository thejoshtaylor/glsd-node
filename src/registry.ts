/**
 * Registry parser for project directory switching.
 *
 * Reads a registry.md file (configurable via PROJECT_REGISTRY_PATH env var)
 * and extracts project entries from the markdown table.
 *
 * If no registry exists, auto-creates one with the current CLAUDE_WORKING_DIR.
 */

import { readFileSync, writeFileSync, mkdirSync, existsSync } from "fs";
import { resolve, dirname } from "path";
import { WORKING_DIR, ALLOWED_PATHS } from "./config";

export interface Project {
  name: string;
  type: string;
  status: string;
  location: string;
  description: string;
}

function getRegistryPath(): string {
  if (process.env.PROJECT_REGISTRY_PATH) {
    return resolve(process.env.PROJECT_REGISTRY_PATH);
  }
  // Default: registry.md next to the bot's working directory config
  return resolve(WORKING_DIR, "registry.md");
}

/**
 * Create a default registry.md with the current working directory as the first project.
 */
function createDefaultRegistry(registryPath: string): void {
  const dirName = WORKING_DIR.replace(/\\/g, "/").split("/").pop() || "default";
  const content = `# Project Registry

| Name | Type | Status | Location | Description |
|------|------|--------|----------|-------------|
| ${dirName} | Project | Active | ${WORKING_DIR.replace(/\\/g, "/")} | Default project |
`;
  mkdirSync(dirname(registryPath), { recursive: true });
  writeFileSync(registryPath, content, "utf-8");
  console.log(`Created default registry at ${registryPath}`);
}

/**
 * Parse registry.md markdown table into Project objects.
 * Returns projects sorted: Active first, then alphabetically by name.
 *
 * If no registry file exists, creates a default one.
 */
export function parseRegistry(): Project[] {
  const registryPath = getRegistryPath();

  if (!existsSync(registryPath)) {
    createDefaultRegistry(registryPath);
  }

  let content: string;
  try {
    content = readFileSync(registryPath, "utf-8");
  } catch (error) {
    console.error(`Failed to read registry: ${error}`);
    return [];
  }

  const projects: Project[] = [];

  // Find the markdown table rows (skip header and separator lines)
  const lines = content.split(/\r?\n/);
  let inTable = false;
  let headerSkipped = false;

  for (const line of lines) {
    const trimmed = line.trim();

    // Detect table start by looking for the header separator (|---|---|...)
    if (trimmed.match(/^\|[\s-]+\|/)) {
      inTable = true;
      headerSkipped = true;
      continue;
    }

    // Skip the header row (comes before separator)
    if (!headerSkipped && trimmed.startsWith("| Name")) {
      continue;
    }

    // Parse table rows
    if (inTable && trimmed.startsWith("|")) {
      const cells = trimmed
        .split("|")
        .map((c) => c.trim())
        .filter(Boolean);

      if (cells.length >= 5) {
        projects.push({
          name: cells[0]!,
          type: cells[1]!,
          status: cells[2]!,
          location: cells[3]!.replace(/\\/g, "/"),
          description: cells[4]!,
        });
      }
    }

    // End of table (empty line after table rows)
    if (inTable && !trimmed.startsWith("|") && trimmed === "") {
      break;
    }
  }

  // Sort: Active first, then alphabetically by name
  projects.sort((a, b) => {
    const aActive = a.status === "Active" ? 0 : 1;
    const bActive = b.status === "Active" ? 0 : 1;
    if (aActive !== bActive) return aActive - bActive;
    return a.name.localeCompare(b.name);
  });

  return projects;
}

/**
 * Check if a path is under one of the allowed paths.
 */
export function isUnderAllowedPath(targetPath: string): boolean {
  const normalized = resolve(targetPath).replace(/\\/g, "/").toLowerCase();
  return ALLOWED_PATHS.some((allowed) => {
    const normalizedAllowed = resolve(allowed).replace(/\\/g, "/").toLowerCase();
    return normalized.startsWith(normalizedAllowed);
  });
}

/**
 * Get allowed paths that can serve as parent directories for new projects.
 */
export function getAllowedParentPaths(): string[] {
  return ALLOWED_PATHS.map((p) => resolve(p).replace(/\\/g, "/"));
}

/**
 * Add a new project to the registry and create its directory.
 * If the directory already exists, it is reused. If the path is already
 * in the registry, the existing entry is kept (no duplicate row).
 * Returns the full path to the project.
 */
export function addProject(parentPath: string, projectName: string): string {
  const safeName = projectName.replace(/[^a-zA-Z0-9_\-. ]/g, "").trim();
  if (!safeName) throw new Error("Invalid project name");

  const fullPath = resolve(parentPath, safeName).replace(/\\/g, "/");

  if (!isUnderAllowedPath(fullPath)) {
    throw new Error("Path is not under an allowed directory");
  }

  // Create directory (no-op if it already exists)
  mkdirSync(fullPath, { recursive: true });

  // Check if already in registry before adding
  const existing = parseRegistry();
  const alreadyRegistered = existing.some(
    (p) => p.location.replace(/\\/g, "/").toLowerCase() === fullPath.toLowerCase()
  );

  if (!alreadyRegistered) {
    const registryPath = getRegistryPath();
    if (!existsSync(registryPath)) {
      createDefaultRegistry(registryPath);
    }

    let content = readFileSync(registryPath, "utf-8");
    const newRow = `| ${safeName} | Project | Active | ${fullPath} | New project |`;
    content = content.trimEnd() + "\n" + newRow + "\n";
    writeFileSync(registryPath, content, "utf-8");
    console.log(`Added project "${safeName}" at ${fullPath}`);
  } else {
    console.log(`Project at ${fullPath} already in registry, reusing`);
  }

  return fullPath;
}
