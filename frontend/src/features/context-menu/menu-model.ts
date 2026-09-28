import type { MenuEntry } from "../../../bindings/win-admin"

export type TargetType = "file" | "directory" | "background" | "desktop-background"
export type MenuKind = "command" | "submenu"

export const TARGETS: Array<{ value: TargetType; label: string }> = [
  { value: "file", label: "文件" },
  { value: "directory", label: "目录" },
  { value: "background", label: "目录背景" },
  { value: "desktop-background", label: "桌面背景" },
]

export const TARGET_LABEL: Record<string, string> = {
  file: "文件",
  directory: "目录",
  background: "目录背景",
  "desktop-background": "桌面背景",
}

export function shellMenuDisplayLabel(value: string): string {
  let result = ""
  for (let index = 0; index < value.length; index += 1) {
    const character = value[index]
    if (character !== "&") {
      result += character
      continue
    }
    if (value[index + 1] === "&") {
      result += "&"
      index += 1
    }
  }
  return result
}

export function createEmptyEntry(target: TargetType, kind: MenuKind = "command"): MenuEntry {
  return {
    id: "",
    label: "",
    displayLabel: "",
    target,
    kind,
    command: "",
    commandTemplate: "",
    icon: "",
    position: "normal",
	extensions: "",
	shiftOnly: false,
    enabled: true,
    managed: true,
    readOnly: false,
	canEdit: true,
	canEnable: true,
	canPosition: true,
	canShift: true,
	canDelete: true,
    registryPath: "",
    children: [],
  }
}

export function normalizeEntry(entry: MenuEntry): MenuEntry {
  return {
    ...entry,
    commandTemplate: entry.command,
    children: (entry.children ?? []).map(normalizeEntry),
  }
}

export function cloneEntry(entry: MenuEntry): MenuEntry {
  return normalizeEntry(structuredClone(entry))
}

export function countChildren(entry: MenuEntry): number {
  return (entry.children ?? []).reduce((total, child) => total + 1 + countChildren(child), 0)
}

export function validateEntry(entry: MenuEntry): string | null {
  if (!entry.label.trim()) {
    return "菜单名称不能为空"
  }
  if (entry.kind === "command" && !entry.command.trim()) {
    return `“${entry.label}”缺少命令`
  }
  if (entry.kind === "submenu") {
    if (!(entry.children ?? []).length) {
      return `“${entry.label}”至少需要一个子项`
    }
    for (const child of entry.children ?? []) {
      const error = validateEntry(child)
      if (error) return error
    }
  }
  return null
}

