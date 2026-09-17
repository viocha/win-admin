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

  useEffect(() => {
    let cancelled = false
    setSource("")

    if (!icon.trim() && !command.trim()) return () => undefined

    void ContextMenuService.IconPreview(icon, command)
      .then((result) => {
        if (!cancelled) setSource(result || "")
      })
      .catch(() => {
        if (!cancelled) setSource("")
      })

    return () => {
      cancelled = true
    }
  }, [command, icon])

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

