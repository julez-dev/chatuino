export type OS = "linux" | "macos" | "windows" | "unknown";

export function detectOS(): OS {
  if (typeof navigator === "undefined") return "unknown";

  const userAgent = navigator.userAgent.toLowerCase();
  const platform = navigator.platform?.toLowerCase() ?? "";

  if (platform.includes("win") || userAgent.includes("windows")) {
    return "windows";
  }

  if (platform.includes("mac") || userAgent.includes("mac")) {
    return "macos";
  }

  if (
    platform.includes("linux") ||
    userAgent.includes("linux") ||
    userAgent.includes("x11")
  ) {
    return "linux";
  }

  return "unknown";
}

export function getOSDisplayName(os: OS): string {
  switch (os) {
    case "linux":
      return "Linux";
    case "macos":
      return "macOS";
    case "windows":
      return "Windows";
    default:
      return "Your OS";
  }
}
