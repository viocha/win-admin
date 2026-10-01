import { useEffect, useState } from "react"
import { FolderTreeIcon, TerminalSquareIcon } from "lucide-react"

import { ContextMenuService } from "../../../bindings/win-admin"
import { cn } from "@/lib/utils"

type MenuIconPreviewProps = {
  icon: string
  command: string
  kind: string
  className?: string
}

export function MenuIconPreview({ icon, command, kind, className }: MenuIconPreviewProps) {
  const [source, setSource] = useState("")
  const hasExplicitIcon = Boolean(icon.trim())
  const iconSource = icon.trim() || commandExecutable(command)

  useEffect(() => {
    let cancelled = false

    if (!iconSource) {
      setSource("")
      return
    }

    void ContextMenuService.IconPreview(iconSource, "")
      .then((result) => {
        if (!cancelled) setSource(result || "")
      })
      .catch(() => {
        if (!cancelled) setSource("")
      })

    return () => {
      cancelled = true
    }
  }, [iconSource])

  if (source) {
    return (
      <img
        src={source}
        alt=""
        draggable={false}
        className={cn("size-5 shrink-0 object-contain", !hasExplicitIcon && "opacity-50", className)}
      />
    )
  }

  const FallbackIcon = kind === "submenu" ? FolderTreeIcon : TerminalSquareIcon
  return <FallbackIcon className={cn("size-5 shrink-0 text-muted-foreground", className)} />
}

// 与后端 splitCommandExecutable 保持一致，图标来源只取命令中的程序名。
function commandExecutable(command: string): string {
  const value = command.trim()
  if (value.startsWith('"')) {
    const end = value.indexOf('"', 1)
    if (end >= 0) return value.slice(1, end).trim()
  }
  return value.split(/[ \t\r\n]/, 1)[0].replace(/^"+|"+$/g, "")
}

